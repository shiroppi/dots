// Package ignore compiles and evaluates the `ignore` patterns of auto rules
// (spec §5.3).
//
// A pattern is matched against the name of each item directly inside the
// source root (auto links only the first level), case-sensitively on every
// OS. It is a plain name in which "*" matches any run of characters,
// including none; no other wildcard exists.
package ignore

import (
	"errors"
	"fmt"
	"strings"
)

// Validate reports why pattern is not an acceptable ignore pattern.
func Validate(pattern string) error {
	switch {
	case pattern == "":
		return errors.New("pattern must not be empty")
	case pattern == "." || pattern == "..":
		return fmt.Errorf("pattern must not be %q", pattern)
	case strings.ContainsAny(pattern, "/\\"):
		return errors.New("pattern must be a single top-level name (auto links only the first level, so path separators are not allowed)")
	case strings.Contains(pattern, "**"):
		return errors.New("\"**\" is not supported; use \"*\"")
	case strings.ContainsAny(pattern, "?[{"):
		return errors.New("only \"*\" is supported as a wildcard")
	}
	return nil
}

// Matcher is a validated set of ignore patterns.
type Matcher struct {
	patterns []string
}

// Compile validates every pattern and returns a Matcher. The error joins the
// problems of all invalid patterns, each naming its index and text.
func Compile(patterns []string) (*Matcher, error) {
	var errs []error
	for i, p := range patterns {
		if err := Validate(p); err != nil {
			errs = append(errs, fmt.Errorf("ignore[%d] %q: %w", i, p, err))
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &Matcher{patterns: append([]string(nil), patterns...)}, nil
}

// Patterns returns a copy of the compiled patterns.
func (m *Matcher) Patterns() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.patterns...)
}

// Match reports whether name, the name of an item directly inside the source
// root, is matched by any pattern. A nil Matcher matches nothing.
func (m *Matcher) Match(name string) bool {
	if m == nil {
		return false
	}
	for _, p := range m.patterns {
		if match(p, name) {
			return true
		}
	}
	return false
}

// match reports whether name matches pattern, where "*" matches any run of
// characters (including none) and everything else matches itself.
func match(pattern, name string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name
	}
	first, last := parts[0], parts[len(parts)-1]
	if !strings.HasPrefix(name, first) {
		return false
	}
	name = name[len(first):]
	for _, mid := range parts[1 : len(parts)-1] {
		i := strings.Index(name, mid)
		if i < 0 {
			return false
		}
		name = name[i+len(mid):]
	}
	return strings.HasSuffix(name, last)
}
