# Worktree Manager vs Worktrunk: análisis comparativo y hoja de ruta

> **Nota (2026-10-01)**: el tag `v0.9.0` que menciona este documento se renombró a `powershell-v0.9.0`, y la rama `v0.9.x` se eliminó: el tag apunta al mismo commit. Ver [0002-versionado-y-releases.md](0002-versionado-y-releases.md).

> Documento de exploración (no de implementación). Fecha: 2026-09-17.
> Fuentes: el código de este repo (`src/`, `tests/`, `README.md`, `CHANGELOG.md`),
> el repo [max-sixty/worktrunk](https://github.com/max-sixty/worktrunk) y su
> documentación en [worktrunk.dev](https://worktrunk.dev/). Los datos de worktrunk
> salen de su documentación pública, no de haber corrido el binario.

---

## 0. Resumen ejecutivo

1. **Worktrunk resolvió un problema más chico con más profundidad.** Tiene solo
   cuatro comandos centrales (`switch`, `list`, `merge`, `remove`) pero cada uno
   está muy pulido: picker interactivo, símbolos de estado, pipeline de merge,
   hooks de ciclo de vida completo, plantillas con filtros, estado por rama,
   integración con GitHub/GitLab. Tu producto tiene **más comandos** (19) pero
   varios son delgados y todos están anclados a Windows + Warp + VS Code + Copilot.
2. **Lo que worktrunk tiene y vos no**, en orden de impacto: `wt merge` (cerrar el
   ciclo del worktree), hooks de ciclo de vida completos con modelo de aprobación
   para hooks del proyecto, plantillas con variables y filtros (`hash_port`,
   `sanitize`), `wt list` con estado rico en una sola vista, picker interactivo,
   shell integration para bash/zsh/fish/nushell/pwsh, aliases y subcomandos
   externos, estado por rama (markers, vars), integración con forges (PR/CI),
   commits generados por LLM, y distribución como binario (brew/winget/cargo).
3. **Lo que vos tenés y worktrunk no**: workspace multi-repo (`reposRoot`,
   `wt repos`, `wt cd`), consola tipo tablero, `wt each`/`wt exec`, `wt sync`
   multi-worktree como comando de primera clase, `wt doctor`, `--dry-run` global,
   `lock`/`unlock` y una lista blanca de seguridad para `.wt.json`. Eso es tu
   diferencial real; no lo pierdas al reformar.
4. **Para ir a terminal-first y sacar VS Code**: el cambio no es "borrar una línea",
   es invertir el modelo. Hoy `create` y `open` giran alrededor de *abrir cosas
   afuera* (editor, tab de Warp). Terminal-first significa que el comando por defecto
   **te deja parado en el worktree** (o corre el agente ahí mismo) y abrir un editor
   o un tab es un extra explícito y configurable.
5. **Para mac + Windows** hay dos caminos serios: (A) subir el piso a PowerShell 7 y
   portar el ~15% del código que es Windows-específico, con wrappers de shell
   integration para zsh/bash/fish; (B) reescribir en Go como binario único.
   **Recomiendo A como fase 1** (reusa ~85% del código y toda la suite de tests) y
   decidir B solo si el producto sale de tu órbita personal/equipo.
6. **Personalización**: reemplazar las claves `warp*`, `editor` y `terminal` por un
   modelo genérico de **launchers** (comandos-plantilla con `{path}`, `{name}`,
   `{branch}`, `{repo}`), y convertir Warp, Windows Terminal, tmux, zellij, kitty,
   WezTerm o iTerm2 en presets opcionales de ese modelo.

---

## 1. Los dos productos en una tabla

| Dimensión | Worktree Manager (este repo, v0.9.0) | Worktrunk |
|---|---|---|
| Lenguaje | PowerShell 5.1+ (módulo `wt.psm1`, ~4.2k líneas en `src/`, ~1.7k de tests) | Rust (binario único) |
| Plataformas | Windows (PowerShell 5.1 / 7) | macOS, Linux, Windows |
| Instalación | `install.ps1` (bloque en `$PROFILE`) | brew, cargo, winget, pacman, conda/pixi; firmado con SignPath |
| Shells | Solo PowerShell (la función `wt` vive en el perfil) | bash, zsh, fish, nushell, PowerShell (`wt config shell install`) |
| Comandos | 19: create, list, open, path, remove, lock, unlock, prune, clean, status, exec, each, sync, repos, cd, config, doctor, console, version | switch, list, merge, remove, config, step, hook (+ aliases y `wt-<x>` externos) |
| Direccionamiento | Por nombre de carpeta **o** de rama; ruta de `worktreeRootTemplate` con `{repoParent} {repo} {name}` | Solo por rama; ruta de `worktree-path` con `{{ repo }}`, `{{ branch \| sanitize }}`, `{{ branch \| codename(2) }}`, etc. |
| Config | JSON: `~/.wt/config.json` + `.wt.json` del repo (lista blanca) + `WT_CONFIG` | TOML: user `~/.config/worktrunk/config.toml` + project `.config/wt.toml` + system; `WORKTRUNK_*` env; `--config-set`; overrides por proyecto con wildcards |
| Hooks | `copyOnCreate` + `postCreate` (solo globales, nunca del repo) | pre/post × switch, start, commit, merge, remove; string / tabla concurrente / pipeline; hooks de proyecto con **aprobación** guardada; post-* en background con logs |
| Ciclo de vida | Crear, abrir, listar, status, sync, remove | Crear, switch, **merge** (commit → squash → rebase → hooks → ff → remove), remove |
| Interactividad | `wt console` (menú/tablero en loop) | Picker con preview en vivo (diff, log, PR) al invocar `switch` sin args |
| Estado | `wt status`: staged/modified/untracked, ahead/behind upstream o `defaultBase`, último commit | `wt list`: 17 símbolos (dirty, conflicto, op en curso, detached, integrado, diverged local y remoto), CI, resumen LLM, columnas custom, render progresivo |
| Multi-repo | `reposRoot` (varias raíces, profundidad), `wt repos`, `wt cd <repo\|worktree>` | No (opera sobre el repo actual) |
| Lote | `wt each`, `wt exec`, `wt sync` | Solo vía alias/recetas ("Rebasing every worktree onto its upstream") |
| Agente | `agentCommand` (default `copilot`) en tab de Warp o Windows Terminal | `wt switch -x <cmd>` corre el agente en el **mismo terminal**; plugins para Claude Code, Codex, OpenCode, Gemini; markers 🤖/💬 en `list` |
| Editor | `editor` (default `code`), se abre solo por defecto | No abre nada por defecto; el editor es responsabilidad del usuario o de un hook |
| Forge | No | GitHub, GitLab, Gitea, Azure DevOps: `switch pr:123`, CI en `list --full` |
| LLM | No | Mensajes de commit y resúmenes de rama con cualquier CLI (claude, codex, llm, aichat) |
| Seguridad | Lista blanca de claves en `.wt.json` (nada ejecutable viene del repo) | Aprobación explícita por comando de proyecto (`approvals.toml`), re-aprobación si cambia |
| Dry-run | `--dry-run` global, con lecturas reales | No documentado como global |
| Tests / CI | Dos runners propios (~525 aserciones), CI en `windows-latest` × {powershell, pwsh}, PSScriptAnalyzer | Cargo tests + tests de integración por shell (bash, zsh, fish, nu, pwsh), Codecov |

---

## 2. Qué tiene worktrunk que no tiene el tuyo (en detalle)

### 2.1 `switch`: un solo comando para "llevame ahí"

```
   worktrunk                              tu wt hoy
   ---------------------------            -------------------------------------
   wt switch feat        (cd)             wt cd feat          (cd, solo en PS)
   wt switch -c feat     (create + cd)    wt create feat      (create + abre VS Code + agente)
   wt switch -c feat -x claude            wt create feat --agent  (tab nuevo de Warp)
   wt switch -           (anterior)       --
   wt switch ^           (default branch) --
   wt switch pr:123      (checkout PR)    --
   wt switch             (picker)         wt console -> 2 -> nombre
```

La diferencia conceptual: en worktrunk **el resultado de crear es estar parado en
el worktree**, en la misma terminal. En el tuyo el resultado es "se abrió VS Code y
un tab de Warp"; tu terminal queda donde estaba. Eso es exactamente lo que querés
invertir.

### 2.2 `merge`: cerrar el ciclo

Ninguno de tus 19 comandos cierra un worktree "bien": hoy el usuario tiene que ir al
principal, mergear/rebasear a mano, volver, `wt remove --delete-branch`. Worktrunk
lo hace en un comando desde adentro del worktree:

```
   wt merge [target]
     1. pre-commit hooks + commit de lo pendiente
     2. squash (opcional)
     3. rebase sobre target
     4. pre-merge hooks (tests locales; si fallan aborta)
     5. fast-forward de target
     6. pre-remove hooks
     7. remove worktree + rama
     8. post-remove / post-merge en background
```

Con varios agentes en paralelo este es el comando que más tiempo ahorra por
worktree, y encaja perfecto con tu `wt sync` (que ya hace el rebase).

### 2.3 Hooks completos con modelo de aprobación

Vos tenés `copyOnCreate` y `postCreate`, y por seguridad **no** permitís que vengan
del repo. Worktrunk resuelve el mismo riesgo de otra forma: los hooks del proyecto
se permiten, pero **se piden aprobar la primera vez** y la aprobación se guarda por
comando (si el comando cambia, se vuelve a pedir). Eso desbloquea el caso de uso
"el equipo comparte `npm ci` al crear y `npm test` antes de mergear" sin abrir la
puerta a un repo hostil.

Eventos: `pre-switch/post-switch`, `pre-start/post-start`, `pre-commit/post-commit`,
`pre-merge/post-merge`, `pre-remove/post-remove`. Los `pre-*` bloquean; los `post-*`
corren en background con la salida en `.git/wt/logs/`.

### 2.4 Plantillas con variables y filtros

Tu `worktreeRootTemplate` tiene tres tokens fijos. Worktrunk usa un motor de
plantillas compartido por rutas, hooks, aliases y columnas:

| Variable / filtro | Para qué |
|---|---|
| `{{ branch \| sanitize }}` | `feature/x` -> `feature-x` (ruta segura) |
| `{{ branch \| hash_port }}` | Puerto determinístico 10000-19999 por rama (un dev server por worktree sin chocar) |
| `{{ branch \| codename(2) }}` | Nombre amigable determinístico |
| `{{ repo }} {{ owner }} {{ repo_path }} {{ default_branch }} {{ upstream }}` | Contexto del repo |
| `{{ vars.ticket }}` | Estado custom por rama |
| `{{ args }}` | Lo que sigue a `--` |

Esto es lo que hace posible un hook como
`docker run --name {{ repo }}-{{ branch | sanitize }}-db -p {{ branch | hash_port }}:5432 ...`.

### 2.5 `list` que reemplaza a `status`

Tu `wt list` muestra nombre, rama, ruta y marcas (principal, obsoleto, bloqueado).
Tu `wt status` es otro comando, con otra tabla. En worktrunk es una sola vista:

```
   branch        status   HEAD  ahead/behind  diff      remote  commit   age    msg
   ^ main                                               |       a1b2c3d  2h     ...
   feature-a     +!?      ↑3    +120/-4                 ⇡       d4e5f6a  10m    ...
   feature-b     ✘        ↕                             ⇅       ...
   old-thing     _  ⊂                                                    (integrada: borrable)
```

Detalles que valen la pena copiar: marcar **integrada** (`⊂`) y **vacía** (`_`)
para saber qué se puede borrar; detectar **operación en curso** (`↻`, un rebase a
medio resolver) y **conflicto** (`✘`); render progresivo (primero lo local, después
lo remoto); columnas custom desde config; salida JSON con esquema versionado; modo
`statusline` de una línea para el prompt o para Claude Code.

### 2.6 Shell integration real

Tu `wt cd` funciona porque la función `wt` vive en el mismo proceso de PowerShell.
Eso no se traslada a zsh en mac. Worktrunk usa un mecanismo simple y portable:

```
   wrapper de shell (zsh/bash/fish/pwsh)
     1. crea un archivo temporal
     2. exporta WORKTRUNK_DIRECTIVE_CD_FILE=<tmp>
     3. corre el binario
     4. si el archivo tiene contenido -> cd a esa ruta
     5. borra el temporal
```

Es exactamente lo que necesitás para que un `wt` escrito en PowerShell 7 pueda
cambiar el directorio de un zsh en mac.

### 2.7 Aliases y subcomandos externos

`[aliases] deploy = "make deploy BRANCH={{ branch }}"` y cualquier ejecutable
`wt-<nombre>` en el PATH aparece como `wt <nombre>` (patrón de git). Es la forma
más barata de que el producto sea "personalizable" sin agregar claves de config.

### 2.8 Estado por rama (markers y vars)

`wt config state marker set "🤖 trabajando"` y `wt config state vars set ticket=ABC-123`
se guardan en `git config` del repo (`worktrunk.state.<branch>.*`) y aparecen en
`list` y en plantillas. Los plugins de agentes ponen 🤖/💬 solos. Para tu caso
(varios agentes en paralelo) es la respuesta a "¿en cuál está trabajando alguien?".

### 2.9 Forges, CI y PRs

`wt switch pr:123`, CI status coloreado en `list --full`, detección automática de
GitHub/GitLab/Gitea/Azure DevOps. Es opcional y caro de mantener; lo marco como
fase tardía.

### 2.10 Commits por LLM

`[commit.generation] command = "claude -p ..."` con `{{ git_diff }}` etc. Con
Copilot CLI se podría hacer lo mismo. Valor medio, esfuerzo bajo una vez que hay
plantillas y `merge`.

### 2.11 Distribución y ecosistema

Binario firmado, brew/winget/cargo/pacman/conda, plugins para cinco agentes,
skill `/worktrunk` para que el propio agente configure la herramienta, docs
navegables por comando. Nada de esto es código de producto, pero define si alguien
más lo instala.

---

## 3. Qué tenés vos que worktrunk no tiene

No es un análisis unilateral. Estas piezas son tu ventaja y deberían quedar
intactas o crecer:

| Capacidad | Por qué importa |
|---|---|
| **Workspace multi-repo** (`reposRoot` con varias raíces y profundidad, `wt repos`, `wt cd <repo\|worktree>`, resolución de nombres entre repos con detección de ambigüedad) | Worktrunk asume que ya estás dentro del repo. Para alguien que salta entre 10 repos, tu `wt cd api` es el comando más usado del día. |
| **`wt each` / `wt exec` / `wt sync` como comandos** | Worktrunk lo deja como receta de alias. Tu `sync` con "nunca stash automático, conflicto se reporta y se sigue" es una política correcta para agentes corriendo adentro. |
| **`wt console` como tablero** | Worktrunk tiene picker, no tablero. Un modo "vista de sala de control" de N worktrees con agentes es un diferencial claro, si se reenfoca (ver 9.6). |
| **`--dry-run` global con lecturas reales** | Muy bueno para agentes que operan la herramienta: ven el plan sin riesgo. |
| **`wt doctor`** | Worktrunk tiene `config show --full`; el tuyo es más didáctico. Hay que sacarle lo hardcodeado (Node, fnm, Copilot, Warp). |
| **Lista blanca de `.wt.json`** | Modelo de seguridad simple y explicable. Se puede combinar con el de aprobación (ver 3.4 del plan). |
| **`lock`/`unlock` como comando** | Worktrunk no los expone. |
| **Validación de flags estricta + códigos de salida documentados** | Igual de bueno que worktrunk. |
| **Arquitectura por capas con funciones puras** | Es lo que hace viable el port multiplataforma sin reescribir: la decisión (`Get-WtOpenPlan`, `Get-WtSyncPlan`, `ConvertFrom-WtArgs`, parsers de porcelain) está separada del efecto. |

---

## 4. Diagnóstico honesto del producto actual

### 4.1 Acoplamientos que hay que romper

```
   +----------------------------------------------------------------------+
   |                     wt (hoy)                                         |
   +----------------------------------------------------------------------+
   |  Windows                Warp                  VS Code     Copilot    |
   |  -------                ----                  -------     -------    |
   |  cmd.exe /c (exec,      tab_configs TOML      editor=     agent=     |
   |   each, postCreate)      en %APPDATA%          'code'      'copilot' |
   |  '\' forzado en          warp:// URI           abre por    Node>=18  |
   |   ConvertTo-WtFullPath   Get-Process warp      defecto     + fnm en  |
   |  wt.exe (Win Terminal)   warpPath, warp*Color  en create   doctor y  |
   |  CON/NUL/PRN             warpAgentTarget       y open      en el tab |
   |  ~\.wt\config.json       wt clean                                    |
   |  notepad.exe             Remove-...TabConfigs                        |
   |  powershell -NoExit                                                  |
   |  $PROFILE.CurrentUserAllHosts                                        |
   +----------------------------------------------------------------------+
```

Concentrado en: `src/Launch.ps1` (casi entero), `Open-WtTerminal` / `Open-WtAgent` /
`Invoke-WtCommandLine` / `Get-WtDoctorRows` en `src/Commands.ps1`,
`Get-WtDefaultConfig` / `Test-WtConfigValue` / `Invoke-WtConfigCommand` en
`src/Config.ps1`, `ConvertTo-WtFullPath` / `Test-WtWorktreeName` en `src/Common.ps1`,
y `install.ps1`. `Repo.ps1`, `Workspace.ps1`, `Console.ps1` y `Completion.ps1`
están prácticamente limpios: ese es el ~85% reutilizable.

### 4.2 Decisiones de producto que hoy van en contra de "terminal-first"

- `openOnCreate = 'all'` por defecto: crear un worktree abre VS Code **y** un tab
  con el agente. El README lo promete explícitamente.
- `wt open` sin flags abre el editor. El nombre `open` en sí es "abrir algo afuera".
- El agente **nunca** corre en la terminal actual; siempre en un tab nuevo de Warp
  o Windows Terminal. Worktrunk hace lo opuesto (`-x` en foreground) y deja el tab
  nuevo como patrón de tmux/hook.
- El `doctor` verifica Node ≥ 18 y fnm aunque `agentCommand` no sea Copilot.
- `wt clean` existe solo para limpiar archivos de Warp.

### 4.3 Limitaciones técnicas

- **PowerShell 5.1 como piso** impide `ForEach-Object -Parallel`, `??`, ternarios,
  `Join-Path` con más de dos segmentos, `[IO.Path]` portables, y obliga al hack
  `2>&1` para separar stderr. Todo eso está justificado en tus comentarios; el
  costo ya lo estás pagando.
- **Latencia**: cada `wt` en PowerShell dispara `git worktree list` y varios
  `rev-parse`. En pwsh 7 eso mejora poco; en un binario, mucho. Mitigación barata:
  cachear el porcelain por repo con invalidación por mtime de `.git/worktrees`.
- **Nombre `wt`** colisiona con `wt.exe` de Windows Terminal. Hoy se salva porque la
  función del perfil gana. Worktrunk instala como `git-wt` en Windows por esto.
- **Tests sin framework** (dos runners propios, sin Pester). Sirven, pero un port
  multiplataforma los va a estresar (rutas, separadores, `cmd.exe`).

---

## 5. Decisión estratégica: cómo llegar a mac + Windows

```
                      esfuerzo
                         ^
        (B) Go/Rust      |   xxxxxxxx  binario unico, arranque instantaneo,
        reescritura      |   x      x  brew/winget/scoop, shell integ nativa
                         |   xxxxxxxx  pierde 100% del codigo, tests desde cero
                         |
        (C) hibrido      |   xxxx      dos codebases, peor de los dos mundos
                         |
        (A) pwsh 7       |   xx        reusa ~85% + tests, portar ~15%,
        + wrappers       |             requiere `pwsh` instalado en mac
                         +------------------------------------------> alcance
```

| Opción | Qué implica | Costo | Riesgo |
|---|---|---|---|
| **A. PowerShell 7 multiplataforma** | Subir `PowerShellVersion` a 7.x, portar los puntos de 4.1, XDG config, `sh -c` en unix, wrappers `wt()` para zsh/bash/fish con archivo-directiva, CI en macOS/Linux | Bajo-medio | En mac, el usuario debe instalar `pwsh` (`brew install powershell`). Arranque ~150-300 ms por invocación. |
| **B. Reescribir en Go** (o Rust) | Un binario, `cobra` + `bubbletea` para la consola, cross-compile trivial, `scoop`/`brew` | Alto (todo de nuevo, incluidos ~525 asserts) | Terminás construyendo un worktrunk con workspace multi-repo. ¿Vale la pena competir en el mismo eje? |
| **C. Core en Go + wrapper PS** | Mantener el módulo como fachada | Alto | Dos codebases para mantener. Descartar. |

**Recomendación: A ahora, B solo si cambia el alcance.** Razones:

1. Tu arquitectura ya separó decisión de efecto; el port es mecánico, no de diseño.
2. Tu diferencial (workspace, each/sync, consola, dry-run) no necesita Rust para
   existir. Lo que necesita es dejar de asumir Windows.
3. Si en seis meses querés distribuirlo públicamente, la fase A ya te dio el
   modelo de config, los launchers y la semántica de comandos; el rewrite sería
   "traducir", no "diseñar".

Condición para reconsiderar: si tus usuarios mac no van a instalar `pwsh`, A no
sirve y hay que ir a B directo.

---

## 6. Plan de mejoras priorizado

Etiquetas: **[Q]** = quitar/invertir (pedido explícito), **[X]** = multiplataforma,
**[P]** = personalización, **[W]** = tomado de worktrunk, **[E]** = eficiencia.

### Fase 1: terminal-first y desacople (sin cambiar de plataforma todavía)

| # | Cambio | Etiq. | Valor | Esfuerzo |
|---|---|---|---|---|
| 1.1 | **`openOnCreate` default `none`** y `wt create` termina con `cd` al worktree (respetando `--no-cd`). El editor solo con `--code`/`--editor`. | Q | Alto | Bajo |
| 1.2 | **Quitar la apertura automática de VS Code de `wt open`**: `open` sin flags deja de abrir el editor. Mejor: **deprecar `open`** y mover sus flags a `wt cd --editor`, `wt cd --terminal`, `wt cd --agent`. | Q | Alto | Bajo |
| 1.3 | **Agente en foreground**: `wt create feat -x copilot` / `wt cd feat -x copilot` corre el comando en la terminal actual, dentro del worktree, y devuelve su exit code. El tab nuevo queda como `--detach` o como launcher. | Q, W | Alto | Bajo-medio |
| 1.4 | **Modelo de launchers**: reemplazar `editor`, `terminal`, `warp*`, `agentCommand`, `agentShell` por una sección `launchers` de comandos-plantilla con `{path} {name} {branch} {repo} {title} {agent}`; presets embebidos para `warp`, `windows-terminal`, `tmux`, `zellij`, `kitty`, `wezterm`, `iterm2`, `code`, `cursor`, `nvim`, `none`. | P, Q | Alto | Medio |
| 1.5 | `wt doctor` deriva sus chequeos de la config (agente configurado, launchers configurados) y deja de asumir Node/fnm/Copilot/Warp. | Q | Medio | Bajo |
| 1.6 | `wt clean` y el ciclo de vida de tab configs de Warp pasan al preset `warp` (se ejecutan solo si ese launcher está activo). | Q | Bajo | Bajo |
| 1.7 | Renombrar `postCreate`/`copyOnCreate` a un bloque `hooks` con `pre-create`, `post-create`, `pre-remove`, `post-remove`, `post-cd` (manteniendo compatibilidad un release). | W | Medio | Bajo |

Sketch de la config resultante (JSON, para no cambiar de formato en esta fase):

```jsonc
{
  "worktreeRootTemplate": "{repoParent}/{repo}.worktrees/{name}",
  "reposRoot": ["~/Repos"],
  "defaultBase": "origin/develop",
  "branchPrefix": "",
  "createCd": true,                 // create termina parado en el worktree
  "openOnCreate": "none",           // none | editor | agent | editor+agent
  "agent": "copilot",               // comando; se corre en foreground con -x
  "launchers": {
    "editor":   { "preset": "code" },                    // o { "command": "nvim {path}" }
    "terminal": { "preset": "tmux" },                    // warp | windows-terminal | tmux | zellij | kitty | wezterm | iterm2 | none
    "agentTab": { "preset": "warp", "color": "green" }   // solo se usa con --detach
  },
  "hooks": {
    "post-create": ["cp .env {path}/.env", "npm ci"],
    "pre-remove":  []
  },
  "aliases": {
    "test": "npm test",
    "dev":  "npm run dev -- --port {hash_port}"
  }
}
```

### Fase 2: multiplataforma (PowerShell 7)

| # | Cambio | Etiq. | Esfuerzo |
|---|---|---|---|
| 2.1 | `PowerShellVersion = '7.2'`; borrar los workarounds de 5.1; CI matrix `windows-latest`, `macos-latest`, `ubuntu-latest` con `pwsh`. | X | Bajo |
| 2.2 | `ConvertTo-WtFullPath` deja de forzar `\`; usar `[IO.Path]::DirectorySeparatorChar` y `Join-Path`; comparación case-insensitive solo en Windows/macOS (APFS es case-insensitive por defecto, ext4 no). | X | Medio |
| 2.3 | Config en `$XDG_CONFIG_HOME/wt/config.json` (unix) y `%APPDATA%\wt\config.json` (Windows), con fallback a `~/.wt/config.json` y aviso de migración. | X | Bajo |
| 2.4 | `Invoke-WtCommandLine`: `cmd.exe /c` en Windows, `sh -c` en unix (o `pwsh -c` si el usuario lo pide en `hooks.shell`). | X | Bajo |
| 2.5 | `Test-WtWorktreeName`: nombres reservados (`CON`, `NUL`) solo en Windows; en unix rechazar solo `/` y nulos. | X | Bajo |
| 2.6 | **Shell integration**: `wt shell init zsh\|bash\|fish\|pwsh` imprime la función wrapper (mecanismo archivo-directiva de 2.6 arriba) y `wt shell install` la agrega al rc. Reemplaza `install.ps1` como camino principal; `install.ps1` queda para Windows. | X, W | Medio |
| 2.7 | Autocompletado: además de `Register-ArgumentCompleter`, generar scripts de completion para zsh/bash/fish a partir de `Get-WtCommandSpecs` (misma fuente de verdad). | X | Medio |
| 2.8 | Presets de launchers por OS: `open -a Warp`, `open -a "Visual Studio Code"`, `osascript` para iTerm2, `tmux new-window -c {path}` en cualquier unix. Verificar en mac dónde guarda Warp sus tab configs antes de portar el preset. | X, P | Medio |
| 2.9 | Nombre del comando: mantener `wt` en unix; en Windows documentar la colisión con `wt.exe` y ofrecer `wtm` como alias principal si el usuario lo prefiere. | X | Bajo |
| 2.10 | Tests: parametrizar los E2E por OS (rutas, separadores, shell) y correrlos en la matrix. | X | Medio |

### Fase 3: eficiencia y capacidades tomadas de worktrunk

| # | Cambio | Etiq. | Valor | Esfuerzo |
|---|---|---|---|---|
| 3.1 | **`wt merge`**: desde el worktree, commit pendiente (opcional) → rebase sobre base (reusa `Get-WtSyncPlan`) → `pre-merge` hooks → ff de la base → `wt remove --delete-branch`. Flags `--no-remove`, `--no-rebase`, `--squash`. | W, E | Alto | Medio |
| 3.2 | **Fusionar `status` en `list`**: una sola tabla con símbolos (dirty, ahead/behind, integrada, conflicto, op en curso, remoto), calculada en paralelo con `ForEach-Object -Parallel`; `wt status` queda como alias de `list --full`. | W, E | Alto | Medio |
| 3.3 | **Plantillas con filtros** en rutas, hooks, launchers y aliases: `{branch\|sanitize}`, `{hash_port}`, `{repo}`, `{base}`, `{args}`. | W, P | Alto | Medio |
| 3.4 | **Hooks del proyecto con aprobación**: `.wt.json` puede declarar `hooks`, pero cada comando se aprueba la primera vez y la aprobación (hash del comando) se guarda en la config del usuario. La lista blanca actual sigue para el resto de claves. | W | Medio | Medio |
| 3.5 | **Picker**: `wt cd` / `wt create` sin nombre abren un selector; usar `fzf` si está en PATH (con preview de `git log`/`diff`), fallback a la lista numerada de la consola. | W | Medio | Bajo |
| 3.6 | **Aliases** en config y **subcomandos externos** `wt-<nombre>` en PATH. | W, P | Medio | Bajo |
| 3.7 | **Atajos de nombre**: `-` (worktree anterior), `^` (principal), `@` (actual). | W | Bajo | Bajo |
| 3.8 | **Estado por rama**: `wt mark <name> "🤖 refactor"` y `wt var <name> ticket=ABC` guardados en `git config`; se muestran en `list` y están disponibles en plantillas. | W | Medio | Bajo |
| 3.9 | **Caché del porcelain** por repo con invalidación por mtime de `.git/worktrees/` para bajar la latencia de cada invocación y del autocompletado. | E | Medio | Bajo |
| 3.10 | `--json` con esquema versionado en **todos** los comandos y variables `WT_<CLAVE>` para override de config. | W, E | Medio | Bajo |

### Fase 4: producto (opcional, si sale de uso personal)

- `wt create pr:123` vía `gh`/`glab` y CI status en `list --full`.
- Mensajes de commit por LLM para `merge --squash` (usa el `agent` configurado).
- Skill/plugin para que el agente (Copilot, Claude Code) configure y use `wt`.
- Distribución: PSGallery, `scoop`, `brew tap`; releases con changelog automatizado.
- README en inglés + docs por comando.

---

## 7. Lista explícita de features a quitar o invertir

| Feature hoy | Qué pasa con ella |
|---|---|
| `wt create` abre VS Code y agente por defecto (`openOnCreate = 'all'`) | **Se quita.** Default `none`; `create` hace `cd`. |
| `wt open` sin flags abre el editor | **Se quita.** `open` se deprecia a favor de `cd --editor`. |
| `editor = 'code'` como default | Pasa a `launchers.editor = none`; `code` es un preset que el usuario elige. |
| `terminal = 'warp'` como default | Pasa a `launchers.terminal = none`; Warp es un preset. |
| Agente siempre en tab nuevo (Warp/Windows Terminal) | **Se invierte.** Foreground por defecto (`-x`), tab nuevo con `--detach`. |
| `warpAgentTarget`, `warpAgentColor`, `warpTerminalColor`, `warpPath` | Se mueven adentro del preset `warp`. Desaparecen del nivel raíz. |
| `agentShell` (`powershell`/`bash`/`none`) | Se reemplaza por `hooks.shell` y por el shell del launcher. |
| `wt clean` (tab configs de Warp) | Solo existe si el preset `warp` está activo; si no, el comando avisa que no aplica. |
| Chequeos de Node ≥ 18 / fnm / Copilot en `doctor` | Se derivan del `agent` configurado (si es `copilot`, chequea Node; si es `claude`, no). |
| PowerShell 5.1 como piso | Se sube a 7.2 en fase 2. |

---

## 8. Cómo queda el flujo terminal-first (objetivo)

```
   $ wt cd api                       # workspace: salto al repo (tu diferencial)
   $ wt create auth -x copilot       # crea, hooks, cd, corre el agente ACA
     ... el agente trabaja, termina, vuelvo al prompt parado en el worktree ...
   $ wt list                         # una tabla: dirty, ahead, integrada, markers
   $ wt each -- npm test             # lote (tu diferencial)
   $ wt merge                        # rebase + tests + ff + remove: worktree cerrado
   $ wt cd -                         # vuelvo al anterior

   # opcionales, nunca por defecto:
   $ wt cd auth --editor             # abre el launcher 'editor' (code, cursor, nvim...)
   $ wt create x -x copilot --detach # el agente en un tab nuevo (warp, tmux, wezterm...)
   $ wt console                      # tablero de N worktrees con agentes
```

---

## 9. Riesgos y preguntas abiertas

1. **¿Reescribir o portar?** Mi recomendación es portar (PowerShell 7). Si tus
   usuarios mac no van a instalar `pwsh`, hay que ir a Go directo. Esto decide todo
   lo demás; conviene fijarlo antes de la fase 2.
2. **¿Mantener el nombre `wt`?** Colisiona con Windows Terminal en Windows y con
   worktrunk en cualquier máquina que lo tenga instalado. Alternativas: `wtm`
   (ya es alias) o un nombre nuevo.
3. **¿Hooks del proyecto con aprobación, o mantener la lista blanca estricta?** La
   aprobación desbloquea el caso de equipo; la lista blanca es más simple. Se
   pueden combinar (3.4).
4. **¿`wt open` se deprecia o se redefine?** Deprecar es más limpio; redefinir
   rompe menos hábitos. Propongo deprecar con aviso durante un release.
5. **¿Mantener PowerShell 5.1 en paralelo?** No: duplica el costo de cada cambio.
   La rama actual queda como `v0.9.x` congelada para Windows legacy.
6. **Consola**: hoy replica todos los comandos. En terminal-first tiene más sentido
   como **tablero de agentes** (refresco automático, markers, último commit) que
   como menú. Vale una conversación aparte.

---

## 10. Próximos pasos sugeridos

1. Fijar la decisión 1 (portar vs. reescribir) y la 2 (nombre).
2. Abrir un cambio OpenSpec por fase: `terminal-first-launchers` (fase 1),
   `cross-platform-pwsh7` (fase 2), `merge-and-rich-list` (fase 3). Cada uno con
   proposal, design y specs de las claves de config nuevas y de las que se quitan.
3. La fase 1 se puede empezar hoy sin tocar la plataforma y ya entrega los dos
   pedidos explícitos: no abrir VS Code y no depender de Warp.

---

## Fuentes

- Este repo: `README.md`, `CHANGELOG.md`, `src/*.ps1`, `tests/*.ps1`,
  `docs/historial/PLAN-MEJORAS.md`, `.github/workflows/ci.yml`.
- [github.com/max-sixty/worktrunk](https://github.com/max-sixty/worktrunk)
- [worktrunk.dev](https://worktrunk.dev/): [config](https://worktrunk.dev/config/),
  [switch](https://worktrunk.dev/switch/), [list](https://worktrunk.dev/list/),
  [merge](https://worktrunk.dev/merge/), [hook](https://worktrunk.dev/hook/),
  [shell-integration](https://worktrunk.dev/shell-integration/),
  [extending](https://worktrunk.dev/extending/),
  [tips-patterns](https://worktrunk.dev/tips-patterns/),
  [claude-code](https://worktrunk.dev/claude-code/), [faq](https://worktrunk.dev/faq/).
