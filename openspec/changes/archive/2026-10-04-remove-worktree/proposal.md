# Proposal: remove-worktree

## Why

Con `wt create` abrir una tarea cuesta un comando, pero cerrarla sigue costando cuatro: salir del worktree, `git worktree remove`, decidir a ojo si la rama ya está mergeada y `git branch -d` (o `-D`, y rezar). Si la carpeta tiene un archivo sin trackear, git se niega; si la borraste a mano, el registro queda colgado hasta un `git worktree prune`. Con tres agentes en paralelo eso se repite varias veces por día, y es lo último que te hace volver a v0.9. Este change cierra el ciclo mínimo de M1: `wt remove feat` borra el worktree sin perder trabajo, se lleva la rama solo si ya está mergeada, y si estabas parado adentro te deja en el worktree principal. Con él, M1 se puede cerrar.

## What Changes

- **Nuevo comando `wt remove <target>`**: borra un worktree del repo actual. `<target>` se resuelve igual que en `wt cd` (nombre, después rama; `@` es el worktree actual).
  - **Rama**: por defecto, la rama del worktree se borra **solo si está mergeada** (ver más abajo); si no lo está, se conserva y la salida lo dice. Un worktree con HEAD detached no tiene rama que borrar.
  - **Bloqueos, todos exit 5 y sin tocar nada**: el worktree tiene archivos modificados o sin trackear (salvo con `--force`); está lockeado; o contiene otro worktree (ni siquiera con `--force`: git borraría el anidado, con su trabajo).
  - El worktree principal (o el repo bare) no se borra: exit 2.
  - Un worktree cuyo directorio ya no existe se borra igual: `wt remove` lo desregistra, sin pasar por `wt prune`.
  - **Moverse**: si la shell está dentro del worktree que se borra, termina en el worktree principal a través de la función. Sin la función activa, borra igual y avisa que la shell quedó en un directorio que ya no existe.
  - Los archivos ignorados por git (`node_modules`, un `.env` ignorado) se borran con el worktree, como hace git; el help lo dice.
- **Flags nuevos de `wt remove`**:
  - `--keep-branch`: conserva la rama aunque esté mergeada.
  - `-D, --force-delete-branch`: borra la rama aunque no esté mergeada.
  - `-f, --force`: borra el worktree aunque tenga archivos modificados o sin trackear. **Nunca** borra una rama sin mergear: para eso está `-D`. Los dos forzados son independientes, a propósito.
  - `--keep-branch` con `-D` es error de uso (exit 2).
- **"Mergeada"**: la punta de la rama es alcanzable desde la base (`default_base`, o la rama por defecto del repo: la misma base que usa `wt create`) o desde su upstream, si tiene uno. Se juzga con las referencias que ya hay, sin fetch. Un squash-merge no cuenta: la rama se conserva y se borra con `-D`.
- **Nuevo comando `wt lock <target> [<reason>]`**: lockea un worktree, con un motivo opcional. Un worktree lockeado no se borra ni se poda. También sirve para un worktree cuyo directorio no está (un disco externo desmontado), que es para lo que git lo tiene.
- **Nuevo comando `wt unlock <target>`**: lo deslockea.
  - Lockear uno ya lockeado o deslockear uno que no lo está: exit 5. El principal no se lockea: exit 2.
- **Nuevo comando `wt prune`**: olvida los worktrees cuyo directorio ya no existe (los que `wt list` marca `prunable`) y lista los que olvidó. No toca ramas.
- **Completions**: `wt remove` y `wt lock` ofrecen los worktrees no principales y no lockeados; `wt unlock`, los lockeados.
- **Nuevos esquemas JSON**: `wt.remove.v1`, `wt.lock.v1`, `wt.unlock.v1` y `wt.prune.v1`, iguales con y sin `--dry-run`.
- **Claves de config**: ninguna nueva ni modificada. `wt remove` lee `default_base`, que ya existe.
- Se quita: nada. Ningún comando ni flag existente cambia.

**Cambio respecto de `docs/design/product.md` §4**, decidido por el autor al proponer este change: la firma era `wt remove <name> [--delete-branch] [--force]` (la rama se conservaba por defecto). Ahora la rama mergeada se borra por defecto, como en worktrunk, y los forzados se separan como en v0.9 y en git (`-f` para el worktree, `-D` para la rama). `--delete-branch` no nace: sería el comportamiento por defecto. Los hooks `pre-remove`/`post-remove` que nombra §4 llegan con `hooks`, en M3.

## Capabilities

### New Capabilities

