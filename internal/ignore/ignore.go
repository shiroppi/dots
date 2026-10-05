// Package ignore compiles and evaluates the `ignore` patterns of auto rules
// (spec §5.3).
//
// Patterns are matched against the slash-separated path relative to the
// source root, case-sensitively on every OS:
//
//   - "README.md" matches only the root-level README.md.
//   - "**/README.md" matches README.md at any depth, including the root.
//   - "*" does not cross "/".
//   - "**" must be a whole path element.
package ignore

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Validate reports why pattern is not an acceptable ignore pattern.
func Validate(pattern string) error {
	if pattern == "" {
		return errors.New("pattern must not be empty")
	}
	if strings.Contains(pattern, "\\") {
		return errors.New("pattern must use \"/\" as the separator (backslash is not allowed)")
	}
	if strings.HasPrefix(pattern, "/") {
		return errors.New("pattern must be relative to the source root (leading \"/\" is not allowed)")
	}
	if strings.HasSuffix(pattern, "/") {
		return errors.New("pattern must not end with \"/\"")
	}
	for _, seg := range strings.Split(pattern, "/") {
		switch {
		case seg == "":
			return errors.New("pattern must not contain empty path elements (\"//\")")
		case seg == "." || seg == "..":
			return fmt.Errorf("pattern must not contain %q path elements", seg)
		case seg != "**" && strings.Contains(seg, "**"):
			return errors.New("\"**\" must be used as a whole path element")
		}
	}
	if !doublestar.ValidatePattern(pattern) {
		return errors.New("malformed glob pattern")
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

// Match reports whether rel, a slash-separated path relative to the source
// root, is matched by any pattern. A nil Matcher matches nothing.
func (m *Matcher) Match(rel string) bool {
	if m == nil {
		return false
	}
	for _, p := range m.patterns {
		if ok, err := doublestar.Match(p, rel); err == nil && ok {
			return true
		}
	}
	return false
}
