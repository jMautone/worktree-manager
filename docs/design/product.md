# wt — diseño del producto (v1)

> Fuente de verdad del alcance, el contrato de CLI y el orden de ejecución.
> Agnóstico de implementación a propósito: las specs de OpenSpec derivan de acá,
> y el detalle de Go vive en el `design.md` de cada change.
>
> Estado: borrador para revisión. Fecha: 2026-09-21.
> Antecedentes: `docs/decisions/0000-analisis-worktrunk.md`, `0001-de-cero-en-go.md`.
> Sesiones, principios 1 a 3 y roadmap desde M4: `docs/decisions/0003-orquestacion-de-sesiones.md` (2026-11-27).

---

## 1. Qué es

`wt` administra git worktrees sobre un **workspace multi-repo**, desde la terminal.

La diferencia con las herramientas del mismo espacio (worktrunk, git-worktree-wrapper, scripts caseros) es que `wt` **no asume que ya estás dentro de un repo**. Descubre repos bajo raíces configurables, resuelve nombres entre repos detectando ambigüedad, y opera en lote. El caso de uso que lo define:

```
   Tenés 10 repos y 3 agentes trabajando en paralelo en worktrees distintos,
   repartidos entre esos repos. Querés saltar a cualquiera por nombre, ver el
   estado de todos en una tabla, correr algo en todos, y cerrar un worktree
   (rebase + tests + merge + borrar) con un comando.
```

Para quién: un dev que trabaja en macOS y Windows, con varios repos y agentes de IA corriendo en worktrees.

Desde M5, cada worktree es además una **sesión**: el worktree más un registro que dice qué agente, qué terminal y qué editor usa, y cómo retomarlo al día siguiente. Desde M6, un agente puede crear, lanzar y cerrar sesiones de otros agentes por MCP. Ver [`0003-orquestacion-de-sesiones.md`](../decisions/0003-orquestacion-de-sesiones.md).

## 2. Principios

1. **Terminal-first.** El resultado de un comando es dejarte parado en el worktree, en la misma terminal. Abrir una app externa (otra terminal, un editor) es siempre opt-in: un flag explícito (`--in <launcher>`, `--editor`) o un perfil del usuario que lo declara. Sin flag ni perfil, ningún comando abre nada.
2. **Los agentes corren en foreground.** `-x <cmd>` y `--agent` corren dentro del worktree, en tu terminal, y propagan el exit code. Cuando un launcher abre otra terminal, el agente corre en foreground ahí, lanzado por `wt` con `git-wt session run`; el launcher nunca ve su comando.
3. **Nada hardcodeado a una herramienta.** Agentes, terminales, editores y perfiles son datos de config, con presets por OS que se pueden pisar. Agregar uno nuevo no toca el código.
4. **El repo no ejecuta código sin permiso.** Config del repo con lista blanca; hooks del repo con aprobación por repo, evento y hash del comando.
5. **El contrato es público.** Exit codes, `--json` versionado y `--dry-run` con lecturas reales son parte de la API, no cosmética. También lo son el registro de sesión (`wt.session.v1`), el protocolo de los plugins de launcher (`wt.launch.v1`) y las herramientas MCP, que devuelven los mismos documentos.

Los principios 1 a 3 se reescribieron en [`0003`](../decisions/0003-orquestacion-de-sesiones.md) para dar lugar a las sesiones, sin cambiar lo que pasa cuando no pedís nada.

## 3. Identidad y plataformas

```
   binario          git-wt        -> tambien expone `git wt <x>` gratis
   lo que tipeas    wt            -> SIEMPRE una funcion de shell

   Por que una funcion y no el binario:
     1. un binario no puede cambiar el directorio de su shell padre.
        Sin wrapper no hay `wt cd`, que es el verbo central.
     2. una funcion de shell gana sobre el PATH, lo que resuelve de
        paso la colision con wt.exe (Windows Terminal).
```

| | |
|---|---|
| macOS (arm64, x64) | Primera clase. Desarrollo diario. |
| Windows (x64) | Primera clase. Uso laboral. |
| Linux (x64, arm64) | Best-effort. Se compila y testea en CI, no se usa a diario. |
| Shells | zsh, bash, fish, PowerShell 7. (nushell: candidato, no v1) |

Toda diferencia de comportamiento entre OS es **explícita**, nunca emergente: separadores de ruta, sensibilidad a mayúsculas del filesystem (APFS case-insensitive, ext4 no), nombres reservados (`CON`, `NUL` solo en Windows), shell de hooks (`sh -c` vs `cmd.exe /c`).

