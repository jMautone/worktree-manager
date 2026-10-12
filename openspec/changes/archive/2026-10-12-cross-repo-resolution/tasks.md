# Tasks

Las tareas **[Windows]** se cierran con el CI de `windows-latest`, salvo la verificación manual de 9.2. Los tests puros de Windows corren en cualquier OS (reciben `goos`). Los escenarios in-process propios de Windows se escriben en 9.3 y se saltean en macOS y Linux.

## 1. Decisiones puras (`internal/workspace`, `internal/worktree`)

- [x] 1.1 Implementar `SplitQualified(target)` y `MatchSide(repos, side, goos)` en `internal/workspace` (D1, D2). Verificar con tests de tabla:
  - `SplitQualified`: `api:feat`, `api:`, `:feat`, `:`, `api:a:b` y un target sin `:`.
  - `MatchSide` con `goos` darwin y windows: nombre exacto, sin mayúsculas y por prefijo (delegando en `Match`); sufijo `GIT/api` entre `~/Repos/api` y `~/Documents/GIT/api`; sufijo sin mayúsculas (`git/API`); `GIT/ap`, que no matchea porque no hay prefijo; `GIT\api`, que es sufijo en windows y nombre en darwin; separadores repetidos y en los bordes (`/GIT//api/`); sufijo ambiguo con los candidatos en el orden de `List`; lado vacío como no encontrado.
- [x] 1.2 Implementar `DisplayNames(repos, goos)` usando `MatchSide` para decidir (D3). Verificar con tests de tabla:
  - nombre único; homónimos bajo dos raíces (`Repos/api`, `GIT/api`); carpetas de organización (`work/api`, `oss/api`);
  - nombres que difieren solo en mayúsculas (`Api`, `api`); `/` como unión con `goos` windows; raíces anidadas sin sufijo único (`/x/api` queda con su ruta, `/y/x/api` con `y/x/api`);
  - y que cada nombre visible con `/`, pasado por `MatchSide`, devuelve su repo.
- [x] 1.3 Implementar `worktree.ResolveOthers(groups, target)` con `Hit` y `OthersAmbiguousError` (D1). Verificar con tests de tabla:
  - NAME en un grupo; rama en otro; NAME antes que rama entre grupos distintos;
  - principales y bare descartados, aunque su NAME o su rama coincidan; worktree con directorio borrado incluido;
  - el mismo NAME en dos grupos da ambigüedad, con los `Hit` en orden de grupo; target vacío como no encontrado.

## 2. El borde: listar varios repos

- [x] 2.1 Factorizar de `loadRepository` la función `a.listWorktrees(ctx, dir, cwdReal)` (git + `EvalSymlinks` + `Build`), sin cambiar comportamiento (D4). Verificar con los tests existentes de `list`, `cd`, `remove`, `lock` y `create` en verde.
- [x] 2.2 Implementar `a.scanWorktrees(ctx, repos, cwdReal)` en `internal/cli/scan.go`, con un semáforo de `GOMAXPROCS`, los resultados en el orden de `repos` y `Err` por repo (D4). Sumar a `internal/testutil` un helper que deja un repo ilegible para git (`HEAD` con basura). Verificar con tests contra repos reales, con `-race`:
  - con 12 repos el orden es estable entre corridas;
  - el repo ilegible queda con `Err` y los demás se leen;
  - solo el repo actual tiene un worktree con `Current`;
  - con `git` fuera del PATH, el error es `git.ErrNotInstalled`.

## 3. Resolución compartida (`navigate`, `cross-repo-resolution`)

- [x] 3.1 Sumar a `target.go` el mapeo de errores de D10, con los hints de forma calificada para la ambigüedad entre repos y entre otros repos, y las dos tablas de "nada encontrado" (por modo). Verificar con tests de tabla del texto de cada mensaje y hint, incluidos los nombres visibles de homónimos.
- [x] 3.2 Implementar `a.resolve(ctx, dir, target, mode)` con `modeNavigate`, el orden de D5 y la carga de config de D6, y reescribir `cdDestination` sobre él (el caso `-` queda aparte). Verificar in-process, con `WT_DIRECTIVE_CD_FILE` en un temporal:
  - los tests existentes de `wt cd` en verde;
  - de `navigate` "Resolving a name": "Worktree of another repository", "Repository before worktrees of other repositories", "Ambiguous repositories stop the search", "Qualified target", "Ambiguous prefix", "Two repositories with the same name", "No worktree or repository" y "No repository";
  - de `cross-repo-resolution`: todos los escenarios de "Worktrees of other repositories" salvo "Directory gone" (va en 4.1), de "Qualified targets", de "Repository side", de "Name side" y de "Repositories git cannot read", salvo los de Windows (9.3) y, en macOS y Linux, "Name side with a colon (macOS, Linux)" y "Backslash on macOS or Linux";
  - y que un `.wt.toml` inválido en el directorio actual da exit 1 en `wt cd <worktree local>`.
