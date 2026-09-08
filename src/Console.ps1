# ---------------------------------------------------------------------------
# Console: menu interactivo.
# Wrapper fino sobre Commands/Workspace: arma el comando CLI equivalente, lo
# muestra y llama a la misma funcion que usa el CLI. No duplica logica.
# Depende de: Common, Config, Repo, Workspace, Commands.
# ---------------------------------------------------------------------------

function Read-WtConsoleLine {
    <#
    .SYNOPSIS
        Lee una linea. Devuelve $null ante EOF (stdin cerrado).
    .NOTES
        [Console]::In funciona igual en uso interactivo y con stdin pipeado, lo que
        hace la consola testeable end-to-end.
    #>
    param([string]$Prompt)
    if ($Prompt) { Write-Host -NoNewline $Prompt }
    return [Console]::In.ReadLine()
}

function Read-WtConsoleChoice {
    param([string]$Prompt)
    $line = Read-WtConsoleLine -Prompt $Prompt
    if ($null -eq $line) { return $null }
    return $line.Trim().ToLowerInvariant()
}

function Read-WtYesNo {
    <#
    .OUTPUTS
        $true / $false, o $null ante EOF.
    #>
    param([Parameter(Mandatory)][string]$Prompt, [bool]$Default = $false)
    $suffix = ' [s/N]: '
    if ($Default) { $suffix = ' [S/n]: ' }
    $answer = Read-WtConsoleLine -Prompt ($Prompt + $suffix)
    if ($null -eq $answer) { return $null }
    $answer = $answer.Trim()
    if (-not $answer) { return $Default }
    return ($answer -match '^(s|si|y|yes)$')
}

function Select-WtConsoleIndex {
    <#
    .SYNOPSIS
        Pide un numero de 1..$Count. Devuelve el indice base 0, o $null si se cancela,
        la seleccion es invalida o llego EOF.
    #>
    param([Parameter(Mandatory)][int]$Count)
    $sel = Read-WtConsoleLine -Prompt 'Numero (vacio para cancelar): '
    if ($null -eq $sel) { return $null }
    $sel = $sel.Trim()
    if (-not $sel) { return $null }
    $n = 0
    if (-not [int]::TryParse($sel, [ref]$n) -or $n -lt 1 -or $n -gt $Count) {
        Write-WtNotice 'Seleccion invalida.'
        return $null
    }
    return ($n - 1)
}

function Select-WtConsoleWorktree {
    param(
        [Parameter(Mandatory)][string]$RepoRoot,
        [Parameter(Mandatory)][string]$Title,
        [switch]$ExcludeMain
    )
    $worktrees = @(Get-WtWorktrees -RepoRoot $RepoRoot)
    if ($ExcludeMain) { $worktrees = @($worktrees | Where-Object { -not $_.IsMain }) }
    if ($worktrees.Count -eq 0) {
        Write-WtNotice 'No hay worktrees disponibles.'
        return $null
    }
    Write-WtInfo $Title
    for ($i = 0; $i -lt $worktrees.Count; $i++) {
        $wt = $worktrees[$i]
        $tag = ''
        if ($wt.IsMain) { $tag = ' (principal)' }
        if ($wt.IsLocked) { $tag = ' (bloqueado)' }
        if ($wt.IsPrunable) { $tag = ' (obsoleto)' }
        $branch = $wt.Branch
        if (-not $branch) { $branch = '(detached)' }
        Write-WtLine ("  {0}) {1}{2} [{3}]" -f ($i + 1), (Split-Path -Leaf $wt.Path), $tag, $branch)
    }
    $index = Select-WtConsoleIndex -Count $worktrees.Count
    if ($null -eq $index) { return $null }
    return $worktrees[$index]
}

function Write-WtConsoleCommand {
    param([Parameter(Mandatory)][string]$Command)
    Write-WtDetail "Ejecutando: $Command"
}

# --- Acciones ---------------------------------------------------------------

function Invoke-WtConsoleCreate {
    $name = Read-WtConsoleLine -Prompt 'Nombre del worktree: '
    if ($null -eq $name -or -not $name.Trim()) { Write-WtDetail 'Cancelado.'; return }
    $name = $name.Trim()
    $defaultBase = [string](Get-WtConfig).defaultBase
    $basePrompt = 'Base (vacio = HEAD actual): '
    if ($defaultBase) { $basePrompt = "Base [$defaultBase]: " }
    $base = Read-WtConsoleLine -Prompt $basePrompt
    if ($null -eq $base) { return }
    $base = $base.Trim()
    $cli = "wt create $name"
    if ($base) { $cli += " --base $base" }
    Write-WtConsoleCommand $cli
    New-WtWorktree -Name $name -Base $base
}

