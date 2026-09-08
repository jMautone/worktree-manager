# ---------------------------------------------------------------------------
# Cli: parsing de argumentos, ayuda y dispatcher.
# Depende de: todo lo anterior. Nadie depende de este archivo.
# ---------------------------------------------------------------------------

# Especificacion declarativa de cada comando: alias, flags booleanos y flags con
# valor. Es la unica fuente de verdad del parser y de la validacion de flags.
function Get-WtCommandSpecs {
    return [ordered]@{
        'create'  = @{ Aliases = @();                    Flags = @('no-open', 'all', 'code', 'agent', 'terminal');           Values = @('base', 'branch') }
        'list'    = @{ Aliases = @('ls');                Flags = @('json');                                                   Values = @() }
        'open'    = @{ Aliases = @();                    Flags = @('code', 'terminal', 'agent', 'all');                       Values = @() }
        'path'    = @{ Aliases = @();                    Flags = @();                                                         Values = @() }
        'remove'  = @{ Aliases = @('rm');                Flags = @('delete-branch', 'force', 'force-branch');                 Values = @() }
        'lock'    = @{ Aliases = @();                    Flags = @();                                                         Values = @('reason') }
        'unlock'  = @{ Aliases = @();                    Flags = @();                                                         Values = @() }
        'prune'   = @{ Aliases = @();                    Flags = @();                                                         Values = @() }
        'clean'   = @{ Aliases = @();                    Flags = @();                                                         Values = @() }
        'repos'   = @{ Aliases = @('repos-list');        Flags = @('json');                                                   Values = @() }
        'cd'      = @{ Aliases = @('go');                Flags = @('open');                                                   Values = @() }
        'config'  = @{ Aliases = @();                    Flags = @();                                                         Values = @() }
        'doctor'  = @{ Aliases = @();                    Flags = @();                                                         Values = @() }
        'console' = @{ Aliases = @('ui', 'menu');        Flags = @();                                                         Values = @() }
        'version' = @{ Aliases = @('--version', '-v');   Flags = @();                                                         Values = @() }
        'help'    = @{ Aliases = @('--help', '-h', '/?'); Flags = @();                                                        Values = @() }
    }
}

function Resolve-WtCommandName {
    <#
    .SYNOPSIS
        Nombre canonico de un comando (resuelve alias). Falla si no existe.
    #>
    param([Parameter(Mandatory)][AllowEmptyString()][string]$Token)
    $name = $Token.ToLowerInvariant()
    $specs = Get-WtCommandSpecs
    if ($specs.Contains($name)) { return $name }
    foreach ($key in $specs.Keys) {
        if ($specs[$key].Aliases -contains $name) { return $key }
    }
    throw "Comando desconocido: '$name'. Usa 'wt help'."
}

function ConvertFrom-WtArgs {
    <#
    .SYNOPSIS
        Parsea la linea de comandos. Funcion pura y testeable.
    .DESCRIPTION
        - Resuelve alias al nombre canonico.
        - Los flags con valor (--base, --branch) exigen un valor; antes un valor
          faltante se convertia silenciosamente en el string 'True'.
        - Los flags no reconocidos por el comando se rechazan, para que un typo
          (--agente) no pase inadvertido.
    .OUTPUTS
        @{ Command; Positional; Flags (hashtable de bool); Values (hashtable de string) }
    #>
    param([AllowEmptyCollection()][string[]]$Arguments)
    $result = @{ Command = ''; Positional = @(); Flags = @{}; Values = @{} }
    if (-not $Arguments -or $Arguments.Count -eq 0) { return $result }

    $command = Resolve-WtCommandName -Token $Arguments[0]
    $spec = (Get-WtCommandSpecs)[$command]
    $result.Command = $command

    $rest = @($Arguments | Select-Object -Skip 1)
    for ($i = 0; $i -lt $rest.Count; $i++) {
        $token = $rest[$i]
        if (-not ($token -match '^--?([^-].*)$')) {
            $result.Positional += $token
            continue
        }
        $key = $Matches[1].ToLowerInvariant()
        if ($spec.Values -contains $key) {
            $next = $null
            if ($i + 1 -lt $rest.Count) { $next = $rest[$i + 1] }
            if ($null -eq $next -or $next -match '^--?[^-]') {
                throw "El flag '--$key' de 'wt $command' requiere un valor. Ej: wt $command <nombre> --$key <valor>"
            }
            $result.Values[$key] = $next
            $i++
            continue
        }
        if ($spec.Flags -contains $key) {
            $result.Flags[$key] = $true
            continue
        }
        $known = @(@($spec.Flags) + @($spec.Values | ForEach-Object { "$_ <valor>" }))
        $valid = 'ninguno'
        if ($known.Count -gt 0) { $valid = (($known | ForEach-Object { "--$_" }) -join ', ') }
        throw "Flag desconocido '--$key' para 'wt $command'. Flags validos: $valid"
    }
    return $result
}

