# 0003 — Orquestación de sesiones

- **Fecha**: 2026-11-27
- **Estado**: aceptada
- **Antecedentes**: [0001-de-cero-en-go.md](0001-de-cero-en-go.md), [0002-versionado-y-releases.md](0002-versionado-y-releases.md), [`docs/design/product.md`](../design/product.md)

## Contexto

`wt` resuelve el ciclo de vida de un worktree: crearlo, saltar a él, verlo en una tabla, cerrarlo. Lo que sigue a mano es todo lo que pasa *alrededor* del worktree cuando trabajás con agentes:

- abrir el agente correcto en el worktree correcto, y a veces en otra terminal para no perder la tuya;
- retomar al día siguiente: qué agente era, cómo se continuaba su conversación, qué faltaba hacer;
- no perder el objetivo, las decisiones y el siguiente paso de cada worktree cuando se cierra la terminal;
- saber qué agente está trabajando y cuál te está esperando;
- y, como objetivo de más largo plazo, que un agente líder abra, siga y cierre sesiones de otros agentes desde la terminal, como un "projects" local.

El roadmap ya tenía piezas sueltas de esto: `launchers` y la clave `agent` en M5, `agent-integrations` y `dashboard` en un M6 opcional. Lo que no tenía era el objeto que las une. Sin él, cada pieza inventaría su propio estado, y los tres principios que hablan de abrir cosas afuera quedaban en contradicción con lo que se quiere construir:

- "Ningún comando abre una app externa por defecto".
- `open` en la lista de lo que no existe.
- `editor` y `terminal` prohibidos como claves de primer nivel.

Esta decisión fija ese objeto (la sesión), reescribe los principios sin romperlos y reordena el roadmap desde M4.

## Decisión

### 1. Una sesión es un worktree más su registro

Una **sesión** es un worktree con un registro declarativo al lado. Repo, rama y ruta salen de git, como hoy. Perfil, agente, terminal, editor, metadata y lo necesario para retomar salen del registro.

- **Relación 1:1 con el worktree en v1.** Las sesiones sin worktree (una investigación sobre varios repos) y las varias sesiones sobre un mismo worktree quedan afuera hasta que aparezca un caso real.
- **Identidad**: la que `wt` ya usa, `<repo>:<name>`, con la misma resolución y el mismo exit 4 por ambigüedad.
- **Sesión implícita**: un worktree sin registro es una sesión con el perfil por defecto. Nadie migra nada.
- **Dónde vive**: `<git-common-dir>/wt/sessions/<id>/`, junto a los logs de hooks que ya están en `<git-common-dir>/wt/logs/`. `<id>` es el id administrativo del worktree (el nombre de `.git/worktrees/<id>`, y `main` para el principal), que git mantiene estable aunque el worktree se mueva.
- **Qué hay adentro**: `session.toml` (esquema `wt.session.v1`), `context.md` (memoria de trabajo), `journal.md` (notas con fecha, solo se agregan) y `run.toml` mientras corre un agente (pid y hora de inicio, tratados como pista).
- **Git es la fuente de verdad.** No hay daemon ni proceso residente. Un índice global, si hace falta para ordenar por uso reciente, es una caché derivada y nunca una fuente de verdad. El registro se lee de forma tolerante, y `wt prune` reconcilia los registros de worktrees que alguien borró a mano.
- **Escrituras atómicas** (temporal más rename, con reemplazo explícito en Windows) para `session.toml` y `context.md`; el diario solo se agrega.

### 2. `wt mark` y `wt var` se guardan en el registro, por worktree

`branch-state` (M4) deja de guardar en `git config` por rama, como decía `product.md`. Guarda en `session.toml`, por worktree, y es la semilla del registro: un solo almacén desde el primer día, y nada que migrar cuando lleguen las sesiones en M5. `branch-state` crea solo lo que necesita (`session.toml` con el marcador y las variables); `sessions` le suma el resto de los campos sin cambiar el esquema ni la ubicación.

La consecuencia es que el estado sigue al worktree, no a la rama: si dos worktrees pasan por la misma rama, cada uno tiene el suyo, y al borrar el worktree el estado se archiva con él (punto 7).

### 3. Agentes como datos

Un agente es una tabla de config, `[agents.<nombre>]`, con `command`, `resume`, `prompt`, `context`, `env` y `detect`. Solo `command` es obligatorio; un agente sin `resume` se retoma arrancando de nuevo. `wt` trae presets embebidos que el usuario puede pisar, y fuera de ellos no sabe nada de ningún agente.

