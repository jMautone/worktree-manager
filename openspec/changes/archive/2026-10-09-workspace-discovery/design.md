# Design: workspace-discovery

## Context

Este change es el primero que lee el filesystem más allá del repo actual. Sigue el patrón de M1: `cli.Run(ctx, Env) int` como único punto de entrada, la decisión pura en un paquete sin efectos (con `goos` como parámetro, para que las reglas de los tres OS se prueben desde cualquiera), git detrás de `git.Runner`, errores tipados `*cli.Error` que `Run` traduce a exit code, y tests contra árboles reales en `t.TempDir()` con `internal/testutil`. El por qué está en `proposal.md`; el comportamiento, en `specs/`.

Lo que ya existe y se reusa:

- `internal/workspace` tiene solo `doc.go`; su comentario ya describe este paquete.
- `config.Keys()` devuelve el `Registry`, y cada `Key` tiene `Validate func(any) (any, error)` y `FromEnv func(string) (any, error)`. Hoy no hay claves lista ni enteras, y ninguna validación depende del OS.
- `cdDestination` (`internal/cli/cd.go`) usa `requireRepository`, que falla con exit 3 fuera de un repo, y después `resolveTarget` (`internal/cli/target.go`), que mapea `worktree.NotFoundError` y `worktree.AmbiguousError` a exit 3 y 4.
- `completeNames` (`internal/cli/target.go`) no carga config, a propósito: así una completion nunca imprime un warning.
- `a.warn` ya calla los warnings con `--json`. `renderTable` y `cell` (`internal/cli/table.go`) alinean por ancho visible.
- `a.home()` da `HOME`, o `USERPROFILE` en Windows; `config/paths.go` tiene un `join(goos, …)` puro.

Hechos observados (git 2.51.2, macOS, go-toml v2.4.3) que moldean el diseño:

1. El `.git` de un worktree enlazado es un archivo `gitdir: <ruta absoluta>/.git/worktrees/<id>`, y ese directorio contiene un archivo `commondir`.
2. Layout `.bare`: `proj/.git` es un archivo `gitdir: ./.bare` (ruta **relativa** al directorio del `.git`), y `proj/.bare` no tiene `commondir`. Desde `proj`, `git worktree list --porcelain` reporta como principal a `proj/.bare` (con `bare`), no a `proj`.
3. `git clone --separate-git-dir`: el `.git` es un archivo con ruta absoluta, sin `commondir`. El principal que reporta `git worktree list` es el directorio del checkout.
4. `toml.Unmarshal` en un `map[string]any` decodifica un array como `[]any` (vacío, `[]any{}`), un entero como `int64` y `2.0` como `float64`.
5. Desde Go 1.23, en Windows `filepath.EvalSymlinks` no evalúa mount points (junctions), y `Lstat` reporta un junction como `ModeIrregular`, no como `ModeSymlink` (notas de Go 1.23, `GODEBUG=winsymlink`). Comparar rutas "reales" como texto no alcanza para deduplicar en Windows.

## Goals / Non-Goals

**Goals:**
- Que el recorrido sea la única pieza que toca el filesystem, y que todo lo que decide (qué es un repo, qué ruta queda, en qué orden, a qué repo resuelve un nombre) sea puro y se pruebe con tablas.
- Que un `wt cd` que resuelve en el repo actual no recorra el workspace: el costo nuevo aparece solo cuando hoy habría un exit 3.
- Que la identidad de un directorio no dependa de comparar texto, para que symlinks, junctions y mayúsculas den lo mismo en los tres OS.

**Non-Goals:**
- Caché del descubrimiento entre corridas.
- Recorrido en paralelo: con `repos_depth` ≤ 3 y sin subprocesos, uno secuencial alcanza; se mide si alguna vez hace falta.
- Generalizar el tipo "lista" de la config más allá de rutas: no hay otra clave lista todavía.

## Decisions

### D1. Paquete `internal/workspace`: qué es puro y qué es efecto

```
  config (repos_root, repos_depth)
        |
        v
  Roots(raw, goos, home)        PURO   expande ~, forma nativa
        |
        v
  Walk(roots, depth)            EFECTO ReadDir, Stat, ReadFile, FileID
        |   -> []Found (ruta bajo la raiz, raiz, hechos del .git, IDs)
        v
  Classify(GitEntry)            PURO   repo / worktree enlazado / roto / nada
  List(found)                   PURO   dedupe por ID, orden NAME/ruta
        |
        +--> Current(repos, id)  PURO   que fila lleva @
        +--> Match(repos, x)     PURO   los tres pasos, ambiguedad
```

