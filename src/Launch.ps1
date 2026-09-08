# ---------------------------------------------------------------------------
# Launch: apertura del editor, de la terminal y del agente Copilot CLI.
# Toda la integracion con Warp (tab configs TOML + esquema warp://) vive aca.
# Depende de: Common.
# ---------------------------------------------------------------------------

# --- Editor -----------------------------------------------------------------

function Open-WtEditor {
    <#
    .SYNOPSIS
        Abre el editor configurado en una ruta. Unica implementacion (la usan
        'open', 'cd --open' y la consola).
    .OUTPUTS
        $true si se abrio algo.
    #>
    param([Parameter(Mandatory)][string]$Path, [Parameter(Mandatory)]$Config)
    $editor = [string]$Config.editor
    if (-not $editor) { return $false }
    if (-not (Test-WtCommand $editor)) {
        Write-WtWarn "Editor '$editor' no encontrado en PATH."
        return $false
    }
    Start-Process -FilePath $editor -ArgumentList (Format-WtProcessArgument $Path)
    Write-WtSuccess "Editor abierto en $Path"
    return $true
}

# --- Node / Copilot ---------------------------------------------------------

function Get-WtNodeVersion {
    if (-not (Test-WtCommand 'node')) { return '' }
    $r = Invoke-WtProcess -FilePath 'node' -Arguments @('--version') -AllowFailure
    if (-not $r.Success) { return '' }
    return [string]($r.StdOut | Select-Object -First 1)
}

function Get-WtNodeMajor {
    <#
    .SYNOPSIS
        Version mayor de Node a usar: la de la sesion si es >= 18; si no, la mayor
        instalada via fnm. $null si no hay ninguna adecuada.
    #>
    $version = Get-WtNodeVersion
    if ($version -match '^v?(\d+)' -and [int]$Matches[1] -ge 18) { return $Matches[1] }
    if (-not (Test-WtCommand 'fnm')) { return $null }
    $listed = (Invoke-WtProcess -FilePath 'fnm' -Arguments @('list') -AllowFailure).StdOut -join "`n"
    $candidates = @([regex]::Matches($listed, 'v?(\d+)\.\d+\.\d+') |
        ForEach-Object { [int]$_.Groups[1].Value } |
        Where-Object { $_ -ge 18 } |
        Sort-Object -Descending -Unique)
    if ($candidates.Count -gt 0) { return [string]$candidates[0] }
    return $null
}

function Get-WtAgentCommands {
    <#
    .SYNOPSIS
        Comandos que corre el tab del agente: fijar Node con fnm (si esta), avisar si
        la version es vieja y lanzar copilot.
    #>
    $commands = @()
    if (Test-WtCommand 'fnm') {
        $major = Get-WtNodeMajor
        if ($major) { $commands += "fnm use $major" }
    }
    $commands += 'if ((node --version 2>$null) -notmatch "^v?(1[89]|[2-9]\d)") { Write-Warning "Node menor a 18: Copilot CLI puede fallar. Revisa la version activa (fnm)." }'
    $commands += 'copilot'
    return $commands
}

# --- Warp: tab configs ------------------------------------------------------

function Get-WtTabConfigDir {
    # Warp Stable en Windows; WT_WARP_TAB_CONFIG lo sobreescribe (pruebas)
    if ($env:WT_WARP_TAB_CONFIG) { return $env:WT_WARP_TAB_CONFIG }
    return (Join-Path $env:APPDATA 'warp\Warp\data\tab_configs')
}

function ConvertTo-WtTomlBasicString {
    <#
    .SYNOPSIS
        Escapa un valor como basic string de TOML ("...").
    #>
    param([AllowEmptyString()][string]$Value)
    # En el string de reemplazo la barra no es especial (solo lo es '$'):
    # '\\' son dos barras literales, que es justo el escape TOML de una barra.
    $escaped = $Value -replace '\\', '\\'
    $escaped = $escaped -replace '"', '\"'
    $escaped = $escaped -replace "`t", '\t'
    $escaped = $escaped -replace "`r", '\r'
    $escaped = $escaped -replace "`n", '\n'
    return '"' + $escaped + '"'
}

function ConvertTo-WtTomlLiteralString {
    <#
    .SYNOPSIS
        Literal string de TOML ('...'), que no admite escapes: las rutas de Windows se
        escriben tal cual y por eso se rechaza la comilla simple.
    #>
    param([Parameter(Mandatory)][AllowEmptyString()][string]$Value, [Parameter(Mandatory)][string]$Description)
    if ($Value.Contains("'")) {
        throw "$Description contiene una comilla simple; no se puede generar el tab config de Warp."
    }
    return "'" + $Value + "'"
}

