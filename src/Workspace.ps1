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

function Find-WtReposOwningWorktree {
    <#
    .SYNOPSIS
        Todos los repos de la raiz que tengan un worktree con ese nombre (carpeta o
        rama), con el worktree encontrado en cada uno.
    .DESCRIPTION
        Antes devolvia solo el primero por orden alfabetico, asi que dos worktrees
        homonimos en repos distintos daban un "exito" silencioso sobre el repo
        equivocado. Devolver todas las coincidencias deja que el llamador decida:
        una es exito, mas de una es una ambiguedad real que hay que explicar (igual
        que Resolve-WtRepoDir para el mismo problema con nombres de repo).
    .OUTPUTS
        Array de @{ Repo; Worktree }.
    #>
    param([Parameter(Mandatory)][string]$Root, [Parameter(Mandatory)][string]$Name)
    $found = @()
    foreach ($dir in (Get-WtRepoDirs -Root $Root)) {
        $worktrees = @()
        try { $worktrees = @(Get-WtWorktrees -RepoRoot $dir.FullName) } catch { continue }
        foreach ($wt in $worktrees) {
            if (Test-WtWorktreeMatchesName -Worktree $wt -Name $Name) {
                $found += @{ Repo = $dir; Worktree = $wt }
            }
        }
    }
    return $found
}

function Resolve-WtRepoContext {
    <#
    .SYNOPSIS
        Repo sobre el que operar: el actual si estamos dentro de uno; si no, se busca
        el nombre en reposRoot (primero como repo, despues como worktree de algun repo).
    .DESCRIPTION
        Funcion pura: resuelve pero NO cambia el directorio actual ni imprime nada.
        Un verbo 'Resolve-' con efectos secundarios rompe la convencion del modulo
        (decision separada de efecto) y sorprende a cualquier llamador que solo quiera
        saber el repo sin moverse (ej. 'wt path' fuera de un repo). ShouldRelocate le
        dice al llamador si tendria sentido hacer el cd (encontrado via reposRoot) o no
        (ya estabamos en un repo).
    .OUTPUTS
        @{ RepoRoot; Source = 'current' | 'repo' | 'worktree'; ShouldRelocate }
    #>
    param([AllowEmptyString()][string]$Name)
    $repoRoot = Find-WtMainRoot -Silent
    if ($repoRoot) { return @{ RepoRoot = $repoRoot; Source = 'current'; ShouldRelocate = $false } }
    # Sin nombre no hay nada que resolver: se relanza sin -Silent para dar el error
    # detallado (worktree huerfano o "no estas en un repo").
    if (-not $Name) { return @{ RepoRoot = (Find-WtMainRoot); Source = 'current'; ShouldRelocate = $false } }

    $root = Get-WtReposRoot -Config (Get-WtConfig)

    $repo = Resolve-WtRepoDir -Root $root -Name $Name -AllowMissing
    if ($repo) {
        return @{ RepoRoot = $repo.FullName; Source = 'repo'; ShouldRelocate = $true }
    }

    $owners = @(Find-WtReposOwningWorktree -Root $root -Name $Name)
    if ($owners.Count -eq 1) {
        return @{ RepoRoot = $owners[0].Repo.FullName; Source = 'worktree'; ShouldRelocate = $true }
    }
    if ($owners.Count -gt 1) {
        $list = ($owners | ForEach-Object { "$($_.Repo.Name) ($($_.Repo.FullName))" }) -join ', '
        throw "El worktree '$Name' existe en varios repos: $list. Entra al repo, o usa 'wt open <repo>' primero."
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
        Funciona porque la funcion 'wt' del perfil llama a Invoke-Wt en el mismo
        proceso (el modulo se importa una vez al cargar el perfil, no en cada llamada).
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