- `-x <cmd>` sigue siendo un comando crudo, exactamente como hoy.
- Se agrega `--agent <nombre>` (`-a`). La clave `agent` pasa a ser el nombre del agente por defecto, el que usan `--agent` sin nombre y `-x` sin argumento.
- Si una integración (M6) captura un id de conversación propio del agente, lo guarda opaco en `[agent_state]` del registro, y la plantilla de `resume` lo usa como `{agent_state.resume_ref}`.

### 4. Launchers: un directorio y un argv

Un launcher sabe hacer una sola cosa: abrir una terminal (o un editor) en un directorio y, si es una terminal, correr un argv. Ese argv es siempre `git-wt session run <repo>:<name>`: la terminal nueva vuelve a entrar por `wt`, que resuelve la sesión y lanza el agente en foreground. El launcher nunca ve el comando del agente, así que no hay comillas anidadas entre zsh, pwsh, AppleScript y `cmd.exe`.

Tres niveles, de menos a más poder:

| Nivel | Forma | Para |
|---|---|---|
| `preset` | Datos embebidos en el binario | Las terminales que `wt` trae |
| `command` | Plantilla en forma de argv (`{path}`, `{title}`, `{argv}`) | Una terminal o un editor que acepta argv |
| `plugin` | Un ejecutable `wt-launch-<x>` en el PATH que recibe un documento `wt.launch.v1` por stdin (`path`, `title`, `argv`, `env`, `session`) y dice con su exit code si abrió | Lo que necesita más que un argv |

**Presets de la primera versión**, los que se usan hoy:

| Preset | macOS | Windows |
|---|---|---|
| `warp` | Warp | Warp |
| `pwsh` | n/a | Una ventana nueva de PowerShell 7 |
| `terminal` | Terminal.app | n/a |

Agregar otra terminal (iTerm2, Windows Terminal como host propio, WezTerm, tmux, kitty) es un PR de datos o un plugin, nunca un cambio del núcleo. Los comandos exactos de cada preset se verifican al implementar `launchers`, con tareas **[macOS]** y **[Windows]** de verificación manual.

Los editores se abren con el mismo mecanismo (un launcher sin argv, que abre la carpeta y no corre el agente). Esta decisión no fija presets de editor: el change `launchers` decide si trae alguno o si alcanza con `command`.

### 5. Perfiles

Un perfil es un overlay de config con nombre, `[profiles.<nombre>]`, que se aplica entre la capa del repo y la del entorno:

```
defaults → usuario → repo (lista blanca) → perfil → WT_* → flags
```

- Solo pisa las claves que declara: un perfil de dos líneas es válido.
- `default_profile` elige el perfil de las sesiones nuevas. Sin `default_profile` ni flags, `wt` se comporta exactamente como hoy.
- `terminal` y `editor` viven **dentro de un perfil**, nunca en el nivel raíz.
- El registro guarda el **nombre** del perfil, no una copia. Si editás el perfil, las sesiones viejas lo heredan al retomarse; solo los flags explícitos de la creación quedan fijados en la sesión.
- Los hooks de un perfil se suman a los del usuario y del repo, como los del repo se suman a los del usuario.
- **`.wt.toml` no declara agentes, launchers ni perfiles, y en 1.0 tampoco sugiere perfiles.** Son ejecución de código en la máquina del usuario. Si algún día un repo quiere sugerir perfiles, solo podrá referenciar por nombre agentes y launchers del usuario, y sus hooks seguirán pidiendo aprobación.

### 6. Principios reescritos

Los principios 1 a 3 de `product.md` §2 y la regla sobre `open` de §4 quedan así:

1. **Terminal-first.** El resultado de un comando es dejarte parado en el worktree, en la misma terminal. Abrir una app externa (otra terminal, un editor) es siempre opt-in: un flag explícito (`--in <launcher>`, `--editor`) o un perfil del usuario que lo declara. Sin flag ni perfil, ningún comando abre nada.
2. **Los agentes corren en foreground.** `-x <cmd>` y `--agent` corren dentro del worktree, en tu terminal, y propagan el exit code. Cuando un launcher abre otra terminal, el agente corre en foreground ahí, lanzado por `wt` con `git-wt session run`; el launcher nunca ve su comando.
3. **Nada hardcodeado a una herramienta.** Agentes, terminales, editores y perfiles son datos de config, con presets por OS que se pueden pisar. Agregar uno nuevo no toca el código.

**El verbo no es `open`.** `open` sigue sin existir. Retomar una sesión es `wt resume`; abrirla en otra terminal es `--in <launcher>` sobre `wt create` o `wt resume`, y del lado de MCP se llama `launch_session`. `--detach`, que `product.md` reservaba para `wt create`, lo reemplaza `--in <launcher>`.

