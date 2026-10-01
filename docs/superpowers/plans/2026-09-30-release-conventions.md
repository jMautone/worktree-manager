# Convenciones de versionado, ramas y releases — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que cada merge a `main` siga la convención de la ADR 0002 sin depender de la memoria: ramas `vX.Y/<change>`, títulos Conventional con la versión exacta, y un tag más un GitHub Release con binarios por cada versión, empezando por `v0.1.0-alpha.1` para walking-skeleton.

**Architecture:** Un programa Go chico, `tools/relcheck`, concentra todas las reglas en funciones puras con tests. Lo usan dos workflows: `pr-conventions` en cada PR (valida rama, título y versión) y `release.yml` en cada push a `main` (lee la versión del título, crea el tag y publica con GoReleaser). Los settings y rulesets del repo hacen que el título del PR sea el commit en `main` y que el check sea obligatorio.

**Tech Stack:** Go 1.27 (solo stdlib en relcheck), GitHub Actions (`actions/checkout@v7`, `actions/setup-go@v7`, `goreleaser/goreleaser-action@v7`), GoReleaser v2 (validado con v2.18.2), `gh` CLI y `jq` para settings y rulesets.

**Spec:** [`docs/decisions/0002-versionado-y-releases.md`](../../decisions/0002-versionado-y-releases.md)

## Global Constraints

- Go 1.27. **Sin dependencias nuevas** en `go.mod`: `relcheck` usa solo la stdlib.
- Código, comentarios, mensajes de error, `CONTRIBUTING.md` y `README.md` en **inglés**. ADRs, `product.md`, la plantilla y los cuerpos de PR en **español**.
- Toda tarea con código termina con `gofmt -l .` vacío, `go vet ./...`, `go build ./...` y `go test ./...` en verde.
- Commits dentro de la rama: Conventional Commits en inglés, terminados con `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Acciones: `actions/checkout@v7`, `actions/setup-go@v7` y `goreleaser/goreleaser-action@v7` con `version: "~> v2"`.
- El check requerido se llama exactamente `pr-conventions` (nombre del job). El job `cross-build` conserva su nombre porque ya es un check requerido.
- El título y la rama del PR llegan a los scripts de los workflows **por `env:`**, nunca interpolados con `${{ }}` dentro de `run:`.
- Este plan se commitea en la rama, pero se saca del índice antes de abrir el PR (Task 12). Nunca llega a `main`.
- Las operaciones sobre GitHub (Fases 1, 3 y 4) son externas y en parte irreversibles: **pedirle confirmación al usuario antes de cada paso marcado con ⚠️**.
- Repo: `jMautone/worktree-manager`. El rol admin del usuario tiene bypass `always` en `protect-maintenance-branches` y `protect-release-tags`, así que no hace falta desactivar rulesets.
- Durante la planificación quedó instalado `~/go/bin/goreleaser` (v2.18.2). Sirve para las verificaciones locales; si no está, usar `go run github.com/goreleaser/goreleaser/v2@v2.18.2`.

## Review Focus

1. **Re-correr `release.yml` después de una falla parcial** (el tag se pusheó y GoReleaser falló): la segunda corrida no tiene que fallar por su propio tag, pero sí si el tag apunta a otro commit. Tests en Task 8 (`TestRunMergeIsIdempotentAfterTagging`, `TestRunMergeRejectsTagOnAnotherCommit`) y `replace_existing_artifacts` en Task 9.
2. **Dos PR abiertos que declaran la misma alpha**: el que se mergea segundo tiene que fallar con "already published" y sugerir la siguiente. Tests en Task 4 (`TestNextCheck`), Task 7 (`TestCheckPRInvalid`) y Task 8 (`TestRunPRReadsPublishedVersionsFromBase`).
3. **Sufijo de versión mal escrito** (`[0.1.0-alpha.1]`, `[v0.1]`, `[wip]`): error explícito. Nunca se trata como parte del summary ni se ignora al leer la historia de `main`. Tests en Task 5 (`TestParseTitleErrors`, `TestPublishedVersions`).
4. **Nombre de change que es sufijo de otro archivado** (`integration` contra `2026-09-30-shell-integration`): no cuenta como existente. Test en Task 6 (`TestArchivedAs`).
5. **Encabezados de CHANGELOG parecidos** (`## [0.1.01]`, `## [0.1.0-alpha.1]`), CRLF y el separador `---`: solo vale la sección exacta, sin el separador. Test en Task 7 (`TestChangelogSection`).

## Estructura de archivos

| Archivo | Responsabilidad |
|---|---|
| `tools/relcheck/version.go` | `Version`, `Minor`, tabla `Milestones`, `OpenMinor`, `NextAlpha`, `NextCheck` |
| `tools/relcheck/title.go` | `Title`, `ParseTitle`, `SubjectVersion`, `PublishedVersions` |
| `tools/relcheck/branch.go` | `Branch`, `ParseBranch`, `ArchivedAs` |
| `tools/relcheck/changelog.go` | `ChangelogSection` |
| `tools/relcheck/check.go` | `Facts`, `CheckPR` |
| `tools/relcheck/main.go` | CLI `pr` / `merge` / `notes`; el único archivo que toca git y el filesystem |
| `tools/relcheck/*_test.go` | Unit tests por archivo; `main_test.go` contra repos git reales en temporales |
| `.goreleaser.yaml` | Targets, archivos, checksums, notas y pre-release |
| `.github/workflows/ci.yml` | Triggers nuevos; `cross-build` pasa a GoReleaser |
| `.github/workflows/pr-conventions.yml` | Check `pr-conventions` |
| `.github/workflows/release.yml` | Tag y Release en cada push a `main`; dry-run por `workflow_dispatch` |
| `.github/pull_request_template.md` | Plantilla en español |
| `CONTRIBUTING.md` | La convención operativa, en inglés |
| `README.md`, `CHANGELOG.md`, `go.mod`, `docs/design/product.md`, `openspec/config.yaml`, ADR 0000 y 0001 | Ajustes puntuales |

---

## Fase 1 — Repo, sin código

### Task 1: Mergear los PR de Dependabot y actualizar la rama

**Files:** ninguno en el repo; cambia `main` en GitHub.

**Interfaces:**
- Produces: `main` con `actions/checkout@v7` y `actions/setup-go@v7` en `.github/workflows/ci.yml`, la base de Task 9.

- [ ] **Step 1: Verificar que #4 y #5 siguen verdes y mergeables**

Run: `gh pr view 4 --json state,mergeable,statusCheckRollup --jq '[.state, .mergeable, ([.statusCheckRollup[].conclusion] | unique)]'` (y lo mismo con `5`)
Expected: `["OPEN","MERGEABLE",["SUCCESS"]]` en los dos.

- [ ] **Step 2: ⚠️ Mergear, con confirmación del usuario**

```bash
gh pr merge 4 --squash
gh pr merge 5 --squash
```

Si #5 queda en conflicto después de #4, correr `gh pr comment 5 --body "@dependabot rebase"`, esperar a que quede verde y mergear.

- [ ] **Step 3: Verificar `main`**

Run: `git fetch origin --prune && git show origin/main:.github/workflows/ci.yml | grep -n 'uses:'`
Expected: cuatro líneas, todas con `@v7`.

- [ ] **Step 4: Rebasar la rama local sobre el `main` nuevo**

La rama `ci/release-conventions` todavía no se pusheó, así que el rebase es seguro.

```bash
git switch ci/release-conventions
git rebase origin/main
git log --oneline origin/main..HEAD
```

Expected: solo los commits de la ADR 0002 y de este plan.

### Task 2: Renombrar la línea PowerShell y limpiar ramas

**Files:** ninguno en el repo; cambian refs en GitHub.

**Interfaces:**
- Produces: tag `powershell-v0.9.0`, rama `powershell/v0.9.x` y rama `v0.1/walking-skeleton` como head del PR #3. Ya no existen `v0.9.0`, `v0.9.x`, `v1/bootstrap`, `chore/dependabot-config` ni `v1/walking-skeleton`.

- [ ] **Step 1: Registrar el estado actual**

Run: `git ls-remote origin refs/tags/v0.9.0 'refs/tags/v0.9.0^{}' refs/heads/v0.9.x refs/heads/v1/bootstrap refs/heads/chore/dependabot-config refs/heads/v1/walking-skeleton`
Expected: `v0.9.0^{}` y `v0.9.x` en `e1447e5c0a1e6ce15b6f342b1d4956a027bbc5b4`, más las otras cuatro refs.

- [ ] **Step 2: Crear el tag y la rama nuevos, conservando el mensaje original**

```bash
git fetch origin --tags
msg="$(git tag -l --format='%(contents)' v0.9.0)"
git tag -a powershell-v0.9.0 e1447e5c0a1e6ce15b6f342b1d4956a027bbc5b4 \
  -m "$msg" -m "Renamed from v0.9.0. See docs/decisions/0002-versionado-y-releases.md."
git push origin refs/tags/powershell-v0.9.0 \
  e1447e5c0a1e6ce15b6f342b1d4956a027bbc5b4:refs/heads/powershell/v0.9.x
```

- [ ] **Step 3: Verificar las refs nuevas**

Run: `git ls-remote origin 'refs/tags/powershell-v0.9.0^{}' refs/heads/powershell/v0.9.x`
Expected: las dos en `e1447e5c0a1e6ce15b6f342b1d4956a027bbc5b4`.

- [ ] **Step 4: ⚠️ Borrar las refs viejas, con confirmación del usuario**

```bash
git push origin --delete refs/tags/v0.9.0 refs/heads/v0.9.x \
  refs/heads/v1/bootstrap refs/heads/chore/dependabot-config
git tag -d v0.9.0
git branch -D v0.9.x v1/bootstrap chore/dependabot-config 2>/dev/null || true
git fetch origin --prune
```

- [ ] **Step 5: ⚠️ Renombrar la rama del PR #3, con confirmación del usuario**

```bash
gh api -X POST repos/jMautone/worktree-manager/branches/v1/walking-skeleton/rename \
  -f new_name='v0.1/walking-skeleton'
```

Si la API responde 404, reintentar con la barra codificada: `.../branches/v1%2Fwalking-skeleton/rename`.

Run: `gh pr view 3 --json headRefName --jq .headRefName`
Expected: `v0.1/walking-skeleton`

- [ ] **Step 6: Actualizar la rama local**

```bash
git branch -m v1/walking-skeleton v0.1/walking-skeleton
git fetch origin --prune
git branch --set-upstream-to=origin/v0.1/walking-skeleton v0.1/walking-skeleton
```

- [ ] **Step 7: Verificar el conjunto completo**

Run: `git ls-remote --heads --tags origin | awk '{print $2}' | grep -v '^refs/heads/dependabot/'`
Expected (sin importar el orden): `refs/heads/main`, `refs/heads/powershell/v0.9.x`, `refs/heads/v0.1/walking-skeleton`, `refs/tags/powershell-v0.9.0` y `refs/tags/powershell-v0.9.0^{}`.

### Task 3: Settings del repo y rulesets

**Files:** ninguno en el repo; cambian settings y rulesets en GitHub.

**Interfaces:**
- Produces: squash como único método, con título `PR_TITLE` y cuerpo `BLANK`; ramas borradas al mergear; rulesets en el estado de fase 1 (el check `pr-conventions` se suma en Task 13).

- [ ] **Step 1: ⚠️ Settings, con confirmación del usuario**

```bash
gh api -X PATCH repos/jMautone/worktree-manager \
  -F allow_squash_merge=true -F allow_rebase_merge=false -F allow_merge_commit=false \
  -f squash_merge_commit_title=PR_TITLE -f squash_merge_commit_message=BLANK \
  -F delete_branch_on_merge=true --jq '{allow_squash_merge, allow_rebase_merge, allow_merge_commit, squash_merge_commit_title, squash_merge_commit_message, delete_branch_on_merge}'
```

Expected: `{"allow_merge_commit":false,"allow_rebase_merge":false,"allow_squash_merge":true,"delete_branch_on_merge":true,"squash_merge_commit_message":"BLANK","squash_merge_commit_title":"PR_TITLE"}`

- [ ] **Step 2: ⚠️ Rulesets, con confirmación del usuario**

Cada ruleset se lee, se edita con `jq` y se reemplaza con `PUT`. Así se conservan los campos que no se tocan.

```bash
R=repos/jMautone/worktree-manager/rulesets
rid() { gh api "$R" --jq ".[] | select(.name==\"$1\") | .id"; }
body='{name, target, enforcement, bypass_actors, conditions, rules}'

id=$(rid main)
gh api "$R/$id" | jq "$body
  | (.rules[] | select(.type==\"pull_request\") | .parameters.allowed_merge_methods) = [\"squash\"]" \
  | gh api -X PUT "$R/$id" --input - >/dev/null

id=$(rid protect-maintenance-branches)
gh api "$R/$id" | jq "$body
  | .conditions.ref_name.include = [\"refs/heads/v*.*.x\", \"refs/heads/powershell/**\"]" \
  | gh api -X PUT "$R/$id" --input - >/dev/null

id=$(rid protect-release-tags)
gh api "$R/$id" | jq "$body
  | .conditions.ref_name.include = [\"refs/tags/v*\", \"refs/tags/powershell-v*\"]" \
  | gh api -X PUT "$R/$id" --input - >/dev/null
```

