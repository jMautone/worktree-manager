# Design: batch-execution

## Context

El por qué está en `proposal.md`, y el comportamiento en `specs/`. Lo que ya existe y moldea el diseño:

- **`-x` ya resuelve la ejecución de un comando.**
  - `process.Command(goos, cmdline, comspec)` es puro: arma `sh -c` o la línea de `cmd.exe /d /s /c "…"`.
  - `process.Run(dir, cmdline, env, stdin, stdout, stderr)` corre el comando con la terminal de `wt` y devuelve su exit code (128 + señal en Unix).
  - Mientras el comando corre, `Run` registra `os.Interrupt` con `signal.Notify` y lo descarta.
  - `create` lo llama a través de la variable `runCommand`, que los tests reemplazan, y con `shell.ChildEnviron`, que saca las variables del protocolo de la shell.
- **`wt list --all-repos` ya arma el conjunto que `each` recorre.** `listAll` (`internal/cli/list.go`) hace, en orden:
  1. `loadRepository` opcional;
  2. `loadConfig("")` y el chequeo de raíces;
  3. `discoverRepos`;
  4. `scanWorktrees` en paralelo;
  5. `newWorkspaceNames`;
  6. `readable`, que escribe los warnings de los repos ilegibles.

  `wt list` sin el flag usa `loadRepository` + `loadConfig(repo.root())` y suma `hintListAll` cuando no hay repo y sí hay raíces.
- **`worktree.Build` ya ordena** (principal primero, después por NAME) y marca `Main`, `Bare`, `Prunable` y `Locked`. Un worktree bloqueado cuyo directorio no existe no es `Prunable` para git, así que saber si el directorio existe exige un `os.Stat`.
- **El formato de error tiene un solo lugar.** `Run` (`internal/cli/run.go`) imprime cualquier `*Error` como `wt: <msg>` más líneas `hint:`, y sale con su `Code`. `jsonRequested(args)` decide el formato de un error que ocurre antes de que cobra parsee `--json`, mirando `args` hasta un `--`.
- **cobra v1.10.2 y pflag v1.0.9**:
  - `FlagSet.SetInterspersed(false)` hace que pflag deje de reconocer flags en el primer argumento posicional; `docker run IMAGE CMD…` usa lo mismo.
  - La completion de cobra ya respeta eso: `completions.go` (línea 364, "Check if interspersed is false or -- was set on a previous arg") deja de ofrecer flags después del primer posicional.
- **El runtime de Go en Windows** convierte `CTRL_C_EVENT` y `CTRL_BREAK_EVENT` en `SIGINT` (`runtime/os_windows.go`, `ctrlHandler`). Por eso `signal.Notify(ch, os.Interrupt)` recibe el Ctrl-C de la consola igual que en Unix.
- **PowerShell borra el `--`.** Medido en pwsh 7.6.6 (brew, macOS): una función simple con `@args` recibe `wt each -- git status -s` como `each git status -s`, porque `$args` ya llega sin el `--`. `'--'` entre comillas sí llega. zsh, bash y fish lo pasan. Probado también contra la función real de `wt.ps1` con `wt config get -- --x`.

## Goals / Non-Goals

**Goals:**
- Que todas las decisiones sean puras y se prueben desde macOS con `goos` como parámetro:
  - qué se corre, qué se saltea y en qué orden;
  - la etiqueta de cada worktree;
  - la línea de comando de cada OS;
  - el texto del resumen.
- Que los únicos efectos nuevos sean el `os.Stat` de cada worktree, el bucle que escribe las cabeceras y corre los comandos, y la escucha de Ctrl-C.
- Reusar sin cambios `process.Run`, `process.Command` y la lectura de `wt list --all-repos`.
- Que `wt each` funcione igual en las cuatro shells, con o sin `--`.

