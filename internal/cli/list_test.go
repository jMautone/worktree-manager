package cli_test

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/testutil"
)

var listColumns = []string{"NAME", "BRANCH", "HEAD", "STATE", "PATH"}

type row struct {
	markers string
	cols    map[string]string
}

// parseTable splits `wt list` output into rows using the header's column
// offsets, which also checks that every row is aligned to the header.
func parseTable(t *testing.T, out string) []row {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	header := lines[0]
	offsets := make([]int, len(listColumns))
	for i, c := range listColumns {
		offsets[i] = strings.Index(header, c)
		if offsets[i] < 0 || (i > 0 && offsets[i] <= offsets[i-1]) {
			t.Fatalf("header %q: column %s missing or out of order", header, c)
		}
	}
	if strings.TrimSpace(header[:offsets[0]]) != "" {
		t.Errorf("header of the markers column is not blank: %q", header)
	}
	var rows []row
	for _, line := range lines[1:] {
		r := row{markers: strings.TrimSpace(line[:offsets[0]]), cols: map[string]string{}}
		for i, c := range listColumns {
			end := len(line)
			if i+1 < len(offsets) && offsets[i+1] < end {
				end = offsets[i+1]
			}
			if offsets[i] >= len(line) {
				continue
			}
			cell := line[offsets[i]:end]
			if i+1 < len(offsets) && !strings.HasSuffix(cell, " ") {
				t.Errorf("row %q: column %s runs into the next one", line, c)
			}
			r.cols[c] = strings.TrimSpace(cell)
		}
		rows = append(rows, r)
	}
	return rows
}

func findRow(t *testing.T, rows []row, name string) row {
	t.Helper()
	for _, r := range rows {
		if r.cols["NAME"] == name {
			return r
		}
	}
	t.Fatalf("no row named %s", name)
	return row{}
}

type listFixture struct {
	h                                   *harness
	repo, feat, det, locked, gone, bare string
}

// newListFixture creates a repository with a linked worktree on a branch with
// a slash, a detached one, one locked with a reason and a prunable one.
func newListFixture(t *testing.T) *listFixture {
	h := newHarness(t)
	f := &listFixture{h: h, repo: h.repo()}
	f.feat = h.sb.Path("feat")
	h.sb.AddWorktree(f.repo, f.feat, "feature/abc1")
	f.det = h.sb.Path("det")
	h.sb.AddDetachedWorktree(f.repo, f.det)
	f.locked = h.sb.Path("locked")
	h.sb.AddWorktree(f.repo, f.locked, "locked-branch")
	h.sb.LockWorktree(f.repo, f.locked, "on usb drive")
	f.gone = h.sb.Path("gone")
	h.sb.AddWorktree(f.repo, f.gone, "gone-branch")
	h.sb.MakePrunable(f.gone)
	return f
}

func TestListTable(t *testing.T) {
	f := newListFixture(t)
	f.h.cwd = f.feat
	head := f.h.sb.Git(f.repo, "rev-parse", "HEAD")

	r := f.h.run("list")
	r.mustCode(t, 0)
	rows := parseTable(t, r.stdout)
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5:\n%s", len(rows), r.stdout)
	}

	main := rows[0]
	if main.markers != "^" || main.cols["NAME"] != "repo" || main.cols["BRANCH"] != "main" {
		t.Errorf("first row = %+v, want the main worktree marked ^", main)
	}
	if main.cols["PATH"] != testutil.Comparable(t, f.repo) {
		t.Errorf("main PATH = %q, want %q", main.cols["PATH"], testutil.Comparable(t, f.repo))
	}
	if main.cols["HEAD"] != head[:7] {
		t.Errorf("main HEAD = %q, want %q", main.cols["HEAD"], head[:7])
	}

	feat := findRow(t, rows, "feat")
	if feat.markers != "@" || feat.cols["BRANCH"] != "feature/abc1" || feat.cols["STATE"] != "" {
		t.Errorf("current row = %+v", feat)
	}
	if det := findRow(t, rows, "det"); det.markers != "" || det.cols["BRANCH"] != "(detached)" || det.cols["HEAD"] != head[:7] {
		t.Errorf("detached row = %+v", det)
	}
	if locked := findRow(t, rows, "locked"); locked.cols["STATE"] != "locked" {
		t.Errorf("locked row = %+v", locked)
	}
	if gone := findRow(t, rows, "gone"); gone.cols["STATE"] != "prunable" {
		t.Errorf("prunable row = %+v", gone)
	}
}

func TestListCurrentAndMainMarkersTogether(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.repo()
	rows := parseTable(t, h.run("list").stdout)
	if rows[0].markers != "@^" {
		t.Errorf("markers = %q, want @^", rows[0].markers)
	}
}

