# Design: cross-repo-resolution

## Context

El por qué está en `proposal.md`, y el comportamiento en `specs/`. Lo que ya existe y moldea el diseño:

- **El orden de `cd` ya está orquestado en un solo lugar.** `cdDestination` (`internal/cli/cd.go`) resuelve el repo actual con `worktree.Resolve`, cae a los repos con `workspace.Match` sobre `a.discoverRepos(cfg, warn)`, y mapea los errores en `target.go` (`worktreeError`, `repoAmbiguity`, `nameNotFound`).
- **`remove`, `lock` y `unlock` usan `requireRepository` + `a.resolveTarget`**, que solo conocen el repo actual. Ahí se lee la config con `loadConfig(repo.root())` y se corre git con `dir`, el directorio de trabajo.
- **`create` usa `requireRepository`, `loadConfig(repo.root())` y `dir`** para todo lo que es git (ramas, refs, fetch, `worktree add`), y el principal para la plantilla (`worktree.PathVars(main, …)`).
- **`loadRepository` corre `git worktree list --porcelain -z` y resuelve symlinks** en cada ruta antes de `worktree.Build`, para que las comparaciones sean puras. `git.Exec` no tiene estado, así que se puede llamar desde varias goroutines.
- **`internal/workspace` no importa ningún paquete interno.** Ahí viven `Roots`, `Walk`, `List`, `Current`, `Match`, `HasPrefixFold` e `IDOf`. `internal/worktree` importa `git` y `template`, no `workspace`.
- **Las completions no escriben nunca en la terminal**: `completeCdTargets` y `completeNames` (`target.go`) usan `loadConfigQuiet` y descartan warnings. cobra v1.10.2 trae `cobra.CompletionWithDesc` (`nombre\tdescripción`) y escapa `:` en bash (`_init_completion -n =:` y `__handle_special_char`) y en zsh (`comp=${comp//:/\\:}`).

Hechos medidos en macOS (Apple Silicon, git 2.51.2) sobre los 10 repos de `~/Documents/GIT`, el mejor de 5 intentos: `git worktree list --porcelain` en secuencia tarda 36 ms; con 4 en paralelo, 13 ms; con 8, 12 ms. En Windows no se midió.

## Goals / Non-Goals

**Goals:**
- Una sola resolución de targets, compartida por `cd`, `path`, `remove`, `lock` y `unlock`, con dos modos (con repos y sin repos). Que cada comando no reimplemente la cascada.
- Que cada paso de la cascada sea una función pura sobre datos ya leídos. Los efectos (recorrer raíces, listar worktrees) quedan entre pasos, y solo ocurren si el paso anterior no resolvió.
- Que leer los worktrees de N repos sea el único efecto nuevo, en un solo lugar, en paralelo y con el resultado en orden determinístico.
- Que la capa de config de un comando sobre otro repo salga de ese repo, sin leer el `.wt.toml` del directorio actual.

**Non-Goals:**
- Caché, o leer `.git/worktrees/` sin git (ver D4).
- Cancelar las demás llamadas en cuanto una encuentra el target: el paso 6 necesita todos los resultados para detectar ambigüedad.
- Generalizar la forma calificada a `prune`, `config` o `repos`.

## Decisions

### D1. Qué es puro y dónde vive

```
  target ──> workspace.SplitQualified ──┐                         PURO
                                         |
   sin ':'                               | con ':'
     |                                   v
     |                     workspace.MatchSide(repos, lado, goos)  PURO
     |                                   |
     v                                   v
  worktree.Resolve(locales)      worktree.Resolve(los del repo)    PURO
     | nada                              ^
     v                                   | git worktree list       EFECTO
  workspace.Match(repos)   <── Walk ─────┘ (un repo)
     | nada                (EFECTO, ya existe)
     v
  worktree.ResolveOthers(grupos)   <── a.scanWorktrees(repos)      EFECTO
     |                                  (N repos, en paralelo)
     v
  workspace.DisplayNames(repos)  -> hints, columna REPO, completions  PURO
```

Funciones nuevas:

```go
// internal/workspace (sigue sin imports internos)
func SplitQualified(target string) (repo, name string, qualified bool)
func MatchSide(repos []Repo, side, goos string) (Repo, error)   // nombre (Match) o sufijo
func DisplayNames(repos []Repo, goos string) []string            // alineado con repos

// internal/worktree
type Hit struct {
	Group    int      // índice del repo en los grupos que recibió
	Worktree Worktree
}
type OthersAmbiguousError struct{ Target string; Hits []Hit }
func ResolveOthers(groups [][]Worktree, target string) (Hit, error)  // *NotFoundError, *OthersAmbiguousError
```