---

## 4. Contrato de CLI

### Ciclo de vida del worktree

| Comando | Qué hace |
|---|---|
| `wt create <name> [-b <branch>] [--base <ref>] [-x <cmd>] [--cd\|--no-cd] [--no-hooks] [--detach]` | Crea el worktree sobre una rama nueva, entre `pre-create` y `post-create` (los dos bloquean), y te deja parado adentro. Con `-x` corre el comando ahí, en foreground, después de `post-create`. `--cd`/`--no-cd` pisan `create_cd`; `--no-hooks` no corre ningún hook. `--profile`, `--agent` y `--in <launcher>` (lanzar en otra terminal) llegan con las sesiones, en M5. |
| `wt cd [<name>\|-\|^\|@] [--no-hooks]` | Salta a un worktree, a un repo, al anterior (`-`), al principal (`^`) o al actual (`@`). Corre `pre-cd` antes del salto y `post-cd` en background después, en el worktree destino; `--no-hooks` no corre ninguno. Sin argumento, abre el picker. |
| `wt list [--full] [--all-repos] [--json]` | Una sola tabla con todo el estado. Reemplaza al `status` de v0.9. |
| `wt remove <name> [--keep-branch] [-D] [-f] [--no-hooks]` | Borra el worktree y, por defecto, su rama si está mergeada (en la base o en su upstream). `--keep-branch` la conserva; `-D` la borra aunque no lo esté; `-f` borra el worktree con archivos sin commitear. Corre `pre-remove` antes de borrar y `post-remove` en background después; `--no-hooks` no corre ninguno. |
| `wt merge [<target>] [--no-rebase] [--no-remove] [--no-hooks]` | Cierra el ciclo desde adentro del worktree: rebase sobre el target → `pre-merge` → fast-forward del target → remove con sus hooks → `post-merge` en background. El target es una rama local, por defecto la base sin el remote. Frena en el primer paso que falla: el conflicto aborta el rebase (exit 6). El commit pendiente y `--squash` llegan con `merge-steps`, en M4; hasta entonces, un worktree sucio frena con exit 5. |
| `wt step commit\|squash\|rebase\|push [<target>]` | Corre un paso suelto del pipeline de `merge`, con sus mismos hooks. Para cuando querés el rebase sin mergear, o el squash sin borrar. |
| `wt sync [<name>...] [--all] [--base <ref>] [--no-fetch]` | Rebase de uno o varios worktrees sobre su base (la de `wt remove`, o `--base`), con un fetch por repo. Nunca stashea: el worktree sucio se saltea. El conflicto se aborta, se reporta y se sigue; exit 1, 6 o 5 si alguno no quedó al día. |
| `wt path <name>` | Imprime la ruta. Para scripts. |
| `wt lock <name> [reason]` / `wt unlock <name>` / `wt prune` | Mantenimiento. |

### Workspace (el diferencial)

| Comando | Qué hace |
|---|---|
| `wt repos [--json]` | Lista los repos descubiertos bajo las raíces configuradas. |
| `wt cd <repo>` | Salta al repo principal. Mismo verbo que para worktrees; la resolución decide. |
| `wt each [--all-repos] [--] <cmd>...` | Corre un comando en cada worktree del repo actual o, con `--all-repos`, en cada uno de `wt list --all-repos`: en serie, en tu terminal, con una cabecera por worktree, y sigue cuando uno falla. Para uno solo alcanza `wt path` (`git -C "$(wt path x)" …`). |

### Extensión y estado

