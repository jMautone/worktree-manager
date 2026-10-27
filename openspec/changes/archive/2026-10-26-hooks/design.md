# Design: hooks

## Context

El por qué está en `proposal.md`, y el comportamiento en `specs/`. Lo que ya existe y moldea el diseño:

- **`-x` ya resuelve cómo correr un comando del usuario.**
  - `process.Command(goos, cmdline, comspec)` es puro: arma `sh -c` o la línea de `cmd.exe /d /s /c "…"`.
  - `process.Run(dir, cmdline, env, stdin, stdout, stderr)` corre el comando, espera y devuelve su exit code (128 + señal en Unix).
  - Mientras el comando corre, `Run` registra `os.Interrupt` con `signal.Notify` y lo descarta.
  - `create` lo llama a través de la variable `runCommand`, y con `shell.ChildEnviron`, que saca las variables del protocolo de la shell.
- **`wt each` ya resolvió Ctrl-C con 130.** Registra su propio canal con `signal.Notify` y, cuando el comando termina, mira con un `select` no bloqueante (`pending`) si llegó una interrupción. `process.Run` no cambió para eso: `signal` entrega la señal a todos los canales registrados.
- **La config es un mapa plano de claves.**
  - `config.Load` decodifica el TOML a `map[string]any`.
  - `config.Resolve` recorre las claves de cada capa contra el `Registry`: avisa las desconocidas, ignora las que el repo no puede setear y valida el tipo con `Key.Validate`.
  - `EnvLayer` lee `WT_<KEY>` para cada clave del registro.
  - Hoy, una tabla `[hooks]` llega como la clave `hooks` con un `map[string]any`, y da el warning de clave desconocida.
- **`configcmd.go` ya muestra listas**: `displayValue` y `wt config get` tratan cualquier `[]string` como lista (TOML array en `list`, una línea por elemento en `get`, array en JSON). Las listas de comandos entran por ahí sin cambios.
- **El motor de plantillas** (`internal/template`) parsea contra una lista de variables permitidas y renderiza con un `map[string]string`. `validateTemplate` lo usa para `worktree_path` al cargar la config. No tiene escape para `{` ni `}`.
- **Los planes ya separan decidir de hacer.**
  - `planCreate` → `worktree.CreatePlan` → `previewCreate` o `create`.
  - `planRemove` → `worktree.RemovePlan` → `previewRemove` o `remove`.
  - `cd` resuelve el destino y escribe la directiva.

  Los hooks se suman a esos planes; no se cuelgan de los efectos.
- **No hay helper para el directorio git común.** `loadRepository` usa `git worktree list --porcelain`, que no lo informa.
- **Los e2e de `cmd/git-wt`** compilan el binario real en `TestMain` y ya prueban señales en Unix (`interrupt_test.go`) y Ctrl-C en Windows con `CREATE_NEW_PROCESS_GROUP` (`interrupt_windows_test.go`).

## Goals / Non-Goals

**Goals:**
- Todas las decisiones puras, y probadas desde macOS con `goos` como parámetro:
  - qué evento corre;
  - en qué directorio;
  - con qué comandos ya renderizados;
  - si va en background;
  - dónde va su log.
- Que los únicos efectos nuevos sean:
  - correr un hook bloqueante (reusa `process.Run`);
  - arrancar el proceso de background;
  - abrir el log;
  - leer el directorio git común.
- Que `--dry-run` salga del mismo plan que la ejecución, sin `if` por todos lados.
- Cero costo cuando no hay hooks configurados: ni un `git` de más, ni un `os.Executable`.

**Non-Goals:**
- Una abstracción genérica de "pipeline" para `merge`: cuando llegue `merge-worktree`, suma sus eventos al mismo paquete `hook`.
- Cambiar `-x`, `process.Run` o `wt each`.
- Que el proceso de background reporte algo a una corrida posterior de `wt`.

## Decisions

### D1. Qué es puro y dónde vive

```
  config (hooks.<evento>: []string, ya validado)
          |
          v
  hook.Plan(event, commands, values, dir, log)  ──>  hook.Run      PURO
          |                                             |
  planCreate / planRemove / cd: arman values,           |
  dir y log según el evento                             |
          |                                             v
  --dry-run: hook.PreviewLines(run) ──> stdout                       EFECTO (stdout)
          |
          v
  bloqueante: runBlocking(run) ──> process.Run por comando           EFECTO
  background: startBackground(run) ──> git-wt __run-hooks (detached) EFECTO
```

