#requires -Version 5.1
<#
.SYNOPSIS
    Instala el comando 'wt' en el perfil de PowerShell del usuario actual.
.DESCRIPTION
    Agrega (o actualiza) un bloque delimitado en $PROFILE.CurrentUserAllHosts que
    define la funcion wt (y el alias wtm), y crea la config global si no existe.
    Idempotente: se puede ejecutar varias veces sin duplicar entradas.
#>
[CmdletBinding(SupportsShouldProcess)]
param()

$ErrorActionPreference = 'Stop'

$moduleDir = $PSScriptRoot
$wtScript = Join-Path $moduleDir 'wt.ps1'
if (-not (Test-Path -LiteralPath $wtScript)) { throw "No se encontro '$wtScript'." }

$markerStart = '# >>> worktree-manager >>>'
$markerEnd = '# <<< worktree-manager <<<'
# La ruta se embebe en un string literal de PowerShell: hay que duplicar las comillas
# simples por si el directorio de instalacion las contiene.
$scriptLiteral = $wtScript.Replace("'", "''")
$block = @"
$markerStart
function wt {
    & '$scriptLiteral' @args
}
Set-Alias wtm -Value wt -ErrorAction SilentlyContinue
$markerEnd
"@

$profilePath = $PROFILE.CurrentUserAllHosts
$profileDir = Split-Path -Parent $profilePath
if ($profileDir -and -not (Test-Path -LiteralPath $profileDir)) {
    New-Item -ItemType Directory -Path $profileDir -Force | Out-Null
}
if (-not (Test-Path -LiteralPath $profilePath)) {
    New-Item -ItemType File -Path $profilePath -Force | Out-Null
}

# Get-Content -Raw devuelve $null en un archivo vacio.
$current = Get-Content -Raw -LiteralPath $profilePath
if ($null -eq $current) { $current = '' }
$pattern = '(?s)' + [regex]::Escape($markerStart) + '.*?' + [regex]::Escape($markerEnd)

if ($current -match $pattern) {
    # MatchEvaluator: evita que -replace interprete '$' dentro del bloque de reemplazo
    $updated = [regex]::Replace($current, $pattern, { param($m) $block })
    Set-Content -LiteralPath $profilePath -Value $updated -NoNewline
    Write-Host "Bloque 'wt' actualizado en $profilePath" -ForegroundColor Green
} else {
    Add-Content -LiteralPath $profilePath -Value ("`r`n" + $block)
    Write-Host "Bloque 'wt' agregado a $profilePath" -ForegroundColor Green
}

$configPath = Join-Path $HOME '.wt\config.json'
if (-not (Test-Path -LiteralPath $configPath)) {
    New-Item -ItemType Directory -Path (Split-Path -Parent $configPath) -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $moduleDir 'config.example.json') -Destination $configPath
    Write-Host "Config de ejemplo creada en $configPath" -ForegroundColor Green
}

Write-Host ''
Write-Host 'Listo. Reinicia la terminal (o ejecuta: . $PROFILE) y usa: wt help' -ForegroundColor Cyan
