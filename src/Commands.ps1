# ---------------------------------------------------------------------------
# Commands: logica de cada comando del CLI.
# Es la unica capa que combina decision (funciones puras) con efectos y salida.
# Depende de: Common, Config, Repo, Workspace, Launch.
# ---------------------------------------------------------------------------

# --- create -----------------------------------------------------------------

function Get-WtCreateOpenPlan {
    <#
    .SYNOPSIS
        Que abrir al terminar 'wt create'. Funcion pura.
    .DESCRIPTION
        --no-open gana sobre todo (equivale a 'none'). Si se paso algun otro flag de
        apertura explicito (--all/--code/--agent/--terminal), esos flags ganan sobre la
        config. Sin flags, decide 'openOnCreate': 'all' (editor + agente, el default:
        es lo que el README promete), 'editor' (solo editor) o 'none' (nada).
    #>
    param(
        [Parameter(Mandatory)]$Config,
        [switch]$Code,
        [switch]$Terminal,
        [switch]$Agent,
        [switch]$All,
        [switch]$NoOpen
    )
    if ($NoOpen) {
        return [pscustomobject]@{ Code = $false; Terminal = $false; Agent = $false }
    }
    $explicit = [bool]$Code -or [bool]$Terminal -or [bool]$Agent -or [bool]$All
    if ($explicit) {
        return (Get-WtOpenPlan -Code:$Code -Terminal:$Terminal -Agent:$Agent -All:$All)
    }
    switch ([string]$Config.openOnCreate) {
        'all'    { return [pscustomobject]@{ Code = $true; Terminal = $false; Agent = $true } }
        'editor' { return [pscustomobject]@{ Code = $true; Terminal = $false; Agent = $false } }
        default  { return [pscustomobject]@{ Code = $false; Terminal = $false; Agent = $false } }
    }
}

function New-WtWorktree {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '',
        Justification = 'Comando principal del CLI: pedir confirmacion en cada create rompe el flujo. No es un cmdlet generico.')]
    param(
        [Parameter(Mandatory)][string]$Name,
        [AllowEmptyString()][string]$Base,
        [AllowEmptyString()][string]$Branch,
        [switch]$Code,
        [switch]$Terminal,
        [switch]$Agent,
        [switch]$All,
        [switch]$NoOpen
    )
    # Se valida antes de tocar git o la config: el error debe hablar del nombre.
    Assert-WtWorktreeName -Name $Name

    $repoRoot = Find-WtMainRoot
    $config = Get-WtConfig
    $baseExplicit = [bool]$Base
    if (-not $Base) { $Base = [string]$config.defaultBase }
    if (-not $Branch) { $Branch = ([string]$config.branchPrefix) + $Name }
    Assert-WtBranchName -Branch $Branch

    $path = Get-WtWorktreePath -Config $config -RepoRoot $repoRoot -Name $Name
    if (Test-WtPathExists $path) { throw "Ya existe un directorio en '$path'." }

    if ($config.fetchBeforeCreate -and $Base -match '^([^/]+)/') {
        $remote = $Matches[1]
        if (Test-WtRemote -RepoRoot $repoRoot -Remote $remote) {
            Write-WtDetail "Actualizando '$remote'..."
            Invoke-WtGit -WorkingDirectory $repoRoot -Arguments @('fetch', $remote, '--prune') -AllowFailure | Out-Null
        }
    }

    $gitArgs = @('worktree', 'add')
    if (Test-WtLocalBranch -RepoRoot $repoRoot -Branch $Branch) {
        if ($baseExplicit) {
            throw "La rama '$Branch' ya existe localmente, pero pediste --base '$Base'. Para no descartar en silencio el historial local de '$Branch', wt no la reutiliza automaticamente. Elegi otro --branch, o borra la rama local primero (git branch -D $Branch) y volve a correr 'wt create'."
        }
        Write-WtDetail "La rama '$Branch' ya existe; se reutiliza."
        $gitArgs += @($path, $Branch)
    } else {
        $gitArgs += @('-b', $Branch, $path)
        if ($Base) { $gitArgs += $Base }
    }

    Write-WtInfo "Creando worktree '$Name' en $path"
    Invoke-WtGit -WorkingDirectory $repoRoot -Arguments $gitArgs | Out-Null
    Write-WtSuccess "OK - worktree listo: $path (rama: $Branch)"

    $openPlan = Get-WtCreateOpenPlan -Config $config -Code:$Code -Terminal:$Terminal -Agent:$Agent -All:$All -NoOpen:$NoOpen
    if ($openPlan.Code -or $openPlan.Terminal -or $openPlan.Agent) {
        Open-WtWorktree -Name $Name -Code:$openPlan.Code -Terminal:$openPlan.Terminal -Agent:$openPlan.Agent
    }
    return $path
}

