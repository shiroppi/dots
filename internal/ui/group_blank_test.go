package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/state"
)

func TestGroupedBlankLineAfterOK(t *testing.T) {
	ok := func(n string) apply.Item { return item(n, state.ValidLink, apply.ActionSkip, nil) }
	cr := func(n string) apply.Item { return item(n, state.NotExist, apply.ActionCreate, nil) }
	rp := func(n string) apply.Item { return item(n, state.FileOrDir, apply.ActionReplace, nil) }
	er := func(n string) apply.Item {
		return item(n, state.NotExist, apply.ActionError, func(r *state.Result) { r.Err = errors.New("nope") })
	}
	// rows returns the item section of the plan output (between header and summary).
	rows := func(items ...apply.Item) string {
		out := render(func(pr *Printer) { pr.Plan(&apply.Plan{Items: items}) })
		if i := strings.Index(out, "Repository:"); i >= 0 { // drop header line and its blank line
			out = out[strings.Index(out[i:], "\n\n")+i+2:]
		}
		return out[:strings.LastIndex(out, "\n\n")+1] // drop the summary
	}
	h := func(n string) string { return p("home", n) }

	if got := rows(ok("a"), ok("b")); strings.Contains(got, "\n\n") || strings.HasPrefix(got, "\n") {
		t.Errorf("only ok rows must have no blank line:\n%q", got)
	}
	if got := rows(cr("a"), rp("b"), er("c")); strings.Contains(got, "\n\n") || strings.HasPrefix(got, "\n") {
		t.Errorf("only changed rows must have no blank line:\n%q", got)
	}
	got := rows(cr("c1"), ok("o1"), er("e1"), rp("r1"), ok("o2"))
	want := "  ✓ " + h("o1") + "\n  ✓ " + h("o2") + "\n\n  + " + h("c1")
	if !strings.HasPrefix(got, want) {
		t.Errorf("got\n%q\nwant prefix\n%q", got, want)
	}
	if strings.Count(got, "\n\n") != 1 {
		t.Errorf("want exactly one blank line:\n%q", got)
	}
	if !strings.Contains(got, "will be backed up and replaced after confirmation\n  ! error "+h("e1")+"\n    nope\n") {
		t.Errorf("details must stay attached and groups contiguous:\n%q", got)
	}
}

func TestStatusGroupsRowsLikeApply(t *testing.T) {
	h := func(n string) string { return p("home", n) }
	plan := &apply.Plan{Items: []apply.Item{
		item("conf", state.FileOrDir, apply.ActionReplace, nil),
		item("miss1", state.NotExist, apply.ActionCreate, nil),
		item("wrong", state.InvalidLink, apply.ActionReplace, func(r *state.Result) { r.Current = "elsewhere" }),
		item("ok1", state.ValidLink, apply.ActionSkip, nil),
		item("miss2", state.NotExist, apply.ActionCreate, nil),
		item("ok2", state.ValidLink, apply.ActionSkip, nil),
	}}
	out := render(func(pr *Printer) { pr.Status(plan, false) })
	want := "  ✓ " + h("ok1") + "\n  ✓ " + h("ok2") + "\n\n" +
		"  × " + h("miss1") + "\n  × " + h("miss2") + "\n" +
		"  ! conflict " + h("conf") + "\n" +
		"  ! wrong link " + h("wrong") + "\n    current link: elsewhere\n\n"
	if !strings.Contains(out, want) {
		t.Errorf("status rows not grouped like apply:\n%s\nwant to contain\n%s", out, want)
	}
}
