# ---------------------------------------------------------------------------
# Completion: autocompletado de 'wt' via Register-ArgumentCompleter.
# Depende de: Common, Config, Repo, Workspace, Cli (Get-WtCommandSpecs,
# Resolve-WtCommandName). Se carga al final: es el unico archivo que necesita
# toda la superficie del parser ya definida.
# ---------------------------------------------------------------------------

function Get-WtCommandCompletionNames {
    <#
    .SYNOPSIS
        Nombres canonicos + alias 'de palabra' (sin --version/-h/'/?') de todos los
        comandos. Funcion pura.
    #>
    param([Parameter(Mandatory)]$Specs)
    $names = @()
    foreach ($key in $Specs.Keys) {
        $names += $key
        foreach ($alias in $Specs[$key].Aliases) {
            if ($alias -match '^[A-Za-z][A-Za-z-]*$') { $names += $alias }
        }
    }
    return @($names | Sort-Object -Unique)
}

function Get-WtConfigValueCandidates {
    <#
    .SYNOPSIS
        Valores admitidos para una clave de config, cuando es una enumeracion cerrada
        (M5). Array vacio para claves libres (editor, defaultBase, branchPrefix, ...) o
        desconocidas: no hay nada que sugerir. Funcion pura.
    #>
    param([Parameter(Mandatory)][AllowEmptyString()][string]$Key)
    switch ($Key) {
        'terminal' { return @('warp', 'wt', 'none') }
        'warpAgentTarget' { return @('auto', 'tab', 'window') }
        { $_ -in 'warpAgentColor', 'warpTerminalColor' } { return @(Get-WtKnownWarpColors) }
        'openOnCreate' { return @('all', 'editor', 'none') }
        'agentShell' { return @('powershell', 'bash', 'none') }
        'fetchBeforeCreate' { return @('true', 'false') }
        default { return @() }
    }
}

function Get-WtCompletion {
    <#
    .SYNOPSIS
        Candidatos de autocompletado para una linea 'wt <Words...><WordToComplete>'.
        Funcion pura: recibe el contexto (worktrees/repos disponibles) por parametro
        en vez de resolverlo por su cuenta, para que sea testeable sin git ni disco.
    .DESCRIPTION
        - Sin palabras previas: nombres de comando y alias.
        - Token que empieza con '--': flags y valores admitidos de ESE comando.
        - 'config get|set': claves conocidas en la 2da posicion; 'config set <clave>':
          los valores admitidos de esa clave si es una enumeracion cerrada (M5).
        - 2da posicion de open/path/remove/cd/lock/unlock/status/exec: worktrees del
          repo actual (si $Worktrees trae algo) o, si no, repos de reposRoot.
        - Comando desconocido: lista vacia (no revienta el completer del usuario).
    .OUTPUTS
        Array de strings candidatos, ya filtrados por $WordToComplete (prefijo,
        case-insensitive) y ordenados.
    #>
    param(
        [AllowEmptyCollection()][string[]]$Words = @(),
        [AllowEmptyString()][string]$WordToComplete = '',
        [AllowEmptyCollection()][string[]]$Worktrees = @(),
        [AllowEmptyCollection()][string[]]$Repos = @()
    )
    $specs = Get-WtCommandSpecs
    $matchesPrefix = { param($candidate) $candidate.StartsWith($WordToComplete, [System.StringComparison]::OrdinalIgnoreCase) }

    if (@($Words).Count -eq 0) {
        $names = Get-WtCommandCompletionNames -Specs $specs
        return @($names | Where-Object { & $matchesPrefix $_ } | Sort-Object)
    }

    $commandName = ''
    try { $commandName = Resolve-WtCommandName -Token $Words[0] } catch { return @() }
    $spec = $specs[$commandName]
    $rest = @($Words | Select-Object -Skip 1)

    if ($WordToComplete.StartsWith('--')) {
        $prefix = $WordToComplete.Substring(2)
        $flagNames = @(@($spec.Flags) + @($spec.Values))
        return @($flagNames | Where-Object { $_.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase) } |
            ForEach-Object { "--$_" } | Sort-Object)
    }

    if ($commandName -eq 'config') {
        if ($rest.Count -eq 0) {
            return @(@('get', 'set', 'list', 'path', 'edit') | Where-Object { & $matchesPrefix $_ } | Sort-Object)
        }
        $action = $rest[0].ToLowerInvariant()
        if ($rest.Count -eq 1 -and $action -in 'get', 'set') {
            return @(Get-WtConfigKeys | Where-Object { & $matchesPrefix $_ } | Sort-Object)
        }
        if ($rest.Count -eq 2 -and $action -eq 'set') {
            $candidates = @(Get-WtConfigValueCandidates -Key $rest[1])
            return @($candidates | Where-Object { & $matchesPrefix $_ } | Sort-Object)
        }
        return @()
    }

    $targetCommands = @('open', 'path', 'remove', 'cd', 'lock', 'unlock', 'status', 'exec')
    if ($targetCommands -contains $commandName -and $rest.Count -eq 0) {
        $candidates = $Worktrees
        if (@($candidates).Count -eq 0) { $candidates = $Repos }
        return @($candidates | Where-Object { & $matchesPrefix $_ } | Sort-Object)
    }

    return @()
}