Paquete nuevo `internal/hook`, sin git ni filesystem:

```go
type Event string

const (
	PreCreate  Event = "pre-create"
	PostCreate Event = "post-create"
	PreCD      Event = "pre-cd"
	PostCD     Event = "post-cd"
	PreRemove  Event = "pre-remove"
	PostRemove Event = "post-remove"
)

var Events = []Event{PreCreate, PostCreate, PreCD, PostCD, PreRemove, PostRemove}

func (e Event) Background() bool   // post-cd y post-remove
func (e Event) Key() string        // "hooks." + string(e)
func Vars(e Event) []string        // base solo en los de create

// Run es lo que un evento va a hacer, ya decidido.
type Run struct {
	Event    Event
	Dir      string   // dónde corren los comandos
	Commands []string // renderizados, en orden; vacío = el evento no hace nada
	Log      string   // ruta del log; "" si no hace falta
}

func Plan(e Event, templates []string, values map[string]string, dir, log string) (Run, error)
func LogPath(commonDir, name string, e Event, goos string) string // <common>/wt/logs/<name|sanitize>/<event>.log
func Header(e Event, command string) string                       // "==> <event>  <command>"
func PreviewLines(r Run) []string                                 // "would run <event> hook: …" o "… in the background: …"
```

- `Plan` vuelve a parsear cada plantilla con `Vars(e)`. No puede fallar si la config se validó, pero devuelve el error igual, así no hay un `panic` escondido si alguien llama a `Plan` con datos que no pasaron por `config`.
- `values` lo arma `internal/worktree` con una función pura nueva, `HookVars(main, w Worktree, base, goos string) map[string]string`. Reusa `PathVars` para `repo`, `repo_parent` y `repo_path`, y suma `name` (el NAME), `branch`, `path` y `base`.
- El NAME del worktree nuevo, en `create`, es el último componente de `plan.Path`, con la misma función que usa `Build`. Así `{name}` en `post-create` y `post-remove` salen del mismo lugar.

*Alternativa descartada:* poner los eventos y sus variables en `internal/template`, junto a `WorktreePathVars`. Los eventos son un concepto de hooks, no del motor, y `merge-worktree` los va a extender.

### D2. Config: claves con punto y la tabla `[hooks]`

- **Seis claves en el `Registry`**, `hooks.pre-create` … `hooks.post-remove`:
  - `Type: "command list"`, `Default: []string{}`, `InRepo: false`;
  - `Validate: validateCommands(event)`;
  - y un campo nuevo, `NoEnv: true`.
- **`Key.NoEnv`**: `EnvLayer` saltea esas claves. `WT_HOOKS.POST-CREATE` no es un nombre de variable válido en `sh`, y `WT_HOOKS_POST_CREATE` sería una regla de nombres solo para esto. La spec dice que no hay variable de entorno, y un `WT_HOOKS_*` cae en "variables que no corresponden a una clave".
- **Aplanado de tablas en `Resolve`**, no en `Load`:
  - `Load` no conoce el registro, y `Resolve` sí.
  - Antes del bucle de claves, por cada capa que no es env, si el registro tiene claves con prefijo `<t>.` y la capa tiene la clave `<t>`:
    - si el valor es `map[string]any`, se reemplaza por `<t>.<k>` = valor para cada `k`;
    - si es cualquier otra cosa, es el error `invalid value for "hooks" in <file>: expected a table, as in [hooks] post-create = ["…"]`.
  - Después, el bucle de siempre hace el resto:
    - `hooks.post-merge` da `unknown key "hooks.post-merge"`;
    - un `[hooks]` en `.wt.toml` da `ignoring "hooks.post-create" in .wt.toml: repository configuration may not set it`.
  - El aplanado es genérico por prefijo. `[launchers.*]` o `[aliases]` lo reusan cuando lleguen.
- **`validateCommands(e)`**:
  - Acepta `[]any` de strings (archivo) o `[]string` (default).
  - Un `string` falla con el mismo texto de `validatePathList`, `write it as ["npm ci"]`.
  - Un elemento en blanco falla con `element <n> is empty`.
  - Cada elemento pasa por `template.Parse(s, hook.Vars(e))`, así que un `{nope}` o un `{base}` en `pre-remove` falla al cargar, con el mismo `SyntaxError` que `worktree_path`.
  - Devuelve `[]string`, nunca `nil`.
