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

func TestDoctorStale(t *testing.T) {
	sr := StateReport{Stale: []statefile.Stale{
		{Record: staleRec("tmux", "config/tmux"), Kind: statefile.StaleLink},
		{Record: staleRec("zshrc", "zshrc"), Kind: statefile.StaleChanged},
		{Record: staleRec("gone", "gone"), Kind: statefile.StaleGone},
	}}
	var ok bool
	out := render(func(pr *Printer) { ok = pr.DoctorState(&apply.Plan{}, sr) })
	if ok {
		t.Error("ok = true, want false")
	}
	checkContains(t, out,
		"  ! stale "+p("home", "tmux")+"  ← config/tmux\n",
		"    not in dots.toml; created by dots on 2026-10-01\n",
		"  ! stale "+p("home", "zshrc")+"\n",
		"    changed since dots created it; dots will not remove it\n",
		"2 stale\n",
		"Not OK",
		"\nRun \"dots unlink --stale\" to remove stale links.\n",
	)
	if strings.Contains(out, "gone") {
		t.Errorf("a gone record is shown:\n%s", out)
	}
	if strings.Contains(out, "\x1b") {
		t.Error("escape sequences without color")
	}
	if strings.Contains(out, "zshrc  ←") {
		t.Error("a changed item has no source")
	}
}

func TestDoctorStaleAlignsArrows(t *testing.T) {
	sr := StateReport{Stale: []statefile.Stale{
		{Record: staleRec("a", "x"), Kind: statefile.StaleLink},
		{Record: staleRec("longer-name", "y"), Kind: statefile.StaleLink},
	}}
	out := render(func(pr *Printer) { pr.DoctorState(&apply.Plan{}, sr) })
	var cols []int
	for _, line := range strings.Split(out, "\n") {
		if i := strings.Index(line, "←"); i >= 0 {
			cols = append(cols, textWidth(line[:i]))
		}
	}
	if len(cols) != 2 || cols[0] != cols[1] {
		t.Errorf("arrows not aligned: %v\n%s", cols, out)
	}
}

func TestDoctorBrokenState(t *testing.T) {
	sr := StateReport{Path: p("state.json"), Err: errors.New("state file is unreadable: bad")}
	var ok bool
	out := render(func(pr *Printer) { ok = pr.DoctorState(&apply.Plan{}, sr) })
	if ok {
		t.Error("ok = true, want false")
	}
	checkContains(t, out, "! error "+p("state.json"), "    state file is unreadable: bad", "1 error")
	if strings.Contains(out, "dots unlink") {
		t.Errorf("no unlink hint without stale links:\n%s", out)
	}
}

func TestUnlinkRows(t *testing.T) {
	rows := []UnlinkRow{
		{Stale: statefile.Stale{Record: staleRec("tmux", "config/tmux")}, Outcome: UnlinkRemove},
		{Stale: statefile.Stale{Record: staleRec("zshrc", "zshrc")}, Outcome: UnlinkKeep},
		{Stale: statefile.Stale{Record: staleRec("moved", "m")}, Outcome: UnlinkRecheck},
		{Stale: statefile.Stale{Record: staleRec("busy", "b")}, Outcome: UnlinkFail, Err: errors.New("busy")},
	}
	out := render(func(pr *Printer) { pr.UnlinkRows(rows); pr.UnlinkSummary(rows, false) })
	checkContains(t, out,
		"  - stale "+p("home", "tmux")+"  ← config/tmux\n",
		"  ! skipped "+p("home", "zshrc")+"\n",
		"    changed since dots created it; dots will not remove it\n",
		"  ! skipped "+p("home", "moved")+"\n",
		"    changed since it was checked; left untouched\n",
		"  ! failed "+p("home", "busy")+"\n", "    busy\n",
		"1 unlinked, 2 skipped, 1 failed\n",
	)
	dry := render(func(pr *Printer) { pr.UnlinkSummary(rows[:2], true) })
	checkContains(t, dry, "1 to unlink, 1 to skip\n", "dry run: nothing was changed")
}