function Register-WtCompletion {
    <#
    .SYNOPSIS
        Registra el autocompletado de 'wt'/'wtm'. Se invoca UNA vez desde el bloque
        del perfil que instala install.ps1 (no al importar el modulo: importar no
        debe tener efectos secundarios, wt.psm1 lo declara).
    .DESCRIPTION
        Presupuesto de tiempo: nunca dispara git. Los worktrees salen de
        Get-WtWorktreesCached (M2/B5: solo si ya estan en la cache de la invocacion
        anterior de 'wt' en esta sesion; si no, vacio) y los repos de un recorrido de
        filesystem (Get-WtRepoDirs), que no invoca ningun subproceso.
    #>
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '',
        Justification = 'Registra un completer en la sesion actual; no es un cambio de estado de dominio.')]
    param()
    $scriptBlock = {
        # param($wordToComplete, $commandAst, $cursorPosition): firma fija que exige
        # Register-ArgumentCompleter para comandos nativos; cursorPosition no hace
        # falta aca, pero hay que declararlo (es posicional) - se referencia una vez
        # como no-op para que el linter no lo marque como no usado.
        param($wordToComplete, $commandAst, $cursorPosition)
        $null = $cursorPosition
        $elements = @($commandAst.CommandElements | Select-Object -Skip 1 | ForEach-Object { $_.Extent.Text })
        if ($elements.Count -gt 0 -and $elements[-1] -eq $wordToComplete) {
            $elements = @($elements | Select-Object -SkipLast 1)
        }
        $worktrees = @()
        $repos = @()
        try {
            $repoRoot = Find-WtMainRoot -Silent
            if ($repoRoot) {
                $worktrees = @(Get-WtWorktreesCached -RepoRoot $repoRoot | ForEach-Object { Split-Path -Leaf $_.Path })
            } else {
                $config = Get-WtConfig
                $root = @(ConvertTo-WtReposRootList -Value $config.reposRoot)
                if ($root.Count -gt 0) {
                    $depth = Get-WtReposDepth -Config $config
                    $repos = @(Get-WtRepoDirs -Root $root -Depth $depth | ForEach-Object { $_.Name })
                }
            }
        } catch {
            # Un completer que revienta la terminal es peor que uno vacio: cualquier
            # error al armar el contexto se ignora a proposito.
            Write-Debug "Get-WtCompletion: contexto vacio ($($_.Exception.Message))"
        }
        $candidates = @(Get-WtCompletion -Words $elements -WordToComplete $wordToComplete -Worktrees $worktrees -Repos $repos)
        return @($candidates | ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) })
    }
    Register-ArgumentCompleter -CommandName 'wt' -ScriptBlock $scriptBlock
    Register-ArgumentCompleter -CommandName 'wtm' -ScriptBlock $scriptBlock
}
