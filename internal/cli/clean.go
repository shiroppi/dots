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

// staleReport loads the state file read-only and classifies it for status
// (spec §8.1). ok is false when the state could not be checked: a broken
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

// staleNotice tells apply's user how many stale items remain (spec §8.2).
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

func newCleanCmd(app *App) *cobra.Command {
	var noConfirm, dryRun bool
	cmd := &cobra.Command{
		Use:     "clean",
		Short:   "Remove links that are no longer in dots.toml",
		GroupID: "core",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.clean(noConfirm, dryRun)
		},
	}
	cmd.Flags().BoolVar(&noConfirm, "no-confirm", false, "Do not ask for confirmation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without changing anything")
	return cmd
}

// clean implements `dots clean` (spec §7): it removes the stale links.
func (a *App) clean(noConfirm, dryRun bool) error {
	res, _, err := a.load()
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

	var rows []ui.CleanRow
	var links []statefile.Stale
	for _, s := range stale {
		switch s.Kind {
		case statefile.StaleLink:
			rows = append(rows, ui.CleanRow{Stale: s, Outcome: ui.CleanRemove})
			links = append(links, s)
		case statefile.StaleChanged:
			rows = append(rows, ui.CleanRow{Stale: s, Outcome: ui.CleanKeep})
		}
	}
	if len(rows) == 0 && (dryRun || len(stale) == 0) {
		fmt.Fprintln(a.Stdout, "No stale links.")
		return nil
	}
	if dryRun {
		pr.CleanRows(rows)
		pr.CleanSummary(rows, true)
		return nil
	}

	// Removing links needs a confirmation; dropping the records of changed or gone
	// targets touches no link and needs none.
	previewed := false
	if len(links) > 0 && !noConfirm {
		if !a.Interactive || a.Ask == nil {
			pr.CleanRows(rows)
			return pr.WrapError(errors.New("removing stale links requires an interactive terminal; pass --no-confirm to remove them without asking (nothing was changed)"))
		}
		pr.CleanRows(rows)
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
		if r.Outcome != ui.CleanRemove {
			continue
		}
		rows[i].Outcome, rows[i].Err = a.removeStale(r.Stale)
	}
	for _, s := range stale {
		st.Remove(s.Record.Target) // below, failed removals are put back
	}
	failed := false
	for _, r := range rows {
		if r.Outcome == ui.CleanFail {
			st.Record(r.Stale.Record) // keep the record of a link that is still there
			failed = true
		}
	}

	shown := rows
	if previewed {
		// The list was printed before the question; show only what differs.
		shown = nil
		for _, r := range rows {
			if r.Outcome == ui.CleanRecheck || r.Outcome == ui.CleanFail {
				shown = append(shown, r)
			}
		}
	}
	pr.CleanRows(shown)
	if len(rows) > 0 {
		pr.CleanSummary(rows, false)
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
// one dots recorded. It returns the outcome and, for CleanFail, the error.
func (a *App) removeStale(s statefile.Stale) (ui.CleanOutcome, error) {
	now, err := statefile.Inspect(a.FS, a.Env.GOOS, s.Record)
	if err != nil {
		return ui.CleanFail, err
	}
	switch now.Kind {
	case statefile.StaleLink:
		if err := a.FS.Remove(s.Record.Target); err != nil {
			return ui.CleanFail, err
		}
		return ui.CleanRemove, nil
	case statefile.StaleGone:
		return ui.CleanRemove, nil // already gone: the goal is reached
	}
	return ui.CleanRecheck, nil
}
