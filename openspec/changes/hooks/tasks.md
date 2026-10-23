# Tasks

Las tareas **[Windows]** se cierran con el CI de `windows-latest` o a mano en Windows, nunca solo desde macOS. Los tests puros reciben `goos` y corren en cualquier OS. Los escenarios propios de Windows se escriben en la sección 5 y se saltean en macOS y Linux.

Los tests in-process de hooks bloqueantes corren comandos reales (`sh -c` o `cmd.exe`) contra repos reales en directorios temporales. Los de background reemplazan `startBackground` por la versión síncrona de D6. El proceso desacoplado real se prueba en los e2e de `cmd/git-wt`.

## 1. Plantillas, eventos y config (puro)

- [ ] 1.1 Implementar el escape `{{`/`}}` en `template.Parse` según D3. Verificar:
  - con tests de tabla:
    - `{{x}}` da `{x}`;
    - `{{{name}}}` da `{`, la expresión y `}`;
    - `a}}b` da `a}b`;
    - `x}}}` y `{{name}` dan error, con la parte ofensiva en `SyntaxError.Part`;
    - toda plantilla de los tests actuales renderiza igual;
  - in-process, los escenarios de `path-templates` "Literal braces", "Literal brace around an expression" y "Lone closing brace after a literal one".
- [ ] 1.2 Crear `internal/hook` según D1: `Event`, `Events`, `Background`, `Key`, `Vars`, `Plan`, `LogPath`, `Header` y `PreviewLines`, con su `doc.go`. Verificar con tests de tabla:
  - `Vars` incluye `base` solo en `pre-create` y `post-create`;
  - `Background` es verdadero solo en `post-cd` y `post-remove`;
  - `Plan` renderiza en orden y devuelve error con `{base}` en `pre-remove`;
  - `Plan` con una lista vacía da un `Run` sin comandos;
  - `LogPath` da `<common>/wt/logs/<name|sanitize>/<event>.log` en darwin, linux y windows, con separadores nativos, y aplica `sanitize` (`a:b` → `a-b`);
  - `Header` da `==> post-create  npm ci`;
  - `PreviewLines` da `would run pre-create hook: …` para los bloqueantes y `would run post-cd hook in the background: …` para los de background.
- [ ] 1.3 Implementar `worktree.HookVars(main, w, base, goos)` según D1, reusando `PathVars`. Verificar con tests de tabla:
  - `name` es el NAME (último componente de `w.Path`), no el `<name>` tipeado;
  - `branch` es vacío con HEAD separado y en un bare;
  - `repo` de un bare va sin `.git`;
  - `path` y `repo_path` van en forma nativa para darwin y windows;
  - `base` pasa tal cual.
- [ ] 1.4 Sumar a `internal/config`, según D2:
  - `Key.NoEnv` y su uso en `EnvLayer`;
  - las seis claves `hooks.*`, con `validateCommands(event)`;
  - el aplanado de tablas en `Resolve`, con el error de `hooks` que no es tabla.

  Verificar:
  - tests de `config` para:
    - el aplanado con un mapa;
    - el error con un array y con un string;
    - el warning `unknown key "hooks.post-merge"`;
    - el warning de repo para `hooks.post-create` en la capa `repo`;
    - que `WT_HOOKS_POST_CREATE` no se lee;
    - que un elemento en blanco, un string suelto y una plantilla con `{nope}` fallan nombrando la clave y el archivo;
  - in-process, los escenarios de `config-layers`:
    - "Defaults with no configuration files", "Key of a table" y "No environment variable for hooks";
    - todos los de "Hook commands";
  - in-process, "Base outside a create event" de `hooks`;
  - actualizar los tests de `wt config list` que enumeran todas las claves.

## 2. Efectos de base

- [ ] 2.1 Implementar `git.CommonDir(ctx, r, dir)` según D7. Verificar contra repos reales del sandbox:
  - desde la raíz del principal da `<main>/.git` absoluto;
  - desde un subdirectorio y desde un worktree enlazado da el mismo;
  - en un bare da el propio repo;
  - con el layout `.bare` da `<root>/.bare`;
  - con `InitSeparateGitDir` da el directorio git separado.
- [ ] 2.2 Implementar `process.StartDetached(path, args, dir, env, out)` según D6, con `process_unix.go` (`Setsid`) y `process_windows.go` (`CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW | CREATE_BREAKAWAY_FROM_JOB`, con reintento sin breakaway ante `ERROR_ACCESS_DENIED`). Verificar:
  - en Unix, con `fakeprog`: el hijo escribe en `out`, `StartDetached` vuelve antes de que el hijo termine, y el hijo corre en otra sesión (`getsid` distinto al del test);
  - en todos los OS, que un ejecutable inexistente devuelve error;
  - con un test puro de la elección de flags: con breakaway primero, y sin él después de `ERROR_ACCESS_DENIED`.
