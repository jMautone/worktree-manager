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

Write-Host "== ConvertFrom-WtArgs: '--' suelto (wt exec/each, item 6) ==" -ForegroundColor Cyan
$p = ConvertFrom-WtArgs -Arguments @('exec', 'feature-a', '--', 'git', 'log', '--oneline', '-1')
Assert-True "positional antes de '--'" (@($p.Positional).Count -eq 1 -and $p.Positional[0] -eq 'feature-a')
Assert-True "'--' captura todo crudo, incluidos flags del comando externo" `
    (($p.Rest -join ' ') -eq 'git log --oneline -1') ($p.Rest -join '|')
Assert-True "el comando externo no se interpreta como flag de wt" (-not $p.Flags.ContainsKey('oneline'))
$p = ConvertFrom-WtArgs -Arguments @('each', '--continue-on-error', '--', 'cmd', '/c', 'echo', 'hola')
Assert-True "flags de wt antes de '--' se parsean normal" ($p.Flags.ContainsKey('continue-on-error'))
Assert-True "Rest despues de '--'" (($p.Rest -join ' ') -eq 'cmd /c echo hola')
$p = ConvertFrom-WtArgs -Arguments @('each', '--')
Assert-True "'--' sin nada despues deja Rest vacio" (@($p.Rest).Count -eq 0)
$p = ConvertFrom-WtArgs -Arguments @('list')
Assert-True 'sin --, Rest queda vacio' (@($p.Rest).Count -eq 0)

Write-Host '== ConvertFrom-WtWorktreePorcelain ==' -ForegroundColor Cyan
$porcelain = @'
worktree C:/repos/MiRepo
HEAD abc1234567890
branch refs/heads/main

worktree C:/repos/MiRepo.worktrees/feature-a
HEAD def1234567890
branch refs/heads/feature-a
locked reviewing sensitive changes

worktree C:/repos/MiRepo.worktrees/stale
HEAD 999
detached
prunable gitdir file points to non-existent location

worktree C:/repos/MiRepo.worktrees/locked-sin-motivo
HEAD 888
branch refs/heads/locked-sin-motivo
locked
'@
$wts = @(ConvertFrom-WtWorktreePorcelain -Text $porcelain)
Assert-True 'parsea 4 worktrees' ($wts.Count -eq 4) ("count={0}" -f $wts.Count)
Assert-True 'normaliza las barras a Windows' ($wts[0].Path -eq 'C:\repos\MiRepo') $wts[0].Path
Assert-True 'el primero es el principal' ($wts[0].IsMain)
Assert-True 'los demas no son principales' (-not $wts[1].IsMain -and -not $wts[2].IsMain)
Assert-True 'lee la rama' ($wts[1].Branch -eq 'feature-a')
Assert-True 'detecta detached' ($wts[2].IsDetached)
Assert-True 'detecta prunable' ($wts[2].IsPrunable)
Assert-True 'guarda el motivo del prunable' ($wts[2].PruneReason -match 'non-existent') $wts[2].PruneReason
Assert-True 'detecta locked con motivo (B4)' ($wts[1].IsLocked -and $wts[1].LockReason -eq 'reviewing sensitive changes') $wts[1].LockReason
Assert-True 'detecta locked sin motivo (B4)' ($wts[3].IsLocked -and $wts[3].LockReason -eq '')
Assert-True 'un worktree no bloqueado no queda marcado' (-not $wts[2].IsLocked)
Assert-True 'texto vacio no rompe' ((@(ConvertFrom-WtWorktreePorcelain -Text '')).Count -eq 0)

Write-Host '== ConvertFrom-WtWorktreePorcelain con un warning intercalado (M1) ==' -ForegroundColor Cyan
$porcelainConWarning = @'
warning: unable to access '/etc/gitconfig': Permission denied
worktree C:/repos/MiRepo
HEAD abc1234567890
branch refs/heads/main

worktree C:/repos/MiRepo.worktrees/feature-a
HEAD def1234567890
branch refs/heads/feature-a
'@
$wtsConWarning = @(ConvertFrom-WtWorktreePorcelain -Text $porcelainConWarning)
Assert-True 'un warning intercalado no rompe el parseo' ($wtsConWarning.Count -eq 2) ("count={0}" -f $wtsConWarning.Count)
Assert-True 'el primer worktree valido sigue siendo el principal' ($wtsConWarning[0].Path -eq 'C:\repos\MiRepo' -and $wtsConWarning[0].IsMain) $wtsConWarning[0].Path

Write-Host '== Invoke-WtProcess separa stdout de stderr (M1) ==' -ForegroundColor Cyan
$mixed = Invoke-WtProcess -FilePath 'cmd.exe' -Arguments @('/c', 'echo ok & echo warn 1>&2')
Assert-True 'exit 0 pese al stderr' ($mixed.Success -and $mixed.ExitCode -eq 0) $mixed.ExitCode
Assert-True 'Text (stdout) no contiene warn' ($mixed.Text -notmatch 'warn') $mixed.Text
Assert-True 'Text (stdout) contiene ok' ($mixed.Text -match 'ok') $mixed.Text
Assert-True 'ErrorText (stderr) contiene warn' ($mixed.ErrorText -match 'warn') $mixed.ErrorText
Assert-True 'StdOut y StdErr son arrays separados' (@($mixed.StdOut) -notcontains 'warn' -and (@($mixed.StdErr) -join ' ') -match 'warn')

Write-Host '== Invoke-WtProcess -WorkingDirectory (hooks de creacion) ==' -ForegroundColor Cyan
$tmpDirA = Join-Path $env:TEMP ("wt-process-cwd-a-" + [guid]::NewGuid().ToString('N'))
$tmpDirB = Join-Path $env:TEMP ("wt-process-cwd-b-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmpDirA -Force | Out-Null
New-Item -ItemType Directory -Path $tmpDirB -Force | Out-Null
try {
    $prevLocation = (Get-Location).Path
    $inA = Invoke-WtProcess -FilePath 'cmd.exe' -Arguments @('/c', 'cd') -WorkingDirectory $tmpDirA
    Assert-True 'corre en el WorkingDirectory pedido' ($inA.Text.TrimEnd('\') -ieq $tmpDirA.TrimEnd('\')) $inA.Text
    Assert-True 'restaura la ubicacion previa al terminar' ((Get-Location).Path -ieq $prevLocation)
    $inB = Invoke-WtProcess -FilePath 'cmd.exe' -Arguments @('/c', 'cd') -WorkingDirectory $tmpDirB
    Assert-True 'cada llamada usa su propio WorkingDirectory' ($inB.Text.TrimEnd('\') -ieq $tmpDirB.TrimEnd('\')) $inB.Text
} finally {
    Remove-Item -LiteralPath $tmpDirA -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $tmpDirB -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host '== Get-WtCommandSource: no revienta con $null bajo StrictMode (B6) ==' -ForegroundColor Cyan
Assert-True 'comando nulo devuelve vacio' ((Get-WtCommandSource -Command $null) -eq '')
$fakeCommand = [pscustomobject]@{ Source = 'C:\algun\comando.exe' }
Assert-True 'comando real devuelve su Source' ((Get-WtCommandSource -Command $fakeCommand) -eq 'C:\algun\comando.exe')

Write-Host '== Test-WtWorktreeMatchesName: mayusculas por diseno (B10) ==' -ForegroundColor Cyan
$wtForCase = [pscustomobject]@{ Path = 'C:\repos\MiRepo.worktrees\Feature-A'; Branch = 'Feature-A' }
Assert-True 'carpeta: NO distingue mayusculas (Windows tampoco en el filesystem)' `
    (Test-WtWorktreeMatchesName -Worktree $wtForCase -Name 'feature-a')
