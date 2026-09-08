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
terminal), define la función `wt` (y alias `wtm`) como un wrapper liviano sobre
`Invoke-Wt`, ya cargado, y registra el autocompletado (`Register-WtCompletion`); crea
`%USERPROFILE%\.wt\config.json` a partir de `config.example.json` solo si no existe.
Luego reiniciá la terminal o ejecutá `. $PROFILE`. Soporta `-WhatIf` (no modifica
nada, solo informa qué haría) y `-Confirm` (pide confirmación antes de cada
escritura: perfil, config).

### Autocompletado

Instalado el bloque del perfil, `wt <TAB>` completa comandos y alias, `wt open --<TAB>`
completa flags, `wt config set terminal <TAB>` completa valores admitidos de esa
clave, y la segunda posición de `open`/`path`/`remove`/`cd`/`lock`/`unlock` completa
con los worktrees del repo actual (o, fuera de un repo, los repos de `reposRoot`). El
completer nunca dispara `git`: usa la caché de worktrees de la invocación anterior de
`wt` en la misma sesión (si no hay nada cacheado, no sugiere nombres) y un recorrido
de filesystem para los repos, para no introducir latencia al escribir.

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
wt create tmp3 --no-hooks                    # crea sin correr copyOnCreate/postCreate
wt list                                      # tabla de worktrees (el principal va marcado)
wt list --json                               # salida JSON para scripting
wt open                                      # abre el checkout actual en VS Code (el worktree si estás dentro de uno)
wt open logging                              # abre el worktree en VS Code
wt open logging --agent                      # abre solo el agente en el worktree (tab de Warp o de Windows Terminal, segun 'terminal')
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
wt status                                    # estado de todos los worktrees del repo actual (alias: st)
wt status logging --json                     # estado de uno solo, en JSON
wt status --fetch                            # fetch --all --prune antes de calcular ahead/behind
wt exec logging -- npm test                  # corre el comando en ese worktree
wt each -- git fetch                         # corre el comando en todos los worktrees (menos el principal), en serie
wt each --continue-on-error -- npm test      # sigue aunque falle alguno; resume al final que worktrees fallaron
wt sync                                      # rebasa cada worktree contra su upstream (o defaultBase)
wt sync logging --base develop --strategy merge
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

### Qué abre cada `terminal`

| `terminal` | `--terminal` (terminal común) | `--agent` (agente) |
|---|---|---|
| `warp` (default) | Tab de Warp (`warpTerminalColor`) | Tab de Warp con `agentCommand` (`warpAgentColor`) |
| `wt` | Pestaña de Windows Terminal | Pestaña de Windows Terminal que corre `agentCommand` (con `fnm use` primero si `agentShell` lo permite) |
| `none` | No se abre nada (avisa) | No se abre nada (avisa) |

El título de la pestaña es el mismo en los dos terminales (`repo > worktree (rama)`);
solo Warp puede colorearla (`warpAgentColor`/`warpTerminalColor` no tienen equivalente
en Windows Terminal). El agente **nunca** se abre como ventana suelta de PowerShell,
solo como tab/pestaña de uno de los dos terminales.

### Hooks de creación: `copyOnCreate` y `postCreate`

Un worktree nuevo es un checkout git limpio: no trae `.env`, certificados,
`appsettings.Development.json` ni `node_modules`. Dos claves de config (solo en la
config global o en `WT_CONFIG`, **nunca** en el `.wt.json` de un repo — ver la lista
blanca abajo) preparan el árbol antes de abrir el editor o el agente:

```powershell
wt config set copyOnCreate ".env;certs/*.pfx"       # patrones (';'-separados), glob simple
wt config set postCreate "npm install;cmd /c echo listo"  # comandos, en orden
wt create feature-x                                  # copia, corre postCreate y recien despues abre
wt create feature-y --no-hooks                       # crea sin correr ninguno de los dos
```