- [x] 3.3 Crear `wt path` en `internal/cli/path.go`, sobre `a.resolve(…, modeNavigate)`, sin el caso `-` y sin exigir la shell integration, con su `Long` y el registro en la raíz. Verificar in-process los escenarios de "Printing a path" (salvo "Native path on Windows", que va en 9.3) y de "JSON output and dry run of path", y que `wt -h` lista `path`.
- [x] 3.4 Actualizar el `Long` de `wt cd`:
  - la cascada con los worktrees de otros repos;
  - la forma `<repo>:<name>`, con `api:`, `api:^` y el sufijo para homónimos;
  - las comillas para `api:^` con `EXTENDED_GLOB` de zsh.

  Verificar con un test que comprueba que `wt cd -h` nombra `<repo>:<name>` y `wt list --all-repos`.

## 4. `remove`, `lock` y `unlock` (`remove-worktree`, `config-layers`)

- [x] 4.1 Reescribir `planRemove` y `changeLock` sobre `a.resolve(…, modeWorktree)`: git corre en `resolved.GitDir`, `CheckRemovable` recibe los worktrees del repo del target, y la config sale de la raíz de D6. Actualizar los `Long` de `remove`, `lock` y `unlock`. Verificar in-process:
  - los tests existentes de `remove`, `lock`, `unlock` y `prune` en verde;
  - todos los escenarios de "Target" de `remove-worktree`;
  - "Directory gone" de `cross-repo-resolution`;
  - de `config-layers`, "Merged into the other repository's base" e "Inspecting from another repository";
  - y que `wt remove --dry-run` sobre un worktree de otro repo no borra nada.

## 5. `wt create <repo>:<name>` (`create-worktree`, `config-layers`)

- [x] 5.1 Cambiar el principio de `planCreate` según D7 (validación del nombre y del repo antes de leer nada, `dir = repo.Path`, config del repo destino, hint fuera de un repo) y tomar `name` del lado del nombre en `wt.create.v1`. Verificar in-process:
  - los tests existentes de `create` en verde;
  - de `create-worktree`, "Repository without a name", "Name without a repository", todos los de "Repository" y "Created in another repository as JSON";
  - de `config-layers`, "File of the other repository" y "Other repository without a file";
  - y que `wt create api:feat --dry-run` desde fuera de un repo imprime `would fetch origin` y el `would create worktree` del repo `api`.

## 6. `wt list --all-repos` (`list-worktrees`)

- [x] 6.1 Sumar `--all-repos` a `wt list` según D8: repo opcional, raíces de la config sin capa de repo, `scanWorktrees`, la columna `REPO` con el nombre visible, el orden, `@` solo en el repo actual, el error sin raíces de `wt repos`, y los warnings de repos ilegibles. Sumar el hint de `wt list` fuera de un repo con raíces. Actualizar el `Long`. Verificar in-process los escenarios de "Listing every repository" y "Outside a repository with roots configured", la alineación con color, y que `--dry-run` da la misma salida.
- [x] 6.2 Implementar el JSON de `--all-repos` (`listAllJSON`, `worktreeAllJSON`, `unreadableJSON`) según D8. Verificar in-process los escenarios de "JSON output of every repository", y que `wt list --json` produce exactamente el documento de hoy.

## 7. Completions (`shell-integration`, `remove-worktree`, `create-worktree`)

- [x] 7.1 Generalizar `completeCdTargets` en `a.completeTargets(mode, filter)` según D9 (los tres grupos, nombres únicos, descripciones con `cobra.CompletionWithDesc`, la palabra con `:`) y usarla en `wt cd` y `wt path`. Verificar in-process con `__complete` todos los escenarios de "Completions" de `shell-integration` (nada en stderr, y en stdout solo los candidatos y la directiva de cobra).
- [x] 7.2 Usar `completeTargets(modeWorktree, …)` en `remove`, `lock` y `unlock`, con sus filtros. Verificar in-process con `__complete` los escenarios de "Completions" de `remove-worktree`.
- [x] 7.3 Completar `<name>` de `wt create` con `<nombre visible>:` y `ShellCompDirectiveNoSpace`, y que `completeBases` liste las refs del repo destino cuando `args[0]` es calificado (D9). Verificar in-process con `__complete` los escenarios de "Completions" de `create-worktree`, incluido el directive `NoSpace`.