function Get-WtPositional {
    param([Parameter(Mandatory)]$Parsed, [Parameter(Mandatory)][int]$Index)
    if ($Parsed.Positional.Count -gt $Index) { return [string]$Parsed.Positional[$Index] }
    return ''
}

function Get-WtValue {
    param([Parameter(Mandatory)]$Parsed, [Parameter(Mandatory)][string]$Key)
    if ($Parsed.Values.ContainsKey($Key)) { return [string]$Parsed.Values[$Key] }
    return ''
}

function Test-WtFlag {
    param([Parameter(Mandatory)]$Parsed, [Parameter(Mandatory)][string]$Key)
    return $Parsed.Flags.ContainsKey($Key)
}

function Show-WtHelp {
    @'
Worktree Manager (wt) - gestiona Git Worktrees para agentes en paralelo

WORKTREES:
  wt create <nombre> [--base <rama>] [--branch <rama>]
             [--all|--code|--agent|--terminal] [--no-open]
  wt list [--json]                           (alias: ls)
  wt open [<nombre>] [--code] [--terminal] [--agent] [--all]
  wt path [<nombre>]
  wt remove <nombre> [--delete-branch] [--force] [--force-branch]   (alias: rm)
  wt lock <nombre> [--reason <texto>]
  wt unlock <nombre>
  wt prune
  wt clean                                    # borra tab configs de Warp huerfanos

  'create' usa el nombre tambien como rama (mas branchPrefix); --branch la cambia.
  Que abre al terminar lo decide 'openOnCreate' ('all' = editor + agente, el
  default; 'editor'; 'none'); --all/--code/--agent/--terminal fuerzan un plan
  especifico (ganan a la config) y --no-open gana a todo (crea sin abrir nada).

  'open' sin nombre abre el checkout actual (el worktree si estas dentro de uno).
  Sin flags abre solo el editor; --all = editor + agente. La terminal solo se abre
  con --terminal. Fuera de un repo, el nombre puede ser un repo de reposRoot o un
  worktree de alguno de ellos: wt entra al repo y abre ahi.

  'remove --delete-branch' borra la rama con 'git branch -d' (seguro): si tiene commits
  sin mergear, aborta sin borrarla (el worktree si se elimina) y sugiere --force-branch
  para forzarlo con 'git branch -D'. --force es independiente: solo fuerza el borrado
  del worktree ('git worktree remove --force', arbol de trabajo sucio) y nunca implica
  --force-branch.

  'wt lock'/'wt unlock' bloquean o desbloquean un worktree ('git worktree lock/unlock');
  un worktree bloqueado se marca en 'wt list' y en la consola, y 'wt remove' lo rechaza
  con un mensaje que sugiere 'wt unlock' primero.

WORKSPACE (funcionan desde cualquier directorio):
  wt repos [--json]                          # lista los repos git de reposRoot
  wt cd [<repo>] [--open]                    # va a reposRoot o a un repo (match por prefijo)
  wt config [list]                           # muestra la config efectiva
  wt config get <clave>                      # muestra un valor
  wt config set <clave> <valor>              # guarda un valor en el archivo global
  wt config path | edit                      # ruta del archivo / abrirlo en el editor
  wt doctor                                  # chequea el setup (git, node, copilot, warp, ...)
  wt console                                 # menu interactivo que arma los comandos (alias: ui, menu)
  wt version                                 # version del modulo y de PowerShell
  wt help

EJEMPLOS:
  wt config set reposRoot C:\Repos
  wt repos
  wt cd MiRepo
  wt cd mirepo --open                        # match por prefijo + abre el editor ahi
  wt create exceptions --base develop
  wt open                                    # abre el checkout actual en el editor
  wt open logging --agent                    # abre solo el agente en ese worktree
  wt open logging --all                      # editor + agente (la terminal solo con --terminal)
  wt open MiRepo --agent                     # funciona tambien fuera del repo
  wt remove logging --delete-branch
  wt doctor

NOTA: 'wt cd' cambia el directorio de TU terminal porque la funcion 'wt' del perfil
corre en el mismo proceso. Si invocas wt.ps1 con -File, el cd solo afecta a ese proceso.

CODIGOS DE SALIDA: 0 OK; 1 error de uso (comando/flag desconocido, argumento
faltante); 2 error de git o del entorno (worktree/repo inexistente, valor de config
invalido, etc.).

CONFIG (editable con 'wt config set', 'wt config edit', o a mano en
%USERPROFILE%\.wt\config.json o .wt.json en la raiz del repo):
  worktreeRootTemplate  Plantilla de rutas: {repoParent} {repo} {name}
  reposRoot             Raiz de los repos git (ej. C:\Repos) para 'repos' y 'cd'
  defaultBase           Base por defecto para 'create' (ej. origin/develop)
  branchPrefix          Prefijo para ramas nuevas (ej. agent/)
  openOnCreate          Que abre 'create' sin flags: 'all' (editor + agente) | 'editor' | 'none'
  editor                Comando del editor ('code', '' para desactivar)
  terminal              'warp' | 'wt' | 'none'
  warpAgentTarget       'auto' | 'tab' (misma ventana) | 'window' (ventana nueva)
  warpAgentColor        Color del tab del agente (green, cyan, ...; '' sin color)
  warpTerminalColor     Color del tab de terminal comun (blue por defecto; '' sin color)
  warpPath              Ruta a warp.exe
  fetchBeforeCreate     Hace fetch del remoto antes de crear desde <remote>/<rama>
  agentCommand          Comando del agente que corre el tab (default: copilot)
  agentShell            'powershell' | 'bash' | 'none' (sin comandos auxiliares, solo agentCommand)
'@ | Write-Host
}

