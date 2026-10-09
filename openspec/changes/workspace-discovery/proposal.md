# Proposal: workspace-discovery

## Why

Con M1, `wt cd` solo conoce los worktrees del repo en el que estás parado. Para saltar a otro repo hay que acordarse de dónde vive y tipear la ruta: `cd ~/Documents/GIT/TrendFisher`. Si ya estás afuera de cualquier repo, `wt cd` ni siquiera arranca (exit 3). Con diez repos y agentes repartidos entre ellos, ese salto es el movimiento más común del día. v0.9 lo resolvía con `reposRoot` y `wt cd <repo>` desde cualquier lado, y es lo que todavía te hace volver a v0.9. Este change abre M2: `wt` descubre los repos bajo las raíces que configures, `wt repos` los lista, y `wt cd <repo>` te deja en cualquiera de ellos desde cualquier directorio, con la misma tolerancia que v0.9 (`wt cd trend` → `TrendFisher`).

## What Changes

- **Nuevas claves de config** (ninguna se puede poner en `.wt.toml`):
  - `repos_root`: lista de raíces donde viven tus repos. Cada una es una ruta absoluta o empieza con `~`. Default `[]`: sin raíces no hay workspace, y todo se comporta como en v0.1.0. En el archivo es siempre un array de TOML: `repos_root = "~/GIT"` es un error que sugiere `["~/GIT"]`.
  - `repos_depth`: cuántos niveles bajo cada raíz se buscan repos, de 1 a 3. Default `1`. Con `2` se ven los layouts `<raíz>/<org>/<repo>`.
  - Variables de entorno `WT_REPOS_ROOT` (raíces separadas por `:` en macOS y Linux, y por `;` en Windows, como `PATH`) y `WT_REPOS_DEPTH`.
- **`wt config get` y `wt config list` muestran listas**: `get` imprime una raíz por línea, tal como está escrita; `list` las muestra como un array (`["~/GIT", "/Volumes/x"]`); `--json` las da como array de JSON.
- **Qué es un repo**: un directorio bajo una raíz con un `.git` adentro, salvo que ese `.git` sea el de un **worktree enlazado**. Así, el layout por defecto de `wt create` (`<repo>.worktrees/<name>`) nunca aparece como repo, aunque viva en la misma raíz (el bug de v0.9 con `reposDepth > 1`), y un repo con el layout `.bare` (un `.git` archivo que apunta a un repo bare al lado) sí cuenta. La búsqueda no entra en repos ni en worktrees, sigue symlinks, no trata distinto a los directorios ocultos, y lista una sola vez un repo al que se llega por dos caminos. Una raíz que no existe (un disco externo desmontado) produce un warning, y se sigue con las demás.
- **Nuevo comando `wt repos`**: tabla con `NAME` y `PATH` de cada repo descubierto, ordenada por nombre sin distinguir mayúsculas, con `@` en el repo al que pertenece el directorio actual (también desde uno de sus worktrees). `--json` usa el nuevo esquema `wt.repos.v1` (`name`, `path`, `root`, `current`). Sin raíces configuradas: exit 1, con un hint que nombra `repos_root` y el archivo de usuario. Con raíces pero sin repos: exit 0 y ninguna fila.
- **`wt cd <target>` resuelve repos** (modificado):
  - Primero el repo actual, **exactamente como en v0.1.0**: nombre del worktree y después rama. Todo `wt cd x` que hoy funciona sigue yendo al mismo lugar.
  - Si eso no encuentra nada (o estás fuera de un repo), busca entre los repos descubiertos, en tres pasos, cada uno solo si el anterior no encontró nada: nombre exacto; nombre exacto sin distinguir mayúsculas; prefijo único sin distinguir mayúsculas. Más de un repo en el paso que encontró es una ambigüedad: exit 4, con un `hint:` por candidato.
  - El destino es el directorio que lista `wt repos` (en un repo normal, su worktree principal).
  - Lo que antes era exit 3 cambia de mensaje: dentro de un repo, `no worktree or repository named "x"` con hints a `wt list` y `wt repos`; fuera, `no repository named "x"` con hint a `wt repos`; fuera y sin raíces, el mismo `not a git repository` de hoy, más un hint que sugiere configurar `repos_root`.
  - `^`, `@` y `-` no cambian. `wt cd` sin argumento sigue siendo error de uso (exit 2): el picker llega en M4.
- **Completions de `wt cd`**: además de los worktrees del repo actual, ofrecen los nombres de los repos descubiertos, también fuera de un repo. Leen la config sin escribir nada en la terminal, ni siquiera un warning.
- Se quita: nada. Ningún flag nuevo. Los exit codes no cambian: 4 ya significa "nombre ambiguo".

## Capabilities

### New Capabilities

- `workspace-discovery`: las raíces de repos y su expansión de `~`, qué cuenta como repo y hasta dónde se busca, cómo se llama cada repo y cuál es su ruta, y el comando `wt repos` con su tabla, su JSON y sus errores.

### Modified Capabilities