| Comando | Qué hace |
|---|---|
| `wt mark <name> <text>` / `wt var <name> <k>=<v>` | Estado por worktree, guardado en su registro de sesión (`<git-common-dir>/wt/sessions/<id>/session.toml`). Visible en `list` y en plantillas. |
| `wt hook list [--json]` / `wt hook approve [<evento>...]` | Los hooks que correrían en el repo actual, del usuario y del repo, con su estado de aprobación; y la aprobación de los del repo que falten. `--dry-run` muestra qué aprobaría. |
| `wt config get\|set\|list\|path` | Config. |
| `wt shell init <zsh\|bash\|fish\|pwsh>` / `wt shell install` | Shell integration y completions. |
| `wt statusline [--format <plantilla>]` | Una sola línea con el estado del worktree actual (rama, sucio, ahead/behind, marker), rápida y sin red, para el prompt de la shell o la statusline de Claude Code. |
| `wt agent install <claude\|codex\|copilot\|gemini\|opencode>` | Instala la integración con el agente: sus hooks llaman a `wt mark` para poner 🤖 mientras trabaja y 💬 cuando espera input, y así `list` y `dash` muestran qué agente está en qué sesión. Si el agente tiene un id de conversación propio, lo guarda en el registro para que `wt resume` lo retome. |
| `wt doctor` | Diagnóstico derivado de la config. No chequea nada que no esté configurado. |
| `wt dash` | Tablero de sesiones (TUI), con acciones: retomar, lanzar, anotar, cerrar. |
| `wt <alias>` / `wt-<x>` en el PATH | Aliases de config y subcomandos externos, patrón git. |

### Sesiones (M5) y orquestación (M6)

Propuesto en [`0003`](../decisions/0003-orquestacion-de-sesiones.md); el contrato exacto lo fija el change de cada uno.

| Comando | Qué hace |
|---|---|
| `wt create <name> [--profile <p>] [--agent <a>] [--in <launcher>]` | Crea el worktree y su registro de sesión. Con `--agent`, arranca el agente en foreground; con `--in`, en otra terminal, y la tuya queda donde estaba. |
| `wt resume [<name>] [--in <launcher>] [--fresh] [--no-agent]` | Retoma una sesión: te deja parado en el worktree, imprime un brief (estado, marcador y "Siguiente paso") y lanza el `resume` del agente. Sin nombre, abre el picker por último uso. |
| `wt session show <name>` | El brief completo, con la metadata. |
| `wt session note <name> <texto>` / `wt session edit <name>` | Agrega una nota al diario / edita `context.md` con `$EDITOR`, en la misma terminal. |
| `wt session run <name>` | Lo que corre un launcher en la terminal nueva: arranca el agente y, cuando termina, deja una shell abierta. |
| `wt mcp serve` | Servidor MCP por stdio sobre las mismas operaciones de la CLI (M6). |

`wt remove` y `wt merge` cierran la sesión y archivan su contexto en `<git-common-dir>/wt/archive/`, salvo `session.archive = false`.

### Flags globales

`--dry-run` (lecturas reales, cero escrituras) · `--json` (esquema versionado) · `-C <dir>` · `--no-color` · `-h`

### Exit codes

| | |
|---|---|
| 0 | OK |
| 1 | Error de ejecución |
| 2 | Error de uso (flag desconocido, argumento faltante) |
| 3 | No encontrado (worktree, repo, rama) |
| 4 | **Nombre ambiguo entre repos** (el caso propio del workspace) |
| 5 | Bloqueado por estado (worktree sucio, lock, rama con cambios) |
| 6 | Conflicto de git (rebase/merge) |
| 7 | Hook falló |
| 8 | Hook del repo no aprobado |
| 130 | Interrumpido con Ctrl-C antes de terminar, en los tres OS |

Con `-x`, el exit code del comando ejecutado se propaga tal cual y desplaza a estos. `wt each` no propaga el de sus comandos: sale con 1 si alguno falló y con 130 si lo interrumpiste.

### Lo que NO existe (y no va a existir)

`open`, `clean`, `status` como comando aparte, `editor`/`terminal` como claves de primer nivel, cualquier clave `warp*`, chequeos de Node/fnm/Copilot en `doctor`. Son las nueve cosas que el §7 del análisis marcó para quitar: en greenfield simplemente no nacen.

Abrir otra terminal o un editor sí existe desde M5, pero como `--in <launcher>`, `--editor` y perfiles del usuario, nunca como `open` ni por defecto. `terminal` y `editor` viven dentro de un perfil, y Warp es un preset de launcher, no una clave.

---

## 5. Config

Capas, de menor a mayor precedencia: **defaults → usuario → repo (lista blanca) → perfil → env `WT_*` → flags**. La capa de perfil llega con `profiles`, en M5: sin `default_profile` ni `--profile`, no existe.

```
   macOS/Linux   $XDG_CONFIG_HOME/wt/config.toml   (default ~/.config/wt/config.toml)
   Windows       %APPDATA%\wt\config.toml
   Repo          .wt.toml en la raiz  -> SOLO claves de la lista blanca
   Env           WT_DEFAULT_BASE, WT_REPOS_ROOT, ...
```