Assert-True 'rama: SI distingue mayusculas (git tambien las distingue)' `
    (-not (Test-WtWorktreeMatchesName -Worktree $wtForCase -Name 'refs/heads/feature-a'))
Assert-True 'rama con mayusculas exactas si matchea' `
    (Test-WtWorktreeMatchesName -Worktree $wtForCase -Name 'refs/heads/Feature-A')

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

Write-Host '== Test-WtConfigValue: validacion por clave (M5) ==' -ForegroundColor Cyan
Assert-True 'worktreeRootTemplate valido' ((Test-WtConfigValue -Key 'worktreeRootTemplate' -Value '{repoParent}\{repo}.worktrees\{name}') -eq '')
Assert-True 'worktreeRootTemplate vacio invalido' ((Test-WtConfigValue -Key 'worktreeRootTemplate' -Value '') -ne '')
Assert-True 'worktreeRootTemplate sin {name} invalido' ((Test-WtConfigValue -Key 'worktreeRootTemplate' -Value 'C:\w\{repo}') -match '\{name\}')
Assert-True 'terminal valido (warp)' ((Test-WtConfigValue -Key 'terminal' -Value 'warp') -eq '')
Assert-True 'terminal invalido lista los admitidos' ((Test-WtConfigValue -Key 'terminal' -Value 'foo') -match 'warp, wt, none')
Assert-True 'warpAgentTarget valido (tab)' ((Test-WtConfigValue -Key 'warpAgentTarget' -Value 'tab') -eq '')
Assert-True 'warpAgentTarget invalido' ((Test-WtConfigValue -Key 'warpAgentTarget' -Value 'xyz') -match 'auto, tab, window')
Assert-True 'warpAgentColor vacio es valido' ((Test-WtConfigValue -Key 'warpAgentColor' -Value '') -eq '')
Assert-True 'warpAgentColor conocido es valido' ((Test-WtConfigValue -Key 'warpAgentColor' -Value 'cyan') -eq '')
Assert-True 'warpAgentColor desconocido invalido' ((Test-WtConfigValue -Key 'warpAgentColor' -Value 'rosa') -ne '')
Assert-True 'warpTerminalColor desconocido invalido' ((Test-WtConfigValue -Key 'warpTerminalColor' -Value 'rosa') -ne '')
Assert-True "openOnCreate valido ('editor')" ((Test-WtConfigValue -Key 'openOnCreate' -Value 'editor') -eq '')
Assert-True 'openOnCreate invalido' ((Test-WtConfigValue -Key 'openOnCreate' -Value 'todo') -match 'all, editor, none')
Assert-True 'fetchBeforeCreate booleano real es valido' ((Test-WtConfigValue -Key 'fetchBeforeCreate' -Value $true) -eq '')
Assert-True "fetchBeforeCreate 'true'/'false' string es valido" ((Test-WtConfigValue -Key 'fetchBeforeCreate' -Value 'false') -eq '')
Assert-True 'fetchBeforeCreate invalido' ((Test-WtConfigValue -Key 'fetchBeforeCreate' -Value 'si') -ne '')
Assert-True 'reposRoot vacio es valido' ((Test-WtConfigValue -Key 'reposRoot' -Value '') -eq '')
Assert-True 'reposRoot ruta absoluta es valido' ((Test-WtConfigValue -Key 'reposRoot' -Value 'C:\Repos') -eq '')
Assert-True 'reposRoot ruta relativa es invalido' ((Test-WtConfigValue -Key 'reposRoot' -Value 'Repos') -ne '')
Assert-True 'warpPath ruta relativa es invalido' ((Test-WtConfigValue -Key 'warpPath' -Value 'warp.exe') -ne '')
Assert-True 'editor es libre' ((Test-WtConfigValue -Key 'editor' -Value 'cualquier-cosa') -eq '')
Assert-True 'defaultBase es libre' ((Test-WtConfigValue -Key 'defaultBase' -Value 'origin/lo-que-sea') -eq '')
Assert-True 'branchPrefix es libre' ((Test-WtConfigValue -Key 'branchPrefix' -Value 'agent/') -eq '')
Assert-True 'clave desconocida es libre (la restriccion de claves vive en otro lado)' ((Test-WtConfigValue -Key 'noExiste' -Value 'x') -eq '')
Assert-Throws 'Assert-WtConfigValue lanza con el motivo' { Assert-WtConfigValue -Key 'terminal' -Value 'foo' } 'terminal'
Assert-True 'copyOnCreate array vacio es valido' ((Test-WtConfigValue -Key 'copyOnCreate' -Value @()) -eq '')
Assert-True 'copyOnCreate array de strings es valido' ((Test-WtConfigValue -Key 'copyOnCreate' -Value @('.env', 'certs/*.pfx')) -eq '')
Assert-True 'copyOnCreate con elemento vacio es invalido' ((Test-WtConfigValue -Key 'copyOnCreate' -Value @('.env', '')) -ne '')
Assert-True 'postCreate con elemento vacio es invalido' ((Test-WtConfigValue -Key 'postCreate' -Value @('')) -ne '')
Assert-True "ConvertTo-WtConfigValue separa copyOnCreate por ';'" `
    ((@(ConvertTo-WtConfigValue -Key 'copyOnCreate' -Value '.env;certs/*.pfx') -join ',') -eq '.env,certs/*.pfx')
Assert-True 'ConvertTo-WtConfigValue descarta vacios entre separadores' `
    ((@(ConvertTo-WtConfigValue -Key 'postCreate' -Value 'echo a;;echo b') -join ',') -eq 'echo a,echo b')
