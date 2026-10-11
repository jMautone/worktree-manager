# Proposal: cross-repo-resolution

## Why

Con `workspace-discovery`, `wt cd <repo>` te lleva a cualquier repo desde cualquier lado, pero los worktrees siguen atados al repo en el que estás parado. Con tres agentes trabajando en worktrees repartidos entre diez repos, para saltar a uno, borrarlo o crear otro primero hay que acordarse de en qué repo vive, entrar al repo y recién ahí operar. Y no hay ninguna vista que muestre todos los worktrees juntos. Los worktrees de Claude Code tienen nombres que nadie tipea (`angry-jang-12fc4f`), así que sin una completion que los encuentre en todo el workspace son, en la práctica, inalcanzables. Este change es el segundo corte de M2, el que convierte el descubrimiento de repos en el diferencial del producto: `wt` resuelve un nombre de worktree en todo el workspace, detecta la ambigüedad entre repos y te da una forma precisa de nombrar cualquier worktree de cualquier repo.

## What Changes

**Se agrega**

- **Forma calificada `<repo>:<name>`**, aceptada por `wt cd`, `wt path`, `wt remove`, `wt lock`, `wt unlock` y `wt create`. Se corta en el primer `:`, y un target con `:` nunca pasa por la resolución de nombres sueltos.
  - El lado del repo se resuelve como `wt cd <repo>` (exacto, sin mayúsculas, prefijo único) o, si tiene un separador de ruta, como sufijo de la ruta del repo (`GIT/api:feat`). Esto último sirve para nombrar repos homónimos.
  - El lado del nombre se resuelve dentro de ese repo como en el repo actual: NAME, después rama, con `^` para el principal.
  - `api:` es el repo mismo. `:feat`, `:`, `api:@` y `api:-` son errores de uso (exit 2).
- **Nombre visible de un repo**: su nombre o, si otro repo descubierto se llama igual, el sufijo de ruta más corto que lo distingue. Es lo que muestran los hints, la columna `REPO` y las completions.
- **Comando `wt path <target>`**: imprime la ruta a la que iría `wt cd <target>`, con la misma resolución. No necesita la shell integration. `--json` usa el nuevo esquema `wt.path.v1`.
- **Flag `wt list --all-repos`**: una sola tabla con los worktrees de todos los repos descubiertos, con una columna `REPO`. En `wt.list.v1` suma, solo con este flag, `repo` y `repo_path` en cada elemento y un array `unreadable` en la raíz.

**Se modifica**

- **`wt cd <name>`**: después de los repos (exacto, sin mayúsculas, prefijo) busca entre los **worktrees enlazados de los otros repos**, por NAME y después por rama, sin contar los principales. Todo `wt cd x` que hoy resuelve sigue yendo al mismo lugar: el paso nuevo solo actúa donde hoy hay un exit 3.
  - Si un nombre aparece en varios repos, sale con exit 4 y un `hint:` por candidato con su forma calificada (`api:feat`) y su ruta.
  - Fuera de un repo, con raíces configuradas, el mensaje de "nada encontrado" pasa de `no repository named "x"` a `no worktree or repository named "x"`. Los hints pasan a nombrar `wt list --all-repos` y `wt repos`.
  - El exit 4 entre repos suma la forma calificada de cada candidato, además de su ruta.
- **`wt remove`, `wt lock` y `wt unlock`**: resuelven un nombre suelto en el repo actual y, si no está ahí, entre los worktrees enlazados de los otros repos. No buscan repos. Funcionan también fuera de un repo, si hay raíces configuradas.
- **`wt create <repo>:<name>`**: crea el worktree en ese repo desde cualquier directorio. Rama, base, fetch, ruta y `-x` se resuelven y corren en el repo destino. `wt create <name>` a secas no cambia; fuera de un repo, con raíces configuradas, suma un hint que sugiere la forma calificada.
- **Capa de repo de la config**: cuando un comando opera sobre un worktree de **otro** repo, la capa de repo es el `.wt.toml` del worktree principal de ese repo, y ya no el del worktree donde estás parado. Por ejemplo, `wt remove api:feat` desde `web` juzga si la rama está mergeada con el `default_base` de `api`.
- **`wt list`** sin `--all-repos`, fuera de un repo: sigue con exit 3, y suma un hint a `wt list --all-repos` si hay raíces configuradas.
- **Completions**:
  - `wt cd` y `wt path` ofrecen, además de lo de hoy, los worktrees enlazados de todos los repos. `<repo>:<TAB>` ofrece los worktrees de ese repo.
  - Cada candidato lleva una descripción con su origen (`worktree`, `repository`, `worktree of api`).
  - `remove`, `lock` y `unlock` ofrecen también los worktrees de otros repos, con sus filtros de hoy.
  - `wt create` ofrece `<repo>:` para `<name>`, y para `--base` las ramas del repo destino.
- **Un repo donde git falla** mientras se recorren otros repos (corrupto, sin permisos, `dubious ownership`): un warning, y se sigue con los demás sin cambiar el exit code.

**Se quita**

- **BREAKING** (caso borde): un worktree o repo cuyo nombre contiene `:` deja de alcanzarse con un nombre suelto, porque el `:` lo vuelve un target calificado. Solo pasa en macOS o Linux, que permiten `:` en un nombre de directorio; git no lo permite en ramas. Un worktree así se sigue alcanzando como `<repo>:<nombre>`, porque el corte es en el primer `:`.
- Ninguna clave de config, variable de entorno ni flag. Los exit codes no cambian.

