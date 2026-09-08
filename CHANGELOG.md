# Changelog

Formato basado en [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Este archivo se actualiza por fase, siguiendo la ejecucion de
[PLAN-CORRECCIONES.md](PLAN-CORRECCIONES.md).

## [Unreleased]

### Fase 0 — Red de contencion

- **M6**: `WT_CONFIG_ONLY=1` aisla la config efectiva a `defaults < WT_CONFIG`,
  ignorando la config global del usuario y el `.wt.json` del repo. La suite E2E
  ahora corre con las 11 claves explicitas, sin depender de la maquina.
- **M9**: se agrega `wt.psd1` (manifiesto del modulo, version `0.1.0`), el
  comando `wt version`, `.gitignore`, `LICENSE` (MIT), este `CHANGELOG.md`,
  CI en GitHub Actions (`windows-latest`, matriz `powershell`/`pwsh`) y
  `PSScriptAnalyzerSettings.psd1` con las reglas por defecto.

### Fase 1 — Riesgo alto

- **A1**: el `.wt.json` del repo ahora pasa por una lista blanca
  (`Get-WtRepoConfigAllowedKeys` / `Select-WtRepoConfigKeys`, funciones puras):
  solo `worktreeRootTemplate`, `defaultBase`, `branchPrefix` y
  `fetchBeforeCreate`. Cualquier otra clave (`editor`, `warpPath`, ...) se
  descarta con un warning que nombra el archivo y la clave, para que un
  `.wt.json` de un repo clonado no pueda decidir que ejecutable lanza
  `wt open`.
- **A2**: `wt remove --delete-branch` ya no corre `git branch -D` a ciegas.
  Ahora intenta `git branch -d` (seguro); si la rama tiene commits sin
  mergear, aborta sin borrarla (el worktree si se elimina) y explica como
  forzarlo con el nuevo flag `--force-branch`. `--force` queda exclusivamente
  para `git worktree remove --force` (arbol de trabajo sucio) y nunca implica
  `--force-branch`. `Remove-WtWorktree` declara `SupportsShouldProcess` y
  confirma worktree y rama por separado.
- **A3**: `wt create` ahora cumple la promesa del README ("VS Code y Warp con
  un solo comando"). Nueva clave `openOnCreate` (default `all` = editor +
  agente; tambien `editor` y `none`) y flags `--all`/`--code`/`--agent`/
  `--terminal` en `create` (ganan a la config); `--no-open` sigue ganando a
  todo. `Get-WtCreateOpenPlan` (funcion pura) resuelve la combinacion.

### Fase 2 — Robustez

- **M1**: `Invoke-WtProcess` capturaba stdout y stderr mezclados (`2>&1`) y
  `Text`/`Output` incluian ambos; un warning de git en una operacion exitosa
  podia colarse como "la ruta" o "la rama". Ahora separa por tipo de registro
  (PowerShell envuelve stderr de un proceso externo en `ErrorRecord`) y expone
  `StdOut`/`StdErr`/`ErrorText` ademas de `Text` (solo stdout) y `Output`
  (la mezcla cruda, por compatibilidad). `Find-WtMainRoot`, `Get-WtCurrentRoot`,
  `Get-WtBranchAt`, `Get-WtWorktrees`, `Get-WtNodeVersion`, `Get-WtNodeMajor` y
  `Test-WtRemote` pasan a leer `StdOut`; los mensajes de error de
  `Invoke-WtGit` y de `remove --force-branch` prefieren `ErrorText`.
- **M5**: `Set-WtConfigValue` solo verificaba que la clave existiera; `terminal
  foo`, `warpAgentTarget xyz` o un `worktreeRootTemplate` sin `{name}` se
  guardaban sin protesta y fallaban despues, lejos de la causa. Nueva
  `Test-WtConfigValue`/`Assert-WtConfigValue` (funcion pura, tabla de
  validadores por clave) reutilizada por `Set-WtConfigValue` (rechaza antes de
  escribir, sin tocar el archivo) y por `Read-WtConfigFile` (descarta claves
  invalidas de cualquier archivo con un warning, nunca lanza). Nueva fila
  `config valida` en `wt doctor`.
- **M2**: los tab configs de Warp se llamaban `wt-<kind>-<nombre>.toml`, sin
  referencia al repo; dos worktrees homonimos (`feature-a`) de repos distintos
  escribian el mismo archivo, y nada los borraba nunca. Nuevo
  `Get-WtPathHash` (SHA1 de 8 hex sobre la ruta normalizada) desambigua el
  nombre de archivo (`wt-<kind>-<nombre>-<hash>.toml`); `wt remove` borra los
  tab configs del worktree que elimina (agente y terminal) antes de borrarlo;
  nuevo comando `wt clean` depura los que quedaron huerfanos por otras vias.
- **M3**: `Resolve-WtRepoContext` hacia `Set-WtLocation` como parte de
  resolver el nombre; fuera de un repo, `wt path logging` te movia ademas de
  imprimir. Ahora es pura (devuelve `ShouldRelocate`) y el efecto queda en el
  llamador: `Open-WtWorktree` relocaliza cuando corresponde, `Invoke-WtPathCommand`
  nunca lo hace.
- **M4**: `Find-WtRepoOwningWorktree` devolvia el primer repo por orden
  alfabetico entre los que tenian un worktree homonimo, mientras que
  `Resolve-WtRepoDir` fallaba y listaba las coincidencias para la misma clase
  de ambiguedad — dos criterios distintos para el mismo problema, y el
  usuario veia un exito silencioso con el repo equivocado. Nueva
  `Find-WtReposOwningWorktree` devuelve **todas** las coincidencias;
  `Resolve-WtRepoContext` falla y las lista (mismo tono que
  `Resolve-WtRepoDir`) cuando hay mas de una.
- **M7**: `install.ps1` declaraba `SupportsShouldProcess` pero nunca llamaba a
  `$PSCmdlet.ShouldProcess`: `-WhatIf` se aceptaba y el perfil se modificaba
  igual. Las tres escrituras (bloque del perfil, directorio de config, copia
  de `config.example.json`) quedan detras de `ShouldProcess`. Se agrega
  tambien un bug real de reversibilidad encontrado al verificar esto a mano:
  `Add-Content` sumaba un salto de linea propio ademas del bloque, y
  `uninstall.ps1` (nuevo, tambien con `SupportsShouldProcess`) no podia dejar
  el resto del perfil byte a byte igual por ese sobrante; ahora usa
  `-NoNewline`. `uninstall.ps1` no toca `~\.wt\config.json` salvo
  `-RemoveConfig`.
- **M8**: `Get-WtAgentCommands` inyectaba un `if { Write-Warning ... }` en
  sintaxis de PowerShell entre los `commands` del tab del agente; si el shell
  por defecto de Warp era bash, WSL o cmd, era un error de sintaxis en cada
  tab. El binario del agente era ademas la cadena literal `copilot`. Nuevas
  claves `agentCommand` (default `copilot`) y `agentShell` (`powershell` por
  defecto, tambien `bash` y `none`); el chequeo de version de Node salio del
  tab (vive solo en `wt doctor`, que ya lo hacia). Ninguna de las dos entra
  en la lista blanca de `.wt.json` (A1).

### Fase 3 — Deuda de mantenimiento

- **B2**: `Common.ps1` declara ser el unico punto que escribe a consola pero
  no exponia un helper de error; los dos `catch` de la consola resolvian con
  `Write-Host ... -ForegroundColor Red` duplicado. Nuevo `Write-WtError`
  junto a los demas helpers de presentacion, usado en ambos `catch`.
- **B1**: superficie muerta eliminada: `--no-code`/`--no-terminal` de `open`
  (aceptados por el parser pero sin documentar ni usarse), el parametro
  `-NoTerminal` de `Open-WtWorktree` (ninguna linea lo leia; era un no-op),
  `-NoCode` de `Get-WtOpenPlan` (sin llamadores tras A3) y `-RepoRoot` de
  `Invoke-WtConsoleCreate` (sin uso; `New-WtWorktree` resuelve el repo solo).
- **B3**: `Set-Content -Encoding UTF8` en PowerShell 5.1 antepone BOM
  (`EF BB BF`), que muchos parsers TOML rechazan. Nuevo
  `Set-WtFileUtf8NoBom` (`[IO.File]::WriteAllText` con `UTF8Encoding($false)`)
  usado tanto para los tab configs de Warp como para `config.json`.
- **B4**: `ConvertFrom-WtWorktreePorcelain` manejaba `bare`, `detached` y
  `prunable`, pero no `locked`. Se agregan `IsLocked`/`LockReason` al parser y
  al modelo (`New-WtWorktreeInfo`), marcados en `wt list`, `--json` y la
  consola. `wt remove` traduce el error crudo de git en un mensaje propio que
  nombra el motivo y sugiere `wt unlock`. Nuevos comandos `wt lock <nombre>
  [--reason <texto>]` y `wt unlock <nombre>` sobre `git worktree lock`/
  `unlock`.
- **B5**: tres cambios de costo de arranque y llamadas a git redundantes,
  independientes entre si:
  1. El bloque del perfil importa el modulo **una sola vez** (al abrir la
     terminal) y define `function wt { Invoke-Wt @args }`, en vez de invocar
     `wt.ps1` (que reimporta con `-Force`) en cada llamada. `wt.ps1` se
     conserva para el modo `-File`.
  2. `Get-WtWorktrees` cachea por `RepoRoot` dentro de una misma invocacion
     (relevante para `wt open <worktree-de-otro-repo>`, que puede recorrer
     varios repos de `reposRoot`); `Invoke-Wt` la invalida junto a la cache
     de config al empezar y despues de
     `create`/`remove`/`lock`/`unlock`/`prune`, y `wt console` la invalida
     en cada vuelta del menu (una sola invocacion de `Invoke-Wt` cubre toda
     la sesion interactiva).
  3. `Get-WtConfig` acepta `-RepoRoot` opcional para no repetir el
     `git rev-parse` que hace por su cuenta cuando el llamador (`create`,
     `open`) ya lo resolvio.

  Medido con `Measure-Command` sobre 15 llamadas a `wt list` (salida
  suprimida), antes/despues del cambio (1): **237 ms/llamada -> 176 ms/llamada**
  (~26% menos), atribuible a evitar el reimport del modulo por llamada.
  El costo restante es sobre todo llamadas a `git` como proceso externo.
- **B6**: se define y documenta el contrato de codigos de salida (`0` OK,
  `1` error de uso, `2` error de git o del entorno). `Invoke-Wt` separa el
  parseo (`ConvertFrom-WtArgs`; cualquier error ahi es por definicion un
  problema de sintaxis, exit `1`) del despacho (`Invoke-WtDispatch`, en un
  `try/catch` propio: los mensajes que empiezan con `Uso:` son `1`, el resto
  `2`) y setea `$global:LASTEXITCODE` en ambos modos de invocacion; `wt.ps1`
  hace `exit $LASTEXITCODE` al terminar para el modo `-File`, sin que la
  funcion del perfil llame nunca a `exit` (cerraria la terminal del usuario).
  Verificar esto a mano con `wt doctor` reveló un bug real preexistente:
  `Get-Command <cmd> -ErrorAction SilentlyContinue` devuelve `$null` cuando
  el comando no existe, y leer `.Source` de ese `$null` bajo
  `Set-StrictMode -Version Latest` es un error terminante
  (`PropertyNotFoundStrict`) — antes quedaba enmascarado porque
  `$ErrorActionPreference = 'Continue'` lo volvia no terminante y `wt doctor`
  seguia de largo con el detalle vacio. Nuevo `Get-WtCommandSource` (helper
  puro y null-safe) reemplaza los cinco accesos directos a `.Source` en
  `Get-WtDoctorRows`.
- **B7**: `wt prune` corria con `-v` pero mandaba toda la salida a
  `Out-Null`, sin mostrar que habia depurado. Ahora emite cada linea con
  `Write-WtDetail`, o dice explicitamente "No habia metadatos de worktrees
  obsoletos" si no depuro nada. La salida verbosa de
  `git worktree prune -v` va a **stderr**, no a stdout (descubierto al
  implementar esto); sale de `StdErr`, no de `StdOut` (ver M1).
- **B8**: `Get-WtConfig` llamaba a `Find-WtMainRoot` (de `Repo`, que invoca
  git) para ubicar el `.wt.json` del repo, contradiciendo el diagrama de
  dependencias del README y el orden de carga de `wt.psm1` (`Config`
  supuestamente depende solo de `Common`). Nuevo `Find-WtRepoConfigFile` (en
  `Common`, sin invocar git) resuelve lo mismo caminando el filesystem: sube
  directorios buscando `.git`; si es un directorio, esa es la raíz
  principal; si es un archivo (worktree), sigue el puntero `gitdir:` dos
  niveles hasta la raíz principal — mismo resultado que
  `git rev-parse --git-common-dir`, sin el subproceso. Un submódulo (u otro
  formato de `.git`-archivo) no se soporta: se trata como si no hubiera
  repo en ese nivel, nunca se adivina una raíz incorrecta.
- **B9**: `New-WtWorktree` terminaba con `return $path`, y ni el dispatcher
  del CLI ni la consola lo silenciaban: la ruta desnuda del worktree salia
  por stdout ademas de los mensajes de progreso. La funcion sigue
  devolviendo la ruta (documentado en `.OUTPUTS`, util para quien la llame
  directamente como funcion del modulo); sus dos llamadores (`Invoke-
  WtDispatch` y `Invoke-WtConsoleCreate`) mandan el retorno a `Out-Null`, asi
  stdout queda reservado a `wt path` y `--json`.
