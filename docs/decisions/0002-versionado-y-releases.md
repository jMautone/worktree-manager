# 0002 — Versionado, ramas y releases

- **Fecha**: 2026-09-30
- **Estado**: aceptada
- **Antecedente**: [0001-de-cero-en-go.md](0001-de-cero-en-go.md)

## Contexto

Hasta `walking-skeleton` el repo no tenía una convención explícita, y se notaba:

- Las ramas de change se llamaban `v1/<change>`, pero el ruleset `protect-maintenance-branches` cubría `v1/**` con *no borrar* y *no force-push*. Cada rama de change quedaba indeleble (`v1/bootstrap` seguía ahí). `v1/` significaba a la vez "línea de mantenimiento" y "change en vuelo".
- El CI corría en push a `v1/**`: el nombre de rama estaba acoplado al CI.
- Nadie había decidido qué versión produce un merge. El CHANGELOG decía `Unreleased — v1` y el binario salía como `0.0.0-dev`: el `-X main.version` existía, pero nadie lo inyectaba.
- Títulos de PR y commits en español y sin formato fijo. Ningún GitHub Release.
- El squash usaba `COMMIT_OR_PR_TITLE`: un PR de un solo commit entraba a `main` con el mensaje del commit, no con el título del PR.

La línea PowerShell estaba en el tag `v0.9.0` y la rama `v0.9.x` (ver 0001).

## Decisión

### Versiones

SemVer. **Cada milestone es un minor**; `v1.0.0` sale cuando están hechos M1 a M5. M6 es opcional (`product.md` §10, riesgos), así que no condiciona la 1.0.

| Milestone | Versión |
|---|---|
| M1 | `0.1.0` |
| M2 | `0.2.0` |
| M3 | `0.3.0` |
| M4 | `0.4.0` |
| M5 | `1.0.0` |
| M6 (opcional) | `1.1.0` |

- Cada change o fix mergeado a `main` publica una pre-release `vX.Y.0-alpha.N` del **minor abierto**, con `N` consecutivo desde 1.
- Cerrar un milestone publica `vX.Y.0`, que pasa a ser *latest*.
- Los parches `vX.Y.Z` salen solo desde una rama `vX.Y.x`, creada cuando haga falta. Hasta entonces no tienen automatización.
- No se usan `beta` ni `rc`.

**Minor abierto**: si la última versión publicada es una alpha de `X.Y`, es `X.Y`. Si es un final `vX.Y.0`, es el siguiente minor de la tabla. Si no hay nada publicado, `0.1`.

Consecuencia directa: **los milestones son secuenciales**. Un change de M2 no se mergea hasta que se publica `v0.1.0`; si se mergeara antes, `v0.1.0` quedaría por debajo de `v0.2.0-alpha.1` y no podría publicarse. Es el mismo orden vertical que ya pide `product.md` §7.

### Ramas

| Patrón | Uso | Publica |
|---|---|---|
| `vX.Y/<change>` | Change de OpenSpec. `<change>` == `openspec/changes/<change>/`, kebab-case, inglés. `X.Y` es el minor del milestone del change. | alpha |
| `fix/<slug>` | Bug fuera de un change | alpha |
| `release/vX.Y.0` | Cierre de milestone | final |
| `chore/` `docs/` `ci/` `refactor/` `test/` + `<slug>` | Todo lo demás | nada |
| `dependabot/**` | Las crea Dependabot | nada |
| `vX.Y.x`, `powershell/v0.9.x` | Líneas de mantenimiento, protegidas | patch |

La versión exacta no va en la rama: `alpha.N` depende del orden de merge y no se conoce al abrirla. Lo que sí se conoce es el minor.

Las ramas de trabajo se borran solas al mergear.

### Títulos de PR

El título del PR es el commit que queda en `main`:

```
<type>(<scope>)[!]: <summary> [<version>]
```

- **type**: `feat` `fix` `docs` `chore` `ci` `refactor` `test` `perf`.
- **scope**: obligatorio, kebab-case.
- **summary**: inglés, imperativo, empieza en minúscula, sin punto final. El título sin el sufijo de versión no pasa de 72 caracteres.
- **`!`**: cambio que rompe el contrato de CLI. Se marca en las notas; no altera la versión (el minor ya lo decide el milestone).
- **`[<version>]`**: la versión exacta que publica el merge. **Es la fuente de verdad**: el workflow de release crea el tag que dice el título.

Reglas por rama:

| Rama | type | scope | Sufijo de versión |
|---|---|---|---|
| `vX.Y/<change>` | cualquiera | `<change>` | obligatorio: `vX.Y.0-alpha.N`, donde `X.Y` es el de la rama y tiene que ser el minor abierto |
| `fix/<slug>` | `fix` | libre | obligatorio: `vX.Y.0-alpha.N` del minor abierto |
| `release/vX.Y.0` | `chore` | `release` | obligatorio: `vX.Y.0`, igual a la rama, del minor abierto y con al menos una alpha publicada |
| `chore/` `docs/` `ci/` `refactor/` `test/` | igual al prefijo | libre | prohibido |
| `dependabot/**` | cualquiera | libre | prohibido |

