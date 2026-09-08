# Auditoría de Worktree Manager

**Revisión integral de `wt`: consistencia, deuda técnica y oportunidades**
Auditoría técnica · 7 de septiembre de 2026

Un CLI de PowerShell 5.1 que gestiona Git Worktrees y lanza agentes de Copilot CLI en tabs de Warp.
La base es sólida — capas explícitas, lógica pura separada de los efectos y una suite de pruebas
que hoy pasa completa. Lo que sigue son 24 hallazgos verificados contra el código y 12 capacidades
nuevas alineadas con el objetivo declarado del proyecto: correr varios agentes en paralelo sobre el mismo repo.

| | |
|---|---|
| Archivos | 15 |
| PowerShell | ~104 KB |
| Aserciones en verde | 210 |
| Hallazgos | 24 |
| Mejoras propuestas | 12 |

---

## Veredicto

La arquitectura hace lo que promete: `Common → Config/Repo → Workspace/Launch → Commands → Console/Cli`,
con las decisiones (parser, porcelain, plan de apertura, TOML, merge de config) aisladas como funciones puras y
testeadas sin tocar git ni disco. Las dos suites corren limpias: **69 unitarias + 141 E2E, cero fallas**.
Eso ya es más disciplina que la mayoría de las herramientas internas.

Los problemas no están en la estructura sino en tres bordes: **seguridad de la configuración por repo**,
**operaciones destructivas sin red de contención** y **una promesa del README que el código no cumple**.
Debajo de eso hay deuda ordinaria — superficie muerta, artefactos que nadie limpia, pruebas que no están aisladas
de la máquina donde corren — y un techo funcional claro: la herramienta está cableada a un único agente
(`copilot`) y a un único terminal (Warp), justo en el eje donde más valor tendría abrirse.

**Triage:**

- **Alto (3):** ejecución de binarios desde config clonada, borrado forzado de ramas y una función documentada que no hace lo que dice.
- **Medio (9):** robustez del parseo de git, colisiones de artefactos, efectos secundarios inesperados y pruebas contaminadas por la config del usuario.
- **Bajo (12):** deuda de mantenimiento: código muerto, convenciones rotas, rendimiento y huecos de documentación.

---

## Hallazgos

### Riesgo alto

#### A1 — El `.wt.json` de un repo clonado puede elegir qué ejecutable lanza `wt`

- **Qué pasa:** `Get-WtConfig` mergea `<repoRoot>\.wt.json` por encima de la config global, sin restringir qué claves puede definir. Entre esas claves están `editor` y `warpPath`, que terminan como `-FilePath` de un `Start-Process`.
- **Impacto y riesgo:** Clonar un repo ajeno que traiga un `.wt.json` y correr `wt open` adentro ejecuta el binario que ese archivo elija — por ejemplo, uno versionado en el propio repo. Es ejecución de código arbitrario disparada por una acción que el usuario percibe como inofensiva. El README promociona `.wt.json` como «config por proyecto, commiteable», así que el vector es parte del diseño, no un accidente.
- **Recomendación:** Lista blanca de claves admitidas en el `.wt.json` del repo — `worktreeRootTemplate`, `defaultBase`, `branchPrefix`, `fetchBeforeCreate` — y descartar con un warning explícito toda clave que resuelva a un ejecutable o a una ruta fuera del repo. Si más adelante se agrega `agentCommand`, entra en la misma prohibición.
- **Referencias:** `src/Config.ps1:105-108` · `src/Launch.ps1:24` · `src/Commands.ps1:251` · `README.md:140`

#### A2 — `remove --delete-branch` borra con `-D`: descarta commits sin avisar

