# Design: remove-worktree

## Context

`remove-worktree` es el segundo comando que escribe, y el primero que borra. Copia el patrón de `create-worktree`: `cli.Run(ctx, Env) int` como único punto de entrada, decisión pura en `internal/worktree`, git detrás de `git.Runner`, errores tipados `*cli.Error` que `Run` traduce a exit code, la directiva vía `shell.WriteDirective`, y tests contra git real en `t.TempDir()` con `internal/testutil`. Ya existe casi todo lo que hace falta leer: `worktree.Build` informa `Locked`, `LockedReason`, `Prunable`, `PrunableReason`, `Head` y `Main`; `worktree.Resolve` resuelve `^`, `@` y nombres; `worktree.DefaultBranch` y `git.OriginHEAD`, `SymbolicHEAD`, `ResolveCommit` y `DeleteBranch` vienen de `create-worktree`. El por qué está en `proposal.md`; el comportamiento, en `specs/`.

Hechos observados (git 2.51, macOS) que moldean el diseño:

1. **`git worktree remove` sobre un worktree sin directorio** lo desregistra y sale con 0.
2. **`git worktree remove` se niega con archivos modificados o sin trackear** ("contains modified or untracked files, use --force", exit 128) y **no cuenta los ignorados**: un `secret.env` ignorado se borra sin aviso. git decide eso corriendo `git status --porcelain --ignore-submodules=none` en el worktree, así que respeta `status.showUntrackedFiles`: con `no`, los archivos sin trackear se borran sin aviso.
3. **Un worktree anidado aparece como sin trackear** en el de afuera, y **`--force` lo borra**: `git worktree remove --force outer` borró `outer/inner` con su archivo sin trackear, y dejó `inner` registrado como `prunable`.
4. **Cuando git no puede borrar el directorio, igual lo desregistra.** Con un subdirectorio sin permiso de escritura: `error: failed to delete '<path>': Permission denied`, exit 255, el worktree ya no aparece en `git worktree list` y la carpeta queda con lo que no se pudo borrar. En `builtin/worktree.c`, `remove_worktree` sigue con `delete_git_dir` aunque `delete_git_work_tree` haya fallado ("there's no going back from here").
5. **Borrar desde adentro funciona en macOS**: `git worktree remove .` con el cwd en el worktree sale con 0 y el proceso queda en un directorio que ya no existe. **En Windows no se puede borrar un directorio que es el directorio de trabajo de un proceso**: cada proceso tiene un handle abierto a su cwd, sin `FILE_SHARE_DELETE`. Esto último es comportamiento documentado de Windows, no observado acá: lo confirma el CI de `windows-latest` (D10).
6. **`git worktree lock`** falla con 128 sobre el principal ("The main working tree cannot be locked or unlocked") y sobre uno ya lockeado; **`unlock`** falla con 128 sobre uno que no lo está. `--reason ""` lockea sin motivo. Un worktree lockeado nunca es `prunable`, ni en `worktree list` ni para `worktree prune`.
7. **`git branch -d` juzga contra el HEAD del directorio donde corre** (o contra el upstream de la rama, si tiene): el mismo comando da distinto resultado según desde qué worktree se corra.

## Goals / Non-Goals

**Goals:**
- Que `--dry-run` sea correcto por construcción: el mismo `RemovePlan` alimenta `--dry-run`, `--json` y el borrado.
- Que ningún camino pierda trabajo sin un flag explícito: todo chequeo que bloquea corre antes de borrar, y `--force` y `-D` son decisiones separadas.
- Que la regla de "rama mergeada" quede en una función pura reutilizable por `merge-worktree` (M3) y `list` (M4).

**Non-Goals:**
- Reimplementar `git worktree remove`: `wt` decide y chequea; el borrado lo hace git.
- Progreso durante el borrado de un worktree grande.

## Decisions

### D1. Resolución de `<target>` compartida

`cd.go` mapea hoy los errores de `worktree.Resolve` a `*Error` (exit 3 con hint `wt list`, exit 4 con un hint por candidato, `ErrNoCurrent`). Se mueve a `internal/cli/target.go`:

```go
// resolveTarget resuelve <target> como wt cd: ^, @, nombre y después rama.
func (a *app) resolveTarget(repo *repository, target string) (worktree.Worktree, error)
```

`cd`, `remove`, `lock` y `unlock` lo usan. `-` sigue siendo cosa de `cd`: para los demás es un nombre como cualquier otro.

*Alternativa descartada:* copiar el mapeo en cada comando. Cuatro copias del mismo contrato de exit codes divergen.

### D2. Decisiones puras: `internal/worktree/remove.go`

