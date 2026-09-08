# ---------------------------------------------------------------------------
# Config: defaults, merge por precedencia, lectura/escritura y comando 'config'.
# Depende de: Common (Find-WtRepoConfigFile ubica el .wt.json del repo actual
# caminando el filesystem, sin invocar git ni depender de Repo).
# ---------------------------------------------------------------------------

function Get-WtDefaultConfig {
    <#
    .SYNOPSIS
        Valores por defecto. Se resuelven al invocarse (no al importar el modulo)
        para que warpPath refleje el entorno real del proceso.
    #>
    $localAppData = $env:LOCALAPPDATA
    if (-not $localAppData) { $localAppData = Join-Path $HOME 'AppData\Local' }
    return [ordered]@{
        # Plantilla de ubicacion de worktrees. Tokens: {repoParent} {repo} {name}
        worktreeRootTemplate = '{repoParent}\{repo}.worktrees\{name}'
        reposRoot            = ''          # raiz de los repos git; string o array (ej. 'C:\Repos')
        reposDepth           = 1           # niveles bajo reposRoot para buscar repos (1-3)
        defaultBase          = ''          # ej. 'develop' o 'origin/develop'
        branchPrefix         = ''          # ej. 'agent/' para ramas agent/<name>
        editor               = 'code'      # comando para abrir el editor ('' para desactivar)
        terminal             = 'warp'      # 'warp' | 'wt' | 'none'
        warpAgentTarget      = 'auto'      # 'auto' | 'tab' (misma ventana) | 'window' (ventana nueva)
        warpAgentColor       = 'green'     # color del tab del agente ('' para no colorear)
        warpTerminalColor    = 'blue'      # color del tab de terminal comun ('' para no colorear)
        warpPath             = (Join-Path $localAppData 'Programs\Warp\warp.exe')
        fetchBeforeCreate    = $true
        openOnCreate         = 'all'        # 'all' (editor + agente) | 'editor' | 'none'
        agentCommand         = 'copilot'   # comando que corre el tab del agente
        agentShell           = 'powershell' # 'powershell' | 'bash' | 'none' (solo agentCommand)
        copyOnCreate         = @()         # patrones (glob simple, relativos al repo) a copiar al worktree nuevo
        postCreate           = @()         # comandos a correr en el worktree nuevo, en orden, tras la copia
    }
}

function Get-WtConfigKeys {
    return @((Get-WtDefaultConfig).Keys)
}

function Get-WtKnownWarpColors {
    return @('black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white')
}

