package approval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jMautone/worktree-manager/internal/hook"
)

func TestHash(t *testing.T) {
	// printf 'npm ci' | sha256sum
	const want = "9db3f780def6105eee3cc930de4d0982607760820fddaa3facdd3813ccb40628"
	if got := Hash("npm ci"); got != want {
		t.Errorf("Hash = %q, want %q", got, want)
	}
}

func TestApproved(t *testing.T) {
	var s Set
	if !s.Add("/src/repo", hook.PostCreate, "npm ci") {
		t.Fatal("Add of a new approval reported false")
	}
	for _, tc := range []struct {
		name    string
		repo    string
		event   hook.Event
		command string
		want    bool
	}{
		{"same", "/src/repo", hook.PostCreate, "npm ci", true},
		{"other command", "/src/repo", hook.PostCreate, "npm ci ", false},
		{"other event", "/src/repo", hook.PreCreate, "npm ci", false},
		{"other repo", "/src/other", hook.PostCreate, "npm ci", false},
		{"case of the path", "/src/Repo", hook.PostCreate, "npm ci", false},
	} {
		if got := s.Approved(tc.repo, tc.event, tc.command); got != tc.want {
			t.Errorf("%s: Approved = %v, want %v", tc.name, got, tc.want)
		}
	}
	if s.Add("/src/repo", hook.PostCreate, "npm ci") {
		t.Error("Add of an existing approval reported true")
	}
	if s.Len() != 1 {
		t.Errorf("Len = %d, want 1", s.Len())
	}
}

func TestRoundTrip(t *testing.T) {
	var s Set
	s.Add(`C:\src\repo`, hook.PostCreate, `cp "{repo_path}/.env" .env`)
	s.Add("/src/repo", hook.PreRemove, "make clean")
	got, err := Parse(s.Marshal(), "approvals.toml")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.list) != 2 || got.list[0] != s.list[0] || got.list[1] != s.list[1] {
		t.Errorf("round trip = %+v, want %+v", got.list, s.list)
	}
	if !strings.Contains(string(s.Marshal()), "[[approval]]") {
		t.Errorf("Marshal has no [[approval]] table:\n%s", s.Marshal())
	}
}

func TestParseCommandDecidesNothing(t *testing.T) {
	data := "[[approval]]\nrepo = \"/r\"\nevent = \"post-create\"\nsha256 = \"" + Hash("npm ci") + "\"\ncommand = \"edited\"\nextra = 1\n"
	s, err := Parse([]byte(data), "a.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Approved("/r", hook.PostCreate, "npm ci") {
		t.Error("an edited command field revoked the approval")
	}
}

func TestParseErrors(t *testing.T) {
	h := Hash("x")
	for name, data := range map[string]string{
		"invalid TOML":     "[[approval",
		"no repo":          "[[approval]]\nevent = \"post-create\"\nsha256 = \"" + h + "\"\n",
		"no event":         "[[approval]]\nrepo = \"/r\"\nsha256 = \"" + h + "\"\n",
		"no sha256":        "[[approval]]\nrepo = \"/r\"\nevent = \"post-create\"\n",
		"short sha256":     "[[approval]]\nrepo = \"/r\"\nevent = \"post-create\"\nsha256 = \"abc\"\n",
		"uppercase sha256": "[[approval]]\nrepo = \"/r\"\nevent = \"post-create\"\nsha256 = \"" + strings.ToUpper(h) + "\"\n",
	} {
		_, err := Parse([]byte(data), "/cfg/approvals.toml")
		if err == nil || !strings.Contains(err.Error(), "/cfg/approvals.toml") {
			t.Errorf("%s: err = %v, want one naming the file", name, err)
		}
	}
}

func TestLoadMissing(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "approvals.toml"))
	if err != nil || s.Len() != 0 {
		t.Errorf("Load of a missing file = %v, %v; want empty and no error", s, err)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	path := filepath.Join(dir, "approvals.toml")
	var s Set
	s.Add("/r", hook.PostCreate, "npm ci")
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	s.Add("/r", hook.PreRemove, "make clean")
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Len() != 2 || !got.Approved("/r", hook.PreRemove, "make clean") {
		t.Errorf("Load after Save = %+v", got.list)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory has %d entries, want only approvals.toml", len(entries))
	}
}
