package worktree

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/jMautone/worktree-manager/internal/template"
)

// CreatePlan is everything wt create decided, before any effect. The same
// plan feeds --dry-run, --json and the creation itself.
type CreatePlan struct {
	Name   string // <name> as given
	Path   string // native absolute path of the new worktree
	Branch string // the new branch
	Base   string // the base as written
	Head   string // full commit id the branch starts at
	Fetch  string // remote fetched before resolving the base; "" for none
	CD     bool   // move the shell into the new worktree
	Exec   string // the -x command; "" for none
}

// InvalidNameError means <name> is empty or escapes its directory.
type InvalidNameError struct {
	Name string
}

func (e *InvalidNameError) Error() string {
	if e.Name == "" {
		return "the name may not be empty"
	}
	return fmt.Sprintf("invalid name %q: it may not have a . or .. component", e.Name)
}

// ValidateName rejects an empty name and a name with a component, separated
// by / or \ on every OS, equal to . or ..: such a name would put the
// worktree outside the directory the template meant.
func ValidateName(name string) error {
	if name == "" {
		return &InvalidNameError{Name: name}
	}
	for _, c := range strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' }) {
		if c == "." || c == ".." {
			return &InvalidNameError{Name: name}
		}
	}
	return nil
}

// BranchFor is the new branch: -b as given when it was given, otherwise the
// prefix followed by the name.
func BranchFor(name, branchFlag, prefix string) string {
	if branchFlag != "" {
		return branchFlag
	}
	return prefix + name
}

// PathVars are the values of the worktree_path variables. main is the main
// worktree as Build returns it, with its native path.
func PathVars(main Worktree, name, branch, goos string) map[string]string {
	parent, last := splitLast(main.Path, goos)
	repo := last
	if trimmed := strings.TrimSuffix(last, ".git"); main.Bare && trimmed != "" {
		repo = trimmed
	}
	return map[string]string{
		"repo":        repo,
		"repo_parent": parent,
		"repo_path":   main.Path,
		"name":        name,
		"branch":      branch,
	}
}

// splitLast splits a native absolute path into its parent and its last
// component. The parent of a component at the root is the root: / or C:\.
func splitLast(p, goos string) (string, string) {
	vol, rest := volume(toSlash(p, goos), goos)
	rest = strings.TrimRight(rest, "/")
	i := strings.LastIndex(rest, "/")
	if i < 0 {
		return fromSlash(vol+"/", goos), rest
	}
	parent := rest[:i]
	if parent == "" {
		parent = "/"
	}
	return fromSlash(vol+parent, goos), rest[i+1:]
}

// ResolvePath turns a rendered template into the native absolute path of
// the new worktree, without reading the filesystem:
//
//   - On windows both / and \ separate components; elsewhere only / does.
//   - A leading ~, alone or followed by a separator, is home.
//   - A relative path is relative to repoPath, the main worktree's path. On
//     windows, a path rooted without a drive (\x) takes repoPath's drive.
//   - . and .. are resolved lexically and repeated separators count as one;
//     a UNC prefix \\server\share is kept.
func ResolvePath(rendered, repoPath, home, goos string) (string, error) {
	p := toSlash(rendered, goos)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home == "" {
			return "", fmt.Errorf("cannot expand ~ in %s: the home directory is not set", rendered)
		}
		p = strings.TrimRight(toSlash(home, goos), "/") + p[1:]
	}
	vol, rest := volume(p, goos)
	switch {
	case strings.HasPrefix(vol, "//") && rest == "":
		rest = "/"
	case !strings.HasPrefix(rest, "/"):
		// C:x is relative to the current directory of drive C, which wt
		// does not have: it stays relative, and CheckWindowsPath rejects
		// the colon.
		vol, rest = volume(strings.TrimRight(toSlash(repoPath, goos), "/")+"/"+p, goos)
	case vol == "" && goos == "windows":
		vol, _ = volume(toSlash(repoPath, goos), goos)
	}
	return fromSlash(vol+path.Clean(rest), goos), nil
}

// volume splits the volume off a path with forward slashes: C: or
// //server/share on windows, nothing elsewhere.
func volume(p, goos string) (string, string) {
	if goos != "windows" {
		return "", p
	}
	if len(p) >= 2 && p[1] == ':' && isLetter(p[0]) {
		return p[:2], p[2:]
	}
	if strings.HasPrefix(p, "//") && !strings.HasPrefix(p, "///") {
		parts := strings.SplitN(p[2:], "/", 3)
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			vol := "//" + parts[0] + "/" + parts[1]
			return vol, p[len(vol):]
		}
	}
	return "", p
}