- **Qué pasa:** `Remove-WtWorktree` ejecuta siempre `git branch -D`, el borrado forzado, sin verificar si la rama está mergeada y sin pedir confirmación desde el CLI.
- **Impacto y riesgo:** Pérdida de trabajo silenciosa, y en el escenario exacto que la herramienta existe para habilitar: un agente commitea en su worktree, no hay push todavía, se limpia el worktree y los commits desaparecen. `git` protege este caso por defecto con `-d`; `wt` desactiva esa protección.
- **Recomendación:** Intentar `-d` primero y, si git rechaza por rama no mergeada, abortar con un mensaje que nombre la rama y exija un flag nuevo y explícito (`--force-branch`). Agregar `SupportsShouldProcess` a `Remove-WtWorktree` para que `-WhatIf`/`-Confirm` funcionen como en cualquier `Remove-*` de PowerShell.
- **Referencias:** `src/Commands.ps1:287-290` · `README.md:37`

#### A3 — `wt create` nunca abre el agente, pero el README y la ayuda dicen que sí

- **Qué pasa:** `New-WtWorktree` termina llamando a `Open-WtWorktree -Name $Name` sin flags. `Get-WtOpenPlan` sin flags devuelve solo editor. El README documenta `--no-open` como «crea sin abrir editor ni agente» y la ayuda como «crea sin abrir nada»; la línea de apertura del README promete «abriendo cada worktree en VS Code y Warp con un solo comando».
- **Impacto y riesgo:** El flujo principal de la herramienta no hace lo documentado. El usuario tiene que encadenar `wt create x --no-open` y después `wt open x --all`, o descubrir por prueba y error que el agente no arrancó. No hay ninguna prueba que cubra qué abre `create`, porque la suite E2E siempre lo invoca con `--no-open`.
- **Recomendación:** Decidir el contrato y hacerlo explícito: una clave `openOnCreate` (`all` | `editor` | `none`) y/o aceptar `--all`/`--agent`/`--code` en `create`, pasándolos al plan. Alinear README y `Show-WtHelp`, y cubrirlo con una prueba E2E que verifique el plan de apertura resultante (con `editor` y `terminal` desactivados, alcanza con verificar la traza).
- **Referencias:** `src/Commands.ps1:53` · `src/Commands.ps1:155-158` · `README.md:3-5` · `README.md:24` · `src/Cli.ps1:120`

### Riesgo medio

#### M1 — stderr se mezcla con stdout y puede corromper rutas y nombres de rama

- **Qué pasa:** `Invoke-WtProcess` captura con `2>&1` para todos los casos. `Find-WtMainRoot` toma `Select-Object -First 1` de esa salida como ruta del repo, y `Get-WtBranchAt` devuelve el `.Text` completo como nombre de rama.
- **Impacto y riesgo:** Cualquier warning que git escriba en stderr durante una operación exitosa — refs rotas, `fsmonitor`, hooks, avisos de configuración — se convierte en «la ruta» o en «la rama». El resultado no es un error visible: es una ruta inválida que se propaga silenciosamente al título del tab, a la comparación de nombres o a un `Test-Path` que da falso.
- **Recomendación:** Separar los streams en `Invoke-WtProcess`: devolver `StdOut` y `StdErr` por separado, usar solo `StdOut` donde se parsea un valor, y reservar la mezcla para construir el mensaje de error. Agregar una prueba unitaria que alimente al parser con salida ruidosa.
- **Referencias:** `src/Common.ps1:70-73` · `src/Repo.ps1:61` · `src/Repo.ps1:84-88`

#### M2 — Los tab configs de Warp colisionan entre repos y nadie los borra

- **Qué pasa:** El archivo se llama `wt-<kind>-<nombre>.toml`, sin ninguna referencia al repo, y `ConvertTo-WtSafeFileName` además colapsa caracteres y trunca a 60. Dos repos con un worktree `feature-a` escriben el mismo archivo. Además `Start-Process` del URI es asíncrono: abrir dos tabs seguidos puede hacer que Warp lea un archivo ya sobrescrito.
- **Impacto y riesgo:** Un tab que abre el worktree equivocado — sutil y difícil de diagnosticar cuando hay varios agentes corriendo, que es el modo de uso previsto. En paralelo, `%APPDATA%\warp\Warp\data\tab_configs` acumula un TOML por cada worktree que existió alguna vez: `wt remove` no limpia nada.
- **Recomendación:** Incluir el repo (o un hash corto de la ruta del worktree) en el nombre del archivo; borrar el tab config correspondiente en `Remove-WtWorktree`; agregar `wt clean` para los huérfanos acumulados.
- **Referencias:** `src/Launch.ps1:164` · `src/Common.ps1:267-277` · `src/Launch.ps1:206-213`

