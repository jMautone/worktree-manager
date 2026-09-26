# Proposal: walking-skeleton

## Why

Hoy `main` no tiene ningún comando: `git-wt` imprime "not implemented" y sale con 1. Antes de construir `cd`, `create` o `remove` hace falta probar el recorrido vertical completo —parseo de la línea de comandos, config, git, salida humana y `--json`, exit codes— en macOS y Windows a la vez, con un comando real y útil. `wt list` del repo actual es ese comando: solo lee, así que ejercita todo el contrato sin riesgo de romper nada, y fija las convenciones que los tres changes siguientes de M1 van a heredar.

## What Changes

- **Nuevo comando `wt list`**: lista los worktrees del repo que contiene el directorio de trabajo, con marcas para el actual (`@`), el principal (`^`), bloqueado y podable. Salida de tabla para humanos y `--json` con esquema versionado.
- **Nuevo comando `wt config`** con subcomandos de solo lectura: `config path` (dónde se buscan los archivos), `config list` (cada clave con su valor efectivo y de qué capa sale), `config get <key>`.
- **Nuevo comando `wt version`** y flag `--version`.
- **Contrato de CLI** que rige a todos los comandos presentes y futuros:
  - flags globales: `--json`, `--dry-run`, `-C <dir>`, `--no-color`, `-h/--help`;
  - validación estricta: flag o comando desconocido es error de uso;
  - tabla de exit codes 0-8 como contrato público (este change usa 0, 1, 2 y 3);
  - errores en stderr con prefijo `wt:`; con `--json`, el error también es JSON.
- **Config por capas**: defaults → archivo de usuario → `.wt.toml` del repo (solo claves en lista blanca) → variables `WT_*`. Formato TOML (decisión D1 de `docs/design/product.md`). Claves de este change: `default_base` y `worktree_path`; sus consumidores llegan con `create-worktree`.
- Ninguna clave, flag ni comando se quita: no existía ninguno.

## Capabilities

### New Capabilities

- `cli-contract`: parseo, despacho, flags globales, validación estricta, exit codes, formato de errores, `--json` versionado, `--dry-run`, `-C`, color, help y versión.
- `config-layers`: ubicación de los archivos por OS, precedencia de capas, lista blanca del archivo del repo, validación de valores y el comando `wt config` de solo lectura.
- `git-worktrees`: cómo se descubre el repo desde un directorio, cómo se enumeran sus worktrees y qué estado se reporta de cada uno (rama, HEAD, detached, bare, bloqueado, podable), incluida la normalización de rutas por OS.
- `list-worktrees`: el comando `wt list` en su forma básica: columnas, marcas, orden, salida `--json`.

### Modified Capabilities

Ninguna. `openspec/specs/` está vacío.

## Non-goals

- `wt list --full`, `--all-repos` y el estado rico (sucio, ahead/behind, integrada, conflicto): son M2 y M4.
- `wt config set` y `wt config edit`: escribir TOML preservando los comentarios del usuario requiere un enfoque propio; se difiere. Mientras tanto el archivo se edita a mano.
- Completions de shell: pertenecen a `shell-integration`. El comando `completion` que cobra agrega por defecto queda deshabilitado.
- Función de shell `wt`: pertenece a `shell-integration`. En este change el binario se invoca como `git-wt` o `git wt`.
- Workspace multi-repo: M2.

## Milestone

M1, primer corte de cuatro (`walking-skeleton` → `shell-integration` → `create-worktree` → `remove-worktree`). Ver `docs/design/product.md` §7.

## Diferencias por OS

Sí, y se especifican por separado:

- Ubicación del archivo de config de usuario: `$XDG_CONFIG_HOME/wt/config.toml` (o `~/.config/wt/config.toml`) en macOS y Linux; `%APPDATA%\wt\config.toml` en Windows.
- Rutas mostradas: git las reporta con `/` en todas las plataformas; `wt` las muestra con el separador nativo (`\` en Windows).
- Identificación del worktree actual: comparación insensible a mayúsculas en macOS y Windows, sensible en Linux; en todas las plataformas se resuelven los enlaces simbólicos antes de comparar (en macOS los temporales viven bajo `/var`, que es un enlace a `/private/var`, y git reporta la ruta real).

## Impact

- Código: `cmd/git-wt`, `internal/cli`, `internal/config`, `internal/git`, y un paquete nuevo `internal/worktree` para el modelo y el armado de la lista (decisión pura, sin git).
- Dependencias nuevas: `github.com/spf13/cobra`, `github.com/pelletier/go-toml/v2`, `golang.org/x/term`, `golang.org/x/sys` (para habilitar secuencias ANSI en la consola de Windows).
- Requisito de runtime: git en el PATH. Se usa `git worktree list --porcelain -z` (disponible desde git 2.36).
- CI: la matrix de 3 OSes existente pasa a correr tests reales de integración contra repos git temporales.
