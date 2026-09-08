# ---------------------------------------------------------------------------
# Workspace: raiz de repos, listado, 'cd' y resolucion del repo de trabajo.
# Depende de: Common, Config, Repo.
# ---------------------------------------------------------------------------

function Get-WtReposRoot {
    <#
    .SYNOPSIS
        Raices de repos configuradas, validadas.
    .OUTPUTS
        Array de rutas (siempre; retrocompatible con la forma anterior de 'reposRoot'
        como string unico, que ConvertTo-WtReposRootList envuelve en un array de 1).
    #>
    param([Parameter(Mandatory)]$Config)
    $roots = @(ConvertTo-WtReposRootList -Value $Config.reposRoot)
    if ($roots.Count -eq 0) { throw 'reposRoot no esta configurado. Usa: wt config set reposRoot C:\Repos' }
    foreach ($root in $roots) {
        if (-not (Test-WtPathExists $root)) { throw "reposRoot '$root' no existe." }
    }
    return $roots
}

function Get-WtReposDepth {
    <#
    .SYNOPSIS
        Profundidad configurada para buscar repos bajo reposRoot (1-3, default 1).
    .DESCRIPTION
        Defensivo: un valor invalido (que Test-WtConfigValue ya deberia haber
        rechazado al guardarse) cae al default en vez de romper la busqueda.
    #>
    param([Parameter(Mandatory)]$Config)
    $parsed = 0
    if ([int]::TryParse([string]$Config.reposDepth, [ref]$parsed) -and $parsed -ge 1 -and $parsed -le 3) {
        return $parsed
    }
    return 1
}

function Get-WtRepoDirs {
    <#
    .SYNOPSIS
        Repos git (con .git, carpeta o archivo) bajo una o mas raices, hasta $Depth
        niveles de profundidad.
    .DESCRIPTION
        Corta la rama al encontrar un .git: un repo no contiene repos, asi que no baja
        a revisar sus propios submodulos o directorios internos en busca de mas.
    #>
    param([Parameter(Mandatory)][string[]]$Root, [int]$Depth = 1)
    $found = @()
    foreach ($r in $Root) {
        $found += Get-WtRepoDirsUnder -Dir $r -RemainingDepth $Depth
    }
    return @($found | Sort-Object Name)
}