#### M3 — `wt path` cambia el directorio de tu terminal

- **Qué pasa:** `Resolve-WtRepoContext` ejecuta `Set-WtLocation` como parte de resolver el nombre, y `Invoke-WtPathCommand` pasa por ahí. Fuera de un repo, `wt path logging` te mueve al repo dueño además de imprimir la ruta.
- **Impacto y riesgo:** Un comando de solo lectura muta el estado de la sesión. El uso documentado es `cd (wt path logging)`, donde el `cd` implícito es a la vez redundante e inesperado. También contradice la separación decisión/efecto que el README declara como principio de arquitectura, y un verbo `Resolve-` con efectos rompe la convención de PowerShell.
- **Recomendación:** Que la resolución sea pura y devuelva el destino; que el `Set-Location` lo apliquen solo `open` y `cd`, que es donde el usuario lo pidió.
- **Referencias:** `src/Workspace.ps1:96-107` · `src/Commands.ps1:111` · `src/Commands.ps1:131-135` · `README.md:166-168`

#### M4 — Un nombre de worktree ambiguo entre repos se resuelve en silencio por orden alfabético

- **Qué pasa:** `Find-WtRepoOwningWorktree` recorre los repos ordenados por nombre y devuelve el primero que tenga una coincidencia. En cambio `Resolve-WtRepoDir`, para el mismo tipo de ambigüedad entre repos, falla y lista las coincidencias.
- **Impacto y riesgo:** Dos criterios distintos para el mismo problema. `wt open feature-a` desde fuera de un repo puede abrir el worktree de otro proyecto sin ningún aviso; el usuario ve un mensaje de éxito con el repo equivocado.
- **Recomendación:** Recolectar todas las coincidencias y aplicar el mismo criterio que en repos: una sola sigue, varias fallan listándolas con su repo de origen.
- **Referencias:** `src/Workspace.ps1:61-75` · `src/Workspace.ps1:50-53`

#### M5 — `wt config set` valida la clave pero nunca el valor

- **Qué pasa:** `Set-WtConfigValue` comprueba que la clave exista y convierte `true`/`false`, nada más. `terminal foo`, `warpAgentTarget xyz`, `warpAgentColor rosa` o un `worktreeRootTemplate` sin `{name}` se guardan sin protesta.
- **Impacto y riesgo:** El error aparece mucho después y sin relación con la causa: `Open-WtTerminal` simplemente devuelve `$false` y no abre nada. El caso de la plantilla es real y está pasando: la config global de esta máquina tiene `worktreeRootTemplate = C:\w\{name}`, sin `{repo}` — dos repos distintos con un worktree del mismo nombre apuntan a la misma carpeta, y el segundo `create` falla con «Ya existe un directorio» sin explicar por qué.
- **Recomendación:** Un validador por clave (enumeración, ruta, plantilla con tokens obligatorios) reutilizado tanto por `set` como por la lectura de los archivos de config, para que un archivo editado a mano también avise.
- **Referencias:** `src/Config.ps1:124-157` · `src/Repo.ps1:204-210` · `src/Commands.ps1:212-233`

#### M6 — Las pruebas E2E heredan la config global del usuario

