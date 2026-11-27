package hook

import (
	"errors"
	"slices"
	"testing"

	"github.com/jMautone/worktree-manager/internal/template"
)

func TestVars(t *testing.T) {
	for _, e := range Events {
		got := slices.Contains(Vars(e), "base")
		if want := e == PreCreate || e == PostCreate; got != want {
			t.Errorf("Vars(%s) has base: %v, want %v", e, got, want)
		}
		got = slices.Contains(Vars(e), "target")
		if want := e == PreMerge || e == PostMerge; got != want {
			t.Errorf("Vars(%s) has target: %v, want %v", e, got, want)
		}
		for _, v := range []string{"repo", "repo_parent", "repo_path", "name", "branch", "path"} {
			if !slices.Contains(Vars(e), v) {
				t.Errorf("Vars(%s) = %v, missing %s", e, Vars(e), v)
			}
		}
	}
}

func TestBackground(t *testing.T) {
	for _, e := range Events {
		if want := e == PostCD || e == PostRemove || e == PostMerge || e == PostCommit; e.Background() != want {
			t.Errorf("%s.Background() = %v, want %v", e, e.Background(), want)
		}
	}
}

func TestKey(t *testing.T) {
	if got := PostCreate.Key(); got != "hooks.post-create" {
		t.Errorf("Key() = %q", got)
	}
	if got := PreMerge.Key(); got != "hooks.pre-merge" {
		t.Errorf("Key() = %q", got)
	}
	if got := PostCommit.Key(); got != "hooks.post-commit" {
		t.Errorf("Key() = %q", got)
	}
	if !slices.Contains(Events, PreCommit) || !slices.Contains(Events, PostCommit) {
		t.Errorf("Events = %v, missing the commit events", Events)
	}
}

func TestPlan(t *testing.T) {
	values := map[string]string{"name": "feat", "repo": "repo", "base": "origin/main"}
	r, err := Plan(PreCreate, []string{"echo {name}", "echo {repo|lower} {base}", "awk '{{print $1}}'"}, values, "/src/repo", "/log")
	if err != nil {
		t.Fatal(err)
	}
	want := Run{Event: PreCreate, Dir: "/src/repo", Commands: []string{"echo feat", "echo repo origin/main", "awk '{print $1}'"}, Log: "/log"}
	if r.Event != want.Event || r.Dir != want.Dir || r.Log != want.Log || !slices.Equal(r.Commands, want.Commands) {
		t.Errorf("Plan = %+v, want %+v", r, want)
	}

	if r, err := Plan(PostCreate, nil, values, "/x", ""); err != nil || len(r.Commands) != 0 {
		t.Errorf("Plan of no commands = %+v, %v", r, err)
	}

	_, err = Plan(PreRemove, []string{"echo {base}"}, values, "/x", "")
	var se *template.SyntaxError
	if !errors.As(err, &se) || se.Part != "base" {
		t.Errorf("Plan with {base} in pre-remove = %v, want a SyntaxError naming base", err)
	}
}

