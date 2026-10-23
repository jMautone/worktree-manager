# Proposal: hooks

## Why

Un worktree recién creado no tiene `.env`, ni `node_modules`, ni nada de lo que el repo no versiona. Hoy, después de cada `wt create`, hay que copiar el `.env` y correr `npm ci` a mano antes de que el árbol compile. Si el agente arrancó con `-x`, arranca en un árbol que no corre. Es la fricción diaria más grande que queda, y v0.9 ya la resolvía con `postCreate`. Lo mismo pasa al cerrar: el contenedor de la base de datos del worktree o el dev server quedan vivos después de `wt remove`.

Este change es el primer corte de M3. Agrega hooks de usuario a los tres comandos que ya existen (`create`, `cd` y `remove`) y deja listo el mecanismo que `merge` va a reusar. Los `post-*` en background son el riesgo multiplataforma de M3: un proceso que sobrevive a `wt` en Windows. Por eso conviene descubrir temprano si se rompe.

## What Changes

**Se agrega**

- **Seis eventos de hook**: `pre-create`, `post-create`, `pre-cd`, `post-cd`, `pre-remove` y `post-remove`.
  - Cada evento lo dispara solo su comando. `wt create` y `wt remove` también mueven la shell, pero no disparan los hooks de `cd`.
  - Los eventos `merge` y `commit` llegan con sus comandos (`merge-worktree` en M3, `merge-steps` en M4).
- **Configuración**: una tabla `[hooks]` en la config del usuario, con una lista de comandos por evento.

  ```toml
  [hooks]
  post-create = ['cp "{repo_path}/.env" .env', "npm ci"]
  post-remove = ["docker rm -f {repo}-{name|sanitize}-db"]
  ```

  - Son seis claves de config nuevas, `hooks.<evento>`, con default `[]`.
  - No tienen variable de entorno.
  - El repo no puede declararlas: un `[hooks]` en `.wt.toml` se ignora con un warning hasta `project-hooks-approval`.
  - `wt config get hooks.post-create` imprime un comando por línea, como las otras listas.
- **Cómo corre un hook**:
  - Usa el intérprete de `-x`: `sh -c` en macOS y Linux y `cmd.exe` en Windows.
  - Los comandos de un evento corren en orden, y el primero que falla corta la lista.
  - No reciben las variables del protocolo de la shell.
- **Bloqueantes** (`pre-*` y `post-create`):
  - Corren en la terminal, con una cabecera `==> <evento>  <comando>` por comando.
  - Su salida va a stderr, para que stdout siga siendo solo de `wt` (texto o JSON).
  - Si fallan, la operación termina con **exit 7**:
    - un `pre-*` que falla no deja nada hecho: no se crea, no se borra y no se mueve la shell;
    - si falla `post-create`, el worktree queda creado y la shell se mueve igual, pero `-x` no corre. Un hint da la ruta.
- **En background** (`post-cd` y `post-remove`):
  - Arrancan desacoplados de la terminal y `wt` no los espera.
  - Siguen vivos si `wt` termina, si apretás Ctrl-C o si cerrás la terminal, en los tres OS.
  - `wt` imprime en stderr dónde quedó el log, y su exit code no cambia por ellos.
- **Logs**:
  - Van en `<git-common-dir>/wt/logs/<name|sanitize>/<evento>.log`, uno por worktree y evento, y cada corrida lo pisa.
  - Para cada comando, el log tiene una cabecera, la salida y el exit code.
  - Como viven en el directorio git compartido, sobreviven al `remove` del worktree.
- **Plantillas en los comandos**:
  - `{repo}`, `{repo_parent}`, `{repo_path}`, `{name}`, `{branch}` y `{path}` en los seis eventos, y `{base}` solo en `create`.
  - `{name}` es el NAME del worktree, el que muestra `wt list`, y es el mismo en `post-create` y en `post-remove`.
  - Los valores se insertan tal cual: las comillas las pone el usuario.
  - Un comando con una plantilla inválida es config inválida (exit 1), igual que `worktree_path`.
- **Flag `--no-hooks`** en `wt create`, `wt cd` y `wt remove`: no corre ningún hook en esa corrida.
- **`--dry-run`** muestra los comandos ya renderizados que correría (`would run pre-create hook: …`), en orden, y no corre ninguno.
- **`--json`**:
  - Los hooks bloqueantes escriben su salida en su log y no en la terminal, para que stdout y stderr sigan teniendo un solo documento.
  - Si uno falla, el `wt.error.v1` trae `code` 7 y el log en `hints`.
- **Ctrl-C durante un hook bloqueante**: llega al hook, `wt` espera a que termine, no sigue con la operación y sale con **130**.

**Se modifica**

- **Plantillas**: `{{` y `}}` escriben una llave literal en cualquier plantilla, incluida `worktree_path`. Hacía falta para `awk '{{print $1}}'` o `${{HOME}}` en un hook. Ninguna plantilla válida hoy cambia de significado, porque `{{` era un error.
- **`wt create`**:
  - El orden pasa a ser chequeos → fetch → `pre-create` → rama y worktree → `post-create` → directiva → `-x`.
  - La línea `created worktree …` sale después de `post-create`.
  - `-x` no corre si un hook falló.
- **`wt remove`**: el orden pasa a ser chequeos → `pre-remove` → borrado → rama → directiva → `post-remove`.
- **`wt cd`**: `pre-cd` corre antes de escribir la directiva, y `post-cd` después. Ninguno de los dos corre si el destino no está dentro de un worktree (por ejemplo, un `wt cd -` a un directorio suelto).
- **Contrato de CLI**: el formato de error suma una excepción, como la de `wt each`. Si un comando falla después de correr hooks bloqueantes, stderr ya tiene sus cabeceras y su salida antes de la línea `wt: …`.
- **`docs/design/product.md`**:
  - §5 dice hoy que todos los `post-*` corren en background. Pasa a decir que `post-create` bloquea, con el motivo: `-x` no puede arrancar antes de `npm ci`.
  - §5 suma el escape de llaves.
  - §4 nombra los hooks en las filas de `create` y `remove`.
  - §7 actualiza la fila de `hooks`.

