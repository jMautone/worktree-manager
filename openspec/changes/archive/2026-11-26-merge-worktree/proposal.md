# Proposal: merge-worktree

## Why

Cerrar un worktree hoy son cinco pasos a mano, en dos lugares distintos:

1. rebasear la rama sobre `main`;
2. correr los tests;
3. ir al worktree principal;
4. `git merge --ff-only feat`;
5. `wt remove feat`.

Si uno se olvida del rebase, el fast-forward falla. Si se olvida de los tests, entra a `main` algo roto. Y si los tests fallan después del merge, `main` ya se movió.

`wt merge` hace los cinco en orden y frena en el primero que falla, sin dejar nada a medio hacer. Los tres changes anteriores de M3 existen para esto: los hooks dan el lugar para los tests (`pre-merge`), la aprobación hace que esos tests sean del repo, y `sync-worktrees` ya resolvió el rebase y su política de conflicto.

## What Changes

**Se agrega**

- **`wt merge [<target>] [--no-rebase] [--no-remove] [--no-hooks]`**: mergea el worktree actual en el target y lo borra.
  - **Desde adentro**: mergea el worktree en el que está el shell, o el de `-C`. No toma un nombre de worktree. En el principal falla con exit 2 y, con HEAD detached, con exit 5.
  - **El target es una rama local**:
    - `<target>` si se da;
    - si no, la base del repo, la misma de `wt remove` y `wt sync`, sin el prefijo del remote: `origin/main` da `main`.
    - Un target que no es una rama local da exit 3, y la rama propia del worktree, exit 2.
  - **Chequeos antes de tocar nada**:
    - el worktree está limpio: sin cambios en archivos versionados ni archivos sin versionar, como los cuenta `wt remove`;
    - sin un rebase, merge, cherry-pick o revert en curso;
    - sin lock, salvo con `--no-remove`;
    - si el target está checkouteado en otro worktree, ese no tiene cambios en archivos versionados;
    - los hooks del repo que van a correr están aprobados (exit 8).

    Cualquiera que falle da exit 5 (u 8) y no cambia nada.
  - **Rebase**: si el target no es ancestro de HEAD, rebasea la rama sobre el target como `wt sync` rebasea un worktree, sin stash. Un conflicto se aborta y da exit 6, con el hint `git rebase <target>`. Con `--no-rebase`, una rama que no contiene el target da exit 5.
  - **`pre-merge`** corre en el worktree, después del rebase y antes de mover nada. Si falla, exit 7: la rama queda rebaseada y el target no se mueve.
  - **Fast-forward del target** al HEAD del worktree. Si el target está checkouteado en otro worktree, sus archivos se actualizan como con un `git merge --ff-only` ahí. Si el target ya contiene la rama, lo dice y sigue.
  - **Remove**: salvo `--no-remove`, borra el worktree como `wt remove`, con `pre-remove`, `post-remove`, sus líneas y el movimiento del shell al principal, y borra la rama, que ahora está en el target, sea cual sea la base.
  - **`post-merge`** corre al final, en background, en el worktree principal.
  - **Un fallo después de mover el target** (`pre-remove`, o el borrado) deja el target movido, sale como `wt remove` y un hint dice que la rama ya está mergeada.
  - **Ctrl-C** durante el rebase espera a git, aborta el rebase si quedó a medio hacer, no mueve el target y sale con 130.
  - **`--dry-run`** hace todos los chequeos y muestra `would rebase`, los hooks, `would merge` y lo que haría `wt remove --dry-run`.
  - **`--json`**: el esquema `wt.merge.v1`.
  - **Completions** del target: las ramas locales menos la propia.
- **Los eventos `pre-merge` y `post-merge`**, con las claves `hooks.pre-merge` y `hooks.post-merge`, y la variable `{target}` solo en esos dos eventos.

**Se modifica**

