package apply

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/shiroppi/dots/internal/fs"
)

// Decision is the answer to a conflict prompt (spec §6).
type Decision int

const (
	// Yes backs up and replaces this target.
	Yes Decision = iota + 1
	// YesToAll backs up and replaces this and every later conflict.
	YesToAll
	// No skips this target.
	No
	// NoToAll skips this and every later conflict.
	NoToAll
)

func (d Decision) String() string {
	switch d {
	case Yes:
		return "Yes"
	case YesToAll:
		return "YesToAll"
	case No:
		return "No"
	case NoToAll:
		return "NoToAll"
	default:
		return fmt.Sprintf("Decision(%d)", int(d))
	}
}

// Prompter asks the user how to resolve a conflict. A nil Prompter in Options
// means there is no TTY.
type Prompter interface {
	Confirm(item Item) (Decision, error)
}

// Replacer archives a conflicting target and swaps in the new link. It is
// implemented by *backup.Store: it archives target, moves it aside in the same
// parent directory, calls place, then deletes the aside on success or restores
// it on failure.
type Replacer interface {
	Replace(target string, place func() error) (archivePath string, err error)
}

// Options configures Execute.
type Options struct {
	Prompter Prompter
	Replacer Replacer
}

// Outcome is the result of one item in a Report.
type Outcome int

const (
	// NotProcessed: apply stopped before reaching the item.
	NotProcessed Outcome = iota
	// Created: a new link was created.
	Created
	// AlreadyOK: the target already was a valid link.
	AlreadyOK
	// Replaced: the target was archived and replaced.
	Replaced
	// SkippedByUser: the user declined to replace the conflicting target.
	SkippedByUser
	// Failed: an unexpected error occurred on this item.
	Failed
)

func (o Outcome) String() string {
	switch o {
	case NotProcessed:
		return "not processed"
	case Created:
		return "created"
	case AlreadyOK:
		return "already ok"
	case Replaced:
		return "replaced"
	case SkippedByUser:
		return "skipped by user"
	case Failed:
		return "failed"
	default:
		return fmt.Sprintf("Outcome(%d)", int(o))
	}
}

// ItemReport is the outcome of one planned item.
type ItemReport struct {
	Item    Item
	Outcome Outcome
	// ArchivePath is the backup location when Outcome is Replaced (it may
	// also be set on Failed when the archive was made before the failure).
	ArchivePath string
	// Err is set when Outcome is Failed.
	Err error
}

// Report lists the outcome of every plan item, in plan order.
type Report struct {
	Items []ItemReport
}

// Count returns how many items have outcome o.
func (r *Report) Count(o Outcome) int {
	n := 0
	for _, it := range r.Items {
		if it.Outcome == o {
			n++
		}
	}
	return n
}

// Convenience counters.
func (r *Report) Created() int       { return r.Count(Created) }
func (r *Report) AlreadyOK() int     { return r.Count(AlreadyOK) }
func (r *Report) Replaced() int      { return r.Count(Replaced) }
func (r *Report) SkippedByUser() int { return r.Count(SkippedByUser) }
func (r *Report) Failed() int        { return r.Count(Failed) }
func (r *Report) NotProcessed() int  { return r.Count(NotProcessed) }

// PartialError is returned with a Report when apply stopped midway. Changes
// already made are not rolled back.
type PartialError struct {
	// Target is the failed target; empty when the stop was caused by the
	// prompter rather than by an I/O failure on an item.
	Target string
	Err    error
	// Done is the number of items fully handled; Remaining the number that
	// were not processed (including the failed one only if it was not
	// attempted, i.e. prompt errors).
	Done      int
	Remaining int
}

func (e *PartialError) Error() string {
	if e.Target != "" {
		return fmt.Sprintf("apply stopped at %s after %d item(s): %v (%d item(s) not processed; completed changes were kept)", e.Target, e.Done, e.Err, e.Remaining)
	}
	return fmt.Sprintf("apply stopped after %d item(s): %v (%d item(s) not processed; completed changes were kept)", e.Done, e.Err, e.Remaining)
}