- **Qué pasa:** `WT_CONFIG` no reemplaza la cadena de precedencia: se suma encima. `Get-WtConfig` siempre incluye `%USERPROFILE%\.wt\config.json` entre los candidatos, y el config de pruebas define solo 6 de las 11 claves.
- **Impacto y riesgo:** Las suites verdes no prueban lo mismo en dos máquinas. `branchPrefix`, `warpPath`, `warpAgentTarget` y los colores salen del entorno real. Con un `branchPrefix = agent/` en la config global, varias aserciones sobre `refs/heads/<name>` fallarían sin que nadie haya tocado el código.
- **Recomendación:** Un modo de aislamiento explícito — `WT_CONFIG_ONLY=1`, o que `WT_CONFIG` sustituya la cadena en vez de sumarse — activado al arranque de la suite. Es también el prerrequisito para poder correr las pruebas en CI de forma reproducible.
- **Referencias:** `src/Config.ps1:104-113` · `tests/Invoke-WtTests.ps1:82-91`

#### M7 — `install.ps1` declara `SupportsShouldProcess` y no lo usa; no hay desinstalador

- **Qué pasa:** El atributo está en la línea 10, pero no hay ninguna llamada a `$PSCmdlet.ShouldProcess` en todo el repositorio. `-WhatIf` se acepta y el perfil se modifica igual.
- **Impacto y riesgo:** El instalador escribe en `$PROFILE.CurrentUserAllHosts`, que es un archivo compartido con todo lo demás que el usuario tenga configurado. Una promesa de simulación que no se cumple es peor que no ofrecerla. Tampoco hay forma limpia de revertir la instalación: el bloque delimitado está pensado para actualizarse, no para quitarse.
- **Recomendación:** Envolver las tres escrituras (perfil, directorio de config, config de ejemplo) en `ShouldProcess`, o quitar el atributo. Agregar `uninstall.ps1` que elimine el bloque entre marcadores y deje el resto del perfil intacto.
- **Referencias:** `install.ps1:10` · `install.ps1:47-62`

#### M8 — El agente está cableado a `copilot` y a sintaxis PowerShell dentro de Warp

- **Qué pasa:** `Get-WtAgentCommands` emite tres comandos fijos, uno de los cuales es un `if ((node --version 2>$null) -notmatch ...)` escrito en PowerShell, que se inyecta como `commands` del tab de Warp. El binario del agente es la cadena literal `copilot`.
- **Impacto y riesgo:** Si el shell por defecto de Warp es bash, WSL o cmd, ese comando es un error de sintaxis en cada tab que se abre. Y el proyecto entero queda atado a un único agente, justo en el eje donde hoy hay más movimiento: la propia arquitectura del README habla de «agentes», en plural y sin marca.
- **Recomendación:** Claves `agentCommand` y `agentShell` en la config (con la restricción de A1 vigente para `.wt.json`). Mover el chequeo de versión de Node a `wt doctor`, que es donde corresponde, en vez de inyectarlo en cada tab.
- **Referencias:** `src/Launch.ps1:56-70` · `src/Commands.ps1:247-250`

#### M9 — Sin manifiesto, sin versión y sin integración continua

- **Qué pasa:** No existen `wt.psd1`, `.gitignore`, `LICENSE`, `CHANGELOG.md` ni workflow de CI. No hay comando `wt version`. El repositorio tiene dos commits y ningún remoto configurado.
- **Impacto y riesgo:** 210 aserciones que solo corren cuando alguien se acuerda de correrlas a mano. Sin versión no hay forma de que un usuario reporte contra qué build le falló algo, ni de publicar el módulo. Sin manifiesto, `Import-Module` carga por convención de archivo en vez de por contrato declarado.
- **Recomendación:** Manifiesto `wt.psd1` con versión y funciones exportadas (hoy la lista vive en `Export-ModuleMember`), comando `wt version`, y un workflow en `windows-latest` que corra ambas suites bajo PowerShell 5.1 y 7 más PSScriptAnalyzer. El analizador ya tiene qué marcar: `Remove-WtWorktree` sin `ShouldProcess`.
- **Referencias:** `wt.psm1:18-40`

### Riesgo bajo · deuda de mantenimiento

#### B1 — Superficie muerta y no documentada

