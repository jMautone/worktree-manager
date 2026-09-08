#requires -Version 5.1
<#
.SYNOPSIS
    Pruebas unitarias (in-process) de la logica pura de Worktree Manager.
.DESCRIPTION
    Complementan a Invoke-WtTests.ps1: no tocan git ni el disco, corren en el mismo
    proceso y cubren el parser de argumentos, el parsing del porcelain de git, la
    normalizacion y comparacion de rutas, la validacion de nombres, el merge de
    configuracion y la generacion del TOML de Warp.
.EXAMPLE
    powershell -File tests\Invoke-WtUnitTests.ps1
#>
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$script:Passed = 0
$script:Failed = 0

function Assert-True {
    param([string]$Name, [bool]$Condition, [string]$Detail = '')
    if ($Condition) {
        $script:Passed++
        Write-Host "  [PASS] $Name" -ForegroundColor Green
    } else {
        $script:Failed++
        Write-Host "  [FAIL] $Name $Detail" -ForegroundColor Red
    }
}

function Assert-Throws {
    param([string]$Name, [scriptblock]$Action, [string]$Pattern = '')
    try {
        & $Action | Out-Null
        Assert-True $Name $false 'no lanzo excepcion'
    } catch {
        $message = $_.Exception.Message
        Assert-True $Name (-not $Pattern -or $message -match $Pattern) $message
    }
}

Import-Module (Join-Path $PSScriptRoot '..\wt.psm1') -Force

Write-Host '== ConvertFrom-WtArgs: comandos y alias ==' -ForegroundColor Cyan
$p = ConvertFrom-WtArgs -Arguments @('ls')
Assert-True 'alias ls -> list' ($p.Command -eq 'list')
$p = ConvertFrom-WtArgs -Arguments @('rm', 'x')
Assert-True 'alias rm -> remove' ($p.Command -eq 'remove')
$p = ConvertFrom-WtArgs -Arguments @('go')
Assert-True 'alias go -> cd' ($p.Command -eq 'cd')
$p = ConvertFrom-WtArgs -Arguments @('--help')
Assert-True 'alias --help -> help' ($p.Command -eq 'help')
$p = ConvertFrom-WtArgs -Arguments @('--version')
Assert-True 'alias --version -> version' ($p.Command -eq 'version')
$p = ConvertFrom-WtArgs -Arguments @()
Assert-True 'sin argumentos no hay comando' ($p.Command -eq '')
Assert-Throws 'comando desconocido falla' { ConvertFrom-WtArgs -Arguments @('inventado') } 'Comando desconocido'

Write-Host '== ConvertFrom-WtArgs: flags y valores ==' -ForegroundColor Cyan
$p = ConvertFrom-WtArgs -Arguments @('create', 'feat', '--base', 'origin/develop', '--no-open')
Assert-True 'positional capturado' (@($p.Positional).Count -eq 1 -and $p.Positional[0] -eq 'feat')
Assert-True 'valor de --base capturado' ($p.Values['base'] -eq 'origin/develop')
Assert-True 'flag --no-open capturado' ($p.Flags.ContainsKey('no-open'))
$p = ConvertFrom-WtArgs -Arguments @('open', 'x', '--all')
Assert-True 'flag --all capturado' ($p.Flags.ContainsKey('all'))
Assert-Throws '--base sin valor falla' { ConvertFrom-WtArgs -Arguments @('create', 'x', '--base') } 'requiere un valor'
Assert-Throws '--base seguido de otro flag falla' { ConvertFrom-WtArgs -Arguments @('create', 'x', '--base', '--no-open') } 'requiere un valor'
Assert-Throws 'flag desconocido falla' { ConvertFrom-WtArgs -Arguments @('open', 'x', '--agente') } 'Flag desconocido'
Assert-Throws 'flag de otro comando falla' { ConvertFrom-WtArgs -Arguments @('list', '--force') } 'Flag desconocido'
$p = ConvertFrom-WtArgs -Arguments @('config', 'set', 'worktreeRootTemplate', 'C:\Mis', 'Repos\{name}')
Assert-True 'config set conserva todos los positionales' (@($p.Positional).Count -eq 4)
$p = ConvertFrom-WtArgs -Arguments @('config', 'set', 'editor', '')
Assert-True 'config set acepta valor vacio' (@($p.Positional).Count -eq 3 -and $p.Positional[2] -eq '')

Write-Host '== ConvertFrom-WtWorktreePorcelain ==' -ForegroundColor Cyan
$porcelain = @'
worktree C:/repos/MiRepo
HEAD abc1234567890
branch refs/heads/main

