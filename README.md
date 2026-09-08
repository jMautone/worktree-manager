# Worktree Manager (`wt`)

Herramienta local para Windows que gestiona **Git Worktrees** y facilita la ejecución
paralela de agentes de **Copilot CLI**, abriendo cada worktree en **VS Code** y **Warp**
con un solo comando.

## Instalación

```powershell
powershell -ExecutionPolicy Bypass -File C:\worktree-manager\install.ps1
```

El instalador es **idempotente**: agrega/actualiza un bloque delimitado en
`$PROFILE.CurrentUserAllHosts` que **importa el módulo una sola vez** (al abrir la
terminal) y define la función `wt` (y alias `wtm`) como un wrapper liviano sobre
`Invoke-Wt`, ya cargado; crea `%USERPROFILE%\.wt\config.json` a partir de
`config.example.json` solo si no existe. Luego reiniciá la terminal o ejecutá
`. $PROFILE`. Soporta `-WhatIf` (no modifica nada, solo informa qué haría) y
`-Confirm` (pide confirmación antes de cada escritura: perfil, config).

### Desinstalación

```powershell
powershell -ExecutionPolicy Bypass -File C:\worktree-manager\uninstall.ps1
```

Quita únicamente el bloque delimitado del perfil (el resto queda intacto) y **no**
toca `%USERPROFILE%\.wt\config.json` salvo que se pase `-RemoveConfig`. También
soporta `-WhatIf`/`-Confirm`.

```powershell
powershell -ExecutionPolicy Bypass -File C:\worktree-manager\uninstall.ps1 -RemoveConfig
```

## Uso

```powershell
wt create exceptions --base develop          # worktree + rama 'exceptions'; abre editor + agente (openOnCreate)
wt create logging --base origin/develop      # hace fetch del remoto y crea desde origin/develop
wt create hotfix --branch fix/urgente        # nombre de carpeta y de rama distintos
wt create tmp --code                         # crea y abre solo el editor (el flag gana a openOnCreate)
wt create tmp2 --no-open                     # crea sin abrir nada (gana a todo)
wt list                                      # tabla de worktrees (el principal va marcado)
wt list --json                               # salida JSON para scripting
wt open                                      # abre el checkout actual en VS Code (el worktree si estás dentro de uno)
wt open logging                              # abre el worktree en VS Code
wt open logging --agent                      # abre solo el agente Copilot CLI en el worktree
wt open logging --code                       # abre solo el editor
wt open logging --terminal                   # abre solo la terminal (tab de Warp)
wt open logging --all                        # editor + agente (la terminal solo con --terminal)
wt open MiRepo --agent                       # funciona fuera del repo: wt entra al repo y abre ahí
wt path logging                              # imprime la ruta (útil para cd (wt path logging))
wt path                                      # sin nombre: ruta del checkout actual
wt remove logging                            # elimina el worktree (conserva la rama)
wt remove logging --delete-branch            # elimina worktree y rama (borrado seguro: git branch -d)
wt remove logging --delete-branch --force-branch  # fuerza el borrado de rama aunque tenga commits sin mergear
wt remove logging --force                    # fuerza aunque haya cambios sin commit
wt lock logging --reason "revision en curso"  # bloquea el worktree (git worktree lock)
wt unlock logging                            # lo desbloquea
wt prune                                     # depura metadatos de worktrees huérfanos
wt clean                                     # depura tab configs de Warp huérfanos
wt console                                   # menú interactivo que arma los comandos sin escribirlos
wt version                                   # versión del módulo (wt.psd1) y de PowerShell
wt help
```

Los nombres se resuelven por **nombre de carpeta** o por **nombre de rama**. La
carpeta se compara **sin distinguir mayúsculas** (Windows tampoco lo hace en el
sistema de archivos: `Feature-A` y `feature-a` serían el mismo directorio); la rama
se compara **distinguiendo mayúsculas**, porque git también las distingue
(`Feature-A` y `feature-a` son ramas distintas) — es intencional, no un descuido.
Hay alias para los comandos más usados: `ls` (`list`), `rm` (`remove`), `go` (`cd`),
`ui`/`menu` (`console`) y `repos-list` (`repos`).

