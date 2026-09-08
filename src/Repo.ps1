# ---------------------------------------------------------------------------
# Repo: descubrimiento del repositorio, modelo de worktrees y resolucion por nombre.
# Depende de: Common.
# ---------------------------------------------------------------------------

function Get-WtOrphanWorktreeInfo {
    <#
    .SYNOPSIS
        Detecta un worktree huerfano: su .git (archivo) apunta a un gitdir inexistente
        porque el repo principal fue movido o eliminado.
    .OUTPUTS
        @{ Dir; GitDir } o $null.
    #>
    param([string]$StartPath)
    $dir = $StartPath
    if (-not $dir) { $dir = (Get-Location).Path }
    while ($dir) {
        $gitPath = Join-Path $dir '.git'
        if (Test-Path -LiteralPath $gitPath -PathType Leaf) {
            $firstLine = (Get-Content -LiteralPath $gitPath -TotalCount 1 -ErrorAction SilentlyContinue)
            if ($firstLine -match '^gitdir:\s*(.+)$') {
                $gitDir = $Matches[1].Trim()
                if (-not [IO.Path]::IsPathRooted($gitDir)) {
                    $gitDir = [IO.Path]::GetFullPath((Join-Path $dir $gitDir))
                }
                if (-not (Test-WtPathExists $gitDir)) {
                    return @{ Dir = $dir; GitDir = $gitDir }
                }
            }
            return $null
        }
        if (Test-Path -LiteralPath $gitPath -PathType Container) { return $null }
        $parent = Split-Path -Parent $dir
        if ($parent -eq $dir) { break }
        $dir = $parent
    }
    return $null
}

function Find-WtMainRoot {
    <#
    .SYNOPSIS
        Raiz del repositorio principal, tambien desde un subdirectorio o desde otro worktree.
    .DESCRIPTION
        --git-common-dir devuelve el .git del repo principal aun estando dentro de un
        worktree, asi que no hace falta caminar directorios a mano.
    #>
    param([switch]$Silent, [string]$WorkingDirectory)
    $result = Invoke-WtGit -WorkingDirectory $WorkingDirectory `
        -Arguments @('rev-parse', '--path-format=absolute', '--git-common-dir') -AllowFailure
    if (-not $result.Success -or -not $result.Text) {
        if ($Silent) { return $null }
        $orphan = Get-WtOrphanWorktreeInfo
        if ($orphan) {
            throw (("Este directorio es un worktree huerfano: su .git apunta a '{0}', que ya no existe " +
                "(el repo principal fue movido o eliminado). Si el repo sigue existiendo en otra ruta, " +
                "reparalo con 'git worktree repair ""{1}""' ejecutado desde el repo principal; si no, elimina este directorio.") -f $orphan.GitDir, $orphan.Dir)
        }
        throw 'No estas dentro de un repositorio git (ni de uno de sus worktrees).'
    }
    $commonDir = ConvertTo-WtFullPath ([string]($result.StdOut | Select-Object -First 1))
    return (Split-Path -Parent $commonDir)
}

function Get-WtCurrentRoot {
    <#
    .SYNOPSIS
        Raiz del checkout actual: el worktree en el que estamos parados, o la raiz
        principal si no estamos en uno. $null si no estamos en un repo.
    #>
    param([string]$WorkingDirectory)
    $result = Invoke-WtGit -WorkingDirectory $WorkingDirectory `
        -Arguments @('rev-parse', '--path-format=absolute', '--show-toplevel') -AllowFailure
    if (-not $result.Success -or -not $result.Text) { return $null }
    return (ConvertTo-WtFullPath ([string]($result.StdOut | Select-Object -First 1)))
}