- `navigate`: `wt cd <target>` suma la búsqueda en el workspace después del repo actual, con el matcheo escalonado de nombres de repo, la ambigüedad entre repos (exit 4) y los nuevos mensajes de "no encontrado".
- `config-layers`: dos claves nuevas (`repos_root`, `repos_depth`) y los tipos que traen: la primera clave lista y la primera entera, con su forma en el archivo, en el entorno y en `wt config get`/`list`.
- `shell-integration`: el requirement "Completions" suma los nombres de repos para el argumento de `wt cd`, dentro y fuera de un repo.

## Non-goals

- **Saltar a un worktree de otro repo por nombre** (`wt cd feat` estando en otro repo o fuera de todos), la forma calificada `<repo>:<name>`, y `wt list --all-repos`: son el próximo change de M2, `cross-repo-resolution`. Requieren un `git worktree list` por repo, y su costo merece un diseño propio.
- **Que `wt path`, `wt remove`, `wt lock` o `wt unlock` resuelvan en otros repos**: también `cross-repo-resolution`.
- **`wt exec` y `wt each`**: `batch-execution`, el tercer change de M2.
- **`wt cd` sin argumento hacia la raíz de repos**, como en v0.9: queda en exit 2 hasta el picker de M4, que va a ocupar ese lugar sin romper nada.
- **Prefijo y mayúsculas para nombres de worktree**: los worktrees siguen exactos y case-sensitive, como en v0.1.0. Sumarlo después sería aditivo.
- **Repos bare sin `.git`** (un `api.git/` suelto con `HEAD`, `objects`, `refs`): no se reconocen; `wt cd` te dejaría en un directorio sin archivos. Se puede sumar después sin romper nada.
- **Desambiguar dos repos homónimos por su ruta** (`wt cd work/api`): exit 4 lista ambos. La forma de nombrarlos llega con `cross-repo-resolution`.
- **`wt config set`**: `repos_root` se escribe a mano en el archivo de usuario, o se pasa por `WT_REPOS_ROOT`.
- **Caché del descubrimiento**: el recorrido es de filesystem, acotado por `repos_depth`, y no corre git. Si alguna vez hace falta, se mide primero.

## Milestone

M2, primer corte de tres (**`workspace-discovery`** → `cross-repo-resolution` → `batch-execution`), elegido para que el primero ya sea usable de punta a punta: el `wt cd <repo>` de todos los días, y no solo un `wt repos` que lista. Ver `docs/design/product.md` §7 ("M2 EL DIFERENCIAL: workspace-discovery, cross-repo-resolution, batch-execution").

Este change suma a `docs/design/product.md` §7 la tabla "M2, desglosado en changes", como la que tiene M1.

Rama: `v0.2/workspace-discovery`. Publica `v0.2.0-alpha.1`.

## Diferencias por OS

Sí, y se especifican por separado:

- **Separador de `WT_REPOS_ROOT`**: `:` en macOS y Linux, `;` en Windows, el mismo que usa `PATH` en cada uno. En Windows `C:\Repos` lleva `:`, así que no hay un separador común.
- **`~`**: se expande a `HOME` en macOS y Linux, y a `USERPROFILE` en Windows, donde también se acepta `~\`.
- **Ruta absoluta**: `/x` en macOS y Linux; en Windows, con letra de unidad (`C:\x`) o UNC (`\\server\share`). `\x` sin unidad no es absoluta en Windows.
- **Links**: en macOS y Linux se siguen los symlinks a directorios; en Windows, también los junctions.
- **Mayúsculas**: el matcheo de nombres de repo es igual en todos los OS (los pasos 2 y 3 ignoran mayúsculas en todos, y el paso 1 no en ninguno). Decidir si dos caminos llevan al mismo repo, y si el directorio actual pertenece a uno, no compara texto sino la identidad del directorio en disco, así que da lo mismo en los tres OS con symlinks, junctions o mayúsculas distintas.
- **Rutas** en la salida y en `--json`: nativas (`C:\Repos\api` en Windows).

## Impact

- Código: `internal/workspace` (hoy solo `doc.go`) recibe las decisiones puras: interpretar las raíces, decidir qué es un repo a partir de lo que se lee del filesystem, deduplicar, ordenar y matchear nombres. El recorrido real queda en un borde chico. `internal/config` suma `repos_root` y `repos_depth`, con los validadores de lista y de entero. `internal/cli` suma `wt repos`, cambia `cdDestination` para que caiga al workspace, y suma los repos a la completion de `wt cd`.
- Dependencias nuevas: ninguna.
- Contrato público: un comando (`wt repos`), un esquema JSON (`wt.repos.v1`), dos claves de config y dos variables de entorno. `wt cd` resuelve casos que antes eran exit 3 y cambia esos mensajes. Exit codes sin cambios.
- CI: sin jobs nuevos. Los tests de discovery corren contra árboles reales en directorios temporales, en los tres OS, incluidos symlinks (y junctions en `windows-latest`).
- Documentación: `docs/design/product.md` (§7), `CHANGELOG.md`.