function Test-WtRemote {
    param([Parameter(Mandatory)][string]$RepoRoot, [Parameter(Mandatory)][string]$Remote)
    $r = Invoke-WtGit -WorkingDirectory $RepoRoot -Arguments @('remote') -AllowFailure
    if (-not $r.Success) { return $false }
    return (@($r.StdOut | ForEach-Object { $_.Trim() }) -contains $Remote)
}

# --- list -------------------------------------------------------------------

function Get-WtWorktreeRows {
    <#
    .SYNOPSIS
        Filas de presentacion de 'wt list'. Funcion pura sobre el modelo de worktrees.
    #>
    param([Parameter(Mandatory)][AllowEmptyCollection()][object[]]$Worktrees)
    return @(foreach ($wt in $Worktrees) {
        $nombre = Split-Path -Leaf $wt.Path
        if ($wt.IsMain) { $nombre = "$nombre (principal)" }
        if ($wt.IsPrunable) { $nombre = "$nombre (obsoleto: corre wt prune)" }
        $rama = $wt.Branch
        if (-not $rama -and $wt.IsDetached -and $wt.Head) {
            $rama = "(detached: {0})" -f $wt.Head.Substring(0, [Math]::Min(7, $wt.Head.Length))
        }
        [pscustomobject]@{ Nombre = $nombre; Rama = $rama; Ruta = $wt.Path }
    })
}

function Get-WtWorktreeList {
    param([switch]$Json)
    $repoRoot = Find-WtMainRoot
    $worktrees = @(Get-WtWorktrees -RepoRoot $repoRoot)
    if ($Json) {
        ConvertTo-WtJson -InputObject @($worktrees | Select-Object Path, Branch, Head, IsMain, IsDetached, IsPrunable)
        return
    }
    Write-WtDetail ("Repositorio: {0}" -f $repoRoot)
    Write-WtTable -Rows (Get-WtWorktreeRows -Worktrees $worktrees)
}

# --- resolucion compartida por open / path ---------------------------------

function Resolve-WtTarget {
    <#
    .SYNOPSIS
        Worktree sobre el que operan 'open' y 'path'. Funcion pura: no cambia el
        directorio actual (ver Resolve-WtRepoContext); eso lo decide cada llamador
        segun si le importa relocalizarse (Open-WtWorktree si, Invoke-WtPathCommand no).
    .DESCRIPTION
        - Sin nombre: el checkout actual (el worktree si estas parado en uno).
        - Con nombre estando en un repo: se busca por carpeta o rama.
        - Fuera de un repo: el nombre puede ser un repo de reposRoot (se opera sobre su
          checkout principal) o un worktree de alguno de ellos.
    .OUTPUTS
        @{ RepoRoot; Worktree; Name; Source; ShouldRelocate }
    #>
    param([AllowEmptyString()][string]$Name)
    $context = Resolve-WtRepoContext -Name $Name
    $repoRoot = $context.RepoRoot

    if (-not $Name) {
        $current = Get-WtCurrentRoot
        if (-not $current) { $current = $repoRoot }
        $worktree = Get-WtCheckoutInfo -Path $current -RepoRoot $repoRoot
    } elseif ($context.Source -eq 'repo') {
        # El nombre identifico un repo (incluso por prefijo): se opera sobre su raiz.
        $worktree = Get-WtCheckoutInfo -Path $repoRoot -RepoRoot $repoRoot
    } else {
        $worktree = Resolve-WtWorktree -RepoRoot $repoRoot -Name $Name
    }
    return @{
        RepoRoot       = $repoRoot
        Worktree       = $worktree
        Name           = (Split-Path -Leaf $worktree.Path)
        Source         = $context.Source
        ShouldRelocate = $context.ShouldRelocate
    }
}

function Invoke-WtPathCommand {
    <#
    .NOTES
        Nunca relocaliza (a diferencia de Open-WtWorktree): 'wt path' fuera de un repo
        imprime la ruta sin mover el directorio actual del proceso.
    #>
    param([AllowEmptyString()][string]$Name)
    $target = Resolve-WtTarget -Name $Name
    Write-Output $target.Worktree.Path
}

# --- open -------------------------------------------------------------------

