package process

import "strings"

// Line builds the command line that runs words with goos's interpreter, for
// Command.
//
// A single word is the command line as is, so that the interpreter reads
// it as written, operators included, as with -x. Several words are joined
// with spaces, each written so that it reaches the program as one argument:
// in sh's single quotes on macOS and Linux, and on windows as the Microsoft
// C runtime splits a command line, with cmd.exe's operators escaped.
func Line(goos string, words []string) string {
	if len(words) == 1 {
		return words[0]
	}
	quote := shQuote
	if goos == "windows" {
		quote = msvcrtQuote
	}
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = quote(w)
	}
	line := strings.Join(quoted, " ")
	if goos == "windows" {
		line = cmdEscape(line)
	}
	return line
}

// shQuote writes w for sh: as is when it is made only of characters sh
// never interprets, and otherwise between single quotes, inside which sh
// interprets nothing; a ' in w ends the quotes, is escaped and reopens them.
func shQuote(w string) string {
	if w != "" && strings.Trim(w, shSafe) == "" {
		return w
	}
	return "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
}

const shSafe = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789@%+=:,./_-"

// msvcrtQuote writes w as the Microsoft C runtime reads one argument: as
// is, unless it is empty or has a space, a tab, a double quote or one of
// cmd.exe's operators, in which case it goes between double quotes, with
// each " written as \" and the backslashes before a " or before the closing
// quote doubled. It is syscall.EscapeArg, which only builds on windows, plus
// the quotes around cmd.exe's operators.
func msvcrtQuote(w string) string {
	if w != "" && !strings.ContainsAny(w, " \t\"&|<>^()") {
		return w
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for i := 0; i < len(w); i++ {
		c := w[i]
		switch c {
		case '\\':
			slashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, slashes+1))
			slashes = 0
		default:
			slashes = 0
		}
		b.WriteByte(c)
	}
	b.WriteString(strings.Repeat(`\`, slashes))
	b.WriteByte('"')
	return b.String()
}

// cmdEscape prefixes with ^ each of cmd.exe's operators that cmd.exe sees
// outside quotes in line. cmd.exe toggles its quote state at every ", the
// ones of \" included, so an operator inside a quoted word can be outside
// for cmd.exe: in "a \"&\" b" the & is. cmd.exe removes the ^ before it
// starts the program. %NAME% is left alone: cmd.exe expands it anyway.
func cmdEscape(line string) string {
	var b strings.Builder
	quoted := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '"':
			quoted = !quoted
		case !quoted && strings.IndexByte("&|<>^()", c) >= 0:
			b.WriteByte('^')
		}
		b.WriteByte(c)
	}
	return b.String()
}
