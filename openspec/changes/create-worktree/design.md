# Design: create-worktree

## Context

`create-worktree` es el primer comando que escribe. Copia el patrón que dejaron `walking-skeleton` y `shell-integration`: `cli.Run(ctx, Env) int` como único punto de entrada, decisión pura en `internal/worktree`, git detrás de `git.Runner`, errores tipados `*cli.Error` que `Run` traduce a exit code, la directiva vía `shell.WriteDirective`, y tests contra git real en `t.TempDir()` con `internal/testutil`. `internal/template` existe pero solo tiene `doc.go`. `config.Keys()` tiene `default_base` y `worktree_path`, ambas string; `config.EnvLayer` guarda todo como string. La motivación está en `proposal.md`; el comportamiento, en `specs/`.

Hechos observados (git 2.51, macOS) que moldean el diseño:

1. **`git worktree add -b <rama> <ruta> <base>` crea la rama antes de mirar la ruta.** Si la ruta ya existe, o hay un worktree registrado ahí sin directorio, git falla con exit 128 y la rama queda creada, huérfana.
2. **Sin `--no-track`, una rama nacida de `origin/main` queda con upstream `origin/main`** (`branch.autoSetupMerge` vale `true` por defecto para bases remotas).
3. **`git check-ref-format --branch` acepta `feat<x>`, `a|b` y `@`**, y rechaza `a b`, `-x`, `HEAD`, `.x`, `x.lock` y `@{-1}`. Una rama válida para git puede no ser un nombre de carpeta válido en Windows: por eso existe `sanitize`.
4. **Un clon bare no tiene ramas remotas**: `git clone --bare` copia `refs/heads/*` y nada en `refs/remotes/`, así que no hay `origin/HEAD`. Su `HEAD` sí apunta a `main`.
5. **`git for-each-ref --format=%(symref:short) refs/remotes/origin/HEAD`** imprime `origin/main`, o nada si no existe, siempre con exit 0. `git rev-parse --verify --quiet --end-of-options <rev>^{commit}` sale con 1 y sin stderr cuando la revisión no existe.
6. **`git worktree list --porcelain` no informa la rama de un repo bare**: solo la línea `bare`.
7. **POSIX: una señal ignorada se hereda a través de `exec`; una con handler vuelve al default.** Si `wt` ignorara SIGINT para sobrevivir a Ctrl-C, el agente lanzado con `-x` también lo ignoraría.
8. **`cmd.exe` no parsea su línea de comando con las reglas de `CommandLineToArgvW`** que usa `os/exec` para quotear argumentos en Windows: pasar `<cmd>` como argumento de `exec.Command("cmd.exe", "/c", cmd)` rompe las comillas internas.

## Goals / Non-Goals

**Goals:**
- Que `--dry-run` sea correcto por construcción: el mismo `CreatePlan` alimenta la salida de `--dry-run` y la ejecución.
- Que el motor de plantillas y la ejecución de comandos queden reutilizables tal cual por hooks (M3) y launchers y aliases (M5): sintaxis, filtros e intérprete por OS no cambian cuando lleguen.
- Que ningún fallo de `wt create` deje una rama o un worktree a medias.

**Non-Goals:**
- Mostrar progreso durante el fetch.
- Paralelizar lecturas de git: `wt create` hace del orden de seis llamadas cortas, más el fetch.

## Decisions

### D1. `internal/template`: parser propio, puro

```go
// Parse valida la sintaxis y que cada variable esté en vars.
func Parse(src string, vars []string) (*Template, error)   // *SyntaxError{Part, Msg}
func (t *Template) Render(values map[string]string) string
func Sanitize(s string) string

var WorktreePathVars = []string{"repo", "repo_parent", "repo_path", "name", "branch"}
var filters = map[string]func(string) string{"sanitize": Sanitize, "lower": strings.ToLower}
```

El parser recorre el texto una vez: literal hasta `{`, expresión hasta `}`, `|` separa filtros, `strings.TrimSpace` en cada nombre. Los errores llevan la parte ofensiva (`{nope}`, `upper`, `{repo_parent/x`) para que el mensaje de config la nombre. `Render` no falla: `Parse` ya garantizó que cada variable existe en el contexto. `Sanitize` implementa las tres reglas de la spec en orden, sin mirar el OS.