function Get-WtOpenPlan {
    <#
    .SYNOPSIS
        Que abrir segun los flags. Funcion pura.
    .DESCRIPTION
        Sin flags: solo el editor. --all = editor + agente. La terminal NUNCA se abre
        salvo que se pida con --terminal (asi --all no duplica terminales: el agente ya
        es una terminal con Copilot corriendo).
    #>
    param(
        [switch]$Code,
        [switch]$Terminal,
        [switch]$Agent,
        [switch]$All,
        [switch]$NoCode
    )
    $wantCode = [bool]$Code -or [bool]$All
    $wantAgent = [bool]$Agent -or [bool]$All
    $explicit = [bool]$Code -or [bool]$Terminal -or [bool]$Agent -or [bool]$All
    if (-not $explicit) { $wantCode = -not $NoCode }
    return [pscustomobject]@{
        Code     = $wantCode
        Terminal = [bool]$Terminal
        Agent    = $wantAgent
    }
}

function Open-WtWorktree {
    <#
    .SYNOPSIS
        Abre un worktree (o el checkout actual si no se pasa nombre).
    .NOTES
        -NoCode y -NoTerminal se aceptan por compatibilidad con invocaciones antiguas;
        -NoTerminal es un no-op porque la terminal ya no se abre por defecto.
        Pendiente: B1 elimina -NoTerminal (y -NoCode si A3 lo deja sin llamadores).
    #>
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSReviewUnusedParameter', 'NoTerminal',
        Justification = 'Compatibilidad hacia atras; B1 la elimina junto con sus llamadores.')]
    param(
        [AllowEmptyString()][string]$Name,
        [switch]$Code,
        [switch]$Terminal,
        [switch]$Agent,
        [switch]$All,
        [switch]$NoCode,
        [switch]$NoTerminal
    )
    $plan = Get-WtOpenPlan -Code:$Code -Terminal:$Terminal -Agent:$Agent -All:$All -NoCode:$NoCode
    $target = Resolve-WtTarget -Name $Name
    if ($target.ShouldRelocate) {
        Set-WtLocation -Path $target.RepoRoot
        if ($target.Source -eq 'repo') {
            Write-WtDetail "Ahora en el repo: $($target.RepoRoot)"
        } else {
            Write-WtDetail "El worktree '$Name' es del repo '$(Split-Path -Leaf $target.RepoRoot)'; ahora en $($target.RepoRoot)"
        }
    }
    $config = Get-WtConfig
    $path = $target.Worktree.Path
    if (-not (Test-WtPathExists $path)) {
        throw "El directorio del worktree no existe: $path. Corre 'wt prune' para depurar los metadatos y volve a crearlo con 'wt create $($target.Name)'."
    }
    $title = Get-WtTabTitle -RepoRoot $target.RepoRoot -Name $target.Name -Branch $target.Worktree.Branch

    $opened = $false
    if ($plan.Code) {
        if (Open-WtEditor -Path $path -Config $config) { $opened = $true }
    }
    if ($plan.Terminal) {
        if (Open-WtTerminal -Path $path -Name $target.Name -Title $title -Config $config) { $opened = $true }
    }
    if ($plan.Agent) {
        if (Open-WtAgent -Path $path -Name $target.Name -Title $title -Config $config) { $opened = $true }
    }
    if (-not $opened) { Write-WtLine $path }
}

function Open-WtTerminal {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$Name,
        [string]$Title,
        [Parameter(Mandatory)]$Config
    )
    $terminal = [string]$Config.terminal
    if ($terminal -eq 'warp') {
        $warp = [string]$Config.warpPath
        if (-not (Test-WtPathExists $warp)) {
            Write-WtWarn "Warp no encontrado en '$warp' (ajusta warpPath en la config)."
            return $false
        }
        $sameWindow = Open-WtTerminalInWarp -Name $Name -Path $Path -Target ([string]$Config.warpAgentTarget) `
            -Title $Title -Color ([string]$Config.warpTerminalColor)
        Write-WtSuccess ("Terminal abierta en {0} ({1})" -f $Path, (Format-WtWarpTarget $sameWindow))
        return $true
    }
    if ($terminal -eq 'wt') {
        if (-not (Test-WtCommand 'wt.exe')) {
            Write-WtWarn 'Windows Terminal (wt.exe) no encontrado.'
            return $false
        }
        Start-Process -FilePath 'wt.exe' -ArgumentList @('-d', (Format-WtProcessArgument $Path))
        Write-WtSuccess "Windows Terminal abierto en $Path"
        return $true
    }
    return $false
}

function Open-WtAgent {
    <#
    .NOTES
        El agente se abre solo como tab de Warp; nunca en una ventana suelta de PowerShell.
    #>
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$Name,
        [string]$Title,
        [Parameter(Mandatory)]$Config
    )
    $agentCommand = [string]$Config.agentCommand
    if (-not (Test-WtCommand $agentCommand)) {
        Write-WtWarn "No se encontro '$agentCommand' en la sesion actual; no se lanza el agente."
        return $false
    }
    if ([string]$Config.terminal -ne 'warp' -or -not (Test-WtPathExists ([string]$Config.warpPath))) {
        Write-WtWarn "El agente se abre solo en Warp; configura terminal = 'warp' y un warpPath valido."
        return $false
    }
    $sameWindow = Open-WtAgentInWarp -Name $Name -Path $Path -Config $Config -Target ([string]$Config.warpAgentTarget) `
        -Title $Title -Color ([string]$Config.warpAgentColor)
    Write-WtSuccess ("Agente '{0}' iniciado en {1} ({2})" -f $agentCommand, $Path, (Format-WtWarpTarget $sameWindow))
    return $true
}

