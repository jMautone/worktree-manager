# Design: sync-worktrees

## Context

El por qué está en `proposal.md`, y el comportamiento en `specs/`. Lo que ya existe y moldea el diseño:

- **Resolver un target** ya está hecho para `wt remove`: `a.resolve(ctx, dir, target, modeWorktree)` devuelve el worktree, su repo (`res.Repo`, con sus worktrees), el git dir y la config de ese repo, con `^`, `@`, `<repo>:<name>`, los otros repos y todos los errores (exit 3 y 4).
- **La base de un repo** se calcula hoy adentro de `readMerged` (`remove.go`):
  - `default_base` de la config de ese repo;
  - si está vacía, `readDefaultBranch`;
  - si no hay rama por defecto, un warning.
- **El fetch** de `wt create` es `worktree.FetchRemote(base, remotes)` (puro, el remote más largo que prefija la base) más `git.Fetch`. Si falla, da un warning y sigue.
- **`git.Status`** ya lista los cambios con `--porcelain -z`, sin versionar incluidos, para el chequeo de `wt remove`.
- **El Ctrl-C de `wt each`**:
  - `signal.Notify(interrupts, os.Interrupt)` evita que Go mate a `wt`;
  - el hijo recibe la señal porque está en el mismo grupo de procesos, o en la misma consola en Windows;
  - `pending(interrupts)` se consulta entre un worktree y el siguiente.
- **El reporte de `wt each`**: `eachReport` arma el `*Error` con el resumen y un hint por fallo.
- **Los labels** (`^`, `NAME`, `<repo>:<name>`) ya los arma `worktree.EachTarget`.

## Goals / Non-Goals

**Goals:**
- Que la decisión "¿qué hago con este worktree?" sea pura: sale del estado leído, y de ella salen la línea de salida, el `--dry-run` y el exit code, sin un `if` por modo.
- Que ningún camino deje un worktree con un rebase a medio hacer, ni con un archivo distinto del que tenía, salvo un rebase terminado.
- Que `merge-worktree` pueda llamar al rebase de un worktree sin traerse el resto de `wt sync`.

**Non-Goals:**
- Paralelismo, `--all-repos`, stash, hooks (ver el proposal).
- Interpretar los mensajes de git: un fallo que no es conflicto se informa con la primera línea de git, sin clasificarlo.

## Decisions

### D1. La decisión por worktree es pura

En `internal/worktree/sync.go`, sin git ni filesystem:

```go
type SyncState struct {
	Missing    bool   // el directorio no existe
	Detached   bool
	Dirty      bool   // cambios en archivos versionados
	InProgress string // "rebase", "merge", "cherry-pick", "revert" o ""
	Contains   bool   // el commit de la base es HEAD o un ancestro
}

type SyncAction int // SyncSkip, SyncUpToDate, SyncRebase

func DecideSync(s SyncState) (SyncAction, string /* motivo del skip */)

type SyncOutcome int // Rebased, UpToDate, Skipped, Conflict, Failed
func SyncExit(outcomes []SyncOutcome) int // 0, 5, 6 o 1, en ese orden de prioridad inversa
```

- El orden de los chequeos de `DecideSync` es el de la spec: falta el directorio, HEAD detached, operación en curso, sucio y al día. Así, un worktree sin directorio no se consulta nada más.
- `SyncTargets(worktrees, all bool)` elige los de `--all`: saca el principal, el bare y los detached, y respeta el orden de `wt list`. También es pura.
- Las líneas de salida (`rebased`, `is up to date with`, `skipped`, `conflict in`, `failed`, y sus `would ...`) salen de una función pura `SyncLine(label, base, outcome, reason, dryRun)`, con test de tabla.

*Alternativa descartada:* decidir mientras se lee el estado. Mezcla lectura y decisión, y el `--dry-run` terminaría siendo un conjunto de `if`.

### D2. Primero se resuelve todo, después se toca

`a.sync` corre en tres fases. Ningún rebase empieza hasta que la fase 2 termina sin error:

```
  1. targets   nombres -> a.resolve (como remove, sin rechazar el principal)
               o --all -> SyncTargets(repo actual); dedup por ruta
  2. repos     por repo, en orden de aparición:
                 base   = --base | default_base | readDefaultBranch   (exit 3 si no hay)
                 fetch  = FetchRemote(base, remotes), una vez, salvo --no-fetch o --dry-run
                 commit = ResolveCommit(base)                         (exit 3 si no resuelve)
  3. worktrees en orden: leer SyncState -> DecideSync -> rebasear o no -> línea
```

- **Un commit por repo**: el rebase usa el commit resuelto en la fase 2, no el nombre. Todos los worktrees de un repo se rebasean sobre el mismo commit, aunque otro proceso fetchee en el medio.
- **La base sale de una función compartida**: `a.repoBase(ctx, dir, cfg, main) (base string, ok bool, err error)`, extraída de `readMerged`. `wt remove` la sigue usando igual, con su warning cuando `ok` es falso. `wt sync`, en cambio, falla con exit 3 y los mismos hints que `wt create` (`--base`, `default_base`).
- **El fetch se hace en la fase 2**, antes del primer rebase. Así lo que dice la spec ("before any rebase") vale para todos los repos, y la base que solo existe en el remote ya se puede resolver.

*Alternativa descartada:* fetchear y rebasear repo por repo. Un `--base` que no existe en el segundo repo se descubriría después de haber rebaseado el primero.

### D3. Leer el estado

Hay dos lecturas nuevas en `internal/git`:

- **`TrackedChanges(ctx, r, dir) (bool, error)`**: `git status --porcelain -z --untracked-files=no --ignore-submodules=none`, y devuelve si hay alguna entrada. Va aparte de `Status` porque `wt remove` sí cuenta los sin versionar y `wt sync` no.
- **`InProgress(ctx, r, dir) (string, error)`**:
  - corre `git rev-parse --git-path rebase-merge --git-path rebase-apply --git-path MERGE_HEAD --git-path CHERRY_PICK_HEAD --git-path REVERT_HEAD`;
  - mira con `os.Stat` cuál existe;
  - `--git-path` da la ruta del git dir propio del worktree vinculado, así que funciona igual en el principal y en los vinculados, en los tres OS.

`Contains` reusa `git.IsAncestor(commitBase, head)`, y `Missing` y `Detached` ya vienen del `worktree.Worktree`.

### D4. Rebasear y deshacer

En `internal/git/rebase.go`:

```go
// Rebase corre, en dir:
//   git -c rebase.autoStash=false -c rebase.updateRefs=false rebase --no-autostash <commit>
func Rebase(ctx context.Context, r Runner, dir, commit string) error
func RebaseAbort(ctx context.Context, r Runner, dir string) error
func Unmerged(ctx context.Context, r Runner, dir string) (bool, error) // git diff --name-only --diff-filter=U
```

Después de un `Rebase` que falla, `a.syncOne` hace esto:

1. si `Unmerged` da verdadero, es un **conflicto**;
2. si no, es un **fallo**, con la primera línea de git (`firstGitLine`);
3. en los dos casos, si `InProgress` da `rebase`, corre `RebaseAbort`;
4. si el abort falla, el resultado es un **fallo** con el mensaje del abort y el hint `git rebase --abort` en esa ruta. Es lo único que puede dejar un rebase a medio hacer, y queda dicho.

La configuración del rebase:

- `-c rebase.autoStash=false` y `--no-autostash` son redundantes a propósito: el flag le gana a la config, y la config cubre un alias o un wrapper de git que ignore el flag.
- `-c rebase.updateRefs=false` evita que el rebase mueva otras ramas que apuntan a commits de esta. En un git viejo que no conoce la clave, git la ignora.
- No hay editor: un rebase no interactivo no lo abre.

`a.syncOne(ctx, target, base, commit) SyncOutcome` es la unidad que `merge-worktree` va a reusar: decide (D1), rebasea (D4) y devuelve el resultado, sin imprimir el resumen.

