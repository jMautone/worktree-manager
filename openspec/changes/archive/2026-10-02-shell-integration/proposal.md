# Proposal: shell-integration

## Why

Hoy `wt` no puede moverte a ningún lado: `git wt list` te muestra los worktrees, pero para entrar a uno tenés que copiar la ruta y tipear `cd`. Un binario no puede cambiar el directorio de la shell que lo lanzó, así que el verbo central del producto —"llevame a ese worktree"— necesita una función de shell que haga el `cd` por él, en zsh, bash, fish y PowerShell 7. Es el mecanismo con más riesgo multiplataforma del proyecto, y `create-worktree` depende de él para "te deja parado adentro": tiene que estar probado en shells reales antes de construir encima.

## What Changes

- **Nuevo comando `wt shell init <zsh|bash|fish|pwsh>`**: imprime el script que se pone en el archivo de arranque de la shell. El script define la función `wt` y registra las completions. No lee la config, para que un archivo de config roto no rompa el arranque de la shell.
- **Función de shell `wt`**: corre `git-wt` con los argumentos tal cual, sin capturar su salida, devuelve su exit code, y cambia de directorio si el binario se lo pide por el **archivo-directiva**. En Windows, la función gana sobre `wt.exe` (Windows Terminal) por precedencia de la shell, como decidió D4 en `docs/design/product.md`.
- **Nuevo comando `wt cd <target>`**, donde `<target>` es:
  - un nombre de worktree (`NAME` de `wt list`) o una rama del repo actual;
  - `^`: el worktree principal;
  - `@`: la raíz del worktree actual;
  - `-`: el directorio en el que estabas antes del último salto de `wt` en esta sesión de shell.
- **`wt cd` sin la función activa falla** con exit 1 y un hint con la línea de `wt shell init` para cada shell. El comportamiento no depende del entorno: `wt cd` nunca imprime la ruta en stdout salvo con `--json` o `--dry-run`.
- **Completions** para los 4 shells, derivadas de la misma definición de comandos: subcomandos, flags, nombres de worktree para `wt cd` y shells para `wt shell init`.
- **Variables de entorno** del protocolo entre la función y el binario (no son claves de config): `WT_DIRECTIVE_CD_FILE` y `WT_PREVIOUS_DIR`. `wt` no se las pasa a los procesos que lanza (git hoy; hooks y `-x` después).
- **Nuevos esquemas JSON**: `wt.cd.v1` y `wt.shell.init.v1`.
- Claves de config: ninguna se agrega, modifica ni quita. Ningún comando ni flag existente cambia.

## Capabilities

### New Capabilities

- `shell-integration`: `wt shell init`, la función `wt` en cada shell, el protocolo del archivo-directiva, la memoria del directorio previo por sesión, el aislamiento de las variables del protocolo respecto de los procesos hijos y las completions.
- `navigate`: `wt cd` y sus destinos (`<name>`, `^`, `@`, `-`): resolución de nombres dentro del repo actual, ambigüedad, errores, `--json` y `--dry-run`.

### Modified Capabilities

Ninguna. `cli-contract` ya cubre lo que `wt cd` y `wt shell init` heredan (flags globales, exit codes, formato de errores, `--json`, `--dry-run`) sin cambiar ningún requirement.

## Non-goals

- **`wt shell install`**: editar `~/.zshrc`, `~/.bashrc`, `config.fish` o `$PROFILE` por el usuario. §7 asigna a este change solo `wt shell init`; mientras tanto la línea se agrega a mano, y el help de `wt shell init` la muestra para cada shell.
- **Picker de `wt cd` sin argumento** (M4). En este change, `wt cd` sin argumento es un error de uso (exit 2); M4 lo va a cambiar.
- **`wt cd <repo>` y resolución entre repos** (M2: `workspace-discovery`, `cross-repo-resolution`). Acá los nombres se resuelven solo dentro del repo actual.
- **`wt path <name>`**: comparte la resolución de `wt cd`, pero no está en el alcance que §7 da a este change.
- **Nombre de la función configurable** (`--cmd wtm` o similar). D4 ya decidió que la función se llame `wt` y gane sobre `wt.exe`.
- **bash, zsh y fish en Windows** (Git Bash, MSYS2, WSL): `wt shell init` imprime el script igual, pero no se testea ni se garantiza. En Windows la shell soportada es PowerShell 7.
- **nushell**: fuera de v1 (`docs/design/product.md` §8).

## Milestone

M1, segundo corte de cuatro (`walking-skeleton` → **`shell-integration`** → `create-worktree` → `remove-worktree`). Ver `docs/design/product.md` §7. Además de lo que §7 lista para este change (`wt cd <name>`), entrega el resto de la capability `navigate` de M1 salvo el picker: `^`, `@` y `-`. `-` se resuelve acá porque su memoria vive en la función de shell, y agregarla después obligaría a rehacer los cuatro scripts. Este change actualiza la fila de §7 para reflejarlo.

Rama: `v0.1/shell-integration`. Publica `v0.1.0-alpha.3`.

## Diferencias por OS

Sí, y se especifican por separado:

- **Shells soportadas**: zsh, bash, fish y PowerShell 7 en macOS y Linux; solo PowerShell 7 en Windows.
- **`wt.exe` de Windows Terminal**: en Windows, la función `wt` de PowerShell gana sobre `wt.exe` en el PATH. Para abrir Windows Terminal hay que tipear `wt.exe`.
- **Rutas**: la directiva lleva la ruta nativa (`C:\...` en Windows, `/...` en macOS y Linux), con la misma regla que `git-worktrees`.
- **Archivo temporal de la directiva**: se crea en el directorio temporal del sistema (`$TMPDIR` en macOS y Linux; el de PowerShell, `%TEMP%`, en Windows).
- **Exit code de la función**: en zsh, bash y fish es el estado de retorno de `wt`; en PowerShell, `$LASTEXITCODE`.

## Impact

- Código: `internal/shell` (hoy solo `doc.go`) pasa a tener los scripts de cada shell, su composición con las completions y la escritura de la directiva; `internal/worktree` suma la resolución de un nombre a un worktree; `internal/cli` suma `cd` y `shell init`; el entorno de los procesos hijos pasa a filtrar las variables del protocolo.
- Dependencias nuevas: ninguna. Las completions las genera `cobra`, que ya está.
- CI: los tests por shell real requieren zsh, bash, fish y pwsh en macOS y Linux, y pwsh en Windows. Los runners traen pwsh y bash; fish (y zsh en Linux) se instalan en el job de test.
- Requisitos de runtime de la función: zsh 5.8+, bash 3.2+ (la de macOS), fish 3.3+, PowerShell 7.4+ (7.2 descarta los argumentos vacíos al llamar a un ejecutable). Las completions de bash requieren el paquete `bash-completion`; las de zsh, que `compinit` haya corrido antes.
- Documentación: `README.md` (cómo instalar la función por shell), `docs/design/product.md` §7 (fila de `shell-integration`).