**Non-Goals:**
- Capturar o prefijar la salida de los comandos: va directo a la terminal.
- Cambiar `-x`, que sigue descartando el Ctrl-C y propagando el exit code.
- Una abstracción de "lote" para `wt sync` (M3): cuando llegue, se decide si reusa algo de acá.

## Decisions

### D1. Qué es puro y dónde vive

```
  args ──> cobra (SetInterspersed(false)) ──> words []string
                                                  |
                                                  v
                                    process.Line(goos, words)          PURO
                                                  |
  repo actual: loadRepository + loadConfig        |                   EFECTO
  --all-repos: a.workspaceWorktrees (de listAll)  |                   EFECTO
           |                                      |
           v                                      |
     []worktree.EachGroup ──> os.Stat de cada ruta no bare            EFECTO
           |                        |                                 
           v                        v                                 
     worktree.PlanEach(groups, gone) ──> EachPlan{Run, Gone}           PURO
                                                  |
                     --dry-run: "would run <line> in <path>"          EFECTO (stdout)
                                                  |
                                                  v
     bucle: interrupción? -> cabecera -> runCommand -> resultado      EFECTO
                                                  |
                                                  v
     eachReport(results, len(Run), interrupted) -> nil | *Error      PURO
```

Tipos y funciones nuevos:

```go
// internal/process (puro; no depende de runtime.GOOS)
func Line(goos string, words []string) string

// internal/worktree
type EachGroup struct {
	Repo      string     // nombre visible; "" fuera de --all-repos
	Worktrees []Worktree // en el orden de Build
}
type EachTarget struct {
	Label    string // "^", "feat", "api:^", "api:feat"
	Worktree Worktree
}
type EachPlan struct {
	Run  []EachTarget // en orden: grupos, y dentro de cada grupo el de Build
	Gone []EachTarget // directorio inexistente: un warning cada uno
}
func PlanEach(groups []EachGroup, gone map[string]bool) EachPlan
func Label(repo string, w Worktree) string
```

`PlanEach` descarta los `Bare` sin dejar rastro y manda a `Gone` los que tienen su `Path` en `gone`, sin mirar `Prunable` ni `Locked`: lo que decide es si el directorio existe. `gone` lo arma la CLI con `os.Stat`. Así `PlanEach` no toca el filesystem y se prueba con un mapa.

`eachReport` vive en `internal/cli/each.go`, porque el texto del mensaje y el exit code son de la CLI. Es pura y se prueba con tests internos.

*Alternativa descartada:* pasar a `PlanEach` una `func(string) bool` respaldada por `os.Stat`. Rompe la regla del repo de leer primero y decidir sobre datos.

### D2. Dónde empieza el comando

```go
cmd := &cobra.Command{
	Use:  "each [--all-repos] [--] <cmd>...",
	Args: cobra.ArbitraryArgs,
	ValidArgsFunction: func(...) (...) { return nil, cobra.ShellCompDirectiveDefault },
	RunE: action(...),
}
cmd.Flags().SetInterspersed(false)
cmd.Flags().BoolVar(&allRepos, "all-repos", false, "...")
```

- Con `SetInterspersed(false)` en el `FlagSet` del subcomando (cobra le suma los flags persistentes de la raíz antes de parsear), el primer argumento posicional corta. Desde ahí todo es `args`, incluidos `--json`, `-C` o `--dry-run`.
- pflag consume un `--` que aparezca antes del primer posicional, así que `wt each -- x` y `wt each x` dan los mismos `args`. Eso es lo que hace que pwsh, que borra el `--`, se comporte igual que las otras shells.
- La validación va en `RunE`, antes de leer nada:
  1. `--json` → `usageError`, con el hint `run 'wt list --json' to read the worktrees`;
  2. `len(args) == 0` → `missing <cmd> for "wt each"`;
  3. un solo arg en blanco (`strings.TrimSpace == ""`) → `empty command for "wt each"`, como `-x`.