```toml
worktree_path       = "{repo_parent}/{repo}.worktrees/{name|sanitize}"
repos_root          = ["~/Repos", "~/Documents/GIT"]
repos_depth         = 2
default_base        = "origin/main"
branch_prefix       = ""
fetch_before_create = true
create_cd           = true          # create termina parado en el worktree (no va en .wt.toml)
agent               = "claude"      # el agente de --agent sin nombre y de -x sin argumento
default_profile     = ""            # sin perfil, nada se abre afuera

[agents.claude]                     # agentes como datos; hay presets embebidos
command = ["claude"]
resume  = ["claude", "--continue"]

[launchers.warp]
preset = "warp"                     # presets iniciales: warp, pwsh (Windows), terminal (macOS)

[launchers.code]
command = ["code", "{path}"]        # un editor abre la carpeta, no corre el agente

[profiles.bugfix]                   # overlay: solo pisa lo que declara
agent    = "claude"
terminal = "warp"
editor   = "code"

[hooks]
post-create = ['cp "{repo_path}/.env" .env', "npm ci"]
pre-merge   = ["npm test"]

[aliases]
dev = "npm run dev -- --port {branch|hash_port}"
```

**Lista blanca del repo** (`.wt.toml`): `default_base`, `branch_prefix`, `fetch_before_create`, `worktree_path`, `hooks` (con aprobación). Nunca `agent`, `agents`, `launchers`, `profiles`, `default_profile` ni rutas absolutas del usuario. En 1.0 un repo tampoco puede sugerir perfiles.

### Plantillas

Un solo motor, usado por rutas, hooks, launchers, aliases y columnas de `list`.

| Variables | `{repo}` `{repo_parent}` `{repo_path}` `{branch}` `{name}` `{path}` `{base}` `{default_branch}` `{upstream}` `{owner}` `{args}` `{vars.X}` |
|---|---|
| Filtros | `sanitize` (`feature/x` → `feature-x`) · `hash_port` (puerto determinístico 10000-19999 por rama) · `codename(n)` · `lower` |

`sanitize` no es un lujo: sin él, una rama como `feature/abc1` no tiene ruta válida.

`{{` y `}}` escriben una llave literal, para lo que la usa en un comando: `awk '{{print $1}}'`, `${{HOME}}`. Se leen de izquierda a derecha, así que `{{{name}}}` es `{`, la expresión y `}`.

### Hooks

Eventos: `pre`/`post` × `create`, `cd`, `remove`, `merge`, `commit`. Hoy existen los de `create`, `cd`, `remove` y `merge`; los de `commit` llegan con `merge-steps`. Cada evento es una lista de comandos que corren en serie, con el intérprete de `-x`, y el primero que falla corta la lista.

- **Bloquean** los `pre-*` y `post-create`: corren en la terminal, con la salida en stderr, y si uno falla la operación termina con exit 7. Un `pre-*` que falla no deja nada hecho. `post-create` bloquea, y no va en background, porque `-x` no puede arrancar antes de que termine `npm ci`.
- **En background** corren `post-cd`, `post-remove` y `post-merge`: desacoplados de la terminal, sobreviven a `wt`, a Ctrl-C y al cierre de la pestaña, y su salida va a `<git-common-dir>/wt/logs/<name|sanitize>/<evento>.log`, que cada corrida pisa.

Los hooks declarados en `.wt.toml` se **suman** a los del usuario: en cada evento corren primero los del usuario y después los del repo, que nunca los pisan. Requieren **aprobación explícita** la primera vez: una aprobación es el repo (la ruta de su worktree principal), el evento y el hash SHA-256 del comando tal como está escrito, y se guarda en `approvals.toml`, junto a la config del usuario, que el repo no puede escribir. Si el comando cambia, se vuelve a pedir. Hasta entonces, `create`, `cd` y `remove` fallan con exit 8 antes de hacer nada, y `wt hook approve` los aprueba. Eso habilita "el equipo comparte `npm ci` al crear" sin darle ejecución arbitraria a un repo clonado.

### Shell integration

```
   funcion wt() en zsh/bash/fish/pwsh
     1. crea un archivo temporal
     2. exporta WT_DIRECTIVE_CD_FILE=<tmp>
     3. corre git-wt con los argumentos tal cual
     4. si el archivo tiene contenido -> cd a esa ruta
     5. borra el temporal
```

Es el único mecanismo portable para que un binario cambie el directorio de su shell padre, y es el que hace posible `wt cd` en zsh igual que en pwsh.

