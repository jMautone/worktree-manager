# Tasks

Las tareas **[Windows]** se cierran con el CI de `windows-latest` o a mano en Windows, nunca solo desde macOS. Los tests puros corren en cualquier OS. Los tests in-process corren contra repos git reales en directorios temporales, con un "remote" que es otro repo local (`git clone` de un directorio), así el fetch no sale a la red.

## 1. Decisión pura (`internal/worktree`)

- [ ] 1.1 Implementar `SyncState`, `DecideSync`, `SyncOutcome`, `SyncExit`, `SyncTargets` y `SyncLine` en `internal/worktree/sync.go` según D1. Verificar con tests de tabla:
  - `DecideSync` respeta el orden de la spec: un worktree sin directorio y sucio da `<path> does not exist`; detached y sucio da `HEAD is detached`; un rebase en curso y sucio da `a rebase is in progress`;
  - `Contains` sin nada que lo frene da `SyncUpToDate`, y si no, `SyncRebase`;
  - `SyncExit`: todos `Rebased`/`UpToDate` → 0; con un `Skipped` → 5; `Skipped` y `Conflict` → 6; cualquier `Failed` → 1;
  - `SyncTargets` con `all` saca el principal, el bare y los detached, y mantiene el orden;
  - `SyncLine` da las líneas exactas de la spec, con y sin `dryRun`.

## 2. Lecturas y rebase (`internal/git`)

- [ ] 2.1 Implementar `git.TrackedChanges` e `git.InProgress` según D3. Verificar con tests sobre repos reales:
  - un archivo versionado modificado o staged da verdadero, y uno sin versionar no;
  - `InProgress` da `rebase` en un worktree vinculado con un rebase parado en un conflicto, `merge` con un merge parado, y `""` en un worktree limpio, mirando el git dir del worktree y no el del principal.
- [ ] 2.2 Implementar `git.Rebase`, `git.RebaseAbort` y `git.Unmerged` según D4. Verificar con tests sobre repos reales:
  - un rebase sin conflicto deja la base como ancestro y conserva el commit propio;
  - con conflicto, `Unmerged` da verdadero y, después de `RebaseAbort`, HEAD y los archivos son los de antes;
  - con `rebase.autoStash = true` en la config del repo y un archivo modificado, `Rebase` falla sin crear un stash (`git stash list` vacío).

## 3. `wt sync` (`sync-worktrees`)

- [ ] 3.1 Extraer `a.repoBase` de `readMerged` según D2, sin cambiar `wt remove`. Verificar con los tests existentes de `wt remove` y de "Merged branch" en verde.
- [ ] 3.2 Crear `internal/cli/sync.go` con el comando, sus flags, las validaciones de uso y el help (D7), y registrarlo en `run.go`. Verificar con tests in-process: "All and a name", "Hooks flag", un `--base ''` con exit 2, y el test de help existente (`Short` y `Long`).
- [ ] 3.3 Implementar las fases 1 y 2 de D2 (targets, base, fetch y commit por repo) y la fase 3 sin Ctrl-C (D4, D6). Verificar con tests in-process los escenarios de `sync-worktrees`:
  - "Current worktree", "Main worktree by name", "Name not found", "Linked worktrees only", "No linked worktree", "All outside a repository", "Worktree of another repository";
  - "Base from the repository configuration", "Explicit base", "One fetch for several worktrees" (contando los fetch con un `git` envuelto o con `GIT_TRACE` en un log), "Remote cannot be reached", "Local base", "Base not found";
  - "Modified file", "Untracked file", "Rebase in progress", "Fresh worktree", "Behind the base", "Autostash configured";
  - "Conflict then the next worktree", "Untracked file in the way", "Skipped and conflict", "Conflict wins over skipped", "Only skipped".
- [ ] 3.4 Implementar `--dry-run` y `--json` según D6. Verificar con tests in-process "Preview", "Synced as JSON" y "Conflict as JSON", y que `--dry-run` no mueve ninguna rama ni fetchea (la ref remota sigue en su commit viejo).
- [ ] 3.5 Sumar el Ctrl-C de D5. Verificar con un test e2e en `cmd/git-wt` (`//go:build !windows`, como el de `wt each`) que le manda SIGINT solo a `git-wt` mientras rebasea el primero de dos worktrees, con un `git` envuelto que tarda en el rebase: escenario "Interrupt during a rebase (macOS, Linux)".
- [ ] 3.6 Implementar las completions de D7. Verificar in-process con `__complete` el escenario "Names" y que `--base` ofrece las ramas como en `wt create`.

## 4. Windows

- [ ] 4.1 **[Windows]** Correr en el CI de `windows-latest` los tests de 2.x y 3.3, que usan rutas nativas y `cmd.exe` solo a través de git. Verificar que pasan sin tests salteados en Windows salvo el e2e de SIGINT.
- [ ] 4.2 **[Windows]** Verificar a mano en Windows Terminal con PowerShell 7.4 el escenario "Ctrl-C in the console (Windows)", con un repo grande o un hook de git `post-rewrite` que tarde, y anotar el resultado en el PR.

## 5. Docs e integración

- [ ] 5.1 Actualizar `docs/design/product.md`: §4 (la fila de `wt sync` con `--base` y `--no-fetch`) y §7 si la fila de `sync-worktrees` cambia; y la lista de comandos de `README.md`. Verificar que `grep -n "wt sync" docs/design/product.md README.md` muestra los flags nuevos.
- [ ] 5.2 Verificar a mano en macOS, con el binario de la rama (`go install ./cmd/git-wt`), en un repo con tres worktrees (uno atrás de `origin/main`, uno sucio y uno que conflictúa), que `wt sync --all` da las tres líneas, el resumen y exit 6, y que el que conflictúa queda como estaba. Anotar el resultado en el PR.
- [ ] 5.3 Correr en macOS `gofmt -l .`, `go vet ./...`, `go test -race ./...`, `openspec validate --all --strict` y la cross-compilación de los 5 targets de `.goreleaser.yaml`. Verificar que gofmt no imprime nada y que no hay fallos nuevos.
- [ ] 5.4 Integrar el change:
  - Sumar en *Added* de `## [Unreleased]` en `CHANGELOG.md`: `wt sync [<name>...] [--all] [--base <ref>] [--no-fetch]` y el esquema `wt.sync.v1`.
  - Abrir el PR titulado `feat(sync-worktrees): add wt sync [v0.3.0-alpha.4]`.

  Verificar el título con `git fetch origin && go run ./tools/relcheck pr --branch "$(git branch --show-current)" --title "<título>"`.

## Workflow follow-up

- Archivar con `/opsx:archive sync-worktrees` en el mismo PR, sincronizando las specs, con todas las tareas tildadas y el CI en verde.
- Correr `openspec validate --all --strict` después del archive y commitear en la misma rama.
