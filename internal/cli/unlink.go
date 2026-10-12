package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/model"
	"github.com/shiroppi/dots/internal/statefile"
	"github.com/shiroppi/dots/internal/ui"
)

// classifyStale returns the stale records of st for the repository of res.
func (a *App) classifyStale(st *statefile.State, res *model.Resolution) ([]statefile.Stale, error) {
	return statefile.Classify(a.FS, a.Env.GOOS, st.Links, res.Root, apply.CurrentTargets(res))
}

// staleReport loads the state file read-only and classifies it for doctor
// (spec §12.1). ok is false when the state could not be checked: a broken
// file is part of the report, anything else is a warning on stderr. A state
// written by a newer dots is warned about and treated as empty.
func (a *App) staleReport(res *model.Resolution) (sr ui.StateReport, ok bool) {
	pr := a.printerFor(res)
	warn := func(err error) {
		fmt.Fprintf(a.Stderr, "warning: %s\n", pr.DisplayText(err.Error()))
	}
	store, err := a.stateStore()
	if err != nil {
		warn(fmt.Errorf("stale links were not checked: %w", err))
		return sr, false
	}
	sr.Path = store.Path
	st, err := store.Load()
	switch {
	case errors.Is(err, statefile.ErrBroken):
		sr.Err = err
		return sr, true
	case errors.Is(err, statefile.ErrNewerVersion):
		warn(fmt.Errorf("%w; stale links were not checked", err))
		return sr, true
	case err != nil:
		warn(fmt.Errorf("stale links were not checked: %w", err))
		return sr, false
	}
	sr.Stale, err = a.classifyStale(st, res)
	if err != nil {
		warn(fmt.Errorf("stale links were not checked: %w", err))
		return sr, false
	}
	return sr, true
}

// staleNotice tells apply's user how many stale items remain (spec §12.2).
// The error is a warning: stale detection failing never undoes the apply.
func (a *App) staleNotice(pr *ui.Printer, st *statefile.State, res *model.Resolution) error {
	stale, err := a.classifyStale(st, res)
	if err != nil {
		return fmt.Errorf("warning: stale links were not checked: %w", err)
	}
	if n := ui.CountStale(stale); n > 0 {
		fmt.Fprintln(a.Stdout)
		pr.StaleCount(n)
		pr.StaleHint("them")
	}
	return nil
}

func newUnlinkCmd(app *App) *cobra.Command {
	var stale, yes, dryRun bool
	cmd := &cobra.Command{
		Use:     "unlink --stale",
		Short:   "Remove links that are no longer in dots.toml",
		GroupID: "core",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case len(args) > 0 && stale:
				return usageError{errors.New("<target> and --stale cannot be combined")}
			case len(args) > 0:
				return errors.New("unlink <target> is not implemented yet; use --stale")
			case !stale:
				return usageError{errors.New("nothing to unlink; use --stale")}
			}
			return app.unlinkStale(yes, dryRun)
		},
	}
	cmd.Flags().BoolVar(&stale, "stale", false, "Remove links dots created that dots.toml no longer declares")
	cmd.Flags().BoolVar(&yes, "yes", false, "Do not ask for confirmation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without changing anything")
	return cmd
}

// unlinkStale implements `dots unlink --stale` (spec §10.4-§10.6).
func (a *App) unlinkStale(yes, dryRun bool) error {
	res, err := a.load()
	if err != nil {
		return err
	}
	pr := a.printerFor(res)
	store, err := a.stateStore()
	if err != nil {
		return pr.WrapError(err)
	}
	st, err := store.Load()
	switch {
	case errors.Is(err, statefile.ErrNewerVersion):
		return pr.WrapError(fmt.Errorf("%w; refusing to change anything", err))
	case errors.Is(err, statefile.ErrBroken):
		return pr.WrapError(fmt.Errorf("%w; nothing was removed", err))
	case err != nil:
		return pr.WrapError(err)
	}
	stale, err := a.classifyStale(st, res)
	if err != nil {
		return pr.WrapError(err)
	}

	var rows []ui.UnlinkRow
	var links []statefile.Stale
	for _, s := range stale {
		switch s.Kind {
		case statefile.StaleLink:
			rows = append(rows, ui.UnlinkRow{Stale: s, Outcome: ui.UnlinkRemove})
			links = append(links, s)
		case statefile.StaleChanged:
			rows = append(rows, ui.UnlinkRow{Stale: s, Outcome: ui.UnlinkKeep})
		}
	}
	if len(rows) == 0 && (dryRun || len(stale) == 0) {
		fmt.Fprintln(a.Stdout, "No stale links.")
		return nil
	}
	if dryRun {
		pr.UnlinkRows(rows)
		pr.UnlinkSummary(rows, true)
		return nil
	}

	// Removing links needs a yes; dropping the records of changed or gone
	// targets touches no link and needs none.
	previewed := false
	if len(links) > 0 && !yes {
		if !a.Interactive || a.Ask == nil {
			pr.UnlinkRows(rows)
			return pr.WrapError(errors.New("removing stale links requires an interactive terminal; pass --yes to confirm (nothing was changed)"))
		}
		pr.UnlinkRows(rows)
		previewed = true
		ans, err := a.Ask(ui.Question{
			Title:   fmt.Sprintf("Remove %d stale link(s)?", len(links)),
			Label:   "stale links",
			Options: []string{"Yes – remove them", "No – keep them"},
			Initial: 1, // No
		})
		if err != nil {
			return err
		}
		if ans.Choice != 0 {
			fmt.Fprintln(a.Stdout, "Nothing was changed.")
			return nil
		}
	}

	for i, r := range rows {
		if r.Outcome != ui.UnlinkRemove {
			continue
		}
		rows[i].Outcome, rows[i].Err = a.removeStale(r.Stale)
	}
	for _, s := range stale {
		st.Remove(s.Record.Target) // below, failed removals are put back
	}
	failed := false
	for _, r := range rows {
		if r.Outcome == ui.UnlinkFail {
			st.Record(r.Stale.Record) // keep the record of a link that is still there
			failed = true
		}
	}

	shown := rows
	if previewed {
		// The list was printed before the question; show only what differs.
		shown = nil
		for _, r := range rows {
			if r.Outcome == ui.UnlinkRecheck || r.Outcome == ui.UnlinkFail {
				shown = append(shown, r)
			}
		}
	}
	pr.UnlinkRows(shown)
	if len(rows) > 0 {
		pr.UnlinkSummary(rows, false)
	}
	if err := store.Save(st); err != nil {
		return pr.WrapError(fmt.Errorf("warning: cannot update state file %s: %w", pr.Display(store.Path), err))
	}
	if failed {
		return silentError{}
	}
	return nil
}

// removeStale re-checks s and removes the symlink only when it still is the
// one dots recorded. It returns the outcome and, for UnlinkFail, the error.
func (a *App) removeStale(s statefile.Stale) (ui.UnlinkOutcome, error) {
	now, err := statefile.Inspect(a.FS, a.Env.GOOS, s.Record)
	if err != nil {
		return ui.UnlinkFail, err
	}
	switch now.Kind {
	case statefile.StaleLink:
		if err := a.FS.Remove(s.Record.Target); err != nil {
			return ui.UnlinkFail, err
		}
		return ui.UnlinkRemove, nil
	case statefile.StaleGone:
		return ui.UnlinkRemove, nil // already gone: the goal is reached
	}
	return ui.UnlinkRecheck, nil
}
