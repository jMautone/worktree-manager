package hook

import (
	"fmt"
	"strings"

	"github.com/jMautone/worktree-manager/internal/template"
)

// Event is the name of a hook event, as written in [hooks].
type Event string

const (
	PreCreate  Event = "pre-create"
	PostCreate Event = "post-create"
	PreCD      Event = "pre-cd"
	PostCD     Event = "post-cd"
	PreRemove  Event = "pre-remove"
	PostRemove Event = "post-remove"
)

// Events are every event, in the order the configuration lists them.
var Events = []Event{PreCreate, PostCreate, PreCD, PostCD, PreRemove, PostRemove}

// Background reports whether wt starts the event's commands detached and
// does not wait for them: post-cd and post-remove. Every other event blocks.
func (e Event) Background() bool {
	return e == PostCD || e == PostRemove
}

// Key is the configuration key holding the event's commands.
func (e Event) Key() string {
	return "hooks." + string(e)
}

// Vars are the variables the event's commands may use: {base} only in the
// events of wt create.
func Vars(e Event) []string {
	vars := []string{"repo", "repo_parent", "repo_path", "name", "branch", "path"}
	if e == PreCreate || e == PostCreate {
		vars = append(vars, "base")
	}
	return vars
}

// Run is what an event is going to do, already decided.
type Run struct {
	Event    Event
	Dir      string   // the working directory of every command
	Commands []string // rendered, in order; empty when the event does nothing
	Log      string   // the log's path; "" when nothing writes one
}

// Plan renders the event's command templates with values, in order. It
// cannot fail on templates the configuration already validated, but it
// parses them again and returns the error rather than trusting its caller.
func Plan(e Event, templates []string, values map[string]string, dir, log string) (Run, error) {
	r := Run{Event: e, Dir: dir, Log: log}
	for _, src := range templates {
		t, err := template.Parse(src, Vars(e))
		if err != nil {
			return Run{}, fmt.Errorf("%s: %w", e.Key(), err)
		}
		r.Commands = append(r.Commands, t.Render(values))
	}
	return r, nil
}

// LogPath is the log of event e for the worktree named name:
// <commonDir>/wt/logs/<name|sanitize>/<event>.log, in goos's form.
// commonDir is the repository's common git directory, native and absolute.
func LogPath(commonDir, name string, e Event, goos string) string {
	sep := "/"
	if goos == "windows" {
		sep = `\`
	}
	// Trimming and adding one separator keeps a root, / or C:\, as it is.
	dir := strings.TrimRight(commonDir, sep) + sep
	return dir + strings.Join([]string{"wt", "logs", template.Sanitize(name), string(e) + ".log"}, sep)
}

// Header is the line written before each command of an event.
func Header(e Event, command string) string {
	return "==> " + string(e) + "  " + command
}

// Footer is the line a log has after each command: its exit code, or why
// it could not start.
func Footer(code int, err error) string {
	if err != nil {
		return "==> could not start: " + err.Error()
	}
	return fmt.Sprintf("==> exited with %d", code)
}

// BackgroundNotice is the line wt writes once it starts a background event.
func BackgroundNotice(r Run) string {
	return "==> " + string(r.Event) + "  running in the background, log: " + r.Log
}

// PreviewLines are the lines of --dry-run for r, one per command.
func PreviewLines(r Run) []string {
	format := "would run %s hook: %s"
	if r.Event.Background() {
		format = "would run %s hook in the background: %s"
	}
	lines := make([]string, 0, len(r.Commands))
	for _, c := range r.Commands {
		lines = append(lines, fmt.Sprintf(format, r.Event, c))
	}
	return lines
}