- [ ] 2.3 Crear el comando oculto `__run-hooks` según D6: `Hidden: true`, `SetInterspersed(false)`, `--event` y `--dir`, y el bucle que escribe la cabecera, corre con `process.Run` contra su propio stdout y escribe el pie. Verificar in-process:
  - el log con dos comandos que salen con 0 tiene las seis líneas en orden;
  - el primero que sale con 1 corta, y el segundo no corre;
  - un comando que no arranca deja `==> could not start: …`;
  - `__run-hooks` no aparece en `wt -h` ni en las completions de `wt `.

## 3. Hooks en los comandos

- [ ] 3.1 Implementar en `internal/cli`, según D5, D6 y D8:
  - `runBlocking`, con la variable `runHook`;
  - `startBackground` como variable, con la versión síncrona para tests;
  - la apertura del log (`MkdirAll` + `O_TRUNC`);
  - las funciones puras de mensajes y hints de fallo e interrupción.

  Verificar con tests internos:
  - los textos `pre-create hook failed: exit 1 exited with 1`, `… could not start: …` e `interrupted: pre-cd hook did not finish`;
  - el hint de `--no-hooks` en los `pre-*`;
  - el hint con la ruta del worktree en `post-create`;
  - el hint con el log bajo `--json`;
  - el código 7 o 130 según el caso;
  - el warning cuando `startBackground` falla.
- [ ] 3.2 Integrar los hooks en `wt create` según D4: `--no-hooks`, `PreHook` y `PostHook` en `CreatePlan`, el nuevo orden, la salida después de `post-create`, la directiva cuando falla `post-create`, `-x` salteado, y las líneas de `--dry-run`. Verificar in-process:
  - de `hooks`:
    - "Order of the create events", "Worktree of another repository", "Values of a create event", "Name made by worktree_path" y "Literal braces";
    - "In order", "First failure stops the event", "Interpreter operators (macOS, Linux)", "Output on stderr" y "The command waits (macOS, Linux)";
    - "pre-create fails" y "post-create fails", con el archivo de directiva del harness;
    - "Hooks of a repository are ignored", "Create without hooks" y "Preview of a create with hooks";
    - "Hook output with JSON" y "Failed hook with JSON";
  - de `create-worktree`: "Line after the post-create hook", "Not run when a hook fails" y "Preview with hooks";
  - de `cli-contract`: "Report after a hook";
  - los tests actuales de `wt create` en verde.
- [ ] 3.3 Integrar los hooks en `wt remove` según D4: `--no-hooks`, `pre-remove` antes del `Chdir`, y `post-remove` después de la salida, también cuando falla el borrado de la rama. Verificar in-process:
  - de `remove-worktree`: "Remove without hooks", "Hooks around the removal", "No post-remove when the removal fails (macOS, Linux)" y "Preview with hooks";
  - de `hooks`: "pre-remove fails", "A failure does not change the exit code", "Log of post-remove" y "Bare repository";
  - `pre-remove` corre en el principal cuando el directorio del worktree ya no existe;
  - los tests actuales de `wt remove` en verde.
- [ ] 3.4 Integrar los hooks en `wt cd` según D4:
  - `--no-hooks`;
  - `cdDestination` devuelve el worktree;
  - `loadRepository(dest)` para `-`, solo con hooks de cd configurados;
  - `pre-cd`, la directiva, `post-cd` y las líneas de `--dry-run`.

  Verificar in-process:
  - de `navigate`: "Hooks around the jump" y "Preview a jump with hooks";
  - de `hooks`:
    - "pre-cd fails" y "Jump without hooks";
    - "Previous directory outside every repository" y "Removing from inside does not fire cd events";
    - "Printing a path runs no hook";
    - "Notice on stderr" y "Each run replaces the log";
    - "Command without hooks";
  - que sin hooks configurados `wt cd -` no corre ningún `git` de más (con el runner que cuenta comandos de los tests);
  - los tests actuales de `wt cd` y `wt path` en verde.
- [ ] 3.5 Actualizar el `Long` de `wt create`, `wt cd` y `wt remove`: qué hooks corre cada uno, `--no-hooks`, que `post-create` bloquea y que `post-*` de cd y remove van en background con su log. Actualizar también la ayuda de `wt config` con un ejemplo de `[hooks]` y el escape `{{`/`}}`. Verificar con tests de que `wt create -h`, `wt cd -h` y `wt remove -h` nombran `--no-hooks` y los eventos.

## 4. Procesos reales y shells