- `jsonRequested` deja de mirar en el primer argumento que no es un flag después de `each`. Si no, en `wt each --bogus npm test --json` el `--json` de npm haría que el error de uso de `--bogus` saliera como JSON. La regla salta el valor de `-C`, el único flag global con valor.
- Completion: `ShellCompDirectiveDefault` hace que la shell complete archivos. Los flags de `wt` desaparecen solos después de la primera palabra, por la guarda de cobra citada en Context.

*Alternativas descartadas:*
- `--` obligatorio (`ArgsLenAtDash`, como v0.9): en pwsh no llega, así que no se podría usar.
- `DisableFlagParsing`: pierde `--all-repos`, los globales y la validación estricta de flags, y obliga a parsearlos a mano.
- Arreglarlo en `wt.ps1`: `$args` ya llega sin el `--`, y reconstruirlo desde `$MyInvocation.Line` es frágil (alias, splatting, varias líneas).

### D3. La línea de comando por OS

`process.Line(goos, words)`:

- **Una palabra**: se devuelve tal cual. Es el mismo contrato que `-x`.
- **Varias palabras, macOS y Linux**:
  - una palabra que solo tiene `[A-Za-z0-9@%+=:,./_-]` va sin comillas;
  - cualquier otra, incluida la vacía, va entre `'…'`, con cada `'` escrito como `'\''`.

  Dentro de comillas simples `sh` no interpreta nada, así que la palabra llega exacta. Dejar sin comillas las palabras simples hace que `--dry-run` muestre `git commit -m 'fix bug'` y no `'git' 'commit' '-m' 'fix bug'`.
- **Varias palabras, Windows**, en dos pasos:
  1. **Comillas para el programa** (reglas del runtime de C de Microsoft):
     - la palabra va entre `"…"` si está vacía o tiene espacio, tab, `"` o alguno de `&|<>^()`;
     - cada `"` interior se escribe `\"`;
     - las barras invertidas que preceden a un `"`, o al `"` de cierre, se duplican.

     Es `syscall.EscapeArg`, más dos agregados: las comillas también cuando hay operadores de cmd, y la implementación en código puro, porque `syscall.EscapeArg` solo compila en Windows.
  2. **Escape para `cmd.exe`**: se recorre la línea ya unida llevando el estado de comillas de cmd, que cambia en cada `"`, incluidos los de `\"`. Cada `&|<>^()` que queda fuera de comillas para cmd se prefija con `^`.

     Eso solo pasa después de un `\"`. Por ejemplo, `a "&" b` se escribe `"a \"&\" b"`, cmd ve el `&` afuera, y queda `"a \"^&\" b"`. cmd saca el `^` antes de lanzar el programa, que recibe `a "&" b`.
- `%NOMBRE%` no se escapa: cmd lo expande en cualquier palabra, que es lo que dice la spec.
- El resultado entra en `process.Command`, que ya lo envuelve en `cmd.exe /d /s /c "…"`.

Ejemplo, `wt each git commit -m "fix bug"`:

```
  macOS/Linux:  sh -c "git commit -m 'fix bug'"
  Windows:      cmd.exe /d /s /c "git commit -m "fix bug""
```

*Alternativas descartadas:*
- Unir con espacios (opción A, v0.9): `-m "fix bug"` llega como `-m fix bug`.
- Ejecutar sin intérprete cuando hay varias palabras (opción C): en Windows, `npm` es `npm.cmd` y necesita cmd.exe igual, y en Unix la misma línea se comportaría distinto según tenga una o varias palabras.
- Escapar todo con `^`, incluidas las comillas, para que `%` tampoco se expanda: cmd deja de ver las comillas al buscar el programa, y una primera palabra como `C:\Program Files\x\y.exe` se parte en el espacio.
- `syscall.EscapeArg` a secas: no pone comillas a `a&b`, que cmd interpreta como dos comandos.

### D4. Lectura de los worktrees

