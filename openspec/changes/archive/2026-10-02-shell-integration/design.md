# Design: shell-integration

## Context

`walking-skeleton` dejó el patrón que este change copia: `cli.Run(ctx, Env) int` como único punto de entrada, decisión pura en `internal/worktree`, efectos en el borde (`git.Runner`, `os`), errores tipados `*cli.Error` que `Run` traduce a exit code, y tests contra git real en `t.TempDir()` con el entorno aislado por `internal/testutil`. `internal/shell` existe pero solo tiene `doc.go`. El comando `completion` de cobra está deshabilitado (`DisableDefaultCmd`); el comando oculto `__complete`, que es el protocolo que usan los scripts de completion, sigue existiendo. La motivación está en `proposal.md`; el comportamiento, en `specs/`.

Hechos observados que moldean el diseño:

1. **bash de macOS es 3.2.** No hay `${var@Q}`, `mapfile`, `local -n` ni traps de función. El script de bash se escribe para 3.2 y se testea con la del sistema en `macos-latest`.
2. **En zsh con `EXTENDED_GLOB`, un `^` suelto se expande** a todos los archivos del directorio (`print -r -- ^` imprime `a b`). Sin la opción, queda literal. `wt cd ^` no puede arreglar esto desde el binario: la expansión ocurre antes de llamarlo.
3. **Los scripts de completion de cobra piden las completions con el nombre tipeado** (`${words[0]}` en bash, `${words[1]}` en zsh, `$args[1]` en fish, `$Program` en pwsh), es decir, llaman a la función `wt`, no a `git-wt`.
4. **El script de bash de cobra (V2) necesita `_get_comp_words_by_ref`** del paquete `bash-completion`; sin él, cada TAB falla con "command not found". **El de zsh empieza con `compdef _wt wt`**, que no existe hasta que corre `compinit`.
5. **PowerShell 7.2 usa por defecto `$PSNativeCommandArgumentPassing = 'Legacy'`**, que descarta los argumentos vacíos al llamar a un ejecutable. 7.4 (LTS) usa `Standard` en macOS y Linux, y `Windows` en Windows, que para un `.exe` común se comporta igual que `Standard`.
6. **En bash y zsh los alias se expanden al definir la función**, no al ejecutarla. Un `alias rm='rm -i'` cargado antes que el script convertiría la limpieza del temporal en una pregunta interactiva.
7. **Los runners de GitHub traen pwsh y bash en los tres OS, y zsh en macOS.** fish no viene en ninguno, y zsh no está garantizado en Ubuntu.
8. **En PowerShell, un `@` suelto no parsea**: `wt cd @` falla con "Unrecognized token" antes de llamar a la función. `wt cd '@'` anda, y `^` no tiene problema.
9. **En pwsh, el stderr del binario llega a la terminal aunque el script de cobra redirija.** Cobra pide las completions con `Invoke-Expression … 2>&1 | Out-Null`, pero ese `2>&1` no alcanza al ejecutable que corre adentro de la función `wt`: la línea `Completion ended with directive: …` que cobra escribe en stderr aparecería en cada TAB.
10. **Sin valores y con `ShellCompDirectiveNoFileComp`, el completer de pwsh de cobra devuelve `""`**, para que PowerShell no complete nombres de archivo. `TabExpansion2` llamado directo puede lanzar una excepción por ese valor; PSReadLine, que es quien lo llama en cada TAB, la ignora. En una pwsh interactiva, TAB no ofrece nada y no escribe nada.

## Goals / Non-Goals

**Goals:**
- Que el protocolo función ↔ binario quede cerrado en este change: `create-worktree` (`te deja adentro`, `-x`) y M2 (`wt cd <repo>`) lo usan sin tocar los cuatro scripts.
- Que cada requirement de `shell-integration` se pruebe en la shell real, en el OS donde se soporta, dentro del CI.

**Non-Goals:**
- Limpiar el temporal cuando el usuario interrumpe con Ctrl-C en bash (ver Riesgos).
- Optimizar la latencia de las completions: cada TAB es un `git-wt __complete` más un `git worktree list`; el caché de porcelain es M4.

## Decisions

### D1. El archivo-directiva lleva una ruta, no código de shell