`WorktreePathVars` vive acá porque `doc.go` ya es el catálogo de variables del producto; hooks y launchers van a sumar sus propios conjuntos.

*Alternativa descartada:* `text/template` de la stdlib. Su sintaxis es `{{.Repo}}` y la de producto es `{repo|sanitize}`; adaptarla sería un preprocesador frágil, y sus errores no nombran la parte que falla en términos del usuario.
*Alternativa descartada:* que `sanitize` dependa del OS (solo `/` en macOS, todo en Windows). Las mismas ramas darían carpetas distintas en las dos máquinas del autor, y sería una diferencia emergente más.

### D2. Config: tres claves, booleanos, y `worktree_path` validada al cargar

```go
type Key struct {
	// ... campos actuales
	// FromEnv convierte el texto de WT_<KEY> antes de Validate; nil = el texto tal cual.
	FromEnv func(string) (any, error)
}
```

- `branch_prefix`: string, `""`, `InRepo: true`.
- `fetch_before_create`: `Type: "boolean"`, `true`, `InRepo: true`, `Validate: validateBool` (acepta solo `bool`), `FromEnv: parseBoolEnv` (`true`/`false` sin distinguir mayúsculas, `1`, `0`).
- `create_cd`: igual, con `InRepo: false`: es una preferencia de quien usa la terminal, no del repo.
- `worktree_path`: `Validate: validateTemplate`, que exige string no vacío y `template.Parse(s, template.WorktreePathVars)`. Devuelve el **texto**, no el `*Template`: `wt config get` sigue imprimiendo la plantilla tal cual, y `create` la vuelve a parsear (es barato). Default: `{repo_parent}/{repo}.worktrees/{name|sanitize}`.

`Resolve` aplica `FromEnv` solo a la capa `SourceEnv`. Así un `create_cd = "false"` en un TOML sigue siendo error de tipo, como pide la spec, mientras que `WT_CREATE_CD=false` vale. `config` pasa a importar `internal/template`, que no importa nada del proyecto.

*Alternativa descartada:* validar la plantilla recién en `wt create`. Una plantilla rota aparecería tarde y solo en un comando, contra la regla de `config-layers` de que un valor inválido falla todo comando que lee config.
*Alternativa descartada:* que `EnvLayer` convierta los booleanos. `EnvLayer` no devuelve error, y la conversión fallida tiene que nombrar la variable con exit 1, que es lo que `Resolve` ya hace para `Validate`.

### D3. Nombre, rama y carpeta: decisiones puras en `internal/worktree/create.go`

```go
func ValidateName(name string) error                        // vacío, o componente "." / ".." -> exit 2
func BranchFor(name, branchFlag, prefix string) string      // -b tal cual; si no, prefix+name
func PathVars(main Worktree, name, branch, goos string) map[string]string
func ResolvePath(rendered, repoPath, home, goos string) (string, error)
func CheckWindowsPath(path string) error                    // solo si goos == "windows"
```

`PathVars` calcula `{repo}` (último componente del principal, sin `.git` si es bare), `{repo_parent}` y `{repo_path}` a partir del `Worktree` principal que ya devuelve `Build`, con las rutas ya resueltas de symlinks por `loadRepository`.

`ResolvePath` trabaja sobre texto, con `goos` como parámetro, como `worktree.Build`: así las reglas de Windows se testean desde macOS.

```
   macOS / Linux                          Windows
   -------------------------------------  -------------------------------------
   separador: /                           separadores: / y \  -> se normaliza a /
   ~ o ~/...  -> HOME                     ~ o ~\... o ~/... -> USERPROFILE
   relativa   -> repoPath + "/" + ruta    relativa -> repoPath + "\" + ruta
   path.Clean                             path.Clean sobre la forma con /,
                                          preservando el prefijo \\server\share
   resultado: /src/repo.worktrees/feat    resultado: C:\src\repo.worktrees\feat
```