`copyOnCreate` es una lista de patrones relativos a la raíz del repo principal
(`.env`, `certs/*.pfx`); los que no matchean ningún archivo se avisan y se sigue, no
fallan. `postCreate` corre en el worktree recién creado, en orden, **después** de la
copia y **antes** de abrir el editor/agente; si un comando falla, la cadena se
detiene, se avisa qué comando falló, y **el worktree no se destruye** (ya existe;
borrarlo sería peor que dejarlo a medio preparar). `wt doctor` informa cuántos
patrones y comandos hay configurados.

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

## `--dry-run` global

Válido para **cualquier** comando (`wt create x --dry-run`, `wt remove logging
--dry-run --delete-branch`, `wt config set terminal none --dry-run`, ...). Con
`--dry-run`, ningún comando externo se ejecuta y ningún archivo se escribe: cada
operación que lo haría se imprime con el prefijo `[dry-run]` en su lugar. Las
lecturas (`git rev-parse`, `git worktree list`, `git show-ref`, ...) se siguen
ejecutando de verdad — son consultas, no cambian nada, y `wt` las necesita para
decidir qué imprimiría. Un ensayo sin errores de uso sale con código `0`.

```powershell
wt create feature-x --dry-run          # muestra el plan completo sin crear nada
wt remove logging --dry-run --delete-branch --force-branch
wt config set terminal none --dry-run  # no toca el archivo de config
```

## `wt status`: estado de todos los worktrees

Reemplaza el "entrar a cada worktree para ver qué pasó" cuando hay varios agentes
trabajando en paralelo:

```powershell
wt status                    # todos los worktrees del repo actual
wt status logging            # solo ese
wt status --json             # el modelo completo, para scripting
wt status --fetch            # git fetch --all --prune antes de calcular (nunca sin el flag)
```

Por worktree: rama, archivos staged/modified/sin trackear, ahead/behind respecto de
su **upstream** (o de `defaultBase` si no tiene upstream) y el último commit (hash
corto, asunto, antigüedad relativa). Un worktree que falle al consultarse (bloqueado,
disco desconectado) se reporta con su error en la fila propia — el resto de la tabla
sigue.

## `wt exec` y `wt each`

Corren un comando externo sin cambiar de directorio, complemento natural de `wt
status` para supervisar trabajo en paralelo. El separador `--` es obligatorio: todo
lo que sigue va **crudo** al comando, sin que `wt` interprete sus flags (así `wt each
-- git log --oneline -1` no confunde `--oneline` con un flag de `wt`).

```powershell
wt exec logging -- npm test          # corre 'npm test' en el worktree 'logging'
wt each -- git fetch                 # lo corre en TODOS los worktrees del repo (menos el principal), en serie
wt each --continue-on-error -- npm test --json
```

`wt each` corta en el primer worktree que falla, salvo `--continue-on-error` (sigue y
al final resume qué worktrees fallaron). Salida por worktree: encabezado con el
nombre, la salida del comando tal cual, y el exit code si es distinto de 0.
`--json` emite el resultado estructurado (nombre, exit code, salida) en vez de la
salida legible. Exit code: `0` si todos salieron `0`, `2` si alguno falló.
`--dry-run` (arriba) lista qué se correría y dónde, sin correr nada.

## `wt sync`

Con varios agentes, la divergencia contra `develop` (o la rama que corresponda) es el
problema recurrente:

```powershell
wt sync                                 # todos los worktrees, rebase contra upstream/defaultBase
wt sync logging --base origin/develop   # solo ese, contra una base explícita
wt sync --strategy merge --continue-on-error
wt sync --dry-run                       # el uso principal las primeras veces: ver el plan sin tocar nada
```