El binario escribe en `WT_DIRECTIVE_CD_FILE` la ruta nativa del destino y nada más; la función la lee entera y hace `cd` a eso. El binario nunca le entrega código para evaluar.

*Alternativa descartada:* que el binario escriba comandos de shell para `eval` (permitiría futuras directivas como `export`). Obliga a escapar rutas para cuatro sintaxis, y convierte cualquier escritura en ese archivo en ejecución de código en la shell del usuario.
*Alternativa descartada:* capturar stdout (`cd "$(git-wt cd x)"`). Le quita la TTY al binario: sin color, sin picker (M4) y sin `-x claude` interactivo.

### D2. La función pasa las variables solo a esa invocación

```
   zsh / bash                                  fish
   ----------------------------------------    -----------------------------------
   tmp=$(command mktemp "${TMPDIR:-/tmp}/      set -l tmp (command mktemp ...)
         wt.XXXXXX")                           WT_DIRECTIVE_CD_FILE=$tmp \
   WT_DIRECTIVE_CD_FILE=$tmp \                   WT_PREVIOUS_DIR=$__wt_previous_dir \
     WT_PREVIOUS_DIR=${__wt_previous_dir-} \     command git-wt $argv
     command git-wt "$@"                       set -l rc $status
   rc=$?                                       ...
   if [ -s "$tmp" ]; then
     dest=$(<"$tmp")                           pwsh
     __wt_previous_dir=$PWD                    -----------------------------------
     builtin cd -- "$dest" || ...              $tmp = [IO.Path]::GetTempFileName()
   fi                                          try {
   command rm -f -- "$tmp"                       $env:WT_DIRECTIVE_CD_FILE = $tmp
   return $rc                                    $env:WT_PREVIOUS_DIR = $global:__WtPreviousDir
                                                 & $exe @args   # Application
                                               } finally {
                                                 restaurar $env:*, leer, Set-Location
                                                 -LiteralPath, Remove-Item
                                               }
```

- **Asignación como prefijo** en zsh, bash y fish: la variable existe solo en el entorno de ese proceso y la shell no la ve después. En pwsh no hay prefijos: se setea `$env:` y se restaura en `finally`, que además limpia el temporal aunque haya Ctrl-C.
- **Memoria de `-`**: una variable de shell **no exportada** (`__wt_previous_dir`, `$global:__WtPreviousDir`), que la función pasa como `WT_PREVIOUS_DIR` en cada llamada. Al no estar exportada, una shell hija arranca sin anterior, como pide la spec. Si el entorno ya trae `WT_PREVIOUS_DIR`, el prefijo la pisa con el valor de la sesión, que puede ser vacío.
- **`command` y `builtin` en todo**: `command mktemp`, `command rm`, `command git-wt`, `builtin cd` en zsh y bash. Por el hecho observado 6, ningún alias ni función del usuario se mete en el medio. En fish se usa `cd` (la función estándar de fish, que mantiene `dirprev` para `prevd` y `cd -`), y en pwsh, `Set-Location -LiteralPath`.
- **pwsh busca `git-wt` como Application** (`Get-Command git-wt -CommandType Application`), igual que `command` en POSIX, y fija `$PSNativeCommandArgumentPassing = 'Standard'` en el scope de la función, por el hecho observado 5.
- **Exit code**: zsh, bash y fish hacen `return $rc`. En pwsh, `$LASTEXITCODE` ya queda con el del binario. Si el `cd` falla y el binario había devuelto 0, la función devuelve 1; en pwsh, setea `$global:LASTEXITCODE = 1`.

*Alternativa descartada:* `export WT_PREVIOUS_DIR`. La heredarían las shells hijas y los agentes lanzados desde esa terminal, con un "anterior" que no es el suyo.
*Alternativa descartada:* guardar el anterior en un archivo de estado del usuario o en `git config`. Sería compartido entre terminales: con tres agentes en tres terminales, `wt cd -` en una te llevaría a donde estuvo otra.
*Alternativa descartada:* embeber en el script la ruta absoluta del binario (`os.Executable()`). Con Homebrew esa ruta apunta a `Cellar/<versión>/` y se rompe en el primer `brew upgrade`.

### D3. Los scripts viven como archivos embebidos en `internal/shell`

