# Proposal: merge-steps

## Why

`wt merge` cierra un worktree de una vez, pero solo si ya está todo commiteado. Hoy, al terminar una tarea, el flujo real es:

1. `git add -A && git commit` a mano, porque un worktree sucio frena `wt merge` con exit 5;
2. si la rama tiene diez commits de "wip", `git reset --soft $(git merge-base HEAD main)` y otro commit, a mano, para no meterlos así en `main`;
3. recién ahí, `wt merge`.

Y cuando no querés cerrar todo, sino un paso (rebasear sin mergear, o squashear sin borrar), no hay forma de pedirle a `wt` solo ese paso: hay que repetir a mano lo que `wt merge` ya sabe hacer, con sus chequeos y sus hooks.

M3 dejó esto afuera a propósito: el commit pendiente y `--squash` comparten el problema del mensaje (editor, `-m`, los mensajes de la rama) y se diseñan una sola vez, acá. Es la primera fricción diaria que quedó después de M3, y no depende de nada más de M4.

## What Changes

**Se agrega**

- **`wt step commit [-m <msg>] [--no-hooks]`**: commitea todo lo pendiente del worktree actual, archivos versionados y sin versionar (lo que cuenta `wt remove`; los ignorados no), en un commit. Sin nada pendiente dice `nothing to commit` y sale con 0.
- **`wt step squash [<target>] [-m <msg>] [--no-hooks]`**: reemplaza los commits propios de la rama (los que no están en el target) y lo pendiente por un solo commit, parado sobre el mismo merge-base. Con un solo commit propio y nada pendiente, o ninguno, dice `nothing to squash` y sale con 0.
- **`wt step rebase [<target>]`**: el rebase de `wt merge`, solo: sobre el target local, sin fetch, sin stash; el conflicto se aborta (exit 6).
- **`wt step push [<target>]`**: el fast-forward de `wt merge`, solo: mueve el target local al HEAD del worktree, entre `pre-merge` y `post-merge`. **No hace `git push`**: el nombre es el del paso del pipeline, y el help lo dice.
- **El mensaje de commit**, uno solo para `commit`, `squash` y `merge`:
  - con `-m <msg>`, ese mensaje, tal cual;
  - si no, el editor de git (el que git usaría para `git commit`: `GIT_EDITOR`, `core.editor`, `VISUAL`, `EDITOR`), en la terminal. Para `squash` arranca con los mensajes de los commits que se juntan, del más viejo al más nuevo;
  - un mensaje vacío aborta: exit 1, y nada cambia;
  - sin terminal, o con `--json`, y sin `-m`: exit 2 antes de tocar nada.
- **Los eventos `pre-commit` y `post-commit`**, con las claves `hooks.pre-commit` y `hooks.post-commit`. Los disparan `wt step commit`, `wt step squash` y `wt merge` cuando commitea o squashea. `pre-commit` bloquea y corre antes de pedir el mensaje, así un lint que falla no te hace escribir el mensaje dos veces. `post-commit` corre en background.
- **Nada queda a medio hacer**: si el commit falla después de empezar (mensaje vacío, un hook nativo de git que falla, Ctrl-C), la rama y el index vuelven a como estaban.
- **`--json`**: el esquema `wt.step.v1`, el mismo para los cuatro pasos.
- **Completions** del target de `squash`, `rebase` y `push`: las ramas locales menos la propia, como en `wt merge`.

**Se modifica**

- **`wt merge`** ya no frena con un worktree sucio: commitea lo pendiente, con `pre-commit` y `post-commit`, y sigue. El pipeline queda: chequeos → commit (o squash) → rebase → `pre-merge` → fast-forward → remove → `post-merge`. **BREAKING** respecto de `v0.3.0-alpha.5`: un worktree sucio daba exit 5 y ahora se commitea.
  - `--squash`: squashea en vez de commitear, antes del rebase.
  - `-m <msg>`: el mensaje del commit o del squash.
  - `--no-commit`: lo pendiente frena con exit 5, como hasta hoy.
  - `--dry-run` muestra `would commit` o `would squash` y sus hooks; `wt.merge.v1` suma los campos `committed` y `squashed` (agregar campos no cambia la versión del esquema).
