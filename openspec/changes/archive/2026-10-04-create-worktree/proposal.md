# Proposal: create-worktree

## Why

Hoy `wt` lista worktrees y te mueve entre ellos, pero no crea ninguno: para empezar una tarea hay que tipear `git worktree add -b feat ../repo.worktrees/feat origin/main`, inventar la ruta a mano (y reemplazar la barra de `feature/x`, que no es un nombre de carpeta válido), `cd` a ese lugar y recién ahí lanzar el agente. Es el paso que más se repite en el flujo de "tres agentes en paralelo", y mientras falte seguís volviendo a v0.9 para hacerlo. Este change entrega el verbo que abre el ciclo: `wt create feat -x claude` crea el worktree en la ruta que dicta la config, te deja parado adentro y corre el agente ahí mismo, en foreground.

## What Changes

- **Nuevo comando `wt create <name>`**: crea un worktree del repo actual sobre una **rama nueva** y te deja parado en su raíz a través de la función de shell.
  - La carpeta sale de `worktree_path` (por defecto `{repo_parent}/{repo}.worktrees/{name|sanitize}`); la rama es `branch_prefix` + `<name>`, o la de `-b`.
  - La rama nace de `--base`, o de `default_base`, o de la rama por defecto del repo (`origin/HEAD`, y si no existe, la rama del worktree principal), y queda **sin upstream**.
  - Solo crea ramas nuevas: si la rama ya existe, local o en un remoto, falla con exit 5 sin tocar nada. Si la ruta ya existe, también.
  - Sin la función de shell activa, crea igual y avisa con un warning que la shell no se mueve.
- **Flags nuevos de `wt create`**:
  - `-b, --branch <branch>`: la rama, tal cual, sin `branch_prefix`.
  - `--base <ref>`: desde dónde nace la rama. Completa nombres de rama locales y remotas.
  - `-x <cmd>`: corre `<cmd>` en el worktree, en la terminal actual, y `wt` sale con su exit code. Lo corre `sh` en macOS y Linux, y `cmd.exe` en Windows.
  - `--cd` / `--no-cd`: pisan `create_cd` para esta invocación. Juntos son error de uso.
- **Motor de plantillas** (`path-templates`): sintaxis `{variable|filtro|...}`, variables `{repo}`, `{repo_parent}`, `{repo_path}`, `{name}`, `{branch}`, y filtros `sanitize` (convierte cualquier texto en un nombre de carpeta válido en macOS, Windows y Linux, igual en los tres) y `lower`. Una plantilla mal escrita en `worktree_path` es config inválida (exit 1) para todos los comandos.
- **Claves de config nuevas**:
  - `branch_prefix` (string, `""`, permitida en `.wt.toml`).
  - `fetch_before_create` (booleano, `true`, permitida en `.wt.toml`): si la base es `<remoto>/<rama>`, hace `git fetch` de ese remoto antes de crear; si falla, warning y sigue con lo que hay.
  - `create_cd` (booleano, `true`, **no** permitida en `.wt.toml`): si `wt create` te mueve.
- **Claves de config modificadas**: `worktree_path` pasa a validarse como plantilla, y su default cambia de `{branch|sanitize}` a `{name|sanitize}`, para que `wt cd <name>` encuentre siempre lo que creaste aunque uses `-b` o `branch_prefix`. Ningún comando de `main` consumía esta clave todavía.
- **Booleanos por entorno**: `WT_FETCH_BEFORE_CREATE` y `WT_CREATE_CD` aceptan `true`, `false`, `1` y `0`.
- **Contrato de exit codes**: con `-x`, una vez que el comando arrancó, su exit code reemplaza a los de `wt`, tal cual (si lo mata una señal en macOS o Linux, 128 + la señal).
- **Nuevo esquema JSON**: `wt.create.v1`. `-x` y `--json` juntos son error de uso.
- Se quita: nada. Ningún comando ni flag existente cambia.

## Capabilities

### New Capabilities

- `path-templates`: sintaxis de plantillas, variables y filtros, `sanitize`, y cómo una plantilla renderizada se convierte en una ruta absoluta y nativa (incluido `~`, rutas relativas y la validez de la ruta en Windows).
- `create-worktree`: `wt create` de punta a punta: nombre y rama, base, fetch previo, ruta, rechazo de ramas y rutas existentes, movimiento de la shell, `-x`, `--json`, `--dry-run`, `-C` y completions de `--base`.

### Modified Capabilities

- `config-layers`: la tabla de claves suma `branch_prefix`, `fetch_before_create` y `create_cd`; `worktree_path` cambia de default y pasa a ser una plantilla; nuevo requirement para leer booleanos del entorno.
- `cli-contract`: el requirement de exit codes suma la excepción de `-x` (el exit code del comando reemplaza a los de `wt`).
- `git-worktrees`: nuevo requirement "rama por defecto" del repo, que hoy consume `wt create` y después van a consumir `sync` y `merge` (M3).

