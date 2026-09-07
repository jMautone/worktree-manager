# ---------------------------------------------------------------------------
# Commands: logica de cada comando del CLI.
# Es la unica capa que combina decision (funciones puras) con efectos y salida.
# Depende de: Common, Config, Repo, Workspace, Launch.
# ---------------------------------------------------------------------------

# --- create -----------------------------------------------------------------

function New-WtWorktree {
    param(
        [Parameter(Mandatory)][string]$Name,
        [AllowEmptyString()][string]$Base,
        [AllowEmptyString()][string]$Branch,
        [switch]$NoOpen
    )
    # Se valida antes de tocar git o la config: el error debe hablar del nombre.
    Assert-WtWorktreeName -Name $Name

    $repoRoot = Find-WtMainRoot
    $config = Get-WtConfig
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
        Write-WtDetail "La rama '$Branch' ya existe; se reutiliza."
        $gitArgs += @($path, $Branch)
    } else {
        $gitArgs += @('-b', $Branch, $path)
        if ($Base) { $gitArgs += $Base }
    }

    Write-WtInfo "Creando worktree '$Name' en $path"
    Invoke-WtGit -WorkingDirectory $repoRoot -Arguments $gitArgs | Out-Null
    Write-WtSuccess "OK - worktree listo: $path (rama: $Branch)"

    if (-not $NoOpen) { Open-WtWorktree -Name $Name }
    return $path
}

function Test-WtRemote {
    param([Parameter(Mandatory)][string]$RepoRoot, [Parameter(Mandatory)][string]$Remote)
    $r = Invoke-WtGit -WorkingDirectory $RepoRoot -Arguments @('remote') -AllowFailure
    if (-not $r.Success) { return $false }
    return (@($r.Text -split "`r?`n" | ForEach-Object { $_.Trim() }) -contains $Remote)
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
        Worktree sobre el que operan 'open' y 'path'.
    .DESCRIPTION
        - Sin nombre: el checkout actual (el worktree si estas parado en uno).
        - Con nombre estando en un repo: se busca por carpeta o rama.
        - Fuera de un repo: el nombre puede ser un repo de reposRoot (se opera sobre su
          checkout principal) o un worktree de alguno de ellos.
    .OUTPUTS
        @{ RepoRoot; Worktree; Name }
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
        RepoRoot = $repoRoot
        Worktree = $worktree
        Name     = (Split-Path -Leaf $worktree.Path)
    }
}

function Invoke-WtPathCommand {
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
    #>
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
    if (-not (Test-WtCommand 'copilot')) {
        Write-WtWarn "No se encontro 'copilot' en la sesion actual; no se lanza el agente."
        return $false
    }
    if ([string]$Config.terminal -ne 'warp' -or -not (Test-WtPathExists ([string]$Config.warpPath))) {
        Write-WtWarn "El agente se abre solo en Warp; configura terminal = 'warp' y un warpPath valido."
        return $false
    }
    $sameWindow = Open-WtAgentInWarp -Name $Name -Path $Path -Target ([string]$Config.warpAgentTarget) `
        -Title $Title -Color ([string]$Config.warpAgentColor)
    Write-WtSuccess ("Agente Copilot CLI iniciado en {0} ({1})" -f $Path, (Format-WtWarpTarget $sameWindow))
    return $true
}

# --- remove / prune ---------------------------------------------------------

function Remove-WtWorktree {
    param(
        [Parameter(Mandatory)][string]$Name,
        [switch]$DeleteBranch,
        [switch]$Force
    )
    $repoRoot = Find-WtMainRoot
    $wt = Resolve-WtWorktree -RepoRoot $repoRoot -Name $Name
    if ($wt.IsMain) { throw 'No se puede eliminar el worktree principal del repositorio.' }

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
        Invoke-WtGit -WorkingDirectory $repoRoot -Arguments @('branch', '-D', $wt.Branch) | Out-Null
        Write-WtSuccess "OK - rama '$($wt.Branch)' eliminada."
    }
}

function Invoke-WtPrune {
    $repoRoot = Find-WtMainRoot
    Invoke-WtGit -WorkingDirectory $repoRoot -Arguments @('worktree', 'prune', '-v') | Out-Null
    Write-WtSuccess 'OK - metadatos de worktrees obsoletos depurados.'
}

# --- doctor -----------------------------------------------------------------

function New-WtDoctorRow {
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
    <#
    .SYNOPSIS
        Chequeos del setup como datos (sin imprimir), para poder testearlos y reusarlos.
    #>
    $config = Get-WtConfig
    $rows = @()

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

    $copilot = Get-Command copilot -ErrorAction SilentlyContinue
    $rows += New-WtDoctorRow -Check 'copilot (agente)' -Ok ($null -ne $copilot) -OkDetail ([string]$copilot.Source) `
        -FailDetail 'Instala Copilot CLI: npm i -g @github/copilot'

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