- [ ] 4.1 Escribir e2e en `cmd/git-wt/hooks_test.go` con el binario real:
  - "wt does not wait";
  - "Signals to the terminal (macOS, Linux)": `wt` en su propio grupo de procesos, y SIGHUP y SIGINT a ese grupo después de que sale;
  - "Protocol variables not passed", con un `fakeprog` que registra su entorno en `post-create` y `post-cd`;
  - que el log real de `post-remove` tiene cabecera, salida y pie.

  Verificar que pasan en macOS y en el CI de Linux, esperando el log con un sondeo con timeout.
- [ ] 4.2 Escribir el e2e de Ctrl-C en `cmd/git-wt/interrupt_test.go`, "Interrupt during pre-create (macOS, Linux)": SIGINT durante un `pre-create` de `sleep 1`. Comprobar que `wt` sale después del hook, con 130, el mensaje, y sin rama ni worktree.
- [ ] 4.3 Sumar a los tests de shell real (zsh, bash, fish, pwsh) el escenario "post-create fails" a través de la función: el shell termina en el worktree, el resultado es 7, y `ran.txt` no existe. Verificar que pasa con las cuatro shells instaladas en macOS y en el CI.

## 5. Windows

- [ ] 5.1 **[Windows]** Escribir los escenarios de Windows, salteados fuera de Windows: "Interpreter operators (Windows)" y "The command waits (Windows)" in-process, y el e2e de 4.1 de que `wt remove` no espera a `post-remove` y que su log aparece. Verificar en el CI de Windows, junto con los tests de 1.x a 4.1 que corren en todos los OS.
- [ ] 5.2 **[Windows]** Escribir el e2e de Ctrl-C para Windows: `git-wt` con `CREATE_NEW_PROCESS_GROUP`, y `CTRL_BREAK_EVENT` con `GenerateConsoleCtrlEvent` mientras corre un `pre-create` que espera (`fakeprog`). Comprobar 130 y que no hay worktree. Si el runner no tiene consola, se saltea con `t.Skip`, que nombra la tarea 5.3. Verificar en el CI de Windows que pasa o que se saltea con ese motivo.
- [ ] 5.3 **[Windows]** Verificar a mano en Windows Terminal con PowerShell 7.4. Anotar el resultado en el PR:
  - "Closing the console (Windows)": un `post-remove` de `ping -n 11 127.0.0.1 > nul& echo done`, cerrando la pestaña enseguida, termina con `==> exited with 0` en el log;
  - no se abre ninguna ventana al arrancar un hook de background;
  - "Ctrl-C in the console (Windows)": un `pre-create` con `pause`, Ctrl-C, `$LASTEXITCODE` 130 y sin worktree;
  - "post-create fails" a través de la función;
  - `wt cd feat` con un `post-cd`, mostrando la línea del log.

## 6. Verificación en macOS, docs e integración

- [ ] 6.1 Verificar a mano en macOS, en Terminal.app, con el binario de la rama instalado (`go install ./cmd/git-wt`) y esta config:

  ```toml
  [hooks]
  post-create = ['cp "{repo_path}/.env" .env', "npm ci"]
  post-remove = ["sleep 5; echo done"]
  ```

  Anotar el resultado en el PR:
  - `wt create x -x "ls node_modules | head -3"` en un repo con `package.json` muestra la salida de `npm ci` antes del `ls`;
  - `wt remove x` vuelve enseguida y, cerrando la pestaña, el log termina con `done` y `==> exited with 0`;
  - `wt create y --dry-run` muestra los comandos renderizados;
  - `wt create z --no-hooks` no copia el `.env`.
- [ ] 6.2 Actualizar `docs/design/product.md` según D9: §4 (filas de `create`, `cd` y `remove`), §5 (Hooks) y §7 (fila de `hooks`). Verificar que `grep -n 'post-\*' docs/design/product.md` ya no dice que todos los `post-*` van en background, y que `openspec validate --all --strict` sigue en verde.
- [ ] 6.3 Correr en macOS `gofmt -l .`, `go vet ./...`, `go test -race ./...`, `openspec validate --all --strict` y la cross-compilación de los 5 targets de `.goreleaser.yaml`. Verificar que gofmt no imprime nada y que no hay fallos.
- [ ] 6.4 Integrar el change:
  - Sumar sus líneas bajo `## [Unreleased]` en `CHANGELOG.md`:
    - en *Added*:
      - la tabla `[hooks]` con los seis eventos;
      - los bloqueantes y los de background, con el log;
      - las variables;
      - `--no-hooks`;
      - el exit 7 y el 130;
      - `--dry-run` y `--json` con hooks;
    - en *Changed*:
      - el escape `{{`/`}}` en las plantillas;
      - que `wt create` imprime su línea después de `post-create`.
  - Abrir el PR titulado `feat(hooks): run hooks on create, cd and remove [v0.3.0-alpha.1]`.

  Verificar el título con `git fetch origin && go run ./tools/relcheck pr --branch "$(git branch --show-current)" --title "<título>"`.
