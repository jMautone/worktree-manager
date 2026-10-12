package cli

import (
	"reflect"
	"testing"

	"github.com/jMautone/worktree-manager/internal/git"
	"github.com/jMautone/worktree-manager/internal/workspace"
	"github.com/jMautone/worktree-manager/internal/worktree"
)

func TestResolutionMessages(t *testing.T) {
	repos := []workspace.Repo{
		{Name: "api", Path: "/Users/me/Documents/GIT/api"},
		{Name: "api", Path: "/Users/me/Repos/api"},
		{Name: "web", Path: "/Users/me/Repos/web"},
	}
	names := newWorkspaceNames(repos, "darwin")
	notRepo := &git.NotRepoError{Dir: "/tmp"}
	for _, tc := range []struct {
		name  string
		err   *Error
		code  int
		msg   string
		hints []string
	}{
		{"ambiguous repositories with the same name",
			repoAmbiguity(&workspace.AmbiguousError{Target: "api", Candidates: repos[:2]}, names), 4,
			`"api" matches more than one repository`,
			[]string{"GIT/api:  /Users/me/Documents/GIT/api", "Repos/api:  /Users/me/Repos/api"}},
		{"ambiguous worktrees of other repositories",
			othersAmbiguity(&worktree.OthersAmbiguousError{Target: "feat", Hits: []worktree.Hit{
				{Group: 0, Worktree: worktree.Worktree{Name: "feat", Path: "/Users/me/Repos/api.worktrees/feat"}},
				{Group: 1, Worktree: worktree.Worktree{Name: "feat", Path: "/Users/me/Repos/web.worktrees/feat"}},
			}}, repos[1:], names), 4,
			`"feat" matches more than one worktree`,
			[]string{"Repos/api:feat  /Users/me/Repos/api.worktrees/feat", "web:feat  /Users/me/Repos/web.worktrees/feat"}},
		{"navigate, inside, no roots", nameNotFound("x", nil, false, modeNavigate), 3,
			`no worktree named "x"`, []string{hintList}},
		{"navigate, inside, roots", nameNotFound("x", nil, true, modeNavigate), 3,
			`no worktree or repository named "x"`, []string{hintListAll, hintRepos}},
		{"navigate, outside, roots", nameNotFound("x", notRepo, true, modeNavigate), 3,
			`no worktree or repository named "x"`, []string{hintListAll, hintRepos}},
		{"navigate, outside, no roots", nameNotFound("x", notRepo, false, modeNavigate), 3,
			"not a git repository: /tmp", []string{hintSetRoots}},
		{"worktree, inside, no roots", nameNotFound("x", nil, false, modeWorktree), 3,
			`no worktree named "x"`, []string{hintList}},
		{"worktree, inside, roots", nameNotFound("x", nil, true, modeWorktree), 3,
			`no worktree named "x"`, []string{hintListAll}},
		{"worktree, outside, roots", nameNotFound("x", notRepo, true, modeWorktree), 3,
			`no worktree named "x"`, []string{hintListAll}},
		{"worktree, outside, no roots", nameNotFound("x", notRepo, false, modeWorktree), 3,
			"not a git repository: /tmp", []string{hintSetRoots}},
		{"qualified, no such worktree", qualifiedNotFound("nope", names.of(repos[1])), 3,
			`no worktree named "nope" in Repos/api`, []string{hintListAll}},
		{"qualified, no such repository", repoNotFound(&workspace.NotFoundError{Target: "GIT/ap"}), 3,
			`no repository named "GIT/ap"`, []string{hintRepos}},
		{"missing repository side", qualifiedUsage(":feat", "", "feat"), 2,
			`missing repository before ":" in ":feat"`, nil},
		{"@ after a repository", qualifiedUsage("api:@", "api", "@"), 2,
			`"api:@": @ and - cannot follow a repository`, nil},
		{"- after a repository", qualifiedUsage("api:-", "api", "-"), 2,
			`"api:-": @ and - cannot follow a repository`, nil},
	} {
		if tc.err == nil {
			t.Errorf("%s: no error", tc.name)
			continue
		}
		if tc.err.Code != tc.code || tc.err.Msg != tc.msg || !reflect.DeepEqual(tc.err.Hints, tc.hints) {
			t.Errorf("%s: %d %q %q; want %d %q %q", tc.name, tc.err.Code, tc.err.Msg, tc.err.Hints, tc.code, tc.msg, tc.hints)
		}
	}
	if err := qualifiedUsage("api:feat", "api", "feat"); err != nil {
		t.Errorf("api:feat: %v", err)
	}
	if err := qualifiedUsage("api:", "api", ""); err != nil {
		t.Errorf("api:: %v", err)
	}
}
