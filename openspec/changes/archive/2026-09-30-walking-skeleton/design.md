# Design: walking-skeleton

## Context

`main` tiene el scaffold de M0: `cmd/git-wt/main.go` con un `run()` que solo responde a `version`, y seis paquetes `internal/*` que por ahora son únicamente `doc.go`. No hay dependencias. La motivación está en `proposal.md`; el comportamiento, en `specs/`.

Tres hechos observados, que no se deducen de las specs y moldean el diseño:

1. **Dentro de un repo bare, `git rev-parse --show-toplevel` falla** (exit 128, "this operation must be run in a work tree"). En cambio `git worktree list --porcelain -z` funciona desde cualquier lugar del repo, incluido el bare.
2. **En macOS, los temporales viven en `/var/folders/...` pero git reporta `/private/var/folders/...`** (`/var` es un symlink). Cualquier comparación entre el cwd y lo que dice git sin resolver symlinks falla, y los tests lo van a encontrar el primer día.
3. **Formato real del porcelain `-z`**: campos separados por NUL, registros separados por un NUL vacío; `locked` y `prunable` llevan la razón en el mismo campo (`locked on usb drive`), y pueden aparecer sin razón. El bare no tiene `HEAD`.

## Goals / Non-Goals

**Goals:**
- Fijar la forma de construir un comando que van a copiar los tres changes siguientes de M1: dónde vive la decisión, dónde el efecto, cómo se testea.
- Que los tests de integración corran idénticos en macOS, Windows y Linux contra repos git reales.

**Non-Goals:**
- Performance: una sola llamada a git por `wt list`; el caché de porcelain es M4.
- Un logger o modo `--verbose`. Si hace falta para depurar git en Windows, se agrega con su propio change.

## Decisions

### D1. Un solo punto de entrada testeable: `cli.Run`

```go
type Env struct {
    Args   []string
    Stdin  io.Reader
    Stdout, Stderr io.Writer
    Getenv func(string) string
    Environ []string         // entorno de los procesos hijos (git)
    Getwd  func() (string, error)
    GOOS   string            // runtime.GOOS en producción; fijable en tests
    IsTTY  bool              // si stdout es terminal que acepta ANSI
    Version string           // el de -ldflags, que vive en main
}
func Run(ctx context.Context, env Env) int   // devuelve el exit code
```

`main` construye un `Env` real y hace `os.Exit(cli.Run(...))`. Los tests de integración llaman a `cli.Run` en proceso: rápido, sin compilar, y con el entorno completamente controlado (config, `GOOS` para las rutas, TTY). Un puñado de tests E2E compila el binario en `TestMain` y lo ejecuta, para verificar lo que solo un proceso real prueba: el exit code del SO y `git wt` resolviendo `git-wt` en el PATH.

*Alternativa descartada:* solo E2E con el binario. Más lento, y no permite fijar `GOOS` para testear las rutas de Windows desde macOS.

### D2. Decisión pura, efecto en el borde

```
   cli (cobra)  --->  config.Resolve(layers)      pura
        |       --->  git.ParseWorktreeList(b)    pura
        |       --->  worktree.Build(entries,     pura: marca current/main,
        |                        cwdReal, goos)         ordena, normaliza rutas
        |       --->  cli: tabla / JSON          pura sobre []Worktree
        v
   efectos: git.Runner (exec git), os.ReadFile, filepath.EvalSymlinks
```

Paquetes:
- `internal/cli`: cobra, flags globales, mapeo de errores a exit codes, salida.
- `internal/config`: registro de claves, `Paths(goos, getenv, home)` puro, `Resolve` puro sobre capas ya leídas, `Load` como borde.
- `internal/git`: `Runner` (interfaz de una función, `exec.CommandContext` en producción) y `ParseWorktreeList`.
- `internal/worktree` (**paquete nuevo**): el modelo `Worktree` y `Build`, que decide current/main/orden/rutas nativas. No importa `os` ni `exec`.
- `internal/term`: detección de TTY y habilitación de ANSI en Windows.