# --- remove / prune ---------------------------------------------------------

function Remove-WtWorktree {
    <#
    .SYNOPSIS
        Elimina un worktree y, opcionalmente, su rama.
    .DESCRIPTION
        --delete-branch intenta un borrado seguro ('git branch -d'); si la rama tiene
        commits no mergeados en ninguna otra, aborta sin borrarla (el worktree ya se
        elimino) y explica como forzarlo con --force-branch. --force es exclusivamente
        para 'git worktree remove --force' (arbol de trabajo sucio) y nunca implica
        --force-branch: son dos decisiones independientes.
    #>
    [CmdletBinding(SupportsShouldProcess)]
    param(
        [Parameter(Mandatory)][string]$Name,
        [switch]$DeleteBranch,
        [switch]$Force,
        [switch]$ForceBranch
    )
    $repoRoot = Find-WtMainRoot
    $wt = Resolve-WtWorktree -RepoRoot $repoRoot -Name $Name
    if ($wt.IsMain) { throw 'No se puede eliminar el worktree principal del repositorio.' }

    if (-not $PSCmdlet.ShouldProcess($wt.Path, 'git worktree remove')) { return }

    # Antes de borrar: el nombre de archivo del tab config depende de la ruta (hash),
    # asi que hay que resolverlo mientras esta todavia identifica a este worktree.
    Remove-WtWorktreeTabConfigs -Name (Split-Path -Leaf $wt.Path) -Path $wt.Path

    # Si estamos parados dentro del worktree a eliminar, git no puede borrarlo.
    if (Test-WtPathIsUnder -Path (Get-Location).Path -Root $wt.Path) {
        Set-WtLocation -Path $repoRoot
        Write-WtDetail "Directorio actual movido a '$repoRoot' (estabas dentro del worktree a eliminar)."
    }

    $gitArgs = @('worktree', 'remove')
    if ($Force) { $gitArgs += '--force' }
    $gitArgs += $wt.Path

    Write-WtInfo "Eliminando worktree '$($wt.Path)'..."
    Invoke-WtGit -WorkingDirectory $repoRoot -Arguments $gitArgs | Out-Null
    Write-WtSuccess 'OK - worktree eliminado.'

    if ($DeleteBranch -and $wt.Branch) {
        if (-not $PSCmdlet.ShouldProcess($wt.Branch, 'git branch delete')) { return }
        $deleteFlag = '-d'
        if ($ForceBranch) { $deleteFlag = '-D' }
        $branchResult = Invoke-WtGit -WorkingDirectory $repoRoot -Arguments @('branch', $deleteFlag, $wt.Branch) -AllowFailure
        if ($branchResult.Success) {
            Write-WtSuccess "OK - rama '$($wt.Branch)' eliminada."
        } elseif ($ForceBranch) {
            $detail = $branchResult.ErrorText
            if (-not $detail) { $detail = $branchResult.Text }
            throw "No se pudo eliminar la rama '$($wt.Branch)': $detail"
        } else {
            $template = "La rama '{0}' tiene commits que no estan mergeados en ninguna otra rama; no se borro. " +
                "El worktree si se elimino. Para borrarla igual: wt remove {0} --delete-branch --force-branch, " +
                "o git branch -D {0}."
            throw ($template -f $wt.Branch)
        }
    }
}

