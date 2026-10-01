# 0001 — Reescribir de cero en Go, multiplataforma

- **Fecha**: 2026-09-21
- **Estado**: aceptada
- **Antecedente**: [0000-analisis-worktrunk.md](0000-analisis-worktrunk.md)

> **Nota (2026-10-01)**: el tag `v0.9.0` que menciona este documento se renombró a `powershell-v0.9.0`, y la rama `v0.9.x` se eliminó: el tag apunta al mismo commit. Ver [0002-versionado-y-releases.md](0002-versionado-y-releases.md).

## Contexto

v0.9.0 es un módulo PowerShell de ~4.2k líneas, con piso PowerShell 5.1, acoplado a Windows + Warp + VS Code + Copilot. El análisis 0000 evaluó tres caminos (§5): (A) portar a PowerShell 7 multiplataforma, (B) reescribir como binario, (C) híbrido. Recomendaba A, con una condición explícita: *"si tus usuarios mac no van a instalar `pwsh`, A no sirve y hay que ir a B directo"*.

Esa condición se cumplió. El desarrollo diario y el uso personal ocurren en macOS; el uso laboral en Windows. Las dos plataformas son de primera clase. Un runtime de PowerShell en macOS, con 150-300 ms de arranque por invocación para algo cuyo verbo central es `cd`, no es "funciona perfectamente en mac".

## Decisión

Reescribir el producto de cero en **Go**, como binario único multiplataforma, en **este mismo repo**.

### Por qué de cero y no portar

El roadmap del análisis ya borraba o invertía una fracción grande del producto actual: los nueve items del §7, el piso 5.1 con sus workarounds, y el modelo de config entero. Portar significaba pagar tres costos que de cero no existen:

- specs de baseline para documentar código que después se borra,
- cuatro capas de compatibilidad apiladas en la config,
- ventanas de deprecación para comandos que en greenfield no nacen.

Lo que la reescritura **no** ahorra es el diseño: el modelo de launchers, los eventos de hook, el esquema de config, el pipeline de `merge` y la semántica de plantillas hay que definirlos igual. Ese diseño ya está hecho, en 0000 y en [`docs/design/product.md`](../design/product.md). Por eso esta reescritura arranca desde una especificación y no desde una hoja en blanco, que es la única clase de reescritura que termina bien.

### Por qué Go y no Rust

| | Go | Rust |
|---|---|---|
| Cross-compile macOS → Windows | `GOOS`/`GOARCH`, sin toolchain cruzada | necesita `cross` o `cargo-zigbuild` |
| TUI (tablero, picker) | `bubbletea`, la mejor del ecosistema | `ratatui` |
| Velocidad de escritura para un dev solo | Alta | Menor |
| Leer worktrunk como referencia | Hay que traducir | Copiar y adaptar |
| Tamaño del binario | ~10-15 MB | ~3-5 MB |

Decide la primera fila combinada con la tercera: se compila para Windows desde macOS todos los días, y la variable que determina si esto se termina es la velocidad de escritura. El costo aceptado es no poder copiar directo de worktrunk (que es Rust) en las partes difíciles.

### Por qué el mismo repo

`github.com/jMautone/worktree-manager` ya existe y es público. GitHub no admite dos repos con el mismo nombre bajo el mismo owner, y no hay nada que preservar que un tag no preserve: 376 KB de historia, sin tags, sin releases, una semana de vida.

Antes de tocar `main` se congeló la línea anterior, y ese orden es lo que hace esto reversible:

```
   tag    v0.9.0    en main (PowerShell puro)
   rama   v0.9.x    desde main
   ambos pusheados ANTES de borrar el arbol PowerShell de main
```

La máquina de trabajo en Windows instala desde el tag y no se entera del cambio hasta que M2 esté listo. No hay big-bang de migración.

### Identidad

Binario `git-wt` (que además expone `git wt <x>` por el mecanismo de subcomandos de git); lo que el usuario tipea es `wt`, siempre a través de la función de shell instalada por `wt shell init`. Esa función es necesaria de todos modos —un binario no puede cambiar el directorio de su shell padre— y de paso resuelve la colisión con `wt.exe` de Windows Terminal, porque una función de shell gana sobre el PATH. Es el mismo mecanismo por el que v0.9 ya convive con Windows Terminal hoy: cero regresión.

## Consecuencias

- Los nueve items del §7 del análisis (quitar VS Code por defecto, quitar Warp, invertir el agente a foreground, etc.) salen gratis: no se implementan.
- `openspec/specs/` arranca vacío y crece solo cuando algo aterriza. No hay baseline que escribir.
- Las specs se escriben agnósticas de lenguaje (regla en `openspec/config.yaml`). Si en algún momento hay que cambiar de lenguaje otra vez, el diseño sobrevive; es lo que permitió que esta decisión no tirara 0000 a la basura.
- Se acepta una ventana sin producto usable en macOS hasta que cierre M1. Mientras tanto, Windows sigue con v0.9.x.
- El criterio de cierre de M1 es "una semana en macOS sin volver a v0.9", para que el rewrite no muera al 70%.

## Alternativas descartadas

- **Portar a PowerShell 7** (opción A de 0000): descartada por la latencia y el peso del runtime en macOS, que es la máquina de desarrollo diario.
- **Repo nuevo con otro nombre**: descartada; se quería conservar el nombre y el repo ya lo tiene.
- **Nombre nuevo para el comando** (`grove`, `arbor`, `wk`): descartada. La colisión con `wt.exe` se resuelve con la función de shell, que hace falta igual.
- **Rust**: descartada por cross-compile y velocidad de escritura, no por capacidad técnica.