Assert-True "worktreeRootTemplate sin {repo}/{repoParent}: advierte, no rechaza" (Test-WtWorktreeRootTemplateNeedsRepoToken -Value 'C:\w\{name}')
Assert-True "worktreeRootTemplate con {repo}: no advierte" (-not (Test-WtWorktreeRootTemplateNeedsRepoToken -Value '{repoParent}\{repo}.worktrees\{name}'))

Write-Host '== Merge de configuracion ==' -ForegroundColor Cyan
$base = [ordered]@{ editor = 'code'; terminal = 'warp'; reposRoot = '' }
$merged = Merge-WtConfig -Base $base -Override ([ordered]@{ editor = ''; reposRoot = 'C:\Repos' })
Assert-True 'override pisa por clave' ($merged.editor -eq '' -and $merged.reposRoot -eq 'C:\Repos')
Assert-True 'las claves no pisadas se conservan' ($merged.terminal -eq 'warp')
Assert-True 'override nulo no rompe' ((Merge-WtConfig -Base $base -Override $null).editor -eq 'code')
Assert-True 'no muta la base' ($base.editor -eq 'code')
$defaults = Get-WtDefaultConfig
Assert-True 'los defaults traen las 17 claves documentadas' (@($defaults.Keys).Count -eq 17) (@($defaults.Keys) -join ',')

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

Write-Host '== Get-WtPathHash / Get-WtTabConfigFileName (M2) ==' -ForegroundColor Cyan
Assert-True 'misma ruta produce siempre el mismo hash' ((Get-WtPathHash 'C:\Repos\A.worktrees\feature-a') -eq (Get-WtPathHash 'C:\Repos\A.worktrees\feature-a'))
Assert-True 'hash ignora mayusculas y / vs \' ((Get-WtPathHash 'C:\Repos\A\feature-a') -eq (Get-WtPathHash 'c:/repos/a/feature-a'))
Assert-True 'rutas distintas producen hashes distintos' ((Get-WtPathHash 'C:\Repos\A.worktrees\feature-a') -ne (Get-WtPathHash 'C:\Repos\B.worktrees\feature-a'))
$fileA = Get-WtTabConfigFileName -Name 'feature-a' -Path 'C:\Repos\A.worktrees\feature-a' -Kind 'agent'
$fileB = Get-WtTabConfigFileName -Name 'feature-a' -Path 'C:\Repos\B.worktrees\feature-a' -Kind 'agent'
Assert-True 'dos repos con worktree homonimo producen archivos distintos' ($fileA -ne $fileB) ("{0} vs {1}" -f $fileA, $fileB)
$fileARepeat = Get-WtTabConfigFileName -Name 'feature-a' -Path 'C:\Repos\A.worktrees\feature-a' -Kind 'agent'
Assert-True 'la misma ruta produce siempre el mismo archivo' ($fileA -eq $fileARepeat)