function Invoke-WtPrune {
    $repoRoot = Find-WtMainRoot
    Invoke-WtGit -WorkingDirectory $repoRoot -Arguments @('worktree', 'prune', '-v') | Out-Null
    Write-WtSuccess 'OK - metadatos de worktrees obsoletos depurados.'
}

function Invoke-WtClean {
    <#
    .SYNOPSIS
        Borra los tab configs de Warp (wt-*.toml) cuyo 'directory' ya no existe.
    .DESCRIPTION
        Nadie los borra automaticamente salvo 'wt remove' (para el worktree que borra);
        este comando depura los que quedaron huerfanos por otras vias (borrado manual
        del directorio, worktrees creados antes de esta version, etc.).
    #>
    $dir = Get-WtTabConfigDir
    if (-not (Test-WtPathExists $dir)) {
        Write-WtSuccess 'OK - no hay tab configs para depurar.'
        return
    }
    $files = @(Get-ChildItem -LiteralPath $dir -Filter 'wt-*.toml' -File -ErrorAction SilentlyContinue)
    $removed = 0
    foreach ($file in $files) {
        $content = Get-Content -Raw -LiteralPath $file.FullName -ErrorAction SilentlyContinue
        $directory = Get-WtTabConfigDirectory -Content $content
        if ($directory -and -not (Test-WtPathExists $directory)) {
            Remove-Item -LiteralPath $file.FullName -Force -ErrorAction SilentlyContinue
            Write-WtDetail ("Eliminado: {0} (directory ya no existe: {1})" -f $file.Name, $directory)
            $removed++
        }
    }
    if ($removed -eq 0) {
        Write-WtSuccess 'OK - no habia tab configs huerfanos.'
    } else {
        Write-WtSuccess ("OK - {0} tab config(s) huerfano(s) depurado(s)." -f $removed)
    }
}

# --- version ------------------------------------------------------------

function Invoke-WtVersion {
    <#
    .SYNOPSIS
        Imprime la version del manifiesto y la version de PowerShell activa.
    #>
    $manifestPath = Join-Path $PSScriptRoot '..\wt.psd1'
    $manifest = Import-PowerShellDataFile -Path $manifestPath
    Write-WtLine ("wt {0} {1} PowerShell {2}" -f $manifest.ModuleVersion, [char]0x00B7, $PSVersionTable.PSVersion)
}

# --- doctor -----------------------------------------------------------------

function New-WtDoctorRow {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '',
        Justification = 'Constructor puro (pscustomobject); el verbo New no implica efectos.')]
    param(
        [Parameter(Mandatory)][string]$Check,
        [Parameter(Mandatory)][bool]$Ok,
        [string]$OkDetail = '',
        [string]$FailDetail = '',
        [string]$FailState = 'FALTA'
    )
    $estado = 'OK'; $detalle = $OkDetail
    if (-not $Ok) { $estado = $FailState; $detalle = $FailDetail }
    return [pscustomobject]@{ Chequeo = $Check; Estado = $estado; Detalle = $detalle }
}