function Test-WtConfigValue {
    <#
    .SYNOPSIS
        Devuelve '' si $Value es valido para $Key, o el motivo del rechazo.
    .DESCRIPTION
        Funcion pura reutilizada por Set-WtConfigValue (antes de escribir) y por
        Read-WtConfigFile (al leer cualquier archivo de config). Claves sin regla
        especifica (editor, defaultBase, branchPrefix) y claves desconocidas son
        libres: la restriccion de QUE claves se admiten vive en otro lado
        (Get-WtConfigKeys para el archivo editable, Get-WtRepoConfigAllowedKeys para
        el .wt.json del repo).
    #>
    param([Parameter(Mandatory)][string]$Key, $Value)
    switch ($Key) {
        'worktreeRootTemplate' {
            $v = [string]$Value
            if (-not $v) { return 'no puede estar vacio' }
            if ($v -notmatch '\{name\}') { return 'debe contener el token {name}' }
            return ''
        }
        'terminal' {
            $allowed = @('warp', 'wt', 'none')
            if ($allowed -notcontains [string]$Value) { return "debe ser uno de: $($allowed -join ', ')" }
            return ''
        }
        'warpAgentTarget' {
            $allowed = @('auto', 'tab', 'window')
            if ($allowed -notcontains [string]$Value) { return "debe ser uno de: $($allowed -join ', ')" }
            return ''
        }
        { $_ -in 'warpAgentColor', 'warpTerminalColor' } {
            $v = [string]$Value
            if (-not $v) { return '' }
            $allowed = Get-WtKnownWarpColors
            if ($allowed -notcontains $v) { return "debe ser vacio o uno de: $($allowed -join ', ')" }
            return ''
        }
        'openOnCreate' {
            $allowed = @('all', 'editor', 'none')
            if ($allowed -notcontains [string]$Value) { return "debe ser uno de: $($allowed -join ', ')" }
            return ''
        }
        'agentShell' {
            $allowed = @('powershell', 'bash', 'none')
            if ($allowed -notcontains [string]$Value) { return "debe ser uno de: $($allowed -join ', ')" }
            return ''
        }
        'fetchBeforeCreate' {
            if ($Value -is [bool]) { return '' }
            if (@('true', 'false') -contains [string]$Value) { return '' }
            return 'debe ser true o false'
        }
        'warpPath' {
            $v = [string]$Value
            if (-not $v) { return '' }
            if (-not [IO.Path]::IsPathRooted($v)) { return 'debe ser una ruta absoluta (o vacio)' }
            return ''
        }
        'reposRoot' {
            # String unico (forma de toda la vida) o array (varias raices, B11): cada
            # elemento no vacio tiene que ser una ruta absoluta.
            $roots = @(ConvertTo-WtReposRootList -Value $Value)
            foreach ($root in $roots) {
                if (-not [IO.Path]::IsPathRooted($root)) {
                    return "cada raiz debe ser una ruta absoluta (o vacio): '$root' no lo es"
                }
            }
            return ''
        }
        'reposDepth' {
            $parsed = 0
            if (-not [int]::TryParse([string]$Value, [ref]$parsed) -or $parsed -lt 1 -or $parsed -gt 3) {
                return 'debe ser un numero entero entre 1 y 3'
            }
            return ''
        }
        { $_ -in 'copyOnCreate', 'postCreate' } {
            foreach ($item in @($Value)) {
                if (-not [string]$item) { return 'cada elemento debe ser un string no vacio' }
            }
            return ''
        }
        default { return '' }
    }
}

function Assert-WtConfigValue {
    param([Parameter(Mandatory)][string]$Key, $Value)
    $reason = Test-WtConfigValue -Key $Key -Value $Value
    if ($reason) { throw "Valor invalido para '$Key': $reason." }
}

function Test-WtWorktreeRootTemplateNeedsRepoToken {
    <#
    .SYNOPSIS
        $true si la plantilla no distingue repos entre si (sin {repo} ni {repoParent}):
        dos worktrees homonimos de repos distintos escribirian la misma ruta.
    .DESCRIPTION
        Advertencia, no rechazo: una plantilla asi es valida (tiene {name}) pero
        peligrosa apenas se maneja mas de un repo desde la misma raiz.
    #>
    param([AllowEmptyString()][string]$Value)
    if (-not $Value) { return $false }
    return (-not ($Value -match '\{repo\}' -or $Value -match '\{repoParent\}'))
}

function Get-WtGlobalConfigPath {
    return (Join-Path $HOME '.wt\config.json')
}

function Get-WtConfigFilePath {
    # Archivo editable por comandos: WT_CONFIG (entorno/pruebas) o el global del usuario
    if ($env:WT_CONFIG) { return $env:WT_CONFIG }
    return (Get-WtGlobalConfigPath)
}

