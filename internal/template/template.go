package template

import (
	"fmt"
	"slices"
	"strings"
)

// WorktreePathVars are the variables a worktree_path template may use.
var WorktreePathVars = []string{"repo", "repo_parent", "repo_path", "name", "branch"}

// filters maps each filter name to its function. Every filter is pure and
// independent of the OS, so a template renders the same on every OS.
var filters = map[string]func(string) string{
	"sanitize": Sanitize,
	"lower":    strings.ToLower,
}

// SyntaxError is an invalid template. Part is the offending part of the
// template, as the user wrote it; Msg says what is wrong with it and names it.
type SyntaxError struct {
	Part string
	Msg  string
}

func (e *SyntaxError) Error() string { return e.Msg }

func syntaxError(part, format string) *SyntaxError {
	return &SyntaxError{Part: part, Msg: fmt.Sprintf(format, part)}
}

// Template is a parsed template, ready to render.
type Template struct {
	parts []part
}

// part is literal text or one expression.
type part struct {
	literal  string
	variable string // "" for literal text
	filters  []func(string) string
}

// Parse parses src and checks that every variable is one of vars and every
// filter exists.
//
// Syntax: literal text with expressions {variable} or
// {variable|filter|filter...}. Whitespace around names inside the braces is
// ignored. There is no way to write a literal { or }.
func Parse(src string, vars []string) (*Template, error) {
	t := &Template{}
	lit := 0 // start of the current literal text
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '}':
			return nil, syntaxError(src[lit:i+1], "} without { in %q")
		case '{':
			end := strings.IndexAny(src[i+1:], "{}")
			if end < 0 || src[i+1+end] == '{' {
				stop := len(src)
				if end >= 0 {
					stop = i + 1 + end
				}
				return nil, syntaxError(src[i:stop], "{ without } in %q")
			}
			end += i + 1
			if lit < i {
				t.parts = append(t.parts, part{literal: src[lit:i]})
			}
			p, err := parseExpr(src[i:end+1], vars)
			if err != nil {
				return nil, err
			}
			t.parts = append(t.parts, p)
			i = end
			lit = end + 1
		}
	}
	if lit < len(src) {
		t.parts = append(t.parts, part{literal: src[lit:]})
	}
	return t, nil
}

// parseExpr parses one expression, braces included.
func parseExpr(expr string, vars []string) (part, error) {
	names := strings.Split(expr[1:len(expr)-1], "|")
	for i := range names {
		names[i] = strings.TrimSpace(names[i])
	}
	p := part{variable: names[0]}
	switch {
	case p.variable == "":
		return part{}, syntaxError(expr, "no variable in %q")
	case !slices.Contains(vars, p.variable):
		return part{}, syntaxError(p.variable, "unknown variable %q (available: "+strings.Join(vars, ", ")+")")
	}
	for _, name := range names[1:] {
		if name == "" {
			return part{}, syntaxError(expr, "empty filter name in %q")
		}
		f, ok := filters[name]
		if !ok {
			return part{}, syntaxError(name, "unknown filter %q (available: lower, sanitize)")
		}
		p.filters = append(p.filters, f)
	}
	return p, nil
}

// Render replaces each expression by its variable's value with the filters
// applied from left to right. It cannot fail: Parse already checked that
// every variable is one the caller provides.
func (t *Template) Render(values map[string]string) string {
	var b strings.Builder
	for _, p := range t.parts {
		if p.variable == "" {
			b.WriteString(p.literal)
			continue
		}
		v := values[p.variable]
		for _, f := range p.filters {
			v = f(v)
		}
		b.WriteString(v)
	}
	return b.String()
}