- `config` importa `hook` (por `Vars` y `Events`). `hook` importa `template` y nada de `config`.

*Alternativas descartadas:*
- Una sola clave `hooks` de tipo tabla: `wt config get hooks` necesitaría un formato de salida nuevo, y `wt config list` no tiene cómo mostrar una tabla en una fila.
- Claves planas `hook_post_create`: el TOML del producto (§5) ya muestra `[hooks]`, y el analisis de worktrunk también.

### D3. Escape de llaves en `template.Parse`

`Parse` deja de copiar literales con `src[lit:i]` y los arma con un `strings.Builder`:

- `{` seguido de `{` → `{` literal, avanza dos;
- `}` seguido de `}` → `}` literal, avanza dos;
- `}` solo → el error de hoy;
- `{` seguido de otra cosa → expresión hasta el primer `{` o `}`, como hoy.

Leyendo de izquierda a derecha, `{{{name}}}` da `{`, la expresión `name` y `}`; `x}}}` da `x}` y después un `}` solo, que es error. Ninguna plantilla válida hoy cambia, porque `{{` y `}}` eran errores.

*Alternativa descartada:* expandir solo `{var}` de variables conocidas y dejar el resto literal. Un typo como `{pth}` pasaría en silencio, y un `${name}` de shell se expandiría.

### D4. Dónde entra cada evento

**`create`**: `CreatePlan` suma `PreHook, PostHook hook.Run`, y `planCreate` los arma al final, cuando ya sabe `Path`, `Branch`, `Base` y el principal.
- `pre-create` corre en `main.Path`, y `post-create` en `plan.Path`.
- `create` pasa a hacer:
  1. `runBlocking(PreHook)`;
  2. rama y worktree, como hoy;
  3. `runBlocking(PostHook)`;
  4. la directiva;
  5. la salida (`created worktree …` o el JSON);
  6. `-x`.
- La salida baja del paso 2 al 5 para que stdout quede vacío si falla `post-create`. Si `post-create` falla, se escribe la directiva igual y se devuelve el `*Error` con el hint de la ruta.

**`remove`**: `RemovePlan` suma `PreHook` y `PostHook`. El `Dir` de `pre-remove` es `w.Path` si existe y `main.Path` si no; el de `post-remove`, `main.Path`.
- `remove` pasa a hacer:
  1. `runBlocking(PreHook)`, antes del `Chdir` al principal, porque el hook corre en el worktree y termina antes del borrado;
  2. lo de hoy: borrado, rama y directiva;
  3. la salida;
  4. `startBackground(PostHook)`.
- Si falla el borrado de la rama, el worktree ya no está: `post-remove` arranca igual, y después se devuelve el error.

**`cd`**: después de `mustExist(dest)`, y solo si hay comandos en `pre-cd` o `post-cd` y no hay `--no-hooks`:
1. Se busca el worktree que contiene `dest`:
   - para nombres, repos, `^` y `@` ya está en la resolución;
   - para `-`, se hace `loadRepository(dest)`.

   Si no hay repo, o no hay worktree que lo contenga, no hay eventos.
2. `runBlocking(pre-cd)`.
3. La directiva.
4. `startBackground(post-cd)`.

`cdDestination` pasa a devolver también el `worktree.Worktree` y el principal cuando los tiene, para no repetir la lectura.

**Log**: `hook.LogPath` necesita el directorio git común, que se lee con `git.CommonDir` (D7) solo cuando hace falta un log: el evento es de background y tiene comandos, o es bloqueante con `--json`.

**`--no-hooks`**: un `bool` en `createFlags`, `removeFlags` y los flags de `cd`. Con él, los `Run` quedan vacíos y todo lo demás sigue igual.

### D5. Hooks bloqueantes

