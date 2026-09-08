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

function Resolve-WtCopyOnCreatePlan {
    <#
    .SYNOPSIS
        Que copiar del repo principal al worktree nuevo segun 'copyOnCreate'. Funcion
        pura: recibe el listado de archivos del repo (relativos, ya enumerados por el
        llamador, que es quien toca disco) y devuelve, por patron, si es valido y que
        matcheo.
    .DESCRIPTION
        Un patron absoluto o con '..' se rechaza (Valid = $false): no se puede copiar
        desde fuera del repo. Los patrones validos que no matchean nada quedan con
        Items vacio; el llamador decide como avisarlo (Write-WtDetail, no falla).
    .OUTPUTS
        Array de @{ Pattern; Valid; Reason; Items (array de @{ From; To }) }.
    #>
    param(
        [Parameter(Mandatory)][AllowEmptyCollection()][string[]]$Patterns,
        [Parameter(Mandatory)][AllowEmptyCollection()][string[]]$RepoFiles
    )
    $normalizedFiles = @($RepoFiles | ForEach-Object { $_.Replace('\', '/') })
    $results = @()
    foreach ($pattern in $Patterns) {
        if (-not $pattern) { continue }
        $normalizedPattern = $pattern.Replace('\', '/')
        if ([IO.Path]::IsPathRooted($pattern) -or $normalizedPattern -match '(^|/)\.\.(/|$)') {
            $results += @{
                Pattern = $pattern; Valid = $false
                Reason  = 'ruta absoluta o con .. (no se puede copiar desde fuera del repo)'
                Items   = @()
            }
            continue
        }
        $wildcard = New-Object System.Management.Automation.WildcardPattern(
            $normalizedPattern, [System.Management.Automation.WildcardOptions]::IgnoreCase)
        $matchedFiles = @($normalizedFiles | Where-Object { $wildcard.IsMatch($_) })
        $items = @($matchedFiles | ForEach-Object { @{ From = $_; To = $_ } })
        $results += @{ Pattern = $pattern; Valid = $true; Reason = ''; Items = $items }
    }
    return $results
}

function Invoke-WtCreateHooks {
    <#
    .SYNOPSIS
        Copia archivos (copyOnCreate) y corre comandos (postCreate) en el worktree
        recien creado, antes de abrir el editor/agente.
    .DESCRIPTION
        Si un postCreate falla, se detiene la cadena y se relanza: el worktree ya
        existe (git worktree add ya corrio) y New-WtWorktree no lo destruye. Ninguna
        de las dos claves entra en la lista blanca de .wt.json (A1): son ejecucion de
        codigo, solo se pueden definir en la config global o en WT_CONFIG.
    #>
    param(
        [Parameter(Mandatory)][string]$RepoRoot,
        [Parameter(Mandatory)][string]$WorktreePath,
        [Parameter(Mandatory)]$Config
    )
    $patterns = @($Config.copyOnCreate)
    if ($patterns.Count -gt 0) {
        $repoFiles = @(Get-ChildItem -LiteralPath $RepoRoot -Recurse -File -Force -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -notmatch '(^|\\)\.git(\\|$)' } |
            ForEach-Object { $_.FullName.Substring($RepoRoot.Length + 1) })
        $plan = @(Resolve-WtCopyOnCreatePlan -Patterns $patterns -RepoFiles $repoFiles)
        foreach ($entry in $plan) {
            if (-not $entry.Valid) {
                Write-WtWarn "copyOnCreate: patron '$($entry.Pattern)' invalido: $($entry.Reason)."
                continue
            }
            if ($entry.Items.Count -eq 0) {
                Write-WtDetail "copyOnCreate: patron '$($entry.Pattern)' no matcheo ningun archivo."
                continue
            }
            foreach ($item in $entry.Items) {
                if (Test-WtDryRun) {
                    Write-WtDetail "[dry-run] copiar $($item.From) -> $WorktreePath\$($item.To)"
                    continue
                }
                $from = Join-Path $RepoRoot $item.From
                $to = Join-Path $WorktreePath $item.To
                $toDir = Split-Path -Parent $to
                if ($toDir -and -not (Test-WtPathExists $toDir)) {
                    New-Item -ItemType Directory -Path $toDir -Force | Out-Null
                }
                Copy-Item -LiteralPath $from -Destination $to -Force
                Write-WtDetail "copyOnCreate: copiado $($item.From)"
            }
        }
    }

    foreach ($command in @($Config.postCreate)) {
        if (-not $command) { continue }
        Write-WtDetail "postCreate: $command"
        $result = Invoke-WtProcess -FilePath 'cmd.exe' -Arguments @('/c', $command) `
            -WorkingDirectory $WorktreePath -AllowFailure
        if (-not $result.Success) {
            $detail = $result.ErrorText
            if (-not $detail) { $detail = $result.Text }
            Write-WtError "postCreate '$command' fallo (exit $($result.ExitCode)): $detail. El worktree quedo creado en $WorktreePath."
            throw "postCreate '$command' fallo (exit $($result.ExitCode)); la cadena de hooks se detuvo."
        }
    }
}

function New-WtWorktree {
    <#
    .OUTPUTS
        La ruta del worktree creado (string). Util para consumidores del modulo que
        llaman a la funcion directamente; el CLI y la consola mandan el retorno a
        Out-Null a proposito, para que stdout quede reservado a 'wt path' y '--json'.
    #>
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
        [switch]$NoOpen,
        [switch]$NoHooks
    )
    # Se valida antes de tocar git o la config: el error debe hablar del nombre.
    Assert-WtWorktreeName -Name $Name

    $repoRoot = Find-WtMainRoot
    $config = Get-WtConfig -RepoRoot $repoRoot
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

    if (-not $NoHooks) {
        Invoke-WtCreateHooks -RepoRoot $repoRoot -WorktreePath $path -Config $config
    }

    $openPlan = Get-WtCreateOpenPlan -Config $config -Code:$Code -Terminal:$Terminal -Agent:$Agent -All:$All -NoOpen:$NoOpen
    if ($openPlan.Code -or $openPlan.Terminal -or $openPlan.Agent) {
        if (Test-WtDryRun) {
            # El path no existe de verdad (el 'git worktree add' fue un no-op):
            # Open-WtWorktree fallaria al chequear el directorio. Solo se informa.
            Write-WtDetail ("[dry-run] abrir (Code={0} Terminal={1} Agent={2})" -f $openPlan.Code, $openPlan.Terminal, $openPlan.Agent)
        } else {
            Open-WtWorktree -Name $Name -Code:$openPlan.Code -Terminal:$openPlan.Terminal -Agent:$openPlan.Agent
        }
    }
    return $path
}

function Test-WtRemote {
    param([Parameter(Mandatory)][string]$RepoRoot, [Parameter(Mandatory)][string]$Remote)
    $r = Invoke-WtGit -WorkingDirectory $RepoRoot -Arguments @('remote') -AllowFailure -ReadOnly
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
        if ($wt.IsLocked) {
            $nombre = "$nombre (bloqueado)"
            if ($wt.LockReason) { $nombre = "${nombre}: $($wt.LockReason)" }
        }
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
        ConvertTo-WtJson -InputObject @($worktrees | Select-Object Path, Branch, Head, IsMain, IsDetached, IsPrunable, IsLocked, LockReason)
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
        [switch]$All
    )
    $explicit = [bool]$Code -or [bool]$Terminal -or [bool]$Agent -or [bool]$All
    $wantCode = ([bool]$Code -or [bool]$All) -or (-not $explicit)
    $wantAgent = [bool]$Agent -or [bool]$All
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
    #>
    param(
        [AllowEmptyString()][string]$Name,
        [switch]$Code,
        [switch]$Terminal,
        [switch]$Agent,
        [switch]$All
    )
    $plan = Get-WtOpenPlan -Code:$Code -Terminal:$Terminal -Agent:$Agent -All:$All
    $target = Resolve-WtTarget -Name $Name
    if ($target.ShouldRelocate) {
        Set-WtLocation -Path $target.RepoRoot
        if ($target.Source -eq 'repo') {
            Write-WtDetail "Ahora en el repo: $($target.RepoRoot)"
        } else {
            Write-WtDetail "El worktree '$Name' es del repo '$(Split-Path -Leaf $target.RepoRoot)'; ahora en $($target.RepoRoot)"
        }
    }
    $config = Get-WtConfig -RepoRoot $target.RepoRoot
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
        Start-WtProcess -FilePath 'wt.exe' -ArgumentList @('-d', (Format-WtProcessArgument $Path))
        Write-WtSuccess "Windows Terminal abierto en $Path"
        return $true
    }
    return $false
}

function Open-WtAgent {
    <#
    .NOTES
        El agente se lanza como tab de Warp o de Windows Terminal (segun 'terminal');
        nunca como ventana suelta de PowerShell.
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
    $terminal = [string]$Config.terminal
    if ($terminal -eq 'warp') {
        if (-not (Test-WtPathExists ([string]$Config.warpPath))) {
            Write-WtWarn "Warp no encontrado en '$($Config.warpPath)' (ajusta warpPath en la config)."
            return $false
        }
        $sameWindow = Open-WtAgentInWarp -Name $Name -Path $Path -Config $Config -Target ([string]$Config.warpAgentTarget) `
            -Title $Title -Color ([string]$Config.warpAgentColor)
        Write-WtSuccess ("Agente '{0}' iniciado en {1} ({2})" -f $agentCommand, $Path, (Format-WtWarpTarget $sameWindow))
        return $true
    }
    if ($terminal -eq 'wt') {
        if (-not (Test-WtCommand 'wt.exe')) {
            Write-WtWarn 'Windows Terminal (wt.exe) no encontrado.'
            return $false
        }
        Open-WtAgentInWindowsTerminal -Path $Path -Title $Title -Config $Config
        Write-WtSuccess ("Agente '{0}' iniciado en {1} (pestana de Windows Terminal)" -f $agentCommand, $Path)
        return $true
    }
    Write-WtWarn "El agente no se abre con terminal = '$terminal'; configura terminal = 'warp' o 'wt'."
    return $false
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
    if ($wt.IsLocked) {
        $reason = $wt.LockReason
        if (-not $reason) { $reason = 'sin motivo especificado' }
        throw "El worktree '$Name' esta bloqueado ($reason). Usa 'wt unlock $Name' para desbloquearlo antes de eliminarlo."
    }

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

function Invoke-WtLock {
    <#
    .SYNOPSIS
        Bloquea un worktree ('git worktree lock'): 'wt remove' y 'wt prune' lo rechazan
        mientras siga bloqueado.
    #>
    param(
        [Parameter(Mandatory)][string]$Name,
        [AllowEmptyString()][string]$Reason
    )
    $repoRoot = Find-WtMainRoot
    $wt = Resolve-WtWorktree -RepoRoot $repoRoot -Name $Name
    if ($wt.IsMain) { throw 'El worktree principal no se bloquea.' }
    $gitArgs = @('worktree', 'lock')
    if ($Reason) { $gitArgs += @('--reason', $Reason) }
    $gitArgs += $wt.Path
    Invoke-WtGit -WorkingDirectory $repoRoot -Arguments $gitArgs | Out-Null
    Write-WtSuccess "OK - worktree '$Name' bloqueado."
}

function Invoke-WtUnlock {
    <#
    .SYNOPSIS
        Desbloquea un worktree ('git worktree unlock').
    #>
    param([Parameter(Mandatory)][string]$Name)
    $repoRoot = Find-WtMainRoot
    $wt = Resolve-WtWorktree -RepoRoot $repoRoot -Name $Name
    Invoke-WtGit -WorkingDirectory $repoRoot -Arguments @('worktree', 'unlock', $wt.Path) | Out-Null
    Write-WtSuccess "OK - worktree '$Name' desbloqueado."
}

function Invoke-WtPrune {
    <#
    .SYNOPSIS
        Depura metadatos de worktrees obsoletos ('git worktree prune -v') y muestra
        que se depuro (o dice explicitamente que no habia nada).
    .DESCRIPTION
        git escribe la salida verbosa de 'worktree prune -v' (una linea 'Removing
        worktrees/<nombre>: <motivo>' por cada entrada depurada) en stderr, no en
        stdout; de ahi sale de StdErr, no de StdOut (ver M1).
    #>
    $repoRoot = Find-WtMainRoot
    $r = Invoke-WtGit -WorkingDirectory $repoRoot -Arguments @('worktree', 'prune', '-v')
    $lines = @($r.StdErr | Where-Object { $_ })
    if ($lines.Count -eq 0) {
        Write-WtDetail 'No habia metadatos de worktrees obsoletos.'
    } else {
        foreach ($line in $lines) { Write-WtDetail $line }
    }
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
            if (Test-WtDryRun) {
                Write-WtDetail ("[dry-run] borrar {0} (directory ya no existe: {1})" -f $file.Name, $directory)
            } else {
                Remove-Item -LiteralPath $file.FullName -Force -ErrorAction SilentlyContinue
                Write-WtDetail ("Eliminado: {0} (directory ya no existe: {1})" -f $file.Name, $directory)
            }
            $removed++
        }
    }
    if ($removed -eq 0) {
        Write-WtSuccess 'OK - no habia tab configs huerfanos.'
    } else {
        Write-WtSuccess ("OK - {0} tab config(s) huerfano(s) depurado(s)." -f $removed)
    }
}

# --- status -------------------------------------------------------------

function Get-WtStatusRows {
    <#
    .SYNOPSIS
        Filas de presentacion de 'wt status'. Funcion pura sobre las entradas que
        arma Get-WtStatusEntry (como Get-WtWorktreeRows con Get-WtWorktrees).
    #>
    param([Parameter(Mandatory)][AllowEmptyCollection()][object[]]$Entries)
    return @(foreach ($e in $Entries) {
        if ($e.Error) {
            [pscustomobject]@{
                Worktree = $e.Name; Rama = $e.Branch; Cambios = 'ERROR'
                'Ahead/Behind' = ''; 'Ultimo commit' = $e.Error
            }
            continue
        }
        $rama = $e.Branch
        if (-not $rama) { $rama = '(detached)' }
        $cambios = "{0} staged, {1} modified, {2} sin trackear" -f $e.Staged, $e.Modified, $e.Untracked
        $ab = "+{0}/-{1}" -f $e.Ahead, $e.Behind
        $ultimo = ''
        if ($e.LastCommitHash) { $ultimo = "{0} {1} ({2})" -f $e.LastCommitHash, $e.LastCommitSubject, $e.LastCommitAge }
        [pscustomobject]@{
            Worktree = $e.Name; Rama = $rama; Cambios = $cambios
            'Ahead/Behind' = $ab; 'Ultimo commit' = $ultimo
        }
    })
}

function Get-WtStatusEntry {
    <#
    .SYNOPSIS
        Estado de UN worktree: status porcelain v2 + ahead/behind + ultimo commit.
    .DESCRIPTION
        Nunca lanza: un worktree que falle (bloqueado, disco desconectado, etc.)
        devuelve 'Error' con el motivo en vez de romper el resto del listado de 'wt
        status'. Ahead/behind sale del upstream si lo tiene; si no, y hay
        'defaultBase' configurado, se calcula contra esa rama con 'rev-list
        --left-right --count' (best-effort: si falla, queda en 0/0).
    #>
    param([Parameter(Mandatory)]$Worktree, [Parameter(Mandatory)]$Config)
    $name = Split-Path -Leaf $Worktree.Path
    try {
        $statusResult = Invoke-WtGit -WorkingDirectory $Worktree.Path -Arguments @('status', '--porcelain=v2', '--branch') -ReadOnly
        $model = ConvertFrom-WtStatusPorcelainV2 -Text ($statusResult.StdOut | Out-String)
        $ahead = $model.Ahead
        $behind = $model.Behind
        if (-not $model.Upstream -and [string]$Config.defaultBase) {
            $range = "$([string]$Config.defaultBase)...HEAD"
            $revResult = Invoke-WtGit -WorkingDirectory $Worktree.Path -Arguments @('rev-list', '--left-right', '--count', $range) -AllowFailure -ReadOnly
            if ($revResult.Success -and $revResult.Text -match '^(\d+)\s+(\d+)$') {
                $behind = [int]$Matches[1]
                $ahead = [int]$Matches[2]
            }
        }
        $logResult = Invoke-WtGit -WorkingDirectory $Worktree.Path -Arguments @('log', '-1', '--format=%h%x09%s%x09%cr') -AllowFailure -ReadOnly
        $hash = ''; $subject = ''; $age = ''
        if ($logResult.Success -and $logResult.Text) {
            $logParts = $logResult.Text -split "`t"
            if ($logParts.Count -ge 3) { $hash = $logParts[0]; $subject = $logParts[1]; $age = $logParts[2] }
        }
        return [pscustomobject]@{
            Name = $name; Path = $Worktree.Path; Branch = $model.Branch; Upstream = $model.Upstream
            Ahead = $ahead; Behind = $behind
            Staged = @($model.Staged).Count; Modified = @($model.Modified).Count; Untracked = @($model.Untracked).Count
            LastCommitHash = $hash; LastCommitSubject = $subject; LastCommitAge = $age
            Error = ''
        }
    } catch {
        return [pscustomobject]@{
            Name = $name; Path = $Worktree.Path; Branch = ''; Upstream = ''; Ahead = 0; Behind = 0
            Staged = 0; Modified = 0; Untracked = 0; LastCommitHash = ''; LastCommitSubject = ''; LastCommitAge = ''
            Error = $_.Exception.Message
        }
    }
}

function Invoke-WtStatus {
    <#
    .SYNOPSIS
        'wt status [<nombre>] [--json] [--fetch]': estado de uno o todos los
        worktrees del repo actual. --fetch hace 'git fetch --all --prune' antes de
        calcular; sin el flag, nunca toca la red (es un comando de consulta que se va
        a correr seguido).
    #>
    param([AllowEmptyString()][string]$Name, [switch]$Json, [switch]$Fetch)
    $repoRoot = Find-WtMainRoot
    $config = Get-WtConfig -RepoRoot $repoRoot
    if ($Fetch) {
        Write-WtDetail 'Actualizando (git fetch --all --prune)...'
        Invoke-WtGit -WorkingDirectory $repoRoot -Arguments @('fetch', '--all', '--prune') -AllowFailure | Out-Null
    }
    $worktrees = @(Get-WtWorktrees -RepoRoot $repoRoot | Where-Object { -not $_.IsPrunable })
    if ($Name) {
        $worktrees = @(Resolve-WtWorktree -RepoRoot $repoRoot -Name $Name)
    }
    $entries = @($worktrees | ForEach-Object { Get-WtStatusEntry -Worktree $_ -Config $config })
    if ($Json) {
        ConvertTo-WtJson -InputObject $entries
        return
    }
    Write-WtDetail ("Repositorio: {0}" -f $repoRoot)
    Write-WtTable -Rows (Get-WtStatusRows -Entries $entries)
}

# --- exec / each ---------------------------------------------------------

function Get-WtEachPlan {
    <#
    .SYNOPSIS
        Sobre que worktrees opera 'wt each'. Funcion pura: excluye los obsoletos
        siempre, y el principal salvo -IncludeMain.
    #>
    param([Parameter(Mandatory)][AllowEmptyCollection()][object[]]$Worktrees, [switch]$IncludeMain)
    return @($Worktrees | Where-Object { -not $_.IsPrunable -and (-not $_.IsMain -or $IncludeMain) })
}

function Invoke-WtCommandLine {
    <#
    .SYNOPSIS
        Corre una linea de comando (ya reconstruida desde 'Rest') en un worktree.
        Efecto compartido por Invoke-WtExec e Invoke-WtEach.
    #>
    param([Parameter(Mandatory)][string]$CommandLine, [Parameter(Mandatory)][string]$WorkingDirectory)
    return (Invoke-WtProcess -FilePath 'cmd.exe' -Arguments @('/c', $CommandLine) -WorkingDirectory $WorkingDirectory -AllowFailure)
}

function Invoke-WtExec {
    <#
    .SYNOPSIS
        'wt exec <nombre> -- <comando...>': corre el comando en ese worktree.
    #>
    param([Parameter(Mandatory)][string]$Name, [Parameter(Mandatory)][AllowEmptyCollection()][string[]]$CommandArgs)
    $repoRoot = Find-WtMainRoot
    $wt = Resolve-WtWorktree -RepoRoot $repoRoot -Name $Name
    $commandLine = ($CommandArgs -join ' ')
    if (Test-WtDryRun) {
        Write-WtDetail "[dry-run] en $($wt.Path): $commandLine"
        return
    }
    $result = Invoke-WtCommandLine -CommandLine $commandLine -WorkingDirectory $wt.Path
    if ($result.Text) { Write-WtLine $result.Text }
    if ($result.ErrorText) { Write-WtLine $result.ErrorText }
    if (-not $result.Success) {
        throw "'$commandLine' fallo en '$Name' (exit $($result.ExitCode))."
    }
}

function Invoke-WtEach {
    <#
    .SYNOPSIS
        'wt each [--continue-on-error] [--json] -- <comando...>': corre el comando en
        todos los worktrees del repo actual (excepto el principal y los obsoletos),
        en serie.
    .DESCRIPTION
        Sin --continue-on-error, corta en el primer worktree que falle; con el flag,
        sigue y al final resume que worktrees fallaron. Exit code (B6): 0 si todos
        salieron 0, 2 si alguno fallo (via throw, que Invoke-Wt traduce).
    #>
    param(
        [Parameter(Mandatory)][AllowEmptyCollection()][string[]]$CommandArgs,
        [switch]$ContinueOnError,
        [switch]$Json
    )
    $repoRoot = Find-WtMainRoot
    $worktrees = @(Get-WtEachPlan -Worktrees (Get-WtWorktrees -RepoRoot $repoRoot))
    $commandLine = ($CommandArgs -join ' ')
    $results = @()
    $failed = @()
    foreach ($wt in $worktrees) {
        $name = Split-Path -Leaf $wt.Path
        if (Test-WtDryRun) {
            Write-WtDetail "[dry-run] en $name ($($wt.Path)): $commandLine"
            continue
        }
        if (-not $Json) { Write-WtInfo $name }
        $result = Invoke-WtCommandLine -CommandLine $commandLine -WorkingDirectory $wt.Path
        if (-not $Json) {
            if ($result.Text) { Write-WtLine $result.Text }
            if ($result.ErrorText) { Write-WtLine $result.ErrorText }
            if (-not $result.Success) { Write-WtDetail "exit $($result.ExitCode)" }
        }
        $results += [pscustomobject]@{
            Name = $name; ExitCode = $result.ExitCode; Success = $result.Success
            Output = $result.Text; ErrorOutput = $result.ErrorText
        }
        if (-not $result.Success) {
            $failed += $name
            if (-not $ContinueOnError) { break }
        }
    }
    if ($Json) {
        ConvertTo-WtJson -InputObject $results
    } elseif (-not (Test-WtDryRun)) {
        if ($failed.Count -eq 0) {
            Write-WtSuccess ("OK - {0} worktree(s), todos exitosos." -f $results.Count)
        } else {
            Write-WtNotice ("Fallaron: {0}" -f ($failed -join ', '))
        }
    }
    if ($failed.Count -gt 0) {
        throw "wt each: fallo en $($failed.Count) worktree(s): $($failed -join ', ')."
    }
}

# --- sync -----------------------------------------------------------------

function Get-WtSyncPlan {
    <#
    .SYNOPSIS
        Decide, por worktree, la accion de 'wt sync'. Funcion pura: recibe el modelo
        de estado del item 5 (Get-WtStatusEntry) y config, sin tocar git.
    .DESCRIPTION
        Un worktree sucio (staged/modified/untracked > 0) se saltea SIEMPRE, nunca se
        hace stash automatico. La base es: --base explicito > upstream de la rama >
        defaultBase. Sin ninguna de las tres, tambien se saltea.
    .OUTPUTS
        Array de @{ Name; Action ('sync'|'skip-dirty'|'skip-no-base'); Base; Strategy;
        GitArgs (array listo para Invoke-WtGit cuando Action es 'sync') }.
    #>
    param(
        [Parameter(Mandatory)][AllowEmptyCollection()][object[]]$Entries,
        [AllowEmptyString()][string]$ExplicitBase,
        [string]$Strategy = 'rebase',
        [AllowEmptyString()][string]$DefaultBase
    )
    $plan = @()
    foreach ($e in $Entries) {
        $dirty = ($e.Staged -gt 0 -or $e.Modified -gt 0 -or $e.Untracked -gt 0)
        if ($dirty) {
            $plan += [pscustomobject]@{ Name = $e.Name; Action = 'skip-dirty'; Base = ''; Strategy = $Strategy; GitArgs = @() }
            continue
        }
        $base = $ExplicitBase
        if (-not $base) { $base = $e.Upstream }
        if (-not $base) { $base = $DefaultBase }
        if (-not $base) {
            $plan += [pscustomobject]@{ Name = $e.Name; Action = 'skip-no-base'; Base = ''; Strategy = $Strategy; GitArgs = @() }
            continue
        }
        $verb = 'rebase'
        if ($Strategy -eq 'merge') { $verb = 'merge' }
        $plan += [pscustomobject]@{ Name = $e.Name; Action = 'sync'; Base = $base; Strategy = $Strategy; GitArgs = @($verb, $base) }
    }
    return $plan
}

function Invoke-WtSync {
    <#
    .SYNOPSIS
        'wt sync [<nombre>] [--base <rama>] [--strategy rebase|merge]
        [--continue-on-error]': rebasa (o mergea) cada worktree contra su base.
    .DESCRIPTION
        Un solo 'git fetch' al inicio, no uno por worktree. Un conflicto no se
        resuelve ni se aborta solo: se reporta, el worktree queda como git lo dejo, y
        se sigue con el proximo (o se corta, segun el flag). El resumen final lista
        sincronizados, salteados por sucios, salteados sin base, y en conflicto.
    #>
    param(
        [AllowEmptyString()][string]$Name,
        [AllowEmptyString()][string]$Base,
        [string]$Strategy = 'rebase',
        [switch]$ContinueOnError
    )
    if ($Strategy -notin 'rebase', 'merge') {
        throw "Uso: wt sync [<nombre>] [--base <rama>] [--strategy rebase|merge] [--continue-on-error]. --strategy debe ser 'rebase' o 'merge'."
    }
    $repoRoot = Find-WtMainRoot
    $config = Get-WtConfig -RepoRoot $repoRoot
    Write-WtDetail 'Actualizando (git fetch --all --prune)...'
    Invoke-WtGit -WorkingDirectory $repoRoot -Arguments @('fetch', '--all', '--prune') -AllowFailure | Out-Null

    $worktrees = @(Get-WtWorktrees -RepoRoot $repoRoot | Where-Object { -not $_.IsPrunable -and -not $_.IsMain })
    if ($Name) { $worktrees = @(Resolve-WtWorktree -RepoRoot $repoRoot -Name $Name) }
    $entries = @($worktrees | ForEach-Object { Get-WtStatusEntry -Worktree $_ -Config $config })
    $plan = @(Get-WtSyncPlan -Entries $entries -ExplicitBase $Base -Strategy $Strategy -DefaultBase ([string]$config.defaultBase))

    $synced = @(); $skippedDirty = @(); $skippedNoBase = @(); $conflicted = @()
    foreach ($item in $plan) {
        $wt = @($worktrees | Where-Object { (Split-Path -Leaf $_.Path) -eq $item.Name })[0]
        switch ($item.Action) {
            'skip-dirty' {
                Write-WtNotice "Saltea '$($item.Name)': tiene cambios sin commitear."
                $skippedDirty += $item.Name
            }
            'skip-no-base' {
                Write-WtNotice "Saltea '$($item.Name)': sin upstream ni defaultBase configurado."
                $skippedNoBase += $item.Name
            }
            'sync' {
                if (Test-WtDryRun) {
                    Write-WtDetail "[dry-run] en $($item.Name): git $($item.GitArgs -join ' ')"
                    continue
                }
                Write-WtInfo "Sincronizando '$($item.Name)' ($($item.Strategy) sobre $($item.Base))..."
                $result = Invoke-WtGit -WorkingDirectory $wt.Path -Arguments $item.GitArgs -AllowFailure
                if ($result.Success) {
                    $synced += $item.Name
                    Write-WtSuccess "OK - '$($item.Name)' sincronizado."
                } else {
                    $conflicted += $item.Name
                    $detail = $result.ErrorText
                    if (-not $detail) { $detail = $result.Text }
                    Write-WtNotice "Conflicto en '$($item.Name)': $detail El worktree queda como git lo dejo; resolvelo a mano (o git $($item.Strategy) --abort)."
                    if (-not $ContinueOnError) { break }
                }
            }
        }
    }
    if (Test-WtDryRun) { return }
    Write-WtLine ''
    Write-WtInfo 'Resumen:'
    Write-WtDetail ("Sincronizados: {0}" -f $(if ($synced.Count -gt 0) { $synced -join ', ' } else { 'ninguno' }))
    Write-WtDetail ("Salteados (sucios): {0}" -f $(if ($skippedDirty.Count -gt 0) { $skippedDirty -join ', ' } else { 'ninguno' }))
    Write-WtDetail ("Salteados (sin base): {0}" -f $(if ($skippedNoBase.Count -gt 0) { $skippedNoBase -join ', ' } else { 'ninguno' }))
    if ($conflicted.Count -gt 0) {
        Write-WtNotice ("En conflicto: {0}" -f ($conflicted -join ', '))
        throw "wt sync: conflicto en $($conflicted.Count) worktree(s): $($conflicted -join ', ')."
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
    $rows += New-WtDoctorRow -Check 'git' -Ok ($null -ne $git) -OkDetail (Get-WtCommandSource $git) `
        -FailDetail 'Instala Git: https://git-scm.com/download/win'

    $nodeVersion = Get-WtNodeVersion
    $nodeOk = ($nodeVersion -match '^v?(\d+)' -and [int]$Matches[1] -ge 18)
    $rows += New-WtDoctorRow -Check 'node >= 18' -Ok $nodeOk -OkDetail $nodeVersion `
        -FailDetail "$nodeVersion (Copilot CLI requiere Node >= 18)"

    $fnm = Get-Command fnm -ErrorAction SilentlyContinue
    $rows += New-WtDoctorRow -Check 'fnm (opcional)' -Ok ($null -ne $fnm) -OkDetail (Get-WtCommandSource $fnm) `
        -FailDetail 'Recomendado para manejar versiones de Node' -FailState 'AVISO'

    $agentCommand = [string]$config.agentCommand
    $agent = Get-Command $agentCommand -ErrorAction SilentlyContinue
    $rows += New-WtDoctorRow -Check "agente ('$agentCommand')" -Ok ($null -ne $agent) -OkDetail (Get-WtCommandSource $agent) `
        -FailDetail "No esta en PATH; ajusta con: wt config set agentCommand <comando> (ej. npm i -g @github/copilot)"

    $editor = [string]$config.editor
    if ($editor) {
        $ed = Get-Command $editor -ErrorAction SilentlyContinue
        $rows += New-WtDoctorRow -Check "editor ('$editor')" -Ok ($null -ne $ed) -OkDetail (Get-WtCommandSource $ed) `
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
        $rows += New-WtDoctorRow -Check 'windows terminal' -Ok ($null -ne $wtcmd) -OkDetail (Get-WtCommandSource $wtcmd) `
            -FailDetail 'wt.exe no encontrado'
    } else {
        $rows += New-WtDoctorRow -Check 'terminal' -Ok $true -OkDetail 'desactivado en la config'
    }

    $configFile = Get-WtConfigFilePath
    $rows += New-WtDoctorRow -Check 'archivo de config' -Ok (Test-WtPathExists $configFile) -OkDetail $configFile `
        -FailDetail "Se crea con 'wt config edit' o al instalar: $configFile"

    $reposRootList = @(ConvertTo-WtReposRootList -Value $config.reposRoot)
    if ($reposRootList.Count -gt 0) {
        $missing = @($reposRootList | Where-Object { -not (Test-WtPathExists $_) })
        $rows += New-WtDoctorRow -Check 'reposRoot' -Ok ($missing.Count -eq 0) -OkDetail ($reposRootList -join ', ') `
            -FailDetail ("No existe: {0}" -f ($missing -join ', '))
    } else {
        $rows += New-WtDoctorRow -Check 'reposRoot' -Ok $false `
            -FailDetail 'Sin configurar; usa: wt config set reposRoot C:\Repos' -FailState 'AVISO'
    }

    $copyPatterns = @($config.copyOnCreate)
    $postCommands = @($config.postCreate)
    if ($copyPatterns.Count -eq 0 -and $postCommands.Count -eq 0) {
        $rows += New-WtDoctorRow -Check 'hooks de creacion' -Ok $true -OkDetail 'sin copyOnCreate ni postCreate configurados'
    } else {
        $rows += New-WtDoctorRow -Check 'hooks de creacion' -Ok $true `
            -OkDetail ("{0} patron(es) copyOnCreate, {1} comando(s) postCreate" -f $copyPatterns.Count, $postCommands.Count)
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