`CheckWindowsPath` aplica a cada componente, salvo la unidad (`C:`) y el prefijo UNC, las reglas de la spec (nombres reservados con o sin extensión, `< > : " | ? *`, control, punto o espacio final). Comparte con `Sanitize` la tabla de nombres reservados.

La rama se valida con git (`check-ref-format --branch`) y no con una regla propia: el criterio de qué es una rama válida es de git y cambia entre versiones (hecho 3). `wt` exige además que la salida sea igual a la entrada, para que nada se expanda (`@{-1}`).

*Alternativa descartada:* reimplementar `check-ref-format`. Son una docena de reglas que git ya mantiene, y la invocación cuesta ~5 ms.
*Alternativa descartada:* `filepath.Clean` y `filepath.Abs`. Dependen del OS que compila, y las reglas de Windows dejarían de testearse en macOS.

### D4. El borde de `wt create`: orden de lecturas, chequeos y efectos

`internal/cli/create.go`, en este orden. Cada paso solo lee hasta que el plan está completo; las escrituras van todas al final.

```
   cobra: args y flags                                          exit 2
   ValidateName, --cd/--no-cd, -x vacío, -x con --json          exit 2
   workdir (-C)                                                 exit 2
   requireRepository  (git worktree list)                       exit 3
   loadConfig(repo.root())                                      exit 1
   branch := BranchFor(...);  git check-ref-format --branch     exit 2
   path := Render + ResolvePath + CheckWindowsPath              exit 2
   os.Lstat(path) ok  /  worktree registrado en path            exit 5
   git for-each-ref refs/heads/<branch>  (local)                exit 5
   base := --base | default_base | DefaultBranch(lecturas)      exit 3
   remote := FetchRemote(base, git remote)  si fetch_before_create
   ── si no es --dry-run: git fetch <remote>   (falla -> warning)
   head := git rev-parse --verify <base>^{commit}               exit 3
   git for-each-ref refs/remotes/  -> <remote>/<branch>         exit 5
   plan := CreatePlan{Name, Path, Branch, Base, Head, Fetch, CD, Exec}
   ── --dry-run: imprimir el plan (texto o wt.create.v1) y salir 0
   git branch --no-track <branch> <head>                        exit 1
   git worktree add <path> <branch>      (falla -> git branch -D, exit 1)
   imprimir "created worktree ..." o wt.create.v1
   moverse ? (activa ? WriteDirective : warning)                exit 1
   -x ? process.Run -> exit code del comando
```

- **Rama y worktree en dos pasos** (hecho 1). `git branch` falla si la rama ya existe, así que si tuvo éxito la rama es de `wt` y se puede borrar con `-D` cuando `worktree add` falla. Con `worktree add -b`, ante una carrera, `wt` no podría distinguir su rama de una ajena. La rama nace del **commit resuelto** (`head`), no del nombre de la base: lo que se crea es exactamente lo que `--dry-run` y `--json` informan. `--no-track` hace explícito el "sin upstream" (hecho 2), aunque con un commit id como base git no lo configuraría igual.
- **Chequeo local antes del fetch, remoto después.** Con una rama local existente, `wt create` falla sin tocar la red; las ramas recién empujadas a un remoto se ven porque el chequeo remoto va después del fetch.
- **"Existe"** es `os.Lstat` sin error. Cualquier error de `Lstat` (no existe, o un componente padre es un archivo) deja seguir, y si la ruta no sirve, falla git con exit 1 y la rama se borra (escenario "Git fails while creating").
- **Worktree registrado sin directorio**: se busca en `repo.worktrees` (ya leído) una entrada con la misma ruta, con la comparación de rutas de `git-worktrees` (sin distinguir mayúsculas en macOS y Windows).
- **Rama por defecto**, pura: `DefaultBranch(originHEAD, main Worktree, bareHEAD string) (string, bool)`. `originHEAD` sale de `for-each-ref … refs/remotes/origin/HEAD` (hecho 5); la rama del principal sale de `Build`, salvo para un bare, que no la informa (hecho 6) y se lee con `git symbolic-ref -q --short HEAD` en su directorio.
- **Remoto a traer**, puro: `FetchRemote(base string, remotes []string) (string, bool)`, el remoto más largo tal que `base` empieza con `<remoto>/`.
- **Ramas tomadas**, puro: `BranchTaken(branch string, refs, remotes []string, ws []Worktree) error`, que devuelve `*BranchExistsError{Branch, Remote, Worktree}` con lo necesario para el mensaje y el hint `wt cd`.

