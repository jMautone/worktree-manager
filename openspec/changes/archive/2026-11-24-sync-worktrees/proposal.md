# Proposal: sync-worktrees

## Why

Con varios worktrees abiertos a la vez, cada uno se queda atrás de `main` a su ritmo. Ponerlos al día hoy es entrar a cada uno y correr `git fetch` y `git rebase origin/main`, un fetch por worktree aunque sea el mismo remote, y acordarse de no hacerlo en el que tiene cambios sin commitear. `wt each 'git rebase origin/main'` no alcanza: no saltea los sucios, deja un rebase a medio hacer cuando hay un conflicto y no dice al final qué quedó al día y qué no.

Va antes de `merge-worktree` porque `merge` empieza con este mismo rebase: así el rebase y su política de conflicto se prueban solos, sin hooks, fast-forward ni borrado.

## What Changes

**Se agrega**

- **`wt sync [<name>...] [--all] [--base <ref>] [--no-fetch]`**: rebasea uno o varios worktrees sobre su base.
  - **Qué worktrees**:
    - sin argumentos, el actual;
    - con nombres, esos, resueltos como `wt remove` resuelve su target: `^`, `@`, `<repo>:<name>` y otros repos;
    - con `--all`, todos los worktrees vinculados del repo actual que tienen una rama, en el orden de `wt list`. El principal no entra: hay que nombrarlo con `^`.
  - **La base**:
    - la misma que usa `wt remove` para decidir si una rama está mergeada, o sea `default_base` o, si está vacía, la rama por defecto del repo;
    - es la base del repo de cada worktree, con su `.wt.toml`;
    - `--base <ref>` la pisa para todos.
  - **Un solo fetch por repo**:
    - si la base empieza con el nombre de un remote (`origin/main`), ese remote se fetchea una vez antes de rebasear el primer worktree de ese repo;
    - `--no-fetch` no fetchea;
    - un fetch que falla da un warning y sigue con las referencias que hay, como en `wt create`.
  - **Nunca stashea**:
    - un worktree con cambios en archivos versionados, staged o no, se saltea;
    - también se saltean uno con HEAD detached, uno con un rebase o un merge en curso, y uno cuyo directorio no existe;
    - los archivos sin versionar no frenan el rebase.
  - **Un worktree que ya contiene la base** no se toca y se informa como al día.
  - **El conflicto se reporta y se sigue**: el rebase con conflicto se aborta y ese worktree queda exactamente como estaba. Después se sigue con el siguiente. Un rebase que git corta por otro motivo se aborta igual y cuenta como fallido.
  - **Resumen y exit code**:
    - por cada worktree, una línea en stdout (`rebased`, al día, `skipped` o `conflict`);
    - si alguno no quedó al día, al final va a stderr `wt: <k> of <m> worktrees not synced`, con un `hint:` por cada uno;
    - el exit code es 1 si alguno falló, si no 6 si hubo un conflicto, si no 5 si alguno se salteó, y si no 0.
  - **Ctrl-C** espera a que termine el git en curso. Si ese git dejó un rebase a medio hacer, lo aborta, no empieza el siguiente worktree y sale con 130.
  - **`--dry-run`**: hace todas las lecturas y no fetchea ni rebasea. Imprime `would fetch`, `would rebase`, lo que está al día y lo que se saltearía.
  - **`--json`**: si todo quedó al día, el esquema `wt.sync.v1`. Si no, `wt.error.v1` con los mismos hints, como pide el contrato.
  - **Completions** de nombres como las de `wt remove`, y de refs para `--base` como las de `wt create`.

**Se modifica**

- `docs/design/product.md` §4 (la fila de `wt sync` con sus flags) y §7 si la fila de `sync-worktrees` cambia.

**Se quita**

- Nada. Ninguna clave de config nueva, ningún exit code nuevo, ningún evento de hook nuevo.

## Capabilities

### New Capabilities

- `sync-worktrees`: cómo se ponen al día uno o varios worktrees sobre su base sin perder trabajo. Cubre:
  - qué worktrees se sincronizan;
  - cuál es la base y cuándo se fetchea;
  - qué se saltea y por qué;
  - qué pasa con un conflicto o con un Ctrl-C;
  - la salida, el resumen y el exit code;
  - `--dry-run`, `--json` y las completions.

### Modified Capabilities

- Ninguna. Los exit codes 5, 6 y 130 ya están en `cli-contract` con este significado. La base y la rama por defecto ya están en `git-worktrees`, y `wt sync` las usa sin cambiarlas.

## Non-goals

- **Stashear** (ni con un flag): un worktree sucio se saltea. El que quiera, commitea o stashea a mano.
- **Dejar el conflicto abierto para resolverlo**: `wt sync` lo aborta. Para resolverlo se corre `git rebase <base>` en ese worktree; el hint lo dice. `merge-worktree` decide su propia política.
- **Hooks** (`pre-sync`/`post-sync`): ningún evento nuevo. `--no-hooks` no existe en `wt sync`.
- **Merge en vez de rebase**, o `--onto` con un punto de corte distinto de la base.
- **Push** de las ramas rebaseadas, ni actualizar su upstream.
- **`--all-repos`**: `--all` es el repo actual; para otros repos se nombran los worktrees con `<repo>:<name>`.
- **Recordar la base con la que se creó cada worktree**: hay una base por repo, la de `wt remove`, y `--base` para otra.
- **Paralelismo**: los worktrees se rebasean en serie.

## Milestone

M3, tercer corte (`hooks` → `project-hooks-approval` → **`sync-worktrees`** → `merge-worktree`). Ver la tabla "M3, desglosado en changes" en `docs/design/product.md` §7.

Rama: `v0.3/sync-worktrees`. Publica `v0.3.0-alpha.4`.

## Diferencias por OS

Ninguna en el comportamiento. La única diferencia es cómo llega el Ctrl-C:

- en macOS y Linux, una SIGINT al grupo de procesos;
- en Windows, el evento de consola.

En los dos casos le llega también al git en curso, y `wt` espera a que salga, como en `wt each`.

## Impact

- **Código**:
  - `internal/worktree` suma la decisión pura: qué hacer con cada worktree según su estado, y el exit code del resumen;
  - `internal/git` suma el rebase, su abort y la detección de un rebase o merge en curso;
  - `internal/cli` suma el comando `wt sync`, que reusa la resolución de targets de `wt remove`, el cálculo de la base de `wt remove` y el fetch de `wt create`.
- **Dependencias nuevas**: ninguna.
- **Contrato público**: un comando y un esquema JSON nuevo (`wt.sync.v1`). Usa los exit codes 5, 6 y 130, que ya existen.
- **CI**: sin jobs nuevos.
- **Documentación**: `docs/design/product.md` §4 y §7, el help de `wt sync`, `README.md` (la lista de comandos) y `CHANGELOG.md`.
