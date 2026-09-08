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
    # exit $LASTEXITCODE: Invoke-Wt (B6) atrapa sus propios errores y solo deja el
    # codigo de salida en la variable, para no cerrar la sesion del usuario en el modo
    # funcion del perfil; en este -Command hay que propagarlo a mano como haria wt.ps1.
    $cmd = "Set-Location -LiteralPath '$cwdEscaped'; Import-Module '$module' -Force; Invoke-Wt -Arguments @($argLiterals); exit `$LASTEXITCODE"
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

function Invoke-WtAndLocation {
    <#
    .SYNOPSIS
        Como Invoke-Wt, pero ademas devuelve el directorio final del proceso (mismo
        -Command, asi que refleja si el comando hizo cd de verdad). Usado por M3 para
        distinguir 'wt path' (nunca relocaliza) de 'wt open' (si, cuando corresponde).
    #>
    param([string[]]$CmdArgs, [string]$Cwd)
    $module = (Join-Path $PSScriptRoot '..\wt.psm1') -replace "'", "''"
    $cwdEscaped = $Cwd -replace "'", "''"
    $argLiterals = ($CmdArgs | ForEach-Object { "'{0}'" -f ($_ -replace "'", "''") }) -join ', '
    $cmd = "Set-Location -LiteralPath '$cwdEscaped'; Import-Module '$module' -Force; " +
        "Invoke-Wt -Arguments @($argLiterals); Write-Output ('WT_FINAL_LOCATION:' + (Get-Location).Path); exit `$LASTEXITCODE"
    $prevEAP = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $out = & powershell.exe -NoProfile -ExecutionPolicy Bypass -Command $cmd 2>&1
        $exit = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $prevEAP
    }
    $text = ($out | Out-String)
    $finalLocation = ''
    if ($text -match 'WT_FINAL_LOCATION:(.*)') { $finalLocation = $Matches[1].Trim() }
    return @{ Output = $text; ExitCode = $exit; FinalLocation = $finalLocation }
}

