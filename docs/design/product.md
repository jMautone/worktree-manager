# wt — diseño del producto (v1)

> Fuente de verdad del alcance, el contrato de CLI y el orden de ejecución.
> Agnóstico de implementación a propósito: las specs de OpenSpec derivan de acá,
> y el detalle de Go vive en el `design.md` de cada change.
>
> Estado: borrador para revisión. Fecha: 2026-09-21.
> Antecedentes: `docs/decisions/0000-analisis-worktrunk.md`, `0001-de-cero-en-go.md`.

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

## 2. Principios

1. **Terminal-first.** El resultado de un comando es dejarte parado en el worktree, en la misma terminal. Ningún comando abre una app externa por defecto.
2. **Los agentes corren en foreground.** `-x <cmd>` corre dentro del worktree, en tu terminal, y propaga el exit code. El tab nuevo es un launcher opcional.
3. **Nada hardcodeado a una herramienta.** Editor, terminal, agente y runtime son launchers configurables con presets por OS.
4. **El repo no ejecuta código sin permiso.** Config del repo con lista blanca; hooks del repo con aprobación por hash.
5. **El contrato es público.** Exit codes, `--json` versionado y `--dry-run` con lecturas reales son parte de la API, no cosmética.

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
| `wt create <name> [-b <branch>] [--base <ref>] [-x <cmd>] [--no-cd] [--detach]` | Crea el worktree, corre `post-create`, te deja parado adentro. Con `-x` corre el comando ahí, en foreground. |
| `wt cd [<name>\|-\|^\|@]` | Salta a un worktree, a un repo, al anterior (`-`), al principal (`^`) o al actual (`@`). Sin argumento, abre el picker. |
| `wt list [--full] [--all-repos] [--json]` | Una sola tabla con todo el estado. Reemplaza al `status` de v0.9. |
| `wt remove <name> [--delete-branch] [--force]` | Borra el worktree, corre `pre-remove`/`post-remove`. |
| `wt merge [<target>] [--squash] [--no-rebase] [--no-remove]` | Cierra el ciclo: commit pendiente → rebase → `pre-merge` → fast-forward → remove. |
| `wt sync [<name>...] [--all]` | Rebase de uno o varios worktrees sobre su base. Nunca stashea; el conflicto se reporta y se sigue. |
| `wt path <name>` | Imprime la ruta. Para scripts. |
| `wt lock <name> [reason]` / `wt unlock <name>` / `wt prune` | Mantenimiento. |

### Workspace (el diferencial)

| Comando | Qué hace |
|---|---|
| `wt repos [--json]` | Lista los repos descubiertos bajo las raíces configuradas. |
| `wt cd <repo>` | Salta al repo principal. Mismo verbo que para worktrees; la resolución decide. |
| `wt each [--all] [--repo <r>] -- <cmd>` | Corre un comando en cada worktree. |
| `wt exec <name> -- <cmd>` | Corre un comando en uno, sin moverte. |

### Extensión y estado

| Comando | Qué hace |
|---|---|
| `wt mark <name> <text>` / `wt var <name> <k>=<v>` | Estado por rama, guardado en `git config`. Visible en `list` y en plantillas. |
| `wt hook list` / `wt hook approve` | Aprobación de hooks declarados por el repo. |
| `wt config get\|set\|list\|path` | Config. |
| `wt shell init <zsh\|bash\|fish\|pwsh>` / `wt shell install` | Shell integration y completions. |
| `wt doctor` | Diagnóstico derivado de la config. No chequea nada que no esté configurado. |
| `wt dash` | Tablero de worktrees y agentes (TUI). |
| `wt <alias>` / `wt-<x>` en el PATH | Aliases de config y subcomandos externos, patrón git. |

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

Con `-x`, el exit code del comando ejecutado se propaga tal cual y desplaza a estos.

### Lo que NO existe (y no va a existir)

