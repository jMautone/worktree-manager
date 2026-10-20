# Proposal: batch-execution

## Why

Con `cross-repo-resolution`, `wt list --all-repos` muestra todos los worktrees del workspace. Pero para hacer algo en cada uno (un `git fetch`, `npm test`, mirar `git status` antes de cerrar el día) todavía hay que entrar uno por uno, o escribir un `for` de shell que no se escribe igual en zsh que en PowerShell. Con varios agentes trabajando en worktrees repartidos entre repos, "correr esto en todos y decirme cuáles fallaron" es la operación en lote del caso de uso que define el producto (`docs/design/product.md` §1).

Este change es el último corte de M2. `wt each` recorre los mismos worktrees que `wt list` (o `wt list --all-repos`) y corre un comando en cada uno: en serie, en la terminal y sin cortar cuando uno falla.

## What Changes

**Se agrega**

- **Comando `wt each [--all-repos] [--] <cmd>...`**: corre `<cmd>` en cada worktree del repo actual o, con `--all-repos`, en cada worktree que lista `wt list --all-repos`, en ese mismo orden. Incluye el worktree principal de cada repo.
  - **Dónde empieza el comando**: en la primera palabra que no es un flag de `wt`. El `--` es opcional. Desde esa palabra, todo es del comando, aunque parezca un flag de `wt`: en `wt each npm test --json`, el `--json` es de `npm`.
  - **Cómo se arma la línea**:
    - Una sola palabra va tal cual al intérprete, como `-x`: `wt each 'npm ci && npm test'`.
    - Varias palabras van cada una entre comillas cuando hace falta, para que lleguen enteras al programa: en `wt each git commit -m "fix bug"`, `fix bug` sigue siendo un solo argumento.
    - El intérprete es el de `-x`: `sh -c` en macOS y Linux, `cmd.exe` en Windows.
  - **Cómo corre**: en serie, en la raíz de cada worktree, con la entrada, la salida y el error de la terminal (color, prompts, Ctrl-C).
  - **Cabecera**: antes de cada comando, una línea en stdout `==> <worktree>  <ruta>`. El worktree aparece como lo toma `wt cd`: `feat` y `^` en el repo actual; `api:feat` y `api:^` con `--all-repos`.
  - **Si un comando falla**:
    - `wt each` sigue con los demás.
    - Al final escribe en stderr `wt: <n> of <m> commands failed` y un `hint:` por cada comando que falló, con su exit code, y sale con 1.
    - Si todos salen 0, no imprime nada más y sale con 0.
  - **Ctrl-C**: espera a que termine el comando que está corriendo, no arranca el siguiente, dice cuántos no corrieron y sale con **130**.
  - **Lo que se saltea** (no cuenta en el resumen ni cambia el exit code):
    - las entradas bare, sin aviso, porque no tienen archivos;
    - un worktree cuyo directorio no existe (prunable, o bloqueado en un disco desconectado), con un warning antes de empezar.
  - **`--dry-run`**: imprime `would run <línea> in <ruta>` por worktree y no corre nada.
  - **Entorno**: el comando no recibe `WT_DIRECTIVE_CD_FILE` ni `WT_PREVIOUS_DIR`, como `-x`.
  - **Completions**: antes del comando, los flags de `wt each`. Desde la primera palabra del comando, los archivos que ofrece la shell, y nunca un flag de `wt`.
- **Exit code 130**: interrumpido desde la terminal (Ctrl-C) antes de terminar. Lo usa `wt each`, con el mismo valor en los tres OS.

**Se modifica**

- **Contrato de CLI**:
  - **Posición de los flags**: hoy los flags globales van "antes o después del nombre del comando". En `wt each` siguen valiendo después de `each`, pero no después de la primera palabra del comando del usuario.
  - **Formato de error**: hoy, cuando un comando falla, no escribe nada en stdout. `wt each` es una excepción: cuando reporta en stderr que algún comando falló o que se interrumpió, ya escribió en stdout las cabeceras y lo que imprimieron los comandos.
- **Shell integration en PowerShell**: la spec dice hoy que la función le pasa al binario los argumentos sin cambios. PowerShell no lo cumple: borra un `--` sin comillas antes de que la función lo vea (`'--'` entre comillas sí pasa). El script `wt.ps1` no cambia, porque desde la función no hay forma de recuperar ese `--`; se corrige la spec. `wt each` está diseñado para no depender de ese `--`.
- **`docs/design/product.md`**: en §4, la fila de `wt each` queda con el alcance de este change y se quita la de `wt exec`. En §7 se actualiza la fila de `batch-execution`.

**Se quita**

- Nada del contrato actual.
- **`wt exec`**, que estaba en el plan del producto, no se implementa: `wt path` ya lo cubre, con `(cd "$(wt path x)" && cmd)` o `git -C "$(wt path x)" …`.
- No se agrega ni se quita ninguna clave de config ni variable de entorno, y no hay esquema JSON nuevo.

## Capabilities

### New Capabilities