Los flags se validan contra el comando: uno desconocido (`--agente`) o un flag con
valor sin valor (`--base` al final de la línea) fallan con un mensaje que dice qué se
esperaba, en vez de ignorarse en silencio.

### Resolución inteligente de `open` y `path`

- **Sin nombre** (dentro de un repo): operan sobre el **checkout actual** — si estás
  dentro de un worktree, abren ese worktree; si no, la raíz principal del repo.
- **Fuera de un repo**: el nombre se busca en `reposRoot` — primero como repo (exacto
  o prefijo único), después como worktree de cualquier repo de la raíz. Si el nombre
  identificó un **repo**, se opera sobre su checkout principal. `wt open` entra al
  repo encontrado (el `cd` queda efectivo en tu terminal) y abre ahí; `wt path` **nunca**
  cambia el directorio actual, solo imprime la ruta — resuelve igual, pero sin el
  efecto secundario, para que sea seguro de usar en scripts o en `cd (wt path ...)`.
- **Worktrees obsoletos**: si el directorio de un worktree se borró a mano, `wt list`
  y la consola lo marcan como `(obsoleto)`, `open`/`path`/`remove` fallan con un
  mensaje que sugiere `wt prune`, y `wt doctor` lo avisa. Si el directorio actual es
  un worktree **huérfano** (su `.git` apunta a un repo principal que ya no existe),
  el error explica cómo repararlo con `git worktree repair` o eliminarlo.

### Flags de `wt open`

Todos opcionales y combinables: `--code` (editor), `--agent` (Copilot CLI en tab de
Warp), `--terminal` (terminal en tab de Warp) y `--all` = **editor + agente**.
**Sin flags** abre solo el editor. **La terminal nunca se abre sola**: solo cuando
se pide explícitamente con `--terminal` (así `--all` no duplica terminales: el agente
ya es una terminal con Copilot corriendo).

## Workspace: raíz de repos

Los comandos de workspace funcionan **desde cualquier directorio** (no requieren estar
en un repo) y giran alrededor de `reposRoot`, la carpeta donde vivís tus repos git
(ej. `C:\Repos`):

```powershell
wt config set reposRoot C:\Repos             # se configura una vez por PC
wt repos                                     # tabla con los repos git de la raíz
wt repos --json                              # salida JSON para scripting
wt cd                                        # va a la raíz de repos
wt cd MiRepo                                 # va al repo (match exacto, case-insensitive)
wt cd mire                                   # match por prefijo único
wt cd MiRepo --open                          # además abre el editor ahí
wt cd logging                                # si 'logging' es un worktree (no un repo), va A ESE worktree
```

`wt cd` cambia el directorio **de tu terminal**: funciona porque la función `wt` que
instala `install.ps1` corre en el mismo proceso de PowerShell. Los repos se detectan
como subdirectorios de `reposRoot` que contienen `.git`, hasta `reposDepth` niveles
(default `1`, solo subdirectorios inmediatos). Igual que `open`/`path` fuera de un
repo, el nombre se busca primero como repo y después como worktree de cualquiera de
ellos; si matchea un worktree, el destino es **su checkout**, no la raíz del repo que
lo contiene (antes `wt cd` solo llegaba a repos; para un worktree había que usar
`cd (wt path logging)`, que sigue funcionando igual). Un nombre que matchea en más de
un repo o worktree es una ambigüedad real y falla listando las coincidencias (mismo
criterio que para worktrees homónimos entre repos).

### Varias raíces y organizaciones anidadas

Si tus repos viven como `C:\Repos\<org>\<repo>` (una carpeta intermedia por
organización), `reposDepth = 2` los hace visibles. `reposRoot` también acepta varias
raíces a la vez (editando el JSON a mano, `wt config set` solo administra un valor por
vez):

```json
{ "reposRoot": ["C:\\Repos", "D:\\OtrosRepos"], "reposDepth": 2 }
```