function Invoke-WtConsoleOpen {
    param([Parameter(Mandatory)][string]$RepoRoot, [switch]$All)
    $wt = Select-WtConsoleWorktree -RepoRoot $RepoRoot -Title 'Elegi el worktree a abrir:'
    if (-not $wt) { return }
    $name = Split-Path -Leaf $wt.Path
    $cli = "wt open $name"
    if ($All) { $cli += ' --all' }
    Write-WtConsoleCommand $cli
    Open-WtWorktree -Name $name -All:$All
}

function Invoke-WtConsolePath {
    param([Parameter(Mandatory)][string]$RepoRoot)
    $wt = Select-WtConsoleWorktree -RepoRoot $RepoRoot -Title 'Elegi el worktree:'
    if (-not $wt) { return }
    $name = Split-Path -Leaf $wt.Path
    Write-WtConsoleCommand "wt path $name"
    Invoke-WtPathCommand -Name $name
}

function Invoke-WtConsoleRemove {
    param([Parameter(Mandatory)][string]$RepoRoot)
    $wt = Select-WtConsoleWorktree -RepoRoot $RepoRoot -Title 'Elegi el worktree a eliminar:' -ExcludeMain
    if (-not $wt) { return }
    $name = Split-Path -Leaf $wt.Path
    $deleteBranch = Read-WtYesNo -Prompt 'Eliminar tambien la rama?'
    if ($null -eq $deleteBranch) { return }
    $force = Read-WtYesNo -Prompt 'Forzar aunque haya cambios sin commit?'
    if ($null -eq $force) { return }
    $cli = "wt remove $name"
    if ($deleteBranch) { $cli += ' --delete-branch' }
    if ($force) { $cli += ' --force' }
    $confirm = Read-WtYesNo -Prompt "Ejecutar '$cli'?"
    if ($null -eq $confirm) { return }
    if (-not $confirm) { Write-WtDetail 'Cancelado.'; return }
    Remove-WtWorktree -Name $name -DeleteBranch:$deleteBranch -Force:$force
}

function Invoke-WtConsoleGoToRepo {
    $root = Get-WtReposRoot -Config (Get-WtConfig)
    $repos = @(Get-WtRepoDirs -Root $root)
    if ($repos.Count -eq 0) {
        Write-WtNotice "No hay repos git en '$root'."
        return
    }
    Write-WtInfo 'Elegi el repo:'
    for ($i = 0; $i -lt $repos.Count; $i++) {
        Write-WtLine ("  {0}) {1}" -f ($i + 1), $repos[$i].Name)
    }
    $index = Select-WtConsoleIndex -Count $repos.Count
    if ($null -eq $index) { return }
    $name = $repos[$index].Name
    Write-WtConsoleCommand "wt cd $name"
    Invoke-WtCd -Name $name
}

function Invoke-WtConsoleConfig {
    while ($true) {
        Write-WtLine ''
        Write-WtInfo 'Configuracion'
        Write-WtLine '  1) Ver valores actuales'
        Write-WtLine '  2) Cambiar un valor'
        Write-WtLine '  3) Abrir el archivo de config en el editor'
        Write-WtLine '  4) Volver'
        $choice = Read-WtConsoleChoice -Prompt 'config> '
        if ($null -eq $choice) { return }
        try {
            switch ($choice) {
                '1' { Invoke-WtConfigCommand -Action 'list' }
                '2' { if (-not (Invoke-WtConsoleConfigSet)) { return } }
                '3' { Invoke-WtConfigCommand -Action 'edit' }
                { $_ -in '4', 'q', 'volver' } { return }
                '' { }
                default { Write-WtNotice "Opcion invalida: '$choice'." }
            }
        } catch {
            Write-WtError "Error: $($_.Exception.Message)"
        }
    }
}

function Invoke-WtConsoleConfigSet {
    <#
    .OUTPUTS
        $false si llego EOF (hay que salir del submenu).
    #>
    $key = Read-WtConsoleLine -Prompt 'Clave (ej. reposRoot): '
    if ($null -eq $key) { return $false }
    $key = $key.Trim()
    if (-not $key) { Write-WtDetail 'Cancelado.'; return $true }
    $value = Read-WtConsoleLine -Prompt "Valor para '$key': "
    if ($null -eq $value) { return $false }
    $cli = "wt config set $key $value"
    $confirm = Read-WtYesNo -Prompt "Ejecutar '$cli'?" -Default $true
    if ($null -eq $confirm) { return $false }
    if (-not $confirm) { Write-WtDetail 'Cancelado.'; return $true }
    Write-WtConsoleCommand $cli
    Invoke-WtConfigCommand -Action 'set' -Key $key -Value $value
    return $true
}

# --- Menu principal ---------------------------------------------------------

