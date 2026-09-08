#requires -Version 5.1
<#
.SYNOPSIS
    Revierte install.ps1: quita el bloque 'wt' del perfil de PowerShell.
.DESCRIPTION
    Elimina unicamente el bloque delimitado por los mismos marcadores que usa
    install.ps1 (# >>> worktree-manager >>> ... # <<< worktree-manager <<<) y deja el
    resto del perfil intacto. No toca %USERPROFILE%\.wt\config.json salvo que se pase
    -RemoveConfig.
.PARAMETER RemoveConfig
    Ademas borra %USERPROFILE%\.wt\config.json (y su carpeta, si queda vacia).
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\uninstall.ps1
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File .\uninstall.ps1 -WhatIf
#>
[CmdletBinding(SupportsShouldProcess)]
param([switch]$RemoveConfig)

$ErrorActionPreference = 'Stop'

$markerStart = '# >>> worktree-manager >>>'
$markerEnd = '# <<< worktree-manager <<<'
$profilePath = $PROFILE.CurrentUserAllHosts

if (-not (Test-Path -LiteralPath $profilePath)) {
    Write-Host "No hay perfil en '$profilePath'; nada que quitar." -ForegroundColor Yellow
} else {
    $current = Get-Content -Raw -LiteralPath $profilePath
    if ($null -eq $current) { $current = '' }
    $pattern = '(?s)' + [regex]::Escape($markerStart) + '.*?' + [regex]::Escape($markerEnd)

    if ($current -match $pattern) {
        # Se quita tambien un salto de linea inmediatamente anterior (el que agrega
        # install.ps1 al insertar el bloque), para no ir dejando lineas en blanco de mas
        # en instalaciones/desinstalaciones repetidas.
        $withNewline = '(?:\r?\n)?' + $pattern
        $updated = [regex]::Replace($current, $withNewline, '')
        if ($PSCmdlet.ShouldProcess($profilePath, "Quitar el bloque 'wt'")) {
            Set-Content -LiteralPath $profilePath -Value $updated -NoNewline
            Write-Host "Bloque 'wt' quitado de $profilePath (el resto del perfil queda igual)." -ForegroundColor Green
        }
    } else {
        Write-Host "No se encontro el bloque 'wt' en $profilePath; nada que quitar." -ForegroundColor Yellow
    }
}

$configPath = Join-Path $HOME '.wt\config.json'
if ($RemoveConfig) {
    if (Test-Path -LiteralPath $configPath) {
        if ($PSCmdlet.ShouldProcess($configPath, 'Borrar config')) {
            Remove-Item -LiteralPath $configPath -Force
            Write-Host "Config borrada: $configPath" -ForegroundColor Green
            $configDir = Split-Path -Parent $configPath
            if ((Test-Path -LiteralPath $configDir) -and -not (Get-ChildItem -LiteralPath $configDir -Force -ErrorAction SilentlyContinue)) {
                if ($PSCmdlet.ShouldProcess($configDir, 'Borrar directorio de config vacio')) {
                    Remove-Item -LiteralPath $configDir -Force
                }
            }
        }
    } else {
        Write-Host "No hay config en '$configPath'; nada que borrar." -ForegroundColor Yellow
    }
} else {
    Write-Host "Config conservada: $configPath (usa -RemoveConfig para borrarla tambien)." -ForegroundColor Cyan
}

Write-Host ''
Write-Host 'Listo. Reinicia la terminal (o ejecuta: . $PROFILE) para que el cambio tome efecto.' -ForegroundColor Cyan