```
internal/shell/
  shell.go          Names, Script(name, completion) (string, error)   pura
                    Active(getenv) bool, ChildEnviron(environ) []string pura
  directive.go      WriteDirective(path, dest string) error           efecto
  scripts/wt.zsh  wt.bash  wt.fish  wt.ps1                          //go:embed
```

`Script` compone la función con el texto de completion que le pasa `cli`, envuelto en la guarda de cada shell (D5). Es pura: los tests comparan su salida sin shell, sin git y sin filesystem. Como archivos sueltos, los scripts se pueden chequear con `zsh -n`, `bash -n`, `fish -n` y el parser de pwsh en los tests.

`WriteDirective` abre con `os.O_WRONLY|os.O_TRUNC`, **sin** `O_CREATE`: si el archivo no existe falla y no crea nada (spec "Missing directive file"). Escribe la ruta sin salto de línea final.

`ChildEnviron` quita `WT_DIRECTIVE_CD_FILE` y `WT_PREVIOUS_DIR` de un entorno. `cli.Run` la aplica una sola vez al construir `git.Exec{Env: ...}`; hooks y `-x` van a recibir el mismo entorno filtrado. En Windows los nombres de variables no distinguen mayúsculas, así que la comparación es `EqualFold` cuando `GOOS` es `windows`.

*Alternativa descartada:* scripts como constantes de Go. No se pueden pasar por `bash -n` ni abrir con el resaltado de su lenguaje, y en un script de shell eso es la mitad de la revisión.

### D4. Resolución pura en `internal/worktree`

```go
// Resolve elige el destino de wt cd entre los worktrees de un repo.
func Resolve(ws []Worktree, target string) (Worktree, error)

type NotFoundError  struct{ Target string }          // -> exit 3
type AmbiguousError struct{ Target string; Candidates []Worktree } // -> exit 4
var  ErrNoCurrent = errors.New("not inside a worktree")            // -> exit 3
```

`^` → el `Main`; `@` → el `Current` o `ErrNoCurrent`; cualquier otro → `NAME` exacto, y si no hay, rama exacta. `-` no pasa por acá: no necesita repo, y lo resuelve `cli` leyendo `WT_PREVIOUS_DIR`.

El borde, en `internal/cli/cd.go`, en este orden:

```
   args (cobra, exit 2) -> workdir/-C (exit 2) -> shell.Active (exit 1)
     -> target == "-" ?  getenv(WT_PREVIOUS_DIR) (exit 3 si vacío)
                      :  requireRepository + loadConfig + worktree.Resolve
     -> os.Stat(dest) es directorio (exit 3)
     -> --dry-run ? imprimir : shell.WriteDirective (exit 1)
     -> --json ? wt.cd.v1 : nada
```

`wt cd` carga la config como `wt list` (con la capa del repo cuando hay uno), aunque hoy ninguna clave la afecte: así los warnings son consistentes y los hooks `pre-cd`/`post-cd` de M3 no cambian el orden. Para `-`, la config se carga sin capa de repo.

Comparación exacta y sensible a mayúsculas **en todos los OS**: los nombres de rama de git lo son, y una regla por OS para nombres sería una diferencia emergente más. La comparación de rutas del worktree actual sigue siendo la de `git-worktrees` (insensible en macOS y Windows).

*Alternativa descartada:* un paquete `internal/navigate`. `Resolve` opera sobre `[]Worktree` y no tiene otro estado; la resolución entre repos de M2 va a `internal/workspace`, como ya dice su `doc.go`.
*Alternativa descartada:* ambigüedad entre nombre y rama (exit 4). Un worktree cuyo nombre coincide con la rama de otro quedaría inalcanzable por su propio nombre.

### D5. Completions: cobra, con guarda por shell

`cli` genera el texto con la misma raíz de cobra que parsea los comandos (`GenZshCompletion`, `GenBashCompletionV2(…, true)`, `GenFishCompletion(…, true)`, `GenPowerShellCompletionWithDesc`) y se lo pasa a `shell.Script`. Los nombres dinámicos salen de `ValidArgsFunction`:

- `wt cd`: `workdir` + `loadRepository`; ante cualquier error devuelve vacío. Solo ofrece `NAME`s de worktrees que no son prunable. No carga la config, para no emitir warnings mientras se tipea.
- `wt shell init`: los cuatro nombres.
- Siempre con `ShellCompDirectiveNoFileComp`.