func TestListBareTable(t *testing.T) {
	h := newHarness(t)
	bare := h.sb.Path("bare.git")
	h.sb.InitBare(bare)
	h.cwd = bare

	rows := parseTable(t, h.run("list").stdout)
	if rows[0].cols["BRANCH"] != "(bare)" || rows[0].cols["HEAD"] != "-" || rows[0].markers != "@^" {
		t.Errorf("bare row = %+v", rows[0])
	}
}

func TestListOrder(t *testing.T) {
	h := newHarness(t)
	repo := h.repo()
	h.cwd = repo
	for _, name := range []string{"zeta", "Alpha", "beta"} {
		h.sb.AddWorktree(repo, h.sb.Path(name), "b-"+strings.ToLower(name))
	}

	var names []string
	for _, r := range parseTable(t, h.run("list").stdout) {
		names = append(names, r.cols["NAME"])
	}
	if want := []string{"repo", "Alpha", "beta", "zeta"}; !reflect.DeepEqual(names, want) {
		t.Errorf("order = %v, want %v", names, want)
	}
}

func TestListJSON(t *testing.T) {
	f := newListFixture(t)
	f.h.cwd = f.feat
	f.h.sb.LockWorktree(f.repo, f.det, "")
	head := f.h.sb.Git(f.repo, "rev-parse", "HEAD")

	r := f.h.run("list", "--json")
	r.mustCode(t, 0)
	doc := decodeOne(t, r.stdout)
	if doc["schema"] != "wt.list.v1" {
		t.Errorf("schema = %v", doc["schema"])
	}
	raw := doc["worktrees"].([]any)
	byPath := map[string]map[string]any{}
	var names []string
	wantKeys := []string{"bare", "branch", "current", "detached", "head", "locked", "locked_reason", "main", "name", "path", "prunable", "prunable_reason"}
	for _, w := range raw {
		w := w.(map[string]any)
		var keys []string
		for k := range w {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if !reflect.DeepEqual(keys, wantKeys) {
			t.Errorf("element fields = %v, want %v", keys, wantKeys)
		}
		byPath[testutil.Comparable(t, w["path"].(string))] = w
		names = append(names, w["name"].(string))
	}

	// Same order as the table.
	var tableNames []string
	for _, row := range parseTable(t, f.h.run("list").stdout) {
		tableNames = append(tableNames, row.cols["NAME"])
	}
	if !reflect.DeepEqual(names, tableNames) {
		t.Errorf("JSON order %v differs from table order %v", names, tableNames)
	}

	main := byPath[testutil.Comparable(t, f.repo)]
	if main["main"] != true || main["current"] != false || main["branch"] != "main" || main["head"] != head {
		t.Errorf("main = %v", main)
	}
	if feat := byPath[testutil.Comparable(t, f.feat)]; feat["current"] != true || feat["branch"] != "feature/abc1" {
		t.Errorf("feat = %v", feat)
	}
	det := byPath[testutil.Comparable(t, f.det)]
	if det["branch"] != nil || det["detached"] != true || det["head"] != head {
		t.Errorf("detached = %v, want branch null, detached true and the full head", det)
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(head) {
		t.Fatalf("unexpected commit id %q", head)
	}
	if det["locked"] != true || det["locked_reason"] != nil {
		t.Errorf("locked without a reason = %v, want locked_reason null", det)
	}
	if locked := byPath[testutil.Comparable(t, f.locked)]; locked["locked"] != true || locked["locked_reason"] != "on usb drive" {
		t.Errorf("locked = %v", locked)
	}
	gone := byPath[testutil.Comparable(t, f.gone)]
	if gone["prunable"] != true || gone["prunable_reason"] == nil || gone["locked"] != false || gone["locked_reason"] != nil {
		t.Errorf("prunable = %v", gone)
	}
	if feat := byPath[testutil.Comparable(t, f.feat)]; feat["prunable_reason"] != nil || feat["bare"] != false {
		t.Errorf("feat = %v", feat)
	}
}

func TestListDryRunAndFlagPosition(t *testing.T) {
	f := newListFixture(t)
	f.h.cwd = f.feat

	plain := f.h.run("list")
	plain.mustCode(t, 0)
	if dry := f.h.run("list", "--dry-run"); dry != plain {
		t.Errorf("--dry-run output differs:\n%s\nvs\n%s", dry.stdout, plain.stdout)
	}
	if dry := f.h.run("--dry-run", "list"); dry != plain {
		t.Errorf("--dry-run before the command differs")
	}

	after := f.h.run("list", "--json")
	after.mustCode(t, 0)
	if before := f.h.run("--json", "list"); before != after {
		t.Errorf("wt --json list differs from wt list --json:\n%s\nvs\n%s", before.stdout, after.stdout)
	}
}
