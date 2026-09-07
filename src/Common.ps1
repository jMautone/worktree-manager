# ---------------------------------------------------------------------------
# Common: procesos externos, rutas, validacion y presentacion.
# Nivel 0 de la arquitectura: no depende de ningun otro archivo de src/.
# ---------------------------------------------------------------------------

# --- Presentacion -----------------------------------------------------------
# Unico punto donde se escribe a la consola. La logica de negocio devuelve datos
# y solo la capa de comandos usa estos helpers.

function Write-WtInfo {
    param([Parameter(Mandatory)][AllowEmptyString()][string]$Message)
    Write-Host $Message -ForegroundColor Cyan
}

function Write-WtSuccess {
    param([Parameter(Mandatory)][AllowEmptyString()][string]$Message)
    Write-Host $Message -ForegroundColor Green
}

function Write-WtDetail {
    param([Parameter(Mandatory)][AllowEmptyString()][string]$Message)
    Write-Host $Message -ForegroundColor DarkGray
}

function Write-WtNotice {
    param([Parameter(Mandatory)][AllowEmptyString()][string]$Message)
    Write-Host $Message -ForegroundColor Yellow
}

function Write-WtWarn {
    param([Parameter(Mandatory)][AllowEmptyString()][string]$Message)
    Write-Warning $Message
}

function Write-WtLine {
    param([AllowEmptyString()][string]$Message = '')
    Write-Host $Message
}

function Write-WtTable {
    param([Parameter(Mandatory)][AllowEmptyCollection()][object[]]$Rows)
    $Rows | Format-Table -AutoSize | Out-String | Write-Host
}

# --- Procesos externos ------------------------------------------------------

function Invoke-WtProcess {
    <#
    .SYNOPSIS
        Unico punto de invocacion de ejecutables externos.
    .DESCRIPTION
        Aisla $ErrorActionPreference (git escribe en stderr en situaciones normales),
        pasa los argumentos como array -nunca concatenados en un string- y devuelve
        salida, texto y codigo de salida. Con -AllowFailure tambien absorbe el caso
        "el ejecutable no existe" en vez de propagarlo.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$FilePath,
        [AllowEmptyCollection()][string[]]$Arguments = @(),
        [switch]$AllowFailure
    )
    $prevEAP = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $output = @()
    $exit = -1
    $failure = $null
    try {
        if ($Arguments -and $Arguments.Count -gt 0) {
            $output = & $FilePath @Arguments 2>&1
        } else {
            $output = & $FilePath 2>&1
        }
        $exit = $LASTEXITCODE
        if ($null -eq $exit) { $exit = 0 }
    } catch {
        $failure = $_.Exception.Message
        $output = @($failure)
        $exit = -1
    } finally {
        $ErrorActionPreference = $prevEAP
    }
    $text = ($output | Out-String).Trim()
    if ($exit -ne 0 -and -not $AllowFailure) {
        $detail = $text
        if (-not $detail) { $detail = $failure }
        throw ("{0} {1} fallo (exit {2}): {3}" -f $FilePath, ($Arguments -join ' '), $exit, $detail)
    }
    return [pscustomobject]@{
        Output   = $output
        Text     = $text
        ExitCode = $exit
        Success  = ($exit -eq 0)
    }
}

function Invoke-WtGit {
    <#
    .SYNOPSIS
        Especializacion de Invoke-WtProcess para git, con -C opcional.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string[]]$Arguments,
        [string]$WorkingDirectory,
        [switch]$AllowFailure
    )
    $full = @()
    if ($WorkingDirectory) { $full += @('-C', $WorkingDirectory) }
    $full += $Arguments
    $result = Invoke-WtProcess -FilePath 'git' -Arguments $full -AllowFailure
    if (-not $result.Success -and -not $AllowFailure) {
        throw ("git {0} fallo (exit {1}): {2}" -f ($Arguments -join ' '), $result.ExitCode, $result.Text)
    }
    return $result
}

