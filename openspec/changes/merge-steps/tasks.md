# Tasks

Las tareas **[Windows]** se cierran con el CI de `windows-latest` o a mano en Windows, nunca solo desde macOS. Los tests puros corren en cualquier OS. Los tests in-process corren contra repos git reales en directorios temporales, con los helpers de `sync_test.go` y `merge_test.go` (`syncRepo`, `commitIn`, `h.head`, `h.contains`, `h.rebasing`), más un `GIT_EDITOR` de prueba (D4): un script `sh` en macOS y Linux, un `.cmd` en Windows, que escribe, vacía o copia el archivo que recibe.

## 1. Decisión pura (`internal/worktree`, `internal/hook`)

- [x] 1.1 Implementar `StepKind`, `StepState`, `CheckStep`, `StepNoop`, `NeedsMessage`, `SquashMessage` y `StepLine` en `internal/worktree/step.go` según D1. Verificar con tests de tabla:
  - `CheckStep` respeta el orden de la spec: en curso, detached, target (3 y 2), cambios versionados para `rebase` y `push` (5, con el hint `wt step commit`), `push` sin `Contains` (5, `feat does not contain main`, hint `wt step rebase`);
  - `StepNoop`: `commit` sin `Pending`; `squash` con `Own` 0 o 1 y sin `Pending`; `rebase` con `Contains`; y que `squash` con `Own` 0 y `Pending` no es noop;
  - `NeedsMessage` es falso con `-m`, en `rebase` y `push`, y en un noop;
  - `SquashMessage` pone el comentario con la cuenta y los mensajes del más viejo al más nuevo, separados por una línea en blanco;
  - `StepLine` da las líneas exactas de la spec, con y sin `dryRun`.
- [x] 1.2 Cambiar `CheckMerge` según D1: `Changes` bloquea solo con `NoCommit`. Verificar con los tests de tabla de `merge_test.go` ampliados: con cambios y sin `NoCommit` da nil; con `NoCommit`, el exit 5 de siempre.
- [x] 1.3 Sumar `PreCommit` y `PostCommit` a `internal/hook` según D8. Verificar con `TestVars`, `TestBackground` y `TestKey` ampliados, y con los tests de `config-layers`: `wt config get hooks.pre-commit` no imprime nada, y el escenario "Event that does not exist" con `post-push`.

## 2. Commit con vuelta atrás (`internal/git`)

- [x] 2.1 Implementar en `internal/git/commit.go` la foto (`write-tree` y HEAD), el stage (`add -A`), el commit con `-m` y la vuelta atrás según D2. Verificar con tests sobre repos reales:
  - un archivo modificado y uno sin versionar quedan en el commit, y un ignorado no;
  - con un hook nativo `commit-msg` que sale con 1: HEAD no se mueve y el index vuelve a la foto, con un cambio stageado que sigue stageado y otro sin stagear que sigue sin stagear;
  - squash: con tres commits propios, el commit nuevo tiene como padre al merge-base y los archivos del HEAD viejo; con el `commit-msg` que falla, la rama vuelve al HEAD viejo.
- [x] 2.2 Implementar el commit con editor según D3 (`git commit` y `git commit --edit -F`, en foreground), y la distinción entre mensaje vacío y fallo de git leyendo `COMMIT_EDITMSG` con `git stripspace`. Verificar con tests sobre repos reales con el `GIT_EDITOR` de prueba: un editor que escribe da ese mensaje; uno que vacía da el error de mensaje vacío y la vuelta atrás; uno que copia el archivo muestra el mensaje inicial del squash; y un `COMMIT_EDITMSG` viejo no se confunde con un mensaje nuevo.

## 3. Extracciones sin cambio de comportamiento (`internal/cli`)

- [x] 3.1 Sumar `StdinTTY` a `Env` y llenarlo en `cmd/git-wt/main.go` según D4. Verificar con los tests existentes en verde y un test de `main_test.go` que, con stdin redirigido, lo deja en falso.
- [x] 3.2 Extraer `a.mergeTarget` (el fast-forward con sus líneas) de `a.merge` según D6. Verificar con los tests existentes de `wt merge` en verde, sin tocarlos.

## 4. `wt step` (`merge-steps`)