Las lecturas nuevas van a `internal/git/refs.go` (`CheckBranchName`, `Refs`, `Remotes`, `OriginHEAD`, `SymbolicHEAD`, `ResolveCommit`, `Fetch`, `CreateBranch`, `DeleteBranch`, `AddWorktree`), todas sobre `git.Runner`, con su test contra git real. `ResolveCommit` traduce "exit 1 sin stderr" en `git.ErrNoRevision`; `--end-of-options` impide que una base que empieza con `-` se lea como flag.

*Alternativa descartada:* `git worktree add -b` en un solo paso y limpiar si falla. Ver arriba: no se puede saber de quién es la rama.
*Alternativa descartada:* chequear todas las ramas, locales y remotas, después del fetch. Una rama local repetida se descubriría recién después de esperar a la red.

### D5. `git fetch`

`git fetch <remoto>`, sin flags: respeta la config de git del usuario (`fetch.prune`, `remote.<r>.fetch`). Corre por el mismo `git.Runner`, así que su salida queda capturada y su entorno no lleva las variables del protocolo. Si falla, el warning lleva la primera línea del error de git:

```
wt: warning: cannot fetch origin (<primera línea de git>); using the references already here
```

Bajo `--dry-run` no se ejecuta: el plan imprime `would fetch origin`, y la base y las ramas remotas se chequean contra lo que ya hay (lo dice la spec).

*Alternativa descartada:* `git fetch <remoto> <rama-de-la-base>`. Más rápido, pero no trae las ramas remotas nuevas que el chequeo de "Only new branches" necesita ver.
*Alternativa descartada:* `--prune` (como v0.9) o `--no-prune`. Las dos pisan una decisión que el usuario ya tomó en su config de git.

### D6. `-x`: paquete `internal/process`

```go
// Command devuelve cómo correr cmdline con el intérprete de goos. Pura.
func Command(goos, cmdline, comspec string) (path string, args []string, rawCmdLine string)

// Run corre el comando en foreground y devuelve su exit code. err solo si no arrancó.
func Run(dir, cmdline string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
```

```
   macOS / Linux                           Windows
   ------------------------------------    ------------------------------------------
   /bin/sh -c <cmd>                        %ComSpec% (o cmd.exe del PATH)
   argv normal de exec.Cmd                 SysProcAttr.CmdLine =
                                             cmd.exe /d /s /c "<cmd>"
   señal -> 128 + n  (WaitStatus)          ExitCode() tal cual (p. ej. 0xC000013A)
   signal.Notify(SIGINT) mientras corre    signal.Notify(os.Interrupt) mientras corre
```

- **Intérprete fijo**, el mismo que `product.md` fija para los hooks: `sh -c` en unix, `cmd.exe` en Windows. `/d` saltea los comandos `AutoRun` del registro; `/s` con la línea entre comillas hace que `cmd` use el resto tal cual, el patrón que usa Node con `shell: true` (hecho 8). `process_unix.go` y `process_windows.go` separan `SysProcAttr` y la lectura del exit code.
- **Foreground**: `exec.Cmd` recibe `Env.Stdin`, `Env.Stdout` y `Env.Stderr`. En producción son `*os.File`, y `os/exec` le pasa los descriptores tal cual al hijo: el agente ve la TTY, con color e input interactivo. En los tests son buffers.
- **Ctrl-C**: la terminal manda SIGINT (o `CTRL_C_EVENT`) a todo el grupo de procesos, `wt` incluido. Mientras el hijo corre, `wt` registra `signal.Notify` y descarta lo que llega; al terminar, `signal.Stop`. Notify y no `signal.Ignore`, por el hecho 7.
- **Entorno**: el mismo `shell.ChildEnviron` que ya usa git; el comando no ve `WT_DIRECTIVE_CD_FILE` ni `WT_PREVIOUS_DIR`.
- **Sin `context`**: el hijo se crea con `exec.Command`, no `CommandContext`; nada de `wt` lo mata.

