// Package hook decides what the user's hooks do around a command: which
// events exist, the variables their commands may use, the commands rendered
// with the real values, where they run, whether wt waits for them, and where
// their log goes.
//
// Decision/effect boundary: everything here is pure. It does not run a
// command, read the filesystem or run git; the OS is a parameter. Running
// the commands, starting the background process and writing the log are
// internal/cli's job.
package hook