Con más de una raíz, `wt repos` y `--json` suman una columna/campo `Raiz` para decir
de cuál sale cada repo; un nombre que matchea en varias (misma raíz o distintas) es una
ambigüedad real y `wt cd`/`wt open` fallan listando las coincidencias (mismo criterio
que para worktrees homónimos entre repos). `wt cd` sin nombre va a la **primera** raíz
configurada. La búsqueda corta la rama al encontrar un `.git` (un repo no contiene
repos), pero con `reposDepth > 1` puede alcanzar la carpeta `<repo>.worktrees` de un
repo vecino si vive al mismo nivel que las organizaciones — sus worktrees también
tienen `.git` propio y aparecerían como "repos"; mantené `reposRoot` apuntando por
encima de esa estructura, no directamente a una carpeta que ya mezcla repos y sus
worktrees, para evitarlo.

## Configuración por comandos

Toda la config se puede editar a mano en el archivo JSON o con comandos:

```powershell
wt config                                    # tabla con los valores efectivos
wt config get reposRoot                      # muestra un valor
wt config set reposRoot C:\Repos             # guarda en el archivo global
wt config set terminal wt                    # usar Windows Terminal en vez de Warp
wt config set editor ''                      # desactivar el editor (setup solo Warp + Copilot)
wt config path                               # ruta del archivo que se edita
wt config edit                               # lo abre en el editor (o notepad)
```

`wt config set` valida la clave contra las conocidas, convierte `true`/`false` a
booleanos y escribe en `%USERPROFILE%\.wt\config.json` (o en el archivo de
`WT_CONFIG` si está definido). Así, lo que cambia entre PCs (rutas, editor, terminal)
queda configurable sin tocar el código.

Además del nombre de la clave, el **valor** se valida contra las reglas de cada una
(`terminal` ∈ `warp`/`wt`/`none`, colores contra los conocidos de Warp, rutas como
`reposRoot`/`warpPath` absolutas o vacías, `fetchBeforeCreate`/`openOnCreate` contra
sus valores admitidos, `worktreeRootTemplate` con `{name}` obligatorio y un aviso —no
un rechazo— si le falta `{repo}`/`{repoParent}`); `editor`, `defaultBase` y
`branchPrefix` son libres. `wt config set` con un valor inválido falla **sin tocar el
archivo** y explica los valores admitidos; un archivo de config con un valor inválido
(editado a mano) descarta esa clave con un warning en vez de romper el CLI.

## Códigos de salida

| Código | Significado |
|---|---|
| `0` | OK |
| `1` | Error de uso: comando o flag desconocido, flag sin el valor que requiere, o argumento posicional faltante (ej. `wt create` sin nombre) |
| `2` | Error de git o del entorno: worktree/repo inexistente o ambiguo, `git worktree remove` fallido, nombre o valor de config inválido, etc. |

`Invoke-Wt` atrapa sus propios errores; nunca deja que una excepción se propague sin
control. En el modo función del perfil (`wt <comando>`) deja el código en
`$LASTEXITCODE` sin llamar a `exit`, para no cerrar la terminal del usuario; en el
modo `wt.ps1 -File` (usado por scripts y CI) sí llama a `exit $LASTEXITCODE` al
terminar, así el proceso externo devuelve el código correcto.

## Doctor: chequeo del setup

`wt doctor` verifica todo lo necesario para armar el workspace — con **Copilot CLI +
Warp + VS Code**, o solo **Warp + Copilot** (o el agente que configures en
`agentCommand`) — y te dice qué falta y cómo instalarlo o
configurarlo: git, Node ≥ 18, fnm (opcional), `agentCommand`, el editor configurado,
Warp/`wt.exe`, el archivo de config, `reposRoot` y que la config efectiva tenga
valores válidos (lista las claves inválidas, si las hay, y avisa si
`worktreeRootTemplate` no distingue repos entre sí). Sale con código `0` siempre: es
informativo.

## Consola interactiva

`wt console` abre un menú en loop: lista los worktrees numerados para elegir sin
escribir nombres, pide solo los datos mínimos (nombre, base, confirmaciones) y arma el
comando CLI equivalente, que **muestra antes de ejecutarlo** para ir aprendiendo el
CLI. Sale con `q`. No duplica lógica: es un wrapper fino sobre las mismas
funciones del módulo, por lo que respeta la misma configuración y protecciones.