function Get-WtDoctorRows {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingEmptyCatchBlock', '',
        Justification = 'Chequeo informativo best-effort: si git falla al listar, se reporta "0 obsoletos" en vez de romper doctor.')]
    param()
    $config = Get-WtConfig
    $rows = @()

    $invalidKeys = @(foreach ($k in $config.Keys) {
        $reason = Test-WtConfigValue -Key $k -Value $config[$k]
        if ($reason) { "$k`: $reason" }
    })
    if ($invalidKeys.Count -gt 0) {
        $rows += New-WtDoctorRow -Check 'config valida' -Ok $false -FailDetail ($invalidKeys -join '; ') -FailState 'AVISO'
    } elseif (Test-WtWorktreeRootTemplateNeedsRepoToken -Value ([string]$config.worktreeRootTemplate)) {
        $rows += New-WtDoctorRow -Check 'config valida' -Ok $false `
            -FailDetail 'worktreeRootTemplate no tiene {repo} ni {repoParent}: worktrees homonimos de repos distintos podrian colisionar.' `
            -FailState 'AVISO'
    } else {
        $rows += New-WtDoctorRow -Check 'config valida' -Ok $true -OkDetail 'todas las claves tienen valores validos'
    }

    $git = Get-Command git -ErrorAction SilentlyContinue
    $rows += New-WtDoctorRow -Check 'git' -Ok ($null -ne $git) -OkDetail ([string]$git.Source) `
        -FailDetail 'Instala Git: https://git-scm.com/download/win'

    $nodeVersion = Get-WtNodeVersion
    $nodeOk = ($nodeVersion -match '^v?(\d+)' -and [int]$Matches[1] -ge 18)
    $rows += New-WtDoctorRow -Check 'node >= 18' -Ok $nodeOk -OkDetail $nodeVersion `
        -FailDetail "$nodeVersion (Copilot CLI requiere Node >= 18)"

    $fnm = Get-Command fnm -ErrorAction SilentlyContinue
    $rows += New-WtDoctorRow -Check 'fnm (opcional)' -Ok ($null -ne $fnm) -OkDetail ([string]$fnm.Source) `
        -FailDetail 'Recomendado para manejar versiones de Node' -FailState 'AVISO'

    $agentCommand = [string]$config.agentCommand
    $agent = Get-Command $agentCommand -ErrorAction SilentlyContinue
    $rows += New-WtDoctorRow -Check "agente ('$agentCommand')" -Ok ($null -ne $agent) -OkDetail ([string]$agent.Source) `
        -FailDetail "No esta en PATH; ajusta con: wt config set agentCommand <comando> (ej. npm i -g @github/copilot)"

    $editor = [string]$config.editor
    if ($editor) {
        $ed = Get-Command $editor -ErrorAction SilentlyContinue
        $rows += New-WtDoctorRow -Check "editor ('$editor')" -Ok ($null -ne $ed) -OkDetail ([string]$ed.Source) `
            -FailDetail 'No esta en PATH; ajusta con: wt config set editor <comando>'
    } else {
        $rows += New-WtDoctorRow -Check 'editor' -Ok $true -OkDetail 'desactivado en la config'
    }

    $terminal = [string]$config.terminal
    if ($terminal -eq 'warp') {
        $warp = [string]$config.warpPath
        $rows += New-WtDoctorRow -Check 'warp' -Ok (Test-WtPathExists $warp) -OkDetail $warp `
            -FailDetail "No existe '$warp'; ajusta warpPath o: wt config set terminal wt"
    } elseif ($terminal -eq 'wt') {
        $wtcmd = Get-Command wt.exe -ErrorAction SilentlyContinue
        $rows += New-WtDoctorRow -Check 'windows terminal' -Ok ($null -ne $wtcmd) -OkDetail ([string]$wtcmd.Source) `
            -FailDetail 'wt.exe no encontrado'
    } else {
        $rows += New-WtDoctorRow -Check 'terminal' -Ok $true -OkDetail 'desactivado en la config'
    }

    $configFile = Get-WtConfigFilePath
    $rows += New-WtDoctorRow -Check 'archivo de config' -Ok (Test-WtPathExists $configFile) -OkDetail $configFile `
        -FailDetail "Se crea con 'wt config edit' o al instalar: $configFile"

    $reposRoot = [string]$config.reposRoot
    if ($reposRoot) {
        $rows += New-WtDoctorRow -Check 'reposRoot' -Ok (Test-WtPathExists $reposRoot) -OkDetail $reposRoot `
            -FailDetail "No existe '$reposRoot'"
    } else {
        $rows += New-WtDoctorRow -Check 'reposRoot' -Ok $false `
            -FailDetail 'Sin configurar; usa: wt config set reposRoot C:\Repos' -FailState 'AVISO'
    }

    $mainRoot = Find-WtMainRoot -Silent
    if ($mainRoot) {
        $stale = @()
        try { $stale = @(Get-WtWorktrees -RepoRoot $mainRoot | Where-Object { $_.IsPrunable }) } catch { }
        $rows += New-WtDoctorRow -Check 'worktrees obsoletos' -Ok ($stale.Count -eq 0) -OkDetail 'ninguno' `
            -FailDetail ("{0} con directorio inexistente; corre 'wt prune'" -f $stale.Count) -FailState 'AVISO'
    }

    return $rows
}

function Invoke-WtDoctor {
    # Siempre sale con codigo 0: es informativo.
    $rows = @(Get-WtDoctorRows)
    Write-WtInfo 'Chequeo del entorno de trabajo:'
    Write-WtTable -Rows $rows
    $missing = @($rows | Where-Object { $_.Estado -eq 'FALTA' })
    if ($missing.Count -eq 0) {
        Write-WtSuccess 'Setup completo: podes trabajar con Copilot CLI + Warp + VS Code (o solo Warp + Copilot).'
    } else {
        Write-WtNotice ("Faltan {0} componente(s). Setup minimo sugerido: git + node + copilot + warp (editor opcional)." -f $missing.Count)
    }
}
