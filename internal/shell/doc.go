// Package shell implements the directive-file mechanism and emits the wrapper
// functions and completions for zsh, bash, fish and PowerShell 7.
//
// A binary cannot change its parent shell's working directory. The wrapper
// creates a temporary file, exports WT_DIRECTIVE_CD_FILE, runs the binary, and
// cd's to whatever path the binary wrote. This is the only portable mechanism
// for `wt cd`, and it is also what makes the shell function shadow wt.exe on
// Windows.
package shell