func (e *PartialError) Unwrap() error { return e.Err }

// PrivilegeError reports that Windows refused to create a symlink.
type PrivilegeError struct{ Err error }

func (e *PrivilegeError) Error() string {
	return fmt.Sprintf("cannot create symbolic link: %v; enable Developer Mode (Settings > System > For developers) or grant the SeCreateSymbolicLinkPrivilege privilege. dots never falls back to copies, junctions, or absolute links", e.Err)
}

func (e *PrivilegeError) Unwrap() error { return e.Err }

func wrapSymlinkErr(err error) error {
	var pe *PrivilegeError
	if err != nil && !errors.As(err, &pe) && isPrivilegeError(err) {
		return &PrivilegeError{Err: err}
	}
	return err
}

// Execute applies plan. Validation errors and conflicts without a Prompter
// are rejected before any change (the returned Report is then nil). An
// unexpected I/O error stops processing without rollback; a Report and a
// *PartialError are returned together.
func Execute(fsys fs.Manager, plan *Plan, opts Options) (*Report, error) {
	if plan.HasErrors() {
		return nil, fmt.Errorf("validation failed, nothing was changed: %w", errors.Join(plan.Errors()...))
	}
	if n := plan.Conflicts(); n > 0 {
		if opts.Prompter == nil {
			return nil, fmt.Errorf("%d conflicting target(s) require confirmation, but conflicts require an interactive terminal; run `dots apply --dry-run` to inspect (nothing was changed)", n)
		}
		if opts.Replacer == nil {
			return nil, errors.New("conflicts cannot be replaced: no backup replacer is configured (nothing was changed)")
		}
	}

	rep := &Report{Items: make([]ItemReport, len(plan.Items))}
	for i, it := range plan.Items {
		rep.Items[i] = ItemReport{Item: it, Outcome: NotProcessed}
	}

	var sticky Decision
	for i, it := range plan.Items {
		ir := &rep.Items[i]
		stop := func(target string, err error) (*Report, error) {
			remaining := 0
			for _, x := range rep.Items {
				if x.Outcome == NotProcessed {
					remaining++
				}
			}
			return rep, &PartialError{Target: target, Err: err, Done: i, Remaining: remaining}
		}

		switch it.Action {
		case ActionSkip:
			ir.Outcome = AlreadyOK
		case ActionCreate:
			if err := place(fsys, it); err != nil {
				ir.Outcome, ir.Err = Failed, err
				return stop(it.Target(), err)
			}
			ir.Outcome = Created
		case ActionReplace:
			d := sticky
			if d == 0 {
				var err error
				d, err = opts.Prompter.Confirm(it)
				if err != nil {
					return stop("", fmt.Errorf("confirmation for %s failed: %w", it.Target(), err))
				}
			}
			switch d {
			case YesToAll:
				sticky = YesToAll
				d = Yes
			case NoToAll:
				sticky = NoToAll
				d = No
			case Yes, No:
			default:
				return stop("", fmt.Errorf("confirmation for %s returned unknown decision %v", it.Target(), d))
			}
			if d == No {
				ir.Outcome = SkippedByUser
				continue
			}
			archive, err := opts.Replacer.Replace(it.Target(), func() error { return place(fsys, it) })
			ir.ArchivePath = archive
			if err != nil {
				err = wrapSymlinkErr(err)
				ir.Outcome, ir.Err = Failed, err
				return stop(it.Target(), err)
			}
			ir.Outcome = Replaced
		default:
			return stop(it.Target(), fmt.Errorf("unexpected action %v", it.Action))
		}
	}
	return rep, nil
}

// place creates the parent directories and the relative symlink.
func place(fsys fs.Manager, it Item) error {
	target := it.Target()
	if err := fsys.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("cannot create parent directory of %s: %w", target, err)
	}
	if err := fsys.Symlink(it.Result.Link, target); err != nil {
		return wrapSymlinkErr(fmt.Errorf("cannot create link %s: %w", target, err))
	}
	return nil
}