Las opciones de worktrees (`1`–`7`) solo aparecen si estás dentro de un repo. Las de
workspace siempre están: `r` lista los repos, `g` te deja elegir **un repo o un
worktree de cualquiera de ellos** (B12) y hace `cd` ahí (si elegís un worktree, las
opciones de worktrees se habilitan para el repo que lo contiene), `c` abre el
submenú de configuración (ver / cambiar valores / abrir el archivo) y `d` corre el
doctor.

## Configuración

Orden de precedencia (el primero que existe gana por clave):

1. Variable de entorno `WT_CONFIG` (apunta a un archivo JSON; usado por las pruebas).
2. `.wt.json` en la raíz del repositorio (config por proyecto, commiteable).
3. `%USERPROFILE%\.wt\config.json` (config global del usuario).
4. Valores por defecto embebidos en `src/Config.ps1`.

`WT_CONFIG_ONLY=1` cambia la cadena a **defaults < `WT_CONFIG`**: se ignoran el
`.wt.json` del repo y la config global del usuario. Pensada para pruebas y para
correr `wt` con una config completamente aislada de la máquina.

### `.wt.json` del repo: lista blanca de claves

A diferencia de la config global y de `WT_CONFIG` (que el usuario controla), el
`.wt.json` del repo puede venir de **un repositorio ajeno recién clonado**. Por eso
solo admite las claves que no pueden convertirse en un ejecutable, un shell o una
ruta arbitraria:

- `worktreeRootTemplate`
- `defaultBase`
- `branchPrefix`
- `fetchBeforeCreate`

Cualquier otra clave (`editor`, `warpPath`, `agentCommand`, ...) se **ignora** con un
warning que nombra el archivo y la clave. Sin esta lista, un `.wt.json` hostil podría
hacer que `wt open` lance el binario que ese archivo elija.

| Clave | Default | Descripción |
|---|---|---|
| `worktreeRootTemplate` | `{repoParent}\{repo}.worktrees\{name}` | Ubicación de los worktrees |
| `reposRoot` | `''` | Raíz de los repos git para `wt repos` y `wt cd`; string único o array de varias raíces (ej. `C:\Repos` o `["C:\\Repos", "D:\\OtrosRepos"]`, a mano en el JSON) |
| `reposDepth` | `1` | Niveles bajo cada `reposRoot` para buscar repos (1 a 3); `2` habilita `C:\Repos\<org>\<repo>` |
| `defaultBase` | `''` | Base por defecto de `create` (ej. `origin/develop`) |
| `branchPrefix` | `''` | Prefijo para ramas nuevas (ej. `agent/`) |
| `openOnCreate` | `all` | Que abre `create` sin flags de apertura: `all` (editor + agente) \| `editor` \| `none` |
| `editor` | `code` | Comando del editor (`''` para desactivar) |
| `terminal` | `warp` | `warp` \| `wt` \| `none` |
| `warpAgentTarget` | `auto` | Dónde abrir los tabs: `auto` (misma ventana si `wt` corre dentro de Warp o hay un Warp activo), `tab`, `window` |
| `warpAgentColor` | `green` | Color del tab del agente (`black`/`red`/`green`/`yellow`/`blue`/`magenta`/`cyan`/`white`; `''` sin color) |
| `warpTerminalColor` | `blue` | Color del tab de terminal común (`''` sin color) |
| `warpPath` | `%LOCALAPPDATA%\Programs\Warp\warp.exe` | Ruta a `warp.exe` |
| `fetchBeforeCreate` | `true` | Fetch + prune cuando la base es `<remote>/<rama>` de un remoto configurado del repo |
| `agentCommand` | `copilot` | Comando que corre el tab del agente (`wt open --agent`) |
| `agentShell` | `powershell` | `powershell` \| `bash` \| `none` (sin comandos auxiliares como `fnm use`, solo `agentCommand`) |

## Decisiones de arquitectura

- **PowerShell 5.1 compatible**: sin ternarios, `??` ni sintaxis de PS7, porque es el
  intérprete que Windows garantiza. Todo corre también en PowerShell 7.