- **Qué pasa:** `--no-code` y `--no-terminal` siguen aceptados por el parser sin figurar en la ayuda ni el README; `-NoTerminal` es un parámetro que ninguna línea lee; `Invoke-WtConsoleCreate` y la acción de prune del menú reciben `-RepoRoot` y no lo usan.
- **Impacto y riesgo:** Compatibilidad con «invocaciones antiguas» que, en un proyecto de dos commits sin usuarios externos, no tiene a quién proteger. Cada flag fantasma es una ruta más que mantener y testear.
- **Recomendación:** Eliminar los flags legacy y los parámetros sin uso; si alguno debe quedar, documentarlo como alias explícito.
- **Referencias:** `src/Cli.ps1:12` · `src/Commands.ps1:181` · `src/Console.ps1:97`

#### B2 — La regla «un solo lugar imprime» está rota para los errores

- **Qué pasa:** `Common.ps1` declara ser el único punto que escribe a consola y expone `Write-WtInfo/Success/Detail/Notice/Warn`, pero no hay `Write-WtError`. La consola resuelve con dos `Write-Host ... -ForegroundColor Red` duplicados.
- **Impacto y riesgo:** La convención se erosiona en el punto donde más importa la consistencia: cómo se ve un error.
- **Recomendación:** Agregar `Write-WtError` a la capa de presentación y usarlo en los dos `catch`.
- **Referencias:** `src/Common.ps1:6-8` · `src/Console.ps1:189` · `src/Console.ps1:282`

#### B3 — El TOML de Warp y la config se escriben con BOM

- **Qué pasa:** `Set-Content -Encoding UTF8` en PowerShell 5.1 antepone `EF BB BF` — verificado en esta máquina. Afecta a los tab configs y al `config.json`.
- **Impacto y riesgo:** Muchos parsers TOML (incluidos los del ecosistema Rust, que es donde vive Warp) rechazan el BOM. Las pruebas no lo detectarían: leen el archivo con `Get-Content -Raw` y `.Contains()`, que lo ignoran. Latente y de diagnóstico difícil si aparece.
- **Recomendación:** Escribir con `[IO.File]::WriteAllText($path, $content, (New-Object Text.UTF8Encoding $false))` y verificar contra Warp una vez.
- **Referencias:** `src/Launch.ps1:165` · `src/Config.ps1:165`

#### B4 — El parser del porcelain ignora el atributo `locked`

- **Qué pasa:** `ConvertFrom-WtWorktreePorcelain` maneja `bare`, `detached` y `prunable`, pero no `locked`, que git emite en el mismo formato.
- **Impacto y riesgo:** Un worktree bloqueado no se marca en `wt list` y `wt remove` falla con el error crudo de git, sin la traducción cuidada que sí tienen los obsoletos. No existen `wt lock`/`wt unlock`.
- **Recomendación:** Parsear `locked` junto a su motivo, marcarlo en `list` y en la consola, y exponer `lock`/`unlock` — útil para worktrees en discos externos o pausados a mitad de una tarea.
- **Referencias:** `src/Repo.ps1:137-146`

#### B5 — Costo de arranque y llamadas a git redundantes

- **Qué pasa:** `wt.ps1` hace `Import-Module -Force` en cada invocación, reparseando los ocho archivos de `src/`. `Find-WtRepoOwningWorktree` lanza un `git worktree list` por cada repo de `reposRoot`. Cada comando dispara al menos dos `git rev-parse` (uno para la config, otro para el comando).
- **Impacto y riesgo:** Latencia perceptible en un comando que se usa decenas de veces por día, y que empeora linealmente con la cantidad de repos en la raíz.
- **Recomendación:** Que el bloque del perfil importe el módulo una vez y defina `wt` como wrapper de `Invoke-Wt`, en vez de invocar el script. Cachear la lista de worktrees por invocación.
- **Referencias:** `wt.ps1:11-12` · `install.ps1:24-31` · `src/Workspace.ps1:67`

#### B6 — No hay contrato de código de salida

