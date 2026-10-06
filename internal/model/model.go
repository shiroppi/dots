// Package model holds the data types exchanged between the resolver
// (configuration -> concrete links), the state evaluator, the planner, and
// the CLI. It has no dependencies on other dots packages.
package model

import "fmt"

// Layer identifies the rule notation a link came from. Higher values win
// (spec §5.4): dots.<os> > dots > auto.<os> > auto.
type Layer int

const (
	LayerAuto Layer = iota + 1
	LayerAutoOS
	LayerDots
	LayerDotsOS
)

// String returns the spec notation name ("auto", "auto.<os>", ...).
func (l Layer) String() string {
	switch l {
	case LayerAuto:
		return "auto"
	case LayerAutoOS:
		return "auto.<os>"
	case LayerDots:
		return "dots"
	case LayerDotsOS:
		return "dots.<os>"
	default:
		return fmt.Sprintf("Layer(%d)", int(l))
	}
}

// Origin describes where in dots.toml a link was declared.
type Origin struct {
	Layer Layer
	// Rule is a human readable locator, e.g. `dots."~/.vimrc"`,
	// `dots.darwin."~/.config/karabiner"`, `auto[0]`, `auto[0].darwin[1]`.
	Rule string
}

func (o Origin) String() string { return o.Rule }

// Entry is one concrete symlink that dots manages after resolution.
type Entry struct {
	// Target is the absolute, cleaned runtime path where the item lives.
	Target string
	// Source is the absolute, cleaned runtime path of the item the symlink
	// must point to (a regular file, a directory, or a symlink item inside
	// the repository; source symlinks are never followed).
	Source string
	Origin Origin
}

// Override records a lower-priority candidate that was discarded because a
// higher-priority rule produced the same target (spec §5.5). doctor and the
// apply Plan display these.
type Override struct {
	Target string
	Winner Origin
	Loser  Entry
}

// Resolution is the output of resolving dots.toml for the current OS.
type Resolution struct {
	// ConfigPath is the absolute path of the dots.toml in use.
	ConfigPath string
	// Root is the repository root (directory containing ConfigPath).
	Root string
	// Entries are unique by Target and sorted lexicographically by Target.
	Entries []Entry
	// Overrides are sorted by Target, then by Loser.Origin.Rule.
	Overrides []Override
}
