package worktree

import (
	"reflect"
	"testing"
)

func TestLabel(t *testing.T) {
	main := Worktree{Name: "repo", Path: "/src/repo", Main: true}
	feat := Worktree{Name: "feat", Path: "/src/repo.worktrees/feat"}
	for _, tc := range []struct {
		repo string
		w    Worktree
		want string
	}{
		{"", main, "^"},
		{"", feat, "feat"},
		{"api", main, "api:^"},
		{"api", feat, "api:feat"},
	} {
		if got := Label(tc.repo, tc.w); got != tc.want {
			t.Errorf("Label(%q, %s) = %q, want %q", tc.repo, tc.w.Name, got, tc.want)
		}
	}
}

func TestPlanEach(t *testing.T) {
	bare := Worktree{Name: ".bare", Path: "/p/.bare", Bare: true, Main: true}
	apiMain := Worktree{Name: "api", Path: "/r/api", Main: true}
	apiFeat := Worktree{Name: "feat", Path: "/r/api.worktrees/feat"}
	apiOld := Worktree{Name: "old", Path: "/r/api.worktrees/old", Prunable: true}
	apiUSB := Worktree{Name: "usb", Path: "/mnt/usb/api", Locked: true}
	apiPlain := Worktree{Name: "plain", Path: "/r/api.worktrees/plain"}
	webMain := Worktree{Name: "web", Path: "/r/web", Main: true}
	pFeat := Worktree{Name: "feat", Path: "/p/feat"}

	labels := func(ts []EachTarget) []string {
		out := []string{}
		for _, t := range ts {
			out = append(out, t.Label+" "+t.Worktree.Path)
		}
		return out
	}

	for _, tc := range []struct {
		name     string
		groups   []EachGroup
		gone     map[string]bool
		run      []string
		goneWant []string
	}{
		{
			name:   "current repository",
			groups: []EachGroup{{Worktrees: []Worktree{apiMain, apiFeat}}},
			run:    []string{"^ /r/api", "feat /r/api.worktrees/feat"},
		},
		{
			name:   "every repository, in the groups' order",
			groups: []EachGroup{{"api", []Worktree{apiMain, apiFeat}}, {"web", []Worktree{webMain}}},
			run:    []string{"api:^ /r/api", "api:feat /r/api.worktrees/feat", "web:^ /r/web"},
		},
		{
			name:   "bare repository left out",
			groups: []EachGroup{{Worktrees: []Worktree{bare, pFeat}}},
			gone:   map[string]bool{"/p/.bare": true},
			run:    []string{"feat /p/feat"},
		},
		{
			name:     "directory gone: prunable, locked or neither",
			groups:   []EachGroup{{"api", []Worktree{apiMain, apiOld, apiPlain, apiUSB}}},
			gone:     map[string]bool{"/r/api.worktrees/old": true, "/mnt/usb/api": true, "/r/api.worktrees/plain": true},
			run:      []string{"api:^ /r/api"},
			goneWant: []string{"api:old /r/api.worktrees/old", "api:plain /r/api.worktrees/plain", "api:usb /mnt/usb/api"},
		},
		{
			name:   "prunable with its directory: run",
			groups: []EachGroup{{Worktrees: []Worktree{apiMain, apiOld}}},
			run:    []string{"^ /r/api", "old /r/api.worktrees/old"},
		},
		{name: "no groups"},
		{name: "empty groups", groups: []EachGroup{{"api", nil}, {"web", []Worktree{}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := PlanEach(tc.groups, tc.gone)
			if tc.run == nil {
				tc.run = []string{}
			}
			if tc.goneWant == nil {
				tc.goneWant = []string{}
			}
			if got := labels(plan.Run); !reflect.DeepEqual(got, tc.run) {
				t.Errorf("Run = %q, want %q", got, tc.run)
			}
			if got := labels(plan.Gone); !reflect.DeepEqual(got, tc.goneWant) {
				t.Errorf("Gone = %q, want %q", got, tc.goneWant)
			}
		})
	}
}