---

## 6. Capabilities

Veintiocho capabilities. Cada una responde una pregunta, y ese es el criterio para saber si una spec está completa.

| Capability | La pregunta que responde |
|---|---|
| `cli-contract` | ¿Cómo se parsea, se despacha, se falla y se documenta un comando? Fuente única de help, completions y docs. |
| `config-layers` | ¿De dónde sale un valor efectivo y quién puede sobrescribirlo? |
| `path-templates` | ¿Cómo se convierte una rama en una ruta válida en este OS? |
| `git-worktrees` | ¿Qué worktrees hay y cuál es su estado, según git? |
| `shell-integration` | ¿Cómo cambia un binario el directorio de su shell padre? |
| `create-worktree` | ¿Cómo nace un worktree y dónde te deja? |
| `navigate` | ¿Cómo salto a algo por nombre, incluidos `-`, `^`, `@`? |
| `list-worktrees` | ¿Qué está pasando en todos mis worktrees, en una tabla? |
| `remove-worktree` | ¿Cómo se borra un worktree sin perder trabajo? |
| `merge-worktree` | ¿Cómo se cierra un worktree completo, de una? |
| `merge-steps` | ¿Cómo corro un solo paso del cierre (commit, squash, rebase, push) sin el resto? |
| `sync-worktrees` | ¿Cómo pongo N worktrees al día sin stashear nada? |
| `workspace-discovery` | ¿Qué repos existen bajo mis raíces? |
| `cross-repo-resolution` | Este nombre, ¿a qué repo pertenece? ¿Y si es ambiguo? |
| `batch-execution` | ¿Cómo corro algo en muchos worktrees y qué pasa si uno falla? |
| `hooks` | ¿Qué corre automáticamente y cuándo? |
| `project-hooks-approval` | ¿Cómo permito hooks del repo sin confiar en el repo? |
| `agents` | ¿Cómo arranco y retomo un agente sin que `wt` sepa cuál es? |
| `sessions` | ¿Cómo retomo un worktree al día siguiente, con su agente y su contexto? |
| `launchers` | ¿Cómo abro una sesión en otra terminal o un editor, cuando lo pido explícitamente? |
| `profiles` | ¿Cómo elijo agente, terminal y editor de una sesión con un solo nombre? |
| `branch-state` | ¿En qué worktree está trabajando quién/qué? |
| `aliases-plugins` | ¿Cómo extiendo `wt` sin tocar su código? |
| `statusline` | ¿Cómo veo el estado del worktree actual en una línea, sin salir del prompt? |
| `session-context` | ¿Cómo no pierdo el objetivo, las decisiones y el siguiente paso de una sesión, y cómo los ve el agente? |
| `agent-integrations` | ¿Cómo sabe `wt`, sin que yo haga nada, si un agente está trabajando o esperándome? |
| `mcp-server` | ¿Cómo opera un agente a `wt`, y orquesta las sesiones de otros agentes, sin parsear texto? |
| `dashboard` | ¿Cómo veo N sesiones con agentes, en vivo, y actúo sobre ellas? |

Más dos que no son de producto pero sí de contrato: `distribution` (brew/scoop/winget, binarios firmados) y `docs-generation` (docs por comando derivadas de `cli-contract`).

---

## 7. Orden de ejecución

El orden es **vertical**, no por capas. La tentación en greenfield es construir toda la infraestructura primero; así se muere un rewrite: meses de fundaciones sin producto usable, mientras seguís usando la herramienta vieja. Cada milestone tiene que ser usable de punta a punta.

```
  M1  ESQUELETO USABLE      objetivo: abandonar v0.9 en macOS
       cli-contract, config-layers, path-templates, git-worktrees,
       shell-integration, create-worktree, navigate, list (basico),
       remove-worktree
         |
  M2  EL DIFERENCIAL        objetivo: mejor que worktrunk para tu flujo
       workspace-discovery, cross-repo-resolution, batch-execution
         |
  M3  CERRAR EL CICLO       objetivo: un worktree se abre y se cierra solo
       hooks, project-hooks-approval, sync-worktrees,
       merge-worktree
         |
  M4  RIQUEZA               objetivo: una sola vista dice todo
       list-worktrees (completo, detecta squash-merges),
       branch-state, picker, statusline, merge-steps
         |
  M5  EXTENSION Y SESIONES  objetivo: retomar cualquier worktree con su agente
       agents, sessions, launchers, profiles,
       doctor, aliases-plugins
         |
  M6  ORQUESTACION          objetivo: un agente orquesta a otros
       session-context, agent-integrations,
       mcp-server, dashboard
         |
  M7  PRODUCTO              objetivo: que lo instale alguien mas
       distribution, docs-generation, forge,
       llm-commits
```

