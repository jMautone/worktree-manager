#requires -Version 5.1
<#
.SYNOPSIS
    Pruebas end-to-end de Worktree Manager contra un repositorio git temporal.
.EXAMPLE
    powershell -File tests\Invoke-WtTests.ps1
#>
[CmdletBinding()]
[Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingEmptyCatchBlock', '',
    Justification = 'Parseos best-effort: si fallan, la asercion siguiente ya lo detecta como "no parseable".')]
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

function Invoke-Wt {
    param([string[]]$CmdArgs, [string]$Cwd)
    $module = (Join-Path $PSScriptRoot '..\wt.psm1') -replace "'", "''"
    $cwdEscaped = $Cwd -replace "'", "''"
    $argLiterals = ($CmdArgs | ForEach-Object { "'{0}'" -f ($_ -replace "'", "''") }) -join ', '
    $cmd = "Set-Location -LiteralPath '$cwdEscaped'; Import-Module '$module' -Force; Invoke-Wt -Arguments @($argLiterals)"
    $prevEAP = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $out = & powershell.exe -NoProfile -ExecutionPolicy Bypass -Command $cmd 2>&1
        $exit = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $prevEAP
    }
    return @{ Output = ($out | Out-String); ExitCode = $exit }
}

function Invoke-WtConsole {
    # Ejecuta 'wt console' con las lineas de entrada pipeadas por stdin
    param([string[]]$InputLines, [string]$Cwd)
    $module = (Join-Path $PSScriptRoot '..\wt.psm1') -replace "'", "''"
    $cwdEscaped = $Cwd -replace "'", "''"
    $cmd = "Set-Location -LiteralPath '$cwdEscaped'; Import-Module '$module' -Force; Invoke-Wt -Arguments @('console')"
    $stdin = ($InputLines -join "`n") + "`n"
    $prevEAP = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $out = $stdin | & powershell.exe -NoProfile -ExecutionPolicy Bypass -Command $cmd 2>&1
        $exit = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $prevEAP
    }
    return @{ Output = ($out | Out-String); ExitCode = $exit }
}

# --- Entorno temporal -------------------------------------------------------
$tempRoot = Join-Path $env:TEMP ("wt-tests-" + [guid]::NewGuid().ToString('N').Substring(0, 8))
$repoDir = Join-Path $tempRoot 'MiRepo'
$configPath = Join-Path $tempRoot 'config.json'
New-Item -ItemType Directory -Path $repoDir -Force | Out-Null

