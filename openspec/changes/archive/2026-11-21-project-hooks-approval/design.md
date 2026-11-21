# Design: project-hooks-approval

## Context

El por qué está en `proposal.md`, y el comportamiento en `specs/`. Lo que ya existe y moldea el diseño:

- **Los hooks del usuario ya están resueltos de punta a punta** (`hooks`, `v0.3.0-alpha.1`):
  - `internal/hook` es puro: `Event`, `Events`, `Vars`, `Plan(e, templates, values, dir, log) Run`, `PreviewLines`.
  - `internal/cli/hooks.go` tiene el `hookPlanner`, que `create`, `cd` y `remove` usan para armar sus `hook.Run` **antes** de cualquier efecto. `configured(events...)` hace que no haya costo cuando no hay hooks.
  - Los planes (`worktree.CreatePlan`, `RemovePlan`, el par `pre`/`post` de `cd`) ya separan decidir de hacer, y `--dry-run` sale del mismo plan.
- **La config es un `Registry` de claves y capas con precedencia.**
  - `config.Resolve` aplana `[hooks]` a `hooks.<evento>` y, para la capa `repo`, ignora con warning las claves con `InRepo: false`. Las seis `hooks.*` son `InRepo: false`.
  - La capa `repo` es el `.wt.toml` del worktree actual, o el del worktree principal del otro repo en los comandos cross-repo; `loadConfig(repoRoot)` la arma.
- **El exit 8 ya existe** como constante (`ExitHookNotApproved`) y en la tabla de `cli-contract`, sin uso.
- **`wt config path`** ya imprime `user:` y `repo:` y tiene el esquema `wt.config.path.v1`.
- **La ruta del archivo del usuario** sale de `config.UserFile(goos, getenv, home)`, puro y testeado para los tres OS.
- **`table.go`** ya arma tablas alineadas con header para `wt list` y `wt repos`.

## Goals / Non-Goals

**Goals:**
- Que la decisión "¿este evento puede correr?" sea pura y salga del mismo plan que ya usan `create`, `cd` y `remove`, así el exit 8 llega antes de cualquier efecto, también en `--dry-run`, sin un `if` por comando.
- Cero costo sin hooks del repo: ni leer `approvals.toml` ni hashear nada.
- Que el repo no pueda influir en su propia aprobación: nada de lo que se lee del repo decide dónde se guarda ni cómo se compara.

**Non-Goals:**
- Un prompt interactivo (ver Non-goals del proposal).
- Bloquear archivos entre procesos: dos `wt hook approve` a la vez pueden perder una aprobación, y se vuelve a pedir.
- Normalizar rutas de repo (mayúsculas, symlinks).

## Decisions

### D1. Los hooks del repo no entran en la precedencia de capas

```
  user file  [hooks] ─┐
                      ├─> Resolve ─> cfg.Values  hooks.<e> (user o default)   lo que muestra wt config
  .wt.toml   [hooks] ─┘           └> cfg.RepoHooks[hooks.<e>]  []string        validado, aparte
```

- Un campo nuevo en `config.Key`, `RepoApart bool`, para las seis `hooks.*`. En `Resolve`, un valor de la capa `repo` de una clave `RepoApart` se valida con el mismo `Validate` y se guarda en `Config.RepoHooks map[string][]string`, con `Config.RepoFile` (el `Origin` de la capa), en lugar de pisar el valor efectivo o dar el warning.
- `wt config get/list` no cambian: leen `cfg.Values`.

*Alternativas descartadas:*
- *`InRepo: true` y precedencia normal*: el repo pisaría el `post-create` del usuario; un `.wt.toml` con `post-create = []` borraría el `cp ~/.npmrc .` del usuario sin que lo note.
- *Concatenar en `cfg.Values` con source `repo`*: `wt config list` mostraría una lista que no está en ningún archivo, y `source` dejaría de ser una sola capa.
- *Un archivo aparte del repo* (`.wt/hooks.toml`): dos archivos de repo para lo mismo; `.wt.toml` ya es la capa del repo.

### D2. Qué es una aprobación y cómo se compara (puro)

Paquete nuevo `internal/approval`, sin git:

```go
type Approval struct {
	Repo    string `toml:"repo"`    // ruta nativa del worktree principal
	Event   string `toml:"event"`
	SHA256  string `toml:"sha256"`  // hex en minúsculas
	Command string `toml:"command"` // para quien lee el archivo; no decide nada
}

type Set struct{ list []Approval }

func Hash(command string) string                         // sha256 hex del texto tal cual está escrito
func (s Set) Approved(repo string, e hook.Event, command string) bool
func (s *Set) Add(repo string, e hook.Event, command string) bool // false si ya estaba
func Parse(data []byte, origin string) (Set, error)       // puro: TOML -> Set, valida campos
func (s Set) Marshal() []byte                             // puro: Set -> TOML, con un comentario de cabecera
```

