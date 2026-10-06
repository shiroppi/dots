// Package apply builds and executes link plans (spec §6, §8). Planning is
// side-effect free so doctor and apply --dry-run share it with apply.
package apply

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/model"
	"github.com/shiroppi/dots/internal/state"
)

// Action is what apply intends to do for one entry.
type Action int

const (
	// ActionCreate: the target does not exist; create parents and the link.
	ActionCreate Action = iota + 1
	// ActionSkip: the target already is a valid link to the source.
	ActionSkip
	// ActionReplace: the target conflicts (InvalidLink or FileOrDir); a
	// backup-and-replace needs confirmation.
	ActionReplace
	// ActionError: the entry failed validation; apply must not start.
	ActionError
)

func (a Action) String() string {
	switch a {
	case ActionCreate:
		return "create"
	case ActionSkip:
		return "skip"
	case ActionReplace:
		return "replace"
	case ActionError:
		return "error"
	default:
		return fmt.Sprintf("Action(%d)", int(a))
	}
}

// Item is one planned entry.
type Item struct {
	Result state.Result
	Action Action
}

// Target is the target path of the item.
func (i Item) Target() string { return i.Result.Entry.Target }

// Plan is the ordered list of items plus the overrides to display.
type Plan struct {
	// Items are sorted lexicographically by normalized target path.
	Items     []Item
	Overrides []model.Override
}

// HasErrors reports whether any item failed validation.
func (p *Plan) HasErrors() bool {
	for _, it := range p.Items {
		if it.Action == ActionError {
			return true
		}
	}
	return false
}

// Conflicts counts the items that need a replace confirmation.
func (p *Plan) Conflicts() int {
	n := 0
	for _, it := range p.Items {
		if it.Action == ActionReplace {
			n++
		}
	}
	return n
}

// Errors returns one error per invalid item, each naming the target and the
// rule that declared it.
func (p *Plan) Errors() []error {
	var errs []error
	for _, it := range p.Items {
		if it.Action != ActionError {
			continue
		}
		e := it.Result.Entry
		errs = append(errs, fmt.Errorf("%s (%s): %w", e.Target, e.Origin, it.Result.Err))
	}
	return errs
}

// BuildPlan evaluates every entry of res. It never modifies the file system.
func BuildPlan(fsys fs.Manager, res *model.Resolution) *Plan {
	p := &Plan{}
	if res == nil {
		return p
	}
	p.Overrides = append(p.Overrides, res.Overrides...)
	p.Items = make([]Item, 0, len(res.Entries))
	for _, e := range res.Entries {
		r := state.Evaluate(fsys, e)
		p.Items = append(p.Items, Item{Result: r, Action: actionFor(r)})
	}
	sort.SliceStable(p.Items, func(i, j int) bool {
		return filepath.Clean(p.Items[i].Result.Entry.Target) < filepath.Clean(p.Items[j].Result.Entry.Target)
	})
	return p
}

func actionFor(r state.Result) Action {
	if r.Err != nil {
		return ActionError
	}
	switch r.Status {
	case state.NotExist:
		return ActionCreate
	case state.ValidLink:
		return ActionSkip
	default:
		return ActionReplace
	}
}
