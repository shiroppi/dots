package ui

import (
	"github.com/shiroppi/dots/internal/model"
	"github.com/shiroppi/dots/internal/statefile"
)

// Status symbol of a removed link (spec §9).
const symRemove = "-"

// StateReport is the state file part of the status report (spec §8.1).
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

// CountStale returns the number of stale records that status and apply
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

// staleItems adds the stale rows of a status report to l and returns how many
// there were. Like the other status rows, they show their source as l.mode
// says.
func (p *Printer) staleItems(l *itemList, stale []statefile.Stale) int {
	n := 0
	for _, s := range stale {
		e := model.Entry{Target: s.Record.Target, Source: s.Record.Source}
		switch s.Kind {
		case statefile.StaleLink:
			l.item(toneYellow, symProblem, "stale", e, false)
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

// StaleHint prints the closing line of status or apply. remove is the object
// of "to remove" ("stale links" in status, "them" in apply).
func (p *Printer) StaleHint(remove string) {
	p.printf("Run \"dots clean\" to remove %s.\n", remove)
}

// StaleCount prints "N stale link(s) found." (apply, after the summary).
func (p *Printer) StaleCount(n int) {
	p.printf("%d stale link(s) found.\n", n)
}

// CleanOutcome is what happened to, or is planned for, one stale record.
type CleanOutcome int

const (
	// CleanRemove: the symlink is removed (or will be).
	CleanRemove CleanOutcome = iota + 1
	// CleanKeep: the target changed since dots created it; it is left alone.
	CleanKeep
	// CleanRecheck: the target differed from the classification just before
	// the removal; it is left alone.
	CleanRecheck
	// CleanFail: removing the symlink failed.
	CleanFail
)

// CleanRow is one line of the clean output.
type CleanRow struct {
	Stale   statefile.Stale
	Outcome CleanOutcome
	Err     error // CleanFail only
}

// CleanRows prints the rows of clean (spec §7.4).
func (p *Printer) CleanRows(rows []CleanRow) {
	if len(rows) == 0 {
		return
	}
	p.header()
	l := p.newList(SourceSimple)
	for _, r := range rows {
		e := model.Entry{Target: r.Stale.Record.Target, Source: r.Stale.Record.Source}
		switch r.Outcome {
		case CleanRemove:
			l.item(toneRed, symRemove, "stale", e, true)
		case CleanKeep:
			l.item(toneYellow, symProblem, "skipped", e, false)
			l.detail(toneDim, "changed since dots created it; dots will not remove it")
		case CleanRecheck:
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

// CleanSummary prints "2 removed, 1 skipped"; with dryRun the wording says
// what would happen.
func (p *Printer) CleanSummary(rows []CleanRow, dryRun bool) {
	var nRemove, nSkip, nFail int
	for _, r := range rows {
		switch r.Outcome {
		case CleanRemove:
			nRemove++
		case CleanKeep, CleanRecheck:
			nSkip++
		default:
			nFail++
		}
	}
	if dryRun {
		p.printf("%s\n", summary(count{nRemove, "to remove"}, count{nSkip, "to skip"}))
		p.printf("%s\n", p.paint(toneDim, "dry run: nothing was changed"))
		return
	}
	p.printf("%s\n", summary(count{nRemove, "removed"}, count{nSkip, "skipped"}, count{nFail, "failed"}))
}