El exit code llega a `Run` con un error nuevo en `cli`:

```go
// exitStatus termina el comando con code sin imprimir nada: el de -x.
type exitStatus struct{ code int }
```

`cli.Run` lo reconoce con `errors.As` antes que a `*Error` y devuelve `code` tal cual. `-x` exitoso con 0 devuelve `nil`.

*Alternativa descartada:* el `$SHELL` del usuario. No existe en Windows, y en unix haría que `-x` se comporte distinto según quién lo corra; los hooks de M3 necesitan la misma regla.
*Alternativa descartada:* `pwsh -Command` en Windows. No está garantizado en todas las máquinas, arranca en ~300 ms, y los hooks usan `cmd.exe`.
*Alternativa descartada:* `-x <prog> -- <args…>` sin intérprete. Contradice la firma `-x <cmd>` de `product.md` y obliga a escribir `sh -c` a mano para `&&`.

### D7. Salida y mensajes

```
created worktree /src/repo.worktrees/feat on new branch feat from origin/main

would fetch origin
would create worktree /src/repo.worktrees/feat on new branch feat from origin/main
would change directory to /src/repo.worktrees/feat
would run claude

wt: warning: shell integration is not active, so the shell stays where it is; see 'wt shell init --help'

wt: branch "feat" already exists
hint: it is checked out in worktree feat: run 'wt cd feat'

wt: branch "feat" already exists on remote origin
hint: choose another branch with -b

wt: path already exists: /src/repo.worktrees/feat

wt: a worktree is registered at /src/repo.worktrees/feat, but its directory is missing
hint: run 'git worktree prune' to forget it

wt: base not found: nope

wt: the repository has no default branch
hint: pass --base <ref>
hint: or set default_base in the configuration

wt: invalid branch name "jm/a b"
hint: it is branch_prefix ("jm/") followed by <name>

wt: path is not valid on Windows: C:\src\nul
hint: apply the sanitize filter in worktree_path, as in {name|sanitize}
```

`--json`: `{"schema":"wt.create.v1","name","path","branch","base","head"}`, igual con y sin `--dry-run`. El `Long` del help explica que `-x` corre con `sh` o `cmd.exe` (y que en PowerShell las comillas pasan primero por pwsh y después por `cmd`), cómo se arma la rama y la ruta, y `--cd`/`--no-cd` frente a `create_cd`.

### D8. Completions de `--base`

`cmd.RegisterFlagCompletionFunc("base", …)`: `workdir` + `git for-each-ref --format=%(refname:short) refs/heads refs/remotes`, sin las que terminan en `/HEAD`, con `withPrefix` y `ShellCompDirectiveNoFileComp`. No carga config, como `wt cd`; ante cualquier error, nada. `<name>` y `-b` devuelven `NoFileComp` sin valores. Los scripts de los cuatro shells no cambian: las completions de flags ya pasan por `__complete`.

### D9. Tests