Default: `rebase` sobre `defaultBase`, o el upstream de la rama si lo tiene (`--base`
explícito gana a los dos). **Antes de tocar nada se verifica que el worktree esté
limpio**: uno sucio se saltea con un aviso — nunca se hace stash automático, mover el
árbol de un agente corriendo adentro es inaceptable. Un conflicto **no se resuelve ni
se aborta solo**: se reporta, el worktree queda como git lo dejó (para resolverlo a
mano, o `git rebase --abort`), y se sigue con el próximo worktree (o se corta, según
`--continue-on-error`). Hace un solo `git fetch --all --prune` al inicio, no uno por
worktree. El resumen final lista: sincronizados, salteados por sucios, salteados sin
base, y en conflicto.

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

Las opciones de worktrees (`1`–`9`, más `s` y `y`) solo aparecen si estás dentro de un
repo: listar, crear, abrir (editor · solo agente · solo terminal · editor + agente,
las cuatro combinaciones que expone el CLI), ver la ruta, eliminar, prune, `s` corre
`wt status` del repo y `y` corre `wt sync`. Las de workspace siempre están: `r` lista
los repos, `g` te deja elegir **un repo o un worktree de cualquiera de ellos** (B12) y
hace `cd` ahí (si elegís un worktree, las opciones de worktrees se habilitan para el
repo que lo contiene), `c` abre el submenú de configuración (ver / cambiar valores /
abrir el archivo) y `d` corre el doctor. Antes de ejecutar cualquier acción se muestra
el comando CLI equivalente (`Write-WtConsoleCommand`).

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

Cualquier otra clave (`editor`, `warpPath`, `agentCommand`, `copyOnCreate`,
`postCreate`, ...) se **ignora** con un warning que nombra el archivo y la clave. Sin
esta lista, un `.wt.json` hostil podría hacer que `wt open` lance el binario que ese
archivo elija, o que `wt create` copie o ejecute lo que ese archivo decida.
`copyOnCreate` y sobre todo `postCreate` son ejecución de código: **nunca** entran en
esta lista, ni aunque parezcan inofensivas — solo se pueden definir en la config
global del usuario o en `WT_CONFIG`.

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
| `copyOnCreate` | `[]` | Patrones (glob simple, relativos al repo) a copiar al worktree nuevo; en `wt config set` van separados por `;` |
| `postCreate` | `[]` | Comandos a correr en el worktree nuevo, en orden, tras la copia; separados por `;` en `wt config set` |

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
- **`--dry-run` global sin flag por comando**: se reconoce en una sola línea de
  `ConvertFrom-WtArgs` (no está en la spec de ningún comando individual) y setea una
  bandera de módulo (`Set-WtDryRun`/`Test-WtDryRun`, en `Common.ps1`) que `Invoke-Wt`
  resetea **siempre** en un `finally`, para que nunca quede encendida entre
  invocaciones del modo instalado (el perfil importa el módulo una sola vez).
  `Invoke-WtProcess`/`Invoke-WtGit` se vuelven no-ops bajo `--dry-run` (imprimen con
  `[dry-run]` y devuelven un resultado sintético exitoso) **salvo** que el llamador
  pase `-ReadOnly` — las lecturas (`rev-parse`, `worktree list`, `show-ref`, `remote`,
  `check-ref-format`, `node --version`, `fnm list`) siguen corriendo de verdad,
  porque `wt` las necesita para decidir qué imprimiría. Nuevo `Start-WtProcess` es el
  único punto que invoca `Start-Process` para procesos "fire and forget" (editor,
  terminales, agente) y aplica el mismo criterio. `Set-WtFileUtf8NoBom` (único punto
  de escritura de archivos que `wt` genera) también consulta `Test-WtDryRun`, así que
  cubre tab configs y `config.json` con un solo cambio.
- **Agente sin dependencia dura de Warp**: `terminal = 'wt'` antes abría una terminal
  común pero nunca el agente (`Open-WtAgent` exigía Warp). `Get-WtWindowsTerminalArgs`
  (función pura) arma los argumentos de `wt.exe new-tab` reusando `Get-WtAgentCommands`
  (la misma decisión de M8: `fnm use <major>` si `agentShell` lo permite, más
  `agentCommand`) — `powershell -NoExit -Command "..."` o `bash -lc "..."` según
  `agentShell`, o el comando desnudo con `agentShell = 'none'`. `Open-WtAgent`
  despacha por `Config.terminal`: Warp sigue siendo el único camino con tab configs
  persistentes; `wt` pasa por argumentos de línea de comandos, sin escribir archivos.