func TestLogPath(t *testing.T) {
	for _, tc := range []struct {
		goos, common, name string
		e                  Event
		want               string
	}{
		{"darwin", "/src/repo/.git", "feat", PostRemove, "/src/repo/.git/wt/logs/feat/post-remove.log"},
		{"linux", "/src/proj/.bare/", "a:b", PostCD, "/src/proj/.bare/wt/logs/a-b/post-cd.log"},
		{"windows", `C:\src\repo\.git`, "feat", PostCreate, `C:\src\repo\.git\wt\logs\feat\post-create.log`},
		{"windows", `C:\src\repo.git\`, "nul", PreCD, `C:\src\repo.git\wt\logs\nul-\pre-cd.log`},
		{"darwin", "/", "x", PreRemove, "/wt/logs/x/pre-remove.log"},
		{"windows", `C:\`, "x", PreRemove, `C:\wt\logs\x\pre-remove.log`},
	} {
		if got := LogPath(tc.common, tc.name, tc.e, tc.goos); got != tc.want {
			t.Errorf("LogPath(%q, %q, %s, %s) = %q, want %q", tc.common, tc.name, tc.e, tc.goos, got, tc.want)
		}
	}
}

func TestHeaderAndFooter(t *testing.T) {
	if got := Header(PostCreate, "npm ci"); got != "==> post-create  npm ci" {
		t.Errorf("Header = %q", got)
	}
	if got := Footer(3, nil); got != "==> exited with 3" {
		t.Errorf("Footer = %q", got)
	}
	if got := Footer(0, errors.New("no such file")); got != "==> could not start: no such file" {
		t.Errorf("Footer = %q", got)
	}
	r := Run{Event: PostCD, Log: "/src/repo/.git/wt/logs/feat/post-cd.log"}
	if got := BackgroundNotice(r); got != "==> post-cd  running in the background, log: /src/repo/.git/wt/logs/feat/post-cd.log" {
		t.Errorf("BackgroundNotice = %q", got)
	}
}

func TestPreviewLines(t *testing.T) {
	got := PreviewLines(Run{Event: PreCreate, Commands: []string{"echo feat", "npm ci"}})
	want := []string{"would run pre-create hook: echo feat", "would run pre-create hook: npm ci"}
	if !slices.Equal(got, want) {
		t.Errorf("PreviewLines = %q, want %q", got, want)
	}
	got = PreviewLines(Run{Event: PostCD, Commands: []string{"git fetch"}})
	want = []string{"would run post-cd hook in the background: git fetch"}
	if !slices.Equal(got, want) {
		t.Errorf("PreviewLines = %q, want %q", got, want)
	}
	if got := PreviewLines(Run{Event: PostRemove}); len(got) != 0 {
		t.Errorf("PreviewLines of no commands = %q", got)
	}
}

func TestUnapproved(t *testing.T) {
	repo := map[string][]string{
		"hooks.pre-create":  {"ok", "make check"},
		"hooks.post-create": {"npm ci"},
		"hooks.pre-remove":  {"make clean"},
	}
	approvedOnly := func(cmds ...string) func(Event, string) bool {
		return func(_ Event, c string) bool { return slices.Contains(cmds, c) }
	}
	for _, tc := range []struct {
		name     string
		events   []Event
		approved func(Event, string) bool
		wantE    Event
		wantC    string
		wantOK   bool
	}{
		{"first in order", []Event{PreCreate, PostCreate}, approvedOnly("ok"), PreCreate, "make check", true},
		{"post event", []Event{PreCreate, PostCreate}, approvedOnly("ok", "make check"), PostCreate, "npm ci", true},
		{"all approved", []Event{PreCreate, PostCreate}, approvedOnly("ok", "make check", "npm ci"), "", "", false},
		{"other events are not checked", []Event{PreCD, PostCD}, approvedOnly(), "", "", false},
	} {
		e, c, ok := Unapproved(tc.events, repo, tc.approved)
		if e != tc.wantE || c != tc.wantC || ok != tc.wantOK {
			t.Errorf("%s: Unapproved = %q, %q, %v; want %q, %q, %v", tc.name, e, c, ok, tc.wantE, tc.wantC, tc.wantOK)
		}
	}
	asked := false
	Unapproved(Events, nil, func(Event, string) bool { asked = true; return false })
	if asked {
		t.Error("Unapproved asked about approvals with no repository commands")
	}
}

func TestEntries(t *testing.T) {
	user := map[string][]string{"hooks.post-create": {"cp ~/.npmrc ."}, "hooks.pre-remove": {"u"}}
	repo := map[string][]string{"hooks.post-create": {"npm ci"}, "hooks.pre-create": {"make check"}}
	got := Entries(user, repo, func(_ Event, c string) bool { return c == "npm ci" })
	want := []Entry{
		{PreCreate, SourceRepo, "make check", false},
		{PostCreate, SourceUser, "cp ~/.npmrc .", true},
		{PostCreate, SourceRepo, "npm ci", true},
		{PreRemove, SourceUser, "u", true},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Entries = %+v, want %+v", got, want)
	}
}