```go
// Root es una raíz de repos_root: Raw como está escrita, Path expandida y nativa.
type Root struct{ Raw, Path string }

// FileID identifica un directorio en disco, sin importar por qué camino se llegó.
type FileID struct{ Dev, Ino uint64 } // en Windows: volumen e índice de archivo

// Repo es un repositorio descubierto.
type Repo struct {
	Name, Path, Root string // Path: ruta bajo la raíz, nativa, sin resolver links
	ID, GitdirID     FileID // el directorio, y su directorio git (para @)
}

func Roots(raw []string, goos, home string) ([]Root, []string)        // warnings
func Classify(g GitEntry) Kind                                         // KindNone, KindRepo, KindLinked, KindBroken
func ParseGitFile(content []byte, dir, goos string) (string, bool)      // "gitdir: <ruta>", relativa a dir
func List(found []Found, goos string) []Repo                           // dedupe + orden
func Current(repos []Repo, id FileID) int                              // -1 si ninguno
func Match(repos []Repo, target string) (Repo, error)                  // *NotFoundError, *AmbiguousError
func Walk(roots []Root, depth int) ([]Found, []string)                 // efecto; warnings
```

`Walk` es el único efecto, y vive en `walk.go` con `fileID` en `fileid_unix.go` / `fileid_windows.go`. Todo lo demás no importa `os`.

*Alternativa descartada:* que `Walk` reciba una interfaz de filesystem falsa para probarlo sin disco. La regla del proyecto es testear contra árboles reales, y lo frágil del recorrido (links, junctions, permisos) es justo lo que un falso no reproduce.

### D2. Qué es un repo: hechos en el borde, decisión pura

En cada directorio candidato, `Walk` hace `Lstat(<dir>/.git)` y arma un `GitEntry` con hechos, sin decidir:

| `.git` | Hechos que junta | `Classify` |
|---|---|---|
| no existe | — | `KindNone`: se baja si queda profundidad |
| directorio | — | `KindRepo`; el gitdir es `<dir>/.git` |
| archivo | contenido; `ParseGitFile` → gitdir; si el gitdir es un directorio; si tiene `commondir` | con `commondir`: `KindLinked`; sin `commondir`: `KindRepo`; gitdir ilegible o inexistente: `KindBroken` |
| symlink u otro | `Stat` (siguiéndolo) y se trata como directorio o archivo | igual que arriba |

Cualquier `.git` existente, sea del tipo que sea, corta el descenso (`KindNone` es el único que baja). `KindBroken` cubre un worktree huérfano cuyo repo se borró: no es un repo, y tampoco se baja en él.

`commondir` es la marca que git usa para un gitdir de worktree enlazado (`gitrepository-layout(5)`); los gitdirs de `.bare` y de `--separate-git-dir` no lo tienen (hechos 1-3). `ParseGitFile` acepta `gitdir: ` con espacios y `\r\n` al final, y resuelve una ruta relativa contra el directorio del `.git`.

*Alternativa descartada:* `git rev-parse --git-common-dir` por candidato. Son N subprocesos por cada `wt cd` que cae al workspace y por cada TAB.
*Alternativa descartada:* reconocer un worktree enlazado porque la ruta del gitdir contiene `/worktrees/`. Un repo dentro de una carpeta llamada `worktrees` daría un falso positivo; `commondir` es la marca que usa git.

### D3. El recorrido

- Las raíces se recorren en el orden de `repos_root`. Dentro de cada directorio, `os.ReadDir` (que ya ordena por nombre). La raíz misma no es candidata.
- Una entrada es candidata si es un directorio, o si su `Type()` es `ModeSymlink` o `ModeIrregular` y `os.Stat` (que sigue el link o el junction) dice que es un directorio. Por eso un junction de Windows, que desde Go 1.23 llega como `ModeIrregular` (hecho 5), se trata igual que un symlink.
- La ruta de cada candidato se arma con `filepath.Join` sobre la ruta bajo la raíz, sin resolver links: es la que el usuario ve en el prompt.
- `ReadDir` que falla en un subdirectorio (permisos) → se saltea sin warning. Raíz que no existe o no es un directorio → warning `repository root not found: <path>` o `repository root is not a directory: <path>`, y se sigue.
- Los nombres que empiezan con `.` no tienen trato especial.
- Los ciclos de symlinks no hacen falta detectarlos: `repos_depth` ≤ 3 acota el recorrido, y `List` deduplica lo que se encuentre dos veces.

