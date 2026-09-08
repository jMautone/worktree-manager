# Plan de correcciones — Worktree Manager

**Guía de ejecución para un agente.** Cubre los 24 hallazgos de [AUDITORIA.md](AUDITORIA.md).
Este plan se ejecuta **primero**; [PLAN-MEJORAS.md](PLAN-MEJORAS.md) va después y da varias
de estas correcciones por hechas.

---

## Reglas de trabajo (aplican a todos los ítems)

1. **Un ítem = un commit.** Mensaje: `fix(<area>): <qué> (<ID>)`, ej. `fix(config): lista blanca de claves en .wt.json (A1)`. Los ítems agrupados en un mismo paso pueden ir juntos si comparten el archivo tocado; está indicado dónde.
2. **Respetar las capas.** `Common → Config/Repo → Workspace/Launch → Commands → Console/Cli`. Ninguna función nueva sube de nivel. La decisión va en una función pura; el efecto, en `Commands`.
3. **Nada imprime fuera de `Common`.** Usar `Write-WtInfo/Success/Detail/Notice/Warn` y el nuevo `Write-WtError` (B2).
4. **PowerShell 5.1.** Sin ternarios, sin `??`, sin `-AsHashtable`. `Set-StrictMode -Version Latest` está activo: toda propiedad tiene que existir antes de leerse.
5. **Verde antes de commitear.** Correr siempre las dos suites:

   ```
   powershell -NoProfile -File tests\Invoke-WtUnitTests.ps1
   powershell -NoProfile -File tests\Invoke-WtTests.ps1
   ```

   Baseline actual: 69 + 141 aserciones, 0 fallas. **Ninguna corrección puede bajar ese número**; casi todas lo suben.