function Invoke-Wt {
    <#
    .SYNOPSIS
        Punto de entrada del CLI: parsea y despacha. Sin logica de negocio.
    .DESCRIPTION
        Codigos de salida: 0 OK; 1 error de uso (comando o flag invalido, argumento
        faltante); 2 error de git o del entorno (worktree/repo inexistente, ambiguo,
        git worktree remove fallido, etc.). Setea $global:LASTEXITCODE en los dos
        modos de invocacion: la funcion del perfil (que no llama a 'exit', para no
        cerrar la terminal) y wt.ps1 -File (que si hace 'exit $LASTEXITCODE' al final).
    #>
    param([Parameter(ValueFromRemainingArguments)][Alias('Args')][string[]]$Arguments)

    Clear-WtConfigCache
    Clear-WtWorktreesCache

    try {
        $parsed = ConvertFrom-WtArgs -Arguments $Arguments
    } catch {
        # Cualquier error de ConvertFrom-WtArgs (comando o flag desconocido, flag sin
        # valor) es por definicion un problema de sintaxis de la linea de comandos.
        Write-WtError $_.Exception.Message
        $global:LASTEXITCODE = 1
        return
    }
    if (-not $parsed.Command) {
        Show-WtHelp
        $global:LASTEXITCODE = 0
        return
    }

    try {
        Invoke-WtDispatch -Parsed $parsed
        $global:LASTEXITCODE = 0
    } catch {
        $message = $_.Exception.Message
        $exitCode = 2
        if ($message.StartsWith('Uso:')) { $exitCode = 1 }
        Write-WtError $message
        $global:LASTEXITCODE = $exitCode
    }
}