M4 no cambia de alcance con las sesiones: solo `branch-state`, que guarda en el registro por worktree y es su semilla. Los milestones M5 a M7 y su orden salen de [`0003`](../decisions/0003-orquestacion-de-sesiones.md).

### M1, desglosado en changes

Los cuatro cortes de M1 se abren ahora. M2 en adelante se desglosa cuando le toca: un proposal escrito hoy para M5 va a estar mal, porque M1 va a enseñar cosas.

| Change | Entrega | Por qué en ese orden |
|---|---|---|
| `walking-skeleton` | `wt list` del repo actual. Contrato de CLI, exit codes, `--json`, config mínima, porcelain, CI en 3 OSes. | Prueba el vertical completo end-to-end antes de acumular superficie. |
| `shell-integration` | Archivo-directiva, `wt shell init` para 4 shells con completions, `wt cd <name>\|^\|@\|-`. | Es el mecanismo con más riesgo multiplataforma del proyecto. Lo querés roto en la semana 2, no en el mes 5. |
| `create-worktree` | Motor de plantillas con `sanitize` y `lower`, `wt create` sobre una rama nueva que te deja adentro, `-x` en foreground, `branch_prefix`, `fetch_before_create` y `create_cd`. | Sin `sanitize` no hay ruta para una rama con barra. |
| `remove-worktree` | `remove`, `lock`/`unlock`, `prune`. | Cierra el ciclo mínimo: ya podés dejar v0.9 en macOS. |

### M2, desglosado en changes

Tres cortes, uno por capability de M2, cada uno usable de punta a punta.

| Change | Entrega | Por qué en ese orden |
|---|---|---|
| `workspace-discovery` | `repos_root` y `repos_depth` (con `WT_REPOS_ROOT` y `WT_REPOS_DEPTH`), `wt repos` con `wt.repos.v1`, y `wt cd <repo>` desde cualquier directorio (nombre exacto, sin mayúsculas, prefijo único), con los repos en la completion de `wt cd`. | Es el salto de todos los días y lo que todavía hace volver a v0.9. El primer corte ya se usa entero, no es solo un listado. Lee el filesystem y no corre git, así que no agrega costo a lo que ya resolvía el repo actual. |
| `cross-repo-resolution` | `wt cd <worktree>` de otro repo o desde fuera de todos, la forma calificada `<repo>:<name>`, `wt list --all-repos`, `path`, `remove`, `lock` y `unlock` que resuelven en otros repos, y `wt create <repo>:<name>` que crea en otro repo. | Necesita un `git worktree list` por repo descubierto: su costo y su ambigüedad merecen un diseño propio, sobre un descubrimiento ya probado. |
| `batch-execution` | `wt each [--all-repos] [--] <cmd>...` sobre los worktrees del repo actual o de todos: en serie, sin cortar cuando uno falla, con resumen y exit 1, y exit 130 con Ctrl-C. `wt exec` no se hace: lo cubre `wt path`. | Itera sobre el conjunto que resuelven los dos anteriores; sin ellos no hay sobre qué iterar. |

### M3, desglosado en changes

Cuatro cortes: las tres capabilities de M3 y `project-hooks-approval`, que se adelanta de M5 porque sin ella los hooks son los mismos para todos los repos.