- **Qué pasa:** Los errores viajan como excepciones. Invocado como función del perfil — el modo instalado y recomendado — la excepción se ve en rojo pero no setea `$LASTEXITCODE`. Solo `doctor` tiene un contrato explícito («sale 0 siempre»).
- **Impacto y riesgo:** Scriptear `wt` obliga a `try/catch`; un `if ($LASTEXITCODE -ne 0)` alrededor de `wt create` no detecta nada. Las pruebas E2E no lo notan porque invocan por `powershell.exe -Command`, donde el error terminante sí produce exit 1.
- **Recomendación:** Definir y documentar los códigos (0 OK, 1 error de uso, 2 error de git) y aplicarlos en ambos modos de invocación.
- **Referencias:** `wt.ps1:11-12` · `src/Cli.ps1:169-233` · `README.md:118`

#### B7 — `wt prune` pide verbose y tira la salida

- **Qué pasa:** `Invoke-WtPrune` corre `git worktree prune -v` y manda el resultado a `Out-Null`, dejando solo un «OK» genérico.
- **Impacto y riesgo:** El usuario no sabe qué se depuró ni si se depuró algo. El `-v` declara una intención que el código anula.
- **Recomendación:** Mostrar las entradas depuradas con `Write-WtDetail`, o quitar el `-v`.
- **Referencias:** `src/Commands.ps1:293-297`

#### B8 — El diagrama de dependencias del README no refleja el código

- **Qué pasa:** El README dibuja `Repo/Config` como un mismo nivel, pero `Get-WtConfig` llama a `Find-WtMainRoot`: `Config` depende de `Repo`. Funciona solo porque PowerShell resuelve los nombres de función en tiempo de ejecución, no de carga — el propio `wt.psm1` carga `Config` antes que `Repo`.
- **Impacto y riesgo:** El documento de arquitectura es la principal defensa contra que las capas se mezclen; si describe algo que no es cierto, deja de servir para decidir dónde poner código nuevo.
- **Recomendación:** Corregir el diagrama, o extraer la ubicación del `.wt.json` del repo a un helper de `Common` para que la dependencia desaparezca de verdad.
- **Referencias:** `README.md:163-165` · `src/Config.ps1:106` · `wt.psm1:14`

#### B9 — `create` emite la ruta al pipeline además de imprimirla

- **Qué pasa:** `New-WtWorktree` termina con `return $path`, y ni el dispatcher del CLI ni la consola silencian ese valor. La salida de `wt create` incluye entonces la ruta cruda, sin documentar.
- **Impacto y riesgo:** Contamina stdout, que es el canal que `path` y `--json` reservan para datos consumibles. Menor, pero rompe la disciplina de separar presentación de retorno.
- **Recomendación:** Decidir si `create` tiene salida consumible: si sí, documentarla; si no, `| Out-Null` en los dos llamadores.
- **Referencias:** `src/Commands.ps1:54` · `src/Cli.ps1:184` · `src/Console.ps1:110`

#### B10 — Sensibilidad a mayúsculas inconsistente al resolver nombres

- **Qué pasa:** `Test-WtWorktreeMatchesName` compara la carpeta con `-ieq` (insensible) y la rama con `-eq` (sensible), en la misma función y sin comentario.
- **Impacto y riesgo:** Defendible — git distingue mayúsculas en las ramas y Windows no en las rutas — pero el usuario no tiene cómo saberlo: `wt open Feature-A` resuelve por carpeta y no por rama, sin explicación.
- **Recomendación:** Dejar el comportamiento y documentarlo en el README, o unificar a insensible y avisar cuando haya más de una rama que difiera solo en mayúsculas.
- **Referencias:** `src/Repo.ps1:167-173`

#### B11 — `reposRoot` es un solo nivel y una sola raíz

- **Qué pasa:** `Get-WtRepoDirs` mira únicamente los subdirectorios inmediatos que contengan `.git`. Una organización habitual como `C:\Repos\<org>\<repo>` queda invisible, y no hay forma de declarar dos raíces.
- **Impacto y riesgo:** `repos`, `cd` y la resolución de `open`/`path` fuera de un repo dejan de funcionar para quien agrupe por organización o cliente.
- **Recomendación:** Aceptar una lista en `reposRoot` y una profundidad de búsqueda configurable (2 alcanza para el caso por organización).
- **Referencias:** `src/Workspace.ps1:14-23`

