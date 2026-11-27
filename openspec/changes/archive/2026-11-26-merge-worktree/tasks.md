# Tasks

Las tareas **[Windows]** se cierran con el CI de `windows-latest` o a mano en Windows, nunca solo desde macOS. Los tests puros corren en cualquier OS. Los tests in-process corren contra repos git reales en directorios temporales, con los helpers de `sync_test.go` (`syncRepo`, `commitIn`, `h.head`, `h.contains`, `h.rebasing`).

## 1. Decisión pura (`internal/worktree`, `internal/hook`)

- [x] 1.1 Implementar `MergeTarget`, `MergeState`, `CheckMerge`, `MergeBlock` y `MergeLine` en `internal/worktree/merge.go` según D1. Verificar con tests de tabla:
  - `MergeTarget("origin/main", ["origin"])` da `main`, `MergeTarget("main", ["origin"])` da `main`, y con los remotes `up` y `up/stream` el más largo gana;
  - `CheckMerge` respeta el orden de D1: un rebase en curso y detached da `a rebase is in progress`; un target inexistente da 3 con su nombre; el propio, 2; cambios, 5; el target checkouteado con cambios, 5 con su ruta; `NoRebase` sin `Contains`, 5 con `feat does not contain main` y el hint `--no-rebase`; y nil cuando no hay nada;
  - `MergeLine` da las líneas exactas de la spec, con y sin `dryRun`.
- [x] 1.2 Sumar `PreMerge` y `PostMerge` a `internal/hook` según D6, con `target` en `Vars`. Verificar con los tests de tabla existentes de `TestVars`, `TestBackground` y `TestKey` ampliados, y con los tests de `config-layers` que listan las claves: `wt config get hooks.pre-merge` no imprime nada y es una clave conocida. Verificar in-process el escenario "Target outside a merge event".

## 2. Fast-forward (`internal/git`)

- [x] 2.1 Implementar `git.FastForward` según D5. Verificar con tests sobre repos reales:
  - con la rama checkouteada en el principal, el principal queda en el commit nuevo y tiene el archivo nuevo;
  - sin checkout, `refs/heads/<branch>` queda en el commit nuevo y ningún worktree cambia;
  - con un `old` que no es el valor actual, falla en los dos casos y la rama no se mueve;
  - con un archivo sin versionar en el principal que choca con uno nuevo, falla y el archivo sigue como estaba.

## 3. Extracciones sin cambio de comportamiento (`internal/cli`)

- [x] 3.1 Extraer `a.rebaseOnto` de `a.syncOne` según D3. Verificar con los tests existentes de `wt sync` en verde, sin tocarlos.
- [x] 3.2 Partir `a.planRemove` en `planRemove` y `planRemoveOf`, y sacar la escritura de `a.remove` a su llamador, según D4. Verificar con los tests existentes de `wt remove` y de los hooks de remove en verde, sin tocarlos.

## 4. `wt merge` (`merge-worktree`)

- [x] 4.1 Crear `internal/cli/merge.go` con el comando, sus flags, `MaximumNArgs(1)` y el help (D9), y registrarlo en `run.go`. Verificar con tests in-process "Two targets" (exit 2 sin leer el repo) y el test de help existente (`Short` y `Long`).
- [x] 4.2 Implementar el plan de D2 (pasos 1 a 5). Verificar con tests in-process los escenarios:
  - "Main worktree", "Working directory override", un worktree con HEAD detached (exit 5), y fuera de un repo (exit 3);
  - "Default target", "Target not found", el target igual a la rama propia (exit 2);
  - "Uncommitted change", un archivo sin versionar (exit 5), un rebase en curso (exit 5), un worktree con lock (exit 5) y el mismo con `--no-remove` (sigue), "Target checked out with changes";
  - "Hook not approved", y "Merge checks the remove events" de `project-hooks-approval`.