| Change | Entrega | Por qué en ese orden |
|---|---|---|
| `hooks` | `[hooks]` en la config del usuario, con los eventos `pre`/`post` de `create`, `cd` y `remove`. Los `pre-*` y `post-create` bloquean y fallan con exit 7: `post-create` porque `-x` no puede arrancar antes de `npm ci`. `post-cd` y `post-remove` corren en background, con la salida en `.git/wt/logs/`. Mismo intérprete que `-x`, y `{path}`, `{base}` y el escape `{{`/`}}` en el motor de plantillas. | `post-create` (copiar el `.env`, `npm ci`) es la fricción diaria más grande, y `post-cd` y `post-remove` en background son el riesgo multiplataforma de M3: un proceso que sobrevive a `wt` en Windows. Como `shell-integration` en M1, lo querés roto temprano. Y `merge` lo necesita. |
| `project-hooks-approval` | `hooks` en la lista blanca de `.wt.toml`, con aprobación por repo, evento y hash del comando guardada del lado del usuario (`approvals.toml`), `wt hook list`, `wt hook approve` y exit 8 cuando un hook del repo no está aprobado. | Sin ella los hooks son globales: un `post-create = ["npm ci"]` corre también en un repo de Go, y los tests de `pre-merge` no pueden ser por repo. Va antes de `merge` para que sus tests sean del repo desde el primer día. |
| `sync-worktrees` | `wt sync [<name>...] [--all] [--base <ref>] [--no-fetch]`: rebase sobre la base con un solo fetch por repo. Nunca stashea (el worktree sucio se saltea), el conflicto se reporta y se sigue, y termina con un resumen y un exit code. | El rebase y la política de conflicto se prueban solos, sin hooks, fast-forward ni borrado. `merge` reusa ese rebase. |
| `merge-worktree` | `wt merge [<target>] [--no-rebase] [--no-remove] [--no-hooks]` desde adentro del worktree: limpio o exit 5, rebase (exit 6), `pre-merge` (exit 7 u 8), fast-forward del target, `remove` con sus hooks y `post-merge`. | Compone los tres anteriores: es el único que depende de todos. |

Dos reglas del corte:

- **Cada evento de hook llega con el comando que lo dispara**: `create`, `cd` y `remove` con `hooks`; `merge` con `merge-worktree`; `commit` con `merge-steps`, en M4.
- **`merge` no escribe commits en M3**: solo mueve los que hay (rebase y fast-forward). El commit pendiente y `--squash` comparten el problema del mensaje (editor, `-m`, los mensajes de la rama, `llm-commits` en M7), y se diseñan una sola vez, con `merge-steps`. Tampoco detecta ramas squash-mergeadas: después del fast-forward la rama ya cuenta como mergeada. Esa detección la necesitan `remove` y la vista completa de `list`, y llega con `list-worktrees` en M4.

### M4, desglosado en changes

Cinco cortes, uno por capability de M4. `merge-steps` va primero porque cierra lo que M3 dejó afuera, y los otros cuatro van en orden de dependencia: cada uno lee lo que calcula el anterior.

| Change | Entrega | Por qué en ese orden |
|---|---|---|
| `merge-steps` | `wt step commit\|squash\|rebase\|push [<target>]`, `wt merge` que commitea lo pendiente en vez de frenar con exit 5, `wt merge --squash`, `-m` para el mensaje (si no, el editor de git), y los eventos `pre-commit` y `post-commit`. | Es la fricción diaria que dejó M3: un worktree sucio no se puede mergear. No depende de nada de M4, y el mensaje de commit se diseña una sola vez para `commit`, `squash` y `merge`. |
| `list-worktrees` | La vista completa de `wt list`: ahead/behind de la base y del upstream, sucio, último commit, `--full`, y la detección de ramas squash-mergeadas, que también usa `wt remove` para borrar la rama. | Es el objetivo del milestone, y la fuente del estado que leen los tres que siguen. |
| `branch-state` | `wt mark` y `wt var`, guardados por worktree en `<git-common-dir>/wt/sessions/<id>/session.toml`, visibles en `list` y en las plantillas. | Agrega una columna y variables sobre la tabla ya completa. Es la semilla del registro de sesión: `sessions` (M5) le suma campos y `agent-integrations` (M6) escribe ahí, sin migrar nada. |
| `picker` | `wt cd` sin argumento abre un picker (TUI con `bubbletea`) sobre las filas de `list`, con filtro por nombre. | Es la primera TUI del proyecto: muestra lo que `list` ya calcula, con el marker. |
| `statusline` | `wt statusline [--format <plantilla>]`: una línea con el estado del worktree actual, rápida y sin red. | Es un subconjunto de una fila de `list` con su marker, con un presupuesto de tiempo propio. Va último porque reusa los dos anteriores. |

Dos reglas del corte:

- **`merge-steps` no detecta squash-merges** hechos fuera de `wt` (en GitHub, por ejemplo). Su `--squash` deja la rama con un solo commit que el fast-forward mete en el target, así que la rama queda mergeada sin necesidad de detectarla. La detección llega con `list-worktrees`.
- **M4 no trae sesiones** ([`0003`](../decisions/0003-orquestacion-de-sesiones.md)), pero no les cierra la puerta: `branch-state` guarda en el registro por worktree en vez de `git config`, y `wt.list.v1` admite los campos de sesión que suma `sessions`, sin cambiar de versión.

