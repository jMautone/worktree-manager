# Tasks

Las tareas **[Windows]** se cierran con el CI de `windows-latest`, salvo la verificación manual de 5.3. Los tests puros de Windows corren en cualquier OS, porque reciben `goos`. Los escenarios propios de Windows se escriben en 5.1 y 5.2, y se saltean en macOS y Linux.

## 1. Decisiones puras (`internal/process`, `internal/worktree`, `internal/cli`)

- [x] 1.1 Implementar `process.Line(goos, words)` según D3. Verificar con tests de tabla:
  - **una palabra**: se devuelve tal cual, con operadores (`echo a && echo b`, `echo a& echo b`) y con `%`.
  - **darwin y linux, varias palabras**:
    - sin comillas: `git`, `-m`, `--format=%h`, `a/b.c`;
    - entre comillas simples: `fix bug`, `a;b`, `$HOME`, `*.go`, `~` y la vacía (`''`);
    - `it's`, escrita como `'it'\''s'`;
    - `git commit -m "fix bug"`, que da `git commit -m 'fix bug'`.
  - **windows, varias palabras**:
    - `fix bug`, que da `"fix bug"`;
    - `a&b`, que da `"a&b"`, y `(x86)`, entre comillas;
    - la vacía (`""`);
    - `say "hi"`, que da `"say \"hi\""`;
    - `C:\dir x\`, con la barra final duplicada (`"C:\dir x\\"`);
    - `C:\dir\`, sin comillas y sin cambios;
    - `a "&" b`, que da `"a \"^&\" b"`;
    - `%WT_TEST%`, sin cambios.
- [x] 1.2 Implementar `worktree.Label`, `EachGroup`, `EachTarget`, `EachPlan` y `worktree.PlanEach(groups, gone)` según D1. Verificar con tests de tabla:
  - etiquetas `^` y `feat` con `Repo` vacío, y `api:^` y `api:feat` con `Repo` `api`;
  - el bare no queda ni en `Run` ni en `Gone`;
  - un worktree en `gone` va a `Gone` con su etiqueta, esté `Prunable`, `Locked` o ninguno;
  - el orden de `Run` sigue a los grupos y, dentro de cada uno, el orden recibido;
  - sin grupos, o con grupos vacíos, el plan queda vacío.
- [x] 1.3 Sumar `ExitInterrupted = 130` a `internal/cli/exit.go` e implementar `eachReport(results, planned, interrupted)` según D5. Verificar con tests internos de tabla:
  - todos con 0 → `nil`;
  - uno con 3 → `ExitError`, `1 of 3 commands failed` y `feat exited with 3`;
  - un comando que no arrancó → `<label> could not start: <err>`;
  - interrumpido después del primero de dos → `ExitInterrupted` e `interrupted: 1 of 2 commands did not run`, con los hints de los que fallaron;
  - interrumpido durante el último → `0 of 2`.

  Extender `exit_test.go` con el 130.

## 2. Lectura compartida con `wt list --all-repos`

- [x] 2.1 Factorizar de `listAll` la función `a.workspaceWorktrees(ctx, dir)` según D4, sin cambiar el comportamiento. Verificar con los tests existentes de `wt list --all-repos` (`list_all_test.go`) y de `wt list` en verde.

## 3. `wt each` (`batch-execution`, `cli-contract`)

- [x] 3.1 Crear `internal/cli/each.go` con:
  - el comando, con `SetInterspersed(false)`, `--all-repos`, `cobra.ArbitraryArgs`, su `Long` (D8) y el registro en la raíz;
  - la validación de D2 (`--json`, sin comando, comando en blanco) antes de leer nada;
  - el ajuste de `jsonRequested` de D2;
  - la lectura de D4, el `os.Stat` de `gone`, `PlanEach`, los warnings y `--dry-run` (D7).

  Verificar in-process:
  - de `batch-execution`:
    - "Outside a repository", "No roots configured", "Working directory override" y "Repository that git cannot read" de "Worktrees visited", con `--dry-run`;
    - "Bare repository" y "Directory gone", con `--dry-run`;
    - "Double dash", "No command", "Empty command" y "Unknown flag before the command";
    - "Line shown by a dry run (macOS, Linux)" y "Preview";
    - "JSON requested";
  - que un `.wt.toml` inválido en el repo actual da exit 1 y no corre nada;
  - que `wt each --bogus echo x --json` reporta el error en texto y no como JSON;
  - que `wt -h` lista `each` y que `wt each -h` explica el `--` opcional y las dos formas de la línea.
- [x] 3.2 Implementar el bucle de D5 (sin la parte de Ctrl-C), las cabeceras de D6 y el resumen con `eachReport`, corriendo con la variable `runCommand` y `shell.ChildEnviron`. Verificar in-process, con comandos reales:
  - de `batch-execution`:
    - "Every worktree of the current repository" y "Every repository";
    - "Flags of the command" y "A flag of wt after the first word" (con `echo`);
    - "Interpreter operators in a single word (macOS, Linux)";
    - "Working directory of each command", desde un subdirectorio;
    - los tres escenarios de "Headers", con la salida sin color;
    - "One command fails", "Every command succeeds" y "Terminated by a signal (macOS, Linux)";
  - de `cli-contract`: "Global flag after the words of another command", "Failed command in a batch" y "Report of a batch";
  - que con `runCommand` reemplazado por uno que no arranca, el hint es `could not start` y el siguiente worktree igual corre;
  - que con color la cabecera, sin ANSI, es idéntica a la de sin color.
- [x] 3.3 Sumar a `wt each` el `signal.Notify` de D5, el chequeo no bloqueante antes de cada cabecera y al final, y el resumen de interrupción. Verificar con un test e2e en `cmd/git-wt` (`//go:build !windows`, como `interrupt_test.go`) que le manda SIGINT solo a `git-wt` mientras corre el primero de dos comandos:
  - los escenarios "Interrupt while a command runs (macOS, Linux)" de `batch-execution` e "Interrupted batch (macOS, Linux)" de `cli-contract`;
  - y que, cuando el primero de dos comandos falla (`sleep 1; exit 3`) y llega el SIGINT, el exit code sigue siendo 130 y stderr tiene `hint: ^ exited with 3`.