try {
    Push-Location $repoDir

    git init -b main --quiet
    git config user.email 'wt-tests@local'
    git config user.name 'wt-tests'
    git config commit.gpgsign false
    Set-Content -Path (Join-Path $repoDir 'README.md') -Value 'test'
    git add README.md
    git commit -m 'init' --quiet
    git branch develop

    # Config aislada: sin editor/terminal para no abrir apps durante las pruebas.
    # Las 11 claves con valores explicitos y neutros: WT_CONFIG_ONLY hace que esta
    # sea la unica fuente (ademas de los defaults), sin importar la config real de
    # la maquina que corre las pruebas.
    $testConfig = [ordered]@{
        worktreeRootTemplate = '{repoParent}\{repo}.worktrees\{name}'
        reposRoot            = $tempRoot
        defaultBase          = ''
        branchPrefix         = ''
        editor               = ''
        terminal             = 'none'
        warpAgentTarget      = 'auto'
        warpAgentColor       = ''
        warpTerminalColor    = ''
        warpPath             = 'C:\no-existe\warp.exe'
        fetchBeforeCreate    = $false
    }
    ([pscustomobject]$testConfig | ConvertTo-Json) | Set-Content -Path $configPath
    $env:WT_CONFIG = $configPath
    $env:WT_CONFIG_ONLY = '1'

    # Repos extra en la raiz para probar 'wt repos' y 'wt cd' (.git basta para detectarlos)
    New-Item -ItemType Directory -Path (Join-Path $tempRoot 'OtroRepo\.git') -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $tempRoot 'OtroRepoDos\.git') -Force | Out-Null

    $wtRoot = Join-Path $tempRoot 'MiRepo.worktrees'

    Write-Host '== wt create (desde rama por defecto) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('create', 'feature-a', '--no-open') -Cwd $repoDir
    Assert-True 'create feature-a exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'directorio creado' (Test-Path (Join-Path $wtRoot 'feature-a'))
    git -C $repoDir show-ref --verify --quiet refs/heads/feature-a
    Assert-True 'rama feature-a creada' ($LASTEXITCODE -eq 0)

    Write-Host '== wt create --base develop ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('create', 'feature-b', '--base', 'develop', '--no-open') -Cwd $repoDir
    Assert-True 'create feature-b exit 0' ($r.ExitCode -eq 0) $r.Output
    $base = (git -C $repoDir rev-parse 'feature-b^{}').Trim()
    $dev  = (git -C $repoDir rev-parse develop).Trim()
    Assert-True 'feature-b apunta a develop' ($base -eq $dev)

    Write-Host '== wt create --base explicito con rama local existente (debe fallar, no reusar en silencio) ==' -ForegroundColor Cyan
    git -C $repoDir branch existente-local
    $r = Invoke-Wt -CmdArgs @('create', 'existente-local', '--base', 'develop', '--no-open') -Cwd $repoDir
    Assert-True 'create con --base y rama local existente falla' ($r.ExitCode -ne 0)
    Assert-True 'mensaje explica el conflicto' ($r.Output -match 'ya existe localmente') $r.Output
    Assert-True 'no crea el directorio del worktree' (-not (Test-Path (Join-Path $wtRoot 'existente-local')))

    Write-Host '== wt create duplicado (debe fallar) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('create', 'feature-a', '--no-open') -Cwd $repoDir
    Assert-True 'create duplicado falla' ($r.ExitCode -ne 0)
    Assert-True 'mensaje de error claro' ($r.Output -match 'Ya existe un directorio') $r.Output

    Write-Host '== wt list ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('list') -Cwd $repoDir
    Assert-True 'list exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'list muestra feature-a' ($r.Output -match 'feature-a')
    Assert-True 'list muestra feature-b' ($r.Output -match 'feature-b')
    Assert-True 'list marca el principal' ($r.Output -match 'principal')

    Write-Host '== wt list --json ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('list', '--json') -Cwd $repoDir
    $parsed = $null
    try { $parsed = ($r.Output | ConvertFrom-Json) } catch {}
    Assert-True 'json parseable' ($null -ne $parsed) $r.Output
    Assert-True 'json tiene 3 worktrees' (@($parsed).Count -eq 3)

    Write-Host '== wt path ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('path', 'feature-a') -Cwd $repoDir
    Assert-True 'path exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'path correcto' ($r.Output.Trim() -eq (Join-Path $wtRoot 'feature-a')) $r.Output

    Write-Host '== wt open (sin editor/terminal imprime la ruta) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('open', 'feature-a') -Cwd $repoDir
    Assert-True 'open exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'open imprime la ruta' ($r.Output -match [regex]::Escape('MiRepo.worktrees\feature-a')) $r.Output
    Assert-True 'open sin errores internos' ($r.Output -notmatch 'Cannot convert|RuntimeException') $r.Output

    Write-Host '== resolucion por nombre de rama ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('path', 'feature-b') -Cwd $repoDir
    Assert-True 'resuelve por rama' ($r.ExitCode -eq 0 -and $r.Output -match 'feature-b')

    Write-Host '== wt remove ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('remove', 'feature-b') -Cwd $repoDir
    Assert-True 'remove exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'directorio eliminado' (-not (Test-Path (Join-Path $wtRoot 'feature-b')))
    git -C $repoDir show-ref --verify --quiet refs/heads/feature-b
    Assert-True 'rama feature-b conservada' ($LASTEXITCODE -eq 0)

    Write-Host '== wt remove --delete-branch ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('remove', 'feature-a', '--delete-branch') -Cwd $repoDir
    Assert-True 'remove --delete-branch exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'directorio feature-a eliminado' (-not (Test-Path (Join-Path $wtRoot 'feature-a')))
    git -C $repoDir show-ref --verify --quiet refs/heads/feature-a
    Assert-True 'rama feature-a eliminada' ($LASTEXITCODE -ne 0)

    Write-Host '== wt remove del principal (debe fallar) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('remove', 'MiRepo') -Cwd $repoDir
    Assert-True 'remove principal falla' ($r.ExitCode -ne 0)
    Assert-True 'mensaje adecuado' ($r.Output -match 'principal') $r.Output

    Write-Host '== wt desde dentro de un worktree ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('create', 'nested', '--no-open') -Cwd $repoDir
    Assert-True 'create nested exit 0' ($r.ExitCode -eq 0) $r.Output
    $nestedDir = Join-Path $wtRoot 'nested'
    $r2 = Invoke-Wt -CmdArgs @('list') -Cwd $nestedDir
    Assert-True 'list desde worktree exit 0' ($r2.ExitCode -eq 0) $r2.Output
    Assert-True 'list desde worktree ve MiRepo' ($r2.Output -match 'MiRepo')
    $r3 = Invoke-Wt -CmdArgs @('remove', 'nested', '--delete-branch') -Cwd $nestedDir
    Assert-True 'remove estando dentro del worktree' ($r3.ExitCode -eq 0) $r3.Output

    Write-Host '== worktree con cambios sin commit (debe fallar sin --force) ==' -ForegroundColor Cyan
    Invoke-Wt -CmdArgs @('create', 'dirty', '--no-open') -Cwd $repoDir | Out-Null
    Set-Content -Path (Join-Path $wtRoot 'dirty\cambio.txt') -Value 'pendiente'
    $r = Invoke-Wt -CmdArgs @('remove', 'dirty') -Cwd $repoDir
    Assert-True 'remove dirty falla sin force' ($r.ExitCode -ne 0)
    $r = Invoke-Wt -CmdArgs @('remove', 'dirty', '--force', '--delete-branch') -Cwd $repoDir
    Assert-True 'remove dirty --force funciona' ($r.ExitCode -eq 0) $r.Output

    Write-Host '== wt prune ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('prune') -Cwd $repoDir
    Assert-True 'prune exit 0' ($r.ExitCode -eq 0) $r.Output

    Write-Host '== worktree obsoleto (directorio borrado a mano) ==' -ForegroundColor Cyan
    Invoke-Wt -CmdArgs @('create', 'stale', '--no-open') -Cwd $repoDir | Out-Null
    Remove-Item -Recurse -Force (Join-Path $wtRoot 'stale')
    $r = Invoke-Wt -CmdArgs @('list') -Cwd $repoDir
    Assert-True 'list marca el obsoleto' ($r.Output -match 'stale \(obsoleto') $r.Output
    $r = Invoke-Wt -CmdArgs @('open', 'stale') -Cwd $repoDir
    Assert-True 'open obsoleto falla' ($r.ExitCode -ne 0)
    Assert-True 'open obsoleto sugiere prune' ($r.Output -match 'obsoleto' -and $r.Output -match 'wt prune') $r.Output
    $r = Invoke-Wt -CmdArgs @('path', 'stale') -Cwd $repoDir
    Assert-True 'path obsoleto falla con guia' ($r.ExitCode -ne 0 -and $r.Output -match 'wt prune') $r.Output
    $r = Invoke-Wt -CmdArgs @('remove', 'stale') -Cwd $repoDir
    Assert-True 'remove obsoleto sugiere prune' ($r.ExitCode -ne 0 -and $r.Output -match 'wt prune') $r.Output
    $r = Invoke-Wt -CmdArgs @('doctor') -Cwd $repoDir
    Assert-True 'doctor avisa worktrees obsoletos' ($r.Output -match 'worktrees obsoletos') $r.Output
    $r = Invoke-Wt -CmdArgs @('prune') -Cwd $repoDir
    Assert-True 'prune limpia el obsoleto' ($r.ExitCode -eq 0) $r.Output
    $r = Invoke-Wt -CmdArgs @('open', 'stale') -Cwd $repoDir
    Assert-True 'tras prune ya no existe el worktree' ($r.ExitCode -ne 0 -and $r.Output -match 'No existe un worktree') $r.Output
    git -C $repoDir branch -D stale 2>&1 | Out-Null

    Write-Host '== wt open sin nombre desde un worktree (abre el worktree actual) ==' -ForegroundColor Cyan
    Invoke-Wt -CmdArgs @('create', 'actual', '--no-open') -Cwd $repoDir | Out-Null
    $actualDir = Join-Path $wtRoot 'actual'
    $r = Invoke-Wt -CmdArgs @('open') -Cwd $actualDir
    Assert-True 'open sin nombre en worktree exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'open sin nombre en worktree imprime su ruta' ($r.Output -match [regex]::Escape('MiRepo.worktrees\actual')) $r.Output
    $r = Invoke-Wt -CmdArgs @('path') -Cwd $actualDir
    Assert-True 'path sin nombre en worktree imprime su ruta' ($r.Output.Trim() -eq $actualDir) $r.Output
    Invoke-Wt -CmdArgs @('remove', 'actual', '--delete-branch') -Cwd $repoDir | Out-Null

    Write-Host '== worktree huerfano (repo principal movido) ==' -ForegroundColor Cyan
    Invoke-Wt -CmdArgs @('create', 'huerfano', '--no-open') -Cwd $repoDir | Out-Null
    $huerfanoDir = Join-Path $wtRoot 'huerfano'
    Set-Location $env:TEMP  # liberar el repo para poder renombrarlo
    Rename-Item -LiteralPath $repoDir -NewName 'MiRepo-movido'
    try {
        $r = Invoke-Wt -CmdArgs @('open', '--agent') -Cwd $huerfanoDir
        Assert-True 'open en huerfano falla' ($r.ExitCode -ne 0)
        Assert-True 'mensaje de worktree huerfano' ($r.Output -match 'huerfano' -and $r.Output -match 'worktree repair') $r.Output
        Assert-True 'no es el mensaje generico de repo' ($r.Output -notmatch 'No estas dentro de un repositorio git') $r.Output
    } finally {
        Rename-Item -LiteralPath "$repoDir-movido" -NewName (Split-Path -Leaf $repoDir)
        Set-Location $repoDir
    }
    Invoke-Wt -CmdArgs @('remove', 'huerfano', '--force', '--delete-branch') -Cwd $repoDir | Out-Null

    Write-Host '== comando desconocido (debe fallar) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('inventado') -Cwd $repoDir
    Assert-True 'comando desconocido falla' ($r.ExitCode -ne 0)

    Write-Host '== wt fuera de un repo (debe fallar con mensaje claro) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('list') -Cwd $env:TEMP
    Assert-True 'fuera de repo falla' ($r.ExitCode -ne 0)
    Assert-True 'mensaje de repo' ($r.Output -match 'repositorio git') $r.Output

    # -Function limita el import para no pisar el helper Invoke-Wt de este script
    Import-Module (Join-Path $PSScriptRoot '..\wt.psm1') -Force -Function Write-WtAgentTabConfig, Test-WtWarpSameWindow

    Write-Host '== tab config de Warp para el agente ==' -ForegroundColor Cyan
    $tabDir = Join-Path $tempRoot 'tab_configs'
    $env:WT_WARP_TAB_CONFIG = $tabDir
    $tabFile = Write-WtAgentTabConfig -Name 'demo' -Path 'C:\repo\wt-demo' -Commands @('fnm use 22', 'copilot')
    Assert-True 'tab config creado' (Test-Path $tabFile)
    $tabToml = Get-Content -Raw -Path $tabFile
    Assert-True 'tab config apunta al worktree' ($tabToml.Contains("directory = 'C:\repo\wt-demo'")) $tabToml
    Assert-True 'tab config corre fnm use y copilot' ($tabToml.Contains("'fnm use 22'") -and $tabToml.Contains("'copilot'")) $tabToml
    Assert-True 'tab config sin color cuando se omite' (-not $tabToml.Contains('color =')) $tabToml
    $tabFile2 = Write-WtAgentTabConfig -Name 'demo2' -Path 'C:\repo\wt-demo2' -Commands @('copilot') -Title 'MiRepo > demo2 (feature/demo2)' -Color 'green'
    $tabToml2 = Get-Content -Raw -Path $tabFile2
    Assert-True 'tab config con titulo repo > worktree (rama)' ($tabToml2.Contains('title = "MiRepo > demo2 (feature/demo2)"')) $tabToml2
    Assert-True 'tab config con color de agente' ($tabToml2.Contains('color = "green"')) $tabToml2
    Remove-Item Env:\WT_WARP_TAB_CONFIG -ErrorAction SilentlyContinue

    Write-Host '== tab config de Warp para terminal comun (sin comandos) ==' -ForegroundColor Cyan
    $tabDir = Join-Path $tempRoot 'tab_configs'
    $env:WT_WARP_TAB_CONFIG = $tabDir
    $termFile = Write-WtAgentTabConfig -Name 'demo' -Path 'C:\repo\wt-demo' -Title 'MiRepo > demo' -Color 'blue' -Kind 'term'
    Assert-True 'tab config de terminal creado' (Test-Path $termFile)
    Assert-True 'tab config de terminal usa prefijo wt-term' ((Split-Path -Leaf $termFile) -eq 'wt-term-demo.toml') $termFile
    $termToml = Get-Content -Raw -Path $termFile
    Assert-True 'tab config de terminal apunta al directorio' ($termToml.Contains("directory = 'C:\repo\wt-demo'")) $termToml
    Assert-True 'tab config de terminal sin comandos' (-not $termToml.Contains('commands =')) $termToml
    Assert-True 'tab config de terminal con color' ($termToml.Contains('color = "blue"')) $termToml
    Remove-Item Env:\WT_WARP_TAB_CONFIG -ErrorAction SilentlyContinue

    Write-Host '== destino de los tabs en Warp (auto/tab/window) ==' -ForegroundColor Cyan
    $origTermProgram = $env:TERM_PROGRAM
    $env:WT_WARP_RUNNING = '0'
    $env:TERM_PROGRAM = 'WarpTerminal'
    Assert-True 'auto dentro de Warp = misma ventana' (Test-WtWarpSameWindow -Target 'auto')
    Remove-Item Env:\TERM_PROGRAM -ErrorAction SilentlyContinue
    Assert-True 'auto sin Warp activo = ventana nueva' (-not (Test-WtWarpSameWindow -Target 'auto'))
    $env:WT_WARP_RUNNING = '1'
    Assert-True 'auto con Warp activo = misma ventana' (Test-WtWarpSameWindow -Target 'auto')
    Remove-Item Env:\WT_WARP_RUNNING -ErrorAction SilentlyContinue
    Assert-True 'tab fuerza misma ventana' (Test-WtWarpSameWindow -Target 'tab')
    Assert-True 'window fuerza ventana nueva' (-not (Test-WtWarpSameWindow -Target 'window'))
    if ($null -ne $origTermProgram) { $env:TERM_PROGRAM = $origTermProgram }

    Write-Host '== wt repos ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('repos') -Cwd $env:TEMP
    Assert-True 'repos exit 0 (fuera de un repo)' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'repos lista MiRepo' ($r.Output -match 'MiRepo') $r.Output
    Assert-True 'repos lista OtroRepo' ($r.Output -match 'OtroRepo') $r.Output

    Write-Host '== wt repos --json ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('repos', '--json') -Cwd $env:TEMP
    $reposJson = $null
    try { $reposJson = ($r.Output | ConvertFrom-Json) } catch {}
    Assert-True 'repos json parseable' ($null -ne $reposJson) $r.Output
    Assert-True 'repos json tiene 3 repos' (@($reposJson).Count -eq 3) $r.Output

    Write-Host '== wt cd sin nombre (va a la raiz) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('cd') -Cwd $env:TEMP
    Assert-True 'cd raiz exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'cd raiz informa la ruta' ($r.Output -match "Ahora en: $([regex]::Escape($tempRoot))") $r.Output

    Write-Host '== wt cd <repo> ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('cd', 'mirepo') -Cwd $env:TEMP
    Assert-True 'cd repo exit 0 (case-insensitive)' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'cd repo informa la ruta' ($r.Output -match "Ahora en: $([regex]::Escape($repoDir))") $r.Output

    Write-Host '== wt cd con prefijo unico ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('cd', 'MiR') -Cwd $env:TEMP
    Assert-True 'cd por prefijo resuelve' ($r.ExitCode -eq 0 -and $r.Output -match [regex]::Escape($repoDir)) $r.Output

    Write-Host '== wt cd ambiguo (debe fallar) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('cd', 'Otro') -Cwd $env:TEMP
    Assert-True 'cd ambiguo falla' ($r.ExitCode -ne 0)
    Assert-True 'cd ambiguo lista las coincidencias' ($r.Output -match 'ambiguo' -and $r.Output -match 'OtroRepoDos') $r.Output

    Write-Host '== wt cd inexistente (debe fallar) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('cd', 'NoExiste') -Cwd $env:TEMP
    Assert-True 'cd inexistente falla' ($r.ExitCode -ne 0)
    Assert-True 'cd inexistente sugiere wt repos' ($r.Output -match 'wt repos') $r.Output

    Write-Host '== wt cd sin reposRoot configurado (debe fallar con guia) ==' -ForegroundColor Cyan
    $sinRootPath = Join-Path $tempRoot 'config-sin-root.json'
    # reposRoot vacio explicito para que no lo complete la config global del usuario
    ([pscustomobject]@{ reposRoot = ''; editor = ''; terminal = 'none' } | ConvertTo-Json) | Set-Content -Path $sinRootPath
    $env:WT_CONFIG = $sinRootPath
    $r = Invoke-Wt -CmdArgs @('cd', 'MiRepo') -Cwd $env:TEMP
    $env:WT_CONFIG = $configPath
    Assert-True 'cd sin reposRoot falla' ($r.ExitCode -ne 0)
    Assert-True 'cd sin reposRoot explica como configurarlo' ($r.Output -match 'wt config set reposRoot') $r.Output

    Write-Host '== wt config get / set ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('config', 'get', 'reposRoot') -Cwd $repoDir
    Assert-True 'config get exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'config get devuelve reposRoot' ($r.Output.Trim() -eq $tempRoot) $r.Output
    $r = Invoke-Wt -CmdArgs @('config', 'set', 'warpAgentColor', 'cyan') -Cwd $repoDir
    Assert-True 'config set exit 0' ($r.ExitCode -eq 0) $r.Output
    $r = Invoke-Wt -CmdArgs @('config', 'get', 'warpAgentColor') -Cwd $repoDir
    Assert-True 'config set persiste el valor' ($r.Output.Trim() -eq 'cyan') $r.Output
    $r = Invoke-Wt -CmdArgs @('config', 'set', 'fetchBeforeCreate', 'true') -Cwd $repoDir
    $r = Invoke-Wt -CmdArgs @('config', 'get', 'fetchBeforeCreate') -Cwd $repoDir
    Assert-True 'config set convierte booleanos' ($r.Output.Trim() -eq 'True') $r.Output
    # vuelve al valor original para no afectar el resto de las pruebas
    Invoke-Wt -CmdArgs @('config', 'set', 'fetchBeforeCreate', 'false') -Cwd $repoDir | Out-Null

    Write-Host '== wt config set con clave invalida (debe fallar) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('config', 'set', 'noExiste', 'x') -Cwd $repoDir
    Assert-True 'config set clave invalida falla' ($r.ExitCode -ne 0)
    Assert-True 'config set lista claves validas' ($r.Output -match 'Clave desconocida' -and $r.Output -match 'reposRoot') $r.Output

    Write-Host '== .wt.json del repo: lista blanca de claves (A1) ==' -ForegroundColor Cyan
    # Requiere que .wt.json del repo sea un candidato real: se desactiva WT_CONFIG_ONLY
    # solo para este caso (WT_CONFIG sigue seteado y sigue ganando por ser el ultimo).
    $repoWtJsonPath = Join-Path $repoDir '.wt.json'
    ([pscustomobject]@{ editor = 'mal.exe'; warpPath = 'C:\hostil\warp.exe'; defaultBase = 'develop' } | ConvertTo-Json) |
        Set-Content -Path $repoWtJsonPath
    Remove-Item Env:\WT_CONFIG_ONLY -ErrorAction SilentlyContinue
    try {
        $r = Invoke-Wt -CmdArgs @('config', 'list') -Cwd $repoDir
        Assert-True '.wt.json hostil no rompe config list' ($r.ExitCode -eq 0) $r.Output
        Assert-True '.wt.json hostil no cambia editor (WT_CONFIG sigue ganando)' ($r.Output -match "(?m)^editor\s*$") $r.Output
        Assert-True 'avisa que editor no se admite' ($r.Output -match "'editor'.*no se admite") $r.Output
        Assert-True 'avisa que warpPath no se admite' ($r.Output -match "'warpPath'.*no se admite") $r.Output
    } finally {
        $env:WT_CONFIG_ONLY = '1'
        Remove-Item -LiteralPath $repoWtJsonPath -ErrorAction SilentlyContinue
    }

    Write-Host '== wt version ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('version') -Cwd $env:TEMP
    Assert-True 'version exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'version imprime la version del manifiesto' ($r.Output -match 'wt 0\.1\.0') $r.Output

    Write-Host '== wt doctor ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('doctor') -Cwd $env:TEMP
    Assert-True 'doctor exit 0 (informativo)' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'doctor chequea git' ($r.Output -match 'git') $r.Output
    Assert-True 'doctor chequea reposRoot' ($r.Output -match 'reposRoot') $r.Output

    Write-Host '== wt open sin nombre (abre el repo actual) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('open') -Cwd $repoDir
    Assert-True 'open sin nombre exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'open sin nombre imprime la ruta del repo' ($r.Output -match [regex]::Escape($repoDir)) $r.Output

    Write-Host '== wt path sin nombre (ruta del repo actual) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('path') -Cwd $repoDir
    Assert-True 'path sin nombre exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'path sin nombre imprime la raiz' ($r.Output.Trim() -eq $repoDir) $r.Output

    Write-Host '== wt open <repo> desde fuera de un repo ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('open', 'MiRepo') -Cwd $env:TEMP
    Assert-True 'open fuera de repo exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'open fuera de repo entra al repo' ($r.Output -match 'Ahora en el repo') $r.Output
    Assert-True 'open fuera de repo imprime la ruta' ($r.Output -match [regex]::Escape($repoDir)) $r.Output

    Write-Host '== wt open <worktree> desde fuera de un repo (busca en reposRoot) ==' -ForegroundColor Cyan
    Invoke-Wt -CmdArgs @('create', 'buscable', '--no-open') -Cwd $repoDir | Out-Null
    $r = Invoke-Wt -CmdArgs @('open', 'buscable') -Cwd $env:TEMP
    Assert-True 'open worktree fuera de repo exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'open worktree identifica el repo dueno' ($r.Output -match "del repo 'MiRepo'") $r.Output
    Assert-True 'open worktree imprime la ruta del worktree' ($r.Output -match [regex]::Escape('MiRepo.worktrees\buscable')) $r.Output
    Invoke-Wt -CmdArgs @('remove', 'buscable', '--delete-branch') -Cwd $repoDir | Out-Null

    Write-Host '== wt open inexistente desde fuera (debe fallar con guia) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('open', 'NoExiste') -Cwd $env:TEMP
    Assert-True 'open inexistente fuera falla' ($r.ExitCode -ne 0)
    Assert-True 'open inexistente sugiere wt repos' ($r.Output -match 'wt repos') $r.Output

    Write-Host '== validacion de argumentos del CLI ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('create', 'x', '--base') -Cwd $repoDir
    Assert-True '--base sin valor falla' ($r.ExitCode -ne 0)
    Assert-True '--base sin valor explica que falta el valor' ($r.Output -match 'requiere un valor') $r.Output
    $r = Invoke-Wt -CmdArgs @('open', 'feature-a', '--agente') -Cwd $repoDir
    Assert-True 'flag desconocido falla' ($r.ExitCode -ne 0)
    Assert-True 'flag desconocido lista los validos' ($r.Output -match 'Flag desconocido' -and $r.Output -match '--agent') $r.Output
    $r = Invoke-Wt -CmdArgs @('create', 'nombre con espacios', '--no-open') -Cwd $repoDir
    Assert-True 'nombre invalido falla' ($r.ExitCode -ne 0)
    Assert-True 'nombre invalido explica el motivo' ($r.Output -match 'Nombre de worktree invalido') $r.Output
    $r = Invoke-Wt -CmdArgs @('create', '..', '--no-open') -Cwd $repoDir
    Assert-True "nombre '..' rechazado" ($r.ExitCode -ne 0 -and $r.Output -match 'Nombre de worktree invalido') $r.Output

    Write-Host '== config set con valor que tiene espacios ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('config', 'set', 'defaultBase', 'origin/mi rama') -Cwd $repoDir
    Assert-True 'config set con espacios exit 0' ($r.ExitCode -eq 0) $r.Output
    $r = Invoke-Wt -CmdArgs @('config', 'get', 'defaultBase') -Cwd $repoDir
    Assert-True 'config set conserva el valor completo' ($r.Output.Trim() -eq 'origin/mi rama') $r.Output
    Invoke-Wt -CmdArgs @('config', 'set', 'defaultBase', '') -Cwd $repoDir | Out-Null

    Write-Host '== remove no confunde worktrees con prefijo comun ==' -ForegroundColor Cyan
    Invoke-Wt -CmdArgs @('create', 'pref', '--no-open') -Cwd $repoDir | Out-Null
    Invoke-Wt -CmdArgs @('create', 'prefijo', '--no-open') -Cwd $repoDir | Out-Null
    $r = Invoke-Wt -CmdArgs @('remove', 'pref', '--delete-branch') -Cwd (Join-Path $wtRoot 'prefijo')
    Assert-True 'remove desde un hermano con prefijo comun exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'no mueve el directorio actual innecesariamente' ($r.Output -notmatch 'Directorio actual movido') $r.Output
    Assert-True 'el hermano sigue existiendo' (Test-Path (Join-Path $wtRoot 'prefijo'))
    Invoke-Wt -CmdArgs @('remove', 'prefijo', '--delete-branch') -Cwd $repoDir | Out-Null

    Write-Host '== open <repo> por prefijo desde fuera de un repo ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('open', 'MiR') -Cwd $env:TEMP
    Assert-True 'open por prefijo de repo exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'open por prefijo imprime la ruta del repo' ($r.Output -match [regex]::Escape($repoDir)) $r.Output
    $r = Invoke-Wt -CmdArgs @('path', 'MiR') -Cwd $env:TEMP
    Assert-True 'path por prefijo de repo resuelve la raiz' ($r.Output -match [regex]::Escape($repoDir)) $r.Output

    Write-Host '== wt console: menu y salida ==' -ForegroundColor Cyan
    $r = Invoke-WtConsole -InputLines @('q') -Cwd $repoDir
    Assert-True 'console exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'console muestra el menu' ($r.Output -match 'consola interactiva') $r.Output

    Write-Host '== wt console fuera de un repo ==' -ForegroundColor Cyan
    $r = Invoke-WtConsole -InputLines @('q') -Cwd $env:TEMP
    Assert-True 'console fuera de repo exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'console fuera de repo lo avisa' ($r.Output -match 'No estas dentro de un repo') $r.Output

    Write-Host '== wt console: listar repos ==' -ForegroundColor Cyan
    $r = Invoke-WtConsole -InputLines @('r', 'q') -Cwd $repoDir
    Assert-True 'console r lista MiRepo' ($r.Output -match 'MiRepo') $r.Output

    Write-Host '== wt console: ir a un repo (cd) ==' -ForegroundColor Cyan
    $r = Invoke-WtConsole -InputLines @('g', '1', 'q') -Cwd $repoDir
    Assert-True 'console g muestra comando equivalente' ($r.Output -match 'Ejecutando: wt cd MiRepo') $r.Output
    Assert-True 'console g hace el cd' ($r.Output -match "Ahora en: $([regex]::Escape($repoDir))") $r.Output

    Write-Host '== wt console: opcion invalida ==' -ForegroundColor Cyan
    $r = Invoke-WtConsole -InputLines @('zz', 'q') -Cwd $repoDir
    Assert-True 'console avisa opcion invalida' ($r.Output -match 'Opcion invalida') $r.Output

    Write-Host '== wt console: create ==' -ForegroundColor Cyan
    $r = Invoke-WtConsole -InputLines @('2', 'console-a', '', 'q') -Cwd $repoDir
    Assert-True 'console create exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'console create muestra comando equivalente' ($r.Output -match 'wt create console-a') $r.Output
    Assert-True 'console create crea el directorio' (Test-Path (Join-Path $wtRoot 'console-a'))
    git -C $repoDir show-ref --verify --quiet refs/heads/console-a
    Assert-True 'console create crea la rama' ($LASTEXITCODE -eq 0)

    Write-Host '== wt console: list ==' -ForegroundColor Cyan
    $r = Invoke-WtConsole -InputLines @('1', 'q') -Cwd $repoDir
    Assert-True 'console list muestra console-a' ($r.Output -match 'console-a') $r.Output

    Write-Host '== wt console: open ==' -ForegroundColor Cyan
    $r = Invoke-WtConsole -InputLines @('3', '2', 'q') -Cwd $repoDir
    Assert-True 'console open muestra comando equivalente' ($r.Output -match 'Ejecutando: wt open console-a') $r.Output

    Write-Host '== wt console: remove con confirmacion ==' -ForegroundColor Cyan
    $r = Invoke-WtConsole -InputLines @('6', '1', 's', 'n', 's', 'q') -Cwd $repoDir
    Assert-True 'console remove exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'console remove elimina el directorio' (-not (Test-Path (Join-Path $wtRoot 'console-a')))
    git -C $repoDir show-ref --verify --quiet refs/heads/console-a
    Assert-True 'console remove elimina la rama' ($LASTEXITCODE -ne 0)

    Write-Host '== wt console: EOF (stdin cerrado) termina limpio ==' -ForegroundColor Cyan
    $r = Invoke-WtConsole -InputLines @('') -Cwd $repoDir
    Assert-True 'console ante EOF sale sin error' ($r.ExitCode -eq 0) $r.Output
}
finally {
    Pop-Location
    Remove-Item Env:\WT_CONFIG -ErrorAction SilentlyContinue
    Remove-Item Env:\WT_CONFIG_ONLY -ErrorAction SilentlyContinue
    if (Test-Path $tempRoot) {
        & git -C $repoDir worktree prune 2>$null
        Remove-Item -Recurse -Force $tempRoot -ErrorAction SilentlyContinue
    }
}

Write-Host ''
$color = 'Green'
if ($script:Failed -gt 0) { $color = 'Red' }
Write-Host ("Resultado: {0} OK, {1} fallidas" -f $script:Passed, $script:Failed) -ForegroundColor $color
if ($script:Failed -gt 0) { exit 1 }
exit 0