worktree C:/repos/MiRepo.worktrees/feature-a
HEAD def1234567890
branch refs/heads/feature-a

worktree C:/repos/MiRepo.worktrees/stale
HEAD 999
detached
prunable gitdir file points to non-existent location
'@
$wts = @(ConvertFrom-WtWorktreePorcelain -Text $porcelain)
Assert-True 'parsea 3 worktrees' ($wts.Count -eq 3) ("count={0}" -f $wts.Count)
Assert-True 'normaliza las barras a Windows' ($wts[0].Path -eq 'C:\repos\MiRepo') $wts[0].Path
Assert-True 'el primero es el principal' ($wts[0].IsMain)
Assert-True 'los demas no son principales' (-not $wts[1].IsMain -and -not $wts[2].IsMain)
Assert-True 'lee la rama' ($wts[1].Branch -eq 'feature-a')
Assert-True 'detecta detached' ($wts[2].IsDetached)
Assert-True 'detecta prunable' ($wts[2].IsPrunable)
Assert-True 'guarda el motivo del prunable' ($wts[2].PruneReason -match 'non-existent') $wts[2].PruneReason
Assert-True 'texto vacio no rompe' ((@(ConvertFrom-WtWorktreePorcelain -Text '')).Count -eq 0)

Write-Host '== Rutas: normalizacion y comparacion ==' -ForegroundColor Cyan
Assert-True 'convierte / en \' ((ConvertTo-WtFullPath 'C:/a/b') -eq 'C:\a\b')
Assert-True 'quita la barra final' ((ConvertTo-WtFullPath 'C:\a\b\') -eq 'C:\a\b')
Assert-True 'colapsa ..' ((ConvertTo-WtFullPath 'C:\a\b\..\c') -eq 'C:\a\c')
Assert-True 'ruta vacia devuelve vacio' ((ConvertTo-WtFullPath '') -eq '')
Assert-True 'igualdad ignora mayusculas' (Test-WtPathEquals 'C:\A\B' 'c:\a\b')
Assert-True 'igualdad ignora barra final' (Test-WtPathEquals 'C:\a\b\' 'C:\a\b')
Assert-True 'un path es "under" de si mismo' (Test-WtPathIsUnder -Path 'C:\a\b' -Root 'C:\a\b')
Assert-True 'subdirectorio real es under' (Test-WtPathIsUnder -Path 'C:\a\b\c' -Root 'C:\a\b')
Assert-True 'hermano con prefijo comun NO es under' (-not (Test-WtPathIsUnder -Path 'C:\a\feature-a-backup' -Root 'C:\a\feature-a'))
Assert-True 'padre no es under del hijo' (-not (Test-WtPathIsUnder -Path 'C:\a' -Root 'C:\a\b'))

Write-Host '== Validacion de nombres ==' -ForegroundColor Cyan
Assert-True 'nombre valido' ((Test-WtWorktreeName -Name 'feature-a') -eq '')
Assert-True 'rechaza espacios' ((Test-WtWorktreeName -Name 'con espacio') -ne '')
Assert-True 'rechaza barras' ((Test-WtWorktreeName -Name 'a/b') -ne '')
Assert-True 'rechaza vacio' ((Test-WtWorktreeName -Name '') -ne '')
Assert-True 'rechaza ..' ((Test-WtWorktreeName -Name '..') -ne '')
Assert-True 'rechaza nombres reservados' ((Test-WtWorktreeName -Name 'CON') -ne '')
Assert-True 'rechaza nombres reservados con extension' ((Test-WtWorktreeName -Name 'nul.txt') -ne '')
Assert-True 'rechaza final en punto' ((Test-WtWorktreeName -Name 'feature.') -ne '')
Assert-True 'saneo de nombre de archivo' ((ConvertTo-WtSafeFileName 'a b/c:d') -eq 'a-b-c-d')

Write-Host '== Merge de configuracion ==' -ForegroundColor Cyan
$base = [ordered]@{ editor = 'code'; terminal = 'warp'; reposRoot = '' }
$merged = Merge-WtConfig -Base $base -Override ([ordered]@{ editor = ''; reposRoot = 'C:\Repos' })
Assert-True 'override pisa por clave' ($merged.editor -eq '' -and $merged.reposRoot -eq 'C:\Repos')
Assert-True 'las claves no pisadas se conservan' ($merged.terminal -eq 'warp')
Assert-True 'override nulo no rompe' ((Merge-WtConfig -Base $base -Override $null).editor -eq 'code')
Assert-True 'no muta la base' ($base.editor -eq 'code')
$defaults = Get-WtDefaultConfig
Assert-True 'los defaults traen las 12 claves documentadas' (@($defaults.Keys).Count -eq 12) (@($defaults.Keys) -join ',')

Write-Host '== Select-WtRepoConfigKeys: lista blanca del .wt.json del repo ==' -ForegroundColor Cyan
$selected = Select-WtRepoConfigKeys -Data ([ordered]@{ editor = 'mal.exe'; defaultBase = 'develop' })
Assert-True 'conserva defaultBase' ($selected.Config.Contains('defaultBase') -and $selected.Config['defaultBase'] -eq 'develop')
Assert-True 'rechaza editor' (-not $selected.Config.Contains('editor'))
Assert-True 'reporta editor como rechazada' (@($selected.Rejected) -contains 'editor')
$selected = Select-WtRepoConfigKeys -Data ([ordered]@{ worktreeRootTemplate = 't'; branchPrefix = 'p/'; fetchBeforeCreate = $true })
Assert-True 'las 4 claves admitidas pasan' (@($selected.Rejected).Count -eq 0 -and $selected.Config.Count -eq 3)
$selected = Select-WtRepoConfigKeys -Data $null
Assert-True 'datos nulos no rompen' ($selected.Config.Count -eq 0 -and @($selected.Rejected).Count -eq 0)
Assert-True 'las 4 claves admitidas son las documentadas' ((Get-WtRepoConfigAllowedKeys) -join ',' -eq 'worktreeRootTemplate,defaultBase,branchPrefix,fetchBeforeCreate')

Write-Host '== WT_CONFIG_ONLY aisla la config global del usuario ==' -ForegroundColor Cyan
$onlyConfigPath = Join-Path $env:TEMP ("wt-only-config-" + [guid]::NewGuid().ToString('N') + '.json')
try {
    ([pscustomobject]@{ branchPrefix = 'agent/' } | ConvertTo-Json) | Set-Content -Path $onlyConfigPath
    $prevConfig = $env:WT_CONFIG
    $prevConfigOnly = $env:WT_CONFIG_ONLY
    $env:WT_CONFIG = $onlyConfigPath
    $env:WT_CONFIG_ONLY = '1'
    Clear-WtConfigCache
    $isolated = Get-WtConfig -Refresh
    $defaultsForCompare = Get-WtDefaultConfig
    Assert-True 'WT_CONFIG_ONLY aplica el override' ($isolated.branchPrefix -eq 'agent/')
    $restoUnchanged = $true
    foreach ($k in $defaultsForCompare.Keys) {
        if ($k -eq 'branchPrefix') { continue }
        if ($isolated[$k] -ne $defaultsForCompare[$k]) { $restoUnchanged = $false }
    }
    Assert-True 'el resto de las claves queda igual a los defaults' $restoUnchanged
} finally {
    if ($null -eq $prevConfig) { Remove-Item Env:\WT_CONFIG -ErrorAction SilentlyContinue } else { $env:WT_CONFIG = $prevConfig }
    if ($null -eq $prevConfigOnly) { Remove-Item Env:\WT_CONFIG_ONLY -ErrorAction SilentlyContinue } else { $env:WT_CONFIG_ONLY = $prevConfigOnly }
    Clear-WtConfigCache
    Remove-Item -LiteralPath $onlyConfigPath -ErrorAction SilentlyContinue
}

Write-Host '== Plan de apertura (flags de open) ==' -ForegroundColor Cyan
$plan = Get-WtOpenPlan
Assert-True 'sin flags: solo editor' ($plan.Code -and -not $plan.Terminal -and -not $plan.Agent)
$plan = Get-WtOpenPlan -All
Assert-True '--all: editor + agente, sin terminal' ($plan.Code -and $plan.Agent -and -not $plan.Terminal)
$plan = Get-WtOpenPlan -Agent
Assert-True '--agent: solo agente' (-not $plan.Code -and $plan.Agent)
$plan = Get-WtOpenPlan -Terminal
Assert-True '--terminal: solo terminal' (-not $plan.Code -and $plan.Terminal -and -not $plan.Agent)
$plan = Get-WtOpenPlan -Code -Terminal
Assert-True 'flags combinables' ($plan.Code -and $plan.Terminal)
$plan = Get-WtOpenPlan -NoCode
Assert-True '--no-code (legacy) desactiva el editor' (-not $plan.Code)

Write-Host '== Plan de apertura de create (openOnCreate + flags) ==' -ForegroundColor Cyan
$cfgNone = [pscustomobject]@{ openOnCreate = 'none' }
$cfgAll = [pscustomobject]@{ openOnCreate = 'all' }
$cfgEditor = [pscustomobject]@{ openOnCreate = 'editor' }
$plan = Get-WtCreateOpenPlan -Config $cfgNone
Assert-True "openOnCreate 'none' sin flags: nada" (-not $plan.Code -and -not $plan.Terminal -and -not $plan.Agent)
$plan = Get-WtCreateOpenPlan -Config $cfgAll
Assert-True "openOnCreate 'all' sin flags: editor + agente" ($plan.Code -and $plan.Agent -and -not $plan.Terminal)
$plan = Get-WtCreateOpenPlan -Config $cfgEditor -Agent
Assert-True "openOnCreate 'editor' + --agent: el flag explicito gana (solo agente)" (-not $plan.Code -and $plan.Agent)
$plan = Get-WtCreateOpenPlan -Config $cfgAll -NoOpen
Assert-True '--no-open gana a cualquier config (all)' (-not $plan.Code -and -not $plan.Terminal -and -not $plan.Agent)
$plan = Get-WtCreateOpenPlan -Config $cfgNone -NoOpen
Assert-True '--no-open gana a cualquier config (none)' (-not $plan.Code -and -not $plan.Terminal -and -not $plan.Agent)
$plan = Get-WtCreateOpenPlan -Config $cfgNone -All
Assert-True '--all gana a openOnCreate none (editor + agente)' ($plan.Code -and $plan.Agent)

Write-Host '== TOML de Warp ==' -ForegroundColor Cyan
$toml = New-WtWarpTabConfigContent -Name 'demo' -Path 'C:\repo\wt demo' -Commands @('copilot') -Title 'MiRepo > demo' -Color 'green'
Assert-True 'directory como literal string' ($toml.Contains("directory = 'C:\repo\wt demo'")) $toml
Assert-True 'commands como literal string' ($toml.Contains("commands = ['copilot']")) $toml
Assert-True 'title escapado como basic string' ($toml.Contains('title = "MiRepo > demo"')) $toml
Assert-True 'color presente' ($toml.Contains('color = "green"')) $toml
$toml = New-WtWarpTabConfigContent -Name 'demo' -Path 'C:\repo\demo' -Title 'MiRepo > d"q" \ x'
Assert-True 'escapa comillas y barras en el titulo' ($toml.Contains('title = "MiRepo > d\"q\" \\ x"')) $toml
Assert-True 'sin comandos no emite commands' (-not $toml.Contains('commands =')) $toml
Assert-True 'sin color no emite color' (-not $toml.Contains('color =')) $toml
Assert-Throws 'ruta con comilla simple falla con guia' { New-WtWarpTabConfigContent -Name 'x' -Path "C:\it's" -Title 't' } 'comilla simple'

Write-Host '== Filas de wt list ==' -ForegroundColor Cyan
$rows = @(Get-WtWorktreeRows -Worktrees $wts)
Assert-True 'marca el principal' ($rows[0].Nombre -match 'principal') $rows[0].Nombre
Assert-True 'marca el obsoleto' ($rows[2].Nombre -match 'obsoleto') $rows[2].Nombre
Assert-True 'muestra el detached acortado' ($rows[2].Rama -match 'detached') $rows[2].Rama

Write-Host '== ConvertTo-WtJson ==' -ForegroundColor Cyan
Assert-True 'coleccion vacia -> []' ((ConvertTo-WtJson -InputObject @()) -eq '[]')
$json = ConvertTo-WtJson -InputObject @([pscustomobject]@{ A = 1 })
$parsedJson = $json | ConvertFrom-Json
Assert-True 'un solo elemento sigue siendo array' ((@($parsedJson)).Count -eq 1 -and $json.TrimStart().StartsWith('[')) $json
$json = ConvertTo-WtJson -InputObject @([pscustomobject]@{ A = 1 }, [pscustomobject]@{ A = 2 })
$parsedJson = $json | ConvertFrom-Json
Assert-True 'varios elementos siguen siendo array' ((@($parsedJson)).Count -eq 2) $json

Write-Host ''
$color = 'Green'
if ($script:Failed -gt 0) { $color = 'Red' }
Write-Host ("Resultado: {0} OK, {1} fallidas" -f $script:Passed, $script:Failed) -ForegroundColor $color
if ($script:Failed -gt 0) { exit 1 }
exit 0