Si un `PUT` rechaza un campo de `parameters` que vino del GET (por ejemplo `required_reviewers`), quitarlo con `del(...)` en el `jq` y reintentar.

- [ ] **Step 3: Verificar**

```bash
R=repos/jMautone/worktree-manager/rulesets
for id in $(gh api "$R" --jq '.[].id'); do
  gh api "$R/$id" --jq '{name, include: .conditions.ref_name.include,
    merge: [.rules[] | select(.type=="pull_request") | .parameters.allowed_merge_methods][0]}'
done
```

Expected: `main` con `merge: ["squash"]`; `protect-maintenance-branches` con `["refs/heads/v*.*.x","refs/heads/powershell/**"]`; `protect-release-tags` con `["refs/tags/v*","refs/tags/powershell-v*"]`.

---

## Fase 2 — PR `ci/release-conventions`

Todas las tareas de esta fase van en la rama `ci/release-conventions`.

### Task 4: relcheck, versiones

**Files:**
- Create: `tools/relcheck/version.go`
- Create: `tools/relcheck/main.go` (provisorio; Task 8 lo reemplaza)
- Test: `tools/relcheck/version_test.go`

**Interfaces:**
- Produces:
  - `type Version struct{ Major, Minor, Patch, Alpha int }` (`Alpha == 0` es un final)
  - `func ParseVersion(s string) (Version, error)`
  - `func (Version) String() string`, `IsAlpha() bool`, `MinorOf() Minor`, `Less(Version) bool`
  - `type Minor struct{ Major, Minor int }` con `String()` → `"0.1"`
  - `var Milestones []Minor`
  - `func OpenMinor([]Version) (Minor, error)`
  - `func NextAlpha([]Version) (Version, error)`
  - `func NextCheck(v Version, published []Version) error`
  - `func run(args []string, stdout, stderr io.Writer) int`, provisoria en `main.go`

- [ ] **Step 1: Escribir los tests**

`tools/relcheck/version_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func mustVersions(t *testing.T, ss ...string) []Version {
	t.Helper()
	var vs []Version
	for _, s := range ss {
		v, err := ParseVersion(s)
		if err != nil {
			t.Fatal(err)
		}
		vs = append(vs, v)
	}
	return vs
}

func TestParseVersion(t *testing.T) {
	good := map[string]Version{
		"v0.1.0":          {0, 1, 0, 0},
		"v0.1.0-alpha.1":  {0, 1, 0, 1},
		"v1.0.0":          {1, 0, 0, 0},
		"v10.20.3":        {10, 20, 3, 0},
		"v0.2.0-alpha.12": {0, 2, 0, 12},
	}
	for in, want := range good {
		got, err := ParseVersion(in)
		if err != nil || got != want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %v", in, got, err, want)
		}
		if got.String() != in {
			t.Errorf("String() = %q, want %q", got.String(), in)
		}
	}
	for _, in := range []string{"", "0.1.0", "v0.1", "v01.1.0", "v0.1.0-alpha.0", "v0.1.0-beta.1", "v0.1.0-alpha", "v0.1.0 "} {
		if _, err := ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) succeeded, want error", in)
		}
	}
}

func TestLess(t *testing.T) {
	ordered := mustVersions(t, "v0.1.0-alpha.1", "v0.1.0-alpha.2", "v0.1.0-alpha.10", "v0.1.0", "v0.2.0-alpha.1", "v0.4.0", "v1.0.0-alpha.1", "v1.0.0")
	for i := 0; i < len(ordered)-1; i++ {
		a, b := ordered[i], ordered[i+1]
		if !a.Less(b) || b.Less(a) {
			t.Errorf("want %s < %s", a, b)
		}
	}
}

func TestOpenMinor(t *testing.T) {
	cases := []struct {
		published []string
		want      Minor
	}{
		{nil, Minor{0, 1}},
		{[]string{"v0.1.0-alpha.1"}, Minor{0, 1}},
		{[]string{"v0.1.0-alpha.1", "v0.1.0"}, Minor{0, 2}},
		{[]string{"v0.1.0", "v0.1.0-alpha.1"}, Minor{0, 2}}, // order of input does not matter
		{[]string{"v0.4.0-alpha.3", "v0.4.0"}, Minor{1, 0}}, // M5 ships 1.0
		{[]string{"v1.0.0"}, Minor{1, 1}},
	}
	for _, c := range cases {
		got, err := OpenMinor(mustVersions(t, c.published...))
		if err != nil || got != c.want {
			t.Errorf("OpenMinor(%v) = %v, %v; want %v", c.published, got, err, c.want)
		}
	}
}

func TestOpenMinorErrors(t *testing.T) {
	for _, published := range [][]string{{"v1.1.0"}, {"v0.5.0"}} {
		if _, err := OpenMinor(mustVersions(t, published...)); err == nil {
			t.Errorf("OpenMinor(%v) succeeded, want error", published)
		}
	}
}

func TestNextAlpha(t *testing.T) {
	cases := map[string][]string{
		"v0.1.0-alpha.1": nil,
		"v0.1.0-alpha.3": {"v0.1.0-alpha.1", "v0.1.0-alpha.2"},
		"v0.2.0-alpha.1": {"v0.1.0-alpha.1", "v0.1.0"},
	}
	for want, published := range cases {
		got, err := NextAlpha(mustVersions(t, published...))
		if err != nil || got.String() != want {
			t.Errorf("NextAlpha(%v) = %v, %v; want %s", published, got, err, want)
		}
	}
}

func TestNextCheck(t *testing.T) {
	cases := []struct {
		v         string
		published []string
		wantErr   string // empty means valid
	}{
		{"v0.1.0-alpha.1", nil, ""},
		{"v0.1.0-alpha.2", []string{"v0.1.0-alpha.1"}, ""},
		{"v0.1.0", []string{"v0.1.0-alpha.1"}, ""},
		{"v0.2.0-alpha.1", []string{"v0.1.0-alpha.1", "v0.1.0"}, ""},
		{"v1.0.0-alpha.1", []string{"v0.4.0-alpha.1", "v0.4.0"}, ""},
		// Two PRs raced for the same alpha: the second one must fail.
		{"v0.1.0-alpha.1", []string{"v0.1.0-alpha.1"}, "already published"},
		{"v0.1.0-alpha.3", []string{"v0.1.0-alpha.1"}, "next alpha of 0.1 is v0.1.0-alpha.2"},
		{"v0.2.0-alpha.1", []string{"v0.1.0-alpha.1"}, "open minor is 0.1"},
		{"v0.1.0-alpha.2", []string{"v0.1.0-alpha.1", "v0.1.0"}, "open minor is 0.2"},
		{"v0.1.0", nil, "at least one alpha"},
		{"v0.1.1", []string{"v0.1.0-alpha.1", "v0.1.0"}, "vX.Y.x branch"},
		{"v0.5.0-alpha.1", []string{"v0.4.0-alpha.1", "v0.4.0"}, "open minor is 1.0"},
	}
	for _, c := range cases {
		err := NextCheck(mustVersions(t, c.v)[0], mustVersions(t, c.published...))
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("NextCheck(%s, %v) = %v, want nil", c.v, c.published, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("NextCheck(%s, %v) = %v, want error containing %q", c.v, c.published, err, c.wantErr)
		}
	}
}
```

- [ ] **Step 2: Correr los tests y verificar que fallan**

Run: `go test ./tools/relcheck/`
Expected: FAIL de compilación con `undefined: ParseVersion` (y similares).

- [ ] **Step 3: Implementar**

`tools/relcheck/version.go`:

```go
// Command relcheck enforces the branch, PR title and version conventions of
// docs/decisions/0002-versionado-y-releases.md. The pr-conventions workflow
// runs it on every pull request to main, and release.yml runs it on every
// push to main.
//
// The decisions live in pure functions (version.go, title.go, branch.go,
// check.go, changelog.go). main.go only reads git and the filesystem.
package main

import (
	"fmt"
	"regexp"
	"strconv"
)

// Version is a release version: vMAJOR.MINOR.PATCH, optionally -alpha.N.
type Version struct {
	Major, Minor, Patch int
	Alpha               int // 0 for a final release
}

var versionRE = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-alpha\.([1-9][0-9]*))?$`)

// ParseVersion parses "v0.1.0" or "v0.1.0-alpha.2".
func ParseVersion(s string) (Version, error) {
	m := versionRE.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("%q is not a version: want vX.Y.Z or vX.Y.Z-alpha.N", s)
	}
	var v Version
	v.Major, _ = strconv.Atoi(m[1])
	v.Minor, _ = strconv.Atoi(m[2])
	v.Patch, _ = strconv.Atoi(m[3])
	if m[4] != "" {
		v.Alpha, _ = strconv.Atoi(m[4])
	}
	return v, nil
}

func (v Version) String() string {
	s := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Alpha > 0 {
		s += fmt.Sprintf("-alpha.%d", v.Alpha)
	}
	return s
}

// IsAlpha reports whether v is a pre-release.
func (v Version) IsAlpha() bool { return v.Alpha > 0 }

// MinorOf returns the minor line v belongs to.
func (v Version) MinorOf() Minor { return Minor{v.Major, v.Minor} }

// Less orders versions by SemVer: v0.1.0-alpha.2 < v0.1.0 < v0.2.0-alpha.1.
func (v Version) Less(w Version) bool {
	if v.Major != w.Major {
		return v.Major < w.Major
	}
	if v.Minor != w.Minor {
		return v.Minor < w.Minor
	}
	if v.Patch != w.Patch {
		return v.Patch < w.Patch
	}
	if v.IsAlpha() != w.IsAlpha() {
		return v.IsAlpha() // a pre-release sorts before its final
	}
	return v.Alpha < w.Alpha
}

// Minor is a MAJOR.MINOR line. Each milestone ships as one minor.
type Minor struct{ Major, Minor int }

func (m Minor) String() string { return fmt.Sprintf("%d.%d", m.Major, m.Minor) }

// Milestones maps M1..M6 to their minor, in order. M5 ships 1.0.0.
// Keep it in sync with the table in docs/decisions/0002-versionado-y-releases.md.
var Milestones = []Minor{{0, 1}, {0, 2}, {0, 3}, {0, 4}, {1, 0}, {1, 1}}

// OpenMinor returns the minor that merges to main publish into: the minor of
// the latest published alpha, or the milestone after the latest final.
func OpenMinor(published []Version) (Minor, error) {
	if len(published) == 0 {
		return Milestones[0], nil
	}
	latest := published[0]
	for _, v := range published[1:] {
		if latest.Less(v) {
			latest = v
		}
	}
	if latest.IsAlpha() {
		return latest.MinorOf(), nil
	}
	for i, m := range Milestones {
		if m != latest.MinorOf() {
			continue
		}
		if i+1 == len(Milestones) {
			return Minor{}, fmt.Errorf("%s closed the last milestone in the table; extend Milestones and ADR 0002", latest)
		}
		return Milestones[i+1], nil
	}
	return Minor{}, fmt.Errorf("latest version %s is not a milestone minor (ADR 0002)", latest)
}

// NextAlpha returns the alpha the next change or fix publishes.
func NextAlpha(published []Version) (Version, error) {
	open, err := OpenMinor(published)
	if err != nil {
		return Version{}, err
	}
	return Version{Major: open.Major, Minor: open.Minor, Alpha: lastAlpha(published, open) + 1}, nil
}

func lastAlpha(published []Version, m Minor) int {
	last := 0
	for _, p := range published {
		if p.MinorOf() == m && p.Alpha > last {
			last = p.Alpha
		}
	}
	return last
}

// NextCheck reports whether v may be published next, given every version
// main already published.
func NextCheck(v Version, published []Version) error {
	if v.Patch != 0 {
		return fmt.Errorf("%s: patch releases come from a vX.Y.x branch, not from main", v)
	}
	for _, p := range published {
		if p == v {
			return fmt.Errorf("%s is already published", v)
		}
	}
	open, err := OpenMinor(published)
	if err != nil {
		return err
	}
	if v.MinorOf() != open {
		return fmt.Errorf("%s targets %s, but the open minor is %s", v, v.MinorOf(), open)
	}
	last := lastAlpha(published, open)
	if v.IsAlpha() {
		if v.Alpha != last+1 {
			next := Version{Major: open.Major, Minor: open.Minor, Alpha: last + 1}
			return fmt.Errorf("%s: the next alpha of %s is %s", v, open, next)
		}
		return nil
	}
	if last == 0 {
		return fmt.Errorf("%s: a final needs at least one alpha of %s published first", v, open)
	}
	return nil
}
```

`tools/relcheck/main.go` provisorio. Sin `func main` el paquete no compila y `go build ./...` del CI falla:

```go
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is replaced by the full CLI in Task 8.
func run(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "relcheck: no subcommands yet")
	return 2
}
```

- [ ] **Step 4: Correr los tests y verificar que pasan**

Run: `go test ./tools/relcheck/ && go vet ./... && go build ./... && gofmt -l .`
Expected: `ok`, sin salida de vet ni de gofmt.

- [ ] **Step 5: Commit**

```bash
git add tools/relcheck/
git commit -m "feat(relcheck): add versions, milestones and next-version rules" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 5: relcheck, títulos y subjects