#### B12 — `wt cd` no puede llevarte a un worktree

- **Qué pasa:** `Invoke-WtCd` resuelve solo contra los repos de `reposRoot`. Para entrar a un worktree, el README propone `cd (wt path logging)`.
- **Impacto y riesgo:** El comando cuyo propósito es moverte no sirve para el objeto central de la herramienta. La consola tiene el mismo hueco: la opción `g` va a repos, no a worktrees.
- **Recomendación:** Extender `cd` con la misma resolución que ya usa `open` (repo, y si no, worktree de cualquier repo de la raíz), y agregar la opción equivalente en la consola.
- **Referencias:** `src/Workspace.ps1:128-144` · `README.md:34` · `src/Console.ps1:231`

---

## Funcionalidades sugeridas

Ordenadas por relación entre el valor que aportan al objetivo declarado — varios agentes trabajando en paralelo
sobre el mismo repositorio — y el esfuerzo de construirlas sobre la arquitectura que ya existe.

| Capacidad | Por qué aporta | Valor | Esfuerzo |
|---|---|---|---|
| Hooks de creación (`copyOnCreate` · `postCreate`) | Es la fricción real del flujo de worktrees: el checkout nuevo no tiene `.env`, `appsettings.Development.json`, certificados ni `node_modules`, así que el agente arranca en un árbol que no compila ni corre. Copiar una lista de archivos no versionados y ejecutar comandos de preparación convierte `create` en un worktree operativo. | Alto | Bajo |
| `wt status` | La vista que falta para operar N agentes: por worktree, cambios sin commit, staged, ahead/behind respecto de su base y último commit. Hoy hay que entrar a cada uno. Se apoya en el modelo de worktrees que ya existe. | Alto | Medio |
| Agente configurable (`agentCommand` · `agentShell`) | Desacopla del Copilot CLI y del shell PowerShell dentro de Warp (ver M8). El README habla de «agentes» en genérico; el código conoce uno solo. | Alto | Bajo |
| Autocompletado (`Register-ArgumentCompleter`) | Completar nombres de worktree, repos, comandos y claves de config al tabular. El parser declarativo de `Get-WtCommandSpecs` ya tiene toda la información necesaria; es casi gratis y cambia la experiencia diaria. | Alto | Bajo |
| `wt exec` y `wt each` | Correr un comando en un worktree, o en todos, sin cambiar de directorio: build, tests, `git fetch`. Complemento natural de `status` para supervisar trabajo paralelo. | Medio | Medio |
| `wt sync` | Traer la base configurada y rebasar cada worktree sobre ella, reportando cuáles quedaron en conflicto. Con varios agentes, la divergencia contra `develop` es el problema recurrente. | Medio | Medio |
| Agente en Windows Terminal | `terminal = 'wt'` hoy abre una terminal pero nunca un agente: `Open-WtAgent` exige Warp. Un `wt.exe new-tab -d <ruta>` con el comando del agente elimina la dependencia dura de un terminal propietario. | Medio | Bajo |
| `--dry-run` global | Imprimir los comandos git que se ejecutarían sin ejecutarlos. Encaja con la filosofía que ya tiene la consola — mostrar el comando equivalente antes de correrlo — y hace auditable lo destructivo. | Medio | Bajo |
| `wt clean` · `lock` · `unlock` | Cierra los huecos de M2 y B4: limpiar tab configs huérfanos y exponer el bloqueo de worktrees que git ya soporta y el parser hoy descarta. | Bajo | Bajo |
| `wt version` + manifiesto | Prerrequisito para reportar bugs contra una versión concreta y para distribuir el módulo fuera de esta máquina. | Bajo | Bajo |
| Raíces múltiples y anidadas | Resuelve B11 para quien agrupe repos por organización o cliente. | Bajo | Bajo |
| Consola: agente y terminal | El menú ofrece «editor» y «editor + agente», pero no «solo agente» ni «terminal», que sí existen en el CLI. Alinear el wrapper con la superficie que envuelve. | Bajo | Bajo |

