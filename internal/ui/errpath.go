package ui

import (
	"path/filepath"
	"strings"
)

// displayError carries an error whose message was rewritten for display. The
// original stays reachable through Unwrap, so errors.Is/As keep working.
type displayError struct {
	msg string
	err error
}

func (e *displayError) Error() string { return e.msg }
func (e *displayError) Unwrap() error { return e.err }

// joinedError is a rewritten errors.Join tree; it still unwraps to the
// rewritten children, whose chains reach the original errors. Its message
// joins the rewritten children with newlines, as errors.Join does.
type joinedError struct {
	kids []error
}

func (j *joinedError) Error() string {
	msgs := make([]string, len(j.kids))
	for i, k := range j.kids {
		msgs[i] = k.Error()
	}
	return strings.Join(msgs, "\n")
}

func (j *joinedError) Unwrap() []error { return j.kids }

// WrapError returns err with every leaf of its errors.Join tree rewritten by
// DisplayText, so that error messages show paths like the result lists do
// (targets under home as ~/..., sources under the repository root relative to
// it). The errors.Join structure and the wrapped chain are preserved.
//
// The rewrite works on the message text: paths reach messages through many
// layers (resolve, state, apply and fs errors format them with %s/%w), and
// typed errors carrying every path would touch each of those sites. Only the
// two known prefixes, Home and Root, are rewritten, and only at path
// boundaries.
func (p *Printer) WrapError(err error) error {
	if err == nil {
		return nil
	}
	if m, ok := err.(interface{ Unwrap() []error }); ok {
		kids := m.Unwrap()
		out := make([]error, len(kids))
		for i, k := range kids {
			out[i] = p.WrapError(k)
		}
		return &joinedError{kids: out}
	}
	msg := err.Error()
	if shown := p.DisplayText(msg); shown != msg {
		return &displayError{msg: shown, err: err}
	}
	return err
}

// DisplayText rewrites absolute paths inside free text: a path under Root
// becomes relative to Root, and a remaining one under Home gets "~".
func (p *Printer) DisplayText(s string) string {
	return rewritePaths(s, p.Home, p.Root, hostOS)
}

// rewritePaths is the pure core of DisplayText. A prefix only matches at a
// path boundary on both sides, so "/home/dev" does not match "/home/developer".
func rewritePaths(s, home, root, goos string) string {
	if home != "" {
		home = filepath.Clean(home)
	}
	if root != "" {
		root = filepath.Clean(root)
	}
	if home == "" && root == "" {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if i == 0 || isLeftBoundary(s[i-1]) {
			if n, repl, ok := matchPrefix(s[i:], home, root, goos); ok {
				b.WriteString(repl)
				i += n
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func isLeftBoundary(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '"', '\'', '`', '(', '[', ':', '=', ',', ';':
		return true
	}
	return false
}

func isRightBoundary(c byte) bool {
	switch c {
	case ' ', '\t', '"', '\'', '`', ')', ']', ':', ',', ';', '\n':
		return true
	}
	return false
}

func isSep(c byte) bool { return c == '/' || c == filepath.Separator }

func hasPrefixOS(s, prefix, goos string) bool {
	if len(s) < len(prefix) {
		return false
	}
	if goos == "windows" || goos == "darwin" {
		return strings.EqualFold(s[:len(prefix)], prefix)
	}
	return s[:len(prefix)] == prefix
}

// matchPrefix reports how many bytes of s a leading root/home path covers and
// what replaces them.
func matchPrefix(s, home, root, goos string) (n int, repl string, ok bool) {
	// Root first: root + separator + rest becomes rest. The root itself is
	// left to the home rule (shown like any other path).
	if root != "" && hasPrefixOS(s, root, goos) {
		rest := s[len(root):]
		switch {
		case len(rest) > 0 && isSep(rest[0]):
			k := 0
			for k < len(rest) && isSep(rest[k]) {
				k++
			}
			if k < len(rest) {
				return len(root) + k, "", true
			}
		case strings.HasSuffix(root, string(filepath.Separator)) && len(rest) > 0:
			return len(root), "", true // root is a filesystem root
		}
	}
	if home != "" && hasPrefixOS(s, home, goos) {
		rest := s[len(home):]
		if len(rest) == 0 || isSep(rest[0]) || isRightBoundary(rest[0]) {
			return len(home), "~", true
		}
		if strings.HasSuffix(home, string(filepath.Separator)) {
			return len(home), "~" + string(filepath.Separator), true
		}
	}
	return 0, "", false
}
