// Package pathx validates and resolves the path strings written in dots.toml
// (spec §4).
//
// Configuration paths are written with "/" separators on every OS and are
// checked with purely string-based rules so the verdict does not depend on the
// host OS. Only the final conversion to runtime paths (Target.Resolve,
// ResolveSource) uses the host's filepath conventions.
package pathx

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// envPercent matches Windows style %VAR% references.
var envPercent = regexp.MustCompile(`%[^%/]+%`)

// CheckSyntax rejects the forms that are forbidden for every configuration
// path (targets and sources): empty strings, control characters, backslashes,
// absolute paths (Unix root, drive letters, UNC) and environment variable
// syntax ($VAR, ${VAR}, %VAR%).
func CheckSyntax(s string) error {
	switch {
	case s == "":
		return errors.New("path must not be empty")
	case strings.ContainsAny(s, "\\"):
		return fmt.Errorf("path %q must use \"/\" as the separator (backslash is not allowed)", s)
	case strings.HasPrefix(s, "//"):
		return fmt.Errorf("path %q is a UNC path; absolute paths are not allowed", s)
	case strings.HasPrefix(s, "/"):
		return fmt.Errorf("path %q is absolute; use a path relative to the repository root or start with ~/", s)
	case hasDrive(s):
		return fmt.Errorf("path %q has a drive letter; absolute paths are not allowed", s)
	case strings.Contains(s, "$"):
		return fmt.Errorf("path %q uses environment variable syntax (\"$\"), which is not supported", s)
	case envPercent.MatchString(s):
		return fmt.Errorf("path %q uses environment variable syntax (%%VAR%%), which is not supported", s)
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("path %q contains a control character", s)
		}
	}
	return nil
}

func hasDrive(s string) bool {
	if len(s) < 2 || s[1] != ':' {
		return false
	}
	c := s[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// cleanRel cleans a slash-separated relative path. The result is "" when the
// path refers to the base itself. Paths that escape the base via ".." fail.
func cleanRel(s string) (string, error) {
	c := path.Clean(s)
	if c == "." {
		return "", nil
	}
	if c == ".." || strings.HasPrefix(c, "../") {
		return "", fmt.Errorf("path %q escapes its base directory via \"..\"", s)
	}
	return c, nil
}

// Target is a parsed, validated target string.
type Target struct {
	// Home reports that the path is relative to the home directory
	// ("~" or "~/..."); otherwise it is relative to the repository root.
	Home bool
	// Rel is the cleaned slash-separated path below the base. It is "" when
	// the target is the base itself (the home directory or the repository
	// root) and never starts with "..".
	Rel string
}

// IsBase reports whether the target is the home directory or repository root
// itself.
func (t Target) IsBase() bool { return t.Rel == "" }

// BaseName describes the base the target is relative to, for messages.
func (t Target) BaseName() string {
	if t.Home {
		return "the home directory"
	}
	return "the repository root"
}

// ParseTarget validates a target string: "~" and "~/..." are relative to the
// home directory, everything else is relative to the repository root. ~user
// is rejected, as are paths escaping their base through "..".
func ParseTarget(s string) (Target, error) {
	if err := CheckSyntax(s); err != nil {
		return Target{}, err
	}
	if strings.HasPrefix(s, "~") {
		if s != "~" && !strings.HasPrefix(s, "~/") {
			return Target{}, fmt.Errorf("path %q: \"~user\" is not supported; only \"~\" and \"~/...\" are allowed", s)
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(s, "~"), "/")
		if rest == "" {
			return Target{Home: true}, nil
		}
		rel, err := cleanRel(rest)
		if err != nil {
			return Target{}, fmt.Errorf("path %q escapes the home directory via \"..\"", s)
		}
		return Target{Home: true, Rel: rel}, nil
	}
	rel, err := cleanRel(s)
	if err != nil {
		return Target{}, fmt.Errorf("path %q escapes the repository root via \"..\"", s)
	}
	return Target{Rel: rel}, nil
}

// Resolve converts the target to an absolute runtime path using the host's
// filepath conventions. home is required for "~" targets.
func (t Target) Resolve(root, home string) (string, error) {
	base := root
	if t.Home {
		if home == "" || !filepath.IsAbs(home) {
			return "", fmt.Errorf("cannot expand \"~\": home directory %q is not an absolute path", home)
		}
		base = home
	}
	return filepath.Join(base, filepath.FromSlash(t.Rel)), nil
}

// ParseSource validates a source string and returns its cleaned
// slash-separated path relative to the repository root. "~" is not allowed
// and the repository root itself is not a valid source.
func ParseSource(s string) (string, error) {
	if err := CheckSyntax(s); err != nil {
		return "", err
	}
	if strings.HasPrefix(s, "~") {
		return "", fmt.Errorf("path %q: sources are relative to the repository root; \"~\" is not allowed", s)
	}
	rel, err := cleanRel(s)
	if err != nil {
		return "", fmt.Errorf("path %q escapes the repository root via \"..\"", s)
	}
	if rel == "" {
		return "", fmt.Errorf("path %q is the repository root itself, which is not a valid source", s)
	}
	return rel, nil
}

// ResolveSource converts a cleaned relative source (from ParseSource) to an
// absolute runtime path below root.
func ResolveSource(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}

// FoldsCase reports whether paths are compared case-insensitively on goos
// (Windows and macOS default file systems).
func FoldsCase(goos string) bool { return goos == "windows" || goos == "darwin" }

// Key returns the normalised comparison key of a runtime path: cleaned, and
// lower-cased when goos compares paths case-insensitively.
func Key(goos, p string) string {
	p = filepath.Clean(p)
	if FoldsCase(goos) {
		p = strings.ToLower(p)
	}
	return p
}

// Contains reports whether child is strictly below parent (runtime paths).
func Contains(goos, parent, child string) bool {
	p, c := Key(goos, parent), Key(goos, child)
	if p == c {
		return false
	}
	if !strings.HasSuffix(p, string(filepath.Separator)) {
		p += string(filepath.Separator)
	}
	return strings.HasPrefix(c, p)
}

// Overlaps reports whether a and b are equal or one contains the other.
func Overlaps(goos, a, b string) bool {
	return Key(goos, a) == Key(goos, b) || Contains(goos, a, b) || Contains(goos, b, a)
}