function Invoke-WtConsole {
    # Ejecuta 'wt console' con las lineas de entrada pipeadas por stdin
    param([string[]]$InputLines, [string]$Cwd)
    $module = (Join-Path $PSScriptRoot '..\wt.psm1') -replace "'", "''"
    $cwdEscaped = $Cwd -replace "'", "''"
    $cmd = "Set-Location -LiteralPath '$cwdEscaped'; Import-Module '$module' -Force; Invoke-Wt -Arguments @('console'); exit `$LASTEXITCODE"
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
    # Las 17 claves con valores explicitos y neutros: WT_CONFIG_ONLY hace que esta
    # sea la unica fuente (ademas de los defaults), sin importar la config real de
    # la maquina que corre las pruebas.
    $testConfig = [ordered]@{
        worktreeRootTemplate = '{repoParent}\{repo}.worktrees\{name}'
        reposRoot            = $tempRoot
        reposDepth           = 1
        defaultBase          = ''
        branchPrefix         = ''
        editor               = ''
        terminal             = 'none'
        warpAgentTarget      = 'auto'
        warpAgentColor       = ''
        warpTerminalColor    = ''
        warpPath             = 'C:\no-existe\warp.exe'
        fetchBeforeCreate    = $false
        openOnCreate         = 'none'
        agentCommand         = 'no-existe-el-agente'
        agentShell           = 'none'
        copyOnCreate         = @()
        postCreate           = @()
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
    $featureAPath = Join-Path $wtRoot 'feature-a'
    Assert-True 'create --no-open no contamina stdout con la ruta desnuda (B9)' `
        ($r.Output -notmatch "(?m)^\s*$([regex]::Escape($featureAPath))\s*`$") $r.Output

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

    Write-Host '== wt create: hooks de creacion (copyOnCreate / postCreate) ==' -ForegroundColor Cyan
    Set-Content -Path (Join-Path $repoDir '.env') -Value 'SECRET=1'
    New-Item -ItemType Directory -Path (Join-Path $repoDir 'certs') -Force | Out-Null
    Set-Content -Path (Join-Path $repoDir 'certs\dev.pfx') -Value 'cert-fake'
    $hooksConfigPath = Join-Path $tempRoot 'config-hooks.json'
    $hooksConfig = [ordered]@{}
    foreach ($k in $testConfig.Keys) { $hooksConfig[$k] = $testConfig[$k] }
    $hooksConfig['copyOnCreate'] = @('.env', 'certs/*.pfx')
    $hooksConfig['postCreate'] = @('cmd /c echo hola> hook.txt')
    ([pscustomobject]$hooksConfig | ConvertTo-Json) | Set-Content -Path $hooksConfigPath
    $env:WT_CONFIG = $hooksConfigPath
    try {
        $r = Invoke-Wt -CmdArgs @('create', 'hooks-ok', '--no-open') -Cwd $repoDir
        Assert-True 'create con hooks exit 0' ($r.ExitCode -eq 0) $r.Output
        $hooksOkDir = Join-Path $wtRoot 'hooks-ok'
        Assert-True 'copyOnCreate copio .env' (Test-Path (Join-Path $hooksOkDir '.env')) $r.Output
        Assert-True 'copyOnCreate copio certs\dev.pfx preservando el subdirectorio' (Test-Path (Join-Path $hooksOkDir 'certs\dev.pfx')) $r.Output
        Assert-True 'postCreate creo hook.txt en el worktree' (Test-Path (Join-Path $hooksOkDir 'hook.txt')) $r.Output

        $r = Invoke-Wt -CmdArgs @('create', 'hooks-no-hooks', '--no-open', '--no-hooks') -Cwd $repoDir
        Assert-True 'create --no-hooks exit 0' ($r.ExitCode -eq 0) $r.Output
        $hooksSkippedDir = Join-Path $wtRoot 'hooks-no-hooks'
        Assert-True '--no-hooks no copia archivos' (-not (Test-Path (Join-Path $hooksSkippedDir '.env'))) $r.Output
        Assert-True '--no-hooks no corre postCreate' (-not (Test-Path (Join-Path $hooksSkippedDir 'hook.txt'))) $r.Output

        $hooksFailConfigPath = Join-Path $tempRoot 'config-hooks-fail.json'
        $hooksFailConfig = [ordered]@{}
        foreach ($k in $testConfig.Keys) { $hooksFailConfig[$k] = $testConfig[$k] }
        $hooksFailConfig['postCreate'] = @('cmd /c exit 7')
        ([pscustomobject]$hooksFailConfig | ConvertTo-Json) | Set-Content -Path $hooksFailConfigPath
        $env:WT_CONFIG = $hooksFailConfigPath
        $r = Invoke-Wt -CmdArgs @('create', 'hooks-fail', '--no-open') -Cwd $repoDir
        Assert-True 'postCreate que falla sale distinto de 0' ($r.ExitCode -ne 0) $r.Output
        Assert-True 'el mensaje nombra el comando que fallo' ($r.Output -match [regex]::Escape('cmd /c exit 7')) $r.Output
        $hooksFailDir = Join-Path $wtRoot 'hooks-fail'
        Assert-True 'el worktree queda creado pese al postCreate fallido' (Test-Path $hooksFailDir) $r.Output
        Remove-Item -LiteralPath $hooksFailConfigPath -ErrorAction SilentlyContinue
    } finally {
        $env:WT_CONFIG = $configPath
        # --force: copyOnCreate/postCreate dejan archivos sin trackear en el worktree
        # (.env, hook.txt), asi que 'git worktree remove' sin --force los rechazaria.
        Invoke-Wt -CmdArgs @('remove', 'hooks-ok', '--delete-branch', '--force') -Cwd $repoDir | Out-Null
        Invoke-Wt -CmdArgs @('remove', 'hooks-no-hooks', '--delete-branch', '--force') -Cwd $repoDir | Out-Null
        Invoke-Wt -CmdArgs @('remove', 'hooks-fail', '--delete-branch', '--force') -Cwd $repoDir | Out-Null
        Remove-Item -LiteralPath $hooksConfigPath -ErrorAction SilentlyContinue
    }

    Write-Host '== wt create: postCreate/copyOnCreate en .wt.json del repo se ignoran (seguridad) ==' -ForegroundColor Cyan
    # Requiere que .wt.json del repo sea un candidato real: se desactiva WT_CONFIG_ONLY
    # solo para este caso (WT_CONFIG sigue seteado y sigue ganando por ser el ultimo).
    $hooksRepoWtJsonPath = Join-Path $repoDir '.wt.json'
    ([pscustomobject]@{ postCreate = @('cmd /c echo hostil> hostil.txt'); copyOnCreate = @('.env') } | ConvertTo-Json) |
        Set-Content -Path $hooksRepoWtJsonPath
    Remove-Item Env:\WT_CONFIG_ONLY -ErrorAction SilentlyContinue
    try {
        $r = Invoke-Wt -CmdArgs @('create', 'hooks-hostile', '--no-open') -Cwd $repoDir
        Assert-True 'create con .wt.json hostil sigue en 0' ($r.ExitCode -eq 0) $r.Output
        Assert-True 'avisa que postCreate no se admite en el .wt.json del repo' ($r.Output -match "'postCreate'.*no se admite") $r.Output
        $hostileDir = Join-Path $wtRoot 'hooks-hostile'
        Assert-True 'postCreate del .wt.json del repo NO se ejecuto' (-not (Test-Path (Join-Path $hostileDir 'hostil.txt'))) $r.Output
    } finally {
        $env:WT_CONFIG_ONLY = '1'
        Remove-Item -LiteralPath $hooksRepoWtJsonPath -ErrorAction SilentlyContinue
        Invoke-Wt -CmdArgs @('remove', 'hooks-hostile', '--delete-branch') -Cwd $repoDir | Out-Null
    }

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

    Write-Host '== wt remove --delete-branch con commits sin mergear (no debe descartarlos) (A2) ==' -ForegroundColor Cyan
    Invoke-Wt -CmdArgs @('create', 'sin-mergear', '--no-open') -Cwd $repoDir | Out-Null
    $sinMergearDir = Join-Path $wtRoot 'sin-mergear'
    Set-Content -Path (Join-Path $sinMergearDir 'nuevo.txt') -Value 'contenido'
    git -C $sinMergearDir add nuevo.txt
    git -C $sinMergearDir commit -m 'commit sin mergear' --quiet
    $r = Invoke-Wt -CmdArgs @('remove', 'sin-mergear', '--delete-branch') -Cwd $repoDir
    Assert-True 'remove --delete-branch con commits sin mergear falla' ($r.ExitCode -ne 0)
    Assert-True 'el worktree se elimino de todos modos' (-not (Test-Path $sinMergearDir)) $r.Output
    git -C $repoDir show-ref --verify --quiet refs/heads/sin-mergear
    Assert-True 'la rama sigue existiendo (no se descartaron los commits)' ($LASTEXITCODE -eq 0)
    Assert-True 'el mensaje sugiere --force-branch' ($r.Output -match '--force-branch') $r.Output
    git -C $repoDir branch -D sin-mergear 2>&1 | Out-Null

    Write-Host '== wt remove --delete-branch --force-branch si descarta commits sin mergear (A2) ==' -ForegroundColor Cyan
    Invoke-Wt -CmdArgs @('create', 'sin-mergear-2', '--no-open') -Cwd $repoDir | Out-Null
    $sinMergear2Dir = Join-Path $wtRoot 'sin-mergear-2'
    Set-Content -Path (Join-Path $sinMergear2Dir 'nuevo.txt') -Value 'contenido'
    git -C $sinMergear2Dir add nuevo.txt
    git -C $sinMergear2Dir commit -m 'commit sin mergear' --quiet
    $r = Invoke-Wt -CmdArgs @('remove', 'sin-mergear-2', '--delete-branch', '--force-branch') -Cwd $repoDir
    Assert-True 'remove --delete-branch --force-branch exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'el worktree se elimino' (-not (Test-Path $sinMergear2Dir)) $r.Output
    git -C $repoDir show-ref --verify --quiet refs/heads/sin-mergear-2
    Assert-True '--force-branch si descarta los commits' ($LASTEXITCODE -ne 0)

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

    Write-Host '== ciclo lock / remove / unlock / remove (B4) ==' -ForegroundColor Cyan
    Invoke-Wt -CmdArgs @('create', 'lockable', '--no-open') -Cwd $repoDir | Out-Null
    $r = Invoke-Wt -CmdArgs @('lock', 'lockable', '--reason', 'revision pendiente') -Cwd $repoDir
    Assert-True 'lock exit 0' ($r.ExitCode -eq 0) $r.Output
    $r = Invoke-Wt -CmdArgs @('list') -Cwd $repoDir
    Assert-True 'list marca el bloqueado con motivo' ($r.Output -match 'bloqueado' -and $r.Output -match 'revision pendiente') $r.Output
    $r = Invoke-Wt -CmdArgs @('remove', 'lockable', '--delete-branch') -Cwd $repoDir
    Assert-True 'remove de un worktree bloqueado falla' ($r.ExitCode -ne 0)
    Assert-True 'el mensaje es claro y sugiere wt unlock' ($r.Output -match 'bloqueado' -and $r.Output -match 'wt unlock lockable') $r.Output
    Assert-True 'el worktree bloqueado no se elimino' (Test-Path (Join-Path $wtRoot 'lockable'))
    $r = Invoke-Wt -CmdArgs @('unlock', 'lockable') -Cwd $repoDir
    Assert-True 'unlock exit 0' ($r.ExitCode -eq 0) $r.Output
    $r = Invoke-Wt -CmdArgs @('remove', 'lockable', '--delete-branch') -Cwd $repoDir
    Assert-True 'remove funciona tras unlock' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'el worktree ya no existe' (-not (Test-Path (Join-Path $wtRoot 'lockable')))

    Write-Host '== wt prune ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('prune') -Cwd $repoDir
    Assert-True 'prune exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'prune sin nada que depurar lo dice explicitamente (B7)' ($r.Output -match 'No habia metadatos') $r.Output

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
    Assert-True 'prune muestra lo que depuro (B7)' ($r.Output -match 'stale') $r.Output
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

    Write-Host '== contrato de codigos de salida (B6): 0 OK, 1 uso, 2 git/entorno ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('inventado') -Cwd $repoDir
    Assert-True "'wt inventado' sale con 1 (error de uso)" ($r.ExitCode -eq 1) $r.Output
    $r = Invoke-Wt -CmdArgs @('open', 'no-existe') -Cwd $repoDir
    Assert-True "'wt open no-existe' sale con 2 (error de git/entorno)" ($r.ExitCode -eq 2) $r.Output
    $r = Invoke-Wt -CmdArgs @('list') -Cwd $repoDir
    Assert-True "'wt list' sale con 0" ($r.ExitCode -eq 0) $r.Output
    $r = Invoke-Wt -CmdArgs @('create') -Cwd $repoDir
    Assert-True "'wt create' sin nombre sale con 1 (argumento faltante)" ($r.ExitCode -eq 1) $r.Output
    $r = Invoke-Wt -CmdArgs @('open', 'feature-a', '--agente') -Cwd $repoDir
    Assert-True "flag desconocido sale con 1" ($r.ExitCode -eq 1) $r.Output

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
    Assert-True 'tab config de terminal usa prefijo wt-term y hash de ruta (M2)' ((Split-Path -Leaf $termFile) -match '^wt-term-demo-[0-9a-f]{8}\.toml$') $termFile
    $termToml = Get-Content -Raw -Path $termFile
    Assert-True 'tab config de terminal apunta al directorio' ($termToml.Contains("directory = 'C:\repo\wt-demo'")) $termToml
    Assert-True 'tab config de terminal sin comandos' (-not $termToml.Contains('commands =')) $termToml
    Assert-True 'tab config de terminal con color' ($termToml.Contains('color = "blue"')) $termToml
    Remove-Item Env:\WT_WARP_TAB_CONFIG -ErrorAction SilentlyContinue

    Write-Host '== ciclo de vida de los tab configs de Warp (M2) ==' -ForegroundColor Cyan
    $tabDirM2 = Join-Path $tempRoot 'tab_configs_m2'
    $env:WT_WARP_TAB_CONFIG = $tabDirM2
    try {
        # Segundo repo real: dos worktrees homonimos ('compartido') en repos distintos
        # deben producir dos tab configs distintos (M2), no uno que se pisa al otro.
        $repo2Dir = Join-Path $tempRoot 'MiRepo2'
        New-Item -ItemType Directory -Path $repo2Dir -Force | Out-Null
        git -C $repo2Dir init -b main --quiet
        git -C $repo2Dir config user.email 'wt-tests@local'
        git -C $repo2Dir config user.name 'wt-tests'
        git -C $repo2Dir config commit.gpgsign false
        Set-Content -Path (Join-Path $repo2Dir 'README.md') -Value 'test2'
        git -C $repo2Dir add README.md
        git -C $repo2Dir commit -m 'init' --quiet

        Invoke-Wt -CmdArgs @('create', 'compartido', '--no-open') -Cwd $repoDir | Out-Null
        Invoke-Wt -CmdArgs @('create', 'compartido', '--no-open') -Cwd $repo2Dir | Out-Null
        $wt1Path = Join-Path $wtRoot 'compartido'
        $wt2Path = Join-Path $tempRoot 'MiRepo2.worktrees\compartido'

        $file1 = Write-WtAgentTabConfig -Name 'compartido' -Path $wt1Path -Commands @('copilot')
        $file2 = Write-WtAgentTabConfig -Name 'compartido' -Path $wt2Path -Commands @('copilot')
        Assert-True 'dos worktrees homonimos en repos distintos producen dos archivos' ($file1 -ne $file2) ("{0} vs {1}" -f $file1, $file2)
        Assert-True 'ambos tab configs existen' ((Test-Path $file1) -and (Test-Path $file2))

        Invoke-Wt -CmdArgs @('remove', 'compartido', '--delete-branch') -Cwd $repoDir | Out-Null
        Assert-True 'wt remove borra el tab config del worktree eliminado' (-not (Test-Path $file1))
        Assert-True 'wt remove no toca el tab config del otro repo' (Test-Path $file2)

        # Huerfano: el directorio desaparece sin pasar por 'wt remove' (ej. borrado a
        # mano); 'wt clean' es quien lo detecta y depura.
        Remove-Item -Recurse -Force $wt2Path
        $file3 = Write-WtAgentTabConfig -Name 'valido' -Path $repoDir -Commands @('copilot')
        $r = Invoke-Wt -CmdArgs @('clean') -Cwd $repoDir
        Assert-True 'wt clean exit 0' ($r.ExitCode -eq 0) $r.Output
        Assert-True 'wt clean borra el huerfano' (-not (Test-Path $file2)) $r.Output
        Assert-True 'wt clean no toca un tab config valido' (Test-Path $file3) $r.Output
    } finally {
        Remove-Item Env:\WT_WARP_TAB_CONFIG -ErrorAction SilentlyContinue
        # MiRepo2 es exclusivo de esta prueba: se borra entero para no aparecer en los
        # conteos de 'wt repos' de las pruebas siguientes.
        if (Test-Path $repo2Dir) {
            git -C $repo2Dir worktree prune 2>&1 | Out-Null
            Remove-Item -Recurse -Force $repo2Dir -ErrorAction SilentlyContinue
        }
    }

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

    Write-Host '== reposRoot con varias raices y reposDepth (B11) ==' -ForegroundColor Cyan
    $b11Base = Join-Path $tempRoot 'b11-multiroot'
    $orgRoot = Join-Path $b11Base 'org-root'
    $flatRoot = Join-Path $b11Base 'flat-root'
    $nestedRepoDir = Join-Path $orgRoot 'myorg\nested-repo'
    $flatRepoDir = Join-Path $flatRoot 'other-repo'
    New-Item -ItemType Directory -Path $nestedRepoDir -Force | Out-Null
    New-Item -ItemType Directory -Path $flatRepoDir -Force | Out-Null
    git init -q -b main $nestedRepoDir
    git init -q -b main $flatRepoDir
    $b11ConfigPath = Join-Path $tempRoot 'config-b11-multiroot.json'
    $b11Config = [ordered]@{}
    foreach ($k in $testConfig.Keys) { $b11Config[$k] = $testConfig[$k] }
    $b11Config['reposRoot'] = @($orgRoot, $flatRoot)
    $b11Config['reposDepth'] = 2
    ([pscustomobject]$b11Config | ConvertTo-Json) | Set-Content -Path $b11ConfigPath
    $env:WT_CONFIG = $b11ConfigPath
    try {
        $r = Invoke-Wt -CmdArgs @('repos') -Cwd $env:TEMP
        Assert-True 'repos con reposDepth 2 exit 0' ($r.ExitCode -eq 0) $r.Output
        Assert-True 'repos encuentra el repo anidado (org/repo)' ($r.Output -match 'nested-repo') $r.Output
        Assert-True 'repos encuentra el repo de la raiz plana' ($r.Output -match 'other-repo') $r.Output
        Assert-True 'con varias raices, la tabla muestra la columna Raiz' ($r.Output -match 'Raiz') $r.Output

        $r = Invoke-Wt -CmdArgs @('repos', '--json') -Cwd $env:TEMP
        $multiRootJson = $null
        try { $multiRootJson = ($r.Output | ConvertFrom-Json) } catch {}
        Assert-True 'repos --json con varias raices parseable' ($null -ne $multiRootJson) $r.Output
        Assert-True 'repos --json tiene 2 repos' (@($multiRootJson).Count -eq 2) $r.Output
        Assert-True 'repos --json incluye el campo Raiz' (@($multiRootJson | Where-Object { $_.Raiz }).Count -eq 2) $r.Output

        $r = Invoke-Wt -CmdArgs @('cd', 'nested-repo') -Cwd $env:TEMP
        Assert-True 'cd al repo anidado funciona' ($r.ExitCode -eq 0 -and $r.Output -match [regex]::Escape($nestedRepoDir)) $r.Output
    } finally {
        $env:WT_CONFIG = $configPath
        Remove-Item -Recurse -Force $b11Base -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $b11ConfigPath -ErrorAction SilentlyContinue
    }

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

    Write-Host '== wt cd lleva a un worktree, no solo a un repo (B12) ==' -ForegroundColor Cyan
    Invoke-Wt -CmdArgs @('create', 'b12-worktree', '--no-open') -Cwd $repoDir | Out-Null
    $b12WorktreeDir = Join-Path $wtRoot 'b12-worktree'
    $r = Invoke-WtAndLocation -CmdArgs @('cd', 'b12-worktree') -Cwd $env:TEMP
    Assert-True 'cd a un worktree exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'cd deja el proceso en el directorio del worktree (no en la raiz del repo)' `
        ($r.FinalLocation.TrimEnd('\') -ieq $b12WorktreeDir.TrimEnd('\')) $r.FinalLocation
    Invoke-Wt -CmdArgs @('remove', 'b12-worktree', '--delete-branch') -Cwd $repoDir | Out-Null

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

    Write-Host '== wt config set con valor invalido (debe fallar sin tocar el archivo) (M5) ==' -ForegroundColor Cyan
    $beforeHash = (Get-FileHash -LiteralPath $configPath -Algorithm SHA256).Hash
    $r = Invoke-Wt -CmdArgs @('config', 'set', 'terminal', 'foo') -Cwd $repoDir
    Assert-True 'config set valor invalido falla' ($r.ExitCode -ne 0)
    Assert-True 'config set valor invalido explica los valores admitidos' ($r.Output -match 'warp, wt, none') $r.Output
    $afterHash = (Get-FileHash -LiteralPath $configPath -Algorithm SHA256).Hash
    Assert-True 'config set valor invalido no modifica el archivo' ($beforeHash -eq $afterHash)

    Write-Host '== wt create sin --no-open respeta openOnCreate (A3) ==' -ForegroundColor Cyan
    Invoke-Wt -CmdArgs @('config', 'set', 'openOnCreate', 'all') -Cwd $repoDir | Out-Null
    try {
        $r = Invoke-Wt -CmdArgs @('create', 'auto-open') -Cwd $repoDir
        Assert-True 'create sin --no-open exit 0' ($r.ExitCode -eq 0) $r.Output
        Assert-True 'create sin --no-open intenta abrir (editor vacio y terminal none: avisa del agente)' ($r.Output -match '(?i)agente') $r.Output
        $r2 = Invoke-Wt -CmdArgs @('create', 'no-auto-open', '--no-open') -Cwd $repoDir
        Assert-True 'create --no-open exit 0' ($r2.ExitCode -eq 0) $r2.Output
        Assert-True 'create --no-open no intenta abrir nada' ($r2.Output -notmatch '(?i)agente') $r2.Output
    } finally {
        Invoke-Wt -CmdArgs @('config', 'set', 'openOnCreate', 'none') -Cwd $repoDir | Out-Null
        Invoke-Wt -CmdArgs @('remove', 'auto-open', '--delete-branch') -Cwd $repoDir | Out-Null
        Invoke-Wt -CmdArgs @('remove', 'no-auto-open', '--delete-branch') -Cwd $repoDir | Out-Null
    }

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

    Write-Host '== .wt.json se resuelve desde dentro de un worktree real (B8) ==' -ForegroundColor Cyan
    # Find-WtRepoConfigFile camina el filesystem sin invocar git: este caso prueba que,
    # parado en un worktree real, encuentra el .wt.json de la raiz PRINCIPAL (no busca
    # uno en el worktree, que no tiene .wt.json propio). WT_CONFIG_ONLY tiene que estar
    # apagado para que .wt.json sea candidato; se usa un WT_CONFIG que NO define
    # defaultBase (a diferencia del resto de la suite) para que el valor del .wt.json
    # no quede tapado, sin perder el aislamiento del resto de las claves.
    Invoke-Wt -CmdArgs @('create', 'b8-worktree', '--no-open') -Cwd $repoDir | Out-Null
    $b8WorktreeDir = Join-Path $wtRoot 'b8-worktree'
    ([pscustomobject]@{ defaultBase = 'origin/b8-test' } | ConvertTo-Json) | Set-Content -Path $repoWtJsonPath
    $testConfigSinDefaultBase = [ordered]@{}
    foreach ($k in $testConfig.Keys) {
        if ($k -eq 'defaultBase') { continue }
        $testConfigSinDefaultBase[$k] = $testConfig[$k]
    }
    $configSinDefaultBasePath = Join-Path $tempRoot 'config-sin-defaultbase.json'
    ([pscustomobject]$testConfigSinDefaultBase | ConvertTo-Json) | Set-Content -Path $configSinDefaultBasePath
    Remove-Item Env:\WT_CONFIG_ONLY -ErrorAction SilentlyContinue
    $env:WT_CONFIG = $configSinDefaultBasePath
    try {
        $r = Invoke-Wt -CmdArgs @('config', 'get', 'defaultBase') -Cwd $b8WorktreeDir
        Assert-True 'defaultBase del .wt.json de la raiz se ve desde el worktree' ($r.Output.Trim() -eq 'origin/b8-test') $r.Output
    } finally {
        $env:WT_CONFIG = $configPath
        $env:WT_CONFIG_ONLY = '1'
        Remove-Item -LiteralPath $repoWtJsonPath -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $configSinDefaultBasePath -ErrorAction SilentlyContinue
    }
    Invoke-Wt -CmdArgs @('remove', 'b8-worktree', '--delete-branch') -Cwd $repoDir | Out-Null

    Write-Host '== wt version ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('version') -Cwd $env:TEMP
    Assert-True 'version exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'version imprime la version del manifiesto' ($r.Output -match 'wt 0\.2\.0') $r.Output

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

    Write-Host '== wt path no cambia el directorio actual; wt open si (M3) ==' -ForegroundColor Cyan
    Invoke-Wt -CmdArgs @('create', 'feature-m3', '--no-open') -Cwd $repoDir | Out-Null
    $r = Invoke-WtAndLocation -CmdArgs @('path', 'feature-m3') -Cwd $env:TEMP
    Assert-True 'path fuera de repo exit 0' ($r.ExitCode -eq 0) $r.Output
    Assert-True 'path fuera de repo imprime la ruta del worktree' ($r.Output -match [regex]::Escape('MiRepo.worktrees\feature-m3')) $r.Output
    Assert-True 'path NO cambia el directorio del proceso' ($r.FinalLocation.TrimEnd('\') -ieq $env:TEMP.TrimEnd('\')) $r.FinalLocation
    $r2 = Invoke-WtAndLocation -CmdArgs @('open', 'feature-m3') -Cwd $env:TEMP
    Assert-True 'open fuera de repo exit 0' ($r2.ExitCode -eq 0) $r2.Output
    Assert-True 'open SI cambia el directorio del proceso (al repo)' ($r2.FinalLocation.TrimEnd('\') -ieq $repoDir.TrimEnd('\')) $r2.FinalLocation
    Invoke-Wt -CmdArgs @('remove', 'feature-m3', '--delete-branch') -Cwd $repoDir | Out-Null

    Write-Host '== wt open inexistente desde fuera (debe fallar con guia) ==' -ForegroundColor Cyan
    $r = Invoke-Wt -CmdArgs @('open', 'NoExiste') -Cwd $env:TEMP
    Assert-True 'open inexistente fuera falla' ($r.ExitCode -ne 0)
    Assert-True 'open inexistente sugiere wt repos' ($r.Output -match 'wt repos') $r.Output

    Write-Host '== worktree homonimo en dos repos: ambiguedad explicita (M4) ==' -ForegroundColor Cyan
    $repoAmbDir = Join-Path $tempRoot 'MiRepoAmbiguo'
    try {
        New-Item -ItemType Directory -Path $repoAmbDir -Force | Out-Null
        git -C $repoAmbDir init -b main --quiet
        git -C $repoAmbDir config user.email 'wt-tests@local'
        git -C $repoAmbDir config user.name 'wt-tests'
        git -C $repoAmbDir config commit.gpgsign false
        Set-Content -Path (Join-Path $repoAmbDir 'README.md') -Value 'ambiguo'
        git -C $repoAmbDir add README.md
        git -C $repoAmbDir commit -m 'init' --quiet
        Invoke-Wt -CmdArgs @('create', 'ambiguo', '--no-open') -Cwd $repoAmbDir | Out-Null
        Invoke-Wt -CmdArgs @('create', 'ambiguo', '--no-open') -Cwd $repoDir | Out-Null

        $r = Invoke-Wt -CmdArgs @('path', 'ambiguo') -Cwd $env:TEMP
        Assert-True 'worktree homonimo en dos repos falla' ($r.ExitCode -ne 0)
        Assert-True 'el mensaje nombra ambos repos' ($r.Output -match 'MiRepo' -and $r.Output -match 'MiRepoAmbiguo') $r.Output
        Assert-True 'el mensaje explica la ambiguedad' ($r.Output -match 'existe en varios repos') $r.Output

        Invoke-Wt -CmdArgs @('remove', 'ambiguo', '--delete-branch') -Cwd $repoDir | Out-Null
    } finally {
        if (Test-Path $repoAmbDir) {
            git -C $repoAmbDir worktree prune 2>&1 | Out-Null
            Remove-Item -Recurse -Force $repoAmbDir -ErrorAction SilentlyContinue
        }
        $repoAmbWorktrees = Join-Path $tempRoot 'MiRepoAmbiguo.worktrees'
        if (Test-Path $repoAmbWorktrees) { Remove-Item -Recurse -Force $repoAmbWorktrees -ErrorAction SilentlyContinue }
    }

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
