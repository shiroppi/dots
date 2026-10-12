package ui

import (
	"github.com/shiroppi/dots/internal/model"
	"github.com/shiroppi/dots/internal/statefile"
)

// Status symbol of an unlinked item (spec §13).
const symUnlink = "-"

// StateReport is the state file part of the doctor report (spec §12.1).
type StateReport struct {
	// Path is the state file, shown when Err is set.
	Path string
	// Err is a broken state file (spec §5.5); it is reported as an error.
	Err error
	// Stale are the classified stale records. StaleGone is not shown.
	Stale []statefile.Stale
}

// shown reports whether a stale record appears in the output.
func shown(s statefile.Stale) bool {
	return s.Kind == statefile.StaleLink || s.Kind == statefile.StaleChanged
}

// CountStale returns the number of stale records that doctor and apply
// report: the links and the changed items, not the gone ones.
func CountStale(stale []statefile.Stale) int {
	n := 0
	for _, s := range stale {
		if shown(s) {
			n++
		}
	}
	return n
}

// staleItems adds the stale rows of a doctor report to l and returns how
// many there were.
func (p *Printer) staleItems(l *itemList, stale []statefile.Stale) int {
	n := 0
	for _, s := range stale {
		e := model.Entry{Target: s.Record.Target, Source: s.Record.Source}
		switch s.Kind {
		case statefile.StaleLink:
			l.item(toneYellow, symProblem, "stale", e, true)
			l.detail(toneDim, "not in dots.toml; created by dots on "+s.Record.RecordedAt.UTC().Format("2006-01-02"))
		case statefile.StaleChanged:
			l.item(toneYellow, symProblem, "stale", e, false)
			l.detail(toneDim, "changed since dots created it; dots will not remove it")
		default:
			continue
		}
		n++
	}
	return n
}

// StaleHint prints the closing line of doctor or apply. remove is the object
// of "to remove" ("stale links" in doctor, "them" in apply).
func (p *Printer) StaleHint(remove string) {
	p.printf("Run \"dots unlink --stale\" to remove %s.\n", remove)
}

// StaleCount prints "N stale link(s) found." (apply, after the summary).
func (p *Printer) StaleCount(n int) {
	p.printf("%d stale link(s) found.\n", n)
}

// UnlinkOutcome is what happened to, or is planned for, one stale record.
type UnlinkOutcome int

const (
	// UnlinkRemove: the symlink is removed (or will be).
	UnlinkRemove UnlinkOutcome = iota + 1
	// UnlinkKeep: the target changed since dots created it; it is left alone.
	UnlinkKeep
	// UnlinkRecheck: the target differed from the classification just before
	// the removal; it is left alone.
	UnlinkRecheck
	// UnlinkFail: removing the symlink failed.
	UnlinkFail
)

// UnlinkRow is one line of the unlink --stale output.
type UnlinkRow struct {
	Stale   statefile.Stale
	Outcome UnlinkOutcome
	Err     error // UnlinkFail only
}

// UnlinkRows prints the rows of unlink --stale (spec §10.6).
func (p *Printer) UnlinkRows(rows []UnlinkRow) {
	if len(rows) == 0 {
		return
	}
	p.header()
	l := p.newList()
	for _, r := range rows {
		e := model.Entry{Target: r.Stale.Record.Target, Source: r.Stale.Record.Source}
		switch r.Outcome {
		case UnlinkRemove:
			l.item(toneRed, symUnlink, "", e, true)
		case UnlinkKeep:
			l.item(toneYellow, symProblem, "skipped", e, false)
			l.detail(toneDim, "changed since dots created it; dots will not remove it")
		case UnlinkRecheck:
			l.item(toneYellow, symProblem, "skipped", e, false)
			l.detail(toneDim, "changed since it was checked; left untouched")
		default:
			l.item(toneYellow, symProblem, "failed", e, false)
			l.errorDetails(r.Err)
		}
	}
	l.flush()
	p.printf("\n")
}

// UnlinkSummary prints "2 unlinked, 1 skipped"; with dryRun the wording says
// what would happen.
func (p *Printer) UnlinkSummary(rows []UnlinkRow, dryRun bool) {
	var nRemove, nSkip, nFail int
	for _, r := range rows {
		switch r.Outcome {
		case UnlinkRemove:
			nRemove++
		case UnlinkKeep, UnlinkRecheck:
			nSkip++
		default:
			nFail++
		}
	}
	if dryRun {
		p.printf("%s\n", summary(count{nRemove, "to unlink"}, count{nSkip, "to skip"}))
		p.printf("%s\n", p.paint(toneDim, "dry run: nothing was changed"))
		return
	}
	p.printf("%s\n", summary(count{nRemove, "unlinked"}, count{nSkip, "skipped"}, count{nFail, "failed"}))
}
