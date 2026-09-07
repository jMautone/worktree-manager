# ---------------------------------------------------------------------------
# Workspace: raiz de repos, listado, 'cd' y resolucion del repo de trabajo.
# Depende de: Common, Config, Repo.
# ---------------------------------------------------------------------------

function Get-WtReposRoot {
    param([Parameter(Mandatory)]$Config)
    $root = [string]$Config.reposRoot
    if (-not $root) { throw 'reposRoot no esta configurado. Usa: wt config set reposRoot C:\Repos' }
    if (-not (Test-WtPathExists $root)) { throw "reposRoot '$root' no existe." }
    return $root
}

function Get-WtRepoDirs {
    <#
    .SYNOPSIS
        Subdirectorios de la raiz que son repos git (tienen .git, carpeta o archivo).
    #>
    param([Parameter(Mandatory)][string]$Root)
    return @(Get-ChildItem -LiteralPath $Root -Directory -ErrorAction SilentlyContinue |
        Where-Object { Test-Path -LiteralPath (Join-Path $_.FullName '.git') } |
        Sort-Object Name)
}

function Select-WtRepoMatches {
    <#
    .SYNOPSIS
        Match exacto por nombre; si no hay, prefijo unico (case-insensitive).
    .DESCRIPTION
        Unica implementacion del criterio de matcheo, compartida por 'wt cd' y por la
        resolucion de contexto de 'open'/'path'.
    #>
    param([Parameter(Mandatory)][AllowEmptyCollection()][object[]]$Repos, [Parameter(Mandatory)][string]$Name)
    $exact = @($Repos | Where-Object { $_.Name -ieq $Name })
    if ($exact.Count -gt 0) { return $exact }
    return @($Repos | Where-Object { $_.Name.StartsWith($Name, [System.StringComparison]::OrdinalIgnoreCase) })
}

function Resolve-WtRepoDir {
    <#
    .SYNOPSIS
        Resuelve un repo de la raiz por nombre. Con -AllowMissing devuelve $null en vez
        de fallar cuando no hay coincidencias (la ambiguedad siempre falla).
    #>
    param(
        [Parameter(Mandatory)][string]$Root,
        [Parameter(Mandatory)][string]$Name,
        [switch]$AllowMissing
    )
    $match = @(Select-WtRepoMatches -Repos (Get-WtRepoDirs -Root $Root) -Name $Name)
    if ($match.Count -gt 1) {
        throw ("El nombre '$Name' es ambiguo; coincide con: {0}" -f (($match | ForEach-Object { $_.Name }) -join ', '))
    }
    if ($match.Count -eq 0) {
        if ($AllowMissing) { return $null }
        throw "No hay un repo '$Name' en '$Root'. Usa 'wt repos' para ver los disponibles."
    }
    return $match[0]
}

function Find-WtRepoOwningWorktree {
    <#
    .SYNOPSIS
        Primer repo de la raiz que tenga un worktree con ese nombre (carpeta o rama).
    #>
    param([Parameter(Mandatory)][string]$Root, [Parameter(Mandatory)][string]$Name)
    foreach ($dir in (Get-WtRepoDirs -Root $Root)) {
        $worktrees = @()
        try { $worktrees = @(Get-WtWorktrees -RepoRoot $dir.FullName) } catch { continue }
        foreach ($wt in $worktrees) {
            if (Test-WtWorktreeMatchesName -Worktree $wt -Name $Name) { return $dir }
        }
    }
    return $null
}

function Resolve-WtRepoContext {
    <#
    .SYNOPSIS
        Repo sobre el que operar: el actual si estamos dentro de uno; si no, se busca
        el nombre en reposRoot (primero como repo, despues como worktree de algun repo)
        y se hace cd ahi.
    .OUTPUTS
        @{ RepoRoot; Source = 'current' | 'repo' | 'worktree' }
    #>
    param([AllowEmptyString()][string]$Name)
    $repoRoot = Find-WtMainRoot -Silent
    if ($repoRoot) { return @{ RepoRoot = $repoRoot; Source = 'current' } }
    # Sin nombre no hay nada que resolver: se relanza sin -Silent para dar el error
    # detallado (worktree huerfano o "no estas en un repo").
    if (-not $Name) { return @{ RepoRoot = (Find-WtMainRoot); Source = 'current' } }

    $root = Get-WtReposRoot -Config (Get-WtConfig)

    $repo = Resolve-WtRepoDir -Root $root -Name $Name -AllowMissing
    if ($repo) {
        Set-WtLocation -Path $repo.FullName
        Write-WtDetail "Ahora en el repo: $($repo.FullName)"
        return @{ RepoRoot = $repo.FullName; Source = 'repo' }
    }

    $owner = Find-WtRepoOwningWorktree -Root $root -Name $Name
    if ($owner) {
        Set-WtLocation -Path $owner.FullName
        Write-WtDetail "El worktree '$Name' es del repo '$($owner.Name)'; ahora en $($owner.FullName)"
        return @{ RepoRoot = $owner.FullName; Source = 'worktree' }
    }

    throw "No hay un repo ni worktree '$Name' bajo '$root'. Usa 'wt repos' para ver los disponibles."
}

function Get-WtRepoList {
    param([switch]$Json)
    $root = Get-WtReposRoot -Config (Get-WtConfig)
    $repos = @(Get-WtRepoDirs -Root $root)
    if ($Json) {
        ConvertTo-WtJson -InputObject @($repos | Select-Object Name, FullName)
        return
    }
    Write-WtDetail ("Raiz de repos: {0}" -f $root)
    if ($repos.Count -eq 0) {
        Write-WtNotice 'No hay repos git en la raiz.'
        return
    }
    Write-WtTable -Rows @($repos | ForEach-Object { [pscustomobject]@{ Repo = $_.Name; Ruta = $_.FullName } })
}

function Invoke-WtCd {
    <#
    .SYNOPSIS
        Cambia el directorio actual a la raiz de repos o a un repo.
    .NOTES
        Funciona porque la funcion 'wt' del perfil invoca el script en el mismo proceso.
        Con -Open abre el editor (la terminal nunca se abre sola).
    #>
    param([AllowEmptyString()][string]$Name, [switch]$Open)
    $config = Get-WtConfig
    $root = Get-WtReposRoot -Config $config
    $target = $root
    if ($Name) { $target = (Resolve-WtRepoDir -Root $root -Name $Name).FullName }
    Set-WtLocation -Path $target
    Write-WtSuccess "Ahora en: $target"
    if ($Open) { Open-WtEditor -Path $target -Config $config | Out-Null }
}

function Open-WtFolder {
    <#
    .SYNOPSIS
        Abre el editor en una carpeta arbitraria (la terminal nunca se abre sola).
    #>
    param([Parameter(Mandatory)][string]$Path, [Parameter(Mandatory)]$Config)
    return (Open-WtEditor -Path $Path -Config $Config)
}