- [x] 4.3 Implementar los efectos de D2 (pasos 6 a 10), sin Ctrl-C. Verificar con tests in-process los escenarios:
  - "Behind the target", "Conflict" (con `h.rebasing` falso y los dos commits sin mover), "Behind without rebasing";
  - "Target checked out in the main worktree", "Nothing to merge";
  - "Merged and removed" (con la directiva del shell), "Keeping the worktree";
  - "pre-remove fails after the merge", "pre-merge fails" y "Values of a merge event" de `hooks`;
  - "Order of the merge events" de `hooks`, esperando a que terminen los de background como en los tests de `post-remove`.
- [x] 4.4 Implementar `--dry-run` y `--json` según D8. Verificar con tests in-process "Preview", "Merged as JSON", que `--dry-run` no mueve ninguna rama ni escribe la directiva, y que `--dry-run` con un hook sin aprobar da exit 8 como sin `--dry-run`.
- [x] 4.5 Sumar el Ctrl-C de D7. Verificar con un test e2e en `cmd/git-wt/interrupt_test.go` (`//go:build !windows`, como `TestInterruptSync`) que le manda SIGINT solo a `git-wt` mientras rebasea, con un hook de git `pre-rebase` que tarda: escenario "Interrupt during the rebase (macOS, Linux)".
- [x] 4.6 Implementar las completions de D9. Verificar in-process con `__complete` el escenario "Targets", y que fuera de un repo no ofrece nada y no escribe en stderr.

## 5. Windows

- [x] 5.1 **[Windows]** _(Cerrada con el job `windows-latest` del PR #30 en `f5047e3`.)_ Correr en el CI de `windows-latest` los tests de 2.1 y 4.x, que borran el worktree desde adentro y actualizan los archivos del principal. Verificar que pasan sin tests salteados en Windows salvo el e2e de SIGINT.
- [ ] 5.2 **[Windows]** Verificar a mano en Windows Terminal con PowerShell 7.4, a través de la función de shell, que `wt merge` desde adentro de un worktree lo borra y deja el shell en el principal, con `main` actualizado. Anotar el resultado en el PR.

## 6. Docs e integración

- [x] 6.1 Actualizar `docs/design/product.md`: §4 (la fila de `wt merge`, sin `--squash` hasta M4 y con `--no-hooks`), §5 (los eventos de merge, que dejan de ser futuros) y §7 si la fila de `merge-worktree` cambia; y la lista de comandos de `README.md`. Verificar con `grep -n "wt merge" docs/design/product.md README.md`.
- [ ] 6.2 Verificar a mano en macOS, con el binario de la rama (`go install ./cmd/git-wt`) y la función de shell, en un repo con `pre-merge = ["go test ./..."]` aprobado, que `wt merge` desde un worktree atrás de `main` rebasea, corre el hook, mergea, borra y deja el shell en el principal. Anotar el resultado en el PR.
- [x] 6.3 _(Corrido en Linux: gofmt, vet, `go test -race`, validate con los mismos 13 avisos previos de main, y los 5 targets. macOS lo cubre el CI.)_ Correr en macOS `gofmt -l .`, `go vet ./...`, `go test -race ./...`, `openspec validate --all --strict` y la cross-compilación de los 5 targets de `.goreleaser.yaml`. Verificar que gofmt no imprime nada y que no hay fallos nuevos.
- [x] 6.4 Integrar el change:
  - Sumar en *Added* de `## [Unreleased]` en `CHANGELOG.md`: `wt merge [<target>] [--no-rebase] [--no-remove] [--no-hooks]`, el esquema `wt.merge.v1`, los eventos `pre-merge` y `post-merge` y la variable `{target}`.
  - Abrir el PR titulado `feat(merge-worktree): add wt merge [v0.3.0-alpha.5]`.

  Verificar el título con `git fetch origin && go run ./tools/relcheck pr --branch "$(git branch --show-current)" --title "<título>"`.

## Workflow follow-up

- Archivar con `/opsx:archive merge-worktree` en el mismo PR, sincronizando las specs, con todas las tareas tildadas y el CI en verde.
- Correr `openspec validate --all --strict` después del archive y commitear en la misma rama.
- No taggear `v0.3.0`: sale después de una semana de uso en macOS.