**Files:**
- Create: `tools/relcheck/title.go`
- Test: `tools/relcheck/title_test.go`

**Interfaces:**
- Consumes: `Version`, `ParseVersion` (Task 4).
- Produces:
  - `type Title struct{ Type, Scope, Summary string; Breaking bool; Version *Version }`
  - `func ParseTitle(s string) (Title, error)`
  - `func SubjectVersion(subject string) (v Version, ok bool, err error)`
  - `func PublishedVersions(subjects []string) ([]Version, error)`

- [ ] **Step 1: Escribir los tests**

`tools/relcheck/title_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestParseTitle(t *testing.T) {
	got, err := ParseTitle("feat(walking-skeleton)!: add wt list, wt config and CLI contract [v0.1.0-alpha.1]")
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "feat" || got.Scope != "walking-skeleton" || !got.Breaking ||
		got.Summary != "add wt list, wt config and CLI contract" ||
		got.Version == nil || got.Version.String() != "v0.1.0-alpha.1" {
		t.Errorf("ParseTitle = %+v", got)
	}

	got, err = ParseTitle("chore(ci): bump actions/checkout from 4 to 7")
	if err != nil || got.Version != nil || got.Breaking {
		t.Errorf("ParseTitle without version = %+v, %v", got, err)
	}

	// The 72-character limit does not count the version suffix.
	long := "docs(x): " + strings.Repeat("a", 63) // 72 characters
	if _, err := ParseTitle(long + " [v0.1.0-alpha.1]"); err != nil {
		t.Errorf("72 characters plus suffix: %v", err)
	}
}

func TestParseTitleErrors(t *testing.T) {
	cases := map[string]string{
		"add wt list":                         "does not match",
		"feat: add wt list":                   "does not match",
		"feat(): add wt list":                 "kebab-case",
		"feat(Walking): add wt list":          "kebab-case",
		"feature(list): add wt list":          "is not one of",
		"feat(list): Add wt list":             "lowercase",
		"feat(list): add wt list.":            "period",
		"feat(list):  add wt list":            "leading or trailing spaces",
		"feat(list): add wt list [0.1.0]":     "version suffix [0.1.0]",
		"feat(list): add wt list [v0.1]":      "version suffix [v0.1]",
		"feat(list): add wt list [wip]":       "version suffix [wip]",
		"docs(x): " + strings.Repeat("a", 64): "the limit is 72",
	}
	for in, want := range cases {
		_, err := ParseTitle(in)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseTitle(%q) = %v, want error containing %q", in, err, want)
		}
	}
}

func TestPublishedVersions(t *testing.T) {
	subjects := []string{
		"feat(shell-integration): add wt shell init and wt cd [v0.1.0-alpha.2] (#6)",
		"chore(ci): bump actions/checkout from 4 to 7 (#5)",
		"feat(walking-skeleton): add wt list, wt config and CLI contract [v0.1.0-alpha.1] (#3)",
		"v1: reescritura de cero en Go, multiplataforma (M0 bootstrap) (#1)",
		"feat(console): improve Clear-WtConsoleScreen function",
		"",
	}
	got, err := PublishedVersions(subjects)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].String() != "v0.1.0-alpha.2" || got[1].String() != "v0.1.0-alpha.1" {
		t.Errorf("PublishedVersions = %v", got)
	}

	if _, err := PublishedVersions([]string{"feat(x): y [v0.1] (#9)"}); err == nil {
		t.Error("a malformed suffix on main must be an error, not be skipped")
	}
}
```

- [ ] **Step 2: Correr los tests y verificar que fallan**

Run: `go test ./tools/relcheck/`
Expected: FAIL de compilación con `undefined: ParseTitle`.

- [ ] **Step 3: Implementar**

`tools/relcheck/title.go`:

```go
package main

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// titleTypes are the Conventional Commits types a title may use.
var titleTypes = map[string]bool{
	"feat": true, "fix": true, "docs": true, "chore": true,
	"ci": true, "refactor": true, "test": true, "perf": true,
}

// maxTitleLen caps the title without its version suffix.
const maxTitleLen = 72

// Title is a PR title: <type>(<scope>)[!]: <summary> [<version>].
type Title struct {
	Type, Scope, Summary string
	Breaking             bool
	Version              *Version // nil when the title has no version suffix
}

var (
	titleRE    = regexp.MustCompile(`^([a-z]+)\(([^()]*)\)(!?): (.*)$`)
	suffixRE   = regexp.MustCompile(` \[([^\[\]]*)\]$`)
	prNumberRE = regexp.MustCompile(` \(#[0-9]+\)$`)
	kebabRE    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

// ParseTitle parses a PR title and checks its format. A trailing " [...]" is
// always read as the version suffix, so a malformed version is an error and
// never silently becomes part of the summary.
func ParseTitle(s string) (Title, error) {
	var t Title
	head := s
	if m := suffixRE.FindStringSubmatchIndex(s); m != nil {
		raw := s[m[2]:m[3]]
		v, err := ParseVersion(raw)
		if err != nil {
			return Title{}, fmt.Errorf("version suffix [%s]: %v", raw, err)
		}
		t.Version = &v
		head = s[:m[0]]
	}
	if n := utf8.RuneCountInString(head); n > maxTitleLen {
		return Title{}, fmt.Errorf("title is %d characters without the version suffix; the limit is %d", n, maxTitleLen)
	}
	m := titleRE.FindStringSubmatch(head)
	if m == nil {
		return Title{}, fmt.Errorf("title %q does not match <type>(<scope>)[!]: <summary> [<version>]", s)
	}
	t.Type, t.Scope, t.Breaking, t.Summary = m[1], m[2], m[3] == "!", m[4]
	if !titleTypes[t.Type] {
		return Title{}, fmt.Errorf("type %q is not one of feat, fix, docs, chore, ci, refactor, test, perf", t.Type)
	}
	if !kebabRE.MatchString(t.Scope) {
		return Title{}, fmt.Errorf("scope %q must be non-empty kebab-case", t.Scope)
	}
	switch first, _ := utf8.DecodeRuneInString(t.Summary); {
	case t.Summary == "":
		return Title{}, fmt.Errorf("summary is empty")
	case strings.TrimSpace(t.Summary) != t.Summary:
		return Title{}, fmt.Errorf("summary %q has leading or trailing spaces", t.Summary)
	case unicode.IsUpper(first):
		return Title{}, fmt.Errorf("summary %q must start in lowercase", t.Summary)
	case strings.HasSuffix(t.Summary, "."):
		return Title{}, fmt.Errorf("summary %q must not end with a period", t.Summary)
	}
	return t, nil
}

// SubjectVersion returns the version a squash subject on main published,
// "<title> [vX.Y.Z] (#N)". ok is false when the subject has no suffix.
func SubjectVersion(subject string) (v Version, ok bool, err error) {
	m := suffixRE.FindStringSubmatch(prNumberRE.ReplaceAllString(subject, ""))
	if m == nil {
		return Version{}, false, nil
	}
	v, err = ParseVersion(m[1])
	if err != nil {
		return Version{}, false, fmt.Errorf("subject %q: %v", subject, err)
	}
	return v, true, nil
}

// PublishedVersions collects the versions the given subjects of main
// published. Subjects without a version suffix are skipped.
func PublishedVersions(subjects []string) ([]Version, error) {
	var vs []Version
	for _, s := range subjects {
		v, ok, err := SubjectVersion(s)
		if err != nil {
			return nil, err
		}
		if ok {
			vs = append(vs, v)
		}
	}
	return vs, nil
}
```

- [ ] **Step 4: Correr los tests y verificar que pasan**

Run: `go test ./tools/relcheck/ && go vet ./... && gofmt -l .`
Expected: `ok`, sin salida de vet ni de gofmt.

- [ ] **Step 5: Verificar que la historia real de `main` no da falsos positivos**

Run: `git log origin/main --format=%s | grep -c '\]$'`
Expected: `0`. Ningún subject histórico termina en `]`, así que `PublishedVersions` sobre `main` hoy devuelve vacío y `v0.1.0-alpha.1` es la primera.

- [ ] **Step 6: Commit**

```bash
git add tools/relcheck/title.go tools/relcheck/title_test.go
git commit -m "feat(relcheck): parse PR titles and published versions" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 6: relcheck, ramas

**Files:**
- Create: `tools/relcheck/branch.go`
- Test: `tools/relcheck/branch_test.go`

**Interfaces:**
- Consumes: `Version`, `Minor`, `ParseVersion` (Task 4).
- Produces:
  - `type BranchKind int` con `ChangeBranch`, `FixBranch`, `ReleaseBranch`, `PlainBranch`, `DependabotBranch`
  - `type Branch struct{ Kind BranchKind; Prefix string; Minor Minor; Change string; Release Version }`
  - `func (Branch) Publishes() bool`
  - `func ParseBranch(s string) (Branch, error)`
  - `func ArchivedAs(dir, change string) bool`

- [ ] **Step 1: Escribir los tests**

`tools/relcheck/branch_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestParseBranch(t *testing.T) {
	cases := map[string]Branch{
		"v0.1/walking-skeleton":                        {Kind: ChangeBranch, Minor: Minor{0, 1}, Change: "walking-skeleton"},
		"v1.0/doctor":                                  {Kind: ChangeBranch, Minor: Minor{1, 0}, Change: "doctor"},
		"fix/list-sort-order":                          {Kind: FixBranch},
		"release/v0.1.0":                               {Kind: ReleaseBranch, Release: Version{Minor: 1}},
		"chore/dependabot-config":                      {Kind: PlainBranch, Prefix: "chore"},
		"ci/release-conventions":                       {Kind: PlainBranch, Prefix: "ci"},
		"docs/contributing":                            {Kind: PlainBranch, Prefix: "docs"},
		"refactor/table-writer":                        {Kind: PlainBranch, Prefix: "refactor"},
		"test/e2e-windows":                             {Kind: PlainBranch, Prefix: "test"},
		"dependabot/github_actions/actions/checkout-7": {Kind: DependabotBranch},
	}
	for in, want := range cases {
		got, err := ParseBranch(in)
		if err != nil || got != want {
			t.Errorf("ParseBranch(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
}

func TestParseBranchErrors(t *testing.T) {
	for _, in := range []string{
		"main",
		"v1/walking-skeleton",    // no minor
		"v0.1.x",                 // maintenance line, never a PR head to main
		"v0.1/Walking_Skeleton",  // not kebab-case
		"v0.1/walking-skeleton/", // trailing slash
		"feat/shell-integration", // features go through vX.Y/<change>
		"fix/",                   // empty slug
		"release/v0.1.0-alpha.1", // a release branch publishes a final
		"release/v0.1.1",         // patches come from vX.Y.x
		"release/0.1.0",
		"powershell/v0.9.x",
	} {
		if _, err := ParseBranch(in); err == nil {
			t.Errorf("ParseBranch(%q) succeeded, want error", in)
		}
	}
	_, err := ParseBranch("feature/x")
	if err == nil || !strings.Contains(err.Error(), "CONTRIBUTING.md") {
		t.Errorf("error should point to CONTRIBUTING.md: %v", err)
	}
}

func TestArchivedAs(t *testing.T) {
	cases := []struct {
		dir, change string
		want        bool
	}{
		{"2026-09-30-walking-skeleton", "walking-skeleton", true},
		{"2026-09-30-shell-integration", "integration", false},
		{"2026-09-30-shell-integration", "shell", false},
		{"walking-skeleton", "walking-skeleton", false},
		{"2026-9-30-walking-skeleton", "walking-skeleton", false},
	}
	for _, c := range cases {
		if got := ArchivedAs(c.dir, c.change); got != c.want {
			t.Errorf("ArchivedAs(%q, %q) = %v, want %v", c.dir, c.change, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Correr los tests y verificar que fallan**

Run: `go test ./tools/relcheck/`
Expected: FAIL de compilación con `undefined: ParseBranch`.

- [ ] **Step 3: Implementar**

`tools/relcheck/branch.go`:

```go
package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// BranchKind is what a branch is for, which decides whether it publishes.
type BranchKind int

const (
	ChangeBranch     BranchKind = iota // vX.Y/<change>: publishes an alpha
	FixBranch                          // fix/<slug>: publishes an alpha
	ReleaseBranch                      // release/vX.Y.0: publishes a final
	PlainBranch                        // chore|docs|ci|refactor|test/<slug>: publishes nothing
	DependabotBranch                   // dependabot/**: publishes nothing
)

// Branch is a parsed head branch name.
type Branch struct {
	Kind    BranchKind
	Prefix  string  // PlainBranch: chore, docs, ci, refactor or test
	Minor   Minor   // ChangeBranch: the minor of the change's milestone
	Change  string  // ChangeBranch: the OpenSpec change name
	Release Version // ReleaseBranch: the final it publishes
}