- `hooks`: la tabla de eventos pasa a diez; `--no-hooks` lo aceptan también `wt merge`, `wt step commit`, `wt step squash` y `wt step push` (la spec de `hooks` todavía no lo decía para `wt merge`, aunque el comando ya lo acepta).
- `config-layers`: las dos claves nuevas. También se corrige un escenario que usaba `post-merge` como evento inexistente, que existe desde M3.
- `project-hooks-approval`: qué eventos chequea cada paso, y `wt merge` cuando va a commitear.
- `docs/design/product.md` §4 (las filas de `wt merge` y `wt step`) y §5 (los eventos de `commit`).

**Se quita**

- Nada. Ningún exit code nuevo.

## Capabilities

### New Capabilities

- `merge-steps`: los pasos sueltos del cierre de un worktree (`wt step commit|squash|rebase|push`) y el mensaje de commit que comparten con `wt merge`. Cubre qué worktree y qué target, los chequeos de cada paso, el mensaje (`-m`, editor, sin terminal), qué queda cuando un paso falla o se interrumpe, `--dry-run`, `--json` y las completions.

### Modified Capabilities

- `merge-worktree`: el commit pendiente, `--squash`, `-m` y `--no-commit`; los chequeos, el dry run y el JSON que cambian con ellos.
- `hooks`: los eventos `pre-commit` y `post-commit`, dónde corren, y quién acepta `--no-hooks`.
- `config-layers`: las claves `hooks.pre-commit` y `hooks.post-commit`.
- `project-hooks-approval`: los eventos que chequean `wt step` y `wt merge`.

## Non-goals

- **Generar el mensaje con un LLM**: llega con `llm-commits`, en M6. El mensaje se resuelve en un solo lugar para que ese change lo enchufe ahí.
- **`git push` al remote**, ni abrir un PR: `wt step push` mueve una rama local. Lo remoto es de `forge`, en M6.
- **Detectar ramas squash-mergeadas fuera de `wt`** (en GitHub): llega con `list-worktrees`, el siguiente change de M4.
- **Commitear solo una parte** (`git add -p`): `wt step commit` commitea todo lo pendiente. Para elegir, `git commit` a mano.
- **`--no-verify`**: los hooks nativos de git del repo corren siempre, porque `wt` commitea con `git commit`. `--no-hooks` saltea solo los de `wt`.
- **Correr un paso en el worktree principal**: `wt step` falla ahí con exit 2, como `wt merge`. Commitear en el principal es `git commit`.
- **Un fixup o un squash interactivo** (`rebase -i`): `squash` junta todo en uno.

## Milestone

M4, primer corte (**`merge-steps`** → `list-worktrees` → `branch-state` → `picker` → `statusline`). Ver la tabla "M4, desglosado en changes" en `docs/design/product.md` §7.

Rama: `v0.4/merge-steps`. Publica `v0.4.0-alpha.1`, y no puede mergearse hasta que salga `v0.3.0`: los milestones no se superponen.

## Diferencias por OS

Ninguna en el comportamiento. Las que hay son de mecanismo:

- **El editor** lo abre git, con su propia resolución en los dos OS: en Windows, Git for Windows corre el editor configurado (por defecto, el que eligió el instalador), y `wt` no arma ese comando.
- **Ctrl-C** mientras el editor está abierto: en macOS y Linux la SIGINT le llega al grupo de procesos, en Windows el evento de consola; en los dos casos `wt` espera a que git termine, deshace lo que haya empezado y sale con 130, como en `wt merge`.
- **"Sin terminal"** es la entrada estándar que no es una terminal, en los dos OS (en Windows, una consola).

## Impact

- **Código**:
  - `internal/worktree` suma la decisión pura: qué hace cada paso con el estado leído, sus bloqueos, el mensaje inicial del squash y las líneas de salida;
  - `internal/git` suma el stage completo, el commit con editor o con mensaje, el reset suave y la restauración del index;
  - `internal/hook` suma los dos eventos;
  - `internal/cli` suma `wt step` con sus cuatro subcomandos, y `wt merge` los compone.
- **Dependencias nuevas**: ninguna.
- **Contrato público**: un comando con cuatro subcomandos, un esquema JSON (`wt.step.v1`), dos campos en `wt.merge.v1`, tres flags en `wt merge`, dos claves de config y dos eventos de hook. Usa los exit codes 1, 2, 3, 5, 6, 7, 8 y 130, que ya existen.
- **CI**: sin jobs nuevos. Los tests del editor usan un `GIT_EDITOR` de prueba, en los tres OS.
- **Documentación**: `docs/design/product.md` §4 y §5, el help de `wt step` y `wt merge`, `README.md` (la lista de comandos) y `CHANGELOG.md`.
