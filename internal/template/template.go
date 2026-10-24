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
// ignored. {{ is a literal { and }} a literal }, read from left to right, so
// {{{name}}} is a literal {, the expression {name} and a literal }.
func Parse(src string, vars []string) (*Template, error) {
	t := &Template{}
	var lit strings.Builder // the current literal text, with braces unescaped
	start := 0              // where the current literal text starts in src
	for i := 0; i < len(src); i++ {
		switch c := src[i]; {
		case (c == '{' || c == '}') && i+1 < len(src) && src[i+1] == c:
			lit.WriteByte(c)
			i++
		case c == '}':
			return nil, syntaxError(src[start:i+1], "} without { in %q")
		case c == '{':
			end := strings.IndexAny(src[i+1:], "{}")
			if end < 0 || src[i+1+end] == '{' {
				stop := len(src)
				if end >= 0 {
					stop = i + 1 + end
				}
				return nil, syntaxError(src[i:stop], "{ without } in %q")
			}
			end += i + 1
			if lit.Len() > 0 {
				t.parts = append(t.parts, part{literal: lit.String()})
				lit.Reset()
			}
			p, err := parseExpr(src[i:end+1], vars)
			if err != nil {
				return nil, err
			}
			t.parts = append(t.parts, p)
			i = end
			start = end + 1
		default:
			lit.WriteByte(c)
		}
	}
	if lit.Len() > 0 {
		t.parts = append(t.parts, part{literal: lit.String()})
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
