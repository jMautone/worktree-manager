# Design: merge-worktree

## Context

El por qué está en `proposal.md`, y el comportamiento en `specs/`. Lo que ya existe y moldea el diseño:

- **El rebase de un worktree** está en `a.syncOne` (`internal/cli/sync.go`):
  - corre `git.Rebase` sobre un commit ya resuelto;
  - si falla, distingue conflicto (`git.Unmerged`) de fallo;
  - si quedó un rebase en curso, corre `git.RebaseAbort`.

  Las lecturas de estado (`git.TrackedChanges`, `git.InProgress`, `git.IsAncestor`) también existen.
- **El plan de `wt remove`** es `a.planRemove(ctx, target, flags)`. Resuelve el target, chequea con `worktree.CheckRemovable` (principal, lock, worktree anidado), cuenta los cambios con `git.Status` (sin versionar incluidos), decide la rama con `worktree.BranchOutcome`, decide si el shell se mueve y planea `pre-remove` y `post-remove`. **`a.remove(ctx, plan, main)`** lo ejecuta y escribe las líneas, o el JSON de `wt.remove.v1`, al final.
- **La base de un repo**: `configString(cfg, "default_base")` o, si está vacía, `a.defaultBranch`, que falla con exit 3 y los hints `--base` y `default_base`. Es la que usa `wt sync`. `worktree.FetchRemote(base, remotes)` da el remote que prefija una base.
- **Los hooks**:
  - `hookPlanner{...}.plan(event, values, dir)` arma un `hook.Run` y valida la plantilla;
  - `.check(events...)` falla con exit 8 si un hook del repo no está aprobado;
  - `a.runBlocking(run)` corre uno bloqueante (exit 7) y `a.background(run)` uno en background;
  - `hook.Vars(e)` dice qué variables acepta cada evento.
- **El Ctrl-C de `wt sync`**: `signal.Notify(interrupts, os.Interrupt)` más `pending(interrupts)` entre pasos.

## Goals / Non-Goals

**Goals:**
- Que todo lo que puede fallar por el estado del repo se lea y se chequee antes del primer efecto, igual que en `wt remove`. El primer efecto es el rebase.
- Que el único estado intermedio posible sea "rebaseada y no mergeada" o "mergeada y no borrada", y que el error lo diga.
- Reusar el rebase de `wt sync` y el borrado de `wt remove` sin copiarlos.

**Non-Goals:**
- Commit pendiente, `--squash`, push, fetch (ver el proposal).
- Clasificar los mensajes de git: un fallo que no es conflicto se informa con la primera línea de git.

## Decisions

### D1. La decisión pura

En `internal/worktree/merge.go`, sin git ni filesystem:

```go
// MergeTarget es el target por defecto: la base sin "<remote>/" cuando
// empieza con el nombre de un remote (el más largo, como FetchRemote).
func MergeTarget(base string, remotes []string) string

type MergeState struct {
	Detached      bool
	InProgress    string // como en SyncState
	Changes       bool   // versionados o sin versionar, como los cuenta wt remove
	TargetFound   bool   // existe refs/heads/<target>
	TargetIsOwn   bool   // el target es la rama del worktree
	TargetDirty   string // la ruta del worktree que tiene el target con cambios, o ""
	NoRebase      bool
	Contains      bool   // el commit del target es HEAD o un ancestro
}

// CheckMerge devuelve el primer chequeo que falla, con su exit code y su
// mensaje, en el orden de la spec; o nil.
func CheckMerge(branch, target string, s MergeState) *MergeBlock

// MergeLine arma "rebased ...", "merged ... into ...", "... is already in ...",
// y sus "would ..." bajo dryRun.
func MergeLine(step MergeStep, branch, target string, dryRun bool) string
```

- **El orden de `CheckMerge`**:
  1. operación en curso (5);
  2. HEAD detached (5). Va después de la operación en curso por lo mismo que en `DecideSync`: git informa como detached a un worktree parado en un rebase;
  3. el target no existe (3) o es la rama propia (2);
  4. cambios (5), target checkouteado con cambios (5);
  5. con `NoRebase` y sin `Contains`, `<branch> does not contain <target>` (5).

  El principal (2) y el lock (5) no están acá: los da `worktree.CheckRemovable`, que ya los mapea al contrato (D4).
- `MergeBlock` es `{Code int; Msg string; Hints []string}`, y `internal/cli` lo convierte en `*Error` sin lógica.

*Alternativa descartada:* reusar `DecideSync`. Sync saltea y sigue; merge falla. Sync no cuenta los sin versionar; merge sí. Compartir el tipo obligaría a un `if` por comando adentro de la decisión.

### D2. Dos fases: plan y efectos

`a.merge` corre en dos fases. Ningún efecto empieza hasta que el plan termina sin error:

```
  plan     1. worktree actual: a.resolve(ctx, dir, "@", modeWorktree)
           2. CheckRemovable sin --no-remove; con --no-remove, solo el principal
           3. target: <target> | MergeTarget(base, remotes); su commit
           4. MergeState -> CheckMerge
           5. hooks: plan de pre-merge y post-merge; con remove, planRemove (D4)
              check(pre-merge, post-merge [, pre-remove, post-remove])   -> exit 8
  efectos  6. rebase (D3), salvo --no-rebase o Contains
           7. pre-merge
           8. fast-forward (D5)
           9. remove (D4), salvo --no-remove
          10. post-merge en background
```

- **El worktree** se resuelve con `@`, la misma resolución que `wt remove @`. Así `-C` funciona y el error de "fuera de un repo" es el de siempre (exit 3).
- **La base** se calcula como en `wt sync` (`default_base` o `a.defaultBranch`), y `MergeTarget` le saca el remote. No hay fetch: el target es una rama local.
- **Las aprobaciones** se chequean en el paso 5, antes del rebase, con todos los eventos que van a correr. Así un `pre-remove` sin aprobar no deja el target movido.
- **El commit del target** se lee en el paso 3 y es el `old` del fast-forward (D5): si otro proceso mueve el target en el medio, el fast-forward falla en vez de pisarlo.

*Alternativa descartada:* planear el remove después del fast-forward. Un fallo de plan (un hook con una variable inválida, un worktree anidado) aparecería con el target ya movido.

### D3. El rebase, compartido con `wt sync`

Se extrae de `a.syncOne` la parte de efecto, sin cambiar `wt sync`:

```go
// rebaseOnto rebasea el worktree en dir sobre commit y, si falla, aborta
// el rebase que haya quedado. Devuelve Rebased, Conflict o Failed y, para
// Failed, el detalle.
func (a *app) rebaseOnto(ctx context.Context, dir, commit string) (worktree.SyncOutcome, string)
```

- `a.syncOne` la llama después de `DecideSync`, y `a.merge` después de `CheckMerge`.
- `a.merge` traduce el resultado:
  - `Conflict`: exit 6, `conflict rebasing <branch> onto <target>; rebase aborted`, hint `git rebase <target>`;
  - `Failed`: exit 1 con el detalle.
- El rebase va sobre el commit del target leído en el plan, no sobre el nombre.

### D4. El remove, compartido con `wt remove`

`a.planRemove` se parte en dos, sin cambiar `wt remove`:

```go
func (a *app) planRemove(ctx, target, flags)                            // resuelve, y llama a:
func (a *app) planRemoveOf(ctx, res, cfg, w, flags) (RemovePlan, main, error)
```

- **`wt merge`** llama a `planRemoveOf` con el worktree que ya resolvió y `forceDelete: true`. La rama se borra sin `readMerged`, porque después del fast-forward está en el target, sea cual sea la base. `git.DeleteBranch` ya usa `-D`.
- **El chequeo de cambios** de `planRemoveOf` repite el de `CheckMerge`. Es una lectura más, a cambio de no tocar el contrato de `wt remove`.
- **`CheckRemovable`** da, sin `--no-remove`, el principal (2), el lock (5) y un worktree anidado (5), con los mismos mensajes y hints de `wt remove`. Con `--no-remove` solo se chequea el principal: un worktree con lock se puede mergear sin borrarlo.
- **La salida**: `a.remove` recibe la función que escribe la salida, en el mismo punto donde hoy llama a `a.removed(plan)`: `wt remove` le pasa `a.removed`, y `wt merge` una que escribe las mismas líneas (`removedLines`) o nada con `--json`. Así las líneas salen en orden (`merged ...`, `removed ...`, `deleted ...`), antes de `post-remove` como en `wt remove`, y con `--json` hay un solo documento, `wt.merge.v1`. Las líneas de `--dry-run` salen igual de `previewRemoveLines`.
- **Un fallo de `a.remove`** (exit 7 de `pre-remove`, exit 1 del borrado) se devuelve tal cual, con un hint más: `<branch> was merged into <target>`.

### D5. El fast-forward

En `internal/git/merge.go`:

```go
// FastForward mueve branch de old a commit. Si branch está checkouteada en
// checkedOut (una ruta), corre ahí git merge --ff-only <commit>; si no,
// git update-ref refs/heads/<branch> <commit> <old>.
func FastForward(ctx context.Context, r Runner, dir, branch, old, commit, checkedOut string) error
```

- **Checkouteada** (lo común: `main` en el principal): `git merge --ff-only <commit>` en ese worktree actualiza la rama, el índice y los archivos a la vez, y git se niega antes de pisar un archivo sin versionar que choca. Con `-c merge.autoStash=false` por la misma razón que en el rebase. Antes de correrlo se confirma que la rama sigue en `old` (`git rev-parse`), porque `merge --ff-only` no tiene un `old`.
- **No checkouteada**: `git update-ref` con el valor viejo es atómico. Si el target se movió, falla y no pisa nada.
- **Un bare** no tiene archivos: el target nunca está checkouteado ahí, y va por `update-ref`.
- Si falla, `wt merge` sale con exit 1 y la primera línea de git, con la rama rebaseada y el target sin mover. Un hint dice que la rama quedó rebaseada.