- **Repo actual**:
  1. `loadRepository(ctx, dir)`;
  2. si da `NotRepoError`, el mismo error que `wt list`, pero con un hint propio que nombra `wt each --all-repos`, solo cuando `loadConfigQuiet("")` tiene raíces;
  3. `loadConfig(repo.root())`, para que un `.wt.toml` inválido sea exit 1 como en `wt list`;
  4. un solo `EachGroup` con `Repo: ""`.
- **`--all-repos`**: se factoriza de `listAll` la función `a.workspaceWorktrees(ctx, dir) ([]repoWorktrees, workspaceNames, error)`. Cubre todo hasta `readable` inclusive, con sus warnings, y la usan `wt list --all-repos` y `wt each --all-repos`. Cada repo legible es un `EachGroup` con su nombre visible.
- `gone`: un `os.Stat` por cada worktree no bare. Cualquier error del `Stat`, no solo `ErrNotExist`, cuenta como directorio inexistente, porque ahí no se puede correr nada.

### D5. El bucle y Ctrl-C

```go
interrupts := make(chan os.Signal, 1)
signal.Notify(interrupts, os.Interrupt)   // al empezar RunE
defer signal.Stop(interrupts)
...
stopped := false
for _, t := range plan.Run {
	if stopped = pending(interrupts); stopped {        // select no bloqueante
		break
	}
	fmt.Fprintf(stdout, "%s %s  %s\n", arrow, label, t.Worktree.Path)
	code, err := runCommand(t.Worktree.Path, line, env, stdin, stdout, stderr)
	results = append(results, eachResult{Label: t.Label, Code: code, Err: err})
}
stopped = stopped || pending(interrupts)  // también si llegó durante el último
return eachReport(results, len(plan.Run), stopped)
```

- **El `Notify` va al principio de `RunE`, no alrededor de cada comando.** Mientras haya un canal registrado, Go no termina el proceso con SIGINT, así que un Ctrl-C entre dos comandos, mientras se escribe una cabecera o durante la lectura de los repos, también llega a ese canal. La regla queda literal: un Ctrl-C mientras `wt each` corre se reporta y sale con 130.
- **`process.Run` no cambia.** Sigue registrando su propio canal mientras corre el comando, y `signal` le entrega la señal a todos los canales registrados. El canal de `each` se entera; el de `Run` la descarta como hoy.
- **El Ctrl-C le llega al comando sin hacer nada**, porque comparte grupo de procesos (Unix) o consola (Windows). `wt` no le reenvía nada. El comando decide cómo terminar, y `wt` espera a que termine.
- **`eachReport`**:
  - interrumpido → `&Error{Code: ExitInterrupted, Msg: "interrupted: <k> of <m> commands did not run", Hints: fallos}`;
  - si no, con fallos → `&Error{Code: ExitError, Msg: "<f> of <m> commands failed", Hints: fallos}`;
  - si no, `nil`.

  Un fallo es `code != 0` o `err != nil`, y su hint es `<label> exited with <code>` o `<label> could not start: <err>`.
- `ExitInterrupted = 130` se suma a `exit.go`, con el comentario de que vale lo mismo en los tres OS.

En Windows el mecanismo es el mismo `os.Interrupt` (ver Context), y `exitCode` de `process_windows.go` no interviene: el 130 lo pone `wt`, no el comando.

*Alternativas descartadas:*
- Salir con 1 al interrumpir: un script no distingue "lo corté yo" de "algo falló", y con un comando que atrapa Ctrl-C y sale con 0 no habría ningún fallo que reportar.
- Re-enviarse SIGINT para morir por señal (lo cooperativo en Unix): no existe en Windows, y daría dos comportamientos donde la spec pide uno.
- Correr cada comando en su propio grupo de procesos (`Setpgid`): el Ctrl-C ya no le llegaría al comando, y un programa interactivo perdería la terminal en primer plano.

### D6. Cabeceras, salida y resumen

