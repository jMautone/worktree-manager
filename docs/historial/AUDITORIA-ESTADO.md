# Estado de la auditoría — Worktree Manager

Tablero de seguimiento de [AUDITORIA.md](AUDITORIA.md) (7 de septiembre de 2026),
ejecutado siguiendo [PLAN-CORRECCIONES.md](PLAN-CORRECCIONES.md). Los 24 hallazgos
están cerrados. El detalle de cada cambio vive en [CHANGELOG.md](CHANGELOG.md); este
documento es solo el cruce hallazgo → commit, para que la próxima auditoría tenga
contra qué comparar.

**Antes → después:** 69 + 141 aserciones (0 fallas) → **163 + 203 aserciones (0 fallas)**.

## Riesgo alto

| # | Hallazgo | Estado | Commit |
|---|---|---|---|
| A1 | `.wt.json` de un repo clonado puede elegir qué ejecutable lanza `wt` | Cerrado | `a175488` |
| A2 | `remove --delete-branch` borra con `-D`: descarta commits sin avisar | Cerrado | `ae9b876` |
| A3 | `wt create` nunca abre el agente, pero el README y la ayuda dicen que sí | Cerrado | `d9b624c` |

## Riesgo medio

| # | Hallazgo | Estado | Commit |
|---|---|---|---|
| M1 | stderr se mezcla con stdout y puede corromper rutas y nombres de rama | Cerrado | `554ef32` |
| M2 | Los tab configs de Warp colisionan entre repos y nadie los borra | Cerrado | `0efcf5a` |
| M3 | `wt path` cambia el directorio de tu terminal | Cerrado | `bbafdb6` |
| M4 | Un nombre de worktree ambiguo entre repos se resuelve en silencio | Cerrado | `08081b5` |
| M5 | `wt config set` valida la clave pero nunca el valor | Cerrado | `83fda0e` |
| M6 | Las pruebas E2E heredan la config global del usuario | Cerrado | `8f4425a` |
| M7 | `install.ps1` declara `SupportsShouldProcess` y no lo usa; no hay desinstalador | Cerrado | `192881e` |
| M8 | El agente está cableado a `copilot` y a sintaxis PowerShell dentro de Warp | Cerrado | `41c1d97` |
| M9 | Sin manifiesto, sin versión y sin integración continua | Cerrado | `b44eccf` |

## Riesgo bajo · deuda de mantenimiento

| # | Hallazgo | Estado | Commit |
|---|---|---|---|
| B1 | Superficie muerta y no documentada | Cerrado | `34458e9` |
| B2 | La regla «un solo lugar imprime» está rota para los errores | Cerrado | `8b87154` |
| B3 | El TOML de Warp y la config se escriben con BOM | Cerrado | `66a8773` |
| B4 | El parser del porcelain ignora el atributo `locked` | Cerrado | `18482c5` |
| B5 | Costo de arranque y llamadas a git redundantes | Cerrado | `b117ef9` |
| B6 | No hay contrato de código de salida | Cerrado | `e2e7658` |
| B7 | `wt prune` pide verbose y tira la salida | Cerrado | `8f17714` |
| B8 | El diagrama de dependencias del README no refleja el código | Cerrado | `212830c` |
| B9 | `create` emite la ruta al pipeline además de imprimirla | Cerrado | `52961c4` |
| B10 | Sensibilidad a mayúsculas inconsistente al resolver nombres | Cerrado | `91e3c86` |
| B11 | `reposRoot` es un solo nivel y una sola raíz | Cerrado | `f4725aa` |
| B12 | `wt cd` no puede llevarte a un worktree | Cerrado | `7ac600e` |

## Hallazgos surgidos durante la implementación (no estaban en la auditoría original)

Efecto de "verificar antes de confiar" (rule de trabajo del plan): al implementar
varios ítems, correr las pruebas reveló bugs reales preexistentes, distintos del
hallazgo que se estaba cerrando. Documentados en detalle en sus commits:

- **Bug de reversibilidad en `install.ps1`** (`Add-Content` sumaba un salto de línea
  propio, impidiendo que `uninstall.ps1` dejara el resto del perfil byte a byte
  igual) — encontrado al verificar M7 a mano, corregido en el mismo commit
  (`192881e`).
- **`PropertyNotFoundStrict` en `Get-WtDoctorRows`** (`Get-Command` devuelve `$null`
  cuando el comando no existe; leer `.Source` de ese `$null` bajo
  `Set-StrictMode -Version Latest` es un error terminante que quedaba enmascarado
  porque `$ErrorActionPreference = 'Continue'` lo volvía no terminante) — encontrado
  al implementar B6 (el nuevo `try/catch` de `Invoke-Wt` lo hizo visible), corregido
  con `Get-WtCommandSource` en el mismo commit (`e2e7658`); reapareció en la misma
  familia de bug (un array de 1 elemento se "desenvuelve" a escalar al retornar de
  una función salvo que el llamador también envuelva con `@()`) durante B11
  (`f4725aa`).
- **Discrepancia de mayúsculas en `-eq`** (el operador `-eq` de PowerShell no
  distingue mayúsculas por defecto, al revés de lo que el código de B10 daba a
  entender; hacía falta `-ceq`) — encontrado al documentar B10, corregido en el
  mismo commit (`91e3c86`).

## Funcionalidades sugeridas (AUDITORIA.md, no parte de este plan)

Las 12 propuestas de la sección "Funcionalidades sugeridas" de AUDITORIA.md
**no** forman parte de este plan de correcciones — varias quedaron cubiertas como
efecto colateral (`wt clean`/`lock`/`unlock` en M2/B4, `wt version` + manifiesto en
M9, agente configurable en M8, raíces múltiples en B11) y están marcadas como tales
en [PLAN-CORRECCIONES.md](PLAN-CORRECCIONES.md). El resto (hooks de creación,
`wt status`, autocompletado, `wt exec`/`each`, `wt sync`, agente en Windows
Terminal, `--dry-run` global) sigue pendiente, a evaluar en
[PLAN-MEJORAS.md](PLAN-MEJORAS.md).