Guardas, por los hechos observados 3 y 4:

```
   zsh   if (( $+functions[compdef] )); then <script de cobra>; fi
   bash  if declare -F _get_comp_words_by_ref >/dev/null; then <script>; fi
   fish  sin guarda (complete es builtin; cobra hace `complete -c wt -e` antes)
   pwsh  sin guarda (Register-ArgumentCompleter reemplaza al anterior)
```

Las completions pasan por la función `wt` (hecho 3), que crea y borra un temporal por TAB. Cobra escribe `Completion ended with directive: …` en stderr. Los scripts de zsh, bash y fish lo descartan, pero en pwsh llega a la terminal (hecho 9), así que `cli.Run` descarta el stderr de los pedidos `__complete` y `__completeNoDesc`, en todas las shells: ningún script de completion lo lee.

*Alternativa descartada:* reescribir los scripts de cobra para que llamen a `git-wt` directo. Sería reemplazar texto dentro de la salida de cobra, y se rompe en silencio con cualquier actualización.
*Alternativa descartada:* completions escritas a mano por shell. Son cuatro implementaciones del mismo árbol de comandos; cobra lo deriva de la definición, que es la "fuente única" que pide `cli-contract`.

### D6. Mensajes y salida de `wt cd`

```
wt: shell integration is not active
hint: load it with the line for your shell from 'wt shell init --help'

wt: "x" matches more than one worktree
hint: /a/x (branch feature/x)
hint: /b/x (branch fix/x)

wt: no worktree named "nope"
hint: run 'wt list' to see the worktrees
```

Éxito: stdout vacío, como `cd`. `--dry-run`: `would change directory to <path>`. `--json`: `{"schema":"wt.cd.v1","path":"…"}`, con y sin `--dry-run`.

### D7. `wt shell init`

Grupo `shell` con `unknownSubcommand`, igual que `config`, y `init` con un validador propio: falta → exit 2 "missing argument <shell>"; nombre desconocido → exit 2 con la lista de soportados. No llama a `loadConfig` ni a git, como `version`. El `Long` del help lleva la tabla de líneas de instalación de la spec. `--json` → `wt.shell.init.v1`.

### D8. Tests por shell real

Viven en `cmd/git-wt/shell_test.go` y reusan el binario que ya compila `TestMain`. Cada test arma un sandbox (`testutil`) con repo y worktrees, pone el binario primero en el `PATH`, apunta el temporal del sistema a un directorio del sandbox (`TMPDIR` en macOS y Linux; `TEMP` y `TMP` en Windows) y corre un script corto en la shell, sin archivos de arranque:

```
   zsh   zsh -f -c '<script>'
   bash  bash --norc --noprofile -c '<script>'
   fish  fish --no-config -c '<script>'
   pwsh  pwsh -NoProfile -NonInteractive -Command '<script>'
```

El script carga `shell init` con la línea de instalación de la spec, ejecuta comandos e imprime `pwd` y el exit code en una forma fácil de parsear. Las rutas se comparan con `testutil.Comparable`.

Disponibilidad: si una shell no está en el `PATH`, el test hace `Skip`, salvo que figure en la variable `WT_TEST_SHELLS`; ahí falla. El CI la define por OS (`zsh,bash,fish,pwsh` en macOS y Linux, `pwsh` en Windows), así que en CI ninguna se saltea en silencio. En Windows, los tests de zsh, bash y fish no corren: no están soportados (spec "Supported shells").

Completions por shell:

```
   bash  PS1 + source bash_completion; COMP_WORDS/COMP_CWORD; __start_wt; COMPREPLY
   fish  complete -C 'wt cd '
   pwsh  try { TabExpansion2 -inputScript 'wt cd ' -cursorColumn 6 } catch { }, sin textos vacíos
   zsh   solo que `compinit` + script registra _comps[wt]; el TAB real se verifica a mano
```

pwsh se pide como lo hace PSReadLine (hecho 10): una excepción o un `CompletionText` vacío cuentan como "no se ofrece nada". `bash-completion` 1.x, la de macOS, no se carga si `PS1` está vacía, así que el test la define. Si bash figura en `WT_TEST_SHELLS` y `bash-completion` no está instalado, el test falla en vez de saltearse.