Ejemplos:

```
feat(walking-skeleton): add wt list, wt config and CLI contract [v0.1.0-alpha.1]
fix(list): sort branch names case-insensitively [v0.1.0-alpha.3]
chore(release): close M1 [v0.1.0]
chore(ci): bump actions/checkout from 4 to 7
ci(release): add branch, title and release conventions
```

### Merge

Solo **squash**. El commit en `main` es `<título del PR> (#N)`, sin cuerpo. El cuerpo del PR va en español, con la plantilla de `.github/pull_request_template.md`; el detalle se lee en el PR. Los commits dentro de la rama son libres, porque el squash los descarta; se recomienda Conventional Commits.

### Releases

Cada versión publicada es un GitHub Release con:

- binarios de los 5 targets (`darwin/arm64`, `darwin/amd64`, `windows/amd64`, `linux/amd64`, `linux/arm64`) compilados con `-X main.version=<versión>`, en `tar.gz` (`zip` en Windows), más `checksums.txt`;
- en una alpha: pre-release, con notas generadas agrupando los títulos desde la versión anterior en *Features*, *Fixes* y *Other*;
- en un final: *latest*, con la sección `[X.Y.0]` de `CHANGELOG.md` como notas.

La distribución por brew, scoop y winget sigue siendo M6.

### CHANGELOG

Formato *Keep a Changelog*. Cada PR que publica suma sus líneas bajo `## [Unreleased]`. El PR `release/vX.Y.0` renombra esa sección a `## [X.Y.0] — <fecha>` y abre un `[Unreleased]` vacío. Las alphas no tocan el CHANGELOG.

### Automatización

```
PR abierto/editado ──► check pr-conventions ──► rama + título + versión
        │                                          (bloquea el merge si falla)
   squash merge
        ▼
push a main ──► release.yml ──► ¿el commit tiene [vX.Y.Z…]?
                                   no → termina
                                   sí → re-valida → tag anotado → GoReleaser → Release
```

- **`tools/relcheck/`** (Go, sin dependencias nuevas): una función pura que recibe rama, título, versiones ya publicadas y dos hechos del árbol (si existe el change de OpenSpec, si `CHANGELOG.md` tiene la sección del final), y devuelve los errores. Tiene tests y corre en la matrix como el resto. Las versiones publicadas salen de los sufijos `[v…]` de los títulos en `main`, no de los tags, para no depender de que el workflow de release ya haya terminado.
- **`pr-conventions`** (workflow en cada PR a `main`): valida las tablas de arriba con `relcheck`. Para `vX.Y/<change>` exige que exista `openspec/changes/<change>/` o `openspec/changes/archive/*-<change>/`. Para `release/vX.Y.0` exige la sección `## [X.Y.0]` en `CHANGELOG.md` y al menos una alpha publicada de `X.Y`.
- **`release.yml`** (push a `main`, y `workflow_dispatch` en modo dry-run): si el commit trae sufijo de versión, re-valida, crea el tag anotado y corre GoReleaser en el **mismo job**. Un tag creado con `GITHUB_TOKEN` no dispara otros workflows.
- **CI**: el job `cross-build` corre `goreleaser build --snapshot` y conserva el nombre, porque es un check requerido. Los targets se definen solo en `.goreleaser.yaml`, así que el CI compila lo mismo que se publica. El push dispara en `main`, `v*/**`, `fix/**` y `release/**`.
- **`go.mod`** declara `retract v0.9.0` (ver *Línea PowerShell*).

### Protecciones

Settings del repo:

- solo squash (sin rebase ni merge commit);
- título del squash `PR_TITLE`, cuerpo `BLANK`;
- borrar la rama al mergear.

Rulesets:

| Ruleset | Cubre | Reglas |
|---|---|---|
| `main` | rama por defecto | sin borrar, sin force-push, historia lineal, PR con solo squash, checks requeridos (los 3 `test`, `cross-build`, `pr-conventions`) con la rama al día |
| `protect-maintenance-branches` | `v*.*.x`, `powershell/**` | sin borrar, sin force-push |
| `protect-release-tags` | `v*`, `powershell-v*` | sin borrar, sin mover |

"Rama al día" evita que dos PR abiertos tomen la misma alpha: al mergear uno, el otro tiene que actualizarse, el check corre de nuevo y ve la versión ya usada. Cuesta un click en *Update branch*.

### Línea PowerShell

Se renombra para que no compita con la numeración `0.x`:

| Antes | Después |
|---|---|
| tag `v0.9.0` | tag `powershell-v0.9.0` (anotado, mismo commit `e1447e5`) |
| rama `v0.9.x` | rama `powershell/v0.9.x` |

`powershell-v0.9.0` no tiene forma semver, así que Go y GoReleaser lo ignoran como versión.

**Limitación conocida**: `proxy.golang.org` ya cacheó `v0.9.0`, y el proxy no borra versiones. Mientras la línea Go esté en `0.x`, `go install …/cmd/git-wt@latest` resuelve `v0.9.0`, que no tiene código Go, y falla. `retract` no lo arregla todavía: Go solo lo respeta si está publicado en una versión mayor a la retractada, y la primera es `v1.0.0`. Se declara igual ahora, para que la 1.0 lo traiga. Mientras tanto, se instala desde los binarios del Release o con versión explícita (`@v0.1.0-alpha.1`).

## Transición

El orden importa: la fase 3 exige un check que tiene que existir antes en `main`.

1. **Repo, sin código**, con el bypass de admin de los rulesets:
   1. mergear los PR de Dependabot abiertos (#4, #5), que ya cumplen la convención y tocan `ci.yml`;
   2. crear `powershell-v0.9.0` y `powershell/v0.9.x`;
   3. borrar `v0.9.0`, `v0.9.x`, `v1/bootstrap` y `chore/dependabot-config`;
   4. renombrar `v1/walking-skeleton` → `v0.1/walking-skeleton` con la API de rename, que mantiene el PR #3;
   5. ajustar rulesets (sin `pr-conventions` todavía) y settings.
2. **PR `ci/release-conventions`**, que no publica: esta ADR, `CONTRIBUTING.md`, `tools/relcheck/`, workflows, `.goreleaser.yaml`, plantilla de PR, `retract`, y los ajustes de `README.md`, `CHANGELOG.md`, `product.md` §7 y `openspec/config.yaml`. Una nota al principio de 0000 y 0001 apunta acá, sin reescribirlas.
3. **Activar el candado**: `pr-conventions` como check requerido, con la rama al día.
4. **PR #3 → `v0.1.0-alpha.1`**: actualizar con `main` (merge, sin force-push; el squash lo aplana), sus líneas bajo `[Unreleased]`, título `feat(walking-skeleton): add wt list, wt config and CLI contract [v0.1.0-alpha.1]`, squash. Se verifica que el Release sea pre-release, que tenga los 5 assets más `checksums.txt`, y que `git-wt version` del binario descargado diga `wt 0.1.0-alpha.1 (…)`.

El design archivado de `walking-skeleton` menciona `v0.9.x`; se deja como está, porque es historia.

## Consecuencias

- Cada versión es trazable de punta a punta: rama `v0.1/<change>` → título `[v0.1.0-alpha.N]` → tag → Release → `wt version`.
- La versión dice en qué milestone está el producto.
- Los milestones no se solapan: M2 no arranca a mergear hasta publicar `v0.1.0`.
- `go install …@latest` no funciona hasta `v1.0.0`.
- Un merge a `main` con PR abiertos obliga a actualizar los demás antes de mergearlos.
- Los proposals de OpenSpec declaran su rama `vX.Y/<change>` en la sección Milestone (regla en `openspec/config.yaml`).

## Alternativas descartadas

- **Numerar la línea Go desde `v0.10.0`** sin renombrar PowerShell: evitaba el problema del proxy, pero sugiere una continuidad que no existe y `0.10` es menos legible que `0.1`.
- **`v1.0.0` al cerrar M1**: compromete el contrato de CLI demasiado temprano.
- **`v1.0.0` al cerrar M6**: M6 es opcional; la 1.0 podría no llegar nunca.
- **Milestone en la pre-release (`v1.0.0-m1.1`)**: poco estándar, y la pre-release duraba todo el roadmap.
- **Ramas `mN/<change>`**: no se leen como versión y obligan a recordar que `m5` es `1.0`.
- **Ramas `<type>/<change>`**: no muestran la versión.
- **Merge por rebase**: cada commit de la rama llega a `main` y las notas se llenan de ruido.
- **Título libre (`[v0.1] …`)**: no se puede parsear.
- **Scope igual a la rama (`feat(v0.1/x): …`)**: muestra el minor pero no la versión exacta.
- **Que todo merge publique**: una alpha por cada bump de Dependabot.
- **Releases sin binarios**: exige Go en Windows para la verificación manual que piden las tasks.
- **CHANGELOG automático (git-cliff)**: pierde la prosa curada.
- **release-please**: calcula la versión desde los commits; choca con "el título es la fuente de verdad" y con el salto de M5 a 1.0.
- **Taguear a mano**: no garantiza que título y tag coincidan.
- **`relcheck` en bash**: difícil de testear; contradice la regla de que toda tarea lleva su test.
- **Restringir la creación de tags al workflow**: no hace falta hoy.