- **Puros**, con tablas: `template` (sintaxis, errores con la parte ofensiva, la tabla de `Sanitize` de la spec, `lower`), `worktree/create.go` (`ValidateName`, `BranchFor`, `PathVars`, `ResolvePath` y `CheckWindowsPath` con `goos` darwin, linux y windows desde macOS, `DefaultBranch`, `FetchRemote`, `BranchTaken`), `config` (claves, `FromEnv`, string en TOML para un booleano, plantilla inválida nombrando la clave y el origen) y `process.Command`.
- **git real**: cada función de `refs.go` en un sandbox. `testutil` suma helpers para un `origin` bare (`InitRemote`, `Clone`, `Commit`, `Push` desde un segundo clon) y para un remoto inalcanzable (URL a un directorio inexistente), sin red.
- **`process.Run` por OS**: en unix, exit 3, `&&`, `kill -INT $$` → 130, directorio de trabajo y entorno filtrado; en Windows, `exit /b 3` y `&`. Los de Windows se cierran con el CI de `windows-latest`.
- **CLI in-process** (`harness`): un test por escenario de `create-worktree` que no necesite una shell real.
- **Binario** (`cmd/git-wt`): el escenario de interrupción en unix, mandando SIGINT al pid de `git-wt` mientras `-x "sleep 1; exit 7"` corre, y esperando exit 7.
- **Shell real** (`shell_test.go`): "Through the function", "Staying put", "The shell ends in the worktree" (`-x "exit 1"`: resultado 1 y la shell adentro) y "Protocol variables not passed", en zsh, bash, fish y pwsh, con la infraestructura de `shell-integration`.

## Risks / Trade-offs

- **[Riesgo] Comillas de `-x` en Windows.** El texto pasa por pwsh (la función), después por `cmd.exe`. `wt create x -x 'claude "fix it"'` llega bien, pero una expresión de PowerShell no. → Se documenta en el help de `wt create`, y se verifica a mano en Windows Terminal (tarea **[Windows]**).
- **[Riesgo] Ctrl-C en Windows.** Llega `CTRL_C_EVENT` a todos los procesos de la consola; `wt` lo descarta y el agente decide. Si el agente muere por eso, el exit code es `0xC000013A` y `$LASTEXITCODE` muestra `-1073741510`. → Es el código real del proceso, como pide la spec. Verificación manual **[Windows]**.
- **[Riesgo] TTY del agente en Windows** (función de pwsh → `git-wt` → `cmd.exe` → agente). Es la misma cadena que ya se verificó para `wt list`, más un nivel. → Verificación manual **[Windows]** con un programa interactivo.
- **[Riesgo] Fetch lento y mudo.** Su salida queda capturada; con una red lenta, `wt create` parece colgado unos segundos. Si git pide credenciales, lo hace por la TTY (ssh y los credential helpers abren `/dev/tty`), sin contexto de `wt`. → `fetch_before_create = false` o `--base` local lo evitan. Se revisa si molesta en el uso diario de M1.
- **[Riesgo] Carrera entre los chequeos y la creación** (otro proceso crea la rama o la ruta en el medio). → `git branch` falla sin crear nada y `wt` sale con exit 1 y el mensaje de git; si falla `worktree add`, `wt` borra solo la rama que creó.
- **[Riesgo] Mayúsculas en refs en macOS.** Si existe la rama `feat` y se pide `Feat` con otra carpeta (`-b Feat`), el chequeo exacto no la ve, pero git puede fallar al crear `refs/heads/Feat` sobre APFS. → Exit 1 con el mensaje de git y sin restos. Caso raro; no se agrega una regla por OS para ramas.
- **[Trade-off] `{repo}` con el layout `proj/.bare`.** `{repo}` es `.bare`, y la ruta por defecto queda `proj/.bare.worktrees/x`. → Se arregla con `worktree_path` explícita; no se agrega una heurística.
- **[Trade-off] Prefijo más nombre con barra.** Con `branch_prefix = "jm/"`, `wt create feature/x` deja la carpeta `feature-x` y la rama `jm/feature/x`: `wt cd feature/x` no la encuentra (`wt cd feature-x` sí). → Non-goal del proposal; sería un cambio de `navigate`.
- **[Trade-off] Una plantilla rota rompe todos los comandos**, `wt cd` incluido. → Es la regla vigente de `config-layers` para valores inválidos, y el mensaje nombra la clave, el archivo y la parte.

## Migration Plan

No hay `wt create` previo en `main`, así que nadie tiene worktrees creados con el default viejo de `worktree_path`. Quien venga de v0.9 encuentra el mismo layout (`{repo_parent}/{repo}.worktrees/{name}`), ahora con `sanitize`. Se actualizan `docs/design/product.md` (§4, §5, §7 y D3 en §9) y `CHANGELOG.md`.