### D4. Identidad de un directorio: `FileID`

`List` deduplica por `Repo.ID`, y `Current` compara IDs. `fileID(path)` sigue links:

| OS | Cómo |
|---|---|
| macOS, Linux | `os.Stat` → `Sys().(*syscall.Stat_t)` → `{Dev, Ino}` |
| Windows | `windows.CreateFile` con `FILE_FLAG_BACKUP_SEMANTICS` (sin `FILE_FLAG_OPEN_REPARSE_POINT`, así sigue el junction) → `windows.GetFileInformationByHandle` → `{VolumeSerialNumber, FileIndexHigh<<32 \| FileIndexLow}` |

`golang.org/x/sys/windows` ya es dependencia (lo usa `internal/term`).

*Alternativa descartada:* `filepath.EvalSymlinks` + comparar sin mayúsculas en darwin/windows. No resuelve junctions desde Go 1.23 (hecho 5) y se equivoca en volúmenes APFS case-sensitive.
*Alternativa descartada:* `os.SameFile` de a pares. Funciona, pero obliga a pasarle a `List` una función de igualdad con efectos, y un `FileID` comparable se prueba con valores inventados.

### D5. `List`, `Current` y `Match`

- `List`: recorre `found` en el orden de `Walk` (raíz por raíz; dentro de cada raíz, por ruta) y se queda con el primer `Found` de cada `ID`. Eso implementa "la primera raíz y, dentro de ella, la ruta que ordena primero". Después ordena por `strings.ToLower(Name)` y desempata por ruta, con el mismo comparador que `worktree.Build`.
- `Current(repos, id)`: la fila cuyo `ID` o `GitdirID` es `id`. El `id` es el `FileID` del worktree principal que reporta `git worktree list` para el directorio de trabajo. Así se cubren los tres layouts: en un repo normal y en `--separate-git-dir`, el principal es el directorio del repo (`ID`); en `.bare`, el principal es el gitdir `proj/.bare` (`GitdirID`, hecho 2). Desde un worktree enlazado, el principal sigue siendo el del repo, así que la fila se marca igual.
- `Match(repos, target)`: target vacío → `*NotFoundError`. Hay tres pasos, y cada uno corre solo si el anterior no encontró nada: igualdad exacta; `strings.EqualFold`; prefijo sin mayúsculas, con `hasPrefixFold` comparando runa a runa con `unicode.SimpleFold` (sin `ToLower`, que cambia longitudes de bytes). Un match → ese repo; más de uno → `*AmbiguousError{Target, Candidates}` en el orden de `List`. No depende de `goos`: la spec pide las mismas reglas en los tres OS.

### D6. Raíces: `Roots(raw, goos, home)`

Puro. Por cada elemento:

- `~` solo → `home`. `~/…`, y en Windows también `~\…` → `home` + el resto.
- Pasa a forma nativa con el separador de `goos` (`/` → `\` en Windows) y limpia `.`, `..` y separadores dobles, sin usar `filepath` (que usa el OS que corre).
- `home` vacío con un `~` → warning `cannot expand ~ in <raw>: HOME is not set` (o `USERPROFILE`) y se saltea esa raíz.

La validación de que cada elemento sea absoluto o empiece con `~` es de la config (D7), así que acá no se repite.

### D7. Config: `Keys(goos)`, listas y enteros

`config.Keys()` pasa a `config.Keys(goos string)`. Los dos llamadores (`configGetCommand` y `loadConfig`) le pasan `a.env.GOOS`. Hace falta porque validar `repos_root` (qué es absoluto) y partir `WT_REPOS_ROOT` (`:` o `;`) dependen del OS, y así las reglas de Windows se prueban desde macOS, como `UserFile`.

| Clave | `Validate` | `FromEnv` | Valor guardado |
|---|---|---|---|
| `repos_root` | `[]any` de strings, cada uno absoluto para `goos` o con `~`; un `string` falla con `expected an array of strings, got a string; write it as ["~/GIT"]` (con el valor real) | `splitPathList(goos)`: corta en `:` o `;`, descarta vacíos, devuelve `[]any` y pasa por el mismo `Validate` | `[]string` (nunca `nil`: `[]string{}`) |
| `repos_depth` | `int64` entre 1 y 3; `float64` u otro tipo falla con el tipo (`describe`) | `strconv.Atoi` | `int` |

Absoluto en macOS/Linux: empieza con `/`. En Windows: `X:\`, `X:/` o `\\`. `isAbs(goos, s)` es puro y vive en `config` junto a `join`.

Los dos `Default` son `[]string{}` y `1`. `InRepo: false` en ambos.

Salida:
- `wt config get`: un `[]string` imprime un elemento por línea (nada si está vacío); el resto, como hoy.
- `wt config list`: `displayValue` muestra un `[]string` como array de TOML, con `strconv.Quote` por elemento (`["~/GIT", "C:\\Repos"]`, que es TOML válido), y uno vacío como `[]`.
- `--json`: `[]string{}` se serializa `[]`, nunca `null`; `int` como número.

*Alternativa descartada:* aceptar también un string suelto en `repos_root`, como v0.9. Contradice "un valor tiene el tipo TOML de su clave" de `config-layers`, y el mensaje de error ya dice cómo escribirlo.
*Alternativa descartada:* un `Registry` global con validadores que leen `runtime.GOOS`. Las reglas de Windows no se podrían probar desde macOS.

### D8. `wt cd`: dónde cae al workspace

`cdDestination` cambia a este orden:

```
  1. `-`                         -> como hoy (no toca repo ni workspace)
  2. repo, notRepo := loadRepository(dir)   (NotRepoError se guarda, no corta)
  3. cfg := loadConfig(repo.root())         (una sola vez; "" fuera de un repo)
  4. si repo != nil:
       resolveTarget(repo, x)  -> ok o exit 4: se devuelve (no hay workspace)
                               -> NotFound / ErrNoCurrent: sigue
  5. `^` o `@`                  -> el error de 4, o gitError(notRepo) si no hay repo
  6. roots vacias               -> el error de hoy (+ hint de repos_root fuera de un repo)
  7. Walk + List + Match        -> destino, o exit 3 / exit 4 de la tabla de navigate
```

Los mensajes y hints de "nada encontrado" salen de una tabla en `target.go` (dentro/fuera de un repo × con/sin raíces), y el mapeo de `workspace.AmbiguousError` a exit 4 (un `hint:` por candidato con su ruta) vive al lado del de `worktree.AmbiguousError`. Los warnings de `Walk` (una raíz que falta) pasan por `a.warn`.

Costo: si el nombre resuelve en el repo actual, no hay recorrido; el orden de lecturas es el mismo de hoy más la carga de config, que ya ocurría.

### D9. `wt repos`

`internal/cli/repos.go`, siguiendo a `list.go`:

1. `workdir`; `loadRepository` opcional (cualquier error → sin marca; `wt repos` no falla por git, solo pierde el `@`).
2. `loadConfig(repo.root())`. `repos_root` vacío → `*Error{Code: ExitError, Msg: "no repository roots configured", Hints: ["set repos_root in <userFile>, e.g. repos_root = [\"~/src\"]"]}`.
3. `Roots` + `Walk` (warnings a `a.warn`) + `List`.
4. Marca: `fileID` del principal del repo de (1) → `Current`.
5. Sin filas → stdout vacío y exit 0. Si hay filas, tabla `[marca, NAME, PATH]` con `renderTable` y el mismo estilo que `wt list` (marca y encabezado en negrita); o `wt.repos.v1`:

```go
type reposJSON struct {
	Schema string     `json:"schema"` // "wt.repos.v1"
	Repos  []repoJSON `json:"repos"`  // nunca null
}
type repoJSON struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Root    string `json:"root"`
	Current bool   `json:"current"`
}
```

`--dry-run` no cambia nada: el comando no escribe.

### D10. Completion de `wt cd`

`completeNames` queda para `remove`/`lock`/`unlock`. `wt cd` pasa a `completeCdTargets`:

1. Los `NAME` locales, como hoy (directorio existente, filtro por prefijo exacto).
2. `loadConfigQuiet(root)`: `loadConfig` se parte en una versión que devuelve los warnings sin imprimirlos, y `loadConfig` los imprime. Si hay error → no hay nombres de repos (los locales sí).
3. `Roots` + `Walk` + `List`, descartando warnings; nombres con `hasPrefixFold(name, toComplete)` que no estén ya en (1).

El filtro sin mayúsculas sirve en fish y pwsh, que muestran el candidato. zsh y bash pueden volver a filtrar con su propio matcher, que por defecto distingue mayúsculas (ver Riesgos). La resolución con Enter no depende de eso.

### D11. Tests

- **Puros** (tablas, con `goos` darwin/linux/windows donde aplica): `Roots` (`~`, `~/x`, `~\x` solo en windows, `C:/x` → `C:\x`, home vacío); `isAbs` y `splitPathList`; `ParseGitFile` (absoluta, relativa, `\r\n`, basura); `Classify`; `List` (dedupe por ID con raíz primera, orden por nombre sin mayúsculas, desempate por ruta); `Current` (por `ID` y por `GitdirID`); `Match` (los tres pasos, exacto antes que sin mayúsculas, exacto antes que prefijo, prefijo ambiguo, vacío, runas no ASCII).
- **`Walk` contra árboles reales** con `testutil`: clone, `api.worktrees/feat` con depth 2, `.bare`, `--separate-git-dir`, bare suelto, sin `.git`, repo dentro de repo con depth 3, `.dotfiles`, raíz inexistente, y por OS: symlink a repo de afuera y symlink duplicado (unix), directorio sin permisos (unix), junction y junction duplicado (Windows, creados con `cmd /c mklink /J`).
- **`fileID`**: el mismo directorio por dos caminos da el mismo ID; dos directorios, IDs distintos; en macOS, con otra capitalización, el mismo ID.
- **Config**: los escenarios de `config-layers` in-process, con `goos` inyectado para los de Windows en los tests de `Keys`.
- **CLI in-process**: los escenarios de `workspace-discovery` y `navigate`, con `WT_DIRECTIVE_CD_FILE` en un temporal, y `__complete` para los de `shell-integration`.
- **Shells reales**: `wt cd <repo>` desde fuera de cualquier repo, a través de la función, en zsh, bash, fish y pwsh.

## Risks / Trade-offs

- [zsh y bash filtran las completions con su matcher, que por defecto distingue mayúsculas: `trend<TAB>` puede no completar `TrendFisher`] → `wt cd trend` + Enter resuelve igual (paso 3). Quien quiera TAB sin mayúsculas configura su shell (`matcher-list` en zsh, `completion-ignore-case` en bash). En Warp el TAB no usa estas completions.
- [Un prefijo pensado para un worktree local lleva a otro repo (`wt cd fea` → `feature-flags`)] → aceptado al explorar el change; se nota en el prompt y `wt cd -` vuelve.
- [Un prefijo único deja de serlo al clonar un repo nuevo, y un script que lo usa pasa a exit 4] → el `--help` de `wt cd` dice que los scripts usen nombres completos; el exit 4 lista los candidatos.
- [Una raíz enorme (`~`, `C:\`) con `repos_depth = 3` hace lento cada `wt cd` que cae al workspace y cada TAB] → el rango tope es 3, el descenso se corta en cada `.git`, y no corre git. Si aparece, la mitigación es una caché, que es un non-goal ahora.
- [`os.Stat` o `CreateFile` se comportan distinto con junctions según la versión de Go o de Windows] → test con junctions reales en `windows-latest` (tarea **[Windows]**); si falla, el arreglo queda en `walk.go`/`fileid_windows.go`, sin tocar las decisiones puras ni la spec.
- [Un worktree enlazado cuyo repo vive fuera de las raíces queda invisible] → es correcto para este change: no es un repo, y los worktrees de otros repos llegan con `cross-repo-resolution`.
- [`config.Keys(goos)` cambia una firma interna usada en tests] → los dos llamadores y `keys_test.go` se actualizan en la misma tarea.

## Migration Plan

No hay migración: con `repos_root` vacío (el default), todo se comporta como en v0.1.0, salvo el hint nuevo de `wt cd <x>` fuera de un repo. Para usarlo, una línea en el archivo de usuario: `repos_root = ["~/Documents/GIT"]`. Rollback: revertir el squash.