- **Se hashea la plantilla, no el comando renderizado**: una aprobación vale para todos los worktrees, y un `{name}` no la invalida.
- **La clave es `(repo, event, sha256)`**:
  - *repo*: sin él, aprobar `make` en un repo lo aprueba en cualquier clone que use `make` (y `make` corre el `Makefile` del repo).
  - *event*: aprobar `rm -rf build` como `post-remove` no lo aprueba como `pre-create`.
  - *repo* es la ruta del worktree principal del repo cuyo `.wt.toml` se leyó: `repo.main().Path` en el repo de la capa. La ruta la elige el usuario al clonar; el repo no puede fingirla.
- **Comparación exacta de strings**, sin normalizar. Una ruta escrita distinto (mayúsculas en APFS o NTFS) pide aprobar de nuevo: falla del lado seguro.
- `Parse` rechaza un `[[approval]]` sin `repo`, `event` o `sha256`, o con un `sha256` que no son 64 hex en minúsculas, con exit 1 nombrando el archivo. Un campo desconocido se ignora (para poder sumar `approved_at` sin romper).

*Alternativas descartadas:*
- *Hash del comando solo* (como dice hoy §5): reusable entre repos; ver arriba.
- *Identificar el repo por el URL de `origin`*: lo setea quien clona igual que la ruta, pero dos clones del mismo remote compartirían aprobaciones aunque estén en ramas con `.wt.toml` distintos, y un repo sin remote no tendría identidad.
- *Guardar solo el hash*: el archivo se vuelve ilegible y revocar a mano es imposible; `command` va como dato informativo.

### D3. Dónde vive `approvals.toml` y cómo se escribe

- `config.ApprovalsFile(goos, getenv, home) (string, error)`: el directorio de `UserFile(...)` + `approvals.toml`, con el `join` por OS que ya existe. Puro; mismo test de tabla que `UserFile` para darwin, linux, windows y `WT_CONFIG`.

  | OS | Ruta |
  |---|---|
  | macOS, Linux | `$XDG_CONFIG_HOME/wt/approvals.toml` o `~/.config/wt/approvals.toml` |
  | Windows | `%APPDATA%\wt\approvals.toml` |
  | con `WT_CONFIG=/x/wt.toml` | `/x/approvals.toml` |

- **Archivo aparte, no `config.toml`**: escribir el archivo del usuario reformatearía su TOML y le borraría los comentarios. `approvals.toml` es de `wt`; el usuario solo borra tablas.
- **Escritura atómica**, en `internal/approval` (el único efecto del paquete, `Save(path, Set)`): `MkdirAll` del directorio, `os.CreateTemp` en el mismo directorio, escribir, `Sync`, `Close`, `os.Rename`. En Windows `os.Rename` reemplaza el destino (`MoveFileEx` con `MOVEFILE_REPLACE_EXISTING`); si un antivirus lo tiene abierto falla con exit 1 y el archivo viejo queda intacto.
- `Load(path) (Set, error)`: archivo inexistente = `Set` vacío.

### D4. El chequeo vive en el `hookPlanner`

```
  hookPlanner{cfg, repoHooks, repoFile, repoPath, crossRepo, approvals (lazy)}
        |
        | plan(e, values, dir)
        v
  commands = cfg hooks.<e> (user) ++ repoHooks[hooks.<e>]
  unapproved = primero de repoHooks[hooks.<e>] con !approvals.Approved(repoPath, e, c)   PURO
        |
        +-- unapproved != "" --> *Error{Code: 8, ...}   antes de cualquier efecto
        +-- ok --> hook.Plan(e, commands, values, dir, log)  (sin cambios)
```