## Non-goals

- **Hooks `post-create` y copiar archivos al worktree** (`.env`, `node_modules`): M3 (`hooks`). Mientras tanto, `-x` puede encadenar lo que haga falta.
- **Sacar una rama que ya existe**, local o remota (por ejemplo, para revisar la rama de otro): decidido fuera de alcance; `wt create` solo crea ramas nuevas, y para eso está `git worktree add`.
- **`--detach`** (lanzar en un tab nuevo) y **la clave `agent`** con `-x` sin argumento: son launchers, M5. Además, un `-x` con valor opcional rompería `wt create feat -x claude` (el parser tomaría `claude` como segundo argumento).
- **Worktrees con HEAD detached** (`git worktree add --detach`).
- **Picker de `wt create` sin nombre** (M4) y **`wt create` en otro repo del workspace** (M2): acá, sin `<name>` es error de uso, y el repo es el del directorio actual o el de `-C`.
- **`-x` en `wt cd`**: no está en el contrato de `docs/design/product.md` §4.
- **El resto del motor de plantillas**: las variables `{path}`, `{base}`, `{default_branch}`, `{upstream}`, `{owner}`, `{args}`, `{vars.X}` y los filtros `hash_port` y `codename(n)` llegan con el change que los use (hooks, aliases). Tampoco hay forma de escribir una llave literal.
- **Que `wt cd` resuelva por nombre sanitizado** (`wt cd feature/x` cuando la carpeta es `feature-x` y la rama lleva prefijo): sería un cambio de `navigate`.

## Milestone

M1, tercer corte de cuatro (`walking-skeleton` → `shell-integration` → **`create-worktree`** → `remove-worktree`). Ver `docs/design/product.md` §7: "motor de plantillas con `sanitize`, `wt create` que te deja adentro, `-x` en foreground". Suma lo que §5 lista para `create` (`branch_prefix`, `fetch_before_create`, `create_cd`) y deja fuera lo que es de M3 (hooks) y M5 (`--detach`, `agent`).

Este change actualiza `docs/design/product.md`: el default de `worktree_path` en §5, la decisión D3 de §9 (que decía `{branch|sanitize}` "igual que v0.9", cuando v0.9 usaba `{name}`), la firma de `wt create` en §4 (`--cd`/`--no-cd`) y la fila de §7.

Rama: `v0.1/create-worktree`. Publica `v0.1.0-alpha.4`.

## Diferencias por OS

Sí, y se especifican por separado:

- **Intérprete de `-x`**: `sh -c` en macOS y Linux; `cmd.exe` en Windows. Las comillas y los operadores (`&&`, `&`) siguen las reglas de ese intérprete, no las de la shell interactiva del usuario (en Windows, PowerShell).
- **Exit code de `-x` terminado por una señal**: 128 + número de señal en macOS y Linux (130 con Ctrl-C). En Windows no hay señales: se propaga el código que devuelve el proceso.
- **Validez de la ruta**: en Windows, una ruta renderizada con un componente reservado (`CON`, `NUL`, `COM1`…), con `< > : " | ? *` o que termina en punto o espacio es error de uso. En macOS y Linux esa validación no existe. `sanitize` produce lo mismo en los tres OS, así que con él esto nunca dispara.
- **Rutas**: la plantilla se escribe con `/` en todos los OS; la ruta resultante se reporta nativa (`C:\src\repo.worktrees\feat` en Windows).
- **Mayúsculas**: en macOS y Windows (filesystems que no distinguen mayúsculas), `wt create Feat` choca con una carpeta `feat` existente y falla por ruta existente; en Linux no.

## Impact

- Código: `internal/template` (hoy solo `doc.go`) pasa a tener el parser, el render y los filtros; `internal/config` suma las tres claves, el tipo booleano y la validación de `worktree_path`; `internal/git` suma las operaciones de rama, base, fetch y `worktree add`; `internal/worktree` suma la decisión de create (rama, ruta, base, plan); paquete nuevo `internal/process` para correr `-x` en foreground; `internal/cli` suma `create`.
- Dependencias nuevas: ninguna.
- Contrato público: un comando, cinco flags, tres claves, dos variables `WT_*`, un esquema JSON y una excepción en los exit codes.
- CI: sin jobs nuevos. Los tests por shell real de `shell_test.go` suman `wt create` a través de la función en zsh, bash, fish y pwsh.
- Documentación: `docs/design/product.md` (§4, §5, §7, §9 D3), `CHANGELOG.md`.