`ResolveOthers` va en `worktree` y no en `workspace` para que `workspace` no dependa de `worktree` (y por lo tanto de `git`). Recibe los grupos como índices, y la CLI traduce el índice a repo y nombre visible. Descarta los principales y los bare; cada paso (NAME, después rama) mira todos los grupos juntos.

`SplitQualified` corta en el primer `:`. El lado del repo vacío se valida en la CLI, porque el mensaje y el exit code son de la CLI.

### D2. `MatchSide`: nombre o sufijo de ruta

- Si el lado no tiene separador (`/`, y en Windows también `\`), llama a `Match` (los tres pasos que ya existen).
- Si lo tiene, parte el lado y el `Path` de cada repo en componentes, con los separadores de `goos` y descartando los vacíos. Matchea los repos cuyos últimos componentes son iguales a los del lado, comparando con `strings.EqualFold` componente a componente. No hay prefijo.
- Un resultado → ese repo; ninguno → `*NotFoundError`; varios → `*AmbiguousError`, en el orden de `List`.
- Recibe `goos` y no usa `filepath`, como `Roots`, para que las reglas de Windows se prueben desde macOS.

*Alternativa descartada:* que el sufijo sea relativo a la raíz. Dos repos `api` directamente bajo dos raíces tienen el mismo sufijo relativo (`api`), y no se distinguirían.

### D3. `DisplayNames`: el sufijo único más corto

Por cada repo cuyo nombre exacto no comparte nadie, el nombre visible es su nombre. Para los demás, se prueba con k = 2, 3, … componentes del final del `Path`, se arma el sufijo con `/` y se pasa por el **mismo** `MatchSide`. El primero que devuelve solo ese repo gana. Si ninguno lo logra (raíces anidadas, ver la spec), el nombre visible es el `Path`.

Al usar `MatchSide` para decidir, la ida y la vuelta coinciden por construcción: lo que muestra un hint, tipeado, vuelve al mismo repo.

Se une con `/` en los tres OS, porque `MatchSide` lo acepta en todos: un nombre visible copiado en Windows funciona igual en macOS.

### D4. El único efecto nuevo: `a.scanWorktrees`

```go
// internal/cli/scan.go
type repoWorktrees struct {
	Repo      workspace.Repo
	Worktrees []worktree.Worktree // nil si Err != nil
	Err       error
}
func (a *app) scanWorktrees(ctx context.Context, repos []workspace.Repo, cwdReal string) []repoWorktrees
```

- Una goroutine por repo, con un semáforo (canal con buffer) de `runtime.GOMAXPROCS(0)`. Cada una escribe en `results[i]`, así que el orden es el de `repos`, sin ordenar después. Sin dependencias nuevas: no hace falta `errgroup`.
- Cada repo se lista con el mismo código que `loadRepository`, factorizado en `a.listWorktrees(ctx, dir, cwdReal)`: `git -C <Repo.Path> worktree list --porcelain -z`, `EvalSymlinks` de cada ruta y `worktree.Build`. Para un `.bare`, `git -C proj` funciona (el `.git` apunta a `./.bare`).
- `cwdReal` solo se pasa para el repo actual (el que `workspace.Current` marca con el `IDOf` de su principal). Los demás reciben `""`, así que ningún worktree de otro repo queda marcado como actual, aunque haya layouts anidados.
- Si algún resultado tiene `git.ErrNotInstalled`, el comando falla con `gitError` (exit 1). Cualquier otro error deja `Err` en ese repo, y quien llama decide: un warning `cannot read the worktrees of <nombre visible>: <mensaje>` por `a.warn` (que ya calla con `--json`), una entrada en `unreadable`, o nada (completions).

En Windows el código es el mismo; lo que cambia es el costo de lanzar git, que se mide en la tarea **[Windows]**.

*Alternativa descartada:* leer `<commondir>/worktrees/*/gitdir`, `HEAD` y `locked` del filesystem. No lanza procesos, pero reimplementa el porcelain (rutas relativas con `extensions.relativeWorktrees` desde git 2.48, `prunable`, el principal con `core.bare`/`core.worktree`), crea dos fuentes de verdad, y se aparta de "git como subproceso con `--porcelain`". Queda como plan B, sin cambiar las specs.
*Alternativa descartada:* llamadas secuenciales. Triplican la latencia en macOS, y en Windows es peor.
*Alternativa descartada:* caché entre corridas. Invalidarla (un `wt create` de otra terminal, un `git worktree add` a mano) cuesta más que el problema.

### D5. Una sola resolución: `a.resolve`

```go
// internal/cli/resolve.go
type resolveMode int
const (
	modeNavigate resolveMode = iota // cd, path: pasos de repo incluidos; "api:" es el repo
	modeWorktree                    // remove, lock, unlock: sin pasos de repo; "api:" es el principal
)

type resolved struct {
	Path     string             // destino de cd y path
	Worktree *worktree.Worktree // nil cuando el destino es un repo (solo modeNavigate)
	Repo     *repository        // el repo del target: sus worktrees y su principal
	GitDir   string             // donde corre git para ese repo
	Other    bool               // no es el repo del directorio de trabajo
}

func (a *app) resolve(ctx context.Context, dir, target string, mode resolveMode) (resolved, *config.Config, error)
```

El orden, con los efectos entre los pasos puros:

```
  1. ':'?  -> lado del repo vacio: exit 2.  Nombre '@' o '-': exit 2.
  2. repo, notRepo := loadRepository(dir)          (NotRepoError se guarda)
  3. base := loadConfigQuiet("")                   (usuario + entorno: las raices)
  4. calificado:
       sin raices -> exit 1 (como wt repos)
       Walk + List + MatchSide -> repo destino (exit 3 / 4)
       nombre vacio: modeNavigate -> su Path, sin git; modeWorktree -> sigue con "^"
       listWorktrees(repo destino)   -> error de git: exit 1 (nunca se saltea)
       worktree.Resolve(nombre)      -> exit 3 "in <nombre visible>" / exit 4
  5. sin ':':
       repo actual: worktree.Resolve -> ok, o exit 4 (corta)
       '^' o '@' -> errores de hoy
       modeNavigate y hay raices: Match -> ok, o exit 4 (corta)
       hay raices: scanWorktrees(los demas) + ResolveOthers -> ok, o exit 4
       nada: la tabla de "nada encontrado" del modo
  6. cfg := loadConfig(raiz de la capa de repo)    (D6; los warnings se imprimen aca, una vez)
```

`cdDestination` pasa a ser `a.resolve(…, modeNavigate)` más el caso `-`. `wt path` es lo mismo, sin el caso `-`. `planRemove` y `changeLock` reemplazan `requireRepository` + `resolveTarget` por `a.resolve(…, modeWorktree)`. Usan `resolved.Repo.worktrees` para `CheckRemovable` y `resolved.GitDir` para correr git. Así, `git worktree remove`, `git branch -d` y `git worktree lock` corren en el repo del target.

Los warnings de un repo que no se pudo leer salen antes del error o de la salida del comando.

### D6. La capa de repo del target

Hoy `loadConfig(repoRoot)` lee usuario, `.wt.toml` de `repoRoot` y entorno, y el que llama decide `repoRoot`. Este change solo cambia qué raíz se pasa:

| Target | `repoRoot` |
|---|---|
| del repo actual (local o calificado al actual) | `repo.root()`, como hoy: el worktree actual |
| de otro repo | el `Path` del principal de ese repo; `""` si es bare |
| un repo en `modeNavigate` (`wt cd api`) | irrelevante: `cd` y `path` no leen claves de repo; se usa `repo.root()`, como hoy |

El paso 3 de D5 lee la config sin capa de repo y sin imprimir: `repos_root` no puede venir de un `.wt.toml` (lista blanca de `config-layers`), así que esa lectura alcanza para descubrir. El paso 6 lee la config efectiva y es la única que imprime warnings. Por eso un `.wt.toml` inválido en el directorio actual no rompe `wt create api:feat`, y los warnings del archivo de usuario no salen dos veces.

Consecuencia: un `.wt.toml` inválido se reporta (exit 1) después de resolver el target, no antes. Un archivo de usuario inválido sigue fallando primero, en el paso 3. Con un `.wt.toml` roto y un target que no existe, el exit pasa de 1 a 3. Es un cambio de precedencia entre dos errores, sin escenario en las specs que lo fije.

`wt config` no cambia: siempre usa el directorio de trabajo.

### D7. `wt create <repo>:<name>`

`planCreate` cambia su principio. Antes de `requireRepository`, `SplitQualified(name)`:

- **Sin `:`** → como hoy, más el hint `use <repo>:<name> to create it in a repository under repos_root` cuando el `NotRepoError` sale con raíces configuradas.
- **Con `:`** → la validación de `<name>` corre sobre el lado del nombre (vacío → exit 2) y el lado del repo vacío da exit 2, las dos antes de leer nada. Después, `loadConfigQuiet("")`, `Walk` + `List` + `MatchSide`, y `listWorktrees(repo.Path)`. El resto de `planCreate` corre con `dir = repo.Path` y `repo` = ese repo, y la config se lee con la raíz de D6.

Todo lo que hoy usa `dir` y `repo` (refs, remotos, fetch, `worktree add`, `BranchTaken`, `RegisteredAt`, `PathVars(main, …)`) queda apuntando al repo destino sin tocar esas funciones. `wt.create.v1` toma `name` del lado del nombre.

### D8. `wt list --all-repos`

`listCommand` suma el flag. Con `--all-repos`:

1. `loadRepository` es opcional (fuera de un repo no falla) y se lee la config de usuario + entorno. Sin raíces, sale el error de `wt repos`, reusado de `repos.go`.
2. `discoverRepos` + `DisplayNames` + `scanWorktrees` (con `cwdReal` solo para el repo actual).
3. Tabla: `listRows` se generaliza con una columna opcional; con `--all-repos`, la segunda columna es `REPO` (nombre visible). Las filas salen en el orden de `List` y, dentro de cada repo, en el de `Build`.
4. JSON, en un documento aparte para no tocar el de `wt list`:

```go
type listAllJSON struct {
	Schema     string            `json:"schema"`     // "wt.list.v1"
	Worktrees  []worktreeAllJSON `json:"worktrees"`  // nunca null
	Unreadable []unreadableJSON  `json:"unreadable"` // nunca null
}
type worktreeAllJSON struct {
	worktreeJSON
	Repo     string `json:"repo"`      // nombre, como en wt repos
	RepoPath string `json:"repo_path"`
}
type unreadableJSON struct {
	Repo    string `json:"repo"`
	Path    string `json:"path"`
	Message string `json:"message"`
}
```

Sin `--all-repos`, el hint nuevo fuera de un repo se agrega al `NotRepoError` cuando hay raíces. Para saberlo hace falta leer la config, que hoy no se lee antes de fallar: se lee `loadConfigQuiet("")` solo en ese camino de error.

### D9. Completions

`completeCdTargets` se generaliza en `a.completeTargets(mode, filter)`:

- **Palabra sin `:`**:
  - grupo 1: worktrees locales que pasan `filter`;
  - grupo 2: solo en `modeNavigate`, repos con `HasPrefixFold`;
  - grupo 3: `scanWorktrees` de los otros repos, filtrados igual.
  - Cada nombre sale una vez; si se repite, gana el grupo más cercano (el mismo orden que Enter).
  - Descripción con `cobra.CompletionWithDesc`: `worktree`, `repository`, `worktree of <nombre visible>`.
- **Palabra con `:`**: `MatchSide` (si no da exactamente uno, nada), `listWorktrees` de ese repo, y `<lado tal como se tipeó>:<NAME>` para los que no son el principal y pasan `filter`.
- Los filtros: `cd` y `path`, directorio existente; `remove` y `lock`, ni principal ni lockeado; `unlock`, lockeado.
- **`wt create`**: `<name>` ofrece `<nombre visible>:` de los repos con `HasPrefixFold`, con `ShellCompDirectiveNoSpace`. `completeBases` mira `args[0]`, y si es calificado, lista las refs del repo destino.

**Repos homónimos (diferencia con lo que se habló al explorar):** en `cd` se ofrecen por su nombre, una vez, no como `GIT/api:`. bash y zsh filtran los candidatos por el texto tipeado, así que `GIT/api:` nunca aparecería para `ap<TAB>`. Enter sobre `api` da el exit 4, cuyos hints nombran `GIT/api:` y `Repos/api:`. En `wt create`, en cambio, sí se ofrece el nombre visible (`GIT/api:`), porque ahí todo candidato es un prefijo calificado.

### D10. Mensajes

`target.go` concentra el mapeo de errores a `*Error`, con tests de tabla:

| Caso | Mensaje | Hints |
|---|---|---|
| ambigüedad entre repos (pasos 3-5 y lado del repo) | `"x" matches more than one repository` | `<nombre visible>:  <ruta>` por candidato |
| ambigüedad entre otros repos (pasos 6-7) | `"x" matches more than one worktree` | `<nombre visible>:<NAME>  <ruta>` por candidato |
| nada, `modeNavigate` | la tabla de `navigate` | `wt list --all-repos`, `wt repos` (o `wt list` sin raíces) |
| nada, `modeWorktree` | `no worktree named "x"`; fuera de un repo y sin raíces, `not a git repository: <dir>` | `wt list --all-repos` con raíces, `wt list` sin ellas; fuera de un repo y sin raíces, `repos_root` |
| calificado sin el worktree | `no worktree named "x" in <nombre visible>` | `wt list --all-repos` |
| lado del repo vacío | `missing repository before ":" in "<target>"` | — |
| `@` o `-` calificados | `"<target>": @ and - cannot follow a repository` | — |

La ambigüedad dentro de un repo (pasos 1-2 y lado del nombre) mantiene sus hints de hoy (`<ruta> (branch x)`): la forma calificada no ayuda entre worktrees del mismo repo.

### D11. Tests

- **Puros, con tablas:**
  - `SplitQualified`: `api:feat`, `api:`, `:feat`, `:`, `api:a:b`, sin `:`.
  - `MatchSide` con `goos` darwin y windows: nombre, sufijo, sufijo sin mayúsculas, sufijo que no es prefijo, `\` como separador solo en windows, separadores repetidos y en los bordes, ambiguo.
  - `DisplayNames`: único, homónimos en dos raíces, carpetas de organización, solo mayúsculas distintas, sin sufijo único, `/` en windows.
  - `ResolveOthers`: NAME antes que rama entre grupos, principales y bare descartados, ambigüedad con los `Hit`, vacío.
  - El mapeo de mensajes de D10.
- **`scanWorktrees` contra repos reales**: orden estable con N repos, un repo con `HEAD` corrupto (queda con `Err`, el resto se lee), y `cwdReal` solo en el repo actual.
- **CLI in-process**: los escenarios de `cross-repo-resolution`, `navigate`, `list-worktrees`, `remove-worktree`, `create-worktree`, `config-layers` y `shell-integration` (estos con `__complete`, incluidas las descripciones y el directive `NoSpace`).
- **Shells reales**: `wt cd api:feat` y `wt cd <worktree de otro repo>` a través de la función, y la completion de `wt cd cortex:` en zsh, bash (donde `:` corta palabras), fish y pwsh.

## Risks / Trade-offs

- [En Windows, lanzar N `git` cuesta varias veces lo que en macOS, y cada TAB de `wt cd` escanea todo el workspace] → paralelo (D4) y una medición manual en la máquina del laburo (tarea **[Windows]**). Si no alcanza, el plan B (leer el filesystem) no cambia las specs.
- [Un repo ilegible tenía el worktree homónimo de otro, y un `wt remove feat` resuelve al único legible, cuando con toda la información habría sido exit 4] → aceptado al explorar: el warning sale antes, y sobre el repo ilegible igual no se podía operar.
- [Un prefijo de repo tapa un worktree exacto de otro repo (`wt cd feat` → `feature-flags`)] → aceptado al explorar (opción A): mantiene todo lo que hoy resuelve, y `api:feat` es la salida exacta.
- [La completion con `:` se comporta distinto en bash por `COMP_WORDBREAKS`] → cobra ya lo maneja (`__handle_special_char`), y hay test con bash real.
- [`wt cd api:^` en zsh con `EXTENDED_GLOB` necesita comillas] → el `--help` de `cd` ya lo dice para `^`; se extiende a la forma calificada.
- [Cambian mensajes y hints que un script podría estar parseando (`no repository named` fuera de un repo, los hints de exit 4)] → los mensajes no son contrato, los exit codes sí, y no cambian. Va en el CHANGELOG.
- [`Build` con `cwdReal` en repos anidados podría marcar dos worktrees como actuales] → `cwdReal` solo va al repo del directorio de trabajo (D4).

## Migration Plan

No hay migración: sin `repos_root` (el default), todo se comporta como en v0.2.0-alpha.1, salvo el hint nuevo (que sugiere configurar `repos_root`) de `wt remove`, `wt lock` y `wt unlock` fuera de un repo. Con raíces configuradas, lo que hoy resuelve sigue resolviendo igual; lo nuevo solo actúa donde hoy hay un exit 3. Rollback: revertir el squash.

## Open Questions

- **El tamaño del pool (`GOMAXPROCS`)**: se ajusta con la medición de Windows, sin cambiar specs ni tareas.