## Capabilities

### New Capabilities

- `cross-repo-resolution`: la forma calificada `<repo>:<name>` y sus dos lados, el sufijo de ruta para repos homónimos, el nombre visible de un repo, la búsqueda entre los worktrees enlazados de los otros repos, los hints de ambigüedad con la forma calificada, y qué pasa cuando git no puede leer un repo.

### Modified Capabilities

- `navigate`: la resolución de `wt cd` suma los worktrees de otros repos y la forma calificada, y cambian los mensajes de "nada encontrado" y los hints de ambigüedad entre repos. Se agrega `wt path`.
- `list-worktrees`: se agrega `--all-repos`, con su tabla, su orden y sus campos de JSON, y el hint fuera de un repo.
- `remove-worktree`: el target de `remove`, `lock` y `unlock` se resuelve también en otros repos y en la forma calificada, y sus completions ofrecen los worktrees de otros repos.
- `create-worktree`: `<name>` acepta la forma `<repo>:<name>`, el repo deja de ser siempre el del directorio actual, y cambian las completions de `<name>` y de `--base`.
- `config-layers`: la capa de repo de un comando que opera sobre otro repo sale del worktree principal de ese repo.
- `shell-integration`: las completions de `wt cd` (y de `wt path`) suman los worktrees de otros repos, la forma calificada y las descripciones.

## Non-goals

- **`wt prune` en otros repos, o en todos.** Su único target es el repo actual; "en todos" es trabajo en lote (`batch-execution`).
- **`wt exec` y `wt each`**: el tercer change de M2, `batch-execution`. Van a heredar esta resolución.
- **Prefijo y mayúsculas en nombres de worktree**: siguen exactos y case-sensitive, también en otros repos.
- **Desambiguar con un nombre suelto con ruta** (`wt cd GIT/api`): el sufijo de ruta solo existe en la forma calificada (`GIT/api:`). En la cascada, un target con `/` sigue siendo una rama del repo actual.
- **`:feat` como "solo en el repo actual"**: hoy es un error de uso. Si hiciera falta después, pasar de error a feature es aditivo.
- **Caché del listado de worktrees, o leer `.git/worktrees/` sin git**: git sigue siendo la única fuente de verdad, con las llamadas en paralelo. Si la medición en Windows no da, leer el filesystem es el plan B, y no cambia las specs.
- **`wt list --full`**, el picker y el tablero: M4 y M6.
- **Nombrar el repo en la salida de texto de `remove`, `lock` y `unlock`**: ya imprimen la ruta completa, que identifica al worktree.

## Milestone

M2, segundo corte de tres (`workspace-discovery` → **`cross-repo-resolution`** → `batch-execution`). Ver `docs/design/product.md` §7, tabla "M2, desglosado en changes". Este change suma `wt create <repo>:<name>` a lo que esa tabla le asignaba (`cd`, `path`, `remove`, `lock`, `unlock` y `--all-repos`) y actualiza la fila.

Rama: `v0.2/cross-repo-resolution`. Publica `v0.2.0-alpha.2`.

## Diferencias por OS

Sí, y se especifican por separado:

- **Separador del sufijo de ruta**: `/` en los tres OS; en Windows también `\`. En macOS y Linux `\` es un carácter válido en un nombre de directorio, así que ahí no separa.
- **Mayúsculas**: las reglas de matcheo (nombres de repo, sufijos, nombres visibles) son iguales en los tres OS.
- **Rutas** en la salida, los hints y `--json`: nativas (`C:\Repos\api` en Windows).
- **`:` en nombres de directorio**: solo es posible en macOS y Linux, así que el caso borde de **Se quita** no existe en Windows.
- **Costo**: lanzar git cuesta más en Windows. El comportamiento es el mismo; la latencia se mide en Windows (tarea **[Windows]**).
- **Shells**: en zsh con `EXTENDED_GLOB`, `api:^` va entre comillas, como ya pasa con `^`. El `:` dentro de una palabra no es especial en zsh, bash, fish ni pwsh. Las completions con `:` se prueban en las cuatro shells reales.

## Impact

- **Código**:
  - `internal/workspace` suma las decisiones puras: partir un target calificado, el sufijo de ruta, los nombres visibles, y la búsqueda entre los worktrees de varios repos.
  - `internal/cli` suma `wt path`, el escaneo en paralelo de varios repos (el único efecto nuevo), la resolución compartida entre `cd`, `path`, `remove`, `lock`, `unlock` y `create`, la carga de config con la capa del repo destino, `--all-repos` en `wt list`, y las completions nuevas.
  - `internal/worktree.Resolve` no cambia: se reusa para el lado del nombre.
- **Dependencias nuevas**: ninguna. El paralelismo usa la biblioteca estándar.
- **Contrato público**:
  - un comando (`wt path`), un esquema (`wt.path.v1`) y un flag (`--all-repos`);
  - campos nuevos en `wt.list.v1`, solo con el flag;
  - la sintaxis `<repo>:<name>`;
  - nuevos casos que resuelven donde antes había exit 3, y mensajes y hints de exit 3 y 4 que cambian.
  - Exit codes, claves de config y variables de entorno sin cambios.
- **CI**: sin jobs nuevos. Los tests corren contra varios repos reales en directorios temporales, en los tres OS, y las completions con `:` en las cuatro shells.
- **Documentación**: `docs/design/product.md` §7 (fila de `cross-repo-resolution`) y `CHANGELOG.md`.