// Publishes reports whether merging the branch publishes a version.
func (b Branch) Publishes() bool {
	return b.Kind == ChangeBranch || b.Kind == FixBranch || b.Kind == ReleaseBranch
}

var (
	changeBranchRE  = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)/([a-z0-9]+(?:-[a-z0-9]+)*)$`)
	slugBranchRE    = regexp.MustCompile(`^(fix|chore|docs|ci|refactor|test)/([a-z0-9]+(?:-[a-z0-9]+)*)$`)
	releaseBranchRE = regexp.MustCompile(`^release/(.*)$`)
	archivedRE      = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}-(.+)$`)
)

// ParseBranch classifies a head branch name.
func ParseBranch(s string) (Branch, error) {
	if m := changeBranchRE.FindStringSubmatch(s); m != nil {
		major, _ := strconv.Atoi(m[1])
		minor, _ := strconv.Atoi(m[2])
		return Branch{Kind: ChangeBranch, Minor: Minor{major, minor}, Change: m[3]}, nil
	}
	if m := slugBranchRE.FindStringSubmatch(s); m != nil {
		if m[1] == "fix" {
			return Branch{Kind: FixBranch}, nil
		}
		return Branch{Kind: PlainBranch, Prefix: m[1]}, nil
	}
	if m := releaseBranchRE.FindStringSubmatch(s); m != nil {
		v, err := ParseVersion(m[1])
		if err != nil || v.IsAlpha() || v.Patch != 0 {
			return Branch{}, fmt.Errorf("branch %q: a release branch is release/vX.Y.0", s)
		}
		return Branch{Kind: ReleaseBranch, Release: v}, nil
	}
	if strings.HasPrefix(s, "dependabot/") {
		return Branch{Kind: DependabotBranch}, nil
	}
	return Branch{}, fmt.Errorf("branch %q matches no allowed pattern: vX.Y/<change>, fix/<slug>, release/vX.Y.0, chore|docs|ci|refactor|test/<slug> (see CONTRIBUTING.md)", s)
}

// ArchivedAs reports whether dir, a directory name under
// openspec/changes/archive, is the archive of change. OpenSpec names archives
// YYYY-MM-DD-<change>, so "2026-09-30-shell-integration" archives
// "shell-integration" and not "integration".
func ArchivedAs(dir, change string) bool {
	m := archivedRE.FindStringSubmatch(dir)
	return m != nil && m[1] == change
}
```

- [ ] **Step 4: Correr los tests y verificar que pasan**

Run: `go test ./tools/relcheck/ && go vet ./... && gofmt -l .`
Expected: `ok`, sin salida de vet ni de gofmt.

- [ ] **Step 5: Commit**

```bash
git add tools/relcheck/branch.go tools/relcheck/branch_test.go
git commit -m "feat(relcheck): classify branch names" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 7: relcheck, reglas del PR y sección del CHANGELOG

**Files:**
- Create: `tools/relcheck/changelog.go`, `tools/relcheck/check.go`
- Test: `tools/relcheck/changelog_test.go`, `tools/relcheck/check_test.go`

**Interfaces:**
- Consumes: `Version`, `NextAlpha`, `NextCheck` (Task 4); `Title`, `ParseTitle` (Task 5); `Branch`, `ParseBranch` (Task 6); `mustVersions` (helper de test de Task 4).
- Produces:
  - `func ChangelogSection(content string, v Version) (body string, ok bool)`
  - `type Facts struct{ Published []Version; ChangeExists bool; ChangelogSection bool }`
  - `func CheckPR(b Branch, t Title, f Facts) (*Version, []error)`

- [ ] **Step 1: Escribir los tests**

`tools/relcheck/changelog_test.go`:

```go
package main

import "testing"

const sampleChangelog = `# Changelog

## [Unreleased]

### Added

- Something new.

## [0.1.0] — 2026-10-15

### Added

- ` + "`wt list`" + `.

### Fixed

- A bug.

---

## [0.1.01] — not a real version

## PowerShell 0.9.0 — 2026-09-21

Last PowerShell release.
`

func TestChangelogSection(t *testing.T) {
	body, ok := ChangelogSection(sampleChangelog, Version{Minor: 1})
	want := "### Added\n\n- `wt list`.\n\n### Fixed\n\n- A bug."
	if !ok || body != want {
		t.Errorf("ChangelogSection(0.1.0) = %q, %v; want %q", body, ok, want)
	}

	// CRLF checkouts (Windows without .gitattributes) read the same.
	crlf := ""
	for _, r := range sampleChangelog {
		if r == '\n' {
			crlf += "\r\n"
		} else {
			crlf += string(r)
		}
	}
	if body, ok := ChangelogSection(crlf, Version{Minor: 1}); !ok || body != want {
		t.Errorf("CRLF: got %q, %v", body, ok)
	}

	for _, v := range []Version{{Minor: 2}, {Minor: 1, Alpha: 1}, {Minor: 9}} {
		if _, ok := ChangelogSection(sampleChangelog, v); ok {
			t.Errorf("ChangelogSection(%s) found a section that does not exist", v)
		}
	}
}
```

`tools/relcheck/check_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func checkPR(t *testing.T, branch, title string, f Facts) (*Version, []error) {
	t.Helper()
	b, err := ParseBranch(branch)
	if err != nil {
		t.Fatal(err)
	}
	ti, err := ParseTitle(title)
	if err != nil {
		t.Fatal(err)
	}
	return CheckPR(b, ti, f)
}

func TestCheckPRValid(t *testing.T) {
	alpha1 := mustVersions(t, "v0.1.0-alpha.1")
	cases := []struct {
		branch, title string
		facts         Facts
		publishes     string // empty means nothing
	}{
		{"v0.1/walking-skeleton", "feat(walking-skeleton): add wt list [v0.1.0-alpha.1]",
			Facts{ChangeExists: true}, "v0.1.0-alpha.1"},
		{"fix/list-sort-order", "fix(list): sort names case-insensitively [v0.1.0-alpha.2]",
			Facts{Published: alpha1}, "v0.1.0-alpha.2"},
		{"release/v0.1.0", "chore(release): close M1 [v0.1.0]",
			Facts{Published: alpha1, ChangelogSection: true}, "v0.1.0"},
		{"ci/release-conventions", "ci(release): add branch, title and release conventions",
			Facts{}, ""},
		{"dependabot/go_modules/golang.org/x/sys-0.49.0", "chore(deps): bump golang.org/x/sys from 0.48.0 to 0.49.0",
			Facts{Published: alpha1}, ""},
	}
	for _, c := range cases {
		v, errs := checkPR(t, c.branch, c.title, c.facts)
		if len(errs) > 0 {
			t.Errorf("%s / %s: unexpected errors %v", c.branch, c.title, errs)
			continue
		}
		got := ""
		if v != nil {
			got = v.String()
		}
		if got != c.publishes {
			t.Errorf("%s: publishes %q, want %q", c.branch, got, c.publishes)
		}
	}
}

func TestCheckPRInvalid(t *testing.T) {
	alpha1 := mustVersions(t, "v0.1.0-alpha.1")
	cases := []struct {
		branch, title string
		facts         Facts
		want          string
	}{
		{"v0.1/walking-skeleton", "feat(list): add wt list [v0.1.0-alpha.1]",
			Facts{ChangeExists: true}, `must be the change name "walking-skeleton"`},
		{"v0.1/walking-skeleton", "feat(walking-skeleton): add wt list [v0.1.0-alpha.1]",
			Facts{}, "does not exist, archived or not"},
		{"v0.1/walking-skeleton", "feat(walking-skeleton): add wt list",
			Facts{ChangeExists: true}, `end the title with " [v0.1.0-alpha.1]"`},
		{"v0.1/shell-integration", "feat(shell-integration): add wt cd [v0.1.0-alpha.2]",
			Facts{ChangeExists: true}, "next alpha of 0.1 is v0.1.0-alpha.1"},
		{"v0.2/workspace-discovery", "feat(workspace-discovery): find repos [v0.2.0-alpha.1]",
			Facts{ChangeExists: true, Published: alpha1}, "open minor is 0.1"},
		{"v0.1/walking-skeleton", "feat(walking-skeleton): add wt list [v0.2.0-alpha.1]",
			Facts{ChangeExists: true}, "publishes an alpha of 0.1"},
		{"v0.1/walking-skeleton", "feat(walking-skeleton): add wt list [v0.1.0]",
			Facts{ChangeExists: true, Published: alpha1}, "publishes an alpha of 0.1"},
		// Two PRs took the same alpha; the second must update its title.
		{"v0.1/shell-integration", "feat(shell-integration): add wt cd [v0.1.0-alpha.1]",
			Facts{ChangeExists: true, Published: alpha1}, "already published"},
		{"fix/list-sort-order", "feat(list): sort names [v0.1.0-alpha.2]",
			Facts{Published: alpha1}, "needs type fix"},
		{"fix/list-sort-order", "fix(list): sort names [v0.1.0]",
			Facts{Published: alpha1}, "publishes an alpha, not v0.1.0"},
		{"release/v0.1.0", "chore(release): close M1 [v0.1.0]",
			Facts{Published: alpha1}, `no "## [0.1.0]" section`},
		{"release/v0.1.0", "chore(release): close M1 [v0.1.0]",
			Facts{ChangelogSection: true}, "at least one alpha"},
		{"release/v0.1.0", "feat(release): close M1 [v0.1.0]",
			Facts{Published: alpha1, ChangelogSection: true}, "needs chore(release)"},
		{"ci/release-conventions", "ci(release): add conventions [v0.1.0-alpha.1]",
			Facts{}, "does not publish; remove the version suffix"},
		{"ci/release-conventions", "docs(release): add conventions",
			Facts{}, "needs type ci"},
		{"dependabot/github_actions/actions/checkout-7", "chore(ci): bump actions/checkout [v0.1.0-alpha.2]",
			Facts{Published: alpha1}, "does not publish"},
	}
	for _, c := range cases {
		v, errs := checkPR(t, c.branch, c.title, c.facts)
		if v != nil {
			t.Errorf("%s / %s: publishes %s despite errors", c.branch, c.title, v)
		}
		var msgs []string
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		if all := strings.Join(msgs, "\n"); !strings.Contains(all, c.want) {
			t.Errorf("%s / %s: errors %q, want one containing %q", c.branch, c.title, all, c.want)
		}
	}
}

func TestCheckPRReportsEveryViolation(t *testing.T) {
	_, errs := checkPR(t, "v0.1/walking-skeleton", "docs(list): add wt list", Facts{})
	if len(errs) != 3 { // wrong scope, missing change, missing version
		t.Errorf("got %d errors, want 3: %v", len(errs), errs)
	}
}
```

- [ ] **Step 2: Correr los tests y verificar que fallan**

Run: `go test ./tools/relcheck/`
Expected: FAIL de compilación con `undefined: ChangelogSection` y `undefined: CheckPR`.

- [ ] **Step 3: Implementar**

`tools/relcheck/changelog.go`:

```go
package main

import "strings"

// ChangelogSection returns the body of the "## [X.Y.Z]" section of a Keep a
// Changelog file, without its heading and without a trailing "---" separator.
// ok is false when the section does not exist.
func ChangelogSection(content string, v Version) (body string, ok bool) {
	heading := "## [" + strings.TrimPrefix(v.String(), "v") + "]"
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	start, end := -1, len(lines)
	for i, l := range lines {
		if start < 0 {
			if l == heading || strings.HasPrefix(l, heading+" ") {
				start = i + 1
			}
			continue
		}
		if strings.HasPrefix(l, "## ") {
			end = i
			break
		}
	}
	if start < 0 {
		return "", false
	}
	body = strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	body = strings.TrimSpace(strings.TrimSuffix(body, "---"))
	return body, true
}
```

`tools/relcheck/check.go`:

```go
package main

import (
	"fmt"
	"strings"
)

// Facts are what CheckPR needs from the repository besides branch and title.
type Facts struct {
	Published        []Version // versions already published on main
	ChangeExists     bool      // ChangeBranch: openspec/changes/<change>/ exists, archived or not
	ChangelogSection bool      // ReleaseBranch: CHANGELOG.md has a "## [X.Y.0]" section
}

// CheckPR validates a pull request against ADR 0002. It returns the version
// the merge publishes (nil if none), or every violation found.
func CheckPR(b Branch, t Title, f Facts) (*Version, []error) {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	switch b.Kind {
	case ChangeBranch:
		if t.Scope != b.Change {
			fail("scope %q must be the change name %q", t.Scope, b.Change)
		}
		if !f.ChangeExists {
			fail("openspec/changes/%s/ does not exist, archived or not", b.Change)
		}
	case FixBranch:
		if t.Type != "fix" {
			fail("a fix/ branch needs type fix, not %q", t.Type)
		}
	case ReleaseBranch:
		if t.Type != "chore" || t.Scope != "release" {
			fail("a release/ branch needs chore(release), not %s(%s)", t.Type, t.Scope)
		}
		if !f.ChangelogSection {
			fail("CHANGELOG.md has no \"## [%s]\" section", strings.TrimPrefix(b.Release.String(), "v"))
		}
	case PlainBranch:
		if t.Type != b.Prefix {
			fail("a %s/ branch needs type %s, not %q", b.Prefix, b.Prefix, t.Type)
		}
	}

	switch {
	case !b.Publishes() && t.Version != nil:
		fail("this branch does not publish; remove the version suffix [%s]", t.Version)
	case b.Publishes() && t.Version == nil:
		fail("this branch publishes; end the title with \" [%s]\"", suggestion(b, f.Published))
	case b.Publishes():
		v := *t.Version
		switch b.Kind {
		case ChangeBranch:
			if !v.IsAlpha() || v.MinorOf() != b.Minor {
				fail("a v%s/ branch publishes an alpha of %s, not %s", b.Minor, b.Minor, v)
			}
		case FixBranch:
			if !v.IsAlpha() {
				fail("a fix/ branch publishes an alpha, not %s", v)
			}
		case ReleaseBranch:
			if v != b.Release {
				fail("release/%s publishes %s, not %s", b.Release, b.Release, v)
			}
		}
		if err := NextCheck(v, f.Published); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return t.Version, nil
}

// suggestion is the version a publishing branch should put in its title.
func suggestion(b Branch, published []Version) string {
	if b.Kind == ReleaseBranch {
		return b.Release.String()
	}
	if v, err := NextAlpha(published); err == nil {
		return v.String()
	}
	return "vX.Y.0-alpha.N"
}
```

- [ ] **Step 4: Correr los tests y verificar que pasan**

Run: `go test ./tools/relcheck/ && go vet ./... && gofmt -l .`
Expected: `ok`, sin salida de vet ni de gofmt.

- [ ] **Step 5: Commit**

```bash
git add tools/relcheck/changelog.go tools/relcheck/changelog_test.go tools/relcheck/check.go tools/relcheck/check_test.go
git commit -m "feat(relcheck): check pull requests and read changelog sections" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 8: relcheck, CLI

**Files:**
- Modify: `tools/relcheck/main.go` (se reemplaza entero)
- Test: `tools/relcheck/main_test.go`

**Interfaces:**
- Consumes: todo lo de Tasks 4 a 7.
- Produces la CLI que usan los workflows (Task 10):
  - `relcheck pr --branch B --title T [--base origin/main] [--root .]`: exit 0 e imprime `ok: publishes vX` o `ok: publishes nothing`; exit 1 con una línea `relcheck: <error>` por violación.
  - `relcheck merge [--rev HEAD] [--root .]`: imprime la versión, o nada si el commit no publica; exit 1 si la versión no es la siguiente, si es un final sin sección en el CHANGELOG o si el tag existe en otro commit.
  - `relcheck notes --version vX.Y.0 [--root .]`: imprime la sección del CHANGELOG.
  - Exit 2 ante un uso inválido.

- [ ] **Step 1: Escribir los tests de integración contra repos git reales**

`tools/relcheck/main_test.go`:

```go
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitT runs git in dir for a test, ignoring the user's signing config.
func gitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=relcheck", "GIT_AUTHOR_EMAIL=relcheck@example.com",
		"GIT_COMMITTER_NAME=relcheck", "GIT_COMMITTER_EMAIL=relcheck@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newRepo creates a real git repository whose main branch has one empty
// commit per subject, oldest first.
func newRepo(t *testing.T, subjects ...string) string {
	t.Helper()
	dir := t.TempDir()
	gitT(t, dir, "init", "-q", "-b", "main")
	for _, s := range append([]string{"initial commit"}, subjects...) {
		gitT(t, dir, "commit", "-q", "--no-verify", "--allow-empty", "-m", s)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runT(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRunUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"publish"}} {
		if code, _, stderr := runT(args...); code != 2 || !strings.Contains(stderr, "usage:") {
			t.Errorf("run(%v) = %d, %q; want 2 and usage", args, code, stderr)
		}
	}
}

func TestRunPR(t *testing.T) {
	dir := newRepo(t, "chore(ci): add dependabot config (#2)")
	writeFile(t, filepath.Join(dir, "openspec", "changes", "archive", "2026-09-30-walking-skeleton", "proposal.md"), "x")

	code, stdout, stderr := runT("pr", "--root", dir, "--base", "main",
		"--branch", "v0.1/walking-skeleton",
		"--title", "feat(walking-skeleton): add wt list, wt config and CLI contract [v0.1.0-alpha.1]")
	if code != 0 || stdout != "ok: publishes v0.1.0-alpha.1\n" {
		t.Errorf("got %d, %q, %q", code, stdout, stderr)
	}

	code, stdout, _ = runT("pr", "--root", dir, "--base", "main",
		"--branch", "ci/release-conventions",
		"--title", "ci(release): add branch, title and release conventions")
	if code != 0 || stdout != "ok: publishes nothing\n" {
		t.Errorf("got %d, %q", code, stdout)
	}
}

func TestRunPRReadsPublishedVersionsFromBase(t *testing.T) {
	dir := newRepo(t, "feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)")
	writeFile(t, filepath.Join(dir, "openspec", "changes", "shell-integration", "proposal.md"), "x")

	code, _, stderr := runT("pr", "--root", dir, "--base", "main",
		"--branch", "v0.1/shell-integration",
		"--title", "feat(shell-integration): add wt cd [v0.1.0-alpha.1]")
	if code != 1 || !strings.Contains(stderr, "relcheck: v0.1.0-alpha.1 is already published") {
		t.Errorf("got %d, %q", code, stderr)
	}
}

func TestRunPRReportsBranchAndTitleTogether(t *testing.T) {
	dir := newRepo(t)
	code, _, stderr := runT("pr", "--root", dir, "--base", "main", "--branch", "feature/x", "--title", "Add stuff")
	if code != 1 || strings.Count(stderr, "relcheck: ") != 2 {
		t.Errorf("got %d, %q; want both errors, one per line", code, stderr)
	}
}

func TestRunPRRelease(t *testing.T) {
	dir := newRepo(t, "feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)")
	args := []string{"pr", "--root", dir, "--base", "main", "--branch", "release/v0.1.0", "--title", "chore(release): close M1 [v0.1.0]"}

	if code, _, stderr := runT(args...); code != 1 || !strings.Contains(stderr, `no "## [0.1.0]" section`) {
		t.Errorf("without changelog: got %d, %q", code, stderr)
	}
	writeFile(t, filepath.Join(dir, "CHANGELOG.md"), "# Changelog\n\n## [0.1.0] — 2026-10-15\n\n- `wt list`.\n")
	if code, stdout, stderr := runT(args...); code != 0 || stdout != "ok: publishes v0.1.0\n" {
		t.Errorf("with changelog: got %d, %q, %q", code, stdout, stderr)
	}
}

func TestRunMerge(t *testing.T) {
	cases := []struct {
		name     string
		subjects []string
		code     int
		stdout   string
		stderr   string
	}{
		{"publishes nothing", []string{"chore(ci): bump actions/checkout from 4 to 7 (#5)"}, 0, "", ""},
		{"first alpha", []string{"feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)"}, 0, "v0.1.0-alpha.1\n", ""},
		{"second alpha", []string{
			"feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)",
			"chore(ci): bump actions/checkout from 4 to 7 (#5)",
			"feat(shell-integration): add wt cd [v0.1.0-alpha.2] (#6)",
		}, 0, "v0.1.0-alpha.2\n", ""},
		{"duplicate", []string{
			"feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)",
			"feat(shell-integration): add wt cd [v0.1.0-alpha.1] (#6)",
		}, 1, "", "already published"},
		{"final without changelog", []string{
			"feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)",
			"chore(release): close M1 [v0.1.0] (#9)",
		}, 1, "", `no "## [0.1.0]" section`},
	}
	for _, c := range cases {
		dir := newRepo(t, c.subjects...)
		code, stdout, stderr := runT("merge", "--root", dir)
		if code != c.code || stdout != c.stdout || !strings.Contains(stderr, c.stderr) {
			t.Errorf("%s: got %d, %q, %q", c.name, code, stdout, stderr)
		}
	}
}

func TestRunMergeIsIdempotentAfterTagging(t *testing.T) {
	dir := newRepo(t, "feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)")
	gitT(t, dir, "tag", "-a", "v0.1.0-alpha.1", "-m", "v0.1.0-alpha.1")

	// Re-running release.yml after GoReleaser failed must not fail on the tag
	// the first run already pushed.
	if code, stdout, stderr := runT("merge", "--root", dir); code != 0 || stdout != "v0.1.0-alpha.1\n" {
		t.Errorf("got %d, %q, %q", code, stdout, stderr)
	}
}

func TestRunMergeRejectsTagOnAnotherCommit(t *testing.T) {
	dir := newRepo(t, "chore(ci): something (#4)", "feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)")
	gitT(t, dir, "tag", "v0.1.0-alpha.1", "HEAD~1")

	if code, _, stderr := runT("merge", "--root", dir); code != 1 || !strings.Contains(stderr, "already exists on another commit") {
		t.Errorf("got %d, %q", code, stderr)
	}
}

func TestRunNotes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CHANGELOG.md"), "# Changelog\n\n## [Unreleased]\n\n## [0.1.0] — 2026-10-15\n\n### Added\n\n- `wt list`.\n\n---\n\n## PowerShell 0.9.0\n")

	code, stdout, stderr := runT("notes", "--root", dir, "--version", "v0.1.0")
	if code != 0 || stdout != "### Added\n\n- `wt list`.\n" {
		t.Errorf("got %d, %q, %q", code, stdout, stderr)
	}
	if code, _, _ := runT("notes", "--root", dir, "--version", "v0.2.0"); code != 1 {
		t.Errorf("missing section: got %d, want 1", code)
	}
}
```

- [ ] **Step 2: Correr los tests y verificar que fallan**

Run: `go test ./tools/relcheck/ -run 'TestRun'`
Expected: FAIL. `TestRunPR`, `TestRunMerge` y el resto fallan porque el `run` provisorio devuelve 2 para todo. `TestRunUsage` también falla, porque no imprime `usage:`.

- [ ] **Step 3: Implementar**

Reemplazar `tools/relcheck/main.go` entero:

```go
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `usage:
  relcheck pr --branch <head-ref> --title <title> [--base <ref>] [--root <dir>]
      Check a pull request. Prints the version its merge publishes.
  relcheck merge [--rev <rev>] [--root <dir>]
      Read the version a squash commit on main publishes and re-check it.
      Prints the version, or nothing when the commit publishes nothing.
  relcheck notes --version <vX.Y.0> [--root <dir>]
      Print the CHANGELOG.md section of a final release.
`

// run executes a subcommand and returns the process exit code: 0 ok,
// 1 a convention is violated or git failed, 2 usage error.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	commands := map[string]func([]string, io.Writer) error{
		"pr":    runPR,
		"merge": runMerge,
		"notes": runNotes,
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprint(stderr, usage)
		return 2
	}
	if err := cmd(args[1:], stdout); err != nil {
		for _, line := range strings.Split(err.Error(), "\n") {
			fmt.Fprintln(stderr, "relcheck:", line)
		}
		return 1
	}
	return 0
}