function Get-WtConsoleMenu {
    <#
    .SYNOPSIS
        Menu como datos: agregar una opcion es agregar una fila.
        RequiresRepo marca las que solo tienen sentido dentro de un repositorio.
    #>
    return @(
        [pscustomobject]@{ Key = '1'; Label = 'Listar worktrees';                       RequiresRepo = $true;  Action = { Get-WtWorktreeList } }
        [pscustomobject]@{ Key = '2'; Label = 'Crear worktree';                         RequiresRepo = $true;  Action = { Invoke-WtConsoleCreate } }
        [pscustomobject]@{ Key = '3'; Label = 'Abrir worktree (editor)';                RequiresRepo = $true;  Action = { param($repoRoot) Invoke-WtConsoleOpen -RepoRoot $repoRoot } }
        [pscustomobject]@{ Key = '4'; Label = 'Abrir worktree completo (editor + agente)'; RequiresRepo = $true; Action = { param($repoRoot) Invoke-WtConsoleOpen -RepoRoot $repoRoot -All } }
        [pscustomobject]@{ Key = '5'; Label = 'Ver la ruta de un worktree';             RequiresRepo = $true;  Action = { param($repoRoot) Invoke-WtConsolePath -RepoRoot $repoRoot } }
        [pscustomobject]@{ Key = '6'; Label = 'Eliminar un worktree';                   RequiresRepo = $true;  Action = { param($repoRoot) Invoke-WtConsoleRemove -RepoRoot $repoRoot } }
        [pscustomobject]@{ Key = '7'; Label = 'Prune (depurar metadatos huerfanos)';    RequiresRepo = $true;  Action = { Invoke-WtPrune } }
        [pscustomobject]@{ Key = 'r'; Label = 'Listar repos del root';                  RequiresRepo = $false; Action = { Get-WtRepoList } }
        [pscustomobject]@{ Key = 'g'; Label = 'Ir a un repo (cd)';                      RequiresRepo = $false; Action = { Invoke-WtConsoleGoToRepo } }
        [pscustomobject]@{ Key = 'c'; Label = 'Configuracion';                          RequiresRepo = $false; Action = { Invoke-WtConsoleConfig } }
        [pscustomobject]@{ Key = 'd'; Label = 'Doctor (chequeo del setup)';             RequiresRepo = $false; Action = { Invoke-WtDoctor } }
        [pscustomobject]@{ Key = 'h'; Label = 'Ayuda del CLI';                          RequiresRepo = $false; Action = { Show-WtHelp } }
    )
}

function Show-WtConsoleMenu {
    param([AllowEmptyString()][string]$RepoRoot, [Parameter(Mandatory)][object[]]$Menu)
    Write-WtLine ''
    if ($RepoRoot) {
        Write-WtDetail ("Repositorio actual: {0}" -f $RepoRoot)
        Write-WtInfo 'Worktrees:'
        foreach ($item in ($Menu | Where-Object { $_.RequiresRepo })) {
            Write-WtLine ("  {0}) {1}" -f $item.Key, $item.Label)
        }
    } else {
        Write-WtDetail 'No estas dentro de un repo: las opciones de worktrees se habilitan al entrar a uno (g).'
    }
    Write-WtInfo 'Workspace:'
    foreach ($item in ($Menu | Where-Object { -not $_.RequiresRepo })) {
        Write-WtLine ("  {0}) {1}" -f $item.Key, $item.Label)
    }
    Write-WtLine '  q) Salir'
}

function Start-WtConsole {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '',
        Justification = 'Entrypoint del menu interactivo; pedir confirmacion para iniciarlo no tiene sentido.')]
    param()
    Write-WtInfo 'Worktree Manager - consola interactiva'
    Write-WtDetail 'Antes de ejecutar se muestra el comando CLI equivalente, para aprenderlo.'
    $menu = @(Get-WtConsoleMenu)
    while ($true) {
        # Se re-evalua en cada vuelta: 'g' (ir a un repo) puede cambiar el repo actual, y
        # las acciones de create/remove/lock/unlock cambian los worktrees del repo actual.
        Clear-WtConfigCache
        Clear-WtWorktreesCache
        $repoRoot = Find-WtMainRoot -Silent
        Show-WtConsoleMenu -RepoRoot $repoRoot -Menu $menu
        $choice = Read-WtConsoleChoice -Prompt 'wt> '
        if ($null -eq $choice) { break }
        if ($choice -in @('q', 'salir', 'exit')) { return }
        if ($choice -eq '') { continue }
        $item = $menu | Where-Object { $_.Key -eq $choice } | Select-Object -First 1
        if (-not $item) {
            Write-WtNotice "Opcion invalida: '$choice'."
            continue
        }
        if ($item.RequiresRepo -and -not $repoRoot) {
            Write-WtNotice 'Entra a un repo primero (g).'
            continue
        }
        try {
            & $item.Action $repoRoot
        } catch {
            Write-WtError "Error: $($_.Exception.Message)"
        }
    }
    Write-WtDetail 'Fin de la consola (stdin cerrado).'
}
