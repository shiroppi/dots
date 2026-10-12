package statefile

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/pathx"
)

// StaleKind classifies a stale record by the current state of its target
// (spec §5.4).
type StaleKind int

const (
	// StaleLink: the target is a symlink whose link string equals the
	// recorded one.
	StaleLink StaleKind = iota + 1
	// StaleChanged: the target is a symlink with a different link string,
	// or a regular file or directory (or any other non-link item).
	StaleChanged
	// StaleGone: the target does not exist.
	StaleGone
)

func (k StaleKind) String() string {
	switch k {
	case StaleLink:
		return "StaleLink"
	case StaleChanged:
		return "StaleChanged"
	case StaleGone:
		return "StaleGone"
	default:
		return fmt.Sprintf("StaleKind(%d)", int(k))
	}
}

// Stale is a recorded link that the current configuration no longer
// declares.
type Stale struct {
	Record Link
	Kind   StaleKind
	// Current is the raw link string found at the target when it is a
	// symlink (empty otherwise).
	Current string
}

// Classify returns the stale records of links in input order. A record is
// stale when its repository equals repoRoot and its target is not among
// current (the targets of the current configuration, overridden rules
// included). Records of other repositories are never stale. Targets and
// repositories compare with the rules of goos (case-insensitive on windows
// and darwin). The target is inspected with Lstat and Readlink only; links
// are never followed. On windows, "/" and "\" are treated alike when link
// strings are compared.
func Classify(fsys fs.Manager, goos string, links []Link, repoRoot string, current []string) ([]Stale, error) {
	cur := make(map[string]struct{}, len(current))
	for _, t := range current {
		cur[pathx.Key(goos, t)] = struct{}{}
	}
	root := pathx.Key(goos, repoRoot)

	var out []Stale
	for _, l := range links {
		if pathx.Key(goos, l.Repository) != root {
			continue
		}
		if _, ok := cur[pathx.Key(goos, l.Target)]; ok {
			continue
		}
		s, err := Inspect(fsys, goos, l)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// Inspect classifies l by the current state of its target, with Lstat and
// Readlink only. It does not check whether l is stale; clean uses it to
// re-check a record right before acting.
func Inspect(fsys fs.Manager, goos string, l Link) (Stale, error) {
	s := Stale{Record: l}
	info, err := fsys.Lstat(l.Target)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink == 0 {
			s.Kind = StaleChanged
			break
		}
		text, err := fsys.Readlink(l.Target)
		if err != nil {
			return s, fmt.Errorf("cannot read link %s: %w", l.Target, err)
		}
		s.Current = text
		if sameLinkText(goos, text, l.Link) {
			s.Kind = StaleLink
		} else {
			s.Kind = StaleChanged
		}
	case isGone(err):
		s.Kind = StaleGone
	default:
		return s, fmt.Errorf("cannot inspect %s: %w", l.Target, err)
	}
	return s, nil
}

func sameLinkText(goos, a, b string) bool {
	if goos == "windows" {
		a, b = strings.ReplaceAll(a, "/", `\`), strings.ReplaceAll(b, "/", `\`)
	}
	return a == b
}

// isGone reports a missing path; ENOTDIR (a path component is a file) counts
// as missing too.
func isGone(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}