func newFlags(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func runPR(args []string, stdout io.Writer) error {
	flags := newFlags("pr")
	branch := flags.String("branch", "", "head branch of the pull request")
	title := flags.String("title", "", "title of the pull request")
	base := flags.String("base", "origin/main", "ref whose history holds the published versions")
	root := flags.String("root", ".", "repository root")
	if err := flags.Parse(args); err != nil {
		return err
	}

	b, berr := ParseBranch(*branch)
	t, terr := ParseTitle(*title)
	if err := errors.Join(berr, terr); err != nil {
		return err
	}
	subjects, err := gitSubjects(*root, *base)
	if err != nil {
		return err
	}
	f := Facts{}
	if f.Published, err = PublishedVersions(subjects); err != nil {
		return err
	}
	switch b.Kind {
	case ChangeBranch:
		if f.ChangeExists, err = changeExists(*root, b.Change); err != nil {
			return err
		}
	case ReleaseBranch:
		content, err := readChangelog(*root)
		if err != nil {
			return err
		}
		_, f.ChangelogSection = ChangelogSection(content, b.Release)
	}

	v, errs := CheckPR(b, t, f)
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if v == nil {
		fmt.Fprintln(stdout, "ok: publishes nothing")
	} else {
		fmt.Fprintln(stdout, "ok: publishes", v)
	}
	return nil
}

func runMerge(args []string, stdout io.Writer) error {
	flags := newFlags("merge")
	rev := flags.String("rev", "HEAD", "the squash commit pushed to main")
	root := flags.String("root", ".", "repository root")
	if err := flags.Parse(args); err != nil {
		return err
	}

	subject, err := git(*root, "log", "-1", "--format=%s", *rev)
	if err != nil {
		return err
	}
	v, ok, err := SubjectVersion(strings.TrimSpace(subject))
	if err != nil || !ok {
		return err // no suffix: the commit publishes nothing
	}
	// Everything main published before this commit. On a re-run after a
	// partial failure the commit itself is excluded, so the check still passes.
	subjects, err := gitSubjects(*root, *rev+"^")
	if err != nil {
		return err
	}
	published, err := PublishedVersions(subjects)
	if err != nil {
		return err
	}
	if err := NextCheck(v, published); err != nil {
		return err
	}
	if !v.IsAlpha() {
		content, err := readChangelog(*root)
		if err != nil {
			return err
		}
		if _, ok := ChangelogSection(content, v); !ok {
			return fmt.Errorf("CHANGELOG.md has no \"## [%s]\" section for the release notes", strings.TrimPrefix(v.String(), "v"))
		}
	}
	// A tag already on this commit means an earlier run got this far.
	if tagged, err := git(*root, "rev-parse", "-q", "--verify", "refs/tags/"+v.String()+"^{commit}"); err == nil {
		commit, err := git(*root, "rev-parse", *rev+"^{commit}")
		if err != nil {
			return err
		}
		if strings.TrimSpace(tagged) != strings.TrimSpace(commit) {
			return fmt.Errorf("tag %s already exists on another commit", v)
		}
	}
	fmt.Fprintln(stdout, v)
	return nil
}

func runNotes(args []string, stdout io.Writer) error {
	flags := newFlags("notes")
	version := flags.String("version", "", "final version, vX.Y.0")
	root := flags.String("root", ".", "repository root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	v, err := ParseVersion(*version)
	if err != nil {
		return err
	}
	content, err := readChangelog(*root)
	if err != nil {
		return err
	}
	body, ok := ChangelogSection(content, v)
	if !ok {
		return fmt.Errorf("CHANGELOG.md has no \"## [%s]\" section", strings.TrimPrefix(v.String(), "v"))
	}
	fmt.Fprintln(stdout, body)
	return nil
}

// readChangelog returns CHANGELOG.md, or "" when the file does not exist.
func readChangelog(root string) (string, error) {
	content, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(content), err
}

// changeExists reports whether openspec/changes/<change>/ or its archive exists.
func changeExists(root, change string) (bool, error) {
	changes := filepath.Join(root, "openspec", "changes")
	if change != "archive" {
		if info, err := os.Stat(filepath.Join(changes, change)); err == nil && info.IsDir() {
			return true, nil
		}
	}
	entries, err := os.ReadDir(filepath.Join(changes, "archive"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() && ArchivedAs(e.Name(), change) {
			return true, nil
		}
	}
	return false, nil
}

func gitSubjects(root, rev string) ([]string, error) {
	out, err := git(root, "log", "--format=%s", rev)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(out, "\n"), "\n"), nil
}

func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}
```

- [ ] **Step 4: Correr toda la suite**

Run: `go test -race -count=1 ./... && go vet ./... && go build ./... && gofmt -l .`
Expected: `ok` en `tools/relcheck`, sin salida de vet ni de gofmt.

- [ ] **Step 5: Probar contra este repo**

Run: `go run ./tools/relcheck pr --branch ci/release-conventions --title "ci(release): add branch, title and release conventions"`
Expected: `ok: publishes nothing`

Run: `go run ./tools/relcheck pr --branch v0.1/walking-skeleton --title "feat(walking-skeleton): add wt list, wt config and CLI contract [v0.1.0-alpha.1]"; echo "exit=$?"`
Expected: `relcheck: openspec/changes/walking-skeleton/ does not exist, archived or not` y `exit=1`. El archive vive en la rama del PR #3, no en esta; en Task 14 sí va a existir.

- [ ] **Step 6: Commit**

```bash
git add tools/relcheck/main.go tools/relcheck/main_test.go
git commit -m "feat(relcheck): add the pr, merge and notes commands" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 9: GoReleaser y CI

**Files:**
- Create: `.goreleaser.yaml`
- Modify: `.github/workflows/ci.yml` (se reemplaza entero)

**Interfaces:**
- Consumes: `cmd/git-wt` con `var version` en `main` (ya existe en `main`).
- Produces: `.goreleaser.yaml`, que usa `release.yml` en Task 10; el job `cross-build` sigue siendo un check requerido con el mismo nombre.

- [ ] **Step 1: Crear la config**

`.goreleaser.yaml`:

```yaml
# yaml-language-server: $schema=https://goreleaser.com/static/schema.json
#
# The single source of the release targets. CI builds a snapshot with this
# file on every PR (job cross-build), and release.yml publishes with it.
# Conventions: docs/decisions/0002-versionado-y-releases.md.
version: 2

project_name: git-wt

builds:
  - id: git-wt
    main: ./cmd/git-wt
    binary: git-wt
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    ldflags:
      - -s -w -X main.version={{ .Version }}
    targets:
      - darwin_arm64
      - darwin_amd64
      - windows_amd64
      - linux_amd64
      - linux_arm64
    mod_timestamp: "{{ .CommitTimestamp }}"

archives:
  - id: git-wt
    formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    files:
      - LICENSE
      - README.md

checksum:
  name_template: checksums.txt

changelog:
  use: git
  sort: asc
  abbrev: -1
  groups:
    - title: Features
      regexp: '^feat(\([^)]*\))?!?:'
      order: 0
    - title: Fixes
      regexp: '^fix(\([^)]*\))?!?:'
      order: 1
    - title: Other
      order: 999

release:
  prerelease: auto
  replace_existing_artifacts: true
```

- [ ] **Step 2: Validar y compilar un snapshot local**

Run: `~/go/bin/goreleaser check && ~/go/bin/goreleaser build --snapshot --clean && ./dist/git-wt_darwin_arm64_v8.0/git-wt version`
Expected: `1 configuration file(s) validated`, `build succeeded`, y una versión que contiene `SNAPSHOT`. `dist/` ya está en `.gitignore`.

- [ ] **Step 3: Reemplazar `.github/workflows/ci.yml`**

```yaml
name: CI

on:
  push:
    branches: [main, "v*/**", "fix/**", "release/**"]
  pull_request:
    branches: [main]

permissions:
  contents: read

env:
  GO_VERSION: "1.27"

jobs:
  # macOS and Windows are both first-class targets (see docs/design/product.md).
  # A change is not done if its tests only run on one OS.
  test:
    name: test (${{ matrix.os }})
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        os: [macos-latest, windows-latest, ubuntu-latest]
    steps:
      - uses: actions/checkout@v7

      - uses: actions/setup-go@v7
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - name: gofmt
        shell: bash
        run: |
          unformatted="$(gofmt -l .)"
          if [ -n "$unformatted" ]; then
            echo "These files are not gofmt'd:"
            echo "$unformatted"
            exit 1
          fi

      - name: go vet
        run: go vet ./...

      - name: build
        run: go build ./...

      # The race detector needs a C toolchain on Windows; skip it there.
      - name: test
        if: matrix.os == 'windows-latest'
        run: go test ./...

      - name: test (race)
        if: matrix.os != 'windows-latest'
        run: go test -race ./...

  # Cross-compilation is a product requirement, not an afterthought: the daily
  # development machine is macOS and the daily use machine is Windows. The
  # targets live in .goreleaser.yaml, so CI builds exactly what a release
  # publishes. The job keeps its name: it is a required check on main.
  cross-build:
    name: cross-build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v7
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - uses: goreleaser/goreleaser-action@v7
        with:
          version: "~> v2"
          args: check

      - uses: goreleaser/goreleaser-action@v7
        with:
          version: "~> v2"
          args: build --snapshot --clean
```

- [ ] **Step 4: Lint del workflow**

Run: `go run github.com/rhysd/actionlint/cmd/actionlint@latest -shellcheck= .github/workflows/ci.yml && echo LINT_OK`
Expected: `LINT_OK`

- [ ] **Step 5: Commit**

```bash
git add .goreleaser.yaml .github/workflows/ci.yml
git commit -m "ci(release): build release targets with goreleaser" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 10: Workflows `pr-conventions` y `release`, y plantilla de PR

**Files:**
- Create: `.github/workflows/pr-conventions.yml`, `.github/workflows/release.yml`, `.github/pull_request_template.md`

**Interfaces:**
- Consumes: la CLI de `relcheck` (Task 8) y `.goreleaser.yaml` (Task 9).
- Produces: el check `pr-conventions` (Task 13 lo vuelve obligatorio) y el workflow `Release` con `workflow_dispatch` (Task 13 corre el dry-run).

- [ ] **Step 1: Crear `.github/workflows/pr-conventions.yml`**

```yaml
name: PR conventions

# Checks the branch name, the pull request title and the version it declares.
# Conventions: docs/decisions/0002-versionado-y-releases.md and CONTRIBUTING.md.

on:
  pull_request:
    branches: [main]
    types: [opened, edited, reopened, synchronize]

permissions:
  contents: read

env:
  GO_VERSION: "1.27"

jobs:
  pr-conventions:
    name: pr-conventions
    runs-on: ubuntu-latest
    steps:
      # Full history: relcheck reads every published version from main's titles.
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v7
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      # Title and branch come from the PR author: they reach the script through
      # the environment and are never interpolated into it.
      - name: relcheck
        env:
          BRANCH: ${{ github.head_ref }}
          TITLE: ${{ github.event.pull_request.title }}
          BASE: origin/${{ github.base_ref }}
        run: go run ./tools/relcheck pr --branch "$BRANCH" --title "$TITLE" --base "$BASE"
```

- [ ] **Step 2: Crear `.github/workflows/release.yml`**

```yaml
name: Release

# Publishes the version a squash commit on main declares at the end of its
# title, "<title> [vX.Y.Z] (#N)". Commits without a version publish nothing.
# Conventions: docs/decisions/0002-versionado-y-releases.md.

on:
  push:
    branches: [main]
  # Dry run: builds the release archives for the chosen ref without tagging or
  # publishing. GitHub only offers it once this file is on main.
  workflow_dispatch:

permissions:
  contents: write

env:
  GO_VERSION: "1.27"

concurrency:
  group: release
  cancel-in-progress: false

jobs:
  release:
    name: release
    runs-on: ubuntu-latest
    steps:
      # Full history and tags: relcheck reads main's titles, GoReleaser finds
      # the previous tag for the notes.
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v7
        with:
          go-version: ${{ env.GO_VERSION }}
          cache: true

      - uses: goreleaser/goreleaser-action@v7
        with:
          install-only: true
          version: "~> v2"

      - name: dry run
        if: github.event_name == 'workflow_dispatch'
        run: goreleaser release --snapshot --clean

      - name: version
        id: version
        if: github.event_name == 'push'
        run: |
          version="$(go run ./tools/relcheck merge --rev HEAD)"
          echo "version=$version" >> "$GITHUB_OUTPUT"
          echo "publishes: ${version:-nothing}"

      # Tag and publish in one job: a tag pushed with GITHUB_TOKEN does not
      # trigger other workflows. Re-running after a failure is safe: the tag is
      # created only if missing, and GoReleaser replaces existing assets.
      - name: tag and publish
        if: steps.version.outputs.version != ''
        env:
          VERSION: ${{ steps.version.outputs.version }}
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          set -euo pipefail
          if ! git rev-parse -q --verify "refs/tags/$VERSION" >/dev/null; then
            git config user.name "github-actions[bot]"
            git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
            git tag -a "$VERSION" -m "$(git log -1 --format=%s HEAD)"
            git push origin "refs/tags/$VERSION"
          fi
          args=(release --clean)
          case "$VERSION" in
            *-alpha.*) ;;
            *)
              go run ./tools/relcheck notes --version "$VERSION" > "$RUNNER_TEMP/notes.md"
              args+=(--release-notes "$RUNNER_TEMP/notes.md")
              ;;
          esac
          goreleaser "${args[@]}"
```

- [ ] **Step 3: Crear `.github/pull_request_template.md`**

```markdown
<!--
Título (lo valida el check pr-conventions; ver CONTRIBUTING.md):
  <type>(<scope>)[!]: <summary> [<version>]

  vX.Y/<change>   feat(<change>): ... [vX.Y.0-alpha.N]
  fix/<slug>      fix(<area>): ... [vX.Y.0-alpha.N]
  release/vX.Y.0  chore(release): close MN [vX.Y.0]
  el resto        sin versión

Si el PR publica, sumá sus líneas bajo [Unreleased] en CHANGELOG.md.
-->

## Resumen

## Qué incluye

## Fuera de alcance

## Plan de pruebas