```go
// RemovePlan es todo lo que wt remove decidió antes de cualquier efecto.
type RemovePlan struct {
	Worktree     Worktree // el target, como lo devuelve Build
	Force        bool     // git worktree remove --force
	Branch       string   // "" si HEAD está detached
	DeleteBranch bool
	KeepReason   string   // "" o "not merged into origin/main"
	CD           string   // ruta del principal si la shell se mueve; "" si no
}

// CheckRemovable aplica, en este orden, los bloqueos que no dependen de git:
// principal (*MainWorktreeError), lockeado (*LockedError) y anidado (*ContainsError).
func CheckRemovable(w Worktree, ws []Worktree, goos string) error

// Contains dice si el directorio dir contiene la ruta p (o es p), por componentes,
// sin distinguir mayúsculas en darwin y windows. Es el contains de Build, exportado.
func Contains(dir, p, goos string) bool

// BranchOutcome decide qué pasa con la rama.
func BranchOutcome(branch string, keep, forceDelete bool, m Merged) (del bool, keepReason string)

// Merged es lo que el borde leyó para juzgar la rama.
type Merged struct {
	Base       string // la base como está escrita; "" si no hay
	InBase     bool   // la punta es la base o un ancestro
	InUpstream bool   // ídem con el upstream, si tiene
}

func CheckLockable(w Worktree) error   // principal -> *MainWorktreeError; lockeado -> *LockedError
func CheckUnlockable(w Worktree) error // principal; no lockeado -> *NotLockedError
func Prunable(ws []Worktree) []Worktree // los prunable, en el orden de Build
```

`CheckRemovable` recorre `ws` en el orden de `Build`, así que con varios anidados el mensaje nombra siempre el mismo. El orden de los bloqueos (principal, lock, anidado y, en el borde, archivos sucios) decide qué error ve el usuario cuando hay más de uno: primero lo que ni `--force` destraba.

### D3. El borde de `wt remove`: orden de lecturas, chequeos y efectos

`internal/cli/remove.go`, en este orden. Todo lo que puede fallar por estado corre antes del primer efecto.

```
   cobra: args y flags; --keep-branch con -D                        exit 2
   workdir (-C)                                                     exit 2
   requireRepository  (git worktree list)                           exit 3
   loadConfig(repo.root())                                          exit 1
   resolveTarget                                                    exit 3 / 4
   CheckRemovable: principal -> lockeado -> anidado                 exit 2 / 5 / 5
   si el directorio existe y no hay --force:
     git -C <path> status --porcelain -z --ignore-submodules=none
                          --untracked-files=normal
     salida no vacía                                                exit 5
   si hay rama, sin --keep-branch ni -D:  Merged (D4)               (warnings)
   cd := Contains(path, getwd real) ? main.Path : ""
   plan := RemovePlan{...}
   ── --dry-run: imprimir el plan (texto o wt.remove.v1) y salir 0
   Chdir(main.Path)                                                 (D5)
   git -C <main> worktree remove [--force] <path>                   exit 1 (+ hint, D7)
   si DeleteBranch: git -C <main> branch -D <rama>                  exit 1
   si cd: directiva (activa) o warning                              exit 1
   imprimir las líneas o wt.remove.v1
```

- **El chequeo de sucio es propio, no el de git.** Es la misma lectura que hace git (hecho 2) más `--untracked-files=normal`, que la spec pide para que `status.showUntrackedFiles = no` no esconda archivos que se perderían. Como el chequeo de `wt` es más estricto que el de git, sin `--force` git nunca se niega por lo mismo; si alguien escribe un archivo entre el chequeo y el borrado, git se niega (exit 1 con su mensaje) y no se pierde nada. Con `--force` se pasa `--force` a git. Nunca `-f -f`: el lock lo frenó antes.
- **El anidado se chequea antes que lo sucio** (hecho 3): si no, el error sería "archivos sin trackear" y la solución sugerida, `--force`, borraría el anidado.
- **Los efectos corren desde el principal.** Después del borrado el directorio de trabajo puede no existir, y en Windows tiene que no estar en uso (D5). Para un bare, el principal es el repo bare, y `git -C <bare> worktree remove` y `branch -D` funcionan igual.
- **La rama se borra con `-D`**, siempre después de que `wt` decidió: con `-d`, git aplicaría su propia regla contra el HEAD del principal (hecho 7) y podría negarse a algo que `wt` ya juzgó mergeado.
- **La salida va al final.** La spec de `cli-contract` pide stdout vacío ante un error; si `branch -D` o la directiva fallan después de borrar el worktree, el mensaje de error dice lo que ya se hizo (`removed worktree <path>, but cannot delete branch feat: …`), y stdout queda vacío.
- **La directiva se escribe aunque falle `branch -D`**: el directorio ya no está, y la shell no tiene que quedar adentro. El primer error es el que se informa.