```go
// runHook corre un comando de hook; los tests lo reemplazan.
var runHook = process.Run

func (a *app) runBlocking(r hook.Run) error {
	if len(r.Commands) == 0 { return nil }
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	out, in, closeLog := a.env.Stderr, a.env.Stdin, func() error { return nil }
	if a.json { out, closeLog = openLog(r.Log); in = nil } // nil: dispositivo nulo
	defer closeLog()
	for _, c := range r.Commands {
		fmt.Fprintln(out, hook.Header(r.Event, c))   // a stderr, o al log con --json
		code, err := runHook(r.Dir, c, env, in, out, out)
		if a.json { writeFooter(out, code, err) }
		switch {
		case pending(interrupts): return hookInterrupted(r)  // 130
		case err != nil || code != 0: return hookFailed(r, c, code, err) // 7
		}
	}
	return nil
}
```

- **stdout del hook → stderr de `wt`.** Así el stdout de `wt` queda solo con lo suyo, y `wt create feat > out.txt` no se ensucia. Es la misma terminal, así que el hook detecta TTY en su stdout igual que antes.
- **Ctrl-C**: el mismo patrón que `wt each`. `process.Run` lo descarta mientras el hook corre, el hook decide cómo terminar, y el canal propio de `runBlocking` se entera. Se mira antes que el exit code, porque un hook que atrapa SIGINT y sale con 0 igual cuenta como interrumpido.
- **Los mensajes** (`pre-create hook failed: <cmd> exited with <n>`, `interrupted: <event> hook did not finish`) y sus hints viven en `internal/cli`, como `eachReport`. Son funciones puras con tests internos.
- **`--json`**: el log se abre con `O_TRUNC`, y las cabeceras y los pies van al log en el mismo formato que en background (D6).

### D6. Hooks en background: `git-wt __run-hooks`

`wt` no puede quedarse esperando y tampoco puede escribir el exit code de cada comando después de salir. Por eso arranca una copia de sí mismo, desacoplada, que corre los comandos en serie y escribe el log:

```
  git-wt remove feat
     |  os.Executable() ; MkdirAll(dir del log) ; abre el log con O_TRUNC
     |  exec: <self> __run-hooks --event post-remove --dir <main> -- <cmd1> <cmd2>
     |        stdin = nulo ; stdout = stderr = el log ; env = ChildEnviron
     |        + atributos de desacople (abajo)
     |  Start() ; Process.Release() ; escribe la línea "==> … log: <ruta>" ; sale
     v
  git-wt __run-hooks     (sigue vivo)
     por cada comando: "==> <event>  <cmd>" ; process.Run(dir, cmd, …, os.Stdout, os.Stdout)
                       "==> exited with <n>" | "==> could not start: <err>" ; corta si falla
```

- **`__run-hooks` es un comando de cobra con `Hidden: true`**. No aparece en el help ni en las completions, no lee la config ni el repo, y su contrato es interno. Usa `SetInterspersed(false)` como `each`, así los comandos llegan como args sin que pflag los toque.
- **El log lo abre el padre.** Así un error de permisos o de disco se reporta como el warning de "no se pudo arrancar", y no se pierde en un proceso sin terminal. El hijo hereda el handle como stdout y stderr.
- **Desacople por OS**, en `process` con build tags, con la función nueva `process.StartDetached(path string, args []string, dir string, env []string, out *os.File) error`:
  - **macOS y Linux**:
    - `SysProcAttr{Setsid: true}`: una sesión nueva sin terminal controlante. El SIGINT de Ctrl-C (que va al grupo de foreground) y el SIGHUP del cierre de la terminal no le llegan.
    - Cuando `wt` sale, el hijo queda adoptado por init. No hace falta doble fork, porque `wt` no espera y termina enseguida.
  - **Windows**:
    - `CreationFlags: CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW | CREATE_BREAKAWAY_FROM_JOB`.
    - `CREATE_NO_WINDOW` le da una consola propia, oculta, que heredan sus `cmd.exe`. No aparece ninguna ventana, y ni el Ctrl-C ni el cierre de la consola del usuario le llegan.
    - `CREATE_BREAKAWAY_FROM_JOB` lo saca del job object del padre, por si la terminal lo mata al cerrarse. Si el job no permite breakaway, `CreateProcess` devuelve `ERROR_ACCESS_DENIED`, y se reintenta sin ese flag.
- **`startBackground` es una variable** que los tests in-process reemplazan por una versión síncrona, que corre el mismo bucle de `__run-hooks` en el proceso. Así los tests de orden y de contenido del log no dependen de esperar a otro proceso. Los e2e de `cmd/git-wt` usan el real.