function New-WtWarpTabConfigContent {
    <#
    .SYNOPSIS
        Genera el contenido TOML de un tab config de Warp. Funcion pura (testeable).
    #>
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '',
        Justification = 'Funcion pura que arma un string; el verbo New no implica efectos.')]
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Path,
        [string[]]$Commands,
        [string]$Title,
        [string]$Color,
        [string]$Kind = 'agent'
    )
    $directory = ConvertTo-WtTomlLiteralString -Value $Path -Description "La ruta '$Path'"
    $lines = @(
        ("name = {0}" -f (ConvertTo-WtTomlBasicString "wt-$Kind-$Name"))
        ("title = {0}" -f (ConvertTo-WtTomlBasicString $Title))
    )
    if ($Color) { $lines += ("color = {0}" -f (ConvertTo-WtTomlBasicString $Color)) }
    $lines += @(
        ''
        '[[panes]]'
        'id = "main"'
        'type = "terminal"'
        "directory = $directory"
    )
    if ($Commands -and $Commands.Count -gt 0) {
        $literals = @($Commands | ForEach-Object {
            ConvertTo-WtTomlLiteralString -Value $_ -Description "El comando '$_'"
        })
        $lines += ("commands = [{0}]" -f ($literals -join ', '))
    }
    return ($lines -join "`n")
}

function Write-WtAgentTabConfig {
    <#
    .SYNOPSIS
        Genera (o actualiza) un tab config TOML de Warp (agente o terminal comun).
    .OUTPUTS
        La ruta del archivo escrito.
    #>
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Path,
        [string[]]$Commands,
        [string]$Title,
        [string]$Color,
        [ValidateSet('agent', 'term')][string]$Kind = 'agent'
    )
    if (-not $Title) { $Title = "copilot: $Name" }
    $content = New-WtWarpTabConfigContent -Name $Name -Path $Path -Commands $Commands `
        -Title $Title -Color $Color -Kind $Kind
    $dir = Get-WtTabConfigDir
    if (-not (Test-WtPathExists $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    $file = Join-Path $dir ("wt-{0}-{1}.toml" -f $Kind, (ConvertTo-WtSafeFileName $Name))
    Set-Content -LiteralPath $file -Value $content -Encoding UTF8
    return $file
}

# --- Warp: destino y apertura ----------------------------------------------

function Test-WtWarpRunning {
    # WT_WARP_RUNNING ('1'/'0') lo sobreescribe (pruebas)
    if ($env:WT_WARP_RUNNING) { return ($env:WT_WARP_RUNNING -eq '1') }
    return ($null -ne (Get-Process -Name 'warp' -ErrorAction SilentlyContinue))
}

function Test-WtWarpSameWindow {
    <#
    .SYNOPSIS
        'tab' = siempre misma ventana; 'window' = siempre nueva;
        'auto' = misma si wt corre dentro de Warp o hay un Warp activo.
    #>
    param([string]$Target = 'auto')
    if ($Target -eq 'tab') { return $true }
    if ($Target -eq 'window') { return $false }
    if ($env:TERM_PROGRAM -eq 'WarpTerminal') { return $true }
    return (Test-WtWarpRunning)
}

function Open-WtWarpTab {
    <#
    .SYNOPSIS
        Escribe un tab config y lo abre con el esquema warp://tab_config/.
    .OUTPUTS
        $true si se abrio como pestana de la ventana activa.
    #>
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Path,
        [string[]]$Commands,
        [string]$Title,
        [string]$Color,
        [ValidateSet('agent', 'term')][string]$Kind = 'agent',
        [string]$Target = 'auto'
    )
    $file = Write-WtAgentTabConfig -Name $Name -Path $Path -Commands $Commands `
        -Title $Title -Color $Color -Kind $Kind
    $stem = [IO.Path]::GetFileNameWithoutExtension($file)
    $uri = "warp://tab_config/" + [uri]::EscapeDataString($stem)
    $sameWindow = Test-WtWarpSameWindow -Target $Target
    if (-not $sameWindow) { $uri += '?new_window=true' }
    Start-Process $uri
    return $sameWindow
}

function Open-WtAgentInWarp {
    <#
    .SYNOPSIS
        Abre Warp con un tab config que corre Copilot CLI en el worktree.
    #>
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Path,
        [string]$Target = 'auto',
        [string]$Title,
        [string]$Color
    )
    return (Open-WtWarpTab -Name $Name -Path $Path -Commands (Get-WtAgentCommands) `
        -Title $Title -Color $Color -Kind 'agent' -Target $Target)
}

function Open-WtTerminalInWarp {
    <#
    .SYNOPSIS
        Abre una terminal comun como tab de Warp: igual que el tab del agente pero
        sin comandos, con titulo y color propios.
    #>
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Path,
        [string]$Target = 'auto',
        [string]$Title,
        [string]$Color
    )
    return (Open-WtWarpTab -Name $Name -Path $Path -Title $Title -Color $Color `
        -Kind 'term' -Target $Target)
}

function Get-WtTabTitle {
    <#
    .SYNOPSIS
        Titulo de tab 'repo > worktree (rama)'. Los tab groups de Warp no son
        scriptables; titulo + color es la aproximacion visual.
    #>
    param(
        [Parameter(Mandatory)][string]$RepoRoot,
        [Parameter(Mandatory)][string]$Name,
        [string]$Branch
    )
    $title = "{0} > {1}" -f (Split-Path -Leaf $RepoRoot), $Name
    if ($Branch -and $Branch -ne $Name) { $title += " ($Branch)" }
    return $title
}

function Format-WtWarpTarget {
    param([bool]$SameWindow)
    if ($SameWindow) { return 'nueva pestana de la ventana activa' }
    return 'ventana nueva de Warp'
}
