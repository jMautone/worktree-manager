# Tasks

Las tareas **[Windows]** se cierran con el CI de `windows-latest` o a mano en Windows, nunca solo desde macOS. Los tests puros reciben `goos` y corren en cualquier OS. Los tests in-process corren contra repos git reales en directorios temporales, con `WT_CONFIG` en un directorio temporal, así `approvals.toml` nunca toca el del usuario.

## 1. Aprobaciones (puro y archivo)

- [ ] 1.1 Crear `internal/approval` según D2, con su `doc.go`: `Approval`, `Set`, `Hash`, `Approved`, `Add`, `Parse` y `Marshal`. Verificar con tests de tabla:
  - `Hash("npm ci")` es el SHA-256 en hex minúsculas (vector fijo en el test);
  - `Approved` es verdadero solo con el mismo `repo`, `event` y hash, y falso si cambia un carácter del comando, el evento o el repo;
  - `Add` devuelve `false` si ya estaba y no duplica;
  - `Parse` de lo que da `Marshal` devuelve el mismo `Set`;
  - `Parse` falla nombrando el archivo con TOML inválido, con un `[[approval]]` sin `repo`, `event` o `sha256`, y con un `sha256` que no es hex de 64 en minúsculas; un campo desconocido se ignora;
  - `Approved` ignora `command`: un archivo con `command` editado sigue aprobando.
- [ ] 1.2 Implementar `approval.Load(path)` y `approval.Save(path, Set)` según D3 (archivo inexistente = vacío; escritura con temporal en el mismo directorio y rename). Verificar con tests sobre `t.TempDir()`:
  - `Load` de un archivo inexistente da un `Set` vacío y sin error;
  - `Save` crea los directorios que faltan y `Load` lee lo guardado;
  - `Save` sobre un archivo existente lo reemplaza y no deja temporales en el directorio.
- [ ] 1.3 Implementar `config.ApprovalsFile(goos, getenv, home)` según D3. Verificar con el test de tabla de `UserFile` extendido: darwin y linux con y sin `XDG_CONFIG_HOME`, windows con `APPDATA`, y `WT_CONFIG=/x/wt.toml` → `/x/approvals.toml` en los tres.

## 2. Config: `[hooks]` del repo aparte

- [ ] 2.1 Sumar `Key.RepoApart`, `Config.RepoHooks` y `Config.RepoFile` según D1, y marcar las seis `hooks.*`. Verificar con tests de `config`:
  - un `[hooks]` en la capa `repo` no da warning, no cambia `cfg.Values` (source sigue `default` o `user`) y queda en `RepoHooks`;
  - un `post-create = "npm ci"` o `["  "]` o `["echo {base}"]` en `pre-remove` de la capa `repo` falla nombrando la clave y `.wt.toml`;
  - las otras claves con `InRepo: false` siguen dando el warning.
- [ ] 2.2 Ajustar los tests in-process de `config-layers` que esperaban el warning por `[hooks]` en `.wt.toml`, y sumar los escenarios "Repository sets hooks" e "Invalid repository hook" de la delta de `config-layers`. Verificar con `go test ./internal/cli -run Config`.

## 3. Hooks del repo en `create`, `cd` y `remove`

- [ ] 3.1 Extender el `hookPlanner` según D4: lista usuario → repo, `configured` que mira `RepoHooks`, carga perezosa de `approvals.toml`, y el `*Error` con exit 8, mensaje y hints. La función que elige el primer comando sin aprobar es pura, en `internal/hook`, con test de tabla (orden por evento y dentro del evento; ninguno sin aprobar; sin comandos del repo no consulta nada).
- [ ] 3.2 Pasarle al planner `repoPath`, `repoFile` y `crossRepo` desde `create`, `cd` y `remove`. Verificar con tests in-process los escenarios:
  - de `hooks`: "User commands before the repository's", "A failing user command stops the repository's", "Hooks of a repository are ignored" y "Preview with repository hooks";
  - de `project-hooks-approval`: "Hooks read from the repository", "Hooks of the other repository", "Approved in one repository only", "Approved for one event only", "Command changed after approval", "Approval valid in every worktree", "post-create not approved", "Background event not approved", "Not approved with JSON", "Hints in the current repository", "Hint for another repository", "Other events are not checked", "Skipping hooks skips the check", "Dry run not approved", "Revoked by hand" e "Invalid repository hook".
- [ ] 3.3 Test de costo cero: con un `.wt.toml` sin `[hooks]`, `wt create` no lee `approvals.toml` (un `approvals.toml` inválido no rompe nada). Verificar con un test in-process.