func isLetter(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }

func toSlash(p, goos string) string {
	if goos == "windows" {
		return strings.ReplaceAll(p, `\`, "/")
	}
	return p
}

func fromSlash(p, goos string) string {
	if goos == "windows" {
		return strings.ReplaceAll(p, "/", `\`)
	}
	return p
}

// InvalidPathError means the path of the new worktree cannot exist on
// Windows.
type InvalidPathError struct {
	Path string
}

func (e *InvalidPathError) Error() string { return "path is not valid on Windows: " + e.Path }

// CheckWindowsPath rejects, on windows, a native path with a component other
// than the drive or the UNC share that Windows does not allow: a reserved
// device name, with or without an extension; one of < > : " | ? * or a
// control character; or a trailing . or space. On other OSes it accepts
// every path: the rule is Windows's, and sanitize makes it never fire.
func CheckWindowsPath(p, goos string) error {
	if goos != "windows" {
		return nil
	}
	_, rest := volume(toSlash(p, goos), goos)
	for _, c := range strings.Split(rest, "/") {
		if c == "" {
			continue
		}
		if template.Reserved(c) || strings.IndexFunc(c, template.Invalid) >= 0 ||
			strings.HasSuffix(c, ".") || strings.HasSuffix(c, " ") {
			return &InvalidPathError{Path: p}
		}
	}
	return nil
}

// RegisteredAt returns the worktree git has registered at p, comparing
// paths by components and ignoring case on darwin and windows, as Build
// does. It finds a registration whose directory is gone (prunable).
func RegisteredAt(ws []Worktree, p, goos string) (Worktree, bool) {
	fold := goos == "darwin" || goos == "windows"
	want := components(p, goos)
	for _, w := range ws {
		got := components(w.Path, goos)
		if len(got) == len(want) && contains(got, want, fold) {
			return w, true
		}
	}
	return Worktree{}, false
}

// DefaultBranch is the repository's default branch: originHEAD (what
// origin/HEAD points to, as origin/<branch>) when it is set; otherwise the
// branch of the main worktree, or for a bare repository bareHEAD, the branch
// its HEAD points to. With neither, there is no default branch.
func DefaultBranch(originHEAD string, main Worktree, bareHEAD string) (string, bool) {
	switch {
	case originHEAD != "":
		return originHEAD, true
	case main.Bare:
		return bareHEAD, bareHEAD != ""
	}
	return main.Branch, main.Branch != ""
}

// FetchRemote is the remote to fetch before resolving base: the longest
// remote name that base starts with, followed by /.
func FetchRemote(base string, remotes []string) (string, bool) {
	best := ""
	for _, r := range remotes {
		if r != "" && strings.HasPrefix(base, r+"/") && len(r) > len(best) {
			best = r
		}
	}
	return best, best != ""
}

// BranchExistsError means the branch wt create would create already exists:
// locally (Remote is "") or as a remote-tracking branch of Remote. Worktree
// is the worktree that has the local branch checked out, if any.
type BranchExistsError struct {
	Branch   string
	Remote   string
	Worktree *Worktree
}

func (e *BranchExistsError) Error() string {
	if e.Remote != "" {
		return fmt.Sprintf("branch %q already exists on remote %s", e.Branch, e.Remote)
	}
	return fmt.Sprintf("branch %q already exists", e.Branch)
}

// BranchTaken checks that branch is new. refs are full reference names
// (refs/heads/x, refs/remotes/origin/x); remotes are the configured remote
// names. A local branch is reported before a remote one.
func BranchTaken(branch string, refs, remotes []string, ws []Worktree) error {
	if slices.Contains(refs, "refs/heads/"+branch) {
		e := &BranchExistsError{Branch: branch}
		for i := range ws {
			if ws[i].Branch == branch {
				e.Worktree = &ws[i]
				break
			}
		}
		return e
	}
	for _, r := range remotes {
		if slices.Contains(refs, "refs/remotes/"+r+"/"+branch) {
			return &BranchExistsError{Branch: branch, Remote: r}
		}
	}
	return nil
}
