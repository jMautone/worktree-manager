package approval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/jMautone/worktree-manager/internal/hook"
)

// Approval is one approved command.
type Approval struct {
	Repo   string `toml:"repo"`   // the main worktree's path, native and absolute
	Event  string `toml:"event"`  // the hook event
	SHA256 string `toml:"sha256"` // Hash of the command, lowercase hex
	// Command is the text approved, for whoever reads the file. It decides
	// nothing: Approved compares Repo, Event and SHA256 only.
	Command string `toml:"command"`
}

// Set is the approvals of one file, in the order they were added.
type Set struct {
	list []Approval
}

// Hash is the SHA-256 of command, as written, in lowercase hex.
func Hash(command string) string {
	sum := sha256.Sum256([]byte(command))
	return hex.EncodeToString(sum[:])
}

// Approved reports whether command is approved for event e in repo. Paths
// are compared as they are: a path written differently asks again.
func (s Set) Approved(repo string, e hook.Event, command string) bool {
	h := Hash(command)
	for _, a := range s.list {
		if a.Repo == repo && a.Event == string(e) && a.SHA256 == h {
			return true
		}
	}
	return false
}

// Add approves command for event e in repo. It reports false, and changes
// nothing, when it was already approved.
func (s *Set) Add(repo string, e hook.Event, command string) bool {
	if s.Approved(repo, e, command) {
		return false
	}
	s.list = append(s.list, Approval{Repo: repo, Event: string(e), SHA256: Hash(command), Command: command})
	return true
}

// Len is the number of approvals.
func (s Set) Len() int { return len(s.list) }

// file is the shape of approvals.toml. Pointers tell a missing field from
// an empty one.
type file struct {
	Approval []struct {
		Repo    *string `toml:"repo"`
		Event   *string `toml:"event"`
		SHA256  *string `toml:"sha256"`
		Command string  `toml:"command"`
	} `toml:"approval"`
}

var hexDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Parse reads the contents of an approvals file; origin names it in
// errors. Unknown keys are ignored, so later versions can add some.
func Parse(data []byte, origin string) (Set, error) {
	var f file
	if err := toml.Unmarshal(data, &f); err != nil {
		var derr *toml.DecodeError
		if errors.As(err, &derr) {
			row, col := derr.Position()
			return Set{}, fmt.Errorf("invalid TOML in %s at line %d, column %d: %s", origin, row, col, strings.TrimPrefix(derr.Error(), "toml: "))
		}
		return Set{}, fmt.Errorf("invalid approvals file %s: %v", origin, err)
	}
	var s Set
	for i, a := range f.Approval {
		var missing []string
		if a.Repo == nil || *a.Repo == "" {
			missing = append(missing, "repo")
		}
		if a.Event == nil || *a.Event == "" {
			missing = append(missing, "event")
		}
		if a.SHA256 == nil || *a.SHA256 == "" {
			missing = append(missing, "sha256")
		}
		if len(missing) > 0 {
			return Set{}, fmt.Errorf("invalid approvals file %s: approval %d has no %s", origin, i+1, strings.Join(missing, ", "))
		}
		if !hexDigest.MatchString(*a.SHA256) {
			return Set{}, fmt.Errorf("invalid approvals file %s: approval %d: sha256 must be 64 lowercase hexadecimal digits", origin, i+1)
		}
		s.list = append(s.list, Approval{Repo: *a.Repo, Event: *a.Event, SHA256: *a.SHA256, Command: a.Command})
	}
	return s, nil
}

const header = `# Approved repository hooks, written by "wt hook approve".
# An approval is repo + event + sha256 of the command as written in .wt.toml;
# command is only for reading. Delete a table to revoke its approval.

`

// Marshal is the contents of the approvals file for s.
func (s Set) Marshal() []byte {
	var b bytes.Buffer
	b.WriteString(header)
	out := struct {
		Approval []Approval `toml:"approval"`
	}{s.list}
	data, err := toml.Marshal(out)
	if err != nil {
		// Strings only: go-toml cannot fail on them.
		panic(err)
	}
	b.Write(data)
	return b.Bytes()
}

// Load reads the approvals file at path. A missing file is no approvals.
func Load(path string) (Set, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Set{}, nil
	}
	if err != nil {
		return Set{}, fmt.Errorf("reading approvals: %w", err)
	}
	return Parse(data, path)
}

// Save writes s to path, creating its directory: to a temporary file in the
// same directory, then renamed over path, so a reader never sees half a
// file. On Windows os.Rename replaces an existing file.
func Save(path string, s Set) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("writing approvals: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".approvals-*.toml")
	if err != nil {
		return fmt.Errorf("writing approvals: %w", err)
	}
	defer func() {
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.Write(s.Marshal()); err != nil {
		tmp.Close()
		return fmt.Errorf("writing approvals: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("writing approvals: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing approvals: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("writing approvals: %w", err)
	}
	return nil
}