- `batch-execution`: el comando `wt each`. Cubre:
  - qué worktrees recorre y en qué orden, y qué se saltea;
  - dónde empieza el comando y cómo se arma su línea en cada OS;
  - cómo corre cada comando, la cabecera y el resumen;
  - los exit codes, incluido Ctrl-C;
  - `--dry-run`, el rechazo de `--json` y las completions.

### Modified Capabilities

- `cli-contract`:
  - el exit code 130;
  - que en `wt each` los flags de `wt` van solo antes del comando del usuario;
  - la excepción de stdout en el formato de error.
- `shell-integration`: en PowerShell, un `--` sin comillas no llega al binario.

## Non-goals

- **`wt exec <name> -- <cmd>`**: lo cubre `wt path`.
- **Elegir sobre qué worktrees corre**: sin `--repo <r>` y sin filtros por rama, estado o marker. `wt each` recorre lo que lista `wt list` o `wt list --all-repos`.
- **Paralelismo y `--fail-fast`**: siempre corre en serie y siempre sigue.
- **`--json`**: es un error de uso, como `-x` con `--json` en `wt create`. La salida de cada comando va a la terminal y no se captura, así que no hay nada que devolver. Para saber qué worktrees hay está `wt list --json`.
- **Plantillas (`{branch}`) o variables `WT_*` en el comando**:
  - las llaves chocan con `awk '{print $1}'` y con `find -exec {} \;`;
  - el directorio de trabajo ya dice en qué worktree corre el comando;
  - si hacen falta, se definen con los hooks, en M3.
- **Prefijar cada línea de salida con su worktree** (como `parallel --tag`): corriendo en serie, alcanza con la cabecera.
- **Paridad con v0.9 en dos puntos**, decidido a propósito:
  - v0.9 excluía el principal salvo con `--include-main`. Acá se incluye, porque `wt list` lo lista.
  - v0.9 cortaba en el primer fallo salvo con `--continue-on-error`. Acá siempre sigue.

## Milestone

M2, tercer y último corte (`workspace-discovery` → `cross-repo-resolution` → **`batch-execution`**). Ver la tabla "M2, desglosado en changes" en `docs/design/product.md` §7.

Esa tabla le asignaba a este change `wt exec` y `wt each` sobre uno o todos los repos. Queda solo `wt each`, sobre el repo actual o sobre todos, y este change actualiza la fila. Con él, M2 queda completo. El cierre de M2 (`v0.2.0`) va en un PR aparte, como pasó con M1.

Rama: `v0.2/batch-execution`. Publica `v0.2.0-alpha.3`.

## Diferencias por OS

Sí. Son explícitas y se especifican por separado:

- **Intérprete**: `sh -c` en macOS y Linux, `cmd.exe` en Windows. Es el mismo de `-x`.
- **Comillas cuando hay varias palabras**:
  - En macOS y Linux se usan las comillas simples de `sh`, dentro de las cuales `sh` no interpreta nada.
  - En Windows se usan las reglas con las que los programas parten su línea de comando (las del runtime de C de Microsoft).
  - `cmd.exe` expande `%NOMBRE%` de una variable definida aunque esté entre comillas, y no hay forma de evitarlo.
  - Los builtins de cmd, como `echo`, reciben la línea con las comillas incluidas.
- **Ctrl-C**: el mismo comportamiento y el mismo 130 en los tres OS.
  - En macOS y Linux llega como SIGINT a todo el grupo de procesos. En Windows llega como evento de consola.
  - En Windows, si el comando es un `.cmd` (como `npm`), cmd.exe pregunta `Terminate batch job (Y/N)?`. Aunque contestes N, `wt each` no arranca el siguiente.
- **Shells**: PowerShell borra un `--` sin comillas, pero `wt each` no lo necesita. En zsh, bash y fish el `--` llega al binario y `wt` lo descarta.
- **Rutas**: nativas en cabeceras, warnings y `--dry-run`.

## Impact

- **Código**:
  - `internal/process` suma una función pura que arma la línea a partir de varias palabras, por OS. `Run` no cambia.
  - `internal/worktree` suma la decisión pura de qué se corre, qué se saltea y en qué orden.
  - `internal/cli` suma `wt each`: el parseo con la primera palabra posicional como corte, la lectura de los worktrees (reusa lo de `wt list` y `wt list --all-repos`), el chequeo de qué directorios existen, el bucle con las cabeceras, la detección de Ctrl-C y el resumen. También suma la constante del exit code 130.
- **Dependencias nuevas**: ninguna.
- **Contrato público**:
  - un comando nuevo (`wt each`) y un exit code nuevo (130);
  - una excepción en el formato de error;
  - la corrección de la spec de la función de PowerShell.
  - Sin esquemas JSON, claves de config ni variables de entorno nuevas.
- **CI**: sin jobs nuevos. Los tests corren contra repos reales en directorios temporales, en los tres OS. El `--` y el `wt each` a través de la función se prueban en las cuatro shells reales.
- **Documentación**: `docs/design/product.md` §4 y §7, y `CHANGELOG.md`.