Las lecturas y escrituras nuevas van a `internal/git/remove.go` (`Status`, `Upstream`, `IsAncestor`, `RemoveWorktree`, `LockWorktree`, `UnlockWorktree`, `PruneWorktrees`), todas sobre `git.Runner`, con su test contra git real.

*Alternativa descartada:* `git worktree remove` sin chequeo previo, y traducir su exit 128 a exit 5. `--dry-run` no sabría que va a fallar, y el mensaje de git no distingue sucio de submódulos ni de lock.
*Alternativa descartada:* borrar el worktree primero y la rama después, fallando si no está mergeada (v0.9). Deja el trabajo a medio cerrar; acá la rama se decide antes y nunca bloquea el borrado.

### D4. Rama mergeada

```
   punta := plan.Worktree.Head                      (el commit, no el nombre de la rama)
   base  := default_base | DefaultBranch(OriginHEAD, principal, SymbolicHEAD del bare)
            sin base            -> warning "the repository has no default branch"
   baseC := ResolveCommit(base)  ErrNoRevision -> warning "base not found: <base>"
   InBase := IsAncestor(punta, baseC)
   up    := git for-each-ref --format=%(upstream:short) refs/heads/<rama>   ("" si no tiene)
   upC   := ResolveCommit(up)    ErrNoRevision -> no cuenta, sin warning
   InUpstream := IsAncestor(punta, upC)
```

- `IsAncestor` es `git merge-base --is-ancestor <a> <b>`: exit 0 sí, exit 1 no, otro exit es error de git (exit 1 de `wt`). Se le pasan commit ids ya resueltos, así que un tag con el nombre de la rama no confunde nada.
- **Un upstream que ya no existe no cuenta, y no avisa.** Es el caso común: GitHub borra la rama del PR al mergearlo, `fetch --prune` borra `origin/feat`, y `branch.feat.merge` sigue configurado. No es un problema de configuración del usuario.
- El warning de la base sale una sola vez, nombra la rama (`cannot tell whether branch feat is merged: the repository has no default branch`) y no cambia el exit code.
- Con `--keep-branch` o `-D` no se lee nada de esto: no hay warnings que no cambian nada.
- La función pura (`BranchOutcome`) recibe solo `Merged`, así que `merge-worktree` (M3) puede sumar detección de squash como un tercer booleano sin tocar el borde de `remove`.

*Alternativa descartada:* `git branch -d` y su regla. Depende de desde dónde corre (hecho 7), y en un flujo de muchos worktrees el HEAD del principal suele estar atrasado.
*Alternativa descartada:* `git cherry` para detectar rebases y squashes. Un squash de varios commits no tiene el mismo patch-id que ninguno de ellos; queda para M3 con un criterio por contenido.

### D5. Dejar el directorio: `cli.Env.Chdir`

```go
type Env struct {
	// ... campos actuales
	// Chdir cambia el directorio de trabajo del proceso; nil no hace nada.
	Chdir func(dir string) error
}
```

`main` pasa `os.Chdir`; el harness de tests in-process, `nil`, porque el cwd es del proceso entero y los tests corren en paralelo. Antes de `git worktree remove`, `remove` hace `Chdir(main.Path)` y corre git con `Dir = main.Path`.

```
   macOS / Linux                                Windows
   -------------------------------------------  ---------------------------------------------
   el cwd de un proceso no impide borrarlo      el cwd de un proceso impide el rmdir (hecho 5)
   git-wt hace Chdir(principal) igual           git-wt hace Chdir(principal): es necesario,
     (un solo camino de código)                   porque arrancó con el cwd de la shell
   git corre con Dir = principal                git corre con Dir = principal
   otro proceso parado adentro: se borra,       otro proceso parado adentro: git borra los
     y ese proceso queda en un dir inexistente    archivos, no la carpeta; desregistra igual;
                                                  wt sale con 1 y el hint de D7
   la shell (zsh, bash, fish, pwsh): n/a        pwsh: su location no es el cwd del proceso
                                                  (Set-Location no lo cambia); a verificar
```

`Chdir` se hace en los tres OS: solo Windows lo necesita, pero así el camino es uno y el test del binario lo prueba en los tres. Si `Chdir` falla, `wt` sigue: en macOS y Linux no hace falta, y en Windows git fallará con un mensaje que lo explica.

