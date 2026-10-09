package workspace

import "testing"

func TestParseGitFile(t *testing.T) {
	for _, tc := range []struct {
		name, goos, content, dir string
		want                     string // "" when it does not parse
	}{
		{"absolute", "darwin", "gitdir: /r/api/.git/worktrees/feat\n", "/r/api.worktrees/feat", "/r/api/.git/worktrees/feat"},
		{"relative", "darwin", "gitdir: ./.bare\n", "/r/proj", "/r/proj/.bare"},
		{"relative upwards", "linux", "gitdir: ../.git/modules/lib", "/r/api/lib", "/r/api/.git/modules/lib"},
		{"CRLF and spaces", "darwin", "gitdir:   /r/sep.git  \r\n", "/r/sep", "/r/sep.git"},
		{"no space after the colon", "linux", "gitdir:/r/sep.git", "/r/sep", "/r/sep.git"},
		{"windows absolute as git writes it", "windows", "gitdir: C:/r/api/.git/worktrees/feat\n", `C:\r\api.worktrees\feat`, `C:\r\api\.git\worktrees\feat`},
		{"windows relative", "windows", "gitdir: ./.bare\r\n", `C:\r\proj`, `C:\r\proj\.bare`},
		{"windows UNC", "windows", "gitdir: //srv/share/api.git\n", `\\srv\share\api`, `\\srv\share\api.git`},
		{"garbage", "darwin", "ref: refs/heads/main\n", "/r/x", ""},
		{"empty", "darwin", "", "/r/x", ""},
		{"no path", "darwin", "gitdir: \n", "/r/x", ""},
		{"two lines", "darwin", "gitdir: /a\ngitdir: /b\n", "/r/x", ""},
		{"prefix in another case", "darwin", "GITDIR: /a\n", "/r/x", ""},
	} {
		got, ok := ParseGitFile([]byte(tc.content), tc.dir, tc.goos)
		if ok != (tc.want != "") || got != tc.want {
			t.Errorf("%s: ParseGitFile(%q, %q) = %q, %v; want %q", tc.name, tc.content, tc.dir, got, ok, tc.want)
		}
	}
}

func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		name string
		g    GitEntry
		want Kind
	}{
		{"no .git", GitEntry{}, KindNone},
		{".git directory", GitEntry{Exists: true, Dir: true, Gitdir: "/r/api/.git", GitdirIsDir: true}, KindRepo},
		{".git file to a gitdir without commondir", GitEntry{Exists: true, Gitdir: "/r/proj/.bare", GitdirIsDir: true}, KindRepo},
		{".git file to a gitdir with commondir", GitEntry{Exists: true, Gitdir: "/r/api/.git/worktrees/feat", GitdirIsDir: true, Commondir: true}, KindLinked},
		{"gitdir that does not exist", GitEntry{Exists: true, Gitdir: "/gone/.git/worktrees/feat"}, KindBroken},
		{"garbage in the .git file", GitEntry{Exists: true}, KindBroken},
	} {
		if got := Classify(tc.g); got != tc.want {
			t.Errorf("%s: Classify = %v, want %v", tc.name, got, tc.want)
		}
	}
}