function Invoke-WtDispatch {
    <#
    .SYNOPSIS
        Despacho por comando. Separado de Invoke-Wt para que el try/catch del
        contrato de codigos de salida (B6) envuelva un solo punto.
    #>
    param([Parameter(Mandatory)]$Parsed)
    $parsed = $Parsed

    switch ($parsed.Command) {
        'create' {
            $name = Get-WtPositional -Parsed $parsed -Index 0
            if (-not $name) { throw 'Uso: wt create <nombre> [--base <rama>]' }
            New-WtWorktree -Name $name `
                -Base (Get-WtValue -Parsed $parsed -Key 'base') `
                -Branch (Get-WtValue -Parsed $parsed -Key 'branch') `
                -Code:(Test-WtFlag -Parsed $parsed -Key 'code') `
                -Terminal:(Test-WtFlag -Parsed $parsed -Key 'terminal') `
                -Agent:(Test-WtFlag -Parsed $parsed -Key 'agent') `
                -All:(Test-WtFlag -Parsed $parsed -Key 'all') `
                -NoOpen:(Test-WtFlag -Parsed $parsed -Key 'no-open') `
            | Out-Null
            Clear-WtWorktreesCache
        }
        'list' {
            Get-WtWorktreeList -Json:(Test-WtFlag -Parsed $parsed -Key 'json')
        }
        'open' {
            Open-WtWorktree -Name (Get-WtPositional -Parsed $parsed -Index 0) `
                -Code:(Test-WtFlag -Parsed $parsed -Key 'code') `
                -Terminal:(Test-WtFlag -Parsed $parsed -Key 'terminal') `
                -Agent:(Test-WtFlag -Parsed $parsed -Key 'agent') `
                -All:(Test-WtFlag -Parsed $parsed -Key 'all')
        }
        'path' {
            Invoke-WtPathCommand -Name (Get-WtPositional -Parsed $parsed -Index 0)
        }
        'remove' {
            $name = Get-WtPositional -Parsed $parsed -Index 0
            if (-not $name) { throw 'Uso: wt remove <nombre> [--delete-branch] [--force] [--force-branch]' }
            Remove-WtWorktree -Name $name `
                -DeleteBranch:(Test-WtFlag -Parsed $parsed -Key 'delete-branch') `
                -Force:(Test-WtFlag -Parsed $parsed -Key 'force') `
                -ForceBranch:(Test-WtFlag -Parsed $parsed -Key 'force-branch')
            Clear-WtWorktreesCache
        }
        'lock' {
            $name = Get-WtPositional -Parsed $parsed -Index 0
            if (-not $name) { throw 'Uso: wt lock <nombre> [--reason <texto>]' }
            Invoke-WtLock -Name $name -Reason (Get-WtValue -Parsed $parsed -Key 'reason')
            Clear-WtWorktreesCache
        }
        'unlock' {
            $name = Get-WtPositional -Parsed $parsed -Index 0
            if (-not $name) { throw 'Uso: wt unlock <nombre>' }
            Invoke-WtUnlock -Name $name
            Clear-WtWorktreesCache
        }
        'prune' { Invoke-WtPrune; Clear-WtWorktreesCache }
        'clean' { Invoke-WtClean }
        'repos' { Get-WtRepoList -Json:(Test-WtFlag -Parsed $parsed -Key 'json') }
        'cd' {
            Invoke-WtCd -Name (Get-WtPositional -Parsed $parsed -Index 0) `
                -Open:(Test-WtFlag -Parsed $parsed -Key 'open')
        }
        'config' {
            # El valor puede contener espacios (ej. una plantilla con una ruta larga),
            # asi que se rearma con todos los positionales restantes.
            $value = ''
            if ($parsed.Positional.Count -gt 2) {
                $value = (@($parsed.Positional | Select-Object -Skip 2) -join ' ')
            }
            Invoke-WtConfigCommand -Action (Get-WtPositional -Parsed $parsed -Index 0) `
                -Key (Get-WtPositional -Parsed $parsed -Index 1) `
                -Value $value
        }
        'doctor' { Invoke-WtDoctor }
        'console' { Start-WtConsole }
        'version' { Invoke-WtVersion }
        'help' { Show-WtHelp }
        default { throw "Comando desconocido: '$($parsed.Command)'. Usa 'wt help'." }
    }
}
