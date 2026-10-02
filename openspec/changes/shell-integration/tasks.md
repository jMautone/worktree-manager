# Tasks

## 1. Protocolo y resolución (decisión pura)

- [x] 1.1 Crear en `internal/shell` `Names`, `Active(getenv)` y `ChildEnviron(environ, goos)`; verificar con tests de tabla: activa solo con `WT_DIRECTIVE_CD_FILE` no vacía, y `ChildEnviron` quita `WT_DIRECTIVE_CD_FILE` y `WT_PREVIOUS_DIR` sin tocar el resto, sin distinguir mayúsculas con `goos = "windows"` (corrido desde macOS)
- [x] 1.2 Implementar `shell.WriteDirective(path, dest)` sin `O_CREATE`; verificar con tests que reemplaza el contenido por la ruta exacta sin salto de línea final, y que con un archivo inexistente devuelve error y no lo crea
- [x] 1.3 Agregar a `internal/config` un test que falla si alguna clave del registro produce `WT_DIRECTIVE_CD_FILE` o `WT_PREVIOUS_DIR` como variable de entorno; verificar que pasa con el registro actual y que falla al inyectar una clave `previous_dir`
- [x] 1.4 Implementar `worktree.Resolve(ws, target)` con `^`, `@`, nombre exacto, rama exacta, `NotFoundError`, `AmbiguousError` y `ErrNoCurrent`; verificar con tests de tabla: nombre antes que rama, dos worktrees con el mismo `NAME`, rama con barra, mayúsculas distintas con `goos` darwin, windows y linux, bare como `^`, y `@` sin worktree actual

## 2. `wt cd` (`navigate`)

- [x] 2.1 Crear `wt cd <target>` en `internal/cli/cd.go` con la validación de argumentos (falta o sobra → exit 2) y el chequeo de integración activa después de `-C` y antes de resolver (exit 1, `shell integration is not active` + hint); verificar con los escenarios "No target", "Two targets", "Run without the function" y "Unknown target without the function"
- [x] 2.2 Conectar nombres, `^` y `@` a `worktree.Resolve`, con carga de config, mapeo a exit 3/4 con sus hints (uno por candidato), chequeo de que el destino existe y escritura de la directiva; verificar in-process, con `WT_DIRECTIVE_CD_FILE` apuntando a un temporal del sandbox, los escenarios de "Resolving a name", "Main and current worktree", "Destination must exist" (prunable: exit 3 y directiva vacía) y "Missing directive file"
- [x] 2.3 Implementar `wt cd -` desde `WT_PREVIOUS_DIR`, sin repo y con config sin capa de repo; verificar: funciona fuera de un repo, variable vacía o ausente → exit 3 `no previous directory`, directorio borrado → exit 3, y `-C` no cambia el destino de `-` pero sí el de `^` ("Main worktree of another repository")
- [x] 2.4 Implementar `--json` (`wt.cd.v1`) y `--dry-run` (sin directiva, `would change directory to <path>`, mismos errores), y documentar en el help de `wt cd` que con `EXTENDED_GLOB` de zsh se escribe `wt cd '^'`; verificar con los escenarios de "JSON output" y "Preview a jump", que `--dry-run --json` imprime el mismo documento, y leyendo `wt cd -h`

## 3. `wt shell init` y completions (`shell-integration`)

- [x] 3.1 Escribir `internal/shell/scripts/wt.{zsh,bash,fish,ps1}` según D2 (`command`/`builtin`, prefijos de entorno, memoria de `-` no exportada, `always` en zsh, `try/finally` y `$PSNativeCommandArgumentPassing = 'Standard'` en pwsh) e implementar `shell.Script(name, completion)` con las guardas de D5; verificar con tests de `Script` por shell (contiene la función y la guarda, nombre desconocido → error) y con `bash -n` y `zsh -n` sobre la salida cuando la shell está disponible
- [x] 3.2 Crear el grupo `wt shell` y `wt shell init <shell>` sin leer config ni git, con `--json` (`wt.shell.init.v1`) y el help con la tabla de líneas de instalación; verificar con los escenarios "Script for zsh", "Broken configuration", "Unsupported shell", "Missing shell", "Help shows the lines", "Script as JSON" y "bash on Windows" (`GOOS=windows` in-process), y que `--dry-run` no cambia la salida
- [x] 3.3 Agregar los `ValidArgsFunction` de `wt cd` (`NAME`s no prunable, sin cargar config, vacío ante cualquier error) y de `wt shell init`, y generar el texto de completion de cobra para cada shell dentro de `Script`; verificar in-process con `__complete`: nombres de worktree, `wt sh` → `shell`, los cuatro shells, y fuera de un repo ningún nombre y nada en stdout salvo la directiva de cobra

