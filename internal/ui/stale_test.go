package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/statefile"
)

func staleRec(target, source string) statefile.Link {
	return statefile.Link{Target: p("home", target), Link: "l", Repository: p("repo"), Source: source,
		RecordedAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
}

func TestStatusStale(t *testing.T) {
	sr := StateReport{Stale: []statefile.Stale{
		{Record: staleRec("tmux", "config/tmux"), Kind: statefile.StaleLink},
		{Record: staleRec("zshrc", "zshrc"), Kind: statefile.StaleChanged},
		{Record: staleRec("gone", "gone"), Kind: statefile.StaleGone},
	}}
	var ok bool
	out := render(func(pr *Printer) { ok = pr.StatusState(&apply.Plan{}, sr, true) })
	if ok {
		t.Error("ok = true, want false")
	}
	checkContains(t, out,
		"  ! stale "+p("home", "tmux")+"   ← config/tmux\n",
		"    not in dots.toml; created by dots on 2026-10-01\n",
		"  ! stale "+p("home", "zshrc")+"  ← zshrc\n",
		"    changed since dots created it; dots will not remove it\n",
		"2 stale\n",
		"Not OK",
		"\nRun \"dots clean\" to remove stale links.\n",
	)
	if strings.Contains(out, "gone") {
		t.Errorf("a gone record is shown:\n%s", out)
	}
	if strings.Contains(out, "\x1b") {
		t.Error("escape sequences without color")
	}
	quiet := render(func(pr *Printer) { pr.StatusState(&apply.Plan{}, sr, false) })
	if strings.Contains(quiet, "←") {
		t.Errorf("status without --verbose shows sources:\n%s", quiet)
	}
}

// Stale rows align their arrows like the other rows of status --verbose.
func TestStatusStaleAlignedArrow(t *testing.T) {
	sr := StateReport{Stale: []statefile.Stale{
		{Record: staleRec("a", "x"), Kind: statefile.StaleLink},
		{Record: staleRec("longer-name", "y"), Kind: statefile.StaleLink},
	}}
	out := render(func(pr *Printer) { pr.StatusState(&apply.Plan{}, sr, true) })
	checkContains(t, out, "  ! stale "+p("home", "a")+"            \u2190 x\n", "  ! stale "+p("home", "longer-name")+"  \u2190 y\n")
}

func TestStatusBrokenState(t *testing.T) {
	sr := StateReport{Path: p("state.json"), Err: errors.New("state file is unreadable: bad")}
	var ok bool
	out := render(func(pr *Printer) { ok = pr.StatusState(&apply.Plan{}, sr, true) })
	if ok {
		t.Error("ok = true, want false")
	}
	checkContains(t, out, "! error "+p("state.json"), "    state file is unreadable: bad", "1 error")
	if strings.Contains(out, "dots clean") {
		t.Errorf("no clean hint without stale links:\n%s", out)
	}
}

func TestCleanRows(t *testing.T) {
	rows := []CleanRow{
		{Stale: statefile.Stale{Record: staleRec("tmux", "config/tmux")}, Outcome: CleanRemove},
		{Stale: statefile.Stale{Record: staleRec("zshrc", "zshrc")}, Outcome: CleanKeep},
		{Stale: statefile.Stale{Record: staleRec("moved", "m")}, Outcome: CleanRecheck},
		{Stale: statefile.Stale{Record: staleRec("busy", "b")}, Outcome: CleanFail, Err: errors.New("busy")},
	}
	out := render(func(pr *Printer) { pr.CleanRows(rows); pr.CleanSummary(rows, false) })
	checkContains(t, out,
		"  - stale "+p("home", "tmux")+"  ← config/tmux\n",
		"  ! skipped "+p("home", "zshrc")+"\n",
		"    changed since dots created it; dots will not remove it\n",
		"  ! skipped "+p("home", "moved")+"\n",
		"    changed since it was checked; left untouched\n",
		"  ! failed "+p("home", "busy")+"\n", "    busy\n",
		"1 removed, 2 skipped, 1 failed\n",
	)
	dry := render(func(pr *Printer) { pr.CleanSummary(rows[:2], true) })
	checkContains(t, dry, "1 to remove, 1 to skip\n", "dry run: nothing was changed")
}