6. **Documentación en el mismo commit.** Si el cambio toca la superficie del CLI, actualizar `Show-WtHelp` ([src/Cli.ps1:108-167](../../src/Cli.ps1#L108-L167)) y el `README.md` junto al código, no después.
7. **Si una decisión de contrato no está clara, elegir la opción marcada como recomendada** y anotarla en el `CHANGELOG.md`. No bloquear el plan esperando confirmación.

---

## Orden de ejecución

| Fase | Ítems | Por qué en este orden |
|---|---|---|
| **0 — Red de contención** | M6, M9 | Sin aislar las pruebas de la config global, toda aserción que agreguen las fases siguientes es poco confiable. CI y manifiesto vuelven verificable cada fase posterior. |
| **1 — Riesgo alto** | A1, A2, A3 | Ejecución de binarios arbitrarios, pérdida de commits y la promesa rota del README. |
| **2 — Robustez** | M1, M5, M2, M3, M4, M7, M8 | Corrupción silenciosa de datos, config sin validar, colisiones y acoplamientos duros. M5 se adelanta a M2/M8 porque su validador se reutiliza en ambos. |
| **3 — Deuda** | B2, B1, B3, B4, B5, B6, B7, B8, B9, B10, B11, B12 | Mantenimiento, convenciones y rendimiento. B2 primero porque el resto lo usa. |

---

# Fase 0 — Red de contención

## M6 · Aislar las pruebas de la config global del usuario

**Problema.** `Get-WtConfig` ([src/Config.ps1:104-113](../../src/Config.ps1#L104-L113)) siempre incluye `%USERPROFILE%\.wt\config.json` entre los candidatos; `WT_CONFIG` se suma encima en vez de reemplazar. El config de pruebas ([tests/Invoke-WtTests.ps1:82-91](../../tests/Invoke-WtTests.ps1#L82-L91)) define 6 de 11 claves: `branchPrefix`, `warpPath`, `warpAgentTarget` y los colores salen de la máquina real.

**Contrato a implementar.** Variable de entorno `WT_CONFIG_ONLY`:

- `WT_CONFIG_ONLY=1` → la cadena de precedencia es **defaults < `WT_CONFIG`**. Se ignoran el global del usuario y el `.wt.json` del repo.
- Sin ella, el comportamiento actual no cambia.

**Cambios.**

1. `src/Config.ps1`, en `Get-WtConfig`: construir `$candidates` según el modo.

   ```powershell
   $config = Get-WtDefaultConfig
   if ($env:WT_CONFIG_ONLY -eq '1') {
       $candidates = @()
       if ($env:WT_CONFIG) { $candidates += $env:WT_CONFIG }
   } else {
       # ... cadena actual
   }
   ```

2. `tests/Invoke-WtTests.ps1`: setear `$env:WT_CONFIG_ONLY = '1'` junto a `$env:WT_CONFIG`, y limpiarlo en el teardown.
3. Completar el config de pruebas con **las 11 claves**, con valores explícitos y neutros (`branchPrefix = ''`, `warpPath = 'C:\no-existe\warp.exe'`, `warpAgentTarget = 'auto'`, colores `''`).
4. Documentar `WT_CONFIG` y `WT_CONFIG_ONLY` en el README.

**Pruebas.** Unitaria nueva: con `WT_CONFIG_ONLY=1` y un `WT_CONFIG` que define solo `branchPrefix`, `Get-WtConfig` devuelve el resto **igual a los defaults**.

**Aceptación.** Setear `branchPrefix = 'agent/'` en la config global real y correr la suite E2E: sigue en 0 fallas.

---

## M9 · Manifiesto, versión y CI

**Problema.** No hay `wt.psd1`, `.gitignore`, `LICENSE`, `CHANGELOG.md`, `wt version` ni workflow de CI. 210 aserciones que solo corren cuando alguien se acuerda.

**Cambios.**

1. **`wt.psd1`** — `RootModule = 'wt.psm1'`, `ModuleVersion = '0.1.0'`, `PowerShellVersion = '5.1'`, `FunctionsToExport` con la lista que hoy vive en `Export-ModuleMember` ([wt.psm1:18-40](wt.psm1#L18-L40)). Dejar `Export-ModuleMember` como está: el manifiesto es el contrato, el módulo la implementación.
2. **`wt version`** — comando nuevo en `Get-WtCommandSpecs` ([src/Cli.ps1:9-23](../../src/Cli.ps1#L9-L23)) sin flags, e `Invoke-WtVersion` en `src/Commands.ps1` que lee la versión del manifiesto (`Import-PowerShellDataFile`) e imprime `wt <version> · PowerShell <$PSVersionTable.PSVersion>`. Sumarlo al help.
3. **`.gitignore`** — artefactos temporales, `TestResults/`, `*.user`.
4. **`LICENSE`** y **`CHANGELOG.md`** (Keep a Changelog; la primera entrada es este plan).
5. **`.github/workflows/ci.yml`** — `runs-on: windows-latest`, matriz `[powershell, pwsh]`, tres pasos: suite unitaria, suite E2E, `Invoke-ScriptAnalyzer -Path . -Recurse -Settings PSScriptAnalyzerSettings.psd1`. El job falla si el exit code de cualquiera no es 0.
6. **`PSScriptAnalyzerSettings.psd1`** — arrancar con las reglas por defecto. Si `PSUseShouldProcessForStateChangingFunctions` genera ruido en funciones que no cambian estado, excluir por función, nunca globalmente. Las que sí lo necesitan (`Remove-WtWorktree`) se arreglan en A2.

**Aceptación.** El workflow corre en verde bajo PS 5.1 y 7. `wt version` imprime la versión del manifiesto.

> Este ítem cubre la propuesta *«`wt version` + manifiesto»* del plan de mejoras: no se repite allá.

---

# Fase 1 — Riesgo alto

## A1 · Lista blanca de claves en el `.wt.json` del repo

**Problema.** `Get-WtConfig` mergea `<repoRoot>\.wt.json` sin restringir claves ([src/Config.ps1:105-108](../../src/Config.ps1#L105-L108)). Entre ellas, `editor` y `warpPath` terminan como `-FilePath` de `Start-Process` ([src/Launch.ps1:24](../../src/Launch.ps1#L24), [src/Commands.ps1:251](../../src/Commands.ps1#L251)). Clonar un repo ajeno y correr `wt open` ejecuta el binario que ese archivo elija.

**Contrato a implementar.**

- Claves permitidas en el `.wt.json` **del repo**: `worktreeRootTemplate`, `defaultBase`, `branchPrefix`, `fetchBeforeCreate`.
- Toda otra clave se descarta con `Write-WtWarn` que nombre el archivo y la clave.
- La lista es una función pura y **la única fuente de verdad**: cualquier clave nueva que resuelva a un ejecutable, a un shell o a una ruta fuera del repo queda prohibida por defecto. Aplica a `agentCommand`/`agentShell` (M8) y a los hooks del plan de mejoras.

**Cambios.**

1. `src/Config.ps1`:

   ```powershell
   function Get-WtRepoConfigAllowedKeys {
       return @('worktreeRootTemplate', 'defaultBase', 'branchPrefix', 'fetchBeforeCreate')
   }

   function Select-WtRepoConfigKeys {
       # Pura: devuelve @{ Config; Rejected } a partir del hashtable leído del .wt.json
   }
   ```

2. En `Get-WtConfig`, aplicar `Select-WtRepoConfigKeys` **solo** al candidato `.wt.json` del repo, con un warning por clave rechazada: `"'.wt.json' del repo define 'editor', que no se admite por seguridad (se ignora). Claves admitidas: ..."`. El global del usuario y `WT_CONFIG` no se filtran.
3. Exportar `Get-WtRepoConfigAllowedKeys` en `wt.psm1` y en `FunctionsToExport`.
4. README: reemplazar la promesa actual de `.wt.json` ([README.md:140](../../README.md#L140)) por la lista explícita y el motivo.

**Pruebas.**

- Unitaria: `Select-WtRepoConfigKeys` con `@{ editor='mal.exe'; defaultBase='develop' }` → conserva `defaultBase`, rechaza `editor`.
- E2E: `.wt.json` en el repo temporal con `editor` y `warpPath` falsos; `wt config list` sigue mostrando los valores de `WT_CONFIG` y la salida contiene el warning.

**Aceptación.** Un `.wt.json` hostil no puede cambiar qué ejecutable se lanza.

---

## A2 · `remove --delete-branch` deja de descartar commits

**Problema.** `Remove-WtWorktree` ([src/Commands.ps1:287-290](../../src/Commands.ps1#L287-L290)) corre siempre `git branch -D`. Es exactamente el escenario que la herramienta habilita: un agente commitea sin push y el borrado se lleva el trabajo.

**Contrato a implementar.**

- `--delete-branch` intenta `git branch -d` (seguro).
- Si git rechaza por rama no mergeada: **abortar** con un mensaje que nombre la rama, aclare que el worktree sí se eliminó y dé la salida hacia adelante: `"La rama 'x' tiene commits que no están mergeados en ninguna otra rama; no se borró. El worktree sí se eliminó. Para borrarla igual: wt remove x --delete-branch --force-branch, o git branch -D x."`
- Flag nuevo `--force-branch` → usa `-D`. `--force` sigue siendo solo para `git worktree remove --force` (árbol sucio); **no** implica `--force-branch`.
- `Remove-WtWorktree` declara `[CmdletBinding(SupportsShouldProcess)]` y llama a `$PSCmdlet.ShouldProcess` dos veces por separado: una para el worktree, otra para la rama.

**Cambios.**

1. `src/Commands.ps1`: parámetro `[switch]$ForceBranch`; borrado con `-AllowFailure` y mensaje propio en la rama de fallo; `SupportsShouldProcess`.
2. [src/Cli.ps1:14](../../src/Cli.ps1#L14): agregar `force-branch` a los flags de `remove` y pasarlo en el dispatcher ([src/Cli.ps1:204-210](../../src/Cli.ps1#L204-L210)).
3. `src/Console.ps1`: la acción de eliminar refleja el mismo contrato; si el borrado de rama falla, muestra el mensaje sin romper el menú.
4. Help y README: documentar los tres flags y la diferencia entre `--force` y `--force-branch`.

**Pruebas E2E.**

- Rama mergeada + `--delete-branch` → la rama desaparece, exit 0.
- Rama con un commit propio + `--delete-branch` → exit ≠ 0, la rama **sigue existiendo**, el mensaje nombra `--force-branch`.
- Ese mismo caso + `--force-branch` → la rama desaparece, exit 0.
- El worktree se elimina en los tres casos.

**Aceptación.** No hay forma de perder commits sin haber escrito `--force-branch`.

---

## A3 · Definir y cumplir qué abre `wt create`

**Problema.** `New-WtWorktree` termina con `Open-WtWorktree -Name $Name` sin flags ([src/Commands.ps1:53](../../src/Commands.ps1#L53)), y `Get-WtOpenPlan` sin flags devuelve solo editor ([src/Commands.ps1:155-158](../../src/Commands.ps1#L155-L158)). El README promete «VS Code y Warp con un solo comando» y el help dice que `--no-open` «crea sin abrir nada». Ninguna prueba cubre el caso: la suite E2E siempre pasa `--no-open`.

**Contrato a implementar (recomendado).**

- Clave de config `openOnCreate`, valores `all` | `editor` | `none`, **default `all`**: es lo que el README promete y el propósito declarado de la herramienta.
- `create` acepta además `--all`, `--code`, `--agent`, `--terminal`, `--no-open`. Los flags explícitos ganan sobre la config; `--no-open` equivale a `none` y gana sobre todo.
- La traducción de `openOnCreate` + flags a un plan de apertura es una **función pura** nueva, `Get-WtCreateOpenPlan`, testeada sin efectos.

**Cambios.**

1. `src/Config.ps1`: sumar `openOnCreate = 'all'` a `Get-WtDefaultConfig`. **No** entra en la lista blanca de A1: es una decisión de la máquina, no del repo.
2. `src/Commands.ps1`: `New-WtWorktree` recibe los switches de apertura y llama a `Open-WtWorktree` con el plan resuelto.
3. [src/Cli.ps1:10](../../src/Cli.ps1#L10): flags de `create` = `no-open`, `all`, `code`, `agent`, `terminal`.
4. Alinear `Show-WtHelp` ([src/Cli.ps1:113](../../src/Cli.ps1#L113), [:120](../../src/Cli.ps1#L120)) y el README ([README.md:3-5](../../README.md#L3-L5), [:24](../../README.md#L24)).

**Pruebas.**

- Unitarias de `Get-WtCreateOpenPlan`: `none` sin flags, `all` sin flags, `editor` + `--agent`, cualquier config + `--no-open`.
- E2E con `editor=''` y `terminal='none'`: `wt create x` (sin `--no-open`) sale 0 y su salida contiene la traza de que se intentó abrir; `wt create y --no-open` no la contiene.

**Aceptación.** README, help y código dicen lo mismo, y hay una prueba que lo fija.

---

# Fase 2 — Robustez

## M1 · Separar stdout de stderr en `Invoke-WtProcess`

**Problema.** `Invoke-WtProcess` captura con `2>&1` siempre ([src/Common.ps1:70-73](../../src/Common.ps1#L70-L73)). `Find-WtMainRoot` toma la primera línea como ruta del repo ([src/Repo.ps1:61](../../src/Repo.ps1#L61)) y `Get-WtBranchAt` devuelve el `.Text` completo como nombre de rama ([src/Repo.ps1:84-88](../../src/Repo.ps1#L84-L88)). Un warning de git en una operación exitosa se convierte en «la ruta» o en «la rama».

**Cambios.**

1. `src/Common.ps1`: separar los streams por tipo de registro, sin perder el orden dentro de cada uno:

   ```powershell
   $raw = & $FilePath @Arguments 2>&1
   $err = @($raw | Where-Object { $_ -is [System.Management.Automation.ErrorRecord] })
   $out = @($raw | Where-Object { $_ -isnot [System.Management.Automation.ErrorRecord] })
   ```

   Devolver `StdOut` (array), `StdErr` (array), `Text` (solo stdout, `.Trim()`), `ErrorText`, `Output` (la mezcla, **solo** por compatibilidad y para armar el mensaje de error), `ExitCode`, `Success`.
2. El `throw` de fallo usa `ErrorText` si existe y cae a `Text` si no.
3. `src/Repo.ps1`: `Find-WtMainRoot` toma `StdOut | Select-Object -First 1`; `Get-WtBranchAt` toma la primera línea de `StdOut` con `.Trim()`; `Get-WtWorktrees` ([:163-164](../../src/Repo.ps1#L163-L164)) parsea `StdOut`.
4. Revisar todos los consumidores de `.Output`/`.Text` (`Get-WtNodeVersion`, `Get-WtNodeMajor`, `Test-WtRemote`) y moverlos a `StdOut`.

**Pruebas.** Unitaria del parser del porcelain alimentado con salida que incluya un warning intercalado; y unitaria de `Invoke-WtProcess` sobre un comando que escriba en ambos streams y salga 0 (`cmd /c "echo ok & echo warn 1>&2"`), verificando que `Text` no contiene `warn`.

---

## M5 · Validar los valores de config, no solo las claves

**Problema.** `Set-WtConfigValue` ([src/Config.ps1:124-157](../../src/Config.ps1#L124-L157)) solo verifica que la clave exista. `terminal foo`, `warpAgentTarget xyz` o un `worktreeRootTemplate` sin `{name}` se guardan sin protesta y fallan mucho después, sin relación con la causa. Ya está pasando en esta máquina: `worktreeRootTemplate = C:\w\{name}` sin `{repo}` hace colisionar worktrees homónimos de repos distintos.

**Contrato a implementar.** Una tabla de validadores por clave, pura, reutilizada por `Set-WtConfigValue` **y** por la lectura de archivos.

| Clave | Regla |
|---|---|
| `worktreeRootTemplate` | no vacía; contiene `{name}`; **advertir** (no rechazar) si no contiene `{repo}` ni `{repoParent}` |
| `terminal` | ∈ `warp`, `wt`, `none` |
| `warpAgentTarget` | ∈ `auto`, `tab`, `window` |
| `warpAgentColor`, `warpTerminalColor` | vacío o color conocido de Warp |
| `openOnCreate` (A3) | ∈ `all`, `editor`, `none` |
| `fetchBeforeCreate` | booleano |
| `reposRoot`, `warpPath` | vacío, o ruta absoluta sintácticamente válida (**no** se exige que exista: `doctor` ya avisa) |
| `editor`, `defaultBase`, `branchPrefix` | libres |

**Cambios.**

1. `src/Config.ps1`: `Test-WtConfigValue -Key -Value` devuelve `''` (OK) o el motivo; `Assert-WtConfigValue` lanza. Mismo patrón que `Test-WtWorktreeName`/`Assert-WtWorktreeName`.
2. `Set-WtConfigValue`: validar **antes** de escribir; el mensaje incluye los valores admitidos.
3. `Read-WtConfigFile`: validar cada clave leída; las inválidas se descartan con `Write-WtWarn` que nombre archivo, clave y motivo. **Nunca lanza**: un archivo roto no puede dejar el CLI inutilizable.
4. `Get-WtDoctorRows`: fila nueva `config valida` que liste las claves con valor inválido y advierta si `worktreeRootTemplate` no tiene `{repo}`.

**Pruebas.** Unitarias por clave (válido/inválido y mensaje). E2E: `wt config set terminal foo` sale ≠ 0 y **no** modifica el archivo.

---

## M2 · Identidad y ciclo de vida de los tab configs de Warp

**Problema.** El archivo se llama `wt-<kind>-<nombre>.toml` ([src/Launch.ps1:164](../../src/Launch.ps1#L164)) sin referencia al repo, y `ConvertTo-WtSafeFileName` ([src/Common.ps1:267-277](../../src/Common.ps1#L267-L277)) colapsa caracteres y trunca a 60: dos repos con un worktree `feature-a` escriben el mismo archivo. Como `Start-Process` del URI es asíncrono ([src/Launch.ps1:206-213](../../src/Launch.ps1#L206-L213)), abrir dos tabs seguidos puede hacer que Warp lea un archivo ya sobrescrito. Y nadie los borra nunca.

**Cambios.**

1. `src/Common.ps1`: `Get-WtPathHash -Path` → 8 hex de un SHA1 sobre la ruta **normalizada y en minúsculas** (`ConvertTo-WtFullPath` primero). Pura y determinista.
2. `src/Launch.ps1`: `Write-WtAgentTabConfig` arma `wt-<kind>-<safeName>-<hash>.toml`. El `name` del TOML puede seguir siendo legible; el que tiene que ser único es el **nombre de archivo**.
3. `src/Commands.ps1`: `Remove-WtWorktree` borra los tab configs (`agent` y `term`) de ese worktree **antes** de eliminarlo — la ruta se necesita para el hash.
4. **`wt clean`** — comando nuevo: recorre `Get-WtTabConfigDir` y, para cada `wt-*.toml`, lee su `directory` y borra el archivo si esa ruta ya no existe. Sumar a specs, dispatcher, help y README.

**Pruebas.** Unitaria: dos rutas distintas con el mismo nombre de worktree producen nombres de archivo distintos; la misma ruta produce siempre el mismo. E2E con `WT_WARP_TAB_CONFIG` en un directorio temporal: dos worktrees homónimos en repos distintos → dos archivos; `wt remove` → el archivo desaparece; huérfano + `wt clean` → se borra.

> Este ítem cubre el `wt clean` de la propuesta *«`wt clean` · `lock` · `unlock`»*; `lock`/`unlock` quedan en B4.

---

## M3 · Resolución pura: `wt path` deja de cambiar el directorio

**Problema.** `Resolve-WtRepoContext` hace `Set-WtLocation` como parte de resolver el nombre ([src/Workspace.ps1:96-107](../../src/Workspace.ps1#L96-L107)), e `Invoke-WtPathCommand` pasa por ahí ([src/Commands.ps1:131-135](../../src/Commands.ps1#L131-L135)). Fuera de un repo, `wt path logging` te mueve además de imprimir. Un verbo `Resolve-` con efectos rompe la convención y contradice la separación decisión/efecto que el README declara.

**Cambios.**

1. `src/Workspace.ps1`: `Resolve-WtRepoContext` **no** llama a `Set-WtLocation`; devuelve `@{ RepoRoot; Source; ShouldRelocate }` y los mensajes «ahora en...» se mueven al llamador.
2. `src/Commands.ps1`: `Open-WtWorktree` aplica el `Set-WtLocation` cuando `ShouldRelocate`; `Invoke-WtPathCommand` **nunca** lo aplica.
3. Verificar que `Start-WtConsole` sigue re-evaluando el repo por vuelta (`Clear-WtConfigCache` + `Find-WtMainRoot -Silent`).
4. README ([README.md:166-168](../../README.md#L166-L168)): `cd (wt path logging)` sigue siendo la forma documentada, ahora sin efecto colateral.

**Pruebas E2E.** Desde el `tempRoot` (fuera de repo): `wt path feature-a` imprime la ruta y **el proceso no cambió de directorio** (verificable imprimiendo `(Get-Location).Path` en el mismo `-Command`). `wt open feature-a` sí lo cambia.

---

## M4 · Ambigüedad explícita al buscar un worktree entre repos

**Problema.** `Find-WtRepoOwningWorktree` ([src/Workspace.ps1:61-75](../../src/Workspace.ps1#L61-L75)) devuelve el primer repo por orden alfabético; `Resolve-WtRepoDir` ([:50-53](../../src/Workspace.ps1#L50-L53)), para la misma clase de ambigüedad, falla y lista las coincidencias. Dos criterios para el mismo problema, y el usuario ve un éxito con el repo equivocado.

**Cambios.**

1. Reescribir como `Find-WtReposOwningWorktree` → devuelve **todas** las coincidencias como `@{ Repo; Worktree }`.
2. En `Resolve-WtRepoContext`: 0 → el `throw` actual; 1 → sigue; >1 → falla listando repo y ruta de cada coincidencia, con el tono de `Resolve-WtRepoDir`: `"El worktree 'feature-a' existe en varios repos: MiRepo (C:\...), OtroRepo (C:\...). Entra al repo, o usa 'wt open <repo>' primero."`

**Pruebas E2E.** Dos repos reales bajo `reposRoot`, cada uno con un worktree `feature-a`; `wt path feature-a` desde fuera → exit ≠ 0 y el mensaje nombra ambos repos.

---

## M7 · `ShouldProcess` real en el instalador + `uninstall.ps1`

**Problema.** [install.ps1:10](install.ps1#L10) declara `SupportsShouldProcess` y no hay una sola llamada a `$PSCmdlet.ShouldProcess` en el repositorio: `-WhatIf` se acepta y el perfil se modifica igual ([install.ps1:47-62](install.ps1#L47-L62)). Tampoco hay forma limpia de revertir.

**Cambios.**

1. `install.ps1`: envolver las tres escrituras — bloque del perfil, directorio de config, copia de `config.example.json` — en `ShouldProcess`, con el target correcto (la ruta) y una acción legible en cada una.
2. **`uninstall.ps1`** nuevo, también con `SupportsShouldProcess`: elimina el bloque entre `# >>> worktree-manager >>>` y `# <<< worktree-manager <<<` con el mismo regex que usa el instalador, deja intacto el resto del perfil, y **no** toca `~\.wt\config.json` salvo que se pase `-RemoveConfig`. Informa qué hizo y qué dejó.
3. README: sección de desinstalación.

**Pruebas.** No van en las suites (tocan el perfil real). Verificación manual documentada en el README: `.\install.ps1 -WhatIf` no modifica el perfil (comparar hash antes/después); `.\uninstall.ps1` deja el resto del perfil byte a byte igual salvo el bloque.

---

## M8 · Desacoplar el agente de `copilot` y del shell de Warp

**Problema.** `Get-WtAgentCommands` ([src/Launch.ps1:56-70](../../src/Launch.ps1#L56-L70)) inyecta como `commands` del tab un `if ((node --version 2>$null) -notmatch ...)` en sintaxis PowerShell: si el shell por defecto de Warp es bash, WSL o cmd, es un error de sintaxis en cada tab. Y el binario del agente es la cadena literal `copilot` ([src/Commands.ps1:247-250](../../src/Commands.ps1#L247-L250)).

**Contrato a implementar.**

- `agentCommand` (default `'copilot'`) — el comando que corre el tab.
- `agentShell` (default `'powershell'`; valores `powershell` | `bash` | `none`) — determina **cómo se escriben los comandos auxiliares**; con `none` se emite únicamente `agentCommand`.
- Ninguna de las dos entra en la lista blanca de A1.
- El chequeo de versión de Node **sale del tab**: queda solo en `doctor`, que ya lo hace ([src/Commands.ps1:326-330](../../src/Commands.ps1#L326-L330)).

**Cambios.**

1. `src/Config.ps1`: dos claves nuevas con sus validadores (M5).
2. `src/Launch.ps1`: `Get-WtAgentCommands -Config` puro respecto de la config — `fnm use <major>` solo si `agentShell` no es `none` y `fnm` existe; después `agentCommand`. Sin el `if` inyectado.
3. `src/Commands.ps1`: `Open-WtAgent` valida `Test-WtCommand $config.agentCommand` y el warning nombra el comando configurado, no `copilot`.
4. `Get-WtDoctorRows`: la fila `copilot (agente)` pasa a chequear `agentCommand`.
5. Help, README y `config.example.json`.

**Pruebas.** Unitarias de `Get-WtAgentCommands` para las tres variantes de `agentShell` y un `agentCommand` custom: el array resultante nunca contiene sintaxis PowerShell cuando `agentShell` no es `powershell`.

---

# Fase 3 — Deuda de mantenimiento

## B2 · `Write-WtError` en la capa de presentación

[src/Common.ps1:6-8](../../src/Common.ps1#L6-L8) declara ser el único punto que escribe a consola pero no expone un helper de error; [src/Console.ps1:189](../../src/Console.ps1#L189) y [:282](../../src/Console.ps1#L282) resuelven con `Write-Host ... -ForegroundColor Red` duplicado. Agregar `Write-WtError` junto a los demás y usarlo en los dos `catch`. **Va primero de la fase**: el resto lo usa.

## B1 · Barrer la superficie muerta

Eliminar `--no-code` y `--no-terminal` de los flags de `open` ([src/Cli.ps1:12](../../src/Cli.ps1#L12)) y del dispatcher; eliminar el parámetro `-NoTerminal` de `Open-WtWorktree` ([src/Commands.ps1:181](../../src/Commands.ps1#L181)) y su `.NOTES`; eliminar `-RepoRoot` de `Invoke-WtConsoleCreate` ([src/Console.ps1:97](../../src/Console.ps1#L97)) y de la acción de prune del menú. Revisar `Get-WtOpenPlan`: si `-NoCode` queda sin llamadores tras A3, sacarlo también y ajustar sus pruebas unitarias. Verificar con `grep` que no queda ninguna referencia.

## B3 · Escribir UTF-8 sin BOM

`Set-Content -Encoding UTF8` en PS 5.1 antepone `EF BB BF`, y muchos parsers TOML lo rechazan. Reemplazar en [src/Launch.ps1:165](../../src/Launch.ps1#L165) y [src/Config.ps1:165](../../src/Config.ps1#L165) por `[IO.File]::WriteAllText($path, $content, (New-Object Text.UTF8Encoding $false))`. Prueba unitaria: escribir un tab config en un directorio temporal y verificar que los tres primeros bytes **no** son `EF BB BF`. Verificar una vez a mano contra Warp real.

## B4 · Parsear `locked` y exponer `wt lock` / `wt unlock`

`ConvertFrom-WtWorktreePorcelain` ([src/Repo.ps1:137-146](../../src/Repo.ps1#L137-L146)) maneja `bare`, `detached` y `prunable` pero no `locked`. Sumar `IsLocked` y `LockReason` a `New-WtWorktreeInfo` y al parser; marcarlo en `Get-WtWorktreeRows`, en `--json` y en la lista de la consola; traducir el error crudo de git en `Remove-WtWorktree` cuando el worktree está bloqueado. Comandos nuevos `wt lock <nombre> [--reason <texto>]` y `wt unlock <nombre>` → `git worktree lock/unlock`. Pruebas: unitarias del parser con `locked` con y sin motivo; E2E del ciclo lock → remove falla con mensaje claro → unlock → remove funciona.

## B5 · Costo de arranque y llamadas a git redundantes

Tres cambios independientes:

1. `install.ps1`: el bloque del perfil importa el módulo **una vez** y define `function wt { Invoke-Wt @args }`, en vez de invocar `wt.ps1` en cada llamada ([wt.ps1:11-12](wt.ps1#L11-L12), [install.ps1:24-31](install.ps1#L24-L31)). `wt.ps1` se conserva para el modo `-File`.
2. Caché por invocación de `Get-WtWorktrees` por `RepoRoot`, invalidada en `Invoke-Wt` junto a `Clear-WtConfigCache` y después de cada `create`/`remove`/`prune`. Acota el `git worktree list` por repo de [src/Workspace.ps1:67](../../src/Workspace.ps1#L67).
3. Reusar el `Find-WtMainRoot` que ya resolvió la config en vez de repetir `rev-parse`, pasando `-RepoRoot` donde el llamador ya lo tiene.

Medir antes y después con `Measure-Command { wt list }` y anotar el resultado en el `CHANGELOG.md`.

## B6 · Contrato de códigos de salida

Definir y documentar: **0** OK · **1** error de uso (comando o flag inválido, argumento faltante) · **2** error de git o del entorno. Introducir un tipo de error propio o una convención que `Invoke-Wt` mapee, setear `$LASTEXITCODE`/`exit` en **ambos** modos de invocación (función del perfil y `wt.ps1 -File`), y documentarlo en el README ([README.md:118](../../README.md#L118)) y el help. Pruebas E2E: `wt inventado` → 1; `wt open no-existe` → 2; `wt list` → 0.

## B7 · `wt prune` muestra lo que depuró

`Invoke-WtPrune` ([src/Commands.ps1:293-297](../../src/Commands.ps1#L293-L297)) corre con `-v` y manda todo a `Out-Null`. Capturar `StdOut` (ya separado por M1) y emitir cada línea con `Write-WtDetail`; si no hubo nada, decirlo explícitamente («no había metadatos obsoletos»).

## B8 · Corregir la dependencia Config → Repo

`Get-WtConfig` llama a `Find-WtMainRoot` ([src/Config.ps1:106](../../src/Config.ps1#L106)), así que `Config` depende de `Repo`, contra lo que dibuja el README ([README.md:163-165](../../README.md#L163-L165)) y contra el orden de carga de [wt.psm1:14](wt.psm1#L14). **Opción recomendada:** mover la ubicación del `.wt.json` a un helper de `Common` (`Find-WtRepoConfigFile`, que camina hacia arriba buscando `.git` sin invocar git) y que la dependencia desaparezca de verdad. Si se prefiere no tocar el comportamiento, corregir el diagrama del README y el comentario de encabezado de `Config.ps1`. En cualquier caso, los dos documentos tienen que terminar diciendo la verdad.

## B9 · `create` deja de contaminar stdout

`New-WtWorktree` termina con `return $path` ([src/Commands.ps1:54](../../src/Commands.ps1#L54)) y ni el dispatcher ([src/Cli.ps1:184](../../src/Cli.ps1#L184)) ni la consola ([src/Console.ps1:110](../../src/Console.ps1#L110)) lo silencian. **Decisión recomendada:** la función sigue devolviendo la ruta (es útil para consumidores del módulo) y **los dos llamadores la mandan a `Out-Null`**; `stdout` queda reservado a `path` y `--json`. Documentar el retorno en el `.OUTPUTS`. Prueba E2E: la salida de `wt create x --no-open` no contiene la ruta desnuda en una línea propia.

## B10 · Sensibilidad a mayúsculas al resolver nombres

`Test-WtWorktreeMatchesName` ([src/Repo.ps1:167-173](../../src/Repo.ps1#L167-L173)) compara la carpeta con `-ieq` y la rama con `-eq`, sin explicación. **Decisión recomendada:** conservar el comportamiento (git distingue mayúsculas en ramas; Windows no en rutas), documentarlo en un comentario de la función y en el README, y agregar una prueba unitaria que lo fije como intencional.

## B11 · `reposRoot` con varias raíces y profundidad configurable

`Get-WtRepoDirs` ([src/Workspace.ps1:14-23](../../src/Workspace.ps1#L14-L23)) solo mira subdirectorios inmediatos: `C:\Repos\<org>\<repo>` queda invisible y no hay forma de declarar dos raíces. Cambios: `reposRoot` acepta **string o array** (retrocompatible; normalizar a array en un helper puro); clave nueva `reposDepth` (default `1`, máximo razonable `3`) validada por M5; `Get-WtRepoDirs` recorre hasta esa profundidad y **corta la rama** al encontrar un `.git` (un repo no contiene repos). Con varias raíces, `wt repos` muestra una columna de raíz y la ambigüedad se resuelve con el criterio de M4. Actualizar `Get-WtReposRoot`, `Invoke-WtCd`, `Resolve-WtRepoContext`, `doctor`, help y README.

## B12 · `wt cd` puede llevarte a un worktree

`Invoke-WtCd` ([src/Workspace.ps1:128-144](../../src/Workspace.ps1#L128-L144)) resuelve solo contra repos de `reposRoot`; para entrar a un worktree el README propone `cd (wt path logging)` ([README.md:34](../../README.md#L34)). Extender `cd` con la misma cadena de resolución de `open`: primero repo, después worktree de cualquier repo de la raíz, con la ambigüedad de M4. En la consola, extender la opción `g` ([src/Console.ps1:231](../../src/Console.ps1#L231)) para ofrecer también worktrees. Prueba E2E: `wt cd feature-a` desde fuera del repo deja el proceso en el directorio del worktree.

---

## Cierre del plan

Antes de pasar a [PLAN-MEJORAS.md](PLAN-MEJORAS.md):

- [ ] Las dos suites en verde, con **más** aserciones que el baseline (69 + 141).
- [ ] CI en verde bajo PS 5.1 y 7; PSScriptAnalyzer sin hallazgos nuevos.
- [ ] `README.md` sin ninguna afirmación que el código no cumpla — releer las secciones de `create`, `.wt.json`, arquitectura y `cd`/`path`.
- [ ] `CHANGELOG.md` con una entrada por fase.
- [ ] `AUDITORIA.md` actualizada (o un `AUDITORIA-ESTADO.md` con el tablero) marcando cada hallazgo cerrado, para que la próxima auditoría tenga contra qué comparar.