Programas falsos: `cmd/git-wt/testdata/fakeprog` (un `main.go` que `go build ./...` ignora por estar en `testdata`), compilado en `TestMain` dos veces: como `git` (registra su entorno en un archivo, para la spec "Processes started by wt") y como `wt`/`wt.exe` (imprime una marca, para "Shadowing other programs named wt"). Un programa Go sirve en los tres OS; un script de shell no sería ejecutable como `wt.exe` en Windows.

*Alternativa descartada:* testear solo con `bash -n`/`zsh -n` y unit tests de `Script`. Es justo el riesgo que §10 de `product.md` dice atacar temprano: el script parsea, pero el `cd` no ocurre.

### D9. CI

En el job `test`:

```yaml
- name: install shells (macOS)
  if: runner.os == 'macOS'
  run: brew install fish bash-completion
- name: install shells (Linux)
  if: runner.os == 'Linux'
  run: sudo apt-get update && sudo apt-get install -y zsh fish bash-completion
```

`bash-completion` es para que el test de completions de bash no se saltee. En macOS va la 1.x, la única que soporta la bash 3.2 del sistema.

y `WT_TEST_SHELLS` por OS desde la matrix (`include`). Ningún job nuevo; el nombre `test (<os>)` no cambia, porque es check requerido en `main`.

## Risks / Trade-offs

- **[Riesgo] Ctrl-C durante `wt` en bash deja el temporal.** Si el binario muere por SIGINT, bash aborta la función antes del `rm`. → Un archivo vacío o con una ruta, en el temporal del sistema, que el OS limpia. zsh usa `{ … } always { … }` y pwsh `try/finally`, que sí limpian. Los programas interactivos (`-x claude`) manejan SIGINT ellos mismos y salen normalmente, así que el caso real es raro.
- **[Riesgo] `wt cd ^` en zsh con `EXTENDED_GLOB`** expande `^` (hecho 2). → Se documenta en el help de `wt cd`: con esa opción, `wt cd '^'`. No se agrega `alias wt='noglob wt'`: cambiaría la expansión de todos los argumentos, incluido `wt each -- cmd *.log` de M2.
- **[Riesgo] `wt cd @` en PowerShell no parsea** (hecho 8). → Se documenta en el help de `wt cd`, junto al caso de zsh: en PowerShell, `wt cd '@'`.
- **[Riesgo] Salida de un ejecutable dentro de una función de pwsh.** Si pwsh redirige la salida de `& git-wt` por estar dentro de una función, el binario no ve una TTY: sin color, y en M4 sin picker. → Es el patrón habitual de perfiles de pwsh (`function v { nvim @args }`) y debería conectar la consola directo. Se verifica a mano en Windows Terminal y en macOS (tarea **[Windows]**). Si falla, el plan B es `Start-Process -NoNewWindow -Wait`, que obliga a reescribir solo la función de pwsh.
- **[Trade-off] Completions de bash solo con `bash-completion`, y de zsh solo después de `compinit`.** → La alternativa es escribir un `_get_comp_words_by_ref` propio, que es reimplementar parte de `bash-completion`. El help de `wt shell init` lo dice; la función anda igual sin completions.
- **[Trade-off] Un temporal por invocación, incluido cada TAB.** → `mktemp` + `rm` cuestan del orden de 1 ms frente a los ~10 ms de `git worktree list`. Se revisa si M4 baja el resto.
- **[Riesgo] `WT_DIRECTIVE_CD_FILE` y `WT_PREVIOUS_DIR` comparten el prefijo `WT_` con las claves de config.** `config-layers` ignora los `WT_*` que no son clave, así que hoy no chocan, pero una clave futura `previous_dir` sí. → Test en `internal/config` que falla si una clave del registro produce el nombre de una variable del protocolo.
- **[Riesgo] Windows: `bash` en el `PATH` del runner puede ser el lanzador de WSL.** → En Windows solo se testea pwsh, como declara la spec.

## Migration Plan

No aplica: no hay usuarios de `main`, y la función de v0.9 vive en el `$PROFILE` de PowerShell de quien la instaló, con otro mecanismo. Quien pruebe esta versión en Windows reemplaza ese bloque por la línea de `wt shell init pwsh`.
