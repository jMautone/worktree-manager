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
        $tagColor = 'Gray'
        $tag = ''
        if ($wt.IsMain) { $tag = ' (principal)'; $tagColor = 'Cyan' }
        if ($wt.IsLocked) { $tag = ' (bloqueado)'; $tagColor = 'Yellow' }
        if ($wt.IsPrunable) { $tag = ' (obsoleto)'; $tagColor = 'DarkGray' }
        $branch = $wt.Branch
        if (-not $branch) { $branch = '(detached)' }
        Write-Host ("  {0}) " -f ($i + 1)) -ForegroundColor Yellow -NoNewline
        Write-Host (Split-Path -Leaf $wt.Path) -NoNewline
        if ($tag) { Write-Host $tag -ForegroundColor $tagColor -NoNewline }
        Write-Host (" [{0}]" -f $branch) -ForegroundColor DarkGray
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
    New-WtWorktree -Name $name -Base $base | Out-Null
}

function Invoke-WtConsoleOpen {
    <#
    .SYNOPSIS
        Abre un worktree elegido con la combinacion de flags que pida el llamador
        (editor, agente, terminal, o editor+agente con -All). El menu cubre las
        cuatro combinaciones que expone el CLI (item 8).
    #>
    param(
        [Parameter(Mandatory)][string]$RepoRoot,
        [switch]$Agent,
        [switch]$Terminal,
        [switch]$All
    )
    $wt = Select-WtConsoleWorktree -RepoRoot $RepoRoot -Title 'Elegi el worktree a abrir:'
    if (-not $wt) { return }
    $name = Split-Path -Leaf $wt.Path
    $cli = "wt open $name"
    if ($All) { $cli += ' --all' }
    elseif ($Agent) { $cli += ' --agent' }
    elseif ($Terminal) { $cli += ' --terminal' }
    Write-WtConsoleCommand $cli
    Open-WtWorktree -Name $name -Agent:$Agent -Terminal:$Terminal -All:$All
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

function Invoke-WtConsoleLock {
    <#
    .SYNOPSIS
        Bloquea un worktree elegido ('git worktree lock'), con motivo opcional.
    #>
    param([Parameter(Mandatory)][string]$RepoRoot)
    $wt = Select-WtConsoleWorktree -RepoRoot $RepoRoot -Title 'Elegi el worktree a bloquear:' -ExcludeMain
    if (-not $wt) { return }
    $name = Split-Path -Leaf $wt.Path
    $reason = Read-WtConsoleLine -Prompt 'Motivo (opcional): '
    if ($null -eq $reason) { return }
    $reason = $reason.Trim()
    $cli = "wt lock $name"
    if ($reason) { $cli += " --reason `"$reason`"" }
    Write-WtConsoleCommand $cli
    Invoke-WtLock -Name $name -Reason $reason
}

function Invoke-WtConsoleUnlock {
    <#
    .SYNOPSIS
        Desbloquea un worktree elegido ('git worktree unlock').
    #>
    param([Parameter(Mandatory)][string]$RepoRoot)
    $wt = Select-WtConsoleWorktree -RepoRoot $RepoRoot -Title 'Elegi el worktree a desbloquear:' -ExcludeMain
    if (-not $wt) { return }
    $name = Split-Path -Leaf $wt.Path
    Write-WtConsoleCommand "wt unlock $name"
    Invoke-WtUnlock -Name $name
}

function Invoke-WtConsoleExec {
    <#
    .SYNOPSIS
        Corre un comando arbitrario en un worktree elegido ('wt exec').
    #>
    param([Parameter(Mandatory)][string]$RepoRoot)
    $wt = Select-WtConsoleWorktree -RepoRoot $RepoRoot -Title 'Elegi el worktree donde ejecutar:'
    if (-not $wt) { return }
    $name = Split-Path -Leaf $wt.Path
    $command = Read-WtConsoleLine -Prompt 'Comando a ejecutar: '
    if ($null -eq $command -or -not $command.Trim()) { Write-WtDetail 'Cancelado.'; return }
    $command = $command.Trim()
    $cli = "wt exec $name -- $command"
    $confirm = Read-WtYesNo -Prompt "Ejecutar '$cli'?" -Default $true
    if ($null -eq $confirm) { return }
    if (-not $confirm) { Write-WtDetail 'Cancelado.'; return }
    Write-WtConsoleCommand $cli
    Write-WtDetail 'Ejecutando, puede tardar...'
    Invoke-WtExec -Name $name -CommandArgs @($command)
}

function Invoke-WtConsoleEach {
    <#
    .SYNOPSIS
        Corre un comando arbitrario en todos los worktrees del repo ('wt each').
    #>
    $command = Read-WtConsoleLine -Prompt 'Comando a ejecutar en todos los worktrees: '
    if ($null -eq $command -or -not $command.Trim()) { Write-WtDetail 'Cancelado.'; return }
    $command = $command.Trim()
    $continueOnError = Read-WtYesNo -Prompt 'Seguir con los demas si alguno falla?'
    if ($null -eq $continueOnError) { return }
    $cli = 'wt each'
    if ($continueOnError) { $cli += ' --continue-on-error' }
    $cli += " -- $command"
    $confirm = Read-WtYesNo -Prompt "Ejecutar '$cli'?" -Default $true
    if ($null -eq $confirm) { return }
    if (-not $confirm) { Write-WtDetail 'Cancelado.'; return }
    Write-WtConsoleCommand $cli
    Invoke-WtEach -CommandArgs @($command) -ContinueOnError:$continueOnError
}

function Invoke-WtConsoleStatus {
    <#
    .SYNOPSIS
        'wt status' del repo actual (item 8): sin prompts, es un comando de consulta.
    #>
    Write-WtConsoleCommand 'wt status'
    Invoke-WtStatus
}

function Invoke-WtConsoleSync {
    <#
    .SYNOPSIS
        'wt sync' del repo actual (item 8), con una base opcional.
    #>
    $base = Read-WtConsoleLine -Prompt 'Base (vacio = upstream/defaultBase de cada worktree): '
    if ($null -eq $base) { return }
    $base = $base.Trim()
    $cli = 'wt sync'
    if ($base) { $cli += " --base $base" }
    $confirm = Read-WtYesNo -Prompt "Ejecutar '$cli'?" -Default $true
    if ($null -eq $confirm) { return }
    if (-not $confirm) { Write-WtDetail 'Cancelado.'; return }
    Write-WtConsoleCommand $cli
    Invoke-WtSync -Base $base
}

function Invoke-WtConsoleClean {
    <#
    .SYNOPSIS
        'wt clean' (item de Workspace): depura tab configs de Warp huerfanos.
        No necesita estar dentro de un repo.
    #>
    Write-WtConsoleCommand 'wt clean'
    Invoke-WtClean
}

function Invoke-WtConsoleVersion {
    Write-WtConsoleCommand 'wt version'
    Invoke-WtVersion
}

function Invoke-WtConsoleGoToRepo {
    <#
    .SYNOPSIS
        Menu de 'ir a un repo (cd)'. Ademas de los repos, ofrece los worktrees de
        cualquiera de ellos (B12: 'wt cd' tambien puede llevarte a uno).
    #>
    $config = Get-WtConfig
    $root = @(Get-WtReposRoot -Config $config)
    $depth = Get-WtReposDepth -Config $config
    $repos = @(Get-WtRepoDirs -Root $root -Depth $depth)
    if ($repos.Count -eq 0) {
        Write-WtNotice ("No hay repos git en '{0}'." -f ($root -join ', '))
        return
    }
    $worktreeEntries = @()
    foreach ($repo in $repos) {
        $wts = @()
        try { $wts = @(Get-WtWorktrees -RepoRoot $repo.FullName | Where-Object { -not $_.IsMain -and -not $_.IsPrunable }) } catch { continue }
        foreach ($wt in $wts) {
            $worktreeEntries += [pscustomobject]@{ Name = (Split-Path -Leaf $wt.Path); RepoName = $repo.Name }
        }
    }
    Write-WtInfo 'Elegi el repo o worktree:'
    $items = @()
    foreach ($repo in $repos) {
        Write-WtLine ("  {0}) {1}" -f ($items.Count + 1), $repo.Name)
        $items += $repo.Name
    }
    foreach ($entry in $worktreeEntries) {
        Write-WtLine ("  {0}) {1} (worktree de {2})" -f ($items.Count + 1), $entry.Name, $entry.RepoName)
        $items += $entry.Name
    }
    $index = Select-WtConsoleIndex -Count $items.Count
    if ($null -eq $index) { return }
    $name = $items[$index]
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

# --- Presentacion -------------------------------------------------------------

function Test-WtConsoleInteractive {
    <#
    .SYNOPSIS
        $true si hay una terminal real del otro lado (no stdin pipeado).
    .DESCRIPTION
        Unico punto de deteccion: limpiar pantalla y pausar con "Enter para
        continuar" solo tiene sentido frente a una persona mirando la terminal.
        Con stdin redirigido (los tests E2E, o 'echo ... | wt console' en un
        script) la consola se comporta exactamente como antes: transcript que
        scrollea, sin limpiar y sin pausas que consuman una linea de entrada que
        el llamador no puso ahi para eso.
    #>
    return -not [Console]::IsInputRedirected
}

function Clear-WtConsoleScreen {
    <#
    .SYNOPSIS
        Limpia la pantalla, solo en modo interactivo. 'Clear-Host' depende de un
        buffer de consola real (RawUI): en un host sin consola (ISE vieja, algun
        runner de CI) puede fallar, por eso el try/catch defensivo.
    #>
    if (-not (Test-WtConsoleInteractive)) { return }
    try { Clear-Host } catch { }
}

function Wait-WtConsoleContinue {
    <#
    .SYNOPSIS
        Pausa hasta Enter, solo en modo interactivo: da tiempo a leer el
        resultado de la accion antes de que la proxima vuelta del loop limpie la
        pantalla y lo borre.
    #>
    if (-not (Test-WtConsoleInteractive)) { return }
    $rule = [string]([char]0x2500) * 52
    Write-Host ''
    Write-Host $rule -ForegroundColor DarkGray
    Read-WtConsoleLine -Prompt 'Presiona Enter para continuar...' | Out-Null
}

$script:WtConsoleLastAction = $null

function Set-WtConsoleLastAction {
    param([Parameter(Mandatory)][string]$Label, [Parameter(Mandatory)][bool]$Success, [AllowEmptyString()][string]$Detail = '')
    $script:WtConsoleLastAction = [pscustomobject]@{ Label = $Label; Success = $Success; Detail = $Detail }
}

function Write-WtConsoleLastAction {
    <#
    .SYNOPSIS
        Breadcrumb con el resultado de la ultima accion, para no perderlo de
        vista apenas la pantalla se redibuja.
    #>
    if (-not $script:WtConsoleLastAction) { return }
    $last = $script:WtConsoleLastAction
    Write-Host ''
    Write-Host '  Ultimo: ' -ForegroundColor DarkGray -NoNewline
    Write-Host $last.Label -NoNewline
    if ($last.Success) {
        Write-Host ' - OK' -ForegroundColor Green
    } else {
        $detail = $last.Detail
        if ($detail) { Write-Host (' - Error: {0}' -f $detail) -ForegroundColor Red }
        else { Write-Host ' - Error' -ForegroundColor Red }
    }
}

function Write-WtConsoleBoxLine {
    <#
    .SYNOPSIS
        Una linea de un recuadro (borde + contenido + borde), centrada o alineada
        a la izquierda. Usa box-drawing (cp437-safe: se ve bien tanto en consolas
        UTF-8 como en el codepage OEM clasico de Windows).
    #>
    param(
        [AllowEmptyString()][string]$Text = '',
        [Parameter(Mandatory)][int]$Width,
        [string]$BorderColor = 'DarkCyan',
        [string]$TextColor = 'Gray',
        [switch]$Center
    )
    $vert = [char]0x2502
    if ($Text.Length -gt $Width) { $Text = $Text.Substring(0, $Width) }
    if ($Center) {
        $padTotal = $Width - $Text.Length
        $padLeft = [int]($padTotal / 2)
        $padRight = $padTotal - $padLeft
        $content = (' ' * $padLeft) + $Text + (' ' * $padRight)
    } else {
        $content = $Text.PadRight($Width)
    }
    Write-Host $vert -ForegroundColor $BorderColor -NoNewline
    Write-Host $content -ForegroundColor $TextColor -NoNewline
    Write-Host $vert -ForegroundColor $BorderColor
}

function Write-WtConsoleBanner {
    <#
    .SYNOPSIS
        Encabezado de la consola. Se imprime una sola vez, al arrancar.
    #>
    $width = 50
    $horiz = [string]([char]0x2500) * $width
    $topLeft = [char]0x250C; $topRight = [char]0x2510
    $botLeft = [char]0x2514; $botRight = [char]0x2518
    Write-Host ''
    Write-Host ($topLeft + $horiz + $topRight) -ForegroundColor DarkCyan
    Write-WtConsoleBoxLine -Text 'WORKTREE MANAGER' -Width $width -BorderColor DarkCyan -TextColor Cyan -Center
    Write-WtConsoleBoxLine -Text 'consola interactiva' -Width $width -BorderColor DarkCyan -TextColor DarkGray -Center
    Write-Host ($botLeft + $horiz + $botRight) -ForegroundColor DarkCyan
}

function Write-WtConsoleSectionTitle {
    param([Parameter(Mandatory)][string]$Title, [string]$Color = 'Cyan')
    $dash = [string]([char]0x2500)
    $filler = $dash * [Math]::Max(3, (40 - $Title.Length))
    Write-Host ''
    Write-Host ("$dash$dash $Title $filler") -ForegroundColor $Color
}

function Write-WtConsoleContext {
    <#
    .SYNOPSIS
        Cabecera de estado que se redibuja en cada vuelta del loop: repo actual,
        resumen (cantidad de worktrees, cuantos bloqueados, rama del checkout en
        el que esta parada la terminal) y la tabla de worktrees en vivo, para que
        el panorama este a la vista sin tener que pedirlo con la opcion '1'.
    #>
    param([AllowEmptyString()][string]$RepoRoot)
    Write-Host ''
    if (-not $RepoRoot) {
        Write-WtNotice 'No estas dentro de un repo: las opciones de worktrees se habilitan al entrar a uno (g).'
        return
    }
    $allWorktrees = @(Get-WtWorktrees -RepoRoot $RepoRoot)
    $active = @($allWorktrees | Where-Object { -not $_.IsPrunable })
    $locked = @($active | Where-Object { $_.IsLocked })
    $branch = ''
    $current = Get-WtCurrentRoot
    if ($current) { $branch = Get-WtBranchAt -Path $current }
    $summary = "{0} worktree(s)" -f $active.Count
    if ($locked.Count -gt 0) { $summary += (", {0} bloqueado(s)" -f $locked.Count) }
    if ($branch) { $summary += (", rama actual: {0}" -f $branch) }
    Write-Host ("  {0}" -f (Split-Path -Leaf $RepoRoot)) -ForegroundColor White -NoNewline
    Write-Host ("  -  {0}" -f $summary) -ForegroundColor DarkGray
    Write-WtDetail ("  {0}" -f $RepoRoot)
    if ($allWorktrees.Count -gt 0) {
        Write-Host ''
        $rows = @(Get-WtWorktreeRows -Worktrees $allWorktrees)
        $lines = @($rows | Format-Table -AutoSize | Out-String -Stream | Where-Object { $_.Trim() })
        foreach ($line in $lines) { Write-Host ("  {0}" -f $line) }
    }
}

# --- Menu principal ---------------------------------------------------------

function Get-WtConsoleMenu {
    <#
    .SYNOPSIS
        Menu como datos: agregar una opcion es agregar una fila.
        RequiresRepo marca las que solo tienen sentido dentro de un repositorio;
        Group define en que seccion visual cae (item: consola completa y prolija).
    #>
    return @(
        [pscustomobject]@{ Key = '1'; Label = 'Listar worktrees';                          Group = 'Worktrees';              RequiresRepo = $true;  Action = { Get-WtWorktreeList } }
        [pscustomobject]@{ Key = '2'; Label = 'Crear worktree';                            Group = 'Worktrees';              RequiresRepo = $true;  Action = { Invoke-WtConsoleCreate } }
        [pscustomobject]@{ Key = '3'; Label = 'Abrir worktree (editor)';                   Group = 'Worktrees';              RequiresRepo = $true;  Action = { param($repoRoot) Invoke-WtConsoleOpen -RepoRoot $repoRoot } }
        [pscustomobject]@{ Key = '4'; Label = 'Abrir worktree completo (editor + agente)'; Group = 'Worktrees';              RequiresRepo = $true;  Action = { param($repoRoot) Invoke-WtConsoleOpen -RepoRoot $repoRoot -All } }
        [pscustomobject]@{ Key = '5'; Label = 'Abrir worktree (solo agente)';              Group = 'Worktrees';              RequiresRepo = $true;  Action = { param($repoRoot) Invoke-WtConsoleOpen -RepoRoot $repoRoot -Agent } }
        [pscustomobject]@{ Key = '6'; Label = 'Abrir worktree (solo terminal)';            Group = 'Worktrees';              RequiresRepo = $true;  Action = { param($repoRoot) Invoke-WtConsoleOpen -RepoRoot $repoRoot -Terminal } }
        [pscustomobject]@{ Key = '7'; Label = 'Ver la ruta de un worktree';                Group = 'Worktrees';              RequiresRepo = $true;  Action = { param($repoRoot) Invoke-WtConsolePath -RepoRoot $repoRoot } }
        [pscustomobject]@{ Key = '8'; Label = 'Eliminar un worktree';                      Group = 'Worktrees';              RequiresRepo = $true;  Action = { param($repoRoot) Invoke-WtConsoleRemove -RepoRoot $repoRoot } }
        [pscustomobject]@{ Key = '9'; Label = 'Prune (depurar metadatos huerfanos)';       Group = 'Worktrees';              RequiresRepo = $true;  Action = { Invoke-WtPrune } }
        [pscustomobject]@{ Key = 'l'; Label = 'Bloquear un worktree (lock)';               Group = 'Worktrees';              RequiresRepo = $true;  Action = { param($repoRoot) Invoke-WtConsoleLock -RepoRoot $repoRoot } }
        [pscustomobject]@{ Key = 'u'; Label = 'Desbloquear un worktree (unlock)';          Group = 'Worktrees';              RequiresRepo = $true;  Action = { param($repoRoot) Invoke-WtConsoleUnlock -RepoRoot $repoRoot } }
        [pscustomobject]@{ Key = 's'; Label = 'Status del repo (wt status)';               Group = 'Ejecutar y sincronizar'; RequiresRepo = $true;  Action = { Invoke-WtConsoleStatus } }
        [pscustomobject]@{ Key = 'y'; Label = 'Sincronizar (wt sync)';                     Group = 'Ejecutar y sincronizar'; RequiresRepo = $true;  Action = { Invoke-WtConsoleSync } }
        [pscustomobject]@{ Key = 'x'; Label = 'Ejecutar un comando en un worktree (exec)'; Group = 'Ejecutar y sincronizar'; RequiresRepo = $true;  Action = { param($repoRoot) Invoke-WtConsoleExec -RepoRoot $repoRoot } }
        [pscustomobject]@{ Key = 'e'; Label = 'Ejecutar un comando en todos (each)';       Group = 'Ejecutar y sincronizar'; RequiresRepo = $true;  Action = { Invoke-WtConsoleEach } }
        [pscustomobject]@{ Key = 'r'; Label = 'Listar repos del root';                     Group = 'Workspace';              RequiresRepo = $false; Action = { Get-WtRepoList } }
        [pscustomobject]@{ Key = 'g'; Label = 'Ir a un repo o worktree (cd)';              Group = 'Workspace';              RequiresRepo = $false; Action = { Invoke-WtConsoleGoToRepo } }
        [pscustomobject]@{ Key = 'c'; Label = 'Configuracion';                             Group = 'Workspace';              RequiresRepo = $false; Action = { Invoke-WtConsoleConfig } }
        [pscustomobject]@{ Key = 'k'; Label = 'Limpiar tab configs huerfanos (clean)';     Group = 'Workspace';              RequiresRepo = $false; Action = { Invoke-WtConsoleClean } }
        [pscustomobject]@{ Key = 'd'; Label = 'Doctor (chequeo del setup)';                Group = 'Workspace';              RequiresRepo = $false; Action = { Invoke-WtDoctor } }
        [pscustomobject]@{ Key = 'v'; Label = 'Version';                                   Group = 'Workspace';              RequiresRepo = $false; Action = { Invoke-WtConsoleVersion } }
        [pscustomobject]@{ Key = 'h'; Label = 'Ayuda del CLI';                             Group = 'Workspace';              RequiresRepo = $false; Action = { Show-WtHelp } }
    )
}

function Show-WtConsoleMenu {
    param([AllowEmptyString()][string]$RepoRoot, [Parameter(Mandatory)][object[]]$Menu)
    Write-WtConsoleContext -RepoRoot $RepoRoot
    Write-WtConsoleLastAction
    $sections = @(
        [pscustomobject]@{ Name = 'Worktrees';              Color = 'Cyan' }
        [pscustomobject]@{ Name = 'Ejecutar y sincronizar'; Color = 'Green' }
        [pscustomobject]@{ Name = 'Workspace';               Color = 'DarkYellow' }
    )
    foreach ($section in $sections) {
        $items = @($Menu | Where-Object { $_.Group -eq $section.Name -and (-not $_.RequiresRepo -or $RepoRoot) })
        if ($items.Count -eq 0) { continue }
        Write-WtConsoleSectionTitle -Title $section.Name -Color $section.Color
        foreach ($item in $items) {
            Write-Host '   ' -NoNewline
            Write-Host $item.Key.PadRight(3) -ForegroundColor Yellow -NoNewline
            Write-Host $item.Label
        }
    }
    Write-Host ''
    Write-Host '   ' -NoNewline
    Write-Host 'q'.PadRight(3) -ForegroundColor Yellow -NoNewline
    Write-Host 'Salir'
}

function Start-WtConsole {
    <#
    .SYNOPSIS
        Punto de entrada del menu interactivo.
    .DESCRIPTION
        Dos modos, segun 'Test-WtConsoleInteractive':
        - Interactivo (terminal real): cada vuelta limpia la pantalla y redibuja
          todo como un tablero (banner + contexto + menu); despues de cada accion
          se pausa con "Enter para continuar" para no perder su salida en el
          siguiente limpiado.
        - No interactivo (stdin pipeado: tests E2E, o un script que le manda
          comandos por pipe): se comporta como antes, un transcript que scrollea
          sin limpiar ni pausar, para no consumir lineas de entrada que el
          llamador no puso ahi para eso.
    #>
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '',
        Justification = 'Entrypoint del menu interactivo; pedir confirmacion para iniciarlo no tiene sentido.')]
    param()
    $interactive = Test-WtConsoleInteractive
    $script:WtConsoleLastAction = $null
    if (-not $interactive) {
        Write-WtConsoleBanner
        Write-WtDetail 'Antes de ejecutar se muestra el comando CLI equivalente, para aprenderlo.'
    }
    $menu = @(Get-WtConsoleMenu)
    while ($true) {
        # Se re-evalua en cada vuelta: 'g' (ir a un repo) puede cambiar el repo actual, y
        # las acciones de create/remove/lock/unlock cambian los worktrees del repo actual.
        Clear-WtConfigCache
        Clear-WtWorktreesCache
        $repoRoot = Find-WtMainRoot -Silent
        if ($interactive) {
            Clear-WtConsoleScreen
            Write-WtConsoleBanner
        }
        Show-WtConsoleMenu -RepoRoot $repoRoot -Menu $menu
        $choice = Read-WtConsoleChoice -Prompt "`nwt> "
        if ($null -eq $choice) { break }
        if ($choice -in @('q', 'salir', 'exit')) { Write-WtSuccess 'Hasta la proxima!'; return }
        if ($choice -eq '') { continue }
        $item = $menu | Where-Object { $_.Key -eq $choice } | Select-Object -First 1
        if (-not $item) {
            Write-WtNotice "Opcion invalida: '$choice'."
            Wait-WtConsoleContinue
            continue
        }
        if ($item.RequiresRepo -and -not $repoRoot) {
            Write-WtNotice 'Entra a un repo primero (g).'
            Wait-WtConsoleContinue
            continue
        }
        try {
            & $item.Action $repoRoot
            Set-WtConsoleLastAction -Label $item.Label -Success $true
        } catch {
            Write-WtError "Error: $($_.Exception.Message)"
            Set-WtConsoleLastAction -Label $item.Label -Success $false -Detail $_.Exception.Message
        }
        Wait-WtConsoleContinue
    }
    Write-WtDetail 'Fin de la consola (stdin cerrado).'
}