Según el código de git para Windows (`mingw_rmdir`), cuando no puede borrar un directorio reintenta con esperas cortas y después pregunta "Should I try again? (y/n)" solo si stdin y stderr son terminales o si `GIT_ASK_YESNO` está definida. `git.Exec` deja stdin en el dispositivo nulo y captura stderr, así que nunca pregunta: eso no tiene que cambiar.

*Alternativa descartada:* `os.Chdir` dentro de `internal/cli`. Rompe los tests in-process en paralelo.
*Alternativa descartada:* hacer `Chdir` solo en Windows. Una diferencia por OS más, sin beneficio.

### D6. A dónde va la shell

La shell se mueve si el directorio en el que arrancó `wt` (`Env.Getwd`, no `-C`), con symlinks resueltos, está dentro del worktree: `worktree.Contains(path, cwdReal, goos)`, la misma comparación que decide el worktree actual. El destino es `main.Path`, el mismo que `wt cd ^`. La directiva se escribe solo si el borrado tuvo éxito (si falló, el directorio sigue ahí). Sin la integración activa, el warning es:

```
wt: warning: shell integration is not active, so the shell stays in a directory that no longer exists; see 'wt shell init --help'
```

### D7. Salida, mensajes y JSON

```
removed worktree /src/repo.worktrees/feat
deleted branch feat

removed worktree /src/repo.worktrees/feat
kept branch feat: not merged into origin/main

would remove worktree /src/repo.worktrees/feat
would keep branch feat: not merged into origin/main
would change directory to /src/repo

wt: cannot remove the main worktree

wt: worktree "feat" is locked: on usb drive
hint: run 'wt unlock feat' first

wt: worktree "outer" contains the worktree "inner" at /src/repo.worktrees/outer/inner
hint: remove "inner" first

wt: worktree "feat" has modified or untracked files
hint: see them with 'git -C /src/repo.worktrees/feat status'
hint: --force removes the worktree with them; they are lost

wt: git worktree remove /src/repo.worktrees/feat: error: failed to delete '...': Permission denied
hint: the directory is still there: /src/repo.worktrees/feat

wt: warning: cannot tell whether branch feat is merged: base not found: origin/develop

locked worktree /src/repo.worktrees/feat
unlocked worktree /src/repo.worktrees/feat
wt: worktree "feat" is already locked: review
wt: worktree "feat" is not locked
wt: the main worktree cannot be locked

pruned worktree /src/repo.worktrees/feat
nothing to prune
```

El hint de "the directory is still there" sale solo si `os.Lstat(path)` encuentra algo después del fallo.

JSON, igual con y sin `--dry-run`:

```
wt.remove.v1  {"schema","name","path","branch": string|null, "branch_deleted": bool}
wt.lock.v1    {"schema","name","path","reason": string|null}
wt.unlock.v1  {"schema","name","path"}
wt.prune.v1   {"schema","pruned": [{"name","path","reason"}]}
```

El `Long` de `wt remove` explica la regla de la rama (y que un squash-merge no cuenta), que `-f` y `-D` son independientes, que los archivos ignorados se borran, y que desde adentro la shell termina en el principal.

### D8. `lock`, `unlock` y `prune`

- `lock` y `unlock`: `resolveTarget`, `CheckLockable`/`CheckUnlockable` (que dan exit 2 y 5 antes de que git falle con su 128, hecho 6), y `git worktree lock [--reason <r>] <path>` o `git worktree unlock <path>`. Un `<reason>` vacío no pasa `--reason`. `lock` funciona sobre un worktree sin directorio sin nada especial: git lo permite, y es su caso de uso.
- `prune`: `Prunable(repo.worktrees)` es la lista que se informa; después, `git worktree prune`. git usa la misma regla para `list` y para `prune` (hecho 6), así que lo informado es lo podado. Si entre la lectura y el prune aparece otro, git lo poda y `wt` no lo nombra; si `Prunable` está vacía, `wt` no corre git.

*Alternativa descartada:* parsear `git worktree prune -v`. Informa `worktrees/<id>`, no la ruta, y su formato no es porcelain.

### D9. Completions

`completeWorktreeNames` (de `cd`) pasa a `completeNames(filter func(worktree.Worktree) bool)`: `cd` ofrece los que no son `Prunable`; `remove` y `lock`, los que no son `Main` ni `Locked`; `unlock`, los `Locked`. El segundo argumento de `lock` no ofrece nada. Como `cd`, no carga config, y ante cualquier error no ofrece nada. Los scripts de los cuatro shells no cambian.

### D10. Tests

