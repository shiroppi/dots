package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/model"
	"github.com/shiroppi/dots/internal/state"
)

// arrowLines returns the output lines that contain an arrow.
func arrowLines(out string) []string {
	var ls []string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, arrow) {
			ls = append(ls, l)
		}
	}
	return ls
}

// arrowColumn is the display column at which the arrow starts.
func arrowColumn(line string) int {
	return runewidth.StringWidth(line[:strings.Index(line, arrow)])
}

func TestArrowOnlyOnChangedRows(t *testing.T) {
	out := render(func(pr *Printer) { pr.Plan(testPlan()) })
	for _, l := range strings.Split(out, "\n") {
		hasArrow := strings.Contains(l, arrow)
		changed := strings.HasPrefix(l, "  + ") || strings.HasPrefix(l, "  ~ ")
		if hasArrow != changed {
			t.Errorf("line %q: arrow = %v, want %v", l, hasArrow, changed)
		}
	}
	if n := len(arrowLines(out)); n != 3 {
		t.Errorf("%d arrow lines, want 3:\n%s", n, out)
	}

	mk := func(name string, o apply.Outcome) apply.ItemReport {
		return apply.ItemReport{Item: item(name, state.NotExist, apply.ActionCreate, nil), Outcome: o}
	}
	rep := &apply.Report{Items: []apply.ItemReport{
		mk("c", apply.Created), mk("a", apply.AlreadyOK), mk("r", apply.Replaced),
		mk("s", apply.SkippedByUser), mk("f", apply.Failed), mk("n", apply.NotProcessed),
	}}
	out = render(func(pr *Printer) { pr.Report(rep) })
	ls := arrowLines(out)
	if len(ls) != 2 || !strings.HasPrefix(ls[0], "  + ") || !strings.HasPrefix(ls[1], "  ~ ") {
		t.Errorf("report must show arrows on + and ~ only:\n%s", out)
	}
}

func TestArrowAlignment(t *testing.T) {
	plan := &apply.Plan{Items: []apply.Item{
		item("a", state.NotExist, apply.ActionCreate, nil),
		item("much-longer-name", state.FileOrDir, apply.ActionReplace, nil),
		item("ok-and-very-long-but-unchanged", state.ValidLink, apply.ActionSkip, nil),
		item("b", state.NotExist, apply.ActionCreate, nil),
	}}
	out := render(func(pr *Printer) { pr.Root = root(); pr.Plan(plan) })
	ls := arrowLines(out)
	if len(ls) != 3 {
		t.Fatalf("got %d arrow lines:\n%s", len(ls), out)
	}
	for _, l := range ls {
		if arrowColumn(l) != arrowColumn(ls[0]) {
			t.Errorf("arrows not aligned:\n%s", out)
		}
	}
	// The unchanged row must not widen the column: the longest arrow row has a two-space gap.
	if !strings.Contains(ls[2], "much-longer-name  ← src/much-longer-name") {
		t.Errorf("longest row should be followed by two spaces: %q", ls[2])
	}
}

func TestArrowAlignmentWideChars(t *testing.T) {
	plan := &apply.Plan{Items: []apply.Item{
		item("vimrc", state.NotExist, apply.ActionCreate, nil),
		item("設定ファイル", state.NotExist, apply.ActionCreate, nil),
		item("ab", state.NotExist, apply.ActionCreate, nil),
	}}
	out := render(func(pr *Printer) { pr.Plan(plan) })
	ls := arrowLines(out)
	if len(ls) != 3 {
		t.Fatalf("got %d arrow lines:\n%s", len(ls), out)
	}
	for _, l := range ls {
		if arrowColumn(l) != arrowColumn(ls[0]) {
			t.Errorf("arrows not aligned by display width:\n%s", out)
		}
	}

	// Full-width characters in the source count 2 columns for the overflow check.
	wide := &apply.Plan{Items: []apply.Item{{Action: apply.ActionCreate, Result: state.Result{
		Entry: model.Entry{Target: p("home", "x"), Source: p("src", "日本語")},
	}}}}
	head := textWidth("  + " + p("home", "x"))
	fit := head + 2 + textWidth(arrow) + 1 + textWidth("src/日本語")
	out = render(func(pr *Printer) { pr.Root = root(); pr.Width = fit; pr.Plan(wide) })
	if !strings.Contains(out, p("home", "x")+"  ← src/日本語") {
		t.Errorf("unexpected output:\n%s", out)
	}
	wide.Items = append(wide.Items, item("long-name-to-pad", state.NotExist, apply.ActionCreate, nil))
	out = render(func(pr *Printer) { pr.Root = root(); pr.Width = fit; pr.Plan(wide) })
	if !strings.Contains(out, p("home", "x")+"  ← src/日本語") {
		t.Errorf("source width must count 2 per wide char; want fallback:\n%s", out)
	}
}