- [x] 4.1 Crear `internal/cli/step.go` con `wt step` y sus cuatro subcomandos, sus flags y su help (D6), y registrarlo en `run.go`. El help de `push` dice que no hace `git push`, y el de `squash` cómo volver con `ORIG_HEAD`. Verificar con tests in-process "No subcommand", "Unknown subcommand", "Commit takes no target", "Rebase runs no hook" y el test de help existente.
- [x] 4.2 Implementar el plan de `a.planStep` y los chequeos. Verificar con tests in-process "Main worktree", "Rebase in progress", HEAD detached (exit 5), fuera de un repo (exit 3), "Default target", "Target not found", "Modified file", "Untracked file", "Behind the target" y "Script without a message".
- [x] 4.3 Implementar `commit` y `squash` con sus hooks. Verificar con tests in-process "Untracked and ignored files", "Commit with a message", "Nothing to commit", "Three commits", "Commits and pending changes", "One commit", "Message from the editor", "Messages of the branch", "Editor saves nothing", "No message needed", "Native hook rejects the commit", "Squash rejected" y "pre-commit fails"; y de `hooks`, "Order of the commit events" y "Commit without hooks".
- [x] 4.4 Implementar `rebase` y `push`. Verificar con tests in-process "Rebased", "Conflict" (con `h.rebasing` falso) y "Target moved, worktree kept", y que `push` corre `pre-merge` y `post-merge`.
- [x] 4.5 Implementar `--dry-run` y `--json` (`wt.step.v1`). Verificar con tests in-process "Preview of a squash", "Dry run needs no message", "Commit as JSON", y que `--dry-run` no mueve ninguna rama ni toca el index.
- [x] 4.6 Implementar las completions. Verificar con `__complete` "Subcommands" y "Targets of a squash", y que fuera de un repo no ofrece nada y no escribe en stderr.
- [x] 4.7 _(El stdin del test no es una terminal, así que el mensaje va con `-m` y un hook `commit-msg` de git que tarda hace de editor: la señal llega igual mientras git commitea. `TestInterruptSquash`.)_ Sumar el Ctrl-C de D9. Verificar con un test e2e en `cmd/git-wt/interrupt_test.go` (`//go:build !windows`) que manda SIGINT a `git-wt` mientras un `GIT_EDITOR` de prueba espera: escenario "Interrupt in the editor (macOS, Linux)".

## 5. `wt merge` compone los pasos (`merge-worktree`)

- [ ] 5.1 Sumar `--squash`, `-m` y `--no-commit` a `wt merge`, y `NeedsMessage` y las aprobaciones de commit a `a.planMerge`, según D7. Verificar con tests in-process "Squash without committing", "Uncommitted change" (con `--no-commit`), "Target checked out with changes", "Message needed in a script", y de `project-hooks-approval` "Merge checks the commit events" y "Clean merge does not check the commit events".
- [ ] 5.2 Implementar el commit y el squash dentro de `a.merge`, con `post-commit` según D7. Verificar con tests in-process "Pending change committed and merged", "Empty message stops the merge", "Merged as one commit", "Order of the events of a merge that commits", y de `hooks` "pre-commit fails in a merge".
- [ ] 5.3 Actualizar el dry run y `wt.merge.v1` (`committed`, `squashed`). Verificar con tests in-process "Preview of a commit" y "Merged as JSON" ampliado con los dos campos.
- [ ] 5.4 Actualizar el help de `wt merge` (el pipeline con el paso del commit, y `--no-commit`). Verificar con el test de help existente.

## 6. Windows

- [ ] 6.1 **[Windows]** Correr en el CI de `windows-latest` los tests de 2.x, 4.x y 5.x con el `GIT_EDITOR` `.cmd`. Verificar que pasan sin tests salteados en Windows salvo el e2e de SIGINT.
- [ ] 6.2 **[Windows]** Verificar a mano en Windows Terminal con PowerShell 7.4, a través de la función de shell, que `wt step commit` sin `-m` abre el editor que tiene configurado Git for Windows, que guardar vacío no commitea nada, y que `wt merge` desde un worktree sucio commitea, mergea y deja el shell en el principal. Anotar el resultado en el PR.

## 7. Docs e integración

- [ ] 7.1 Actualizar `docs/design/product.md` §4 (las filas de `wt merge`, con `--squash`, `-m` y `--no-commit`, y de `wt step`, con `-m`, `--no-hooks` y que `push` no es `git push`) y §5 (los eventos de `commit`, que dejan de ser futuros), y la lista de comandos de `README.md`. Verificar con `grep -n "wt step\|wt merge\|pre-commit" docs/design/product.md README.md`.
- [ ] 7.2 Verificar a mano en macOS, con el binario de la rama (`go install ./cmd/git-wt`) y la función de shell, en un worktree con tres commits y un archivo modificado: `wt merge --squash` abre el editor con los tres mensajes, y al guardar mergea un solo commit, borra el worktree y deja el shell en el principal. Anotar el resultado en el PR.
- [ ] 7.3 Correr en macOS `gofmt -l .`, `go vet ./...`, `go test -race ./...`, `openspec validate --all --strict` (CLI 1.13.0) y la cross-compilación de los 5 targets de `.goreleaser.yaml`. Verificar que gofmt no imprime nada y que no hay fallos nuevos.
- [ ] 7.4 Integrar el change:
  - Sumar en `## [Unreleased]` de `CHANGELOG.md`: en *Added*, `wt step commit|squash|rebase|push`, el esquema `wt.step.v1`, `wt merge --squash`, `-m` y `--no-commit`, y los eventos `pre-commit` y `post-commit`; en *Changed*, que `wt merge` commitea lo pendiente en vez de frenar con exit 5.
  - Abrir el PR titulado `feat(merge-steps): add wt step and commit in wt merge [v0.4.0-alpha.1]`.

  Verificar el título con `git fetch origin && go run ./tools/relcheck pr --branch "$(git branch --show-current)" --title "<título>"`, una vez publicado `v0.3.0`.

## Workflow follow-up

- Archivar con `/opsx:archive merge-steps` en el mismo PR, sincronizando las specs, con todas las tareas tildadas y el CI en verde.
- Correr `openspec validate --all --strict` después del archive y commitear en la misma rama.
- No mergear antes de que salga `v0.3.0`: los milestones no se superponen.