function Read-WtConfigFile {
    <#
    .SYNOPSIS
        Lee un JSON de config como ordered hashtable. $null si no existe o es ilegible.
    .DESCRIPTION
        Cada clave se valida con Test-WtConfigValue; las invalidas se descartan con un
        warning que nombra el archivo, la clave y el motivo. Nunca lanza: un archivo de
        config roto (sintacticamente valido pero con un valor invalido) no puede dejar
        el CLI inutilizable.
    #>
    param([Parameter(Mandatory)][AllowEmptyString()][string]$Path)
    if (-not (Test-WtPathExists $Path)) { return $null }
    try {
        $json = Get-Content -Raw -LiteralPath $Path | ConvertFrom-Json
    } catch {
        Write-WtWarn "No se pudo leer la configuracion '$Path': $($_.Exception.Message)"
        return $null
    }
    $data = [ordered]@{}
    if ($null -ne $json) {
        foreach ($prop in $json.PSObject.Properties) {
            $reason = Test-WtConfigValue -Key $prop.Name -Value $prop.Value
            if ($reason) {
                Write-WtWarn "'$Path' define '$($prop.Name)' con un valor invalido ($reason); se ignora."
                continue
            }
            $data[$prop.Name] = $prop.Value
        }
    }
    return $data
}

function Merge-WtConfig {
    <#
    .SYNOPSIS
        Devuelve una copia de $Base con las claves de $Override aplicadas encima.
    .DESCRIPTION
        Funcion pura: es el corazon de la precedencia de configuracion y por eso se
        testea sin tocar el disco.
    #>
    param(
        [Parameter(Mandatory)]$Base,
        $Override
    )
    $merged = [ordered]@{}
    foreach ($k in $Base.Keys) { $merged[$k] = $Base[$k] }
    if ($Override) {
        foreach ($k in $Override.Keys) { $merged[$k] = $Override[$k] }
    }
    return $merged
}

# La config efectiva depende del directorio actual (.wt.json del repo), asi que la
# cache se invalida sola cuando cambia la ubicacion.
$script:ConfigCache = $null

function Clear-WtConfigCache {
    $script:ConfigCache = $null
}

function Get-WtRepoConfigAllowedKeys {
    <#
    .SYNOPSIS
        Claves permitidas en el .wt.json DEL REPO (no en el global del usuario ni en
        WT_CONFIG). Unica fuente de verdad: cualquier clave nueva que resuelva a un
        ejecutable, a un shell o a una ruta fuera del repo queda prohibida por defecto
        a menos que se agregue aca explicitamente.
    #>
    return @('worktreeRootTemplate', 'defaultBase', 'branchPrefix', 'fetchBeforeCreate')
}

function Select-WtRepoConfigKeys {
    <#
    .SYNOPSIS
        Filtra un hashtable de config contra la lista blanca del repo.
    .DESCRIPTION
        Funcion pura: separa las claves admitidas de las rechazadas para que el
        llamador decida como avisar del rechazo (Get-WtConfig usa Write-WtWarn).
    .OUTPUTS
        @{ Config; Rejected } - Config solo con las claves admitidas, Rejected como
        array de nombres de clave descartados.
    #>
    param([AllowEmptyCollection()]$Data)
    $allowed = Get-WtRepoConfigAllowedKeys
    $result = [ordered]@{}
    $rejected = @()
    if ($Data) {
        foreach ($key in $Data.Keys) {
            if ($allowed -contains $key) { $result[$key] = $Data[$key] }
            else { $rejected += $key }
        }
    }
    return @{ Config = $result; Rejected = $rejected }
}

