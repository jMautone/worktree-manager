# Tasks

## 1. Base y herramientas de test

- [x] 1.1 Agregar `cobra`, `go-toml/v2`, `x/term` y `x/sys` a `go.mod`; verificar con `go mod tidy && go build ./...` sin cambios pendientes en `go.sum`
- [x] 1.2 Crear `internal/testutil` con el helper que crea repos git reales en `t.TempDir()` con entorno aislado (`GIT_CONFIG_NOSYSTEM`, `GIT_CONFIG_GLOBAL`, `HOME`/`USERPROFILE`/`XDG_CONFIG_HOME`/`APPDATA` dentro del temp) y helpers para worktrees linked, detached, locked, prunable y bare; verificar con un test que crea cada tipo y lo ve en `git worktree list`
- [x] 1.3 Agregar a `testutil` una función que devuelve rutas comparables (`EvalSymlinks` + `FromSlash`); verificar con un test que en macOS compara `/var/...` contra `/private/var/...` y da igual

## 2. Contrato de CLI (`cli-contract`)

- [x] 2.1 Definir en `internal/cli` los exit codes 0-8 como constantes y el tipo `Error{Code, Msg, Hints}`; verificar con un test de tabla que cada constante tiene el valor del contrato
- [x] 2.2 Implementar `cli.Run(ctx, Env) int` con el comando raíz de cobra, `SilenceErrors`/`SilenceUsage`, completions por defecto deshabilitadas y flags globales persistentes (`--json`, `--dry-run`, `-C`, `--no-color`); verificar que `wt`, `wt help`, `wt -h` salen con 0 y listan los comandos
- [x] 2.3 Convertir errores de parseo a exit 2 (comando desconocido con sugerencia, flag desconocido, flag sin valor, argumentos de más) y reemplazar `help` para que un tema desconocido salga con 2; verificar con los escenarios de "Strict validation" y que stdout queda vacío
- [x] 2.4 Implementar la salida de errores: texto (`wt: msg` + `hint:`) y JSON (`wt.error.v1` en stderr, stdout vacío), warnings `wt: warning:` suprimidos con `--json`; verificar con tests de ambos formatos
- [x] 2.5 Implementar `-C <dir>`: relativo al cwd real, exit 2 si no existe o no es directorio; verificar con los escenarios "Listing another repository" y "Missing directory"
- [x] 2.6 Crear `internal/term`: detección de TTY con `x/term`, regla de color (TTY, `NO_COLOR`, `--no-color`) y `term_windows.go` que habilita VT o apaga el color; verificar la regla con un test de tabla sobre las tres entradas
- [x] 2.7 Implementar `wt version` y `--version` (texto y `wt.version.v1`) resueltos antes de cargar config, con `commit` desde `debug.ReadBuildInfo`; verificar que funcionan con un archivo de config con TOML inválido
- [x] 2.8 Reemplazar `cmd/git-wt/main.go` para que solo construya el `Env` real y haga `os.Exit(cli.Run(...))`; verificar con `go run ./cmd/git-wt version`

## 3. Config por capas (`config-layers`)

- [x] 3.1 Implementar el registro de claves (`default_base`, `worktree_path`) con tipo, default, `InRepo` y validación; verificar con tests de validación de tipos y de `worktree_path` vacío
- [x] 3.2 Implementar `config.Paths(goos, getenv, home)` pura para macOS/Linux (XDG, `~/.config`) y Windows (`%APPDATA%`), con `WT_CONFIG` como override; verificar con un test de tabla que incluye `GOOS=windows` corrido desde macOS
- [x] 3.3 Implementar `config.Resolve(registry, layers)` pura: precedencia default < user < repo < env, `WT_<KEY>` vacío como no seteado, clave desconocida → warning, clave no permitida en repo → warning, tipo inválido → error con clave y origen; verificar con tests que inyectan un registro con una clave no permitida en el repo
- [x] 3.4 Implementar la carga desde disco (archivo ausente no es error, TOML inválido → exit 1 con archivo y línea); verificar con el escenario "Malformed TOML" y su número de línea
- [x] 3.5 Implementar `wt config path`, `wt config list` y `wt config get <key>` en texto y JSON (`wt.config.{path,list,get}.v1`); verificar con los escenarios "Listing sources", "Unknown key requested" y "Paths outside a repository"
- [x] 3.6 Test de integración de precedencia de extremo a extremo (archivo de usuario + `.wt.toml` + `WT_DEFAULT_BASE`) con `cli.Run`; verificar los tres escenarios de "Layer precedence"

## 4. Worktrees de git (`git-worktrees`)

- [x] 4.1 Implementar `git.ParseWorktreeList` sobre la salida `--porcelain -z`: branch corta, detached, bare, locked/prunable con y sin razón; verificar con fixtures tomadas de salida real, incluida una con rutas estilo Windows (`C:/...`)
- [x] 4.2 Implementar `git.Runner` con `exec.CommandContext`: git ausente → exit 1 "git not found"; "not a git repository" → exit 3 con el directorio; cualquier otro fallo → exit 1 con el stderr de git; verificar los tres casos (git ausente con un `PATH` vacío)
- [x] 4.3 Crear `internal/worktree` con el modelo y `Build(entries, cwdReal, goos)` pura: rutas nativas, main = primera, current por prefijo de componentes con la coincidencia más profunda, `EqualFold` en darwin/windows; verificar con tests de tabla para anidados, `/a/repo` vs `/a/repo2`, mayúsculas por OS y bare como current
- [x] 4.4 Test de integración contra repo real: desde un worktree linked, desde un subdirectorio, desde el bare y fuera de todo repo; verificar current, main y exit 3

## 5. `wt list` (`list-worktrees`)

- [x] 5.1 Implementar el renderer de tabla propio (anchos sobre texto plano, color aplicado después del relleno); verificar que al quitar ANSI todas las columnas arrancan en el mismo offset
- [x] 5.2 Implementar `wt list`: columnas y marcas `@`/`^`, `(detached)`/`(bare)`, `STATE`, orden (main primero, luego `NAME` sin distinguir mayúsculas y desempate por ruta), sin argumentos posicionales; verificar con los escenarios de "Table layout" y "Order"
- [x] 5.3 Implementar `wt list --json` (`wt.list.v1`) con todos los campos y `null` donde corresponde; verificar con los escenarios "Detached worktree in JSON" y "Lock reason in JSON"
- [x] 5.4 Verificar que `wt list --dry-run` produce salida idéntica a `wt list`, y que `wt --json list` es idéntico a `wt list --json`

## 6. E2E y CI

- [x] 6.1 Tests E2E que compilan el binario en `TestMain` y verifican exit codes reales del proceso (`version` → 0, `lsit` → 2, `list` fuera de repo → 3) y `git wt version` con `git-wt` en el PATH; verificar que pasan en macOS
- [x] 6.2 Documentar en el help de la raíz que `git wt --help` lo intercepta git y que se use `git wt -h`; verificar leyendo `wt --help`
- [x] 6.3 Correr `gofmt -l`, `go vet ./...` y `go test -race ./...` en macOS y cross-compilar los 5 targets; verificar sin salida de gofmt ni fallos
- [ ] 6.4 **[Windows]** Verificar en el CI de `windows-latest` que pasan los tests de rutas nativas, current worktree con mayúsculas distintas y nombres cortos 8.3 del temp; si falla por 8.3, agregar la normalización con `GetLongPathName` descrita en el design