- **Un módulo, capas explícitas**: `wt.psm1` solo carga los archivos de `src/` y define
  la superficie pública; importarlo no produce efectos secundarios. Las dependencias van
  en una sola dirección — `Cli`/`Console` → `Commands` → `Workspace`/`Launch` →
  `Repo`/`Config` → `Common` — y las decisiones (parseo de argumentos, parseo del
  porcelain de git, plan de apertura, generación del TOML, merge de config) son funciones
  puras separadas de los efectos (`Start-Process`, `Set-Location`, escritura de archivos)
  y de la presentación. Eso es lo que hace posible probarlas sin lanzar un subproceso.
- **Ejecución de procesos externos centralizada** en `Invoke-WtProcess` (y su
  especialización `Invoke-WtGit`): un único lugar aísla `$ErrorActionPreference`, pasa
  los argumentos como array —nunca concatenados en un string— y normaliza el código de
  salida y el mensaje de error.
- **Rutas normalizadas en un solo lugar**: `ConvertTo-WtFullPath` (de `/` a `\`,
  absoluta, sin barra final) y `Test-WtPathEquals` / `Test-WtPathIsUnder`, que comparan
  por componentes y sin distinguir mayúsculas. La comparación por componentes es lo que
  evita que un hermano con prefijo común (`feature-a-backup` vs `feature-a`) se tome por
  un subdirectorio.
- **Worktrees como carpetas hermanas** (`<repo>.worktrees\<nombre>`) en vez de dentro
  del repo: evita ruido en `git status`, búsquedas del editor y herramientas que
  recorren el árbol. Se puede cambiar con `worktreeRootTemplate`.
- **Resolución del repo principal vía `git rev-parse --git-common-dir`** y del checkout
  actual vía `--show-toplevel`: funcionan igual desde la raíz, un subdirectorio o
  *dentro de otro worktree*, sin caminar directorios a mano. `open`/`path` sin nombre
  operan sobre el checkout actual (el worktree si estás dentro de uno). Si el `.git`
  del directorio actual es un archivo que apunta a un gitdir inexistente (worktree
  huérfano), el error lo detecta y explica cómo repararlo.
- **Worktrees obsoletos (`prunable`)**: se detectan parseando el atributo `prunable`
  de `git worktree list --porcelain`. `list` y la consola los marcan, `open`/`path`/
  `remove` los rechazan con un mensaje que sugiere `wt prune`, `doctor` los avisa y
  `open` nunca abre el editor o la terminal sobre un directorio que no existe (antes
  VS Code lo abría como "archivo nuevo" y Warp caía en el home del usuario).
- **Parsing de `git worktree list --porcelain`** (formato estable) en lugar de la salida
  tabular, con normalización de `/` a `\` para comparaciones de rutas en Windows. El
  worktree **principal** se identifica por ser la primera entrada del porcelain —lo que
  git garantiza— y no por el directorio actual: así el resultado no depende de desde
  dónde se consulte el repo.
- **Ramas con el mismo nombre del worktree** por defecto (simple de razonar);
  `branchPrefix` permite adoptar convenciones tipo `agent/<nombre>` sin cambiar el flujo.
- **`create` reutiliza la rama si ya existe** (`git worktree add <path> <rama>`) y solo
  crea rama nueva con `-b` cuando no existe.
- **Protecciones**: no permite eliminar el worktree principal; si el directorio actual
  está dentro del worktree a eliminar, se mueve a la raíz del repo antes de borrar;
  los nombres se validan **antes** de tocar git (caracteres inválidos para rutas de
  Windows, `.`/`..`, nombres reservados como `CON`/`NUL`, final en punto o espacio) y el
  nombre de rama resultante se valida con `git check-ref-format`.
- **`remove --delete-branch` nunca descarta commits en silencio**: borra la rama con
  `git branch -d` (falla si tiene commits que no están mergeados en ninguna otra rama).
  Si falla, el worktree ya se eliminó y el mensaje explica cómo forzarlo con
  `--force-branch` (`git branch -D`). `--force` es una decisión independiente: solo
  fuerza `git worktree remove --force` (árbol de trabajo sucio) y nunca implica
  `--force-branch`. `Remove-WtWorktree` declara `SupportsShouldProcess` y confirma el
  worktree y la rama por separado.
- **`wt lock`/`wt unlock`**: envuelven `git worktree lock`/`unlock`. El parser del
  porcelain (`ConvertFrom-WtWorktreePorcelain`) detecta el atributo `locked` (con o
  sin motivo) igual que ya hacía con `prunable`; `wt list`, `--json` y la consola lo
  marcan, y `wt remove` lo rechaza con un mensaje que nombra el motivo (si lo hay) y
  sugiere `wt unlock` en vez de dejar que git falle con su mensaje crudo.
- **`create` cumple lo que promete abrir**: `Get-WtCreateOpenPlan` (función pura)
  resuelve `openOnCreate` (`all` por defecto = editor + agente) contra los flags
  explícitos de apertura (`--all`/`--code`/`--agent`/`--terminal`, que ganan a la
  config) y `--no-open` (que gana a todo). `openOnCreate` es una decisión de la
  máquina, no del repo: no entra en la lista blanca de claves de `.wt.json`.
- **`open` desacoplado**: editor y terminal son comandos configurables. Con
  `terminal = 'warp'`, tanto la terminal común como `--agent` generan un **Tab Config
  de Warp** (`%APPDATA%\warp\Warp\data\tab_configs\wt-term-<nombre>-<hash>.toml` y
  `wt-agent-<nombre>-<hash>.toml`, con `<hash>` = 8 hex de `Get-WtPathHash` sobre la
  ruta del worktree) con el worktree como `directory`, y lo abren con
  `warp://tab_config/...`. El sufijo de hash evita que dos worktrees homónimos de
  repos distintos (mismo `<nombre>` saneado) se pisen el tab config entre sí; el
  `name` legible adentro del TOML no lo necesita. El del agente suma `commands` =
  `fnm use <major>` (versión de la sesión si es ≥ 18, si no la mayor instalada; se
  omite si `agentShell = 'none'`) + `agentCommand` (`copilot` por defecto).
  Los tabs se titulan `repo > worktree (rama)` y se colorean con `warpTerminalColor`
  (terminal, default azul) o `warpAgentColor` (agente, default verde) — los tab groups
  de Warp no son scriptables; título + color es la aproximación visual. El destino lo
  controla `warpAgentTarget`: `auto` (default) abre una **pestaña en la ventana
  activa** cuando `wt` corre dentro de Warp (`TERM_PROGRAM=WarpTerminal`) **o hay un
  proceso de Warp corriendo**, y ventana nueva solo si Warp no está abierto; `tab` y
  `window` lo fuerzan. **Nunca abre ventanas sueltas de PowerShell**: el agente solo se
  lanza como tab de Warp; si `agentCommand` no existe en la sesión o `terminal` no es
  `warp`, avisa con un warning en vez de abrir otra cosa.
- **Agente desacoplado de `copilot` y del shell de Warp**: antes el tab del agente
  inyectaba un `if { Write-Warning ... }` en sintaxis de PowerShell entre sus
  `commands`; si el shell por defecto de Warp era bash, WSL o cmd, era un error de
  sintaxis en cada tab, y el binario era la cadena literal `copilot`. `agentCommand`
  (default `copilot`) es el comando que corre el tab; `agentShell` (`powershell` por
  defecto; también `bash` y `none`) decide si se emiten comandos auxiliares como
  `fnm use` (`none` = únicamente `agentCommand`). El chequeo de versión de Node salió
  del tab: vive solo en `wt doctor`. Ninguna de las dos claves entra en la lista
  blanca de `.wt.json`.
- **Ciclo de vida de los tab configs**: `wt remove` borra los del worktree que
  elimina (agente y terminal) antes de borrarlo — el nombre de archivo depende del
  hash de la ruta, así que hay que resolverlo mientras esta todavía lo identifica.
  `wt clean` depura los que quedaron huérfanos por otras vías (borrado manual del
  directorio, worktrees creados con una versión anterior): lee el `directory` de
  cada `wt-*.toml` en la carpeta de tab configs y borra el archivo si esa ruta ya
  no existe.
- **Instalador reversible y con `-WhatIf` real**: `install.ps1` declaraba
  `SupportsShouldProcess` pero nunca llamaba a `$PSCmdlet.ShouldProcess`, así que
  `-WhatIf` no evitaba ninguna escritura. Las tres escrituras (bloque del perfil,
  directorio de config, copia de `config.example.json`) están detrás de
  `ShouldProcess`; `uninstall.ps1` revierte el bloque del perfil con el mismo par de
  marcadores, deja el resto **byte a byte igual** y no toca la config salvo
  `-RemoveConfig`.
- **TOML generado con escapes explícitos**: `directory` y `commands` se escriben como
  *literal strings* de TOML (`'...'`, sin escapes) y por eso se rechaza una comilla
  simple con un mensaje claro; `name`, `title` y `color` van como *basic strings*
  (`"..."`) con `\` y `"` escapados. El nombre del archivo se sanea y el URI
  `warp://tab_config/...` se URL-encodea. Se escriben como **UTF-8 sin BOM**
  (`Set-WtFileUtf8NoBom`, también usado para `config.json`): `Set-Content -Encoding
  UTF8` en PowerShell 5.1 antepone `EF BB BF`, que muchos parsers TOML rechazan o
  interpretan como parte del primer valor.
- **Consola interactiva como wrapper**: `wt console` compone los mismos comandos del
  CLI en vez de duplicar lógica. Lee con `[Console]::In.ReadLine()`, así funciona igual
  en uso interactivo y con stdin pipeado (lo que la hace testeable E2E), y ante EOF
  termina limpio (el lector devuelve `$null`, sin banderas globales). El menú es una
  tabla de datos con una marca `RequiresRepo`, así que el repo actual se re-evalúa en
  cada vuelta del loop y la opción `g` (ir a un repo) habilita las opciones de worktrees
  sin salir de la consola.
- **Workspace sin repo**: `repos`, `cd`, `config` y `doctor` no llaman a
  `Find-WtMainRoot`, así funcionan desde cualquier directorio. `wt cd` resuelve por
  nombre exacto o prefijo único (case-insensitive) entre los subdirectorios de
  `reposRoot` que contienen `.git`; `open`/`path` reutilizan ese mismo criterio.
- **Resolución sin efectos secundarios**: `Resolve-WtRepoContext` (y `Resolve-WtTarget`,
  que la usa) solo *resuelven* — nunca hacen `cd` ni imprimen nada; devuelven
  `ShouldRelocate` para que decida el llamador. `Open-WtWorktree` lo aplica (por eso
  `wt open <repo o worktree de reposRoot>` deja tu terminal posicionada ahí);
  `Invoke-WtPathCommand` nunca lo aplica, así `wt path` es seguro de llamar desde un
  script sin mover el directorio actual del proceso.
- **Config editable por comandos**: `wt config set` escribe solo el archivo global
  (o `WT_CONFIG`), nunca los defaults embebidos ni el `.wt.json` del repo, y valida
  las claves contra los defaults para evitar typos. La config efectiva se cachea por
  directorio actual dentro de un mismo proceso, así un comando no dispara varias
  llamadas a git para releer lo mismo.
- **`Config` depende solo de `Common`, de verdad**: ubicar el `.wt.json` del repo
  actual usaba `Find-WtMainRoot` (de `Repo`, que invoca git), contradiciendo tanto el
  diagrama de dependencias de arriba como el orden de carga del módulo.
  `Find-WtRepoConfigFile` (en `Common`) resuelve lo mismo caminando el filesystem —
  sube directorios buscando `.git`; si es un archivo (worktree), sigue el puntero
  `gitdir:` hasta la raíz principal — sin invocar ningún subproceso.
- **Costo de arranque**: el bloque del perfil importa el módulo **una sola vez** (al
  abrir la terminal) en vez de reimportarlo en cada invocación de `wt` — `wt.ps1`
  sigue reimportando en cada llamada, pero solo se usa para el modo `-File`.
  `Get-WtWorktrees` cachea por repo dentro de una misma invocación (`wt open
  <worktree-de-otro-repo>` puede recorrer varios repos de `reposRoot`); `Invoke-Wt`
  la invalida junto a la cache de config al empezar y después de
  `create`/`remove`/`lock`/`unlock`/`prune`, y `wt console` la invalida en cada
  vuelta del menú. `Get-WtConfig` acepta `-RepoRoot` para no repetir el
  `git rev-parse` cuando el llamador ya lo resolvió.

## Estructura

```
worktree-manager/
├── wt.psm1                      # Carga de src/ y superficie pública del módulo
├── wt.psd1                      # Manifiesto del módulo (versión, PowerShellVersion, contrato de export)
├── wt.ps1                       # Entrypoint para invocación por -File
├── install.ps1                  # Instalador idempotente (perfil + config global)
├── uninstall.ps1                # Revierte install.ps1 (bloque del perfil; -RemoveConfig)
├── config.example.json          # Config de referencia
├── PSScriptAnalyzerSettings.psd1 # Reglas de lint (CI)
├── CHANGELOG.md                 # Historial por fase (Keep a Changelog)
├── LICENSE
├── .github/workflows/ci.yml     # CI: suites + PSScriptAnalyzer en PS 5.1 y 7
├── src/
│   ├── Common.ps1               # Procesos externos, rutas, validación, presentación
│   ├── Config.ps1               # Defaults, precedencia, get/set y comando 'config'
│   ├── Repo.ps1                 # Repo actual, modelo de worktrees y resolución
│   ├── Workspace.ps1            # reposRoot, 'repos', 'cd' y contexto de trabajo
│   ├── Launch.ps1               # Editor, terminal, agente y tab configs de Warp
│   ├── Commands.ps1             # create/list/path/open/remove/prune/doctor
│   ├── Console.ps1              # Consola interactiva (wrapper sobre Commands)
│   └── Cli.ps1                  # Parser de argumentos, ayuda y dispatcher
├── tests/
│   ├── Invoke-WtTests.ps1       # Pruebas E2E contra un repo temporal
│   └── Invoke-WtUnitTests.ps1   # Pruebas unitarias de la lógica pura
└── README.md
```

## Pruebas

```powershell
powershell -ExecutionPolicy Bypass -File C:\worktree-manager\tests\Invoke-WtUnitTests.ps1
powershell -ExecutionPolicy Bypass -File C:\worktree-manager\tests\Invoke-WtTests.ps1
```

Las **unitarias** corren en el mismo proceso, sin git ni disco, y cubren la lógica pura:
parser de argumentos (alias, flags desconocidos, flags con valor faltante), parsing del
porcelain de `git worktree list`, normalización y comparación de rutas, validación de
nombres, merge de configuración, plan de apertura de `open` y generación del TOML de Warp.

Las **E2E** crean un repositorio temporal en `%TEMP%` y ejercen `create`/`list`/`open`/
`path`/`remove`/`prune` (incluidos casos de error: duplicado, nombre inválido, worktree
principal, cambios sin commit, uso desde dentro de un worktree, ejecución fuera de un
repo, worktree obsoleto por directorio borrado a mano y worktree huérfano por repo
principal movido), la validación de flags del CLI, la generación del tab config de Warp
para el agente, los comandos de workspace (`repos`, `cd` con match por prefijo y
ambigüedad, `config get/set` incluidos valores con espacios, `doctor`, `version`) y la
consola interactiva vía stdin pipeado (menú, create, list, open, remove con
confirmación, ir a un repo, EOF). Se limpia al terminar. Exit code `0` = todo OK,
`1` = hubo fallas.

`.github/workflows/ci.yml` corre ambas suites y `Invoke-ScriptAnalyzer` (con
`PSScriptAnalyzerSettings.psd1`) en `windows-latest`, bajo `powershell` (5.1) y `pwsh`
(7). Localmente:

```powershell
Install-Module PSScriptAnalyzer -Scope CurrentUser
Invoke-ScriptAnalyzer -Path . -Recurse -Settings PSScriptAnalyzerSettings.psd1
```

### Verificación manual de `install.ps1` / `uninstall.ps1`

No van en las suites automáticas porque tocan el perfil real de PowerShell del
usuario. Verificar a mano tras tocar cualquiera de los dos:

```powershell
# .\install.ps1 -WhatIf no debe modificar el perfil (comparar hash antes/después)
(Get-FileHash $PROFILE.CurrentUserAllHosts).Hash
.\install.ps1 -WhatIf
(Get-FileHash $PROFILE.CurrentUserAllHosts).Hash   # debe ser igual al anterior

# .\uninstall.ps1 debe dejar el resto del perfil byte a byte igual, salvo el bloque
.\install.ps1
.\uninstall.ps1
# diff manual contra una copia del perfil de antes de instalar
```