- **Autocompletado sin costo de latencia**: `Get-WtCompletion` (función pura, en
  `src/Completion.ps1`) decide los candidatos a partir de `Get-WtCommandSpecs` (la
  misma fuente de verdad que el parser) y de un contexto (worktrees/repos) que le
  pasa el llamador — nunca resuelve nada por su cuenta, así se testea sin git ni
  disco. `Register-WtCompletion` (efecto) arma ese contexto con
  `Get-WtWorktreesCached` (nuevo: como `Get-WtWorktrees`, pero solo si el repo ya
  está en la caché de la invocación anterior de `wt` en la sesión — si no, vacío en
  vez de disparar `git worktree list`) y un recorrido de filesystem para los repos.
  Se registra desde el bloque del perfil de `install.ps1`, no al importar el módulo
  (importar no debe tener efectos secundarios).
- **`wt exec`/`wt each` y el `--` del parser**: `ConvertFrom-WtArgs` reconoce un `--`
  suelto y corta ahí el parseo — todo lo que sigue va crudo a `Rest`, sin
  interpretarse como flags de `wt` (necesario para que `wt each -- git log --oneline`
  no confunda `--oneline` con un flag propio). `Get-WtEachPlan` (función pura) decide
  sobre qué worktrees opera `each`: excluye los obsoletos siempre y el principal
  salvo `-IncludeMain`. `Invoke-WtCommandLine` (compartida por `exec` y `each`) corre
  la línea reconstruida vía `cmd.exe /c` con `Invoke-WtProcess`, así hereda gratis el
  soporte de `--dry-run` y `-WorkingDirectory`.
- **`wt sync`**: `Get-WtSyncPlan` (función pura, en `src/Commands.ps1`) decide, a
  partir del modelo de estado del item 5 (`Get-WtStatusEntry`), la acción por
  worktree (`sync` | `skip-dirty` | `skip-no-base`) y el `git rebase`/`git merge` a
  correr — toda la decisión es testeable sin git. `Invoke-WtSync` ejecuta el plan: un
  solo `fetch` al inicio, nunca stash automático (un worktree sucio se saltea), y un
  conflicto se reporta y se deja tal cual lo dejó git — ni se resuelve ni se aborta
  solo — para que el próximo worktree del plan igual se procese (o se corte, según
  `--continue-on-error`).
- **`wt status`**: `ConvertFrom-WtStatusPorcelainV2` (función pura, en `src/Repo.ps1`)
  parsea `git status --porcelain=v2 --branch` — formato estable, con las líneas
  `# branch.*` que ya traen ahead/behind del upstream — y distingue staged (columna
  X) de modified (columna Y) sin ambigüedad. `Get-WtStatusEntry` (efecto) suma, por
  worktree, el último commit (`git log -1`) y, cuando no hay upstream, ahead/behind
  contra `defaultBase` vía `git rev-list --left-right --count` (best-effort: si
  falla, queda en `0/0`); nunca lanza — un worktree que falle queda con su error en
  la entrada, no rompe el resto. `Get-WtStatusRows` (pura) arma la tabla.