function Get-WtConfig {
    <#
    .SYNOPSIS
        Config efectiva. Precedencia creciente (el ultimo gana por clave):
        defaults < global del usuario < .wt.json del repo < WT_CONFIG.
    .DESCRIPTION
        El .wt.json del repo es el unico candidato filtrado por lista blanca
        (Select-WtRepoConfigKeys): es el unico archivo de config que puede llegar de
        un repo ajeno (clonado), asi que una clave como 'editor' o 'warpPath' ahi no
        puede terminar siendo el ejecutable que 'wt open' lanza.
    .PARAMETER RepoRoot
        Si el llamador ya resolvio la raiz del repo (ej. Find-WtMainRoot), pasarla
        evita que Get-WtConfig la busque por su cuenta (Find-WtRepoConfigFile, que
        camina el filesystem sin invocar git).
    #>
    param([switch]$Refresh, [string]$RepoRoot)
    $cwd = (Get-Location).Path
    if (-not $Refresh -and $script:ConfigCache -and $script:ConfigCache.Cwd -eq $cwd) {
        return $script:ConfigCache.Config
    }

    $config = Get-WtDefaultConfig
    $repoConfigPath = ''
    if ($env:WT_CONFIG_ONLY -eq '1') {
        $candidates = @()
        if ($env:WT_CONFIG) { $candidates += $env:WT_CONFIG }
    } else {
        $candidates = @(Get-WtGlobalConfigPath)
        if ($RepoRoot) {
            $repoConfigPath = Join-Path $RepoRoot '.wt.json'
        } else {
            $repoConfigPath = Find-WtRepoConfigFile
            if (-not $repoConfigPath) { $repoConfigPath = '' }
        }
        if ($repoConfigPath) { $candidates += $repoConfigPath }
        if ($env:WT_CONFIG) { $candidates += $env:WT_CONFIG }
    }

    foreach ($path in $candidates) {
        $data = Read-WtConfigFile -Path $path
        if ($path -eq $repoConfigPath -and $data) {
            $selected = Select-WtRepoConfigKeys -Data $data
            foreach ($key in $selected.Rejected) {
                $template = "'.wt.json' del repo define '{0}', que no se admite por seguridad (se ignora). " +
                    "Claves admitidas: {1}"
                Write-WtWarn ($template -f $key, ((Get-WtRepoConfigAllowedKeys) -join ', '))
            }
            $data = $selected.Config
        }
        $config = Merge-WtConfig -Base $config -Override $data
    }

    $script:ConfigCache = @{ Cwd = $cwd; Config = $config }
    return $config
}

function ConvertTo-WtConfigValue {
    <#
    .SYNOPSIS
        Convierte el string crudo de 'wt config set' al tipo efectivo de la clave.
    .DESCRIPTION
        'copyOnCreate' y 'postCreate' son arrays: se escriben separados por ';'
        (ej. 'wt config set copyOnCreate ".env;certs/*.pfx"') y se guardan como array
        JSON. El resto conserva la conversion de siempre (booleano/entero/string).
    #>
    param([AllowEmptyString()][string]$Key = '', [AllowEmptyString()][string]$Value)
    if ($Key -in 'copyOnCreate', 'postCreate') {
        return @($Value -split ';' | ForEach-Object { $_.Trim() } | Where-Object { $_ })
    }
    if ($Value -match '^(true|false)$') { return ($Value -eq 'true') }
    if ($Value -match '^\d+$') { return [int]$Value }
    return $Value
}

function Set-WtConfigValue {
    <#
    .SYNOPSIS
        Escribe una clave en el archivo editable (WT_CONFIG o el global del usuario).
        Nunca toca los defaults embebidos ni el .wt.json del repo.
    .OUTPUTS
        @{ Path; Value } con la ruta escrita y el valor efectivo (ya convertido).
    #>
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '',
        Justification = 'Comando directo de wt config set; pedir confirmacion en cada valor rompe el flujo de uso.')]
    param(
        [Parameter(Mandatory)][string]$Key,
        [Parameter(Mandatory)][AllowEmptyString()][string]$Value
    )
    $keys = Get-WtConfigKeys
    if ($keys -notcontains $Key) {
        throw "Clave desconocida '$Key'. Claves validas: $($keys -join ', ')"
    }
    $path = Get-WtConfigFilePath
    $data = [ordered]@{}
    if (Test-WtPathExists $path) {
        try {
            $json = Get-Content -Raw -LiteralPath $path | ConvertFrom-Json
            if ($null -ne $json) {
                foreach ($p in $json.PSObject.Properties) { $data[$p.Name] = $p.Value }
            }
        } catch {
            throw "No se pudo leer '$path': $($_.Exception.Message)"
        }
    }
    $effective = ConvertTo-WtConfigValue -Key $Key -Value $Value
    Assert-WtConfigValue -Key $Key -Value $effective
    if ($Key -eq 'worktreeRootTemplate' -and (Test-WtWorktreeRootTemplateNeedsRepoToken -Value ([string]$effective))) {
        Write-WtWarn "worktreeRootTemplate no contiene {repo} ni {repoParent}: worktrees homonimos de repos distintos escribirian la misma ruta."
    }
    $data[$Key] = $effective
    Save-WtConfigFile -Path $path -Data $data
    Clear-WtConfigCache
    return @{ Path = $path; Value = $effective }
}