*Alternativa descartada:* poner `Build` en `internal/git`. Mezcla "qué dice git" con "cómo lo presenta wt"; la segunda parte crece en M4 (estado rico) y no tiene nada que ver con git.

### D3. Errores tipados → exit codes en un solo lugar

```go
type Error struct { Code int; Msg string; Hints []string }
```

Todo error que sale de un comando es un `*cli.Error` o se envuelve como exit 1. `Run` es el único lugar que imprime errores y elige el código, en texto (`wt: msg` + `hint:`) o JSON (`wt.error.v1`) según `--json`. Los errores de parseo de cobra (flag desconocido, args) se convierten a exit 2 con `SetFlagErrorFunc` y validadores `Args` propios; `SilenceErrors` y `SilenceUsage` apagan la salida propia de cobra para que nada escape del formato del contrato.

Los códigos 4-8 se declaran como constantes ahora aunque no se usen: son contrato.

### D4. Cobra y sus ajustes

- `CompletionOptions.DisableDefaultCmd = true`: las completions son de `shell-integration`.
- `wt help <desconocido>`: cobra devuelve exit 0 con "Unknown help topic"; se reemplaza el comando `help` para devolver exit 2, coherente con la validación estricta.
- Sugerencias "did you mean": las de cobra (`SuggestionsMinimumDistance = 2`).
- `-C` no tiene forma larga, como en git. pflag no admite flags solo-shorthand, así que se registra con nombre largo vacío y el template de uso reemplaza la línea `-C, -- dir` por `-C dir` conservando el ancho. Agregar `--directory` habría sumado superficie pública que la spec no pide.
- cobra trae dos dependencias transitivas: `spf13/pflag` y `inconshreveable/mousetrap` (solo Windows).

*Alternativa descartada:* parser propio (como `ConvertFrom-WtArgs` en v0.9). Cobra da help, sugerencias y, en `shell-integration`, completions para 4 shells desde la misma definición: es exactamente la "fuente única" que pide `cli-contract`.

### D5. TOML con `go-toml/v2`, decodificando a mapa

Cada archivo se decodifica a `map[string]any` y se valida contra el registro de claves. Eso resuelve con el mismo código las tres cosas que la spec pide: clave desconocida → warning, clave no permitida en el repo → warning, tipo incorrecto → error con clave y archivo. Los errores de sintaxis de `go-toml/v2` (`*toml.DecodeError`) exponen la posición, de donde sale la línea que la spec exige.

```go
type Key struct {
    Name      string
    Default   any
    InRepo    bool                 // permitido en .wt.toml
    Validate  func(any) (any, error)
}
```

El registro es un parámetro de `Resolve`, no una variable global: los tests inyectan una clave no permitida en el repo para probar ese escenario, que con las dos claves reales de este change no se puede construir.

*Alternativa descartada:* `BurntSushi/toml`. Funciona, pero `go-toml/v2` tiene mejor reporte de posición de error y es el más mantenido hoy (v2.4.3, julio 2026).

### D6. Rutas del archivo de usuario: función pura de `GOOS`

`config.UserFile(goos, getenv, homeDir)` devuelve la ruta sin tocar el filesystem. En macOS se usa `~/.config/wt/config.toml`, **no** `~/Library/Application Support`: es la convención de las herramientas de desarrollo de terminal (git, gh, starship) y la que un usuario de macOS espera encontrar. `os.UserConfigDir()` de Go devolvería `Application Support`, por eso no se usa.

### D7. Worktree actual: comparar contra el porcelain, no preguntarle a git

Por el hecho observado 1, el worktree actual no se obtiene con `rev-parse --show-toplevel`. Se calcula así, en `worktree.Build`:

1. `cwdReal = filepath.EvalSymlinks(cwd)` (borde).
2. Cada ruta del porcelain → `filepath.FromSlash` (git usa `/` también en Windows) → `EvalSymlinks` si existe (una ruta prunable no existe; se usa tal cual).
3. El current es el worktree cuya ruta es prefijo de `cwdReal` por componentes completos (no por string: `/a/repo` no contiene a `/a/repo2`), eligiendo el más profundo.
4. Comparación con `strings.EqualFold` si `goos` es `darwin` o `windows`.