Write-Host '== Get-WtTabConfigDirectory (M2) ==' -ForegroundColor Cyan
$tabContent = New-WtWarpTabConfigContent -Name 'demo' -Path 'C:\repo\wt-demo' -Title 'MiRepo > demo'
Assert-True 'extrae directory de un tab config generado' ((Get-WtTabConfigDirectory -Content $tabContent) -eq 'C:\repo\wt-demo')
Assert-True 'contenido sin directory devuelve vacio' ((Get-WtTabConfigDirectory -Content "name = ""x""") -eq '')

Write-Host '== Tab config escrito como UTF-8 sin BOM (B3) ==' -ForegroundColor Cyan
$bomTabDir = Join-Path $env:TEMP ("wt-bom-test-" + [guid]::NewGuid().ToString('N'))
$prevTabConfigEnv = $env:WT_WARP_TAB_CONFIG
try {
    $env:WT_WARP_TAB_CONFIG = $bomTabDir
    $bomTabFile = Write-WtAgentTabConfig -Name 'bomcheck' -Path 'C:\repo\bomcheck' -Commands @('copilot')
    $bytes = [IO.File]::ReadAllBytes($bomTabFile)
    $hasBom = ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF)
    Assert-True 'el tab config no tiene BOM (EF BB BF)' (-not $hasBom)
} finally {
    if ($null -eq $prevTabConfigEnv) { Remove-Item Env:\WT_WARP_TAB_CONFIG -ErrorAction SilentlyContinue } else { $env:WT_WARP_TAB_CONFIG = $prevTabConfigEnv }
    Remove-Item -Recurse -Force $bomTabDir -ErrorAction SilentlyContinue
}