function Save-WtConfigFile {
    param([Parameter(Mandatory)][string]$Path, [Parameter(Mandatory)]$Data)
    $dir = Split-Path -Parent $Path
    if ($dir -and -not (Test-WtPathExists $dir)) {
        if (Test-WtDryRun) {
            Write-WtDetail "[dry-run] crear directorio $dir"
        } else {
            New-Item -ItemType Directory -Path $dir -Force | Out-Null
        }
    }
    $json = [pscustomobject]$Data | ConvertTo-Json -Depth 5
    Set-WtFileUtf8NoBom -Path $Path -Content $json
}

function ConvertTo-WtConfigDisplayValue {
    <#
    .SYNOPSIS
        Representacion legible de un valor de config. Los arrays ('copyOnCreate',
        'postCreate') se muestran separados por ';', igual a como se escriben con
        'wt config set'.
    #>
    param($Value)
    if ($Value -is [array] -or $Value -is [System.Collections.IEnumerable] -and $Value -isnot [string]) {
        return (@($Value) -join ';')
    }
    return [string]$Value
}

function Invoke-WtConfigCommand {
    param(
        [AllowEmptyString()][string]$Action,
        [AllowEmptyString()][string]$Key,
        [AllowEmptyString()][string]$Value
    )
    $file = Get-WtConfigFilePath
    switch ($Action) {
        { $_ -in '', 'list', 'ls' } {
            $config = Get-WtConfig
            Write-WtDetail ("Archivo editable: {0}" -f $file)
            Write-WtDetail '(valores efectivos; precedencia: WT_CONFIG > .wt.json del repo > archivo global > defaults)'
            $rows = @(foreach ($k in $config.Keys) {
                [pscustomobject]@{ Clave = $k; Valor = (ConvertTo-WtConfigDisplayValue $config[$k]) }
            })
            Write-WtTable -Rows $rows
        }
        'path' { Write-WtLine $file }
        'get' {
            if (-not $Key) { throw 'Uso: wt config get <clave>' }
            $config = Get-WtConfig
            if (-not $config.Contains($Key)) { throw "Clave desconocida '$Key'." }
            Write-WtLine (ConvertTo-WtConfigDisplayValue $config[$Key])
        }
        'set' {
            if (-not $Key) { throw 'Uso: wt config set <clave> <valor>' }
            $written = Set-WtConfigValue -Key $Key -Value $Value
            Write-WtSuccess ("OK - '{0}' = '{1}' (guardado en {2})" -f $Key, (ConvertTo-WtConfigDisplayValue $written.Value), $written.Path)
        }
        'edit' {
            if (-not (Test-WtPathExists $file)) {
                Save-WtConfigFile -Path $file -Data (Get-WtDefaultConfig)
                Write-WtSuccess "Config creada en $file"
                Clear-WtConfigCache
            }
            $editor = [string](Get-WtConfig).editor
            if (-not (Test-WtCommand $editor)) { $editor = 'notepad.exe' }
            Start-WtProcess -FilePath $editor -ArgumentList (Format-WtProcessArgument $file)
            Write-WtSuccess "Editando $file"
        }
        default { throw "Accion de config desconocida: '$Action'. Usa: wt config [list|get|set|path|edit]" }
    }
}