`open`, `clean`, `status` como comando aparte, `editor`/`terminal` como claves de primer nivel, cualquier clave `warp*`, chequeos de Node/fnm/Copilot en `doctor`. Son las nueve cosas que el §7 del análisis marcó para quitar: en greenfield simplemente no nacen.

---

## 5. Config

Capas, de menor a mayor precedencia: **defaults → usuario → repo (lista blanca) → env `WT_*` → flags**.

```
   macOS/Linux   $XDG_CONFIG_HOME/wt/config.toml   (default ~/.config/wt/config.toml)
   Windows       %APPDATA%\wt\config.toml
   Repo          .wt.toml en la raiz  -> SOLO claves de la lista blanca
   Env           WT_DEFAULT_BASE, WT_REPOS_ROOT, ...
```

```toml
worktree_path       = "{repo_parent}/{repo}.worktrees/{branch|sanitize}"
repos_root          = ["~/Repos", "~/Documents/GIT"]
repos_depth         = 2
default_base        = "origin/main"
branch_prefix       = ""
fetch_before_create = true
create_cd           = true          # create termina parado en el worktree
agent               = "copilot"     # lo que corre -x sin argumento

[launchers.editor]
preset = "code"                     # o command = "nvim {path}"

[launchers.terminal]
preset = "tmux"                     # solo se usa con --detach

[hooks]
post-create = ["cp .env {path}/.env", "npm ci"]
pre-merge   = ["npm test"]

[aliases]
dev = "npm run dev -- --port {branch|hash_port}"
```

**Lista blanca del repo** (`.wt.toml`): `default_base`, `branch_prefix`, `fetch_before_create`, `worktree_path`, `hooks` (con aprobación). Nunca `launchers`, `agent` ni rutas absolutas del usuario.

### Plantillas

Un solo motor, usado por rutas, hooks, launchers, aliases y columnas de `list`.

| Variables | `{repo}` `{repo_parent}` `{repo_path}` `{branch}` `{name}` `{path}` `{base}` `{default_branch}` `{upstream}` `{owner}` `{args}` `{vars.X}` |
|---|---|
| Filtros | `sanitize` (`feature/x` → `feature-x`) · `hash_port` (puerto determinístico 10000-19999 por rama) · `codename(n)` · `lower` |

`sanitize` no es un lujo: sin él, una rama como `feature/abc1` no tiene ruta válida.

### Hooks

Eventos: `pre`/`post` × `create`, `cd`, `remove`, `merge`, `commit`. Los `pre-*` bloquean (exit ≠ 0 aborta la operación); los `post-*` corren en background con la salida en `.git/wt/logs/`.

Los hooks declarados en `.wt.toml` requieren **aprobación explícita** la primera vez, guardada por hash del comando en la config del usuario. Si el comando cambia, se vuelve a pedir. Eso habilita "el equipo comparte `npm ci` al crear" sin darle ejecución arbitraria a un repo clonado.

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

Veinte capabilities. Cada una responde una pregunta, y ese es el criterio para saber si una spec está completa.

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
| `sync-worktrees` | ¿Cómo pongo N worktrees al día sin stashear nada? |
| `workspace-discovery` | ¿Qué repos existen bajo mis raíces? |
| `cross-repo-resolution` | Este nombre, ¿a qué repo pertenece? ¿Y si es ambiguo? |
| `batch-execution` | ¿Cómo corro algo en muchos worktrees y qué pasa si uno falla? |
| `hooks` | ¿Qué corre automáticamente y cuándo? |
| `project-hooks-approval` | ¿Cómo permito hooks del repo sin confiar en el repo? |
| `launchers` | ¿Cómo abro una app externa, cuando lo pido explícitamente? |
| `branch-state` | ¿En qué rama está trabajando quién/qué? |
| `aliases-plugins` | ¿Cómo extiendo `wt` sin tocar su código? |
| `dashboard` | ¿Cómo veo N worktrees con agentes, en vivo? |

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
       hooks, sync-worktrees, merge-worktree
         |
  M4  RIQUEZA               objetivo: una sola vista dice todo
       list-worktrees (completo), branch-state, picker
         |
  M5  EXTENSION             objetivo: adaptable sin tocar el codigo
       launchers, aliases-plugins, project-hooks-approval, doctor
         |
  M6  PRODUCTO              objetivo: que lo instale alguien mas
       dashboard, distribution, docs-generation, forge, llm-commits