Los principios 4 ("el repo no ejecuta código sin permiso") y 5 ("el contrato es público") no cambian. El 5 suma dos contratos versionados: `wt.session.v1` (el registro y su salida `--json`) y `wt.launch.v1` (el protocolo de los plugins de launcher).

### 7. Retomar y cerrar

`wt resume [<name>] [--in <launcher>] [--fresh] [--no-agent]` hace, en orden, lo que hoy se hace a mano:

1. Resuelve la sesión por nombre o, sin nombre, abre el picker sobre las sesiones, ordenadas por último uso.
2. Te deja parado en el worktree, con la misma directiva y los mismos hooks `pre-cd`/`post-cd` de `wt cd`.
3. Imprime un brief: rama, sucio, ahead/behind, último commit, marcador del agente y la sección "Siguiente paso" de `context.md`.
4. Lanza el `resume` del agente en foreground; con `--in`, en otra terminal; con `--fresh`, su `command` en vez del `resume`; con `--no-agent`, nada.

`wt remove` y `wt merge` cierran la sesión. **Por defecto, el contexto se archiva**: `context.md` y `journal.md` se mueven a `<git-common-dir>/wt/archive/`, porque son texto chico y perderlos duele. `session.archive = false` los borra con el worktree.

### 8. Memoria de trabajo, no transcripciones

`wt` guarda la memoria curada de una sesión: `context.md` con secciones fijas (Objetivo, Decisiones, Siguiente paso, Enlaces), sembrado desde la plantilla del perfil, y el diario. No guarda transcripciones: cada agente ya tiene su historial, y copiarlo acoplaría `wt` a formatos que cambian cada mes.

Un agente la ve sin acoplarse, por cualquiera de estos canales, todos opcionales:

- variables de entorno al arrancarlo: `WT_SESSION`, `WT_SESSION_DIR`, `WT_CONTEXT_FILE`;
- la variable `{context_file}` en la plantilla del agente;
- los recursos del servidor MCP;
- una copia materializada en el worktree, agregada a `.git/info/exclude` y nunca a `.gitignore`.

Nada sale de la máquina, como ya fija el non-goal de v1.

### 9. MCP: hacia la orquestación completa

`wt mcp serve` es un servidor MCP por stdio, que arranca el cliente del agente y muere con él. Es un adaptador fino: cada herramienta llama a la misma operación que su comando, devuelve el mismo documento `wt.*.v1` como salida estructurada y traduce el exit code a un error con código. Para eso, antes, la lógica que hoy vive en `internal/cli` mezclada con cobra se extrae a una capa de operaciones reutilizable, comando por comando y solo cuando un segundo adaptador lo necesita. Ese refactor no cambia nada visible, así que va como trabajo fuera de change (`refactor/`).

**El objetivo es la orquestación completa**: que un agente líder cree, lance, siga y cierre sesiones de otros agentes. Por eso `launch_session` y `close_session` son parte del diseño desde ahora, no un agregado posterior. Se entrega en dos fases, las dos dentro de M6:

| Fase | Herramientas | Tipo |
|---|---|---|
| Lectura y memoria | `list_repos`, `list_sessions`, `get_session`, `list_profiles`, `list_agents`, `list_launchers`, `resume_session` (ruta, brief y comando de retomar, sin abrir nada), `update_session_context`, `append_session_note`, `set_session_status` | Lectura, o escritura acotada a la sesión |
| Orquestación | `create_worktree`, `create_session`, `launch_session`, `sync_worktrees`, `close_session` | Escritura y efecto externo |

Más los recursos `wt://session/{repo}/{name}/context`, `…/journal` y `…/logs/{evento}`, y un prompt `handoff` que arma el brief para que un agente retome.

Límites que protegen al usuario de un agente inducido por el contenido de un repo:

- `create_*` y `sync_worktrees` admiten `dry_run`. `close_session` es destructiva: `dry_run` por defecto y `confirm: true` para ejecutar.
- `launch_session` solo acepta launchers y agentes **por nombre**, de la config del usuario. Nunca recibe un comando.
- `[mcp] tools` limita qué grupos expone el servidor (por ejemplo `["read", "sessions", "write"]`).
- No existen `approve_hooks` (un agente aprobando hooks anula el principio 4), ni `merge` en la primera versión (es irreversible sobre la rama destino; si se suma, con `confirm`), ni "correr un comando en el worktree": el agente ya tiene su propia shell.

### 10. Roadmap