- **Hooks de creación (`copyOnCreate`/`postCreate`)**: `Resolve-WtCopyOnCreatePlan`
  (función pura) decide, a partir de los patrones configurados y un listado de
  archivos del repo que le pasa el llamador, qué copiar y a dónde; rechaza patrones
  absolutos o con `..` (no se puede copiar desde fuera del repo) sin abortar el resto.
  `Invoke-WtCreateHooks` aplica ese plan y corre `postCreate` (vía `cmd.exe /c`, con
  `Invoke-WtProcess -WorkingDirectory`) en el worktree recién creado, entre el `git
  worktree add` y la apertura del editor/agente. Un `postCreate` que falla detiene la
  cadena y se relanza como error de `wt create` (el worktree ya existe: destruirlo
  sería peor), pero no impide que futuros `postCreate` de otra corrida se ejecuten.
  `--no-hooks` saltea ambos.
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
- **Consola alineada con el CLI (item 8)**: el menú ofrecía «editor» y «editor +
  agente» pero no «solo agente» ni «solo terminal», que sí existen en el CLI; ahora
  `Invoke-WtConsoleOpen` cubre las cuatro combinaciones. El menú suma `s` (`wt
  status` del repo) e `y` (`wt sync`, con una base opcional) — el wrapper cubre la
  superficie que envuelve.
- **Consola interactiva como wrapper**: `wt console` compone los mismos comandos del
  CLI en vez de duplicar lógica. Lee con `[Console]::In.ReadLine()`, así funciona igual
  en uso interactivo y con stdin pipeado (lo que la hace testeable E2E), y ante EOF
  termina limpio (el lector devuelve `$null`, sin banderas globales). El menú es una
  tabla de datos con una marca `RequiresRepo`, así que el repo actual se re-evalúa en
  cada vuelta del loop y la opción `g` (ir a un repo) habilita las opciones de worktrees
  sin salir de la consola.
- **Workspace sin repo**: `repos`, `cd`, `config` y `doctor` no llaman a
  `Find-WtMainRoot`, así funcionan desde cualquier directorio. Para nombres de
  **repo**, `Select-WtRepoMatches` es la única implementación del criterio (exacto o
  prefijo único, case-insensitive), compartida por `cd`, `open` y `path`.
  `Resolve-WtRepoOrWorktreeOwner` (B12) extiende esa resolución a **worktrees** de
  cualquier repo de `reposRoot` (primero repo, después worktree), compartida ahora
  también por los tres — antes solo `open`/`path` llegaban a un worktree fuera de un
  repo, `cd` se quedaba corto en la raíz de su repo.
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
│   ├── Cli.ps1                  # Parser de argumentos, ayuda y dispatcher
│   └── Completion.ps1           # Autocompletado (Register-ArgumentCompleter)
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
parser de argumentos (alias, flags desconocidos, flags con valor faltante, el `--`
suelto de `exec`/`each`), parsing del porcelain de `git worktree list` y de `git status
--porcelain=v2`, normalización y comparación de rutas, validación de nombres, merge de
configuración, plan de apertura de `open`/`create`, generación del TOML de Warp y de
los argumentos de Windows Terminal, autocompletado (`Get-WtCompletion`), el plan de
`copyOnCreate` y el de `wt sync` (`Get-WtSyncPlan`), y el modo `--dry-run` de
`Invoke-WtProcess`/`Invoke-WtGit`.

Las **E2E** crean un repositorio temporal en `%TEMP%` y ejercen `create`/`list`/`open`/
`path`/`remove`/`prune`/`clean`/`lock`/`unlock`/`status`/`exec`/`each`/`sync`
(incluidos casos de error: duplicado, nombre inválido, worktree principal, cambios sin
commit, uso desde dentro de un worktree, ejecución fuera de un repo, worktree obsoleto
por directorio borrado a mano, worktree huérfano por repo principal movido, hooks de
creación con patrón hostil en `.wt.json`, conflicto de `sync` sin auto-resolver), la
validación de flags del CLI, `--dry-run` de punta a punta (`create`/`remove`/`config
set`), la generación del tab config de Warp para el agente, los comandos de workspace
(`repos`, `cd` con match por prefijo y ambigüedad, `config get/set` incluidos valores
con espacios, `doctor`, `version`) y la consola interactiva vía stdin pipeado (menú,
create, list, open en sus cuatro combinaciones, remove con confirmación, status, sync,
ir a un repo, EOF). Se limpia al terminar. Exit code `0` = todo OK,
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
