# ---------------------------------------------------------------------------
# Cli: parsing de argumentos, ayuda y dispatcher.
# Depende de: todo lo anterior. Nadie depende de este archivo.
# ---------------------------------------------------------------------------

# Especificacion declarativa de cada comando: alias, flags booleanos y flags con
# valor. Es la unica fuente de verdad del parser y de la validacion de flags.
function Get-WtCommandSpecs {
    return [ordered]@{
        'create'  = @{ Aliases = @();                    Flags = @('no-open', 'all', 'code', 'agent', 'terminal', 'no-hooks'); Values = @('base', 'branch') }
        'list'    = @{ Aliases = @('ls');                Flags = @('json');                                                   Values = @() }
        'open'    = @{ Aliases = @();                    Flags = @('code', 'terminal', 'agent', 'all');                       Values = @() }
        'path'    = @{ Aliases = @();                    Flags = @();                                                         Values = @() }
        'remove'  = @{ Aliases = @('rm');                Flags = @('delete-branch', 'force', 'force-branch');                 Values = @() }
        'lock'    = @{ Aliases = @();                    Flags = @();                                                         Values = @('reason') }
        'unlock'  = @{ Aliases = @();                    Flags = @();                                                         Values = @() }
        'prune'   = @{ Aliases = @();                    Flags = @();                                                         Values = @() }
        'clean'   = @{ Aliases = @();                    Flags = @();                                                         Values = @() }
        'status'  = @{ Aliases = @('st');                Flags = @('json', 'fetch');                                          Values = @() }
        'exec'    = @{ Aliases = @();                    Flags = @();                                                         Values = @() }
        'each'    = @{ Aliases = @();                    Flags = @('continue-on-error', 'json');                              Values = @() }
        'sync'    = @{ Aliases = @();                    Flags = @('continue-on-error');                                      Values = @('base', 'strategy') }
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
        - Un '--' suelto corta el parseo ahi: todo lo que sigue va crudo a
          'Rest', sin interpretarse como flags (lo usan 'exec'/'each' para separar
          el comando externo de los propios flags de wt).
    .OUTPUTS
        @{ Command; Positional; Flags (hashtable de bool); Values (hashtable de
        string); Rest (array de string, lo que sigue a un '--' suelto) }
    #>
    param([AllowEmptyCollection()][string[]]$Arguments)
    $result = @{ Command = ''; Positional = @(); Flags = @{}; Values = @{}; Rest = @() }
    if (-not $Arguments -or $Arguments.Count -eq 0) { return $result }

    $command = Resolve-WtCommandName -Token $Arguments[0]
    $spec = (Get-WtCommandSpecs)[$command]
    $result.Command = $command

    $rest = @($Arguments | Select-Object -Skip 1)
    for ($i = 0; $i -lt $rest.Count; $i++) {
        $token = $rest[$i]
        if ($token -eq '--') {
            $result.Rest = @($rest | Select-Object -Skip ($i + 1))
            break
        }
        if (-not ($token -match '^--?([^-].*)$')) {
            $result.Positional += $token
            continue
        }
        $key = $Matches[1].ToLowerInvariant()
        if ($key -eq 'dry-run') {
            # Global: valido para cualquier comando (item 4), no forma parte de la
            # spec de cada uno.
            $result.Flags[$key] = $true
            continue
        }
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
             [--all|--code|--agent|--terminal] [--no-open] [--no-hooks]
  wt list [--json]                           (alias: ls)
  wt open [<nombre>] [--code] [--terminal] [--agent] [--all]
  wt path [<nombre>]
  wt remove <nombre> [--delete-branch] [--force] [--force-branch]   (alias: rm)
  wt lock <nombre> [--reason <texto>]
  wt unlock <nombre>
  wt prune
  wt clean                                    # borra tab configs de Warp huerfanos
  wt status [<nombre>] [--json] [--fetch]     # estado de uno o todos los worktrees (alias: st)
  wt exec <nombre> -- <comando...>            # corre <comando> en ese worktree
  wt each [--continue-on-error] [--json] -- <comando...>   # corre <comando> en todos
  wt sync [<nombre>] [--base <rama>] [--strategy rebase|merge] [--continue-on-error]

  'create' usa el nombre tambien como rama (mas branchPrefix); --branch la cambia.
  Antes de abrir nada corren los hooks de creacion: copia 'copyOnCreate' y ejecuta
  'postCreate' en el worktree nuevo (--no-hooks los saltea). Que abre al terminar lo
  decide 'openOnCreate' ('all' = editor + agente, el default; 'editor'; 'none');
  --all/--code/--agent/--terminal fuerzan un plan especifico (ganan a la config) y
  --no-open gana a todo (crea sin abrir nada).

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

  'wt status' sin nombre muestra todos los worktrees del repo actual: rama, archivos
  staged/modified/sin trackear, ahead/behind contra el upstream (o 'defaultBase' si no
  tiene) y el ultimo commit. --fetch actualiza contra el remoto antes de calcular; sin
  el flag nunca toca la red. Un worktree que falla (bloqueado, disco desconectado) se
  reporta con su error en la fila, sin romper el resto.

  'wt exec'/'wt each' corren un comando externo tal cual, sin que wt interprete sus
  flags: todo lo que sigue a un '--' suelto va crudo al comando. 'each' lo corre en
  todos los worktrees del repo (menos el principal y los obsoletos), en serie; sin
  --continue-on-error corta en el primer fallo, con el flag sigue y resume al final
  que worktrees fallaron. Exit 2 si alguno fallo.

  'wt sync' rebasa (default) o mergea (--strategy merge) cada worktree contra su base
  (--base explicito > upstream de la rama > defaultBase). Un worktree sucio se saltea
  siempre (nunca hace stash automatico); un conflicto se reporta y el worktree queda
  como git lo dejo, sin resolverlo ni abortarlo solo. Un solo 'git fetch' al inicio.

WORKSPACE (funcionan desde cualquier directorio):
  wt repos [--json]                          # lista los repos git de reposRoot
  wt cd [<repo>|<worktree>] [--open]          # va a reposRoot, a un repo, o a un worktree suyo
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

AUTOCOMPLETADO: install.ps1 registra el autocompletado de comandos, flags, claves y
valores de config (TAB en 'wt <TAB>', 'wt open --<TAB>', 'wt config set terminal <TAB>')
y nombres de worktree/repo en la 2da posicion de open/path/remove/cd/lock/unlock.

CODIGOS DE SALIDA: 0 OK; 1 error de uso (comando/flag desconocido, argumento
faltante); 2 error de git o del entorno (worktree/repo inexistente, valor de config
invalido, etc.).

--dry-run: valido para CUALQUIER comando. Ningun comando externo se ejecuta y ningun
archivo se escribe; cada operacion se imprime con el prefijo '[dry-run]'. Las lecturas
(git rev-parse, worktree list, show-ref, ...) se siguen ejecutando de verdad. Exit 0
si el ensayo no encuentra errores de uso.

CONFIG (editable con 'wt config set', 'wt config edit', o a mano en
%USERPROFILE%\.wt\config.json o .wt.json en la raiz del repo):
  worktreeRootTemplate  Plantilla de rutas: {repoParent} {repo} {name}
  reposRoot             Raiz de los repos git para 'repos' y 'cd'; string o array (varias raices)
  reposDepth            Niveles bajo reposRoot para buscar repos: 1 (default) a 3
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
  copyOnCreate          Patrones (';'-separados) a copiar del repo al worktree nuevo
  postCreate            Comandos (';'-separados) a correr en el worktree nuevo tras la copia
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
        # Set-WtDryRun se resetea SIEMPRE en el finally: si quedara encendida entre
        # invocaciones dentro del mismo proceso (modo instalado, el perfil importa el
        # modulo una sola vez) todo comando posterior seria un no-op silencioso.
        Set-WtDryRun -Value (Test-WtFlag -Parsed $parsed -Key 'dry-run')
        Invoke-WtDispatch -Parsed $parsed
        $global:LASTEXITCODE = 0
    } catch {
        $message = $_.Exception.Message
        $exitCode = 2
        if ($message.StartsWith('Uso:')) { $exitCode = 1 }
        Write-WtError $message
        $global:LASTEXITCODE = $exitCode
    } finally {
        Set-WtDryRun -Value $false
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
                -NoHooks:(Test-WtFlag -Parsed $parsed -Key 'no-hooks') `
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
        'status' {
            Invoke-WtStatus -Name (Get-WtPositional -Parsed $parsed -Index 0) `
                -Json:(Test-WtFlag -Parsed $parsed -Key 'json') `
                -Fetch:(Test-WtFlag -Parsed $parsed -Key 'fetch')
        }
        'exec' {
            $name = Get-WtPositional -Parsed $parsed -Index 0
            if (-not $name -or @($parsed.Rest).Count -eq 0) { throw 'Uso: wt exec <nombre> -- <comando...>' }
            Invoke-WtExec -Name $name -CommandArgs $parsed.Rest
        }
        'each' {
            if (@($parsed.Rest).Count -eq 0) { throw 'Uso: wt each [--continue-on-error] [--json] -- <comando...>' }
            Invoke-WtEach -CommandArgs $parsed.Rest `
                -ContinueOnError:(Test-WtFlag -Parsed $parsed -Key 'continue-on-error') `
                -Json:(Test-WtFlag -Parsed $parsed -Key 'json')
        }
        'sync' {
            $strategy = Get-WtValue -Parsed $parsed -Key 'strategy'
            if (-not $strategy) { $strategy = 'rebase' }
            Invoke-WtSync -Name (Get-WtPositional -Parsed $parsed -Index 0) `
                -Base (Get-WtValue -Parsed $parsed -Key 'base') `
                -Strategy $strategy `
                -ContinueOnError:(Test-WtFlag -Parsed $parsed -Key 'continue-on-error')
        }
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