- [x] 3.4 Completar `wt each` según D2 (`ShellCompDirectiveDefault` para las palabras). Verificar in-process con `__complete`:
  - "Flags before the command" (`each --a` ofrece `--all-repos`);
  - "Flags after the first word" (`each git log --a` no ofrece flags de `wt`);
  - que para `each ` y `each git ` la directiva es la que hace que la shell complete archivos;
  - que nada se escribe en stderr.

## 4. Binario y shells reales

Los tests de fish y pwsh corren en macOS solo si están instalados. Si no, se cierran con el CI de `macos-latest` y `ubuntu-latest`.

- [x] 4.1 Sumar a `cmd/git-wt/testdata/fakeprog` un modo `printargs`, que imprime cada argumento en su propia línea, y construirlo en `TestMain`. Verificar con un test e2e "Words kept whole (macOS, Linux)" de `batch-execution`, salteado en Windows.
- [x] 4.2 Extender `cmd/git-wt/shell_test.go` en las cuatro shells, a través de la función:
  - `wt each -- git rev-parse --show-toplevel` imprime la ruta del principal y la de `feat`, con resultado 0 ("Through the function in PowerShell", y lo mismo en zsh, bash y fish);
  - "Shell stays where it is" y "Protocol variables not passed" de `batch-execution`;
  - "Double dash (zsh, bash, fish)", "Double dash (PowerShell)" y "Quoted double dash (PowerShell)" de `shell-integration`.

  Verificar que pasa en macOS para zsh y bash, y en el CI para fish y pwsh.

## 5. Windows

