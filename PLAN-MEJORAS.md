# Plan de mejoras — Worktree Manager

**Guía de ejecución para un agente.** Cubre las 12 capacidades propuestas en [AUDITORIA.md](AUDITORIA.md).

> **Requisito de entrada:** este plan **se ejecuta después** de [PLAN-CORRECCIONES.md](PLAN-CORRECCIONES.md)
> y asume su checklist de cierre completo. Varios ítems dependen de correcciones concretas y no
> se pueden construir bien antes: los hooks necesitan la lista blanca de A1, el agente configurable
> es M8, `wt status` necesita el stdout limpio de M1, y todo lo nuevo necesita las pruebas aisladas de M6.

---

## Qué queda fuera de este plan (ya está en correcciones)

| Propuesta de la auditoría | Dónde se resuelve |
|---|---|
| `wt version` + manifiesto | **M9** (completa) |
| `wt clean` | **M2** (completa) |
| `wt lock` / `wt unlock` | **B4** (completa) |
| Raíces múltiples y anidadas | **B11** (completa) |
| Agente configurable (`agentCommand`/`agentShell`) | **M8** hace el desacople; acá solo lo consume el ítem 3 |

Quedan por construir, entonces: **hooks de creación, `wt status`, autocompletado, `wt exec`/`wt each`, agente en Windows Terminal, `--dry-run` global, `wt sync` y la alineación de la consola.**

---

## Reglas de trabajo (aplican a todos los ítems)

1. **Un ítem = una rama + un commit** (o unos pocos). Mensaje: `feat(<area>): <qué>`.
2. **Toda clave de config nueva pasa por dos puertas:**
   - un validador en la tabla de **M5**;
   - una decisión explícita en la lista blanca de **A1**. El default es **no** admitirla en el `.wt.json` del repo. Solo entra si no puede derivar en ejecución de código ni en escritura fuera del worktree — y eso hay que argumentarlo en el comentario de la función.