**Se quita**

- Nada del contrato actual. No hay esquemas JSON nuevos ni campos nuevos en los que ya existen.

## Capabilities

### New Capabilities

- `hooks`: qué corre automáticamente y cuándo. Cubre:
  - los eventos y qué comando dispara cada uno;
  - dónde corre cada hook y en qué orden respecto de la operación;
  - las variables de sus plantillas;
  - los bloqueantes y los de background;
  - los logs, el exit 7 y qué queda hecho cuando un hook falla;
  - Ctrl-C, `--no-hooks`, `--dry-run` y `--json`.

### Modified Capabilities

- `config-layers`: las claves `hooks.<evento>`, la tabla `[hooks]`, las listas de comandos y su validación, y que esas claves no tienen variable de entorno.
- `path-templates`: `{{` y `}}` como llaves literales.
- `create-worktree`: el flag `--no-hooks`, el orden con `pre-create` y `post-create`, `-x` después de los hooks, y las líneas de `--dry-run`.
- `remove-worktree`: el flag `--no-hooks`, el orden con `pre-remove` y `post-remove`, y las líneas de `--dry-run`.
- `navigate`: `pre-cd` antes de la directiva, `post-cd` después, y las líneas de `--dry-run`.
- `cli-contract`: la excepción del formato de error cuando corrieron hooks bloqueantes.

## Non-goals

- **Hooks del repo** (`[hooks]` en `.wt.toml`), `wt hook list` y `wt hook approve`: son `project-hooks-approval`, el próximo change.
- **Los eventos `merge` y `commit`**: un `pre-merge` en la config da el warning de clave desconocida hasta que llega `merge-worktree`.
- **Elegir por comando si va en background**, comandos con nombre o comandos en paralelo dentro de un evento: un evento es una lista de strings que corren en serie.
- **Variables de entorno con los valores** (`WT_HOOK_PATH`, …) y comillas automáticas en las plantillas: los valores van tal cual, y las comillas las pone el usuario.
- **Retención de logs**, `wt hook log` o ver qué hooks siguen corriendo: el log se pisa en cada corrida.
- **Hooks en `lock`, `unlock`, `prune`, `path` y `each`**.
- **Timeouts**: un hook bloqueante colgado se corta con Ctrl-C.
- **Un equivalente cross-OS de `copyOnCreate` de v0.9**: `cp` o `copy` en `post-create`, según el OS de la máquina, porque la config del usuario es de esa máquina.
- **Variables y filtros que todavía no existen** (`{default_branch}`, `{upstream}`, `{vars.X}`, `hash_port`): llegan con sus capabilities.

## Milestone

M3, primer corte (**`hooks`** → `project-hooks-approval` → `sync-worktrees` → `merge-worktree`). Ver la tabla "M3, desglosado en changes" en `docs/design/product.md` §7.

Esa fila dice que los `post-*` corren en background. Este change deja `post-create` bloqueante y actualiza la fila y §5. Los otros `post-*` siguen en background, así que el riesgo multiplataforma sigue en este corte.

Rama: `v0.3/hooks`. Publica `v0.3.0-alpha.1`.

## Diferencias por OS

Sí, y son explícitas:

- **Intérprete**: `sh -c` en macOS y Linux, `cmd.exe` en Windows. Es el mismo de `-x`, así que un comando como `cp` no es portable entre OS. La config del usuario es de cada máquina.
- **Background desacoplado**:
  - En macOS y Linux, el proceso arranca en una sesión nueva, sin terminal controlante, así que no le llegan ni SIGINT ni SIGHUP.
  - En Windows arranca con una consola propia sin ventana y en un grupo de procesos nuevo, y sale del job object del padre cuando el job lo permite. Así, ni Ctrl-C ni cerrar la pestaña lo terminan.
- **Ctrl-C** en un hook bloqueante: el mismo comportamiento y el mismo 130 en los tres OS, como `wt each`.
- **Rutas**: nativas en las cabeceras, en los logs, en `{path}` y en los hints.

## Impact

- **Código**:
  - `internal/template` suma el escape `{{`/`}}`.
  - Un paquete nuevo `internal/hook` suma, todo puro:
    - los eventos;
    - las variables de cada uno;
    - el render;
    - la ruta del log;
    - el plan de qué corre, dónde y cómo.
  - `internal/config`:
    - las claves `hooks.*`;
    - el aplanado de la tabla `[hooks]`;
    - la validación de listas de comandos;
    - la marca de clave sin variable de entorno.
  - `internal/process` suma el arranque desacoplado por OS.
  - `internal/git` suma `git rev-parse --git-common-dir`.
  - `internal/cli`:
    - `--no-hooks`;
    - los hooks en `create`, `cd` y `remove`;
    - un comando interno y oculto que corre los hooks de background y escribe su log.
- **Dependencias nuevas**: ninguna.
- **Contrato público**:
  - seis claves de config;
  - un flag en tres comandos;
  - el escape de llaves en las plantillas;
  - una excepción en el formato de error;
  - el uso del exit 7, que ya estaba reservado.
- **CI**: sin jobs nuevos. Los tests de background corren contra el binario real en los tres OS. La supervivencia al cierre de la terminal en Windows se verifica a mano.
- **Documentación**: `docs/design/product.md` §4, §5 y §7, y `CHANGELOG.md`.