- [ ] 5.1 **[Windows]** Escribir los escenarios de Windows, salteados fuera de Windows:
  - "Interpreter operators in a single word (Windows)" y "Line shown by a dry run (Windows)", in-process;
  - "Words kept whole (Windows)" y "Environment variable reference (Windows)", e2e con `printargs`.

  Correr en `windows-latest` los tests de 1.x, 3.x, 4.1 y 4.2 (en pwsh). Verificar en el CI de Windows.
- [ ] 5.2 **[Windows]** Escribir un test e2e de Ctrl-C para Windows:
  - crea `git-wt` con `CREATE_NEW_PROCESS_GROUP`;
  - le manda `CTRL_BREAK_EVENT` con `GenerateConsoleCtrlEvent` mientras corre el primero de dos comandos (`fakeprog` esperando);
  - comprueba que el segundo no corre y que el exit code es 130.

  Si el runner no tiene consola, el test se saltea con `t.Skip`, que nombra la tarea 5.3. Verificar en el CI de Windows que pasa o que se saltea con ese motivo.
- [ ] 5.3 **[Windows]** Verificar a mano en Windows Terminal con PowerShell 7.4. Anotar el resultado en el PR:
  - "Ctrl-C in the console (Windows)": `wt each pause` en un repo con un worktree enlazado, Ctrl-C en el primero → el segundo no corre y `$LASTEXITCODE` es 130;
  - con un `.cmd` (`wt each npm --version` y Ctrl-C durante un `npm` lento), contestar N a `Terminate batch job (Y/N)?` → `wt each` no arranca el siguiente;
  - `wt each -- git status --short` y `wt each git commit --dry-run -m "a b"` a través de la función;
  - las cabeceras con color en la consola;
  - `wt each <TAB>` completa archivos.

## 6. Verificación en macOS, docs e integración

- [ ] 6.1 Verificar a mano en macOS, en Terminal.app, con `repos_root = ["~/Documents/GIT"]` y el binario de la rama instalado (`go install ./cmd/git-wt`). Anotar el resultado en el PR:
  - `wt each --all-repos git status --short` muestra una cabecera `repo:worktree` por cada fila de `wt list --all-repos`;
  - `wt each 'sleep 5'` con Ctrl-C en el primero termina con `wt: interrupted: …` y `echo $?` da 130;
  - `wt each git log --oneline -1` con color;
  - `wt each <TAB>` completa archivos y `wt each --<TAB>` ofrece `--all-repos`.
- [x] 6.2 Actualizar `docs/design/product.md` según D8:
  - §4: la fila de `wt each` pasa a `wt each [--all-repos] [--] <cmd>...`, se quita la de `wt exec` y se suma el 130 a la tabla de exit codes;
  - §7: la fila de `batch-execution` de la tabla "M2, desglosado en changes".

  Verificar que `grep -n 'wt exec' docs/design/product.md` ya no encuentra la fila del comando y que `openspec validate --all --strict` sigue en verde.
- [x] 6.3 Correr en macOS `gofmt -l .`, `go vet ./...`, `go test -race ./...`, `openspec validate --all --strict` y la cross-compilación de los 5 targets. Verificar que gofmt no imprime nada y que no hay fallos.
- [ ] 6.4 Integrar el change:
  - Sumar sus líneas bajo `## [Unreleased]` en `CHANGELOG.md`:
    - en *Added*: `wt each` con `--all-repos`, el `--` opcional, la línea de una o varias palabras, las cabeceras, el resumen con exit 1, lo que se saltea, `--dry-run` y el exit code 130 con Ctrl-C;
    - en *Changed*: la nota de que en PowerShell un `--` sin comillas no llega a `wt`.
  - Abrir el PR titulado `feat(batch-execution): add wt each [v0.2.0-alpha.3]`.

  Verificar el título con `git fetch origin && go run ./tools/relcheck pr --branch "$(git branch --show-current)" --title "<título>"`.