| Milestone | Versión | Qué cambia |
|---|---|---|
| M4 Riqueza | `0.4.0` | Mismo alcance y orden. `branch-state` guarda en el registro por worktree (punto 2), y `wt.list.v1` admite los campos de sesión que suma después `sessions`, sin cambiar de versión. |
| M5 Extensión y sesiones | `1.0.0` | `agents`, `sessions`, `launchers`, `profiles`, `doctor`, `aliases-plugins`. 1.0 incluye sesiones. |
| M6 Orquestación | `1.1.0` | `session-context`, `agent-integrations`, `mcp-server` (en sus dos fases) y `dashboard` como tablero de sesiones con acciones. Deja de ser opcional. |
| M7 Producto | `1.2.0` | `distribution`, `docs-generation`, `forge`, `llm-commits` (lo que antes era M6), más el resumen automático de contexto, que comparte mecanismo con `llm-commits`. Opcional. |

El orden tentativo de los changes de M5 es `agents` → `sessions` → `launchers` → `profiles` → `doctor` → `aliases-plugins`: cada uno usa lo que entrega el anterior, y `sessions` ya sirve para retomar en tu terminal sin abrir nada. Como pide `CONTRIBUTING.md`, la tabla de cada milestone se escribe en `product.md` §7 cuando el milestone abre, no ahora.

## Consecuencias

- Todo worktree que existe hoy ya es una sesión sin perfil. Sin `default_profile`, `--agent`, `--in` ni `--profile`, `wt` hace exactamente lo de hoy, y los `--json` solo suman campos.
- Un solo almacén de estado por worktree desde M4: `branch-state`, `sessions`, `agent-integrations` y MCP escriben en el mismo lugar.
- 1.0 congela más contrato que antes: `wt.session.v1`, `wt.launch.v1`, `wt resume`, `wt session *`, `--agent`, `--in` y `--profile`. Es el costo de que 1.0 traiga lo que hoy falta.
- `config.Registry` necesita un tipo de clave nuevo, la tabla con nombre (`agents.<nombre>.command`), sin variable `WT_*`. Se diseña en `agents`.
- `launchers` es el change de más riesgo multiplataforma desde `shell-integration`: AppleScript y el permiso de Automatización en macOS, el YAML de Warp, y `wt.exe` tapado por la función `wt` en Windows.
- La capa de operaciones es el refactor más grande del plan. Las pruebas de `internal/cli` son la red y no cambian.
- `tools/relcheck` (`Milestones`) y la tabla de 0002 conocen M1 a M6. Hay que sumarles M7 (`1.2.0`) antes de publicar `v1.1.0`, que es cuando `relcheck` pide el minor siguiente.
- `product.md` cambia en §1, §2, §4, §5, §6, §7, §8 y §10; `openspec/config.yaml`, `CONTRIBUTING.md` y `README.md` siguen a los principios y a la tabla de versiones.

## Alternativas descartadas

- **Adelantar la orquestación a M4**: M4 está en curso, y sus changes (la vista completa de `list`, `branch-state`, el picker) son prerrequisitos de las sesiones.
- **Sesiones en 1.1, con el M5 original como 1.0**: 1.0 saldría sin lo que hoy falta, y es el contrato que después cuesta cambiar.
- **Mantener `mark`/`var` en `git config` por rama**: dos almacenes, una migración en M5, y es malo para estructuras y texto largo.
- **Un índice global como fuente de verdad** (`~/.local/state/wt/`, `%LOCALAPPDATA%\wt\`): una segunda fuente que se desincroniza con git.
- **El registro dentro del worktree** (`.wt/session.toml`): ensucia el árbol, se commitea por error y se pierde con `git clean`.
- **Un daemon que sepa qué agente corre**: rompe "estado en disco, sin proceso residente"; la señal buena es la que escriben las integraciones.
- **Varias sesiones por worktree, o sesiones sin worktree, en v1**: sin un caso real, complican la identidad y la resolución.
- **Borrar el contexto al borrar el worktree**: es texto chico, y perderlo es lo que esta decisión quiere evitar.
- **Que el launcher reciba el comando del agente**: cuatro reglas de quoting apiladas, y el agente dejaría de correr en foreground bajo `wt`.
- **Recuperar el verbo `open`**: es "abrir algo afuera" por nombre, lo que 0000 quitó a propósito.
- **Perfiles sugeridos por `.wt.toml` en 1.0**: es ejecución de código decidida por el repo.
- **Guardar transcripciones de los agentes**: acopla `wt` a formatos ajenos que cambian seguido.
- **MCP solo de lectura y memoria**: no alcanza para que un agente orqueste a otros, que es el objetivo.
- **Lógica propia en el servidor MCP**: dos implementaciones de cada operación que terminan divergiendo.