## 8. Binario y shells reales

Los tests de fish y pwsh corren en macOS solo si están instalados; si no, se cierran con el CI de `macos-latest` y `ubuntu-latest`.

- [x] 8.1 Extender `cmd/git-wt/shell_test.go`:
  - a través de la función, con `WT_REPOS_ROOT` apuntando a una raíz del sandbox y desde un directorio fuera de cualquier repo, `wt cd <worktree de otro repo>` y `wt cd api:feat` terminan en ese worktree con resultado 0;
  - en `TestShellCompletions`, `wt cd api:` ofrece `api:feat` en bash (con `:` en `COMP_WORDBREAKS`), fish y pwsh.

  Verificar que pasa en macOS para zsh y bash, y en el CI para fish y pwsh.

## 9. Verificación manual, Windows, docs e integración

- [ ] 9.1 Verificar a mano en macOS, con `repos_root = ["~/Documents/GIT"]` en `~/.config/wt/config.toml` y el binario de la rama instalado. Anotar el resultado en el PR:
  - `wt list --all-repos` muestra los worktrees de `cortex` y `dym-knowledge-base` con su `REPO`;
  - desde `/tmp`, en Terminal.app, `wt cd angry<TAB>` completa el worktree de `cortex`, y Enter te deja ahí;
  - `wt cd cortex:<TAB>` ofrece sus tres worktrees;
  - `wt path dym-knowledge-base:^` imprime la ruta del repo;
  - `wt create worktree-manager:scratch --no-cd` y después `wt remove scratch`, los dos desde `/tmp`, crean y borran el worktree y su rama.
- [ ] 9.2 **[Windows]** Verificar a mano en Windows Terminal con PowerShell 7.4, con los repos del trabajo en `repos_root`. Anotar los números y el resultado en el PR:
  - medir `Measure-Command { wt list --all-repos }` y la latencia percibida de `wt cd <TAB>`;
  - `wt cd <worktree de otro repo>` desde `C:\` deja la shell ahí;
  - `wt cd <repo>:<TAB>` ofrece sus worktrees.

  Si `wt list --all-repos` tarda más de 300 ms, abrir un issue para el plan B de D4.
- [x] 9.3 **[Windows]** Escribir los escenarios in-process de Windows, salteados fuera de Windows: "Backslash on Windows" y "Forward slashes on Windows" de `cross-repo-resolution`, y "Native path on Windows" de `navigate` (`wt path`). Correr en `windows-latest` los tests de 2.2, 3.2, 4.1, 5.1, 6.1 y 8.1 (en pwsh). Verificar en el CI de Windows.
- [x] 9.4 Actualizar en `docs/design/product.md` §7 la fila de `cross-repo-resolution` de la tabla "M2, desglosado en changes", sumando `wt create <repo>:<name>`. Verificar que la fila nombra `create` y que `openspec validate --all --strict` sigue en verde.
- [x] 9.5 Correr en macOS `gofmt -l .`, `go vet ./...`, `go test -race ./...`, `openspec validate --all --strict` y la cross-compilación de los 5 targets. Verificar sin salida de gofmt ni fallos.
- [x] 9.6 Integrar el change:
  - Sumar sus líneas bajo `## [Unreleased]` en `CHANGELOG.md`: la forma `<repo>:<name>` con el sufijo para homónimos; `wt cd` hacia worktrees de otros repos; `wt path` con `wt.path.v1`; `remove`, `lock`, `unlock` y `create` en otros repos; `wt list --all-repos` con `repo`, `repo_path` y `unreadable`; las completions del workspace con descripciones; y en *Changed*, los mensajes de exit 3 y los hints de exit 4 que cambian, y la capa de repo de la config cuando se opera sobre otro repo.
  - Abrir el PR titulado `feat(cross-repo-resolution): resolve worktrees across repositories [v0.2.0-alpha.2]`.

  Verificar el título con `git fetch origin && go run ./tools/relcheck pr --branch "$(git branch --show-current)" --title "<título>"`.