function Get-WtRepoDirsUnder {
    param([Parameter(Mandatory)][string]$Dir, [Parameter(Mandatory)][int]$RemainingDepth)
    $result = @()
    foreach ($child in @(Get-ChildItem -LiteralPath $Dir -Directory -ErrorAction SilentlyContinue)) {
        if (Test-Path -LiteralPath (Join-Path $child.FullName '.git')) {
            $result += $child
        } elseif ($RemainingDepth -gt 1) {
            $result += Get-WtRepoDirsUnder -Dir $child.FullName -RemainingDepth ($RemainingDepth - 1)
        }
    }
    return $result
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
        Resuelve un repo por nombre entre una o mas raices. Con -AllowMissing devuelve
        $null en vez de fallar cuando no hay coincidencias (la ambiguedad siempre
        falla, sea cual sea la raiz de la que vengan las coincidencias).
    #>
    param(
        [Parameter(Mandatory)][string[]]$Root,
        [Parameter(Mandatory)][string]$Name,
        [int]$Depth = 1,
        [switch]$AllowMissing
    )
    $match = @(Select-WtRepoMatches -Repos (Get-WtRepoDirs -Root $Root -Depth $Depth) -Name $Name)
    if ($match.Count -gt 1) {
        $list = ($match | ForEach-Object { "$($_.Name) ($($_.FullName))" }) -join ', '
        throw "El nombre '$Name' es ambiguo; coincide con: $list"
    }
    if ($match.Count -eq 0) {
        if ($AllowMissing) { return $null }
        throw "No hay un repo '$Name' en '$($Root -join ', ')'. Usa 'wt repos' para ver los disponibles."
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
    param([Parameter(Mandatory)][string[]]$Root, [Parameter(Mandatory)][string]$Name, [int]$Depth = 1)
    $found = @()
    foreach ($dir in (Get-WtRepoDirs -Root $Root -Depth $Depth)) {
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

function Resolve-WtRepoOrWorktreeOwner {
    <#
    .SYNOPSIS
        Resuelve $Name contra una o mas raices: primero como repo, despues como
        worktree de alguno de ellos.
    .DESCRIPTION
        Funcion pura, compartida por Resolve-WtRepoContext ('open'/'path') e
        Invoke-WtCd (B12: 'wt cd' tambien puede llevarte a un worktree, no solo a un
        repo). Unico punto que aplica el criterio de ambiguedad de M4: 0 coincidencias
        falla con guia, 1 sigue, mas de 1 falla listando repo y ruta de cada una.
    .OUTPUTS
        @{ Repo; Worktree } - Worktree es $null cuando $Name matcheo un repo
        directamente; si no, es el worktree encontrado (y Repo, el que lo contiene).
    #>
    param([Parameter(Mandatory)][string[]]$Root, [Parameter(Mandatory)][string]$Name, [int]$Depth = 1)
    $repo = Resolve-WtRepoDir -Root $Root -Name $Name -Depth $Depth -AllowMissing
    if ($repo) { return @{ Repo = $repo; Worktree = $null } }

    $owners = @(Find-WtReposOwningWorktree -Root $Root -Name $Name -Depth $Depth)
    if ($owners.Count -eq 1) { return @{ Repo = $owners[0].Repo; Worktree = $owners[0].Worktree } }
    if ($owners.Count -gt 1) {
        $list = ($owners | ForEach-Object { "$($_.Repo.Name) ($($_.Repo.FullName))" }) -join ', '
        throw "El worktree '$Name' existe en varios repos: $list. Entra al repo, o usa 'wt open <repo>' primero."
    }
    throw "No hay un repo ni worktree '$Name' bajo '$($Root -join ', ')'. Usa 'wt repos' para ver los disponibles."
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

    $config = Get-WtConfig
    $root = @(Get-WtReposRoot -Config $config)
    $depth = Get-WtReposDepth -Config $config

    $found = Resolve-WtRepoOrWorktreeOwner -Root $root -Name $Name -Depth $depth
    $source = 'repo'
    if ($found.Worktree) { $source = 'worktree' }
    return @{ RepoRoot = $found.Repo.FullName; Source = $source; ShouldRelocate = $true }
}

function Get-WtRepoList {
    <#
    .SYNOPSIS
        Lista los repos de reposRoot (una o mas raices). Con mas de una raiz, la tabla
        (y el --json) suman una columna 'Raiz' para decir de cual sale cada repo.
    #>
    param([switch]$Json)
    $config = Get-WtConfig
    $root = @(Get-WtReposRoot -Config $config)
    $depth = Get-WtReposDepth -Config $config
    $rows = @(foreach ($r in $root) {
        foreach ($repo in (Get-WtRepoDirs -Root @($r) -Depth $depth)) {
            [pscustomobject]@{ Repo = $repo.Name; Ruta = $repo.FullName; Raiz = $r }
        }
    })
    $rows = @($rows | Sort-Object Repo)
    if ($Json) {
        ConvertTo-WtJson -InputObject @($rows | Select-Object @{N = 'Name'; E = { $_.Repo } }, @{N = 'FullName'; E = { $_.Ruta } }, Raiz)
        return
    }
    Write-WtDetail ("Raiz de repos: {0}" -f ($root -join ', '))
    if ($rows.Count -eq 0) {
        Write-WtNotice 'No hay repos git en la raiz.'
        return
    }
    if ($root.Count -gt 1) {
        Write-WtTable -Rows $rows
    } else {
        Write-WtTable -Rows @($rows | Select-Object Repo, Ruta)
    }
}

function Invoke-WtCd {
    <#
    .SYNOPSIS
        Cambia el directorio actual a la raiz de repos (la primera, si hay varias), a
        un repo, o a un worktree de cualquiera de ellos (B12).
    .DESCRIPTION
        Con nombre, resuelve igual que 'open'/'path' fuera de un repo (primero como
        repo, despues como worktree de alguno de ellos; ambiguedad = M4): si matchea un
        worktree, el destino es SU checkout, no la raiz del repo que lo contiene.
    .NOTES
        Funciona porque la funcion 'wt' del perfil llama a Invoke-Wt en el mismo
        proceso (el modulo se importa una vez al cargar el perfil, no en cada llamada).
        Con -Open abre el editor (la terminal nunca se abre sola).
    #>
    param([AllowEmptyString()][string]$Name, [switch]$Open)
    $config = Get-WtConfig
    $root = @(Get-WtReposRoot -Config $config)
    $depth = Get-WtReposDepth -Config $config
    $target = $root[0]
    if ($Name) {
        $found = Resolve-WtRepoOrWorktreeOwner -Root $root -Name $Name -Depth $depth
        $target = $found.Repo.FullName
        if ($found.Worktree) { $target = $found.Worktree.Path }
    }
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