- La cabecera se escribe con `fmt.Fprintf(a.env.Stdout, …)` antes de `runCommand`. `os.Stdout` no tiene buffer en Go, así que sale antes que cualquier byte del comando, que escribe en el mismo descriptor.
- Con `a.color()`, `==>` va en negrita y la etiqueta con el estilo de la columna NAME de `wt list`. Sin color, el texto es exactamente el de la spec.
- La etiqueta sale de `worktree.Label(repo, w)`: `^` si `w.Main`; si no, `w.Name`; con `repo != ""`, se le antepone `repo + ":"`.
- El resumen es un `*Error`, así que lo imprime `Run` con el formato de siempre (`wt:` más `hint:`), después de la salida del último comando. No hace falta un camino de salida nuevo.
- Los warnings (repos ilegibles, directorios inexistentes) se escriben antes del bucle, con `a.warn`.

### D7. `--dry-run`

Hace la misma lectura y el mismo plan, escribe los warnings, y por cada `EachTarget` de `Run` imprime `would run <line> in <path>`, con la línea de `process.Line`. No instala el `Notify` del bucle ni corre nada, y sale con 0.

### D8. Documentación

- **`Long` de `wt each`**:
  - el `--` opcional y que los flags de `wt` van antes del comando;
  - una palabra contra varias, con `wt each 'npm ci && npm test'` y `wt each git commit -m "fix bug"`;
  - que un glob sin comillas lo expande la shell del usuario en el directorio actual (`wt each 'ls *.go'`);
  - la cabecera, que sigue cuando uno falla, el exit 1 y el 130 con Ctrl-C;
  - que en Windows `%NOMBRE%` se expande.
- **`docs/design/product.md`**:
  - §4: la fila de `wt each` pasa a `wt each [--all-repos] [--] <cmd>...`, se quita la de `wt exec` y se suma el 130 a la tabla de exit codes;
  - §7: se actualiza la fila de `batch-execution`.

## Risks / Trade-offs

- **[Un usuario de pwsh escribe `--` sin saber que se pierde]** → No pasa nada: el `--` es opcional por D2, y queda documentado en la spec de `shell-integration`.
- **[`%NOMBRE%` se expande en Windows aunque la palabra vaya entre comillas]** → Es una diferencia por OS declarada en la spec y en el `Long`. Si alguna vez molesta, el escape con `^` solo es posible fuera de comillas y rompería las rutas con espacios (D3).
- **[En Windows, un `.cmd` interrumpido pregunta `Terminate batch job (Y/N)?`]** → Si se contesta N, ese comando sigue, pero `wt each` igual no arranca el siguiente, porque ya vio el Ctrl-C. Se verifica a mano en la tarea **[Windows]** y se anota.
- **[El runner de Windows del CI puede no tener consola para `GenerateConsoleCtrlEvent`]** → El test de Ctrl-C en Windows crea `git-wt` con `CREATE_NEW_PROCESS_GROUP` y le manda `CTRL_BREAK_EVENT`, que Go mapea a `os.Interrupt`. Si el runner no tiene consola, el test se saltea con `t.Skip` y vale la verificación manual.
- **[Un comando que lee todo stdin]** (por ejemplo `wt each cat` con entrada por pipe) → El primero se lo come y los demás reciben EOF, igual que en un `for` de shell. No se especifica ni se mitiga.
- **[Un comando interactivo que toma la terminal en primer plano]** (por ejemplo `wt each zsh`) → Un Ctrl-C adentro no le llega a `wt`, porque la subshell es el grupo en primer plano. Al salir de ella, `wt each` sigue con el siguiente worktree. Es lo esperable y no se especifica.
- **[Ctrl-C mientras se leen los repos con `--all-repos`]** → Los git hijos reciben el Ctrl-C y fallan, así que pueden aparecer warnings de repos ilegibles antes de `wt: interrupted: <m> of <m> commands did not run`. Es aceptable: el exit code es 130 igual.

## Migration Plan

No hay migración: es un comando nuevo. `wt exec` nunca existió en la línea Go. Rollback: revertir el squash del PR.