Esto cubre bare (el cwd dentro del bare lo marca como current), anidados y el symlink de `/private` con una sola regla. La raíz del worktree actual es también donde se busca `.wt.toml`.

### D8. Tabla: renderer propio, no `text/tabwriter`

`tabwriter` mide bytes; las secuencias ANSI rompen la alineación en cuanto una fila tiene color y otra no. El renderer calcula anchos sobre el texto plano, rellena, y **después** envuelve en color. Son ~40 líneas y dejan testeable la regla de alineación de la spec quitando ANSI.

### D9. Color en Windows

`golang.org/x/term.IsTerminal` para detectar TTY. En Windows, un archivo `term_windows.go` intenta habilitar `ENABLE_VIRTUAL_TERMINAL_PROCESSING` con `golang.org/x/sys/windows`; si la consola lo rechaza (conhost viejo), el color se apaga. Build tags por archivo, sin `if runtime.GOOS` en el camino caliente.

*Alternativa descartada:* `lipgloss`/`termenv` ahora. Llegan con `bubbletea` en M4; traerlos para colorear dos columnas es adelantar peso.

### D10. Versión

`-ldflags "-X main.version=..."` en release. Si no se inyectó, `runtime/debug.ReadBuildInfo()` provee `vcs.revision` para el campo `commit`. `version` se resuelve antes de cargar config, para cumplir "funciona con config rota".

### D11. Tests contra git real, aislados del usuario

Un helper `internal/testutil` crea repos en `t.TempDir()` y corre git con un entorno controlado:

```
GIT_CONFIG_NOSYSTEM=1
GIT_CONFIG_GLOBAL=<tmp>/gitconfig      (con user.name, user.email, init.defaultBranch=main)
HOME / USERPROFILE / XDG_CONFIG_HOME / APPDATA  -> dentro de <tmp>
WT_CONFIG sin setear salvo que el test lo pida
```

Sin esto, la config global de git y de wt de quien corre los tests se filtra y los resultados dependen de la máquina. Las rutas esperadas se comparan siempre después de `EvalSymlinks`, por el hecho observado 2.

## Risks / Trade-offs

- **[Riesgo] Nombres cortos 8.3 en Windows** (`C:\Users\RUNNER~1\...`): el temp de los runners de GitHub puede venir abreviado y git reportar la forma larga. → `EvalSymlinks` en Windows normaliza a la forma larga; si no alcanza, el CI lo muestra en la primera corrida y se agrega una normalización con `GetLongPathName` en un archivo solo-Windows. Tarea marcada para verificar en Windows. *Verificado en el CI de `windows-latest` (run 36366373000): el temp del runner es `C:\Users\RUNNER~1\...` y los tests de worktree actual, rutas nativas y mayúsculas pasan solo con `EvalSymlinks`; `GetLongPathName` no hizo falta.*
- **[Riesgo] `git wt --help` no llega a `git-wt`**: git intercepta `--help` en subcomandos externos y busca una página de manual. → Limitación conocida de git; `git wt -h` y `git-wt --help` funcionan. Se documenta en el help.
- **[Trade-off] git ≥ 2.36 por `--porcelain -z`**: sin `-z`, una ruta con salto de línea rompe el parseo. Git for Windows y Homebrew traen versiones muy superiores; `wt doctor` (M5) lo va a verificar.
- **[Trade-off] Dos claves de config sin consumidor** (`default_base`, `worktree_path`) hasta `create-worktree`. Se aceptan porque sin al menos una clave el mecanismo de capas no es observable ni testeable.
- **[Riesgo] Warnings de config en stderr en cada comando**: una clave desconocida molesta en cada `wt list`. → Es la intención (el usuario debe arreglar su archivo), y con `--json` se suprimen.

## Migration Plan

No aplica: no hay usuarios de `main`. `v0.9.x` no se toca.