- `remove-worktree`: `wt remove` de punta a punta (resolución, bloqueos, rama, borrado, movimiento de la shell, `--json`, `--dry-run`, `-C`), `wt lock`, `wt unlock`, `wt prune` y sus completions.

### Modified Capabilities

- `git-worktrees`: nuevo requirement "Merged branch", la definición de rama mergeada. Hoy la usa `wt remove`; después la van a usar `merge-worktree` (M3) y `list-worktrees` completo (M4), por eso vive acá y no en `remove-worktree`, como "Default branch".

## Non-goals

- **Detectar squash-merges** (contenido ya integrado aunque los commits no sean ancestros): decidido fuera de alcance para M1. Una rama squash-mergeada se conserva y se borra con `-D`. Se puede sumar a la definición de "mergeada" con `merge-worktree` (M3).
- **Fetch antes de juzgar si una rama está mergeada**: `wt remove` no toca la red. Con una base remota vieja, una rama mergeada parece no estarlo y se conserva: es el lado seguro.
- **Hooks `pre-remove` y `post-remove`**: M3 (`hooks`).
- **Borrar varios worktrees de una** (`wt remove a b`, "todos los mergeados"): `batch-execution`, M2. **`wt remove` en otro repo del workspace**: M2.
- **Confirmación interactiva** (v0.9 preguntaba en PowerShell): `wt` no pregunta; la seguridad viene de los chequeos, y así lo pueden correr scripts y agentes.
- **Borrado en background** (worktrunk): `wt remove` termina cuando el worktree ya no está, y su exit code lo dice.
- **Borrar el directorio padre que queda vacío** (`repo.worktrees/` después del último worktree).
- **Que `wt prune` borre ramas** o acepte `--expire`: `wt prune` solo olvida registros, como `git worktree prune`. Para llevarse también la rama mergeada está `wt remove`.
- **Worktrees con submódulos inicializados**: git se niega a borrarlos sin `--force`; `wt` informa el error de git (exit 1) y `--force` lo resuelve.

## Milestone

M1, cuarto y último corte (`walking-skeleton` → `shell-integration` → `create-worktree` → **`remove-worktree`**). Ver `docs/design/product.md` §7: "`remove`, `lock`/`unlock`, `prune`. Cierra el ciclo mínimo: ya podés dejar v0.9 en macOS". Después de este change, M1 se cierra con `release/v0.1.0`, cuando se cumpla la definición de "listo" de §7 (una semana de uso en macOS sin volver a v0.9).

Este change actualiza `docs/design/product.md` §4: la firma de `wt remove` y la nota de que sus hooks llegan en M3.

Rama: `v0.1/remove-worktree`. Publica `v0.1.0-alpha.5`.

## Diferencias por OS

Sí, y se especifican por separado:

- **Directorio en uso.** En Windows no se puede borrar un directorio que es el directorio de trabajo de un proceso. `wt` deja el directorio del worktree antes de borrarlo (en los tres OS; solo Windows lo necesita), así que `wt remove @` desde adentro funciona igual en los tres. Pero si **otro** proceso está parado ahí (un editor, el agente, otra terminal), en Windows git no puede borrar la carpeta: `wt` sale con exit 1 y el mensaje de git, y un hint nombra lo que quedó. En macOS y Linux el borrado funciona y ese proceso queda en un directorio que ya no existe.
- **Mayúsculas**: "la shell está dentro del worktree" y "un worktree contiene a otro" comparan rutas sin distinguir mayúsculas en macOS y Windows, y distinguiéndolas en Linux, con la regla de "Current worktree" de `git-worktrees`. La resolución de `<target>` es exacta en todos los OS, como en `wt cd`.
- **Rutas**: se reportan nativas (`C:\src\repo.worktrees\feat` en Windows).

## Impact

- Código: `internal/git` suma las operaciones de remove, lock, unlock, prune, status, ancestro y upstream; `internal/worktree` suma las decisiones puras de remove (bloqueos, rama, a dónde va la shell) y de prune; `internal/cli` suma `remove`, `lock`, `unlock` y `prune`, saca de `cd` la resolución de `<target>` para compartirla, y `cli.Env` suma `Chdir`.
- Dependencias nuevas: ninguna.
- Contrato público: cuatro comandos, tres flags y cuatro esquemas JSON. Exit codes sin cambios: 5 ya es "bloqueado por estado (worktree sucio, lock)".
- CI: sin jobs nuevos. Los tests por shell real suman `wt remove @` a través de la función en zsh, bash, fish y pwsh; los tests del binario suman `wt remove @` con el proceso parado dentro del worktree, que en `windows-latest` es la prueba de que `wt` deja el directorio.
- Documentación: `docs/design/product.md` (§4), `CHANGELOG.md`.