- `create`, `cd` y `remove` ya planifican `pre` y `post` antes de hacer nada (y antes del preview de `--dry-run`), así que el error del `post-*` llega antes del primer efecto **sin tocar** esos comandos más que para pasarle al planner los datos del repo.
- `configured(events...)` pasa a mirar también `repoHooks`.
- `approvals` se carga la primera vez que un evento tiene comandos del repo, y una sola vez por corrida.
- `--no-hooks` corta antes, como hoy: no se carga nada.
- **Orden de chequeo**: en el orden en que el comando planifica (`pre` y después `post`), y dentro del evento en el orden de `.wt.toml`. El mensaje nombra el primero.
- **El hint**: `wt hook approve` si el repo de la capa es el del directorio de trabajo; si no, `wt -C <repoPath> hook approve`. `-C` al worktree principal lee el `.wt.toml` del principal, que es justo el que usan los comandos cross-repo (`config-layers`), así que aprueba lo mismo que se pidió.
- `repoPath` y `crossRepo` los conoce quien llama a `loadConfig`: `create` y `remove` tienen `main`; `cd` tiene `t.repo` o el repo que carga desde `t.dest`. Para `cd -` hacia otro repo la capa es la del directorio de trabajo (como hoy), y `repoPath` es el principal de **ese** repo, no del destino: la aprobación es del repo que declaró el comando.

*Alternativa descartada:* chequear en `runBlocking`/`background`, al momento de correr. Un `post-create` sin aprobar dejaría el worktree creado y tendría que deshacerlo; y `--dry-run` no lo vería.

### D5. `wt hook list` y `wt hook approve`

- Comando `hook` con dos subcomandos, en `internal/cli/hookcmd.go`. Los dos usan el repo del directorio de trabajo y su capa, como `wt config` (`repoRootForConfig`), y `loadRepository` para `main().Path`. Fuera de un repo: exit 3 por `gitError`, como `wt list`.
- **Puro**: `hook.Entries(userHooks, repoHooks map[string][]string, approved func(e, c) bool) []Entry`, con `Entry{Event, Source, Command, Approved}`, en el orden de `hook.Events` y usuario → repo. `list` lo muestra; `approve` filtra `Source == repo && !Approved` (y los eventos pedidos) y hace `Set.Add`.
- **Texto**: `list` usa `table.go` con las columnas `EVENT SOURCE APPROVED COMMAND`, sin color en `APPROVED` (el valor ya dice `yes`/`no`). Sin filas, no imprime nada, ni el header.
- **JSON**: `wt.hook.list.v1` `{schema, repo_path, repo_file, hooks: [{event, source, command, approved}]}`; `wt.hook.approve.v1` `{schema, approved: [{event, command}]}`, `approved` siempre array (nunca `null`).
- **`--dry-run`** en `approve`: imprime `would approve …` y no llama a `Save`.
- **Eventos como argumentos**: validados contra `hook.Events`; uno desconocido es `usageError` (exit 2). Completion de los argumentos con `hook.Events`.
- `wt hook` sin subcomando muestra el help, como `wt config` y `wt shell`.

### D6. `wt config path`

Suma `approvals: <path> (exists|missing)` como tercera línea, con el mismo `existence()`. En JSON, `approvals: {path, exists}`. Campo nuevo: no cambia la versión del esquema (`cli-contract`: solo quitar, renombrar o cambiar el tipo la sube).

### D7. Documentación

- El help de `wt config` dice hoy que el repo no puede declarar hooks: pasa a decir que puede, que se suman después de los del usuario y que hay que aprobarlos con `wt hook approve`.
- `docs/design/product.md`: §5 "Hooks" cambia "guardada por hash del comando en la config del usuario" por la clave `(repo, evento, hash)` en `approvals.toml`, y suma el orden usuario → repo; §4 describe `wt hook list` / `wt hook approve` con sus flags; §7 ajusta la fila si hace falta.

## Risks / Trade-offs

- [Un comando aprobado corre código del repo igual: `npm ci` ejecuta los scripts de `package.json`, y `make` el `Makefile`] → Es inherente: la aprobación es del comando, no de lo que el comando lee. El help de `wt hook approve` lo dice en una línea.
- [Fricción: el primer `wt create` en un repo con hooks falla con exit 8] → El hint da el comando exacto, y `wt hook list` muestra antes qué correría. Un prompt puede sumarse después sin cambiar el formato de las aprobaciones.
- [Aprobaciones viejas se acumulan en `approvals.toml`] → Son pocas líneas por repo; un `wt hook prune` puede llegar después.
- [Mover o re-clonar el repo pide aprobar de nuevo] → Falla del lado seguro; es un comando.
- [Dos `wt hook approve` concurrentes] → El rename atómico evita un archivo corrupto; en el peor caso se pierde una aprobación y se vuelve a pedir.

## Migration Plan

Nada que migrar: hoy `[hooks]` en `.wt.toml` se ignora. Un repo que ya lo tenga empieza a dar exit 8 en `create`, `cd` y `remove` hasta que el usuario apruebe; el CHANGELOG lo dice en *Changed*. Rollback: volver a la versión anterior, que lo ignora con un warning; `approvals.toml` queda sin uso.