*Alternativas descartadas:*
- `sh -c "a && b" > log 2>&1 &`: no escribe la cabecera y el exit code de cada comando, la redirección y el `&` no existen igual en `cmd.exe`, y en Windows igual hacen falta los flags de desacople.
- Una goroutine: muere con `wt`.
- `DETACHED_PROCESS` en vez de `CREATE_NO_WINDOW`: el proceso queda sin consola, y cada `cmd.exe` que arranca abre una ventana nueva.
- Pasar los comandos por un archivo temporal: los args alcanzan, y `os/exec` los cita con las reglas del runtime de C que Go revierte en el hijo.

### D7. El directorio git común

`git.CommonDir(ctx, r, dir) (string, error)` corre `git rev-parse --git-common-dir`.

- Si la salida es relativa (git devuelve `.git` en la raíz del principal), se resuelve contra `dir` y se limpia. No se usa `--path-format=absolute` para no exigir git 2.31.
- Para un bare, la salida es el propio repo; para el layout `.bare`, `<root>/.bare`.
- Se llama una vez por comando, solo si hace falta un log (D4).

### D8. Salida en stderr y color

- `==> <event>  <command>` antes de cada comando bloqueante, y `==> <event>  running in the background, log: <ruta>` una vez por evento de background.
- Sin color: van a stderr, y `Env.IsTTY` habla de stdout. Sumar detección de TTY en stderr no paga lo que cuesta.
- Con `--json` no se escribe ninguna de las dos.

### D9. `docs/design/product.md`

- §5, Hooks:
  - `post-create` bloquea; `post-cd` y `post-remove` van en background;
  - la ruta del log;
  - el escape `{{`/`}}`;
  - los eventos que existen hoy.
- §4: la fila de `create` dice "corre `pre-create` y `post-create`", y la nota de `remove` sobre los hooks pasa de futuro a presente. `--no-hooks` en las filas de `create`, `cd` y `remove`.
- §7: la fila de `hooks` dice que `post-create` bloquea y nombra el motivo.

## Risks / Trade-offs

- **[Riesgo] Un hook de background que sobrevive en Windows depende del host.** Windows Terminal, conhost y VS Code se comportan distinto con los job objects. → Breakaway con fallback (D6), un e2e en el CI de Windows con `CREATE_NEW_PROCESS_GROUP`, y una verificación manual cerrando la pestaña, marcada **[Windows]**.
- **[Riesgo] Dos corridas del mismo evento sobre el mismo worktree pisan el log a la vez** (dos `wt cd feat` seguidos con un `post-cd` lento). → Se acepta: el log queda mezclado, pero nada se rompe. La spec solo promete que cada corrida reemplaza el contenido.
- **[Riesgo] Un worktree enlazado con el mismo NAME que el principal comparte el directorio de logs.** → Se acepta. Es raro (`/src/repo` y `/x/repo`) y solo afecta al log.
- **[Trade-off] `post-create` bloquea**: `wt create` tarda lo que tarde `npm ci`. → Es lo que pide `-x`. Quien no quiera esperar puede poner `npm ci &` (sh) o `start /b npm ci` (cmd), pero ese proceso no tiene log ni desacople garantizado.
- **[Trade-off] Los valores no se citan**: una ruta con espacios rompe `cp .env {path}/.env`. → La spec lo dice y los ejemplos usan comillas. Citar según el intérprete rompería a quien ya puso las comillas.
- **[Riesgo] Una plantilla de hook inválida rompe todos los comandos** (`wt list` incluido), como pasa con `worktree_path`. → Es la regla de config inválida que ya existe. El mensaje nombra la clave y el archivo.
- **[Riesgo] `cd` con hooks cuesta un `git worktree list` más en `wt cd -`.** → Solo cuando hay hooks de cd configurados. Sin hooks, el costo es cero (Goals).

## Migration Plan

Nada que migrar: las claves son nuevas, `[hooks]` antes daba el warning de clave desconocida, y `{{`/`}}` antes era un error. Rollback: revertir el PR. Los logs que hayan quedado en `.git/wt/logs/` no molestan.

## Open Questions

- ¿El e2e de supervivencia en Windows puede correr en `windows-latest` sin consola, o queda como `t.Skip` con la verificación manual? Se resuelve al escribir la tarea, como pasó con el Ctrl-C de `wt each`.