## 4. `wt hook list` y `wt hook approve`

- [ ] 4.1 Implementar `hook.Entries` según D5 (puro). Verificar con test de tabla: orden de `hook.Events`, usuario antes que repo, `Approved` verdadero para los del usuario.
- [ ] 4.2 Implementar `wt hook list` en `internal/cli/hookcmd.go` (texto con `table.go` y `wt.hook.list.v1`). Verificar con tests in-process los escenarios "User and repository hooks", "No hooks", "Outside a repository", "JSON" (de "Hook list as JSON") y "Broken file".
- [ ] 4.3 Implementar `wt hook approve [<event>...]` con `--dry-run` y `wt.hook.approve.v1`, y la completion de eventos. Verificar con tests in-process los escenarios "Approve then create", "Only the events named", "Nothing to approve", "Unknown event", "Preview", "JSON" (de "Approving hooks in a dry run or as JSON"), "Entry written", "Next to WT_CONFIG" y "Location (macOS, Linux)".
- [ ] 4.4 **[Windows]** Escenario "Location (Windows)": test puro de `ApprovalsFile` con `goos` `windows` (cerrable desde macOS) y, en el CI de `windows-latest`, un test in-process que aprueba con `APPDATA` en un temporal y `WT_CONFIG` vacío y encuentra `<APPDATA>\wt\approvals.toml`, y que `wt create` con ese hook aprobado corre bajo `cmd.exe`.
- [ ] 4.5 Help de `wt hook`, `wt hook list` y `wt hook approve` (qué se aprueba, por qué está atado al repo y al evento, y que un comando aprobado igual corre el código que lea del repo), y actualizar el help de `wt config` según D7. Verificar con el test de help existente (todos los comandos tienen `Short` y `Long`) y un test que busca `wt hook approve` en `wt config --help`.

## 5. `wt config path`

- [ ] 5.1 Sumar la línea `approvals:` y el campo `approvals` de `wt.config.path.v1` según D6. Verificar con tests in-process: el escenario "Path of the approvals file", y que el JSON tiene `approvals.path` y `approvals.exists`.

## 6. Integración

- [ ] 6.1 Verificar a mano en macOS, con el binario de la rama (`go install ./cmd/git-wt`), en un repo con este `.wt.toml`, y anotar el resultado en el PR:

  ```toml
  [hooks]
  post-create = ['cp "{repo_path}/.env" .env']
  ```

  - `wt create x` falla con exit 8 y el hint da `wt hook approve`;
  - `wt hook list` lo muestra como `no`, `wt hook approve` lo aprueba y `wt create x` copia el `.env`;
  - cambiar el comando en `.wt.toml` vuelve a dar exit 8.
- [ ] 6.2 Actualizar `docs/design/product.md` según D7: §4 (fila de `wt hook list` / `wt hook approve`), §5 (Hooks: clave de la aprobación, `approvals.toml`, orden usuario → repo) y §7 si la fila de `project-hooks-approval` cambia. Verificar que `grep -n "config del usuario" docs/design/product.md` ya no dice que las aprobaciones se guardan en la config, y que `openspec validate --all --strict` sigue en verde.
- [ ] 6.3 Correr en macOS `gofmt -l .`, `go vet ./...`, `go test -race ./...`, `openspec validate --all --strict` y la cross-compilación de los 5 targets de `.goreleaser.yaml`. Verificar que gofmt no imprime nada y que no hay fallos.
- [ ] 6.4 Integrar el change:
  - Sumar sus líneas bajo `## [Unreleased]` en `CHANGELOG.md`:
    - en *Added*: hooks del repo en `.wt.toml` con aprobación por repo, evento y hash; `wt hook list` y `wt hook approve`; el exit 8; `approvals` en `wt config path`;
    - en *Changed*: `[hooks]` en `.wt.toml` ya no se ignora: sin aprobar, `create`, `cd` y `remove` fallan con exit 8.
  - Abrir el PR titulado `feat(project-hooks-approval): approve repository hooks by hash [v0.3.0-alpha.2]`.

  Verificar el título con `git fetch origin && go run ./tools/relcheck pr --branch "$(git branch --show-current)" --title "<título>"`.

## Workflow follow-up

- Archivar con `/opsx:archive project-hooks-approval` en el mismo PR, sincronizando las specs, con todas las tareas tildadas y el CI en verde.
- Correr `openspec validate --all --strict` después del archive y commitear en la misma rama.