- **Puros**, con tablas, corridos en macOS con `goos` darwin, linux y windows: `CheckRemovable` (cada bloqueo, su orden, anidado con otra capitalización: bloquea en darwin y windows, no en linux), `Contains`, `BranchOutcome` (los flags contra cada combinación de `Merged`), `CheckLockable`, `CheckUnlockable` y `Prunable`.
- **git real**: cada función de `internal/git/remove.go` en un sandbox: `Status` con `status.showUntrackedFiles = no`, `Upstream` (sin upstream, con upstream, con el upstream borrado), `IsAncestor` (sí, no, error), `RemoveWorktree` (con y sin `--force`, sin directorio, y en macOS y Linux con un subdirectorio sin escritura), `LockWorktree` con y sin motivo, `UnlockWorktree` y `PruneWorktrees`. `testutil` suma `SetUpstream`, `SquashMerge` y `ReadOnlyDir` (solo unix).
- **CLI in-process** (`harness`): un test por escenario de `remove-worktree` y de "Merged branch" que no necesite una shell real. "Directory in use by another process" arranca un hijo con `Dir` dentro del worktree: `sleep 30` en macOS y Linux, `cmd /c pause` con stdin abierto en Windows; el proceso del test nunca está adentro, así que `Chdir` en `nil` no cambia nada.
- **Binario** (`cmd/git-wt`): "Binary started inside the worktree", con `exec.Cmd.Dir` en `<feat>/src`. En `windows-latest` es la prueba de D5.
- **Shell real** (`shell_test.go`): "From inside, through the function" en zsh, bash, fish y pwsh, con la infraestructura de `shell-integration`.

## Risks / Trade-offs

- **[Riesgo] pwsh y su directorio de trabajo en Windows.** Si `Set-Location` cambiara el cwd del proceso de pwsh, `wt remove @` desde adentro fallaría: git borraría los archivos, no la carpeta, y desregistraría el worktree (hecho 4). Hasta donde se sabe, en PowerShell 7 la location del provider no cambia el cwd del proceso. → Lo prueba el test de shell real "From inside, through the function" en pwsh, en el CI de `windows-latest`, y lo confirma la verificación manual **[Windows]** en Windows Terminal. Plan B si falla: en Windows, cuando la shell está dentro del worktree, `wt remove` falla con exit 5 antes de tocar nada y un hint con `wt cd ^`; cambia solo el escenario "From inside, through the function" para Windows.
- **[Riesgo] Otro proceso parado en el worktree, en Windows** (el editor, el agente, otra terminal). git borra lo que puede, desregistra el worktree y deja la carpeta. → Exit 1 con el mensaje de git y el hint que nombra la carpeta. No se pierde trabajo: el chequeo de sucio garantiza que todo lo borrado estaba commiteado o ignorado. La carpeta se borra a mano.
- **[Riesgo] Squash-merges.** Muchos repos en GitHub mergean con squash, y esas ramas nunca cuentan como mergeadas: después de cada PR la rama se conserva y hay que usar `-D`. → La salida lo dice (`kept branch feat: not merged into origin/main`), el help lo explica, y la detección por contenido es candidata para `merge-worktree` (M3). D4 deja el lugar.
- **[Riesgo] Base vieja.** Sin fetch, una rama mergeada en el remoto después del último fetch parece no mergeada. → Se conserva: es el lado seguro.
- **[Trade-off] Una rama pusheada cuenta como mergeada aunque su PR esté abierto.** `wt remove feat` borra la rama local; los commits siguen en `origin/feat`. Para retomarla, `wt create` no sirve (solo crea ramas nuevas, y `origin/feat` existe): hace falta `git worktree add <ruta> feat`. → Decisión del autor (la regla del upstream); `--keep-branch` lo evita, y el help lo menciona.
- **[Trade-off] Los archivos ignorados se pierden** (un `.env` ignorado, `node_modules`). → Es la semántica de git y de `.gitignore`; la spec y el help lo dicen.
- **[Riesgo] Submódulos inicializados.** git se niega a borrar un worktree con submódulos aunque esté limpio. → Exit 1 con el mensaje de git; `--force` lo resuelve. No se agrega un chequeo propio.
- **[Riesgo] Carrera entre el chequeo y el borrado.** Un archivo nuevo entre `status` y `worktree remove`. → Sin `--force`, git hace su propio chequeo y se niega (exit 1); nada se pierde.

## Migration Plan

No hay `wt remove` previo en `main`. Quien venga de v0.9 encuentra dos diferencias, que el `CHANGELOG` y el help cuentan: la rama mergeada se borra por defecto (en v0.9 hacía falta `--delete-branch`), y el forzado de la rama es `-D` (en v0.9, `--force-branch`). Se actualizan `docs/design/product.md` (§4) y `CHANGELOG.md`.
