# Design: merge-steps

## Context

El por qué está en `proposal.md`, y el comportamiento en `specs/`. Lo que ya existe y moldea el diseño:

- **`wt merge`** (`internal/cli/merge.go`) separa `a.planMerge`, que lee y chequea todo, de `a.merge`, que ejecuta. Sus chequeos puros están en `worktree.CheckMerge` y sus líneas en `worktree.MergeLine` (`internal/worktree/merge.go`). Hoy `MergeState.Changes` bloquea con exit 5.
- **El rebase** es `a.rebaseOnto`, extraído de `wt sync`: corre `git.Rebase`, distingue conflicto (`git.Unmerged`) de fallo y aborta con `git.RebaseAbort`.
- **El fast-forward** es `git.FastForward(ctx, r, dir, branch, old, commit, checkedOut)`, con el valor viejo como guarda.
- **Los hooks**: `hookPlanner.plan`, `.check(events...)` (exit 8), `a.runBlocking` (exit 7) y `a.background`. `hook.Events` y `hook.Vars(e)` listan los eventos y sus variables, y `config-layers` deriva de ahí las claves `hooks.*`.
- **El Ctrl-C** de `wt merge`: `signal.Notify(interrupts, os.Interrupt)` antes del primer efecto, y `pending(interrupts)` entre pasos.
- **La terminal**: `Env` ya inyecta `Stdin` y un `IsTTY` que es de stdout (para el color). No hay un dato de si stdin es una terminal.
- **OpenSpec**: el repo usa la CLI 1.13.0 (`generatedBy` de `.claude/skills/`). Con 2.x, `validate --all --strict` ya falla en `main` por el aviso de requirements de más de 500 caracteres, que en 1.13 es informativo. Los requirements nuevos de este change quedan por debajo; los MODIFIED copian los bloques existentes enteros, como pide el flujo.

## Goals / Non-Goals

**Goals:**
- Un solo lugar que decide de dónde sale el mensaje (`-m`, editor, o "hace falta uno"), para que `llm-commits` (M6) se enchufe ahí sin tocar los pasos.
- Que un commit o un squash que falla, por cualquier motivo, deje la rama, el index y los archivos como estaban. El único estado intermedio nuevo en `wt merge` es "commiteada y no mergeada", igual que hoy existe "rebaseada y no mergeada".
- Que `wt step rebase` y `wt step push` sean los pasos de `wt merge`, no copias.

**Non-Goals:**
- Que `wt` abra el editor por su cuenta: lo abre git (ver D3).
- Clasificar los errores de un hook nativo de git: se informan con la salida de git.

## Decisions

### D1. La decisión pura

En `internal/worktree/step.go`, sin git ni filesystem:

```go
type StepKind int // StepCommit, StepSquash, StepRebase, StepPush

type StepState struct {
	MergeState            // detached, en curso, target, contains... (se reusa)
	Pending     bool      // versionados o sin versionar, como los cuenta wt remove
	Tracked     bool      // solo versionados: lo que bloquea rebase y push
	Own         int       // commits de la rama que el target no tiene
	MessageFlag bool      // se dio -m
	Terminal    bool      // stdin es una terminal y no hay --json
}

// CheckStep es el primer bloqueo del paso, en el orden de la spec, o nil.
func CheckStep(kind StepKind, branch, target string, s StepState) *MergeBlock

// StepNoop dice si el paso no tiene nada que hacer y la línea que imprime
// ("nothing to commit", "nothing to squash", "feat already contains main").
func StepNoop(kind StepKind, branch, target string, s StepState) (string, bool)

// NeedsMessage: el paso commitea, no hay -m. Con !Terminal es el exit 2.
func NeedsMessage(kind StepKind, s StepState) bool

// SquashMessage arma el mensaje inicial del editor: un comentario con la
// cuenta y los mensajes completos, del más viejo al más nuevo.
func SquashMessage(branch string, messages []string) string

// StepLine arma "committed ...", "squashed ...", "would ..." y sus variantes.
func StepLine(kind StepKind, branch, target, short string, n int, dryRun bool) string
```

`CheckMerge` cambia en un solo punto: `Changes` bloquea solo con `NoCommit`, y suma el caso de `--squash --no-commit` con cambios. `wt merge` arma un `StepState` además del `MergeState` cuando va a commitear o squashear, y pregunta `NeedsMessage` en el mismo lugar de los chequeos, antes de las aprobaciones.

### D2. Commit a prueba de fallos: foto del index y vuelta atrás

En `internal/git/commit.go`:

1. **Foto**: `git write-tree` guarda el index actual como un tree (los cambios ya stageados del usuario incluidos) y se recuerda `HEAD`. No hay entradas sin mergear, porque una operación en curso ya bloqueó antes.
2. **Squash**: `git reset --soft <merge-base>`. La rama baja y el index queda con los archivos de HEAD.
3. **Stage**: `git add -A` (respeta `.gitignore`).
4. **Commit**: `git commit -q` con `-m <msg>` (D3), o con el editor.
5. **Vuelta atrás**, si 4 falla o se interrumpe: `git reset --soft <HEAD de la foto>` cuando hubo squash, y `git read-tree <tree de la foto>` más `git update-index -q --refresh`. Los archivos del worktree nunca se tocaron, así que con eso el estado es exactamente el de antes.

`commit` y `squash` son el mismo camino, con o sin el paso 2.

Alternativas descartadas:
- `git commit-tree` + `git update-ref` (sin mover nada hasta el final): no corre los hooks nativos de git del repo, y `wt step commit` sí los tiene que correr para no saltearse un husky.
- Stashear: la política del proyecto es no stashear nunca.
- `git merge --squash` en el target: necesita el target checkouteado y mezcla el squash con el merge, que son pasos distintos.

