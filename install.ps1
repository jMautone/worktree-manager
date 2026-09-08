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
    if ($PSCmdlet.ShouldProcess($profileDir, 'Crear directorio del perfil')) {
        New-Item -ItemType Directory -Path $profileDir -Force | Out-Null
    }
}
if (-not (Test-Path -LiteralPath $profilePath)) {
    if ($PSCmdlet.ShouldProcess($profilePath, 'Crear archivo de perfil')) {
        New-Item -ItemType File -Path $profilePath -Force | Out-Null
    }
}

# Get-Content -Raw devuelve $null en un archivo vacio; bajo -WhatIf el archivo puede
# no existir todavia (la creacion de arriba se salteo), asi que se trata igual que vacio.
$current = ''
if (Test-Path -LiteralPath $profilePath) {
    $current = Get-Content -Raw -LiteralPath $profilePath
    if ($null -eq $current) { $current = '' }
}
$pattern = '(?s)' + [regex]::Escape($markerStart) + '.*?' + [regex]::Escape($markerEnd)

if ($current -match $pattern) {
    # MatchEvaluator: evita que -replace interprete '$' dentro del bloque de reemplazo.
    # Sin 'param' explicito: el Match que pasa el delegado no se usa (el reemplazo sale
    # del closure $block), asi que no hay parametro declarado que quede sin uso.
    $updated = [regex]::Replace($current, $pattern, { $block })
    if ($PSCmdlet.ShouldProcess($profilePath, "Actualizar el bloque 'wt'")) {
        Set-Content -LiteralPath $profilePath -Value $updated -NoNewline
        Write-Host "Bloque 'wt' actualizado en $profilePath" -ForegroundColor Green
    }
} else {
    if ($PSCmdlet.ShouldProcess($profilePath, "Agregar el bloque 'wt'")) {
        # -NoNewline: Add-Content agrega un salto de linea propio ademas del contenido:
        # sin esto, uninstall.ps1 no puede dejar el resto del perfil byte a byte igual
        # (le quedaria un \r\n colgando donde estuvo el bloque).
        Add-Content -LiteralPath $profilePath -Value ("`r`n" + $block) -NoNewline
        Write-Host "Bloque 'wt' agregado a $profilePath" -ForegroundColor Green
    }
}

$configPath = Join-Path $HOME '.wt\config.json'
if (-not (Test-Path -LiteralPath $configPath)) {
    $configDir = Split-Path -Parent $configPath
    if (-not (Test-Path -LiteralPath $configDir) -and $PSCmdlet.ShouldProcess($configDir, 'Crear directorio de config')) {
        New-Item -ItemType Directory -Path $configDir -Force | Out-Null
    }
    if ($PSCmdlet.ShouldProcess($configPath, 'Copiar config.example.json')) {
        Copy-Item -LiteralPath (Join-Path $moduleDir 'config.example.json') -Destination $configPath
        Write-Host "Config de ejemplo creada en $configPath" -ForegroundColor Green
    }
}

Write-Host ''
Write-Host 'Listo. Reinicia la terminal (o ejecuta: . $PROFILE) y usa: wt help' -ForegroundColor Cyan
