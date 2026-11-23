# Proposal: project-hooks-approval

## Why

Hoy los hooks viven solo en la config del usuario, así que son los mismos para todos los repos: un `post-create = ["npm ci"]` corre también al crear un worktree de un repo de Go, y no hay forma de que un repo diga "al crear un worktree, copiá el `.env` y corré `npm ci`" para todo el que lo clona. El `[hooks]` de `.wt.toml` se ignora con un warning desde `hooks`.

Dejar que el repo declare hooks sin más es darle ejecución arbitraria a cualquier clone: alcanza con `git clone` y `wt create` para correr lo que el repo quiera. Este change permite los hooks del repo, pero cada comando corre solo después de que el usuario lo aprueba explícitamente, y se vuelve a pedir si el comando cambia. Va antes de `merge-worktree` para que `pre-merge` (los tests) sea del repo desde el primer día.

## What Changes

**Se agrega**

- **Hooks del repo**: la tabla `[hooks]` de `.wt.toml`, con los mismos seis eventos y la misma forma que la del usuario.

  ```toml
  # .wt.toml, versionado en el repo
  [hooks]
  post-create = ['cp "{repo_path}/.env" .env', "npm ci"]
  pre-remove  = ["docker compose down"]
  ```

  - Se lee del mismo `.wt.toml` que ya da la capa del repo: el del worktree actual, o el del worktree principal del otro repo cuando el comando opera en otro repo.
  - Se valida igual que la del usuario: un comando vacío o con una variable inválida es config inválida (exit 1), nombrando la clave y `.wt.toml`.
  - **Se suman, no se pisan**: en cada evento corren primero los comandos del usuario y después los del repo, como una sola lista. El primero que falla corta el resto.
- **Aprobación por comando**:
  - Un comando del repo corre solo si está aprobado para ese repo y ese evento. La aprobación es del texto del comando tal como está escrito en `.wt.toml`, antes de renderizar: una aprobación vale para todos los worktrees del repo.
  - Si el comando cambia (aunque sea un espacio), su hash cambia y hay que aprobarlo de nuevo.
  - Las aprobaciones se guardan del lado del usuario, en `approvals.toml`, en el mismo directorio que su `config.toml`. El repo no puede escribirlas.
- **Exit 8** (ya reservado en el contrato): si alguno de los eventos que dispararía el comando tiene un comando del repo sin aprobar, `wt create`, `wt cd` y `wt remove` fallan **antes de hacer nada**, con exit 8. El mensaje nombra el evento y el comando, y los hints dicen cómo aprobarlo (`wt hook approve`, con `-C` cuando es otro repo) y que `--no-hooks` corre sin hooks.
  - Se chequea al principio, aunque el hook sea `post-create`: nunca queda un worktree a medio crear por un hook sin aprobar.
  - `--dry-run` falla igual, con exit 8: muestra lo que pasaría de verdad.
  - `--no-hooks` no corre ningún hook y no chequea aprobaciones.
- **`wt hook list [--json]`**: lista, por evento, los comandos que correrían en el repo actual, con su origen (`user` o `repo`) y, para los del repo, si están aprobados. Con `--json`, el esquema `wt.hook.list.v1`.
- **`wt hook approve [<event>...] [--json]`**: aprueba los comandos del repo actual que todavía no están aprobados, todos o solo los de los eventos nombrados, e imprime uno por línea lo que aprobó. Con `--dry-run` muestra qué aprobaría y no escribe. Con `--json`, el esquema `wt.hook.approve.v1`.

**Se modifica**

- **`config-layers`**:
  - `[hooks]` en `.wt.toml` deja de dar el warning "repository configuration may not set it".
  - `wt config get` y `wt config list` siguen mostrando solo los hooks del usuario (`hooks.<evento>` con origen `default` o `user`): los del repo no pisan una clave, se suman, y se ven con `wt hook list`.
  - `wt config path` suma la ruta de `approvals.toml` y si existe. En JSON, `wt.config.path.v1` suma el campo `approvals` (un campo nuevo, sin cambio de versión).