func TestArrowTerminalWidthFallback(t *testing.T) {
	plan := &apply.Plan{Items: []apply.Item{
		item("a", state.NotExist, apply.ActionCreate, nil),
		item("a-much-longer-name", state.NotExist, apply.ActionCreate, nil),
	}}
	fits := render(func(pr *Printer) { pr.Width = 200; pr.Plan(plan) })
	ls := arrowLines(fits)
	if arrowColumn(ls[0]) != arrowColumn(ls[1]) || !strings.Contains(ls[0], p("home", "a")+strings.Repeat(" ", len("a-much-longer-name")-1+2)+"←") {
		t.Errorf("should align when it fits:\n%s", fits)
	}

	// Too narrow for the aligned layout: each row gets two spaces instead.
	narrow := render(func(pr *Printer) { pr.Width = 50; pr.Plan(plan) })
	ls = arrowLines(narrow)
	if !strings.Contains(ls[0], p("home", "a")+"  ← ") || !strings.Contains(ls[1], p("home", "a-much-longer-name")+"  ← ") {
		t.Errorf("want two spaces before the arrow:\n%s", narrow)
	}
}

func TestArrowNoColor(t *testing.T) {
	out := render(func(pr *Printer) { pr.Plan(testPlan()) })
	if strings.Contains(out, "\x1b") {
		t.Errorf("escapes without color:\n%q", out)
	}
	if !strings.Contains(out, "← "+p("src", "missing")) {
		t.Errorf("arrow missing without color:\n%s", out)
	}
	var sb strings.Builder
	(&Printer{W: &sb, Color: true}).Plan(testPlan())
	if !strings.Contains(sb.String(), "\x1b[2m← "+p("src", "missing")+"\x1b[0m") {
		t.Errorf("source should be dim with color:\n%q", sb.String())
	}
}

func TestDoctorArrowOnEveryItem(t *testing.T) {
	out := render(func(pr *Printer) { pr.Doctor(testPlan()) })
	if n := len(arrowLines(out)); n != 5 {
		t.Errorf("doctor arrow lines = %d, want 5:\n%s", n, out)
	}
	if strings.Contains(out, " -> "+p("src", "ok")) {
		t.Errorf("old arrow still shown:\n%s", out)
	}
}

func TestArrowPadding(t *testing.T) {
	got := arrowPadding([]int{4, 10, 6}, []int{3, 3, 3}, 0)
	if want := []int{8, 2, 6}; !reflect.DeepEqual(got, want) {
		t.Errorf("aligned = %v, want %v", got, want)
	}
	// 10 + 2 + "←" (1) + 1 + 3 = 17 columns needed.
	if got := arrowPadding([]int{4, 10}, []int{3, 3}, 17); !reflect.DeepEqual(got, []int{8, 2}) {
		t.Errorf("exact fit = %v", got)
	}
	if got := arrowPadding([]int{4, 10}, []int{3, 3}, 16); !reflect.DeepEqual(got, []int{2, 2}) {
		t.Errorf("overflow = %v", got)
	}
	if got := arrowPadding(nil, nil, 10); len(got) != 0 {
		t.Errorf("empty = %v", got)
	}
}