- [ ] CI verde en macos-latest, windows-latest y ubuntu-latest
- [ ] `go test ./...` en verde en macOS
```

- [ ] **Step 4: Lint de los tres workflows**

Run: `go run github.com/rhysd/actionlint/cmd/actionlint@latest -shellcheck= && echo LINT_OK`
Expected: `LINT_OK`

- [ ] **Step 5: Simular localmente la publicación de una alpha**

Prueba en un clon descartable que el tag y GoReleaser funcionan juntos. No publica nada.

```bash
S=$(mktemp -d) && git clone -q . "$S" && cd "$S"
git commit -q --allow-empty -m "feat(walking-skeleton): add wt list [v0.1.0-alpha.1] (#3)"
version="$(go run ./tools/relcheck merge --rev HEAD)" && echo "$version"
git tag -a "$version" -m test
~/go/bin/goreleaser release --clean --skip=publish,validate
ls dist/*.tar.gz dist/*.zip dist/checksums.txt && head dist/CHANGELOG.md
cd - && rm -rf "$S"
```

Expected: `v0.1.0-alpha.1`, los 5 archivos más `checksums.txt`, y un `CHANGELOG.md` con `### Features` y la línea `feat(walking-skeleton)…`.

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/pr-conventions.yml .github/workflows/release.yml .github/pull_request_template.md
git commit -m "ci(release): add the pr-conventions check and the release workflow" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 11: Documentación y `retract`

**Files:**
- Create: `CONTRIBUTING.md`
- Modify: `CHANGELOG.md` (se reemplaza entero), `README.md`, `go.mod`, `docs/design/product.md`, `openspec/config.yaml`, `docs/decisions/0000-analisis-worktrunk.md`, `docs/decisions/0001-de-cero-en-go.md`

**Interfaces:**
- Produces: `CONTRIBUTING.md`, al que apuntan el error de `ParseBranch` y la plantilla de PR; `## [Unreleased]` en `CHANGELOG.md`, el encabezado que espera el flujo de cierre de milestone.

- [ ] **Step 1: Crear `CONTRIBUTING.md`**

~~~~markdown
# Contributing

This repository follows one convention for branches, pull request titles,
versions and releases. The reasoning, and the alternatives that were
discarded, are in
[docs/decisions/0002-versionado-y-releases.md](docs/decisions/0002-versionado-y-releases.md)
(Spanish). The `pr-conventions` check enforces everything below.

## Versions

SemVer. Each milestone of [the roadmap](docs/design/product.md) ships as one
minor version:

| Milestone | Version |
|---|---|
| M1 | `0.1.0` |
| M2 | `0.2.0` |
| M3 | `0.3.0` |
| M4 | `0.4.0` |
| M5 | `1.0.0` |
| M6 (optional) | `1.1.0` |

- Every change or fix merged to `main` publishes a pre-release of the **open
  minor**: `v0.1.0-alpha.1`, `v0.1.0-alpha.2`, and so on.
- Closing a milestone publishes its final version, such as `v0.1.0`.
- The open minor is the minor of the latest published alpha. After a final,
  it is the next one in the table.
- Milestones do not overlap: a change of M2 cannot merge until `v0.1.0` is
  published.
- Patch releases (`v0.1.1`) come only from a `v0.1.x` maintenance branch,
  created when needed. They are not automated yet.

## Branches

| Branch | For | Publishes |
|---|---|---|
| `vX.Y/<change>` | An OpenSpec change. `<change>` is its directory in `openspec/changes/`; `X.Y` is the minor of its milestone. | alpha |
| `fix/<slug>` | A bug fix outside a change | alpha |
| `release/vX.Y.0` | Closing a milestone | final |
| `chore/`, `docs/`, `ci/`, `refactor/`, `test/` + `<slug>` | Everything else | nothing |
| `dependabot/**` | Dependabot | nothing |
| `vX.Y.x`, `powershell/v0.9.x` | Maintenance lines (protected) | patch |

Change names and slugs are lowercase kebab-case. Work branches are deleted
when their pull request merges.

## Pull request titles

The title becomes the commit on `main`, so it follows
[Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>)[!]: <summary> [<version>]
```

- **type**: `feat`, `fix`, `docs`, `chore`, `ci`, `refactor`, `test` or
  `perf`.
- **scope**: required, kebab-case. On a `vX.Y/<change>` branch it is the
  change name.
- **summary**: imperative, starts in lowercase, no final period. The title
  without the version suffix is at most 72 characters.
- **`!`**: the change breaks the CLI contract (exit codes, `--json` schemas,
  flags).
- **`[<version>]`**: the exact version the merge publishes. The release
  workflow tags whatever this says, so the suffix is required on branches
  that publish and forbidden on the rest.

| Branch | type | scope | Version suffix |
|---|---|---|---|
| `vX.Y/<change>` | any | `<change>` | the next alpha, `vX.Y.0-alpha.N` |
| `fix/<slug>` | `fix` | any | the next alpha, `vX.Y.0-alpha.N` |
| `release/vX.Y.0` | `chore` | `release` | `vX.Y.0` |
| `chore/`, `docs/`, `ci/`, `refactor/`, `test/` | same as the prefix | any | none |
| `dependabot/**` | any | any | none |

Examples:

```
feat(walking-skeleton): add wt list, wt config and CLI contract [v0.1.0-alpha.1]
fix(list): sort branch names case-insensitively [v0.1.0-alpha.3]
chore(release): close M1 [v0.1.0]
chore(ci): bump actions/checkout from 4 to 7
ci(release): add branch, title and release conventions
```

If another pull request publishes first, `main` requires yours to be up to
date: update the branch, and the check tells you the alpha to use instead.

To check a title before pushing:

```sh
go run ./tools/relcheck pr --branch "$(git branch --show-current)" --title "<title>"
```

## Merging

Squash only. The commit on `main` is the title plus ` (#N)`, with no body;
the details stay in the pull request, which follows
[the template](.github/pull_request_template.md). Commits inside a branch are
free-form, since squash discards them; Conventional Commits are recommended.

## Releases

Merging a title with a version runs `.github/workflows/release.yml`. It tags
the squash commit and publishes a GitHub Release with GoReleaser:

- archives for `darwin/arm64`, `darwin/amd64`, `windows/amd64`,
  `linux/amd64` and `linux/arm64`, plus `checksums.txt`, all built with the
  version injected into `wt version`;
- an alpha is marked as a pre-release, with notes grouped from the titles
  merged since the previous version;
- a final is marked as latest, with its `CHANGELOG.md` section as the notes.

The targets live in `.goreleaser.yaml`. The `cross-build` CI job builds them
on every pull request.

### Changelog

Every pull request that publishes adds its lines under `## [Unreleased]` in
`CHANGELOG.md`, following
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

### Closing a milestone

1. Check the milestone's definition of done in `docs/design/product.md` §7.
2. Branch `release/vX.Y.0` from `main`.
3. In `CHANGELOG.md`, rename `## [Unreleased]` to `## [X.Y.0] — YYYY-MM-DD`
   and add a new, empty `## [Unreleased]` above it.
4. Open the pull request titled `chore(release): close MN [vX.Y.0]`.
5. Squash-merge it. The release notes come from the new changelog section.
~~~~

- [ ] **Step 2: Reemplazar `CHANGELOG.md` entero**

Las líneas que el PR #3 modifica (la de "20 capabilities") quedan intactas, para que su merge en Task 14 no choque acá.

~~~~markdown
# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions follow
SemVer with one minor per milestone, and every merged change ships as a
pre-release of it; see [CONTRIBUTING.md](CONTRIBUTING.md).

## [Unreleased]

v1 is a from-scratch rewrite in Go, with macOS and Windows as first-class
platforms. No code is ported from v0.9. See
[docs/decisions/0001-de-cero-en-go.md](docs/decisions/0001-de-cero-en-go.md).
It ships as `0.x` pre-releases until M5 closes with `1.0.0`.

### Added

- Project bootstrap: Go module, package layout, CI matrix on macOS, Windows and
  Linux, plus cross-compilation checks for all release targets.
- [`docs/design/product.md`](docs/design/product.md) — the full product design:
  CLI contract, config schema, the 20 capabilities and the M1–M6 execution order.
- OpenSpec project context and per-artifact rules, including the rule that specs
  stay language-agnostic.
- Release pipeline: every merged change publishes a GitHub pre-release with
  binaries for macOS, Windows and Linux, plus `checksums.txt`.

### Changed

- Nothing works yet. `main` has no usable commands; the stable line is
  `powershell/v0.9.x`.
- The PowerShell line was renamed: the tag `v0.9.0` is now `powershell-v0.9.0`
  and the branch `v0.9.x` is now `powershell/v0.9.x`.

---

## PowerShell 0.9.0 — 2026-09-21

Last PowerShell release. Windows only, PowerShell 5.1+. Frozen: this line
receives no new features.

Its history is preserved on the `powershell/v0.9.x` branch and the
`powershell-v0.9.0` tag. The changelog for that line is at
[docs/historial/CHANGELOG-v0.9.md](docs/historial/CHANGELOG-v0.9.md).
~~~~

- [ ] **Step 3: Editar `README.md`**

Los bloques del mapa de milestones que toca el PR #3 (las líneas de M4 y M6) no se tocan.

1. En el aviso del principio, reemplazar `> git checkout v0.9.x     # or the v0.9.0 tag` por:

```
> git checkout powershell/v0.9.x     # or the powershell-v0.9.0 tag
```

2. En la tabla de Status, reemplazar las dos filas por:

```markdown
| `powershell/v0.9.x` / tag `powershell-v0.9.0` | **Stable.** PowerShell 5.1+, Windows only. Frozen — no new features. |
| `main` | **In development.** Go, macOS + Windows + Linux. Pre-releases on [Releases](https://github.com/jMautone/worktree-manager/releases). See the milestone map below. |
```

3. Inmediatamente después del bloque de código del mapa de milestones, y antes de `## Building`, insertar:

~~~~markdown
Each milestone ships as one minor version, and every merged change as a
pre-release of it: M1 is `0.1.0`, M2 `0.2.0`, M3 `0.3.0`, M4 `0.4.0`, and M5
is `1.0.0`. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Installing

Download the archive for your platform from
[Releases](https://github.com/jMautone/worktree-manager/releases) and put
`git-wt` on your `PATH`. With Go installed, name the version explicitly:

```sh
go install github.com/jMautone/worktree-manager/cmd/git-wt@v0.1.0-alpha.1
```

`@latest` does not work until `1.0.0`: the Go module proxy still serves the
PowerShell `v0.9.0`, which has no Go code.

~~~~

- [ ] **Step 4: Agregar el `retract` a `go.mod`**

Al final de `go.mod`:

```
// v0.9.0 is the PowerShell line, renamed to the tag powershell-v0.9.0. It has
// no Go code, but the module proxy cached it, so @latest resolves to it until
// a higher release exists. The retraction takes effect from v1.0.0 on.
// See docs/decisions/0002-versionado-y-releases.md.
retract v0.9.0
```

Run: `go mod edit -json | jq -c .Retract && go build ./...`
Expected: `[{"Low":"v0.9.0","High":"v0.9.0","Rationale":"..."}]` (el rationale es el comentario) y build sin errores.

- [ ] **Step 5: Agregar la tabla de versiones a `docs/design/product.md` §7**

Insertar entre el párrafo de `### Definición de "listo" por milestone` y el `---` que precede a `## 8. Non-goals de v1`:

```markdown
### Versiones

Cada milestone sale como un minor, y cada change mergeado como una pre-release de ese minor (`v0.1.0-alpha.N`). El detalle está en [`0002-versionado-y-releases.md`](../decisions/0002-versionado-y-releases.md).

| Milestone | Versión |
|---|---|
| M1 | `0.1.0` |
| M2 | `0.2.0` |
| M3 | `0.3.0` |
| M4 | `0.4.0` |
| M5 | `1.0.0` |
| M6 (opcional) | `1.1.0` |
```

- [ ] **Step 6: Actualizar `openspec/config.yaml`**

1. En `context`, reemplazar `esta congelada en el tag \`v0.9.0\` y la rama \`v0.9.x\`; no se` por `esta congelada en el tag \`powershell-v0.9.0\` y la rama \`powershell/v0.9.x\`; no se`, sin acentos, como el resto del bloque.
2. En `rules.proposal`, agregar al final:

```yaml
    - 'En la sección "Milestone", nombrá la rama del change: vX.Y/<nombre-del-change>, donde X.Y es el minor del milestone (tabla en docs/decisions/0002-versionado-y-releases.md).'
```

3. En `rules.tasks`, agregar al final:

```yaml
    - 'La última tarea integra el change: suma sus líneas bajo [Unreleased] en CHANGELOG.md y abre el PR con el título <type>(<change>): <summary> [vX.Y.0-alpha.N] (ver CONTRIBUTING.md).'
```

Run: `python3 -c 'import yaml; yaml.safe_load(open("openspec/config.yaml"))' && echo YAML_OK && openspec validate --specs --strict`
Expected: `YAML_OK` y las specs válidas.

- [ ] **Step 7: Nota al principio de las ADR 0000 y 0001**

En `docs/decisions/0000-analisis-worktrunk.md`, entre el título y el blockquote `> Documento de exploración…`, y en `docs/decisions/0001-de-cero-en-go.md`, después de la línea `- **Antecedente**: …` (dejando una línea en blanco antes), insertar:

```markdown
> **Nota (2026-09-30)**: el tag `v0.9.0` y la rama `v0.9.x` que menciona este documento se renombraron a `powershell-v0.9.0` y `powershell/v0.9.x`. Ver [0002-versionado-y-releases.md](0002-versionado-y-releases.md).
```

- [ ] **Step 8: Verificar que no quedan referencias viejas fuera de lo histórico**

Run: `git grep -n -E 'v0\.9\.0|v0\.9\.x' -- ':!docs/historial' ':!openspec/changes/archive' ':!docs/decisions/0000-*' ':!docs/decisions/0001-*' ':!docs/superpowers'`
Expected: solo menciones que incluyen `powershell-` o `powershell/`, más tres que nombran a propósito la versión vieja: la ADR 0002, el comentario y la línea del `retract` en `go.mod`, y la nota de `@latest` en `README.md`.

- [ ] **Step 9: Commit**

```bash
git add CONTRIBUTING.md CHANGELOG.md README.md go.mod docs/design/product.md openspec/config.yaml docs/decisions/0000-analisis-worktrunk.md docs/decisions/0001-de-cero-en-go.md
git commit -m "docs(release): document versioning, branches and releases" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 12: Sacar el plan, abrir el PR y mergearlo

**Files:**
- Remove from the index: `docs/superpowers/plans/2026-09-30-release-conventions.md`. Queda en disco, sin trackear e ignorado localmente.

**Interfaces:**
- Produces: `main` con todo lo de Tasks 4 a 11, mergeado con el título `ci(release): add branch, title and release conventions (#N)`.

- [ ] **Step 1: Sacar el plan del índice sin borrarlo del disco**

```bash
git rm --cached docs/superpowers/plans/2026-09-30-release-conventions.md
echo 'docs/superpowers/' >> "$(git rev-parse --git-common-dir)/info/exclude"
git commit -m "chore: drop the implementation plan from the branch" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Run: `git diff --stat origin/main...HEAD | grep -c superpowers; test -f docs/superpowers/plans/2026-09-30-release-conventions.md && echo STILL_ON_DISK`
Expected: `0` y `STILL_ON_DISK`.

- [ ] **Step 2: Verificación completa antes de pushear**

Run: `gofmt -l . && go vet ./... && go build ./... && go test -race -count=1 ./...`
Expected: sin salida de gofmt ni de vet; `ok` en todos los paquetes con tests.

- [ ] **Step 3: ⚠️ Push y PR, con confirmación del usuario**

```bash
git push -u origin ci/release-conventions
gh pr create --base main --head ci/release-conventions \
  --title "ci(release): add branch, title and release conventions" \
  --body-file - <<'EOF'
## Resumen

Implementa la ADR 0002: convención de ramas, títulos, versiones y releases. Desde este merge, cada PR a `main` se valida con el check `pr-conventions`, y cada merge cuyo título termina en `[vX.Y.Z…]` publica ese tag y un GitHub Release con binarios. Este PR no publica nada: es `ci/`.

## Qué incluye

- `docs/decisions/0002-versionado-y-releases.md`: la decisión y las alternativas descartadas.
- `tools/relcheck/`: las reglas en Go, con tests contra repos git reales.
- Workflows: `pr-conventions.yml` (check), `release.yml` (tag, GoReleaser y Release; dry-run por `workflow_dispatch`) y `ci.yml` (`cross-build` con GoReleaser; triggers `v*/**`, `fix/**`, `release/**`).
- `.goreleaser.yaml`: 5 targets, `checksums.txt`, alphas como pre-release y finales con las notas del CHANGELOG.
- `CONTRIBUTING.md`, plantilla de PR, tabla de versiones en `product.md` §7, reglas nuevas en `openspec/config.yaml`.
- `retract v0.9.0` en `go.mod`, que se activa con `v1.0.0`.
- README y CHANGELOG con la línea PowerShell renombrada (`powershell-v0.9.0`, `powershell/v0.9.x`).

## Fuera de alcance

- Activar `pr-conventions` como check obligatorio: va después del merge, porque exigir un check que todavía no existe en `main` bloquea todos los PR.
- Parches desde ramas `vX.Y.x`: no están automatizados hasta que haga falta el primero.
- brew, scoop y winget: M6.

## Plan de pruebas

- [ ] CI verde en macos-latest, windows-latest y ubuntu-latest, incluido `tools/relcheck`
- [ ] `cross-build` verde con `goreleaser check` y `build --snapshot`
- [ ] `pr-conventions` verde sobre este mismo PR (`ok: publishes nothing`)
- [ ] Release simulado en un clon local: 5 archivos, `checksums.txt` y notas agrupadas
- [ ] actionlint sin errores en los 3 workflows
EOF
```

- [ ] **Step 4: Esperar los checks**

Run: `gh pr checks --watch`
Expected: `pr-conventions`, `test (macos-latest)`, `test (windows-latest)`, `test (ubuntu-latest)` y `cross-build` en `pass`. En el log de `pr-conventions` aparece `ok: publishes nothing`.

- [ ] **Step 5: ⚠️ Squash merge, con confirmación del usuario**

```bash
gh pr merge --squash
git fetch origin --prune
git log origin/main -1 --format=%s%n%b
```

Expected: `ci(release): add branch, title and release conventions (#N)` y nada debajo, porque el cuerpo es `BLANK`. La rama remota se borró sola (`git ls-remote --heads origin ci/release-conventions` vacío).

---

## Fase 3 — Activar el candado

### Task 13: Verificar `release.yml` y volver obligatorio `pr-conventions`

**Files:** ninguno en el repo; cambia el ruleset `main`.

**Interfaces:**
- Consumes: `release.yml` y `pr-conventions` ya en `main` (Task 12).
- Produces: `main` exige `pr-conventions` y la rama al día.

- [ ] **Step 1: La corrida de `Release` del merge no publicó nada**

```bash
run=$(gh run list --workflow release.yml --branch main --limit 1 --json databaseId --jq '.[0].databaseId')
gh run watch "$run" --exit-status
gh run view "$run" --log | grep 'publishes:'
```

Expected: la corrida termina bien, el log dice `publishes: nothing`, y `gh release list` sigue vacío.

- [ ] **Step 2: Dry-run por `workflow_dispatch`**

```bash
gh workflow run release.yml --ref main
run=$(gh run list --workflow release.yml --event workflow_dispatch --limit 1 --json databaseId --jq '.[0].databaseId')
gh run watch "$run" --exit-status
```

GitHub tarda unos segundos en registrar la corrida: si `run` sale vacío, volver a correr el `gh run list` (o esperar con Monitor; no usar `sleep` en primer plano).

Expected: termina bien, el log del paso `dry run` muestra los 5 archivos y `checksums.txt`, y no se creó ningún tag (`git ls-remote --tags origin 'v*'` vacío).

- [ ] **Step 3: ⚠️ Volver obligatorio el check, con confirmación del usuario**

```bash
R=repos/jMautone/worktree-manager/rulesets
id=$(gh api "$R" --jq '.[] | select(.name=="main") | .id')
gh api "$R/$id" | jq '{name, target, enforcement, bypass_actors, conditions, rules}
  | (.rules[] | select(.type=="required_status_checks") | .parameters) |=
      (.strict_required_status_checks_policy = true
       | .required_status_checks += [{"context": "pr-conventions", "integration_id": 15368}])' \
  | gh api -X PUT "$R/$id" --input - >/dev/null
```

- [ ] **Step 4: Verificar**

```bash
R=repos/jMautone/worktree-manager/rulesets
id=$(gh api "$R" --jq '.[] | select(.name=="main") | .id')
gh api "$R/$id" --jq '.rules[] | select(.type=="required_status_checks") | .parameters | {strict: .strict_required_status_checks_policy, checks: [.required_status_checks[].context]}'
```

Expected: `{"strict":true,"checks":["test (ubuntu-latest)","test (macos-latest)","test (windows-latest)","cross-build","pr-conventions"]}`

---

## Fase 4 — PR #3 → `v0.1.0-alpha.1`

### Task 14: Integrar walking-skeleton como primera alpha

**Files:**
- Modify (en la rama `v0.1/walking-skeleton`): `CHANGELOG.md`, y lo que resuelva el merge de `main`.

**Interfaces:**
- Consumes: todo lo anterior.
- Produces: tag `v0.1.0-alpha.1` y su GitHub pre-release con 5 archivos más `checksums.txt`.

- [ ] **Step 1: Actualizar la rama con `main` (merge, sin force-push)**

```bash
git switch v0.1/walking-skeleton
git pull --ff-only
git merge origin/main
```

Conflictos esperables y cómo resolverlos:
- `go.mod`: quedarse con los dos lados. Primero los `require` del PR #3, después el bloque del `retract`.
- `CHANGELOG.md`, `README.md` y `docs/design/product.md`: quedarse con los dos lados; los cambios de cada uno son independientes.

Después de resolver:

```bash
go mod tidy && git diff --exit-code go.mod go.sum
gofmt -l . && go vet ./... && go build ./... && go test -race -count=1 ./...
git commit --no-edit
```

Expected: `go mod tidy` no cambia nada, sin salida de gofmt ni vet, todos los tests en `ok`.

- [ ] **Step 2: Sumar las líneas de walking-skeleton al CHANGELOG**

En `CHANGELOG.md`, bajo `## [Unreleased]` → `### Added`, agregar al final:

```markdown
- `wt list`: one table with every worktree of the current repository, current
  (`@`) and main (`^`) marks and a `STATE` column; `--json` uses `wt.list.v1`.
- `wt config path|list|get`: layered configuration (defaults < user file <
  `.wt.toml` < `WT_*`), with an allowlist for the repository file.
- The CLI contract: documented exit codes 0–8, strict validation of commands
  and flags, `wt:`/`hint:` errors or `wt.error.v1` with `--json`, and the
  global flags `--json`, `--dry-run`, `-C` and `--no-color`.
- `wt version`, which works even when the configuration is broken.
```

En `### Changed`, reemplazar el bullet `Nothing works yet. …` por:

```markdown
- `main` has read-only commands only (`list`, `config`, `version`); the stable
  line is still `powershell/v0.9.x`.
```

```bash
git add CHANGELOG.md
git commit -m "docs(walking-skeleton): add the changelog entries" \
  -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

- [ ] **Step 3: Probar el check localmente**

Run: `go run ./tools/relcheck pr --branch v0.1/walking-skeleton --title "feat(walking-skeleton): add wt list, wt config and CLI contract [v0.1.0-alpha.1]"`
Expected: `ok: publishes v0.1.0-alpha.1`

- [ ] **Step 4: ⚠️ Push y título nuevo, con confirmación del usuario**

```bash
git push
gh pr edit 3 --title "feat(walking-skeleton): add wt list, wt config and CLI contract [v0.1.0-alpha.1]"
gh pr checks 3 --watch
```

Expected: los 5 checks en `pass`, `pr-conventions` incluido, con `ok: publishes v0.1.0-alpha.1` en su log.

- [ ] **Step 5: ⚠️ Squash merge, con confirmación del usuario**

Run: `gh pr merge 3 --squash`

- [ ] **Step 6: Seguir el release**

```bash
run=$(gh run list --workflow release.yml --branch main --event push --limit 1 --json databaseId,headSha --jq '.[0].databaseId')
gh run watch "$run" --exit-status
gh run view "$run" --log | grep 'publishes:'
```

Confirmar que la corrida corresponde al commit del merge (`headSha` igual a `git rev-parse origin/main` después de un `git fetch`); si todavía no apareció, repetir el `gh run list`.

Expected: termina bien y el log dice `publishes: v0.1.0-alpha.1`.

- [ ] **Step 7: Verificar el Release**

Run: `gh release view v0.1.0-alpha.1 --json isPrerelease,isDraft,assets --jq '{pre: .isPrerelease, draft: .isDraft, assets: [.assets[].name]}'`
Expected: `pre: true`, `draft: false` y estos assets: `checksums.txt`, `git-wt_0.1.0-alpha.1_darwin_amd64.tar.gz`, `git-wt_0.1.0-alpha.1_darwin_arm64.tar.gz`, `git-wt_0.1.0-alpha.1_linux_amd64.tar.gz`, `git-wt_0.1.0-alpha.1_linux_arm64.tar.gz`, `git-wt_0.1.0-alpha.1_windows_amd64.zip`.

- [ ] **Step 8: Verificar el binario descargado**

```bash
D=$(mktemp -d)
gh release download v0.1.0-alpha.1 -D "$D" -p 'git-wt_0.1.0-alpha.1_darwin_arm64.tar.gz' -p checksums.txt
cd "$D" && shasum -a 256 -c checksums.txt --ignore-missing && tar -xzf git-wt_0.1.0-alpha.1_darwin_arm64.tar.gz
./git-wt version
cd - && rm -rf "$D"
```

Expected: `git-wt_0.1.0-alpha.1_darwin_arm64.tar.gz: OK` y `wt 0.1.0-alpha.1 (<12 caracteres del commit>)`.

- [ ] **Step 9: Pedirle al usuario la verificación en Windows**

Descargar `git-wt_0.1.0-alpha.1_windows_amd64.zip` del Release, descomprimir y correr `git-wt.exe version` y `git-wt.exe list` dentro de un repo. Lo tiene que hacer el usuario en la máquina Windows.

- [ ] **Step 10: Limpieza local**

```bash
git switch main && git pull --ff-only
git branch -D v0.1/walking-skeleton ci/release-conventions
git fetch origin --prune
```

`-D` es necesario: con squash, git no ve esas ramas como mergeadas. El plan sigue en disco, ignorado por `info/exclude`; se puede borrar a mano cuando ya no haga falta.