- `hooks`: la tabla de eventos pasa a ocho, `pre-merge` bloquea y `post-merge` va en background, `{target}` es una variable nueva, y `wt merge` dispara también `pre-remove` y `post-remove`.
- `config-layers`: las dos claves nuevas.
- `project-hooks-approval`: `wt merge` chequea las aprobaciones de sus eventos y, salvo `--no-remove`, las de los de `wt remove`.
- `docs/design/product.md` §4 (la fila de `wt merge`, que hoy lista `--squash`) y §7 si la fila de `merge-worktree` cambia.

**Se quita**

- Nada. Ningún exit code nuevo.

## Capabilities

### New Capabilities

- `merge-worktree`: cómo se cierra un worktree de una vez. Cubre:
  - qué worktree y qué target;
  - qué se chequea antes y en qué orden;
  - el rebase y su conflicto;
  - el fast-forward;
  - el remove;
  - los fallos a mitad de camino;
  - el Ctrl-C;
  - `--dry-run`, `--json` y las completions.

### Modified Capabilities

- `hooks`: los eventos `pre-merge` y `post-merge` y la variable `{target}`.
- `config-layers`: las claves `hooks.pre-merge` y `hooks.post-merge`.
- `project-hooks-approval`: qué eventos chequea `wt merge`.

## Non-goals

- **Commitear los cambios pendientes, o `--squash`**: un worktree sucio frena con exit 5. Los dos comparten el problema del mensaje de commit y llegan juntos con `merge-steps`, en M4 (ver `docs/design/product.md` §7).
- **Push** del target, ni fetch antes de mergear: `wt merge` mueve ramas locales. Para traer el remote está `wt sync`.
- **Un merge commit** (`--no-ff`): el target solo se fast-forwardea.
- **Detectar ramas squash-mergeadas**: llega con `list-worktrees`, en M4. Acá no hace falta, porque después del fast-forward la rama está en el target.
- **Dejar el conflicto abierto** para resolverlo: se aborta, como en `wt sync`. El hint dice cómo seguir a mano.
- **Mergear desde el principal** nombrando un worktree (`wt merge feat`): el único argumento es el target, y el worktree es el actual. `-C` sirve para otro.
- **Un target que es una rama remota**, o un target en otro repo.

## Milestone

M3, cuarto y último corte (`hooks` → `project-hooks-approval` → `sync-worktrees` → **`merge-worktree`**). Ver la tabla "M3, desglosado en changes" en `docs/design/product.md` §7.

Rama: `v0.3/merge-worktree`. Publica `v0.3.0-alpha.5`. El tag `v0.3.0` sale después de una semana de uso en macOS, no con este change.

## Diferencias por OS

Ninguna en el comportamiento. Las que ya existen se heredan sin cambios:

- **Ctrl-C**: en macOS y Linux, una SIGINT al grupo de procesos; en Windows, el evento de consola. En los dos casos le llega al git en curso y `wt` espera a que salga, como en `wt sync`.
- **El shell de los hooks**: `sh -c` o `cmd.exe /c`, como en `hooks`.
- **Borrar el worktree desde adentro**: en Windows, `wt` sale del directorio antes de borrarlo, como ya hace `wt remove`.

## Impact

- **Código**:
  - `internal/worktree` suma la decisión pura: el target por defecto a partir de la base, los chequeos y su orden, y las líneas de salida;
  - `internal/git` suma el fast-forward de una rama, checkouteada o no;
  - `internal/hook` suma los dos eventos y la variable `{target}`;
  - `internal/cli` suma el comando, que reusa el rebase de `wt sync` y el plan y el borrado de `wt remove`.
- **Dependencias nuevas**: ninguna.
- **Contrato público**: un comando, un esquema JSON (`wt.merge.v1`), dos claves de config y dos eventos de hook. Usa los exit codes 2, 3, 5, 6, 7, 8 y 130, que ya existen.
- **CI**: sin jobs nuevos.
- **Documentación**: `docs/design/product.md` §4 y §7, el help de `wt merge`, `README.md` (la lista de comandos) y `CHANGELOG.md`.