*Alternativas descartadas:*
- *`git fetch . feat:main`*: se niega a actualizar una rama checkouteada, que es el caso común, y pasa por la maquinaria de fetch.
- *`git branch -f`*: no tiene valor viejo y no actualiza los archivos del worktree que tiene la rama.
- *`git checkout main && git merge` en el principal*: cambiaría lo que el usuario tiene checkouteado ahí.

### D6. Eventos y variable

En `internal/hook`:

- `PreMerge` y `PostMerge` se suman a `Events`, después de los de remove, en el orden de la config.
- `Background()` suma `PostMerge`.
- `Vars(e)` suma `target` para los dos eventos de merge.
- `worktree.HookVars` no cambia: `wt merge` agrega `values["target"]` al mapa que devuelve.

Dónde corren:
- `pre-merge` corre en el worktree;
- `post-merge` corre en el principal, porque el worktree ya puede no existir. Con `--no-remove` también va en el principal, así un hook no tiene dos directorios posibles.

`config-layers` lista las claves nuevas por `hook.Events`, sin otro cambio.

### D7. Ctrl-C

Es el mismo mecanismo que `wt sync`:

- `signal.Notify` antes del rebase;
- `pending(interrupts)` después del rebase y antes del fast-forward.

Si llega una interrupción:

- durante el rebase, git sale con error. `rebaseOnto` aborta el rebase que quedó, y `wt merge` sale con 130 sin mover el target;
- entre el rebase terminado y el fast-forward, sale con 130 con la rama rebaseada. El hint lo dice;
- durante `pre-merge` y `pre-remove`, manda `hooks`, como hoy en `a.runBlocking`.

El comportamiento por OS:

- **macOS y Linux**: la SIGINT de la terminal le llega al grupo de procesos, git incluido. `wt` la recibe por el canal y no muere.
- **Windows**: el evento de Ctrl-C le llega a todos los procesos de la consola. Go lo entrega como `os.Interrupt`.

### D8. Dry run, JSON y salida

- **`--dry-run`** corre el plan entero (D2, pasos 1 a 5), así los errores y los exit codes son los mismos. Después imprime, en orden:
  - `would rebase` si `!Contains && !NoRebase`;
  - `hook.PreviewLines` de `pre-merge`;
  - `would merge`;
  - `a.previewRemove(plan)` salvo `--no-remove`;
  - las líneas de `post-merge`.

  No escribe la directiva.
- **`--json`** imprime `wt.merge.v1`: `{schema, name, path, branch, target, rebased, head, removed}`, con `head` como el commit del target después. Bajo `--dry-run`, si haría falta un rebase, `head` es `null`, porque los commits rebaseados todavía no existen. Un fallo sale como `wt.error.v1` por el camino de siempre.
- **Las líneas de texto** salen a medida que pasa cada paso (`rebased`, `merged`), no al final como en `wt remove`. Si falla el remove, lo ya hecho está en stdout y el error en stderr.

### D9. Comando y completions

- **`internal/cli/merge.go`**:
  - `mergeCommand` con `--no-rebase`, `--no-remove` y `--no-hooks`, `Args: cobra.MaximumNArgs(1)` como usage error (exit 2);
  - el help explica el orden de los pasos y qué queda hecho cuando uno falla.

  Se registra en `run.go` después de `syncCommand`.
- **Completions**: una función nueva, `a.completeLocalBranches(exclude)`, que lista `refs/heads` del repo de `dir` sin la rama del worktree actual. Si no puede leer el repo, devuelve vacío sin escribir nada. `ShellCompDirectiveNoFileComp`.

## Risks / Trade-offs

- **El estado "rebaseada y no mergeada"**: si `pre-merge` falla, la rama quedó reescrita. Es lo que se quiere, porque los tests corrieron sobre el código rebaseado, y un `wt merge` siguiente no rebasea de nuevo. Pero un push previo de la rama queda divergente, como con `wt sync`.
- **El estado "mergeada y no borrada"**: un `pre-remove` que falla deja el target movido. Lo dice el hint, y `wt remove` lo termina, porque la rama ya cuenta como mergeada en el target.
- **Un `post-merge` que corre sin el worktree**: en background y en el principal, así que no puede usar archivos del worktree borrado. `{path}` sigue siendo la ruta que tenía. El help lo dice.
- **Un worktree principal con archivos sin versionar** que chocan con el merge: git se niega y nada se mueve, con exit 1. No se chequea antes porque sería repetir la lógica de git.
- **Windows y el directorio actual**: `a.remove` ya sale del worktree antes de borrarlo. El fast-forward corre con `-C` del worktree del target, así que no depende del directorio actual.
- **Costo**: unas pocas lecturas más que `wt remove`, y el rebase. Lo que tarda es `pre-merge`.