Write-Host '== Find-WtRepoConfigFile: ubica .wt.json sin invocar git (B8) ==' -ForegroundColor Cyan
$b8Root = Join-Path $env:TEMP ("wt-b8-test-" + [guid]::NewGuid().ToString('N'))
try {
    $mainRepo = Join-Path $b8Root 'repo'
    $subDir = Join-Path $mainRepo 'sub\dir'
    New-Item -ItemType Directory -Path $mainRepo\.git -Force | Out-Null
    New-Item -ItemType Directory -Path $subDir -Force | Out-Null
    $found = Find-WtRepoConfigFile -StartPath $subDir
    Assert-True 'desde un subdirectorio del repo, encuentra la raiz principal' ($found -eq (Join-Path $mainRepo '.wt.json')) $found

    # Worktree: <mainRepo>\.git\worktrees\<nombre> existe, y el .git del worktree es
    # un archivo 'gitdir: <esa ruta>' -exactamente el formato que usa git.
    $worktreeGitDir = Join-Path $mainRepo '.git\worktrees\feature-a'
    New-Item -ItemType Directory -Path $worktreeGitDir -Force | Out-Null
    $worktreeDir = Join-Path $b8Root 'repo.worktrees\feature-a'
    New-Item -ItemType Directory -Path $worktreeDir -Force | Out-Null
    Set-Content -LiteralPath (Join-Path $worktreeDir '.git') -Value "gitdir: $worktreeGitDir"
    $foundFromWorktree = Find-WtRepoConfigFile -StartPath $worktreeDir
    Assert-True 'desde un worktree, encuentra el .wt.json de la raiz PRINCIPAL (no la del worktree)' `
        ($foundFromWorktree -eq (Join-Path $mainRepo '.wt.json')) $foundFromWorktree

    $outsideAnyRepo = Join-Path $b8Root 'sin-repo'
    New-Item -ItemType Directory -Path $outsideAnyRepo -Force | Out-Null
    Assert-True 'fuera de cualquier repo devuelve null' ($null -eq (Find-WtRepoConfigFile -StartPath $outsideAnyRepo))

    $bogusGitFile = Join-Path $b8Root 'bogus.git'
    Set-Content -LiteralPath $bogusGitFile -Value 'no es un puntero gitdir'
    Assert-True 'Read-WtGitDirPointer con formato invalido devuelve vacio' ((Read-WtGitDirPointer -GitFilePath $bogusGitFile) -eq '')
} finally {
    Remove-Item -Recurse -Force $b8Root -ErrorAction SilentlyContinue
}

Write-Host '== ConvertTo-WtReposRootList: string o array, retrocompatible (B11) ==' -ForegroundColor Cyan
$reposRootSingle = @(ConvertTo-WtReposRootList -Value 'C:\Repos')
Assert-True 'string unico se envuelve en un array de 1' ($reposRootSingle.Count -eq 1 -and $reposRootSingle[0] -eq 'C:\Repos')
Assert-True 'array se conserva tal cual' (@(ConvertTo-WtReposRootList -Value @('C:\A', 'C:\B')).Count -eq 2)
Assert-True 'vacio devuelve array vacio' (@(ConvertTo-WtReposRootList -Value '').Count -eq 0)
Assert-True 'null devuelve array vacio' (@(ConvertTo-WtReposRootList -Value $null).Count -eq 0)
Assert-True 'elementos vacios en el array se descartan' (@(ConvertTo-WtReposRootList -Value @('C:\A', '', 'C:\B')).Count -eq 2)

Write-Host '== Get-WtRepoDirs: profundidad configurable, corta al encontrar .git (B11) ==' -ForegroundColor Cyan
$b11Root = Join-Path $env:TEMP ("wt-b11-test-" + [guid]::NewGuid().ToString('N'))
try {
    # C:\Repos\repoDirecto (.git a 1 nivel) y C:\Repos\org\repoAnidado (.git a 2 niveles).
    New-Item -ItemType Directory -Path (Join-Path $b11Root 'repoDirecto\.git') -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $b11Root 'org\repoAnidado\.git') -Force | Out-Null
    # Un repo no contiene repos: esto NO deberia encontrarse ni con reposDepth alto.
    New-Item -ItemType Directory -Path (Join-Path $b11Root 'repoDirecto\subrepo-fantasma\.git') -Force | Out-Null

    $depth1 = @(Get-WtRepoDirs -Root @($b11Root) -Depth 1)
    Assert-True 'depth 1: encuentra el repo directo' (@($depth1 | Where-Object { $_.Name -eq 'repoDirecto' }).Count -eq 1)
    Assert-True 'depth 1: NO encuentra el anidado bajo org' (@($depth1 | Where-Object { $_.Name -eq 'repoAnidado' }).Count -eq 0)

    $depth2 = @(Get-WtRepoDirs -Root @($b11Root) -Depth 2)
    Assert-True 'depth 2: encuentra ambos (directo y anidado)' ($depth2.Count -eq 2)
    Assert-True 'depth 2: corta la rama en un repo (no baja a buscar dentro)' (@($depth2 | Where-Object { $_.Name -eq 'subrepo-fantasma' }).Count -eq 0)

    $secondRoot = Join-Path $b11Root 'segunda-raiz'
    New-Item -ItemType Directory -Path (Join-Path $secondRoot 'otroRepo\.git') -Force | Out-Null
    $multiRoot = @(Get-WtRepoDirs -Root @($b11Root, $secondRoot) -Depth 1)
    Assert-True 'varias raices: agrega resultados de todas' (@($multiRoot | Where-Object { $_.Name -eq 'repoDirecto' }).Count -eq 1 -and @($multiRoot | Where-Object { $_.Name -eq 'otroRepo' }).Count -eq 1)
} finally {
    Remove-Item -Recurse -Force $b11Root -ErrorAction SilentlyContinue
}

Write-Host '== Test-WtConfigValue: reposRoot (array) y reposDepth (B11) ==' -ForegroundColor Cyan
Assert-True 'reposRoot string absoluto es valido' ((Test-WtConfigValue -Key 'reposRoot' -Value 'C:\Repos') -eq '')
Assert-True 'reposRoot array de absolutos es valido' ((Test-WtConfigValue -Key 'reposRoot' -Value @('C:\A', 'D:\B')) -eq '')
Assert-True 'reposRoot con un elemento relativo es invalido' ((Test-WtConfigValue -Key 'reposRoot' -Value @('C:\A', 'relativo')) -ne '')
Assert-True 'reposDepth 1 es valido' ((Test-WtConfigValue -Key 'reposDepth' -Value 1) -eq '')
Assert-True 'reposDepth 3 es valido' ((Test-WtConfigValue -Key 'reposDepth' -Value '3') -eq '')
Assert-True 'reposDepth 0 es invalido' ((Test-WtConfigValue -Key 'reposDepth' -Value 0) -ne '')
Assert-True 'reposDepth 4 es invalido' ((Test-WtConfigValue -Key 'reposDepth' -Value 4) -ne '')
Assert-True 'reposDepth no numerico es invalido' ((Test-WtConfigValue -Key 'reposDepth' -Value 'dos') -ne '')

Write-Host '== Get-WtAgentCommands: agentShell y agentCommand (M8) ==' -ForegroundColor Cyan
$cfgNoneAgent = [pscustomobject]@{ agentShell = 'none'; agentCommand = 'copilot' }
$cmdsNone = @(Get-WtAgentCommands -Config $cfgNoneAgent)
Assert-True "agentShell 'none': unicamente agentCommand (sin fnm)" ($cmdsNone.Count -eq 1 -and $cmdsNone[0] -eq 'copilot') ($cmdsNone -join '|')
$cfgPs = [pscustomobject]@{ agentShell = 'powershell'; agentCommand = 'copilot' }
$cmdsPs = @(Get-WtAgentCommands -Config $cfgPs)
Assert-True "agentShell 'powershell' no inyecta sintaxis de PowerShell" (($cmdsPs -join ' ') -notmatch 'Write-Warning|-notmatch|[{}]') ($cmdsPs -join '|')
Assert-True "agentShell 'powershell' incluye agentCommand" ($cmdsPs -contains 'copilot')
$cfgBash = [pscustomobject]@{ agentShell = 'bash'; agentCommand = 'copilot' }
$cmdsBash = @(Get-WtAgentCommands -Config $cfgBash)
Assert-True "agentShell 'bash' no contiene sintaxis de PowerShell" (($cmdsBash -join ' ') -notmatch 'Write-Warning|-notmatch|[{}]') ($cmdsBash -join '|')
Assert-True "agentShell 'bash' incluye agentCommand" ($cmdsBash -contains 'copilot')
$cfgCustom = [pscustomobject]@{ agentShell = 'none'; agentCommand = 'claude' }
$cmdsCustom = @(Get-WtAgentCommands -Config $cfgCustom)
Assert-True 'agentCommand personalizado se respeta' ($cmdsCustom.Count -eq 1 -and $cmdsCustom[0] -eq 'claude')

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
Assert-True 'marca el bloqueado con motivo (B4)' ($rows[1].Nombre -match 'bloqueado.*reviewing sensitive changes') $rows[1].Nombre
Assert-True 'marca el bloqueado sin motivo (B4)' ($rows[3].Nombre -match 'bloqueado' -and $rows[3].Nombre -notmatch ':') $rows[3].Nombre

Write-Host '== ConvertTo-WtJson ==' -ForegroundColor Cyan
Assert-True 'coleccion vacia -> []' ((ConvertTo-WtJson -InputObject @()) -eq '[]')
$json = ConvertTo-WtJson -InputObject @([pscustomobject]@{ A = 1 })
$parsedJson = $json | ConvertFrom-Json
Assert-True 'un solo elemento sigue siendo array' ((@($parsedJson)).Count -eq 1 -and $json.TrimStart().StartsWith('[')) $json
$json = ConvertTo-WtJson -InputObject @([pscustomobject]@{ A = 1 }, [pscustomobject]@{ A = 2 })
$parsedJson = $json | ConvertFrom-Json
Assert-True 'varios elementos siguen siendo array' ((@($parsedJson)).Count -eq 2) $json

Write-Host '== Resolve-WtCopyOnCreatePlan (hooks de creacion) ==' -ForegroundColor Cyan
$repoFiles = @('.env', 'certs\dev.pfx', 'certs\prod.pfx', 'src\app.js', 'README.md')
$plan = @(Resolve-WtCopyOnCreatePlan -Patterns @('.env') -RepoFiles $repoFiles)
Assert-True 'patron exacto matchea 1 archivo' ($plan[0].Valid -and @($plan[0].Items).Count -eq 1 -and $plan[0].Items[0].From -eq '.env')
$plan = @(Resolve-WtCopyOnCreatePlan -Patterns @('certs/*.pfx') -RepoFiles $repoFiles)
Assert-True 'patron con subdirectorio matchea varios' ($plan[0].Valid -and @($plan[0].Items).Count -eq 2) (@($plan[0].Items) | ForEach-Object { $_.From })
$plan = @(Resolve-WtCopyOnCreatePlan -Patterns @('no-existe.*') -RepoFiles $repoFiles)
Assert-True 'patron sin matches queda valido con Items vacio' ($plan[0].Valid -and @($plan[0].Items).Count -eq 0)
$plan = @(Resolve-WtCopyOnCreatePlan -Patterns @('C:\fuera\del\repo.txt') -RepoFiles $repoFiles)
Assert-True 'patron absoluto se rechaza' (-not $plan[0].Valid) $plan[0].Reason
$plan = @(Resolve-WtCopyOnCreatePlan -Patterns @('..\fuera.txt') -RepoFiles $repoFiles)
Assert-True 'patron con .. se rechaza' (-not $plan[0].Valid) $plan[0].Reason
$plan = @(Resolve-WtCopyOnCreatePlan -Patterns @('src/../secret.txt') -RepoFiles $repoFiles)
Assert-True 'patron con .. en el medio tambien se rechaza' (-not $plan[0].Valid) $plan[0].Reason
$plan = @(Resolve-WtCopyOnCreatePlan -Patterns @() -RepoFiles $repoFiles)
Assert-True 'sin patrones no hay resultados' ((@($plan)).Count -eq 0)

Write-Host '== Get-WtCompletion (autocompletado) ==' -ForegroundColor Cyan
Assert-True "'wt op' -> open" ((@(Get-WtCompletion -Words @() -WordToComplete 'op')) -contains 'open')
Assert-True "'wt op' no sugiere otros comandos" ((@(Get-WtCompletion -Words @() -WordToComplete 'op')).Count -eq 1)
Assert-True "'wt ' sin filtro trae varios comandos" ((@(Get-WtCompletion -Words @() -WordToComplete '')).Count -gt 5)
Assert-True "'wt open --a' -> --agent y --all" (
    (Compare-Object (@(Get-WtCompletion -Words @('open') -WordToComplete '--a')) (@('--agent', '--all')) | Measure-Object).Count -eq 0)
Assert-True "'wt config set ter' -> terminal" ((@(Get-WtCompletion -Words @('config', 'set') -WordToComplete 'ter')) -contains 'terminal')
Assert-True "'wt config set terminal ' -> warp/wt/none" (
    (Compare-Object (@(Get-WtCompletion -Words @('config', 'set', 'terminal') -WordToComplete '')) (@('none', 'warp', 'wt')) | Measure-Object).Count -eq 0)
Assert-True "'wt config set editor ' (clave libre) no sugiere nada" ((@(Get-WtCompletion -Words @('config', 'set', 'editor') -WordToComplete '')).Count -eq 0)
Assert-True 'comando desconocido -> lista vacia' ((@(Get-WtCompletion -Words @('inventado') -WordToComplete '')).Count -eq 0)
Assert-True "'wt open ' con worktrees en contexto los sugiere" (
    (@(Get-WtCompletion -Words @('open') -WordToComplete '' -Worktrees @('feature-a', 'feature-b'))) -contains 'feature-a')
Assert-True "'wt open ' sin worktrees cae a repos" (
    (@(Get-WtCompletion -Words @('open') -WordToComplete '' -Repos @('MiRepo'))) -contains 'MiRepo')
Assert-True "'wt list ' no tiene 2da posicion de nombre" ((@(Get-WtCompletion -Words @('list') -WordToComplete '' -Worktrees @('feature-a'))).Count -eq 0)
Assert-True "alias 'rm' resuelve igual que 'remove' para --flags" (
    (@(Get-WtCompletion -Words @('rm') -WordToComplete '--del')) -contains '--delete-branch')

Write-Host '== Get-WtWindowsTerminalArgs (agente en Windows Terminal, item 3) ==' -ForegroundColor Cyan
$wtCfgPowershell = [pscustomobject]@{ agentCommand = 'copilot'; agentShell = 'powershell' }
$wtArgs = @(Get-WtWindowsTerminalArgs -Path 'C:\repo wt\feature-a' -Title 'MiRepo > feature-a' -Config $wtCfgPowershell)
Assert-True 'new-tab primero' ($wtArgs[0] -eq 'new-tab')
Assert-True 'ruta con espacios entrecomillada' ($wtArgs -contains '"C:\repo wt\feature-a"') ($wtArgs -join ' ')
Assert-True 'titulo presente' ($wtArgs -contains '--title') ($wtArgs -join ' ')
Assert-True 'agentShell powershell envuelve en powershell -NoExit' ($wtArgs -contains 'powershell' -and $wtArgs -contains '-NoExit') ($wtArgs -join ' ')
Assert-True 'agentCommand custom presente' (($wtArgs -join ' ') -match 'copilot') ($wtArgs -join ' ')

$wtCfgNone = [pscustomobject]@{ agentCommand = 'mi-agente-custom'; agentShell = 'none' }
$wtArgsNone = @(Get-WtWindowsTerminalArgs -Path 'C:\repo\feature-a' -Title '' -Config $wtCfgNone)
Assert-True "agentShell 'none': agentCommand directo, sin shell wrapper" ($wtArgsNone[-1] -eq 'mi-agente-custom') ($wtArgsNone -join ' ')
Assert-True "agentShell 'none': no arma --title sin titulo" (-not ($wtArgsNone -contains '--title'))

$wtCfgBash = [pscustomobject]@{ agentCommand = 'copilot'; agentShell = 'bash' }
$wtArgsBash = @(Get-WtWindowsTerminalArgs -Path 'C:\repo\feature-a' -Title 't' -Config $wtCfgBash)
Assert-True "agentShell 'bash' usa bash -lc" ($wtArgsBash -contains 'bash' -and $wtArgsBash -contains '-lc') ($wtArgsBash -join ' ')

Write-Host '== --dry-run global (item 4) ==' -ForegroundColor Cyan
$p = ConvertFrom-WtArgs -Arguments @('create', 'x', '--dry-run')
Assert-True "--dry-run se parsea como flag" ($p.Flags.ContainsKey('dry-run'))
$p = ConvertFrom-WtArgs -Arguments @('list', '--dry-run')
Assert-True "--dry-run es valido en un comando que no lo declara en su spec ('list')" ($p.Flags.ContainsKey('dry-run'))
Assert-True 'Test-WtDryRun arranca apagado' (-not (Test-WtDryRun))
try {
    Set-WtDryRun -Value $true
    Assert-True 'Set-WtDryRun lo enciende' (Test-WtDryRun)
    $dryResult = Invoke-WtProcess -FilePath 'cmd.exe' -Arguments @('/c', 'echo', 'no-deberia-correr')
    Assert-True 'Invoke-WtProcess bajo dry-run no corre nada (Text vacio)' ($dryResult.Text -eq '') $dryResult.Text
    Assert-True 'Invoke-WtProcess bajo dry-run devuelve exito sintetico' ($dryResult.Success -and $dryResult.ExitCode -eq 0)
    $readResult = Invoke-WtProcess -FilePath 'cmd.exe' -Arguments @('/c', 'echo', 'si-corre') -ReadOnly
    Assert-True '-ReadOnly sigue corriendo de verdad bajo dry-run' ($readResult.Text -match 'si-corre') $readResult.Text
} finally {
    Set-WtDryRun -Value $false
}
Assert-True 'Test-WtDryRun vuelve a apagarse' (-not (Test-WtDryRun))

Write-Host '== ConvertFrom-WtStatusPorcelainV2 (wt status, item 5) ==' -ForegroundColor Cyan
$statusLimpio = @'
# branch.oid abc1234
# branch.head main
# branch.upstream origin/main
# branch.ab +0 -0
'@
$m = ConvertFrom-WtStatusPorcelainV2 -Text $statusLimpio
Assert-True 'limpio: rama' ($m.Branch -eq 'main')
Assert-True 'limpio: upstream' ($m.Upstream -eq 'origin/main')
Assert-True 'limpio: sin cambios' (@($m.Staged).Count -eq 0 -and @($m.Modified).Count -eq 0 -and @($m.Untracked).Count -eq 0)

$statusConCambios = @'
# branch.oid abc1234
# branch.head feature-a
# branch.upstream origin/feature-a
# branch.ab +2 -3
1 M. N... 100644 100644 100644 aaa bbb src/a.txt
1 .M N... 100644 100644 100644 ccc ddd src/b.txt
1 MM N... 100644 100644 100644 eee fff src/c.txt
? nuevo.txt
? carpeta/otro nuevo.txt
'@
$m = ConvertFrom-WtStatusPorcelainV2 -Text $statusConCambios
Assert-True 'ahead/behind del branch.ab' ($m.Ahead -eq 2 -and $m.Behind -eq 3)
Assert-True 'staged: X != .' ((@($m.Staged) -join ',') -eq 'src/a.txt,src/c.txt') (@($m.Staged) -join ',')
Assert-True 'modified: Y != .' ((@($m.Modified) -join ',') -eq 'src/b.txt,src/c.txt') (@($m.Modified) -join ',')
Assert-True 'untracked con espacio en la ruta' (@($m.Untracked) -contains 'carpeta/otro nuevo.txt') (@($m.Untracked) -join ',')
Assert-True 'untracked count' (@($m.Untracked).Count -eq 2)

$statusSinUpstream = @'
# branch.oid abc1234
# branch.head sin-upstream
'@
$m = ConvertFrom-WtStatusPorcelainV2 -Text $statusSinUpstream
Assert-True 'sin upstream: Upstream vacio' ($m.Upstream -eq '')
Assert-True 'sin upstream: ahead/behind en 0' ($m.Ahead -eq 0 -and $m.Behind -eq 0)

$statusDetached = @'
# branch.oid abc1234
# branch.head (detached)
'@
$m = ConvertFrom-WtStatusPorcelainV2 -Text $statusDetached
Assert-True 'detached: Branch vacio (no "(detached)" literal)' ($m.Branch -eq '')

$statusVacio = ConvertFrom-WtStatusPorcelainV2 -Text ''
Assert-True 'texto vacio no rompe' ($statusVacio.Branch -eq '' -and @($statusVacio.Staged).Count -eq 0)

Write-Host '== Get-WtEachPlan (wt each, item 6) ==' -ForegroundColor Cyan
$eachWorktrees = @(
    [pscustomobject]@{ Path = 'C:\r\main'; IsMain = $true; IsPrunable = $false }
    [pscustomobject]@{ Path = 'C:\r.worktrees\a'; IsMain = $false; IsPrunable = $false }
    [pscustomobject]@{ Path = 'C:\r.worktrees\stale'; IsMain = $false; IsPrunable = $true }
)
$plan = @(Get-WtEachPlan -Worktrees $eachWorktrees)
Assert-True 'excluye el principal por defecto' (-not ($plan | Where-Object { $_.IsMain }))
Assert-True 'excluye los obsoletos siempre' (-not ($plan | Where-Object { $_.IsPrunable }))
Assert-True 'sin -IncludeMain: solo 1 worktree' ($plan.Count -eq 1 -and $plan[0].Path -eq 'C:\r.worktrees\a')
$planConMain = @(Get-WtEachPlan -Worktrees $eachWorktrees -IncludeMain)
Assert-True '-IncludeMain suma el principal (pero no los obsoletos)' ($planConMain.Count -eq 2)

Write-Host '== Get-WtSyncPlan (wt sync, item 7) ==' -ForegroundColor Cyan
function New-WtSyncEntry {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '',
        Justification = 'Constructor puro de un objeto sintetico para pruebas; el verbo New no implica efectos.')]
    param($Name, $Upstream = '', $Staged = 0, $Modified = 0, $Untracked = 0)
    return [pscustomobject]@{ Name = $Name; Upstream = $Upstream; Staged = $Staged; Modified = $Modified; Untracked = $Untracked }
}
$planLimpioConUpstream = @(Get-WtSyncPlan -Entries @(New-WtSyncEntry -Name 'a' -Upstream 'origin/a') -DefaultBase 'develop')
Assert-True 'limpio con upstream: sync contra el upstream' ($planLimpioConUpstream[0].Action -eq 'sync' -and $planLimpioConUpstream[0].Base -eq 'origin/a')
Assert-True 'default strategy es rebase (GitArgs)' (($planLimpioConUpstream[0].GitArgs -join ' ') -eq 'rebase origin/a')

$planSucio = @(Get-WtSyncPlan -Entries @(New-WtSyncEntry -Name 'b' -Upstream 'origin/b' -Modified 1) -DefaultBase 'develop')
Assert-True 'sucio (modified): skip-dirty, nunca stash' ($planSucio[0].Action -eq 'skip-dirty')
$planSucioStaged = @(Get-WtSyncPlan -Entries @(New-WtSyncEntry -Name 'c' -Staged 1) -DefaultBase 'develop')
Assert-True 'sucio (staged): skip-dirty' ($planSucioStaged[0].Action -eq 'skip-dirty')
$planSucioUntracked = @(Get-WtSyncPlan -Entries @(New-WtSyncEntry -Name 'd' -Untracked 1) -DefaultBase 'develop')
Assert-True 'sucio (untracked): skip-dirty' ($planSucioUntracked[0].Action -eq 'skip-dirty')

$planSinBase = @(Get-WtSyncPlan -Entries @(New-WtSyncEntry -Name 'e') -DefaultBase '')
Assert-True 'sin upstream ni defaultBase: skip-no-base' ($planSinBase[0].Action -eq 'skip-no-base')

$planDefaultBase = @(Get-WtSyncPlan -Entries @(New-WtSyncEntry -Name 'f') -DefaultBase 'develop')
Assert-True 'sin upstream, con defaultBase: sync contra defaultBase' ($planDefaultBase[0].Action -eq 'sync' -and $planDefaultBase[0].Base -eq 'develop')

$planExplicito = @(Get-WtSyncPlan -Entries @(New-WtSyncEntry -Name 'g' -Upstream 'origin/g') -ExplicitBase 'origin/main' -DefaultBase 'develop')
Assert-True '--base explicito gana sobre upstream y defaultBase' ($planExplicito[0].Base -eq 'origin/main')

$planMerge = @(Get-WtSyncPlan -Entries @(New-WtSyncEntry -Name 'h' -Upstream 'origin/h') -Strategy 'merge' -DefaultBase 'develop')
Assert-True "strategy 'merge' arma 'git merge <base>'" (($planMerge[0].GitArgs -join ' ') -eq 'merge origin/h')

Write-Host ''
$color = 'Green'
if ($script:Failed -gt 0) { $color = 'Red' }
Write-Host ("Resultado: {0} OK, {1} fallidas" -f $script:Passed, $script:Failed) -ForegroundColor $color
if ($script:Failed -gt 0) { exit 1 }
exit 0