---

## Acciones recomendadas

### Corto plazo (1–2 semanas · contener riesgo y cerrar la brecha con la documentación)

1. **Cerrar los dos riesgos destructivos.** Lista blanca de claves en el `.wt.json` del repo, y `-d` con opt-in explícito para el borrado forzado de ramas. *(A1 · A2)*
2. **Definir qué abre `create`** y alinear README, ayuda y una prueba E2E nueva que lo fije. *(A3)*
3. **Aislar las pruebas de la config global** antes de automatizarlas: de lo contrario CI heredará el mismo problema. *(M6)*
4. **Validar los valores de config** por clave, reutilizando el validador también en la lectura de archivos. *(M5)*
5. **Poner el proyecto en CI** en `windows-latest`, con ambas suites bajo PS 5.1 y 7, más PSScriptAnalyzer. Sumar manifiesto y versión. *(M9)*
6. **`ShouldProcess` real en el instalador** y un `uninstall.ps1` que quite el bloque del perfil. *(M7)*
7. **Barrer la superficie muerta** y agregar `Write-WtError` a la capa de presentación. *(B1 · B2)*

### Mediano plazo (1–2 meses · robustez y las capacidades que hoy faltan)

1. **Separar stdout de stderr** en `Invoke-WtProcess`, con una prueba unitaria que alimente al parser con salida ruidosa. *(M1)*
2. **Identidad y ciclo de vida de los tab configs**: nombre por repo, borrado en `remove`, `wt clean` para lo acumulado. *(M2)*
3. **Resolución pura, sin `cd` implícito**, y ambigüedad explícita al buscar worktrees entre repos. *(M3 · M4)*
4. **Hooks de creación** (`copyOnCreate`, `postCreate`): el cambio con mayor impacto en el uso diario, y ya seguro una vez que A1 esté cerrado. *(Nuevo)*
5. **Agente configurable** y soporte de agente en Windows Terminal; el chequeo de Node se muda a `doctor`. *(M8 · Nuevo)*
6. **`wt status` y autocompletado**: la vista de supervisión que falta y la mejora de UX más barata disponible. *(Nuevo)*
7. **Arranque sin `Import-Module -Force`** y caché de la lista de worktrees por invocación. *(B5)*

---

## Cómo se verificó

- Lectura completa de los 8 módulos de `src/`, `wt.psm1`, `wt.ps1`, `install.ps1`, `config.example.json` y el README (~104 KB).
- `tests\Invoke-WtUnitTests.ps1` ejecutado: **69 aserciones, 0 fallas, exit 0**.
- `tests\Invoke-WtTests.ps1` ejecutado contra un repo temporal real: **141 aserciones, 0 fallas, exit 0**.
- Ausencia de `wt.psd1`, `.gitignore`, `LICENSE`, `CHANGELOG.md`, `.github/` y configuración de PSScriptAnalyzer confirmada por inspección del árbol.
- Comportamiento del BOM en `Set-Content -Encoding UTF8` comprobado empíricamente en PowerShell 5.1.26100: `EF BB BF`.
- Config global del usuario leída para contrastar la precedencia real: `worktreeRootTemplate` sin `{repo}`, evidencia directa de M5.
- Historial git revisado: 2 commits, sin remoto configurado.

No se modificó ningún archivo del proyecto. Las suites se ejecutaron tal cual están, en subprocesos aislados
con `-NoProfile`; la E2E crea y elimina su propio repositorio bajo `%TEMP%`.
Los riesgos de A1 y B3 se describen a partir del código y de la precedencia verificada, sin ejercitar el vector.

---

*Auditoría de Worktree Manager · 24 hallazgos · 12 propuestas · sin cambios aplicados*