*Alternativas descartadas:*
- *Dejar el conflicto abierto*: con N worktrees, el usuario se encuentra rebases a medio hacer en lugares que no está mirando, y un `wt sync` siguiente los saltearía.
- *`git pull --rebase`*: usa el upstream de la rama, no la base, y fetchearía una vez por worktree.
- *`git merge`*: no es lo que pide el producto, y deja commits de merge en ramas que después se fast-forwardean.

### D5. Ctrl-C

Es el mismo mecanismo que `wt each`:

- `signal.Notify` antes de la fase 2;
- `pending(interrupts)` antes de cada fetch y de cada rebase, y una vez más al final.

Un git interrumpido sale con error. `a.syncOne` lo trata como cualquier fallo, o sea que aborta si dejó un rebase, pero el comando sale con 130 y el resumen `wt: interrupted: <k> of <m> worktrees not synced`.

- **macOS y Linux**: la SIGINT de la terminal le llega al grupo de procesos, git incluido. `wt` la recibe por el canal y no muere.
- **Windows**: el evento de Ctrl-C de la consola le llega a todos los procesos de la consola. Go lo entrega como `os.Interrupt`, y git termina por su cuenta.

En los dos casos el `RebaseAbort` corre después de que git salió, así que no compite con él por el índice.

### D6. Reporte, exit code y JSON

- **`syncReport(results, planned, interrupted) error`**:
  - es el equivalente de `eachReport`;
  - devuelve nil cuando `SyncExit` da 0;
  - si no, un `*Error` con `Code` según `SyncExit` (o 130), `Msg` con `<k> of <m> worktrees not synced` y un hint por worktree.
- **Las líneas por worktree** van a stdout a medida que termina cada uno, sin buffer: un `wt sync --all` largo muestra progreso.
- **`--json`**:
  - con exit 0, imprime `wt.sync.v1`;
  - con otro exit code, el `*Error` sale como `wt.error.v1` por el camino de siempre, y no se imprime ninguna línea por worktree;
  - esto respeta "Machine-readable output" de `cli-contract` sin modificarlo.

*Alternativa descartada:* imprimir `wt.sync.v1` también con exit 5 o 6. Es más útil para un script, pero cambia el contrato de `--json` para todos los comandos. Si hace falta, va en su propio change.

### D7. Comando y completions

- **`internal/cli/sync.go`**:
  - `syncCommand` con `--all`, `--base` y `--no-fetch`;
  - las validaciones de uso en `Args`: `--all` con nombres, `--base` vacío;
  - el help (`Short`, `Long`) explica la base, el fetch, qué se saltea y por qué nunca stashea.
- **Completions**:
  - `completeTargets(modeWorktree, filter)` con un filtro que deja afuera los detached y los nombres ya dados;
  - para `--base`, la misma función de `wt create`.

## Risks / Trade-offs

- **Rebasear reescribe commits ya pusheados.** `wt sync` no pushea ni toca el upstream, pero la rama local diverge de la remota. Es lo mismo que hace `git rebase`, y el help lo dice.
- **Un archivo bloqueado en Windows** (un editor con el archivo abierto) puede hacer fallar el rebase o el abort. El rebase falla como cualquier otro. Un abort que falla es el único caso que deja un rebase en curso, y el hint dice cómo terminarlo. El CI de `windows-latest` cubre el camino normal; el del archivo bloqueado no tiene test automático.
- **Sin versionar que chocan con la base**: git corta el rebase antes de pisar el archivo, y se informa como `failed`. La spec tiene el escenario.
- **`--no-fetch` y la base local**: con `--base main` no se fetchea nada, y `main` puede estar atrás de `origin/main`. Es lo que se pidió, y la línea dice sobre qué base se rebaseó.
- **Costo**: por worktree, un `status`, un `rev-parse --git-path`, un `merge-base --is-ancestor` y, si hace falta, el rebase. Con decenas de worktrees sigue dominando el fetch, que es uno por repo.