function Get-WtBranchAt {
    <#
    .SYNOPSIS
        Rama del checkout ubicado en $Path ('' si esta detached o no se puede leer).
    #>
    param([Parameter(Mandatory)][string]$Path)
    $r = Invoke-WtGit -WorkingDirectory $Path -Arguments @('rev-parse', '--abbrev-ref', 'HEAD') -AllowFailure
    if (-not $r.Success) { return '' }
    $branch = [string]($r.StdOut | Select-Object -First 1)
    $branch = $branch.Trim()
    if ($branch -eq 'HEAD') { return '' }
    return $branch
}

function Test-WtLocalBranch {
    param([Parameter(Mandatory)][string]$RepoRoot, [Parameter(Mandatory)][string]$Branch)
    $r = Invoke-WtGit -WorkingDirectory $RepoRoot `
        -Arguments @('show-ref', '--verify', '--quiet', "refs/heads/$Branch") -AllowFailure
    return $r.Success
}

function New-WtWorktreeInfo {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '',
        Justification = 'Constructor puro (pscustomobject); el verbo New no implica efectos.')]
    param(
        [Parameter(Mandatory)][string]$Path,
        [string]$Head = '',
        [string]$Branch = '',
        [bool]$IsBare = $false,
        [bool]$IsDetached = $false,
        [bool]$IsMain = $false,
        [bool]$IsPrunable = $false,
        [string]$PruneReason = '',
        [bool]$IsLocked = $false,
        [string]$LockReason = ''
    )
    return [pscustomobject]@{
        Path        = $Path
        Head        = $Head
        Branch      = $Branch
        IsBare      = $IsBare
        IsDetached  = $IsDetached
        IsMain      = $IsMain
        IsPrunable  = $IsPrunable
        PruneReason = $PruneReason
        IsLocked    = $IsLocked
        LockReason  = $LockReason
    }
}

function ConvertFrom-WtWorktreePorcelain {
    <#
    .SYNOPSIS
        Parsea la salida de 'git worktree list --porcelain' (formato estable).
    .DESCRIPTION
        Funcion pura (sin git ni disco): permite testear el parsing, incluida la
        deteccion de worktrees obsoletos via el atributo 'prunable' y bloqueados via
        'locked'. git garantiza que la primera entrada es el worktree principal, y de
        ahi sale IsMain.
    #>
    param([AllowEmptyString()][string]$Text)
    $worktrees = @()
    $current = $null
    foreach ($line in ($Text -split "`r?`n")) {
        if ($line -match '^worktree (.+)$') {
            if ($current) { $worktrees += $current }
            $current = New-WtWorktreeInfo -Path (ConvertTo-WtFullPath $Matches[1])
        } elseif ($current) {
            if ($line -match '^HEAD (.+)$') { $current.Head = $Matches[1] }
            elseif ($line -match '^branch refs/heads/(.+)$') { $current.Branch = $Matches[1] }
            elseif ($line -eq 'bare') { $current.IsBare = $true }
            elseif ($line -eq 'detached') { $current.IsDetached = $true }
            elseif ($line -match '^prunable\s*(.*)$') {
                $current.IsPrunable = $true
                $current.PruneReason = $Matches[1].Trim()
            }
            elseif ($line -match '^locked\s*(.*)$') {
                $current.IsLocked = $true
                $current.LockReason = $Matches[1].Trim()
            }
        }
    }
    if ($current) { $worktrees += $current }
    if ($worktrees.Count -gt 0) { $worktrees[0].IsMain = $true }
    return $worktrees
}

# Cache por invocacion: 'wt open <worktree>' fuera de un repo puede recorrer varios
# repos de reposRoot (Find-WtReposOwningWorktree) y volver a consultar el mismo repo
# mas de una vez en el mismo comando. Invoke-Wt la limpia al empezar (junto a
# Clear-WtConfigCache) y despues de create/remove/prune/lock/unlock, asi que nunca
# sobrevive entre comandos distintos aunque el proceso de PowerShell si.
$script:WorktreesCache = @{}

function Clear-WtWorktreesCache {
    $script:WorktreesCache = @{}
}

function Get-WtWorktrees {
    <#
    .SYNOPSIS
        Worktrees del repositorio, con Path, Head, Branch, IsMain, IsBare, IsDetached,
        IsPrunable, PruneReason, IsLocked y LockReason.
    .NOTES
        IsMain se deriva de $RepoRoot (primera entrada del porcelain), no del directorio
        actual: asi el resultado es el mismo se llame desde donde se llame.
    #>
    param([Parameter(Mandatory)][string]$RepoRoot, [switch]$Refresh)
    $key = (ConvertTo-WtFullPath $RepoRoot).ToLowerInvariant()
    if (-not $Refresh -and $script:WorktreesCache.ContainsKey($key)) {
        return $script:WorktreesCache[$key]
    }
    $r = Invoke-WtGit -WorkingDirectory $RepoRoot -Arguments @('worktree', 'list', '--porcelain')
    $result = @(ConvertFrom-WtWorktreePorcelain -Text ($r.StdOut | Out-String))
    $script:WorktreesCache[$key] = $result
    return $result
}

function Test-WtWorktreeMatchesName {
    param([Parameter(Mandatory)]$Worktree, [Parameter(Mandatory)][string]$Name)
    $leaf = Split-Path -Leaf $Worktree.Path
    if ($leaf -ieq $Name) { return $true }
    if ($Worktree.Branch -and $Worktree.Branch -eq ($Name -replace '^refs/heads/', '')) { return $true }
    return $false
}

function Resolve-WtWorktree {
    <#
    .SYNOPSIS
        Busca un worktree por nombre de carpeta o de rama. Rechaza los obsoletos.
    #>
    param([Parameter(Mandatory)][string]$RepoRoot, [Parameter(Mandatory)][string]$Name)
    foreach ($wt in (Get-WtWorktrees -RepoRoot $RepoRoot)) {
        if (-not (Test-WtWorktreeMatchesName -Worktree $wt -Name $Name)) { continue }
        if ($wt.IsPrunable) {
            $reason = $wt.PruneReason
            if (-not $reason) { $reason = 'el directorio ya no existe' }
            throw (("El worktree '$Name' esta obsoleto: '{0}' ya no existe ($reason). " +
                "Corre 'wt prune' para depurarlo y, si lo necesitas, crealo de nuevo con 'wt create $Name'.") -f $wt.Path)
        }
        return $wt
    }
    throw "No existe un worktree llamado '$Name'. Usa 'wt list' para ver los disponibles."
}

function Get-WtWorktreePath {
    <#
    .SYNOPSIS
        Ruta destino de un worktree segun worktreeRootTemplate.
    #>
    param(
        [Parameter(Mandatory)]$Config,
        [Parameter(Mandatory)][string]$RepoRoot,
        [Parameter(Mandatory)][string]$Name
    )
    $template = [string]$Config.worktreeRootTemplate
    if (-not $template) { throw "worktreeRootTemplate esta vacio. Usa: wt config set worktreeRootTemplate '{repoParent}\{repo}.worktrees\{name}'" }
    $resolved = $template.
        Replace('{repoParent}', (Split-Path -Parent $RepoRoot)).
        Replace('{repo}', (Split-Path -Leaf $RepoRoot)).
        Replace('{name}', $Name)
    return (ConvertTo-WtFullPath $resolved)
}

function Get-WtCheckoutInfo {
    <#
    .SYNOPSIS
        Describe un checkout arbitrario (raiz principal o worktree actual) con la misma
        forma que las entradas de Get-WtWorktrees, para que los consumidores no
        distingan casos.
    #>
    param([Parameter(Mandatory)][string]$Path, [string]$RepoRoot)
    $info = New-WtWorktreeInfo -Path (ConvertTo-WtFullPath $Path)
    if ($RepoRoot) { $info.IsMain = (Test-WtPathEquals $info.Path $RepoRoot) }
    if (Test-WtPathExists $info.Path) { $info.Branch = Get-WtBranchAt -Path $info.Path }
    return $info
}