## 4. Pruebas por shell real

Los tests de fish y pwsh corren en macOS solo si están instalados (`brew install fish powershell`); si no, esas partes se cierran con el CI de `macos-latest` y `ubuntu-latest`. Las tareas **[Windows]** se cierran con el CI de `windows-latest`.

- [x] 4.1 Armar la infraestructura de D8 y D9: `cmd/git-wt/testdata/fakeprog` compilado en `TestMain` como `git` (registra su entorno) y como `wt`/`wt.exe` (imprime una marca), helper `runShell` con `Skip` salvo que la shell esté en `WT_TEST_SHELLS`, temporal del sistema dentro del sandbox, y en `ci.yml` la instalación de fish (y zsh en Linux) más `WT_TEST_SHELLS` por OS; verificar con el escenario "Silent load" en cada shell disponible y el CI verde en los 3 OS sin shells salteadas
- [x] 4.2 Probar la función en zsh, bash, fish y pwsh: "The binary moves the shell" (`wt cd feat`), "Commands that do not move the shell", "Output is not captured", "Argument with spaces", "Exit code" (`$?`/`$status`/`$LASTEXITCODE`), "No file left behind", "Variable not left set", "Loaded twice", y en zsh y bash que alias de `rm`, `cd` y `mktemp` definidos antes de cargar el script no interfieren; verificar que pasan en macOS para zsh y bash, y en el CI para fish y pwsh
- [x] 4.3 **[Windows]** Correr los tests de 4.2 en pwsh sobre `windows-latest`, incluida la directiva con ruta `C:\...` ("Directive contents (Windows)"); verificar en el CI de Windows
- [x] 4.4 Probar "Previous directory" por shell: ida y vuelta con `wt cd -`, `-` hacia un directorio fuera de un repo, y una shell hija que carga el script sin anterior ("New session"); verificar que pasan en las cuatro shells (fish y pwsh en el CI)
- [x] 4.5 Probar "Processes started by wt" con el `git` falso primero en el `PATH`: `wt list` a través de la función, y el entorno registrado no tiene ninguna de las dos variables; verificar en zsh en macOS y en pwsh en el CI de los 3 OS
- [x] 4.6 Probar "Shadowing other programs named wt" con el `wt` falso en el `PATH`: `wt version` a través de la función imprime la versión de `git-wt`; verificar en las cuatro shells en macOS y Linux, y **[Windows]** con `wt.exe` en pwsh en el CI de Windows
- [x] 4.7 Probar "Completions" por shell según D8: bash con `bash-completion` cargado (`COMP_WORDS` + `__start_wt`), fish con `complete -C`, pwsh con `TabExpansion2` y zsh con el registro en `_comps` después de `compinit`, más "zsh without compinit" (carga silenciosa y `wt cd feat` funciona); verificar que pasan en el CI de macOS y Linux, y pwsh también en Windows

## 5. Verificación manual, docs e integración

- [x] 5.1 Verificar a mano en macOS, en una zsh interactiva con el script en `~/.zshrc`: `wt cd <TAB>` ofrece los nombres, `wt list` mantiene el color a través de la función, `wt cd -` vuelve, y anotar el resultado en el PR
- [ ] 5.2 **[Windows]** Verificar a mano en Windows Terminal con PowerShell 7.4: `wt list` con color a través de la función (el binario ve una TTY), `wt cd` mueve la shell, TAB completa nombres, y `wt.exe` sigue abriendo Windows Terminal; si la salida pierde la TTY, aplicar el plan B de Riesgos y repetir; anotar el resultado en el PR
- [x] 5.3 Documentar la instalación por shell en `README.md` (sección "Installing") y actualizar la fila de `shell-integration` en `docs/design/product.md` §7 (`wt cd <name>|^|@|-`, completions); verificar que las líneas del README coinciden con las de `wt shell init --help`
- [x] 5.4 Correr en macOS `gofmt -l .`, `go vet ./...`, `go test -race ./...`, `openspec validate --all --strict` y la cross-compilación de los 5 targets; verificar sin salida de gofmt ni fallos
- [x] 5.5 Sumar las líneas del change bajo `## [Unreleased]` en `CHANGELOG.md` (`wt cd`, `wt shell init`, completions, variables del protocolo) y abrir el PR titulado `feat(shell-integration): add wt cd, wt shell init and completions [v0.1.0-alpha.3]`; verificar el título con `git fetch origin && go run ./tools/relcheck pr --branch "$(git branch --show-current)" --title "<título>"`
