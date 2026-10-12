package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/state"
)

func TestReportGroupsRowsByOutcome(t *testing.T) {
	mk := func(name string, o apply.Outcome, mod func(*apply.ItemReport)) apply.ItemReport {
		ir := apply.ItemReport{Item: item(name, state.NotExist, apply.ActionCreate, nil), Outcome: o}
		if mod != nil {
			mod(&ir)
		}
		return ir
	}
	arch := p("backups", "a.tar.gz")
	rep := &apply.Report{Items: []apply.ItemReport{
		mk("f1", apply.Failed, func(i *apply.ItemReport) { i.Err = errors.New("boom") }),
		mk("c1", apply.Created, nil),
		mk("ok1", apply.AlreadyOK, nil),
		mk("r1", apply.Replaced, func(i *apply.ItemReport) {
			i.ArchivePath = arch
			i.Warning = errors.New("careful")
		}),
		mk("c2longername", apply.Created, nil),
		mk("ok2", apply.AlreadyOK, nil),
		mk("s1", apply.SkippedByUser, nil),
		mk("n1", apply.NotProcessed, nil),
	}}
	out := render(func(pr *Printer) { pr.Report(rep) })
	var rows []string
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(ln, "  ") {
			rows = append(rows, ln)
		}
	}
	want := []string{
		"  \u2713 " + p("home", "ok1"),
		"  \u2713 " + p("home", "ok2"),
		"  + " + p("home", "c1"),
		"  + " + p("home", "c2longername"),
		"  ~ " + p("home", "r1"),
		"    backup: " + arch,
		"    warning: careful",
		"  ! failed " + p("home", "f1"),
		"    boom",
		"  ! skipped " + p("home", "s1"),
		"  ! not processed " + p("home", "n1"),
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d:\n%s", len(rows), len(want), out)
	}
	for i, w := range want {
		if !strings.HasPrefix(rows[i], w) {
			t.Errorf("row %d = %q, want prefix %q", i, rows[i], w)
		}
	}
	// Arrows line up across + and ~ rows although they were not adjacent in the input.
	col := -1
	for _, r := range rows {
		i := strings.Index(r, "\u2190")
		if i < 0 {
			continue
		}
		c := textWidth(r[:i])
		if col >= 0 && c != col {
			t.Errorf("arrow column %d != %d in %q", c, col, r)
		}
		col = c
	}
	if col < 0 {
		t.Errorf("no arrows in output:\n%s", out)
	}
}

func TestPlanGroupsRowsByOutcome(t *testing.T) {
	plan := &apply.Plan{Items: []apply.Item{
		item("bad", state.NotExist, apply.ActionError, func(r *state.Result) { r.Err = errors.New("nope") }),
		item("w", state.InvalidLink, apply.ActionReplace, func(r *state.Result) { r.Current = "../x" }),
		item("m1", state.NotExist, apply.ActionCreate, nil),
		item("ok", state.ValidLink, apply.ActionSkip, nil),
		item("m2", state.NotExist, apply.ActionCreate, nil),
	}}
	out := render(func(pr *Printer) { pr.Plan(plan) })
	seq := []string{"  \u2713 " + p("home", "ok"), "  + " + p("home", "m1"), "  + " + p("home", "m2"),
		"  ~ " + p("home", "w"), "will be backed up and replaced", "  ! error " + p("home", "bad"), "    nope"}
	prev := -1
	for _, s := range seq {
		i := strings.Index(out, s)
		if i < 0 || i <= prev {
			t.Fatalf("%q out of order (idx %d, prev %d):\n%s", s, i, prev, out)
		}
		prev = i
	}
}