- **`hooks`**: el requirement "Hook configuration" pasa a decir que un evento corre los comandos del usuario y después los del repo, y suma la aprobación y el exit 8. `--dry-run` muestra también los comandos del repo.
- **`docs/design/product.md`**: §4 (fila de `wt hook list` / `wt hook approve`), §5 (Hooks: dónde se guardan las aprobaciones y el orden usuario → repo) y §7 (la fila de `project-hooks-approval`, si cambia algo de lo que dice).

**Se quita**

- El warning por `[hooks]` en `.wt.toml`. Nada del contrato JSON ni de los exit codes.

## Capabilities

### New Capabilities

- `project-hooks-approval`: cómo se permiten hooks del repo sin confiar en el repo. Cubre:
  - qué comandos del repo necesitan aprobación y qué identifica a una aprobación (repo, evento y hash del comando);
  - dónde se guardan;
  - el exit 8 y cuándo se chequea;
  - `wt hook list` y `wt hook approve`, con sus esquemas JSON y su `--dry-run`.

### Modified Capabilities

- `hooks`: "Hook configuration" (los hooks del repo se suman a los del usuario, en ese orden) y "Dry run with hooks" (incluye los del repo).
- `config-layers`: "Hook commands" (`[hooks]` en `.wt.toml` se lee y se valida, sin warning, y `wt config` no lo muestra) e "Inspecting configuration" (`wt config path` muestra `approvals.toml`).

## Non-goals

- **Preguntar en la terminal** ("¿aprobar? [y/N]") en medio de un `wt create`: el comando falla con exit 8 y la aprobación es un paso explícito, `wt hook approve`. Un prompt se puede sumar después sin cambiar nada de esto.
- **Revocar aprobaciones** (`wt hook revoke`) y limpiar las viejas: una aprobación de un comando que ya no está en `.wt.toml` no hace nada. Se borra editando `approvals.toml`.
- **Aprobar "todo el repo"** o confiar en un repo por su remote: la aprobación es siempre por comando.
- **Hooks del usuario por repo** (overrides con wildcards en la config del usuario).
- **Desactivar los hooks del usuario desde el repo**: el repo no puede sacar lo que el usuario configuró.
- **Los eventos `merge` y `commit`**: llegan con sus comandos; cuando lleguen, la aprobación los cubre sin cambios.

## Milestone

M3, segundo corte (`hooks` → **`project-hooks-approval`** → `sync-worktrees` → `merge-worktree`). Ver la tabla "M3, desglosado en changes" en `docs/design/product.md` §7.

Rama: `v0.3/project-hooks-approval`. Publica `v0.3.0-alpha.3` (el `v0.3.0-alpha.2` salió del PR que solo traía estos artefactos).

## Diferencias por OS

Solo dónde vive `approvals.toml`, que sigue a la config del usuario:

- macOS y Linux: `$XDG_CONFIG_HOME/wt/approvals.toml` (default `~/.config/wt/approvals.toml`).
- Windows: `%APPDATA%\wt\approvals.toml`.
- Con `WT_CONFIG`, en el directorio de ese archivo, en los tres OS.

El repo de una aprobación es la ruta del worktree principal en forma nativa, tal como la informa git. En macOS y Windows, donde el filesystem no distingue mayúsculas, una ruta escrita distinto pide aprobar de nuevo: falla del lado seguro.

## Impact

- **Código**:
  - `internal/hook` suma, todo puro: el hash de un comando, el chequeo de aprobaciones de un plan y el armado de la lista usuario → repo.
  - Un paquete nuevo `internal/approval` lee y escribe `approvals.toml` (escritura atómica: archivo temporal y rename).
  - `internal/config`: la tabla `[hooks]` del repo se valida y se devuelve aparte, sin pasar por la precedencia de capas.
  - `internal/cli`: el chequeo en el `hookPlanner` que ya usan `create`, `cd` y `remove`; los comandos `wt hook list` y `wt hook approve`; la línea de `approvals` en `wt config path`.
- **Dependencias nuevas**: ninguna (`crypto/sha256` es de la stdlib; `go-toml` ya está).
- **Contrato público**: dos comandos, dos esquemas JSON nuevos, un campo nuevo en `wt.config.path.v1`, y el uso del exit 8, que ya estaba reservado.
- **CI**: sin jobs nuevos.
- **Documentación**: `docs/design/product.md` §4, §5 y §7, el help de `wt config` (que hoy dice que el repo no puede declarar hooks) y `CHANGELOG.md`.