### D3. El editor lo abre git

Sin `-m`, `wt` corre `git commit` en foreground con su stdin, stdout y stderr (el mismo runner que `-x`), y git abre su editor con su propia resolución (`GIT_EDITOR`, `core.editor`, `VISUAL`, `EDITOR`) y su propio template con el estado. Para el squash, `git commit --edit -F <archivo>` con el mensaje de `SquashMessage`, en un archivo temporal del git dir del worktree. Así `wt` no arma el comando del editor, que en Windows tiene reglas de comillas de Git for Windows.

**Mensaje vacío contra hook nativo que falla**: los dos dan exit 1 en git, y su texto depende del idioma (`LANG`). Cuando `git commit` falla, `wt` lee `COMMIT_EDITMSG` del git dir del worktree y le pasa `git stripspace --strip-comments`: si queda vacío, es "empty commit message"; si no, es un fallo de git y se informa con su salida. Con `-m`, un mensaje vacío se rechaza antes de correr git.

Alternativa descartada: que `wt` corra `git var GIT_EDITOR` y lance el editor él mismo. Duplica lo que git ya hace bien en los dos OS.

### D4. La terminal

`Env` suma `StdinTTY bool`, que `cmd/git-wt` llena con `term.IsTerminal(os.Stdin)`. `Terminal` de D1 es `StdinTTY && !json`. Los tests in-process lo ponen en `true` y usan un `GIT_EDITOR` de prueba: una función de shell que escribe, vacía o copia el archivo que recibe. git corre su editor con su propio `sh` en los tres OS (Git for Windows trae uno), así que el mismo `GIT_EDITOR` sirve también en Windows, sin un `.cmd`.

### D5. El mensaje en un solo lugar

```go
type messageSource struct{ flag string; given bool }
func (a *app) commitMessage(kind worktree.StepKind, s worktree.StepState) (messageSource, error)
```

Hoy devuelve `-m` o "editor"; `llm-commits` sumará una tercera fuente acá (una clave de config que genera el mensaje), sin tocar los pasos ni `wt merge`.

### D6. `wt step` en Cobra

`internal/cli/step.go`: un comando `step` sin `RunE` propio (sin subcomando imprime el help y sale con 2, como pide la spec) y cuatro hijos. `rebase` no registra `--no-hooks`, así Cobra lo rechaza como flag desconocido (exit 2). Cada hijo arma su plan con `a.planStep(kind, target, flags)` y lo ejecuta con `a.step(plan)`. `rebase` llama a `a.rebaseOnto` y `push` a la misma función que hace el paso 4 de `wt merge` (`a.mergeTarget`, que se extrae de `a.merge` sin cambiar su comportamiento).

### D7. `wt merge` compone los pasos

El pipeline nuevo, en `a.merge`:

```
chequeos (+ NeedsMessage, + aprobaciones de commit) -> [pre-commit -> commit|squash]
  -> rebase -> pre-merge -> fast-forward -> remove -> post-commit, post-remove, post-merge
```

- `post-commit` se planea con los valores del worktree commiteado. Con `--no-remove` se larga justo después del commit, en el worktree; si el worktree se va a borrar, se larga después del remove, en el principal, porque un proceso parado en un directorio que se borra falla en Windows.
- `wt.merge.v1` suma `committed` y `squashed`. Agregar campos no sube la versión (`cli-contract`).
- `--squash` hace el squash sobre el merge-base con el target, antes del rebase, así el rebase tiene a lo sumo un commit que conflictúe.

### D8. Los eventos

`hook.PreCommit` y `hook.PostCommit` en `internal/hook`, con `Background()` verdadero para `post-commit` y las variables de siempre (sin `{target}`, que sigue siendo solo de merge). Las claves `hooks.pre-commit` y `hooks.post-commit` salen solas de `hook.Events`.

### D9. Ctrl-C

El mismo patrón que `wt merge`: `signal.Notify` antes del primer efecto. Mientras el editor o `git commit` corren, la señal les llega a ellos (el grupo de procesos en unix, la consola en Windows); `wt` espera, hace la vuelta atrás de D2 y sale con 130.

## Risks / Trade-offs

- **[Un `kill -9` de `wt` entre el `reset --soft` y el commit del squash deja la rama en el merge-base con todo stageado]** → Los archivos no se pierden (están en el index y en el worktree), y `git reset --soft ORIG_HEAD` la devuelve. Se documenta en el help de `wt step squash`.
- **[`wt merge` ahora commitea un worktree sucio; antes frenaba]** → Sin `-m` abre el editor, así que nunca commitea a ciegas en una terminal; en un script sin `-m` frena con exit 2. `--no-commit` vuelve al comportamiento de M3. Va al CHANGELOG como cambio de comportamiento.
- **[`wt step push` suena a `git push`]** → El help y la línea de salida dicen `merged feat into main`, y el proposal lo deja como non-goal. Si confunde en el uso, renombrarlo es barato mientras sea alpha.
- **[Los hooks nativos de git corren y pueden ser lentos]** → Es lo que haría `git commit`; `--no-verify` queda afuera a propósito.
- **[`COMMIT_EDITMSG` podría tener un mensaje viejo si git falló antes de escribirlo]** → `wt` lo borra antes de correr `git commit`; si no existe después, el fallo es de git, no un mensaje vacío.

## Migration Plan

Nada que migrar: no hay config ni datos nuevos guardados. Quien dependía de que `wt merge` frenara con un worktree sucio usa `--no-commit`.