function Format-WtProcessArgument {
    <#
    .SYNOPSIS
        Entrecomilla un argumento para Start-Process -ArgumentList.
    .DESCRIPTION
        Las rutas de Windows no pueden contener '"', asi que alcanza con envolverlas;
        el unico caso especial es la barra final, que escaparia la comilla de cierre
        (ej. 'C:\' -> "C:\\").
    #>
    param([Parameter(Mandatory)][AllowEmptyString()][string]$Value)
    $v = $Value
    if ($v.EndsWith('\')) { $v += '\' }
    return '"' + $v + '"'
}

function ConvertTo-WtJson {
    <#
    .SYNOPSIS
        Serializa a JSON garantizando siempre un array.
    .DESCRIPTION
        ConvertTo-Json colapsa las colecciones de un solo elemento en un objeto, lo que
        rompe a cualquier consumidor que itere la salida de --json.
    #>
    param([AllowEmptyCollection()][object[]]$InputObject)
    $items = @($InputObject)
    if ($items.Count -eq 0) { return '[]' }
    $json = ConvertTo-Json -InputObject $items -Depth 6
    if (-not $json.TrimStart().StartsWith('[')) { $json = "[`r`n$json`r`n]" }
    return $json
}

function Test-WtCommand {
    param([Parameter(Mandatory)][AllowEmptyString()][string]$Name)
    if (-not $Name) { return $false }
    return ($null -ne (Get-Command $Name -ErrorAction SilentlyContinue))
}

# --- Rutas ------------------------------------------------------------------

function ConvertTo-WtFullPath {
    <#
    .SYNOPSIS
        Normaliza una ruta: '/' -> '\', absoluta y sin barra final.
    .DESCRIPTION
        git devuelve rutas con '/'; el resto del programa compara y muestra rutas de
        Windows. Las relativas se resuelven contra la ubicacion actual de PowerShell
        (no contra el cwd del proceso, que puede diferir).
    #>
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSAvoidUsingEmptyCatchBlock', '',
        Justification = 'Ruta sintetica o invalida: se devuelve la forma normalizada disponible en vez de fallar.')]
    param([AllowEmptyString()][string]$Path)
    if (-not $Path) { return '' }
    $p = $Path.Trim() -replace '/', '\'
    if (-not $p) { return '' }
    try {
        if (-not [IO.Path]::IsPathRooted($p)) {
            $p = Join-Path (Get-Location).Path $p
        }
        $p = [IO.Path]::GetFullPath($p)
    } catch {
        # Rutas sinteticas o invalidas: se devuelve la forma normalizada disponible.
    }
    if ($p.Length -gt 3) { $p = $p.TrimEnd('\') }
    return $p
}

function Test-WtPathEquals {
    param([AllowEmptyString()][string]$Left, [AllowEmptyString()][string]$Right)
    if (-not $Left -or -not $Right) { return $false }
    return ((ConvertTo-WtFullPath $Left) -ieq (ConvertTo-WtFullPath $Right))
}

function Test-WtPathIsUnder {
    <#
    .SYNOPSIS
        $Path esta dentro de $Root (o es $Root), comparando por componentes.
    .DESCRIPTION
        Un StartsWith crudo daria verdadero para 'C:\x\feature-a-backup' vs
        'C:\x\feature-a'; por eso se exige el separador de directorio.
    #>
    param([AllowEmptyString()][string]$Path, [AllowEmptyString()][string]$Root)
    if (-not $Path -or -not $Root) { return $false }
    $p = ConvertTo-WtFullPath $Path
    $r = ConvertTo-WtFullPath $Root
    if ($p -ieq $r) { return $true }
    return $p.StartsWith(($r.TrimEnd('\') + '\'), [System.StringComparison]::OrdinalIgnoreCase)
}

function Test-WtPathExists {
    param([AllowEmptyString()][string]$Path)
    if (-not $Path) { return $false }
    return (Test-Path -LiteralPath $Path)
}

function Set-WtLocation {
    <#
    .SYNOPSIS
        Cambia la ubicacion actual (y con eso la config efectiva, que depende del repo).
    #>
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '',
        Justification = 'Wrapper directo de Set-Location, invocado por comandos que ya decidieron moverse.')]
    param([Parameter(Mandatory)][string]$Path)
    Set-Location -LiteralPath $Path
}

# --- Validacion -------------------------------------------------------------

$script:WtInvalidNameChars = '[\\/:*?"<>|\s]'
$script:WtReservedNames = @(
    'CON', 'PRN', 'AUX', 'NUL',
    'COM1', 'COM2', 'COM3', 'COM4', 'COM5', 'COM6', 'COM7', 'COM8', 'COM9',
    'LPT1', 'LPT2', 'LPT3', 'LPT4', 'LPT5', 'LPT6', 'LPT7', 'LPT8', 'LPT9'
)

function Test-WtWorktreeName {
    <#
    .SYNOPSIS
        Devuelve '' si el nombre es valido, o el motivo del rechazo.
    #>
    param([AllowEmptyString()][string]$Name)
    if (-not $Name) { return 'no puede estar vacio' }
    if ($Name -match $script:WtInvalidNameChars) {
        return 'no puede contener espacios ni los caracteres \ / : * ? " < > |'
    }
    if ($Name -eq '.' -or $Name -eq '..') { return "'.' y '..' estan reservados" }
    if ($Name.EndsWith('.') -or $Name.EndsWith(' ')) { return 'no puede terminar en punto ni en espacio' }
    $stem = ($Name -split '\.')[0]
    if ($script:WtReservedNames -contains $stem.ToUpperInvariant()) {
        return "'$stem' es un nombre reservado de Windows"
    }
    return ''
}

function Assert-WtWorktreeName {
    param([AllowEmptyString()][string]$Name)
    $reason = Test-WtWorktreeName -Name $Name
    if ($reason) { throw "Nombre de worktree invalido: '$Name': $reason." }
}

function Assert-WtBranchName {
    <#
    .SYNOPSIS
        Valida el nombre de rama con el propio git, para fallar con un mensaje de wt
        en vez de con el error crudo de 'git worktree add'.
    #>
    param([AllowEmptyString()][string]$Branch)
    if (-not $Branch) { throw 'El nombre de rama no puede estar vacio.' }
    $check = Invoke-WtGit -Arguments @('check-ref-format', '--branch', $Branch) -AllowFailure
    if (-not $check.Success) {
        throw "Nombre de rama invalido: '$Branch'. Revisa 'branchPrefix' en la config o el valor de --branch."
    }
}

function ConvertTo-WtSafeFileName {
    <#
    .SYNOPSIS
        Nombre utilizable como archivo (tab configs de Warp) a partir de un nombre libre.
    #>
    param([Parameter(Mandatory)][AllowEmptyString()][string]$Name)
    $safe = ($Name -replace '[^A-Za-z0-9._-]', '-').Trim('-. ')
    if (-not $safe) { $safe = 'wt' }
    if ($safe.Length -gt 60) { $safe = $safe.Substring(0, 60) }
    return $safe
}