3. **Decisión pura, efecto en `Commands`.** Cada comando nuevo aporta al menos una función pura testeable sin git ni disco. Es el patrón que ya tienen `Get-WtOpenPlan`, `ConvertFrom-WtWorktreePorcelain` y `ConvertFrom-WtArgs`.
4. **Superficie del CLI:** todo comando nuevo se declara en `Get-WtCommandSpecs` ([src/Cli.ps1:9-23](src/Cli.ps1#L9-L23)) — es la única fuente de verdad del parser, de la validación de flags y (tras el ítem 3) del autocompletado.
5. **Cada ítem cierra con:** pruebas unitarias de su parte pura, al menos una E2E del camino feliz y una del error esperado, entrada en `Show-WtHelp`, sección en el README, entrada en `CHANGELOG.md` y bump de `ModuleVersion` en `wt.psd1`.
6. **Las dos suites en verde antes de commitear** (ver reglas de PLAN-CORRECCIONES.md).

---

## Orden de ejecución

Ordenado por **valor sobre el objetivo declarado** — varios agentes en paralelo sobre el mismo repo —
y por dependencias entre ítems.

| # | Capacidad | Valor | Esfuerzo | Depende de |
|---|---|---|---|---|
| 1 | Hooks de creación (`copyOnCreate` · `postCreate`) | Alto | Bajo | A1, A3, M5 |
| 2 | Autocompletado (`Register-ArgumentCompleter`) | Alto | Bajo | M9 (instalador/perfil), B5 |
| 3 | Agente en Windows Terminal | Medio | Bajo | M8 |
| 4 | `--dry-run` global | Medio | Bajo | B6 |
| 5 | `wt status` | Alto | Medio | M1 |
| 6 | `wt exec` y `wt each` | Medio | Medio | 4, 5 |
| 7 | `wt sync` | Medio | Medio | 5, 6 |
| 8 | Consola: agente, terminal y worktrees | Bajo | Bajo | 3, 5, B12 |

Los ítems 1–4 son independientes entre sí y se pueden paralelizar. 5 → 6 → 7 forman una cadena:
`status` define el modelo de estado que `each` recorre y que `sync` reporta.

---

# 1 · Hooks de creación: `copyOnCreate` y `postCreate`

**Estado: implementado.** `Resolve-WtCopyOnCreatePlan` / `Invoke-WtCreateHooks` en
`src/Commands.ps1`, claves `copyOnCreate`/`postCreate` en `src/Config.ps1`, flag
`--no-hooks` en `create`. Ver `CHANGELOG.md`.

**Por qué.** Es la fricción real del flujo de worktrees: el checkout nuevo no tiene `.env`, `appsettings.Development.json`, certificados ni `node_modules`, así que el agente arranca en un árbol que no compila ni corre. Es la mejora con mayor impacto diario y la más barata.

**Contrato.**

- `copyOnCreate` — array de patrones (glob simple, relativos a la raíz del repo principal). Cada uno se copia del repo principal al worktree nuevo, preservando la estructura de subdirectorios. Los que no matchean nada se informan con `Write-WtDetail`, no fallan.
- `postCreate` — array de comandos que se ejecutan **en el worktree recién creado**, en orden, después de la copia.
- Ambos corren **antes** de la apertura del editor/agente (A3): el agente tiene que encontrar el árbol ya preparado.
- Si un `postCreate` falla: se informa con `Write-WtError`, se **detiene la cadena** de hooks, pero el worktree **no** se destruye — ya existe y borrarlo sería peor. El mensaje dice qué comando falló y que el worktree quedó creado.
- Flag `--no-hooks` en `create` para saltearlos.

**Seguridad — no negociable.** `copyOnCreate` y sobre todo `postCreate` son ejecución de código. **Ninguna de las dos entra en la lista blanca de A1**: solo se pueden definir en la config global del usuario o en `WT_CONFIG`, nunca en el `.wt.json` de un repo clonado. Agregar la prueba que lo fije: un `.wt.json` con `postCreate` se rechaza con warning.

**Cambios.**

1. `src/Config.ps1`: dos claves nuevas (default `@()`), validadores en M5 (array de strings; `postCreate` no vacío por elemento). `Set-WtConfigValue` recibe strings desde el CLI: definir cómo se setea una lista (recomendado: `wt config set copyOnCreate ".env;certs/*.pfx"` separado por `;`, documentado, con la lista guardada como array JSON).
2. `src/Commands.ps1`, funciones nuevas:
   - `Resolve-WtCopyOnCreatePlan -Patterns -RepoRoot -WorktreePath` → **pura**: devuelve la lista de `@{ From; To }` a partir de un listado de archivos que recibe por parámetro. La expansión real del glob queda en el llamador, que sí toca disco.
   - `Invoke-WtCreateHooks -RepoRoot -WorktreePath -Config` → efectos: copia y ejecuta, con `Write-WtDetail` por paso.
3. `New-WtWorktree`: invocar los hooks entre el `git worktree add` y la apertura.
4. `src/Cli.ps1`: flag `no-hooks` en `create`.
5. `Get-WtDoctorRows`: fila informativa con cuántos patrones y comandos hay configurados.

**Pruebas.**

- Unitarias de `Resolve-WtCopyOnCreatePlan`: patrón sin matches, patrón con subdirectorio, patrón absoluto o con `..` → **rechazado** (no se puede copiar desde fuera del repo).
- E2E: repo con `.env` no versionado + `copyOnCreate = @('.env')` → `wt create x` deja el `.env` en el worktree. `postCreate = @('cmd /c echo hola > hook.txt')` → el archivo aparece en el worktree. Un `postCreate` que falla → exit ≠ 0, el worktree existe, el mensaje nombra el comando.
- E2E de seguridad: `.wt.json` con `postCreate` → se ignora con warning.

---

# 2 · Autocompletado con `Register-ArgumentCompleter`

**Por qué.** El parser declarativo de `Get-WtCommandSpecs` ya tiene toda la información necesaria; es casi gratis y cambia la experiencia diaria.

**Contrato.**

- Posición 1: nombres de comando y alias.
- Tokens que empiezan con `--`: los flags y valores válidos **de ese comando**, leídos de las specs.
- Posición 2 de `open`, `path`, `remove`, `cd`, `lock`, `unlock`, `status`, `exec`: nombres de worktree del repo actual; fuera de un repo, nombres de repo de `reposRoot`.
- Posición 2 de `config get|set`: claves de `Get-WtConfigKeys`. Posición 3 de `config set`: valores admitidos por el validador de M5 cuando la clave es una enumeración.
- **Presupuesto de tiempo: el completer nunca puede tardar más de ~150 ms.** Se apoya en la caché de B5 y, si el dato no está cacheado, devuelve lista vacía en vez de disparar git. Un completer lento arruina la terminal.

**Cambios.**

1. `src/Cli.ps1` (o `src/Completion.ps1` nuevo, mismo nivel): `Get-WtCompletion -CommandName -WordToComplete -CommandAst -CursorPosition` → **función pura** que recibe también el contexto (worktrees y repos disponibles) por parámetro y devuelve la lista de candidatos. Toda la lógica testeable vive acá.
2. `Register-WtCompletion` — registra el completer para `wt` y `wtm` usando la función anterior. Se invoca desde el bloque del perfil que instala B5, no al importar el módulo (importar no debe tener efectos, `wt.psm1:9`).
3. `install.ps1`: sumar la llamada al bloque delimitado.
4. Sumar `Get-WtCompletion` y `Register-WtCompletion` a `Export-ModuleMember` y a `FunctionsToExport`.

**Pruebas.** Unitarias de `Get-WtCompletion` con contexto sintético: `wt op` → `open`; `wt open --a` → `--agent`, `--all`; `wt config set ter` → `terminal`; `wt config set terminal ` → `warp`, `wt`, `none`; comando desconocido → lista vacía.

---

# 3 · Agente en Windows Terminal

**Por qué.** `terminal = 'wt'` hoy abre una terminal pero nunca un agente: `Open-WtAgent` exige Warp ([src/Commands.ps1:251-254](src/Commands.ps1#L251-L254)). Elimina la dependencia dura de un terminal propietario.

**Contrato.**

- Con `terminal = 'wt'`, `wt open x --agent` abre una pestaña de Windows Terminal en el worktree corriendo `agentCommand` (la clave que introdujo M8).
- El título de la pestaña usa el mismo `Get-WtTabTitle` que Warp — la identidad visual no depende del terminal.
- Con `terminal = 'none'`, `--agent` sigue avisando que no hay dónde abrirlo y devolviendo `$false`. Sin cambios.

**Cambios.**

1. `src/Launch.ps1`: `Open-WtAgentInWindowsTerminal -Path -Title -Config` y una función **pura** `Get-WtWindowsTerminalArgs -Path -Title -Config` que arma el array de argumentos (`new-tab`, `-d <ruta>`, `--title <titulo>`, el shell y el comando). Testear el array, no el `Start-Process`.
2. `src/Commands.ps1`: `Open-WtAgent` despacha por `$Config.terminal` en vez de exigir Warp. Warp sigue siendo el camino con tab configs; `wt` usa argumentos de línea de comandos.
3. `Get-WtDoctorRows`: cuando `terminal = 'wt'`, la fila del agente deja de sugerir Warp.
4. Help y README: la tabla de qué abre cada terminal.

**Pruebas.** Unitarias de `Get-WtWindowsTerminalArgs`: ruta con espacios correctamente entrecomillada (reusar `Format-WtProcessArgument`), título presente, `agentCommand` custom respetado. E2E no aplica (no se puede lanzar wt.exe en CI): verificar solo que con `terminal='none'` el comportamiento no cambió.

---

# 4 · `--dry-run` global

**Por qué.** Encaja con la filosofía que ya tiene la consola — mostrar el comando equivalente antes de correrlo — y hace auditable lo destructivo. Es además la red de contención de `each` y `sync`, que se construyen después.

**Contrato.**

- `--dry-run` es un flag **válido para todos los comandos** (se agrega a cada entrada de `Get-WtCommandSpecs`, o el parser lo trata como global; preferible lo segundo, con una sola línea en `ConvertFrom-WtArgs`).
- Con `--dry-run`, ningún comando externo se ejecuta y ningún archivo se escribe. En su lugar, cada operación se imprime con `Write-WtDetail` en la forma `[dry-run] git -C <repo> worktree add ...`.
- Alcanza a: git, `Start-Process`, escritura de tab configs, escritura de config, copia y `postCreate` de los hooks.
- Exit code 0 (B6): un ensayo exitoso no es un error.

**Cambios.**

1. `src/Common.ps1`: bandera de módulo `$script:WtDryRun` con `Set-WtDryRun`/`Test-WtDryRun`. `Invoke-WtProcess` y `Invoke-WtGit`, cuando está activa, imprimen y devuelven un objeto de éxito sintético (`ExitCode = 0`, `StdOut = @()`) **sin ejecutar**.
2. Las escrituras de archivo (`Save-WtConfigFile`, `Write-WtAgentTabConfig`, hooks) consultan `Test-WtDryRun` antes de escribir.
3. `Invoke-Wt`: setear la bandera al inicio según el flag y **resetearla siempre** en un `finally` — dejarla encendida entre invocaciones dentro del mismo proceso (el modo instalado, B5) sería un bug grave.
4. Cuidado con los comandos que **leen** para decidir: en dry-run, `git worktree list` y `rev-parse` deben seguir ejecutándose de verdad (son lecturas). Distinguir lectura de escritura en `Invoke-WtGit` con un parámetro `-ReadOnly`, o con una lista blanca de subcomandos de lectura documentada en la función.

**Pruebas.** E2E: `wt create x --dry-run` → exit 0, **el directorio no existe**, la rama no existe, la salida contiene `[dry-run] git`. `wt remove feature-a --dry-run --delete-branch` → el worktree sigue estando. `wt config set terminal none --dry-run` → el archivo no cambió. Unitaria: la bandera queda apagada después de `Invoke-Wt`.

---

# 5 · `wt status`

**Por qué.** Es la vista que falta para operar N agentes: hoy hay que entrar a cada worktree para saber qué pasó. Se apoya en el modelo de worktrees que ya existe.

**Contrato.**

- `wt status [<nombre>] [--json] [--fetch]`.
- Por worktree: nombre, rama, archivos modificados, staged, sin trackear, ahead/behind respecto de su **upstream** (y si no tiene upstream, respecto de `defaultBase`), y el último commit (hash corto + asunto + edad relativa).
- Sin nombre: todos los worktrees del repo actual. Con nombre: solo ese.
- `--fetch` hace `git fetch --all --prune` antes de calcular; **sin el flag no toca la red** (es un comando de consulta que se va a correr seguido).
- Los datos salen de dos comandos por worktree: `git status --porcelain=v2 --branch` y `git log -1 --format=...`. Un worktree que falle (bloqueado, disco desconectado) se reporta con su error en la fila, **no** rompe el listado del resto.
- `--json` emite el modelo completo vía `ConvertTo-WtJson` (que ya garantiza array).

**Cambios.**

1. `src/Repo.ps1`: `ConvertFrom-WtStatusPorcelainV2 -Text` → **pura**, devuelve `@{ Branch; Upstream; Ahead; Behind; Staged; Modified; Untracked }`. Es el corazón del ítem y se testea con salidas fijas, sin git. Usar `--porcelain=v2` (formato estable y con las líneas `# branch.ab`), no `v1`.
2. `src/Commands.ps1`: `Get-WtStatusRows` (pura, del modelo a filas de presentación, como `Get-WtWorktreeRows`) e `Invoke-WtStatus` (efectos).
3. `src/Cli.ps1`: spec `status` con flags `json`, `fetch`; alias `st`.
4. Rendimiento: es N×2 invocaciones de git. Reusar la caché de worktrees de B5 y medir con 10 worktrees; si pasa de ~2 s, evaluar `git status` con `--no-optional-locks`.

**Pruebas.** Unitarias del parser: limpio, con staged y modificados, sin upstream, detached, con `# branch.ab +2 -3`. E2E: crear dos worktrees, ensuciar uno, commitear en el otro, y verificar que `wt status --json` refleja ambos estados.

---

# 6 · `wt exec` y `wt each`

**Por qué.** Correr un comando en un worktree, o en todos, sin cambiar de directorio: build, tests, `git fetch`. Complemento natural de `status` para supervisar trabajo paralelo.

**Contrato.**

- `wt exec <nombre> -- <comando...>` — corre el comando en ese worktree.
- `wt each [--continue-on-error] [--json] -- <comando...>` — lo corre en **todos** los worktrees del repo actual, en serie (paralelo queda fuera de alcance: complica la salida y el orden).
- El separador `--` es obligatorio y todo lo que le sigue es el comando, sin interpretación de flags por parte de `wt`. **Esto exige tocar `ConvertFrom-WtArgs`** ([src/Cli.ps1:40-89](src/Cli.ps1#L40-L89)): al encontrar `--` suelto, el resto va crudo a `result.Rest`. Es la única función pura del parser: agregar sus unitarias.
- Salida por worktree: encabezado con el nombre (`Write-WtInfo`), la salida del comando tal cual, y el exit code si es ≠ 0.
- `each` sin `--continue-on-error` **corta en el primer fallo**; con el flag sigue y al final resume qué worktrees fallaron.
- Exit code (B6): 0 si todos salieron 0; 2 si alguno falló.
- `--dry-run` (ítem 4) lista qué se correría y dónde, sin correr nada.

**Cambios.**

1. `src/Cli.ps1`: soporte de `--` en el parser + specs de `exec` y `each`.
2. `src/Commands.ps1`: `Invoke-WtExec` e `Invoke-WtEach`, apoyadas en una `Get-WtEachPlan` **pura** que decide sobre qué worktrees se opera (excluye prunables y, salvo `--include-main`, el principal).

**Pruebas.** Unitarias del parser con `--` (incluyendo un comando que a su vez tiene flags: `wt each -- git log --oneline -1`). Unitarias de `Get-WtEachPlan`. E2E: `wt each -- cmd /c echo hola` toca los dos worktrees; un comando que falla corta o continúa según el flag, con el exit code correcto.

---

# 7 · `wt sync`

**Por qué.** Con varios agentes, la divergencia contra `develop` es el problema recurrente.

**Contrato.**

- `wt sync [<nombre>] [--base <rama>] [--strategy rebase|merge] [--continue-on-error]`.
- Default: `rebase` sobre `defaultBase` (o el upstream de la rama si existe; la base explícita gana).
- **Antes de tocar nada, se verifica que el worktree esté limpio.** Uno sucio se saltea con `Write-WtNotice`; nunca se hace stash automático — con un agente corriendo adentro, mover su árbol es inaceptable.
- Un conflicto **no se resuelve ni se aborta solo**: se reporta, se deja el worktree en el estado en que git lo dejó y se sigue con el próximo (o se corta, según el flag). El resumen final lista: sincronizados, salteados por sucios, en conflicto.
- Hace un solo `git fetch` al inicio, no uno por worktree.
- `--dry-run` imprime el plan completo sin ejecutar: es el uso principal las primeras veces.

**Cambios.**

1. `src/Commands.ps1`: `Get-WtSyncPlan` **pura** — recibe el modelo de estado del ítem 5 y devuelve, por worktree, la acción (`sync` | `skip-dirty` | `skip-no-base`) y el comando git a ejecutar. Toda la decisión es testeable sin git.
2. `Invoke-WtSync` ejecuta el plan y arma el resumen.
3. `src/Cli.ps1`: spec con `base` y `strategy` como valores; validar `strategy` con el mismo mecanismo de M5.

**Pruebas.** Unitarias de `Get-WtSyncPlan` sobre modelos sintéticos (limpio con base, sucio, sin upstream ni `defaultBase`, ya al día). E2E: dos worktrees, avanzar `develop`, `wt sync` → ambos rebasados; ensuciar uno → se saltea con el aviso; provocar un conflicto real → se reporta y el resumen lo lista.

---

# 8 · Consola: agente, terminal y worktrees

**Por qué.** El menú ofrece «editor» y «editor + agente», pero no «solo agente» ni «terminal», que sí existen en el CLI ([src/Console.ps1:225-235](src/Console.ps1#L225-L235)). El wrapper tiene que cubrir la superficie que envuelve.

**Contrato.** El menú expone, para el worktree elegido: editor · agente · terminal · editor + agente. Y suma las capacidades nuevas: `status` del repo, `sync`, e ir a un worktree (B12 ya extendió `cd`; acá se refleja en el menú). Se mantiene la regla que la consola ya cumple: **antes de ejecutar se muestra el comando CLI equivalente** (`Write-WtConsoleCommand`), que es lo que la hace didáctica.

**Cambios.**

1. `src/Console.ps1`: entradas nuevas en `Get-WtConsoleMenu`; reusar `Select-WtConsoleWorktree` para no duplicar la selección. Al crecer el menú, revisar el agrupado «Worktrees / Workspace» para que siga entrando en pantalla.
2. Verificar que ninguna acción nueva rompa el modo no interactivo: la suite E2E maneja la consola por stdin (`Invoke-WtConsole`), y toda opción tiene que tolerar EOF devolviendo el control, como ya hacen `Read-WtConsoleLine` y `Read-WtConsoleChoice`.

**Pruebas E2E.** Extender el bloque de consola existente: elegir la opción de agente con `terminal='none'` no rompe el menú; la opción de `status` imprime la tabla; EOF sale limpio con exit 0.

---

## Cierre del plan

- [ ] Las dos suites en verde, con las aserciones de los 8 ítems sumadas.
- [ ] CI en verde bajo PS 5.1 y 7.
- [ ] `README.md` con una sección por capacidad nueva y la tabla de config actualizada (todas las claves nuevas: `copyOnCreate`, `postCreate`, más las de correcciones).
- [ ] `wt help` cubre todos los comandos: `status`, `exec`, `each`, `sync`, `clean`, `lock`, `unlock`, `version`, y el `--dry-run` global.
- [ ] Ninguna clave nueva quedó admitida en el `.wt.json` del repo sin justificación escrita (A1).
- [ ] `CHANGELOG.md` y `ModuleVersion` al día.