```

### M1, desglosado en changes

Los cuatro cortes de M1 se abren ahora. M2 en adelante se desglosa cuando le toca: un proposal escrito hoy para M5 va a estar mal, porque M1 va a enseñar cosas.

| Change | Entrega | Por qué en ese orden |
|---|---|---|
| `walking-skeleton` | `wt list` del repo actual. Contrato de CLI, exit codes, `--json`, config mínima, porcelain, CI en 3 OSes. | Prueba el vertical completo end-to-end antes de acumular superficie. |
| `shell-integration` | Archivo-directiva, `wt shell init` para 4 shells, `wt cd <name>`. | Es el mecanismo con más riesgo multiplataforma del proyecto. Lo querés roto en la semana 2, no en el mes 5. |
| `create-worktree` | Motor de plantillas con `sanitize`, `wt create` que te deja adentro, `-x` en foreground. | Sin `sanitize` no hay ruta para una rama con barra. |
| `remove-worktree` | `remove`, `lock`/`unlock`, `prune`. | Cierra el ciclo mínimo: ya podés dejar v0.9 en macOS. |

### Definición de "listo" por milestone

Un milestone no está cerrado hasta que: los tests pasan en la matrix de 3 OSes, `wt doctor` no reporta nada roto, las specs están sincronizadas a `openspec/specs/`, y **lo usaste una semana en macOS sin volver a v0.9**.

---

## 8. Non-goals de v1

- Portar código de v0.9. Se lee como referencia; no se traduce.
- Soportar PowerShell 5.1 o Windows sin PowerShell 7.
- Reemplazar a git. `wt` orquesta git, no lo reimplementa (sin libgit2).
- Colaboración multi-usuario, sincronización de estado entre máquinas, o servicio remoto.
- Integración con forges (PR, CI) antes de M6. Es caro de mantener y opcional.
- Soporte de nushell en v1.

## 9. Decisiones abiertas

| # | Decisión | Recomendación | Bloquea |
|---|---|---|---|
| D1 | Formato de config: TOML o JSON | **TOML**: admite comentarios, y una herramienta con config rica los necesita. v0.9 usaba JSON, pero en greenfield no hay compatibilidad que preservar. | `config-layers` (M1) |
| D2 | ¿`wt list` muestra todos los repos por defecto, o solo el actual? | Solo el actual, con `--all-repos` para el workspace. Menos sorpresa y más rápido. | `list-worktrees` (M1) |
| D3 | Nombre del directorio de worktrees por defecto | `{repo_parent}/{repo}.worktrees/{branch\|sanitize}`, igual que v0.9. Funciona y ya tenés la memoria muscular. | `path-templates` (M1) |
| D4 | ¿`wt` gana sobre `wt.exe` en Windows, o se instala como `wtm`? | La función de shell gana; es lo que ya pasa en v0.9 sin fricción. Costo residual: tipear `wt.exe` para Windows Terminal. | `shell-integration` (M1) |

## 10. Riesgos

| Riesgo | Mitigación |
|---|---|
| La shell integration se comporta distinto en cada shell y se vuelve un pozo de bugs | Está en M1 a propósito, con tests por shell real en CI. Si va a fallar, que falle temprano. |
| El rewrite nunca cruza el umbral de "ya es mejor, me cambio" y muere al 70% | M1 tiene como criterio de cierre "una semana sin volver a v0.9". El umbral es explícito y temprano. |
| Windows se verifica poco porque el desarrollo es en macOS | Matrix de CI en los 3 OSes desde el primer change; las tareas que requieren verificación manual en Windows se marcan como tales. |
| El alcance crece hacia worktrunk y nunca termina | M6 es opcional. El diferencial está en M2, no en M4-M6. |