### Definición de "listo" por milestone

Un milestone no está cerrado hasta que: los tests pasan en la matrix de 3 OSes, `wt doctor` no reporta nada roto, las specs están sincronizadas a `openspec/specs/`, y **lo usaste una semana en macOS sin volver a v0.9**.

### Versiones

Cada milestone sale como un minor, y cada change mergeado como una pre-release de ese minor (`v0.1.0-alpha.N`). El detalle está en [`0002-versionado-y-releases.md`](../decisions/0002-versionado-y-releases.md).

| Milestone | Versión |
|---|---|
| M1 | `0.1.0` |
| M2 | `0.2.0` |
| M3 | `0.3.0` |
| M4 | `0.4.0` |
| M5 | `1.0.0` |
| M6 | `1.1.0` |
| M7 (opcional) | `1.2.0` |

---

## 8. Non-goals de v1

- Portar código de v0.9. Se lee como referencia; no se traduce.
- Soportar PowerShell 5.1 o Windows sin PowerShell 7.
- Reemplazar a git. `wt` orquesta git, no lo reimplementa (sin libgit2).
- Colaboración multi-usuario, sincronización de estado entre máquinas, o servicio remoto.
- Integración con forges (PR, CI) antes de M7. Es caro de mantener y opcional.
- Sesiones sin worktree, o varias sesiones sobre un mismo worktree. En v1 la relación es 1:1.
- Guardar transcripciones de los agentes. `wt` guarda la memoria curada de la sesión; el historial es de cada agente.
- Un daemon o proceso residente. El estado vive en archivos y se deriva de git cada vez.
- Soporte de nushell en v1.

## 9. Decisiones abiertas

| # | Decisión | Recomendación | Bloquea |
|---|---|---|---|
| D1 | Formato de config: TOML o JSON | **TOML**: admite comentarios, y una herramienta con config rica los necesita. v0.9 usaba JSON, pero en greenfield no hay compatibilidad que preservar. | `config-layers` (M1) |
| D2 | ¿`wt list` muestra todos los repos por defecto, o solo el actual? | Solo el actual, con `--all-repos` para el workspace. Menos sorpresa y más rápido. | `list-worktrees` (M1) |
| D3 | Nombre del directorio de worktrees por defecto | `{repo_parent}/{repo}.worktrees/{name\|sanitize}`: el layout de v0.9 (que usaba `{name}`), más `sanitize`. Con `{name}` y no `{branch}`, `wt cd <name>` encuentra lo que creaste aunque uses `-b` o `branch_prefix`. Decidido en `create-worktree`. | `path-templates` (M1) |
| D4 | ¿`wt` gana sobre `wt.exe` en Windows, o se instala como `wtm`? | La función de shell gana; es lo que ya pasa en v0.9 sin fricción. Costo residual: tipear `wt.exe` para Windows Terminal. | `shell-integration` (M1) |

## 10. Riesgos

| Riesgo | Mitigación |
|---|---|
| La shell integration se comporta distinto en cada shell y se vuelve un pozo de bugs | Está en M1 a propósito, con tests por shell real en CI. Si va a fallar, que falle temprano. |
| El rewrite nunca cruza el umbral de "ya es mejor, me cambio" y muere al 70% | M1 tiene como criterio de cierre "una semana sin volver a v0.9". El umbral es explícito y temprano. |
| Windows se verifica poco porque el desarrollo es en macOS | Matrix de CI en los 3 OSes desde el primer change; las tareas que requieren verificación manual en Windows se marcan como tales. |
| El alcance crece hacia worktrunk y nunca termina | 1.0 cierra en M5; M6 y M7 son posteriores, y M7 es opcional. El diferencial está en M2, no en M4-M7. |
| Abrir terminales de forma portable se vuelve el nuevo pozo de bugs | Contrato mínimo (directorio + argv), presets como datos, plugins para lo raro, y verificación manual **[macOS]** y **[Windows]** en `launchers`. |
| Un agente conectado por MCP, inducido por el contenido de un repo, borra o lanza lo que no debe | Destructivas en `dry_run` por defecto con `confirm`, launchers y agentes solo por nombre de la config del usuario, grupos de herramientas configurables y ninguna herramienta que apruebe hooks. |
