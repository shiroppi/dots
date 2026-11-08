package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/state"
)

func makeOKPlanItems(n int) []apply.Item {
	items := make([]apply.Item, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("ok%d", i)
		items[i] = item(name, state.ValidLink, apply.ActionSkip, nil)
	}
	return items
}

func makeOKReportItems(n int) []apply.ItemReport {
	reports := make([]apply.ItemReport, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("ok%d", i)
		reports[i] = apply.ItemReport{
			Item:    item(name, state.ValidLink, apply.ActionSkip, nil),
			Outcome: apply.AlreadyOK,
		}
	}
	return reports
}

func findItemLine(out, prefix string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), prefix) {
			return l
		}
	}
	return ""
}

func checkNoBlankLinesInItems(t *testing.T, out string) {
	t.Helper()
	lines := strings.Split(out, "\n")
	start := -1
	end := -1
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "✓") || strings.HasPrefix(trimmed, "…") ||
			strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "~") ||
			strings.HasPrefix(trimmed, "!") || strings.HasPrefix(trimmed, "×") {
			if start == -1 {
				start = i
			}
			end = i
		}
	}
	if start == -1 || end == -1 || start >= end {
		t.Fatalf("could not find item boundaries in output:\n%s", out)
	}
	for i := start; i <= end; i++ {
		if strings.TrimSpace(lines[i]) == "" {
			t.Errorf("found blank line inside item list at line %d:\n%s", i, out)
		}
	}
}

func TestFoldPlanThresholds(t *testing.T) {
	tests := []struct {
		name     string
		u        int
		k        int
		wantFold bool
		wantN    int
	}{
		{"U < K (U=1, K=2)", 1, 2, false, 0},
		{"U == K (U=2, K=2)", 2, 2, false, 0},
		{"U == K+1 (U=3, K=2)", 3, 2, false, 0},
		{"U == K+2 (U=4, K=2)", 4, 2, true, 2},
		{"U > K+2 (U=7, K=2)", 7, 2, true, 5},
		{"K=5, U=6 (U==K+1)", 6, 5, false, 0},
		{"K=5, U=7 (U>K+1)", 7, 5, true, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			items := makeOKPlanItems(tc.u)
			items = append(items, item("create", state.NotExist, apply.ActionCreate, nil))
			plan := &apply.Plan{Items: items}

			out := render(func(pr *Printer) {
				pr.Fold = true
				pr.FoldKeep = tc.k
				pr.Plan(plan)
			})

			if !tc.wantFold {
				if strings.Contains(out, "…") {
					t.Errorf("expected no fold line, got:\n%s", out)
				}
				for i := 0; i < tc.u; i++ {
					target := p("home", fmt.Sprintf("ok%d", i))
					if !strings.Contains(out, "✓ "+target) {
						t.Errorf("missing row %d: %s in output:\n%s", i, target, out)
					}
				}
			} else {
				foldLine := fmt.Sprintf("  … %d unchanged", tc.wantN)
				if !strings.Contains(out, foldLine) {
					t.Errorf("expected fold line %q in output:\n%s", foldLine, out)
				}
				// First K rows must appear in original order
				var indices []int
				for i := 0; i < tc.k; i++ {
					target := p("home", fmt.Sprintf("ok%d", i))
					idx := strings.Index(out, "✓ "+target)
					if idx == -1 {
						t.Errorf("expected visible row %d: %s in output:\n%s", i, target, out)
					}
					indices = append(indices, idx)
				}
				for i := 1; i < len(indices); i++ {
					if indices[i] <= indices[i-1] {
						t.Errorf("rows out of order: row %d before row %d", i, i-1)
					}
				}
				// Folded rows must NOT appear as ✓ rows
				for i := tc.k; i < tc.u; i++ {
					target := p("home", fmt.Sprintf("ok%d", i))
					if strings.Contains(out, "✓ "+target) {
						t.Errorf("folded row %d: %s should not appear as ✓ in output:\n%s", i, target, out)
					}
				}
				// Fold line must be placed before the created row
				foldIdx := strings.Index(out, foldLine)
				createIdx := strings.Index(out, "+ "+p("home", "create"))
				if foldIdx == -1 || createIdx == -1 || foldIdx >= createIdx {
					t.Errorf("fold line must precede + row: foldIdx=%d createIdx=%d", foldIdx, createIdx)
				}
			}
		})
	}
}

func TestFoldReportThresholds(t *testing.T) {
	tests := []struct {
		name     string
		u        int
		k        int
		wantFold bool
		wantN    int
	}{
		{"Report U == K+1 (U=3, K=2)", 3, 2, false, 0},
		{"Report U == K+2 (U=4, K=2)", 4, 2, true, 2},
		{"Report U > K+2 (U=7, K=2)", 7, 2, true, 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reps := makeOKReportItems(tc.u)
			reps = append(reps, apply.ItemReport{
				Item:    item("create", state.NotExist, apply.ActionCreate, nil),
				Outcome: apply.Created,
			})
			rep := &apply.Report{Items: reps}

			out := render(func(pr *Printer) {
				pr.Fold = true
				pr.FoldKeep = tc.k
				pr.Report(rep)
			})

			if !tc.wantFold {
				if strings.Contains(out, "…") {
					t.Errorf("expected no fold line in Report, got:\n%s", out)
				}
				for i := 0; i < tc.u; i++ {
					target := p("home", fmt.Sprintf("ok%d", i))
					if !strings.Contains(out, "✓ "+target) {
						t.Errorf("missing row %d: %s in Report output:\n%s", i, target, out)
					}
				}
			} else {
				foldLine := fmt.Sprintf("  … %d unchanged", tc.wantN)
				if !strings.Contains(out, foldLine) {
					t.Errorf("expected fold line %q in Report output:\n%s", foldLine, out)
				}
				for i := 0; i < tc.k; i++ {
					target := p("home", fmt.Sprintf("ok%d", i))
					if !strings.Contains(out, "✓ "+target) {
						t.Errorf("expected visible row %d: %s in Report output:\n%s", i, target, out)
					}
				}
				for i := tc.k; i < tc.u; i++ {
					target := p("home", fmt.Sprintf("ok%d", i))
					if strings.Contains(out, "✓ "+target) {
						t.Errorf("folded row %d: %s should not appear as ✓ in Report output:\n%s", i, target, out)
					}
				}
				foldIdx := strings.Index(out, foldLine)
				createIdx := strings.Index(out, "+ "+p("home", "create"))
				if foldIdx == -1 || createIdx == -1 || foldIdx >= createIdx {
					t.Errorf("fold line must precede + row: foldIdx=%d createIdx=%d", foldIdx, createIdx)
				}
			}
		})
	}
}

func TestFoldKeepZero(t *testing.T) {
	// K = 0 and U = 1: U <= K+1 (1 <= 1), 1 ✓ row shown, no "…" line.
	plan1 := &apply.Plan{Items: makeOKPlanItems(1)}
	outPlan1 := render(func(pr *Printer) {
		pr.Fold = true
		pr.FoldKeep = 0
		pr.Plan(plan1)
	})
	if strings.Contains(outPlan1, "…") {
		t.Errorf("K=0, U=1 must not fold, got:\n%s", outPlan1)
	}
	if !strings.Contains(outPlan1, "✓ "+p("home", "ok0")) {
		t.Errorf("K=0, U=1 must show the 1 ✓ row, got:\n%s", outPlan1)
	}

	rep1 := &apply.Report{Items: makeOKReportItems(1)}
	outRep1 := render(func(pr *Printer) {
		pr.Fold = true
		pr.FoldKeep = 0
		pr.Report(rep1)
	})
	if strings.Contains(outRep1, "…") {
		t.Errorf("Report K=0, U=1 must not fold, got:\n%s", outRep1)
	}
	if !strings.Contains(outRep1, "✓ "+p("home", "ok0")) {
		t.Errorf("Report K=0, U=1 must show the 1 ✓ row, got:\n%s", outRep1)
	}

	// K = 0 and U >= 2: no ✓ row, only "  … U unchanged".
	for _, u := range []int{2, 5} {
		plan := &apply.Plan{Items: makeOKPlanItems(u)}
		outPlan := render(func(pr *Printer) {
			pr.Fold = true
			pr.FoldKeep = 0
			pr.Plan(plan)
		})
		if strings.Contains(outPlan, "✓") {
			t.Errorf("K=0, U=%d must show no ✓ row, got:\n%s", u, outPlan)
		}
		wantFold := fmt.Sprintf("  … %d unchanged", u)
		if !strings.Contains(outPlan, wantFold) {
			t.Errorf("K=0, U=%d must show %q, got:\n%s", u, wantFold, outPlan)
		}

		rep := &apply.Report{Items: makeOKReportItems(u)}
		outRep := render(func(pr *Printer) {
			pr.Fold = true
			pr.FoldKeep = 0
			pr.Report(rep)
		})
		if strings.Contains(outRep, "✓") {
			t.Errorf("Report K=0, U=%d must show no ✓ row, got:\n%s", u, outRep)
		}
		if !strings.Contains(outRep, wantFold) {
			t.Errorf("Report K=0, U=%d must show %q, got:\n%s", u, wantFold, outRep)
		}
	}
}

func TestFoldDisabled(t *testing.T) {
	plan := &apply.Plan{Items: makeOKPlanItems(7)}
	outPlan := render(func(pr *Printer) {
		pr.Fold = false
		pr.FoldKeep = 2
		pr.Plan(plan)
	})
	if strings.Contains(outPlan, "…") {
		t.Errorf("pr.Fold=false must not fold Plan, got:\n%s", outPlan)
	}
	for i := 0; i < 7; i++ {
		target := p("home", fmt.Sprintf("ok%d", i))
		if !strings.Contains(outPlan, "✓ "+target) {
			t.Errorf("pr.Fold=false missing row %d: %s in Plan:\n%s", i, target, outPlan)
		}
	}

	rep := &apply.Report{Items: makeOKReportItems(7)}
	outRep := render(func(pr *Printer) {
		pr.Fold = false
		pr.FoldKeep = 2
		pr.Report(rep)
	})
	if strings.Contains(outRep, "…") {
		t.Errorf("pr.Fold=false must not fold Report, got:\n%s", outRep)
	}
	for i := 0; i < 7; i++ {
		target := p("home", fmt.Sprintf("ok%d", i))
		if !strings.Contains(outRep, "✓ "+target) {
			t.Errorf("pr.Fold=false missing row %d: %s in Report:\n%s", i, target, outRep)
		}
	}
}

func TestFoldSummaryAllUnchanged(t *testing.T) {
	// When some items changed: summary must count ALL unchanged items.
	planWithChange := &apply.Plan{
		Items: append(makeOKPlanItems(7), item("create", state.NotExist, apply.ActionCreate, nil)),
	}
	outPlanChange := render(func(pr *Printer) {
		pr.Fold = true
		pr.FoldKeep = 2
		pr.Plan(planWithChange)
	})
	checkContains(t, outPlanChange, "1 to create, 7 unchanged")
	if strings.Contains(outPlanChange, "already ok") {
		t.Errorf("Plan summary must not contain 'already ok':\n%s", outPlanChange)
	}
	if strings.Contains(outPlanChange, "2 unchanged") {
		t.Errorf("Plan summary must count all 7 unchanged rows, not 2:\n%s", outPlanChange)
	}

	repWithChange := &apply.Report{
		Items: append(makeOKReportItems(7), apply.ItemReport{
			Item:    item("create", state.NotExist, apply.ActionCreate, nil),
			Outcome: apply.Created,
		}),
	}
	outRepChange := render(func(pr *Printer) {
		pr.Fold = true
		pr.FoldKeep = 2
		pr.Report(repWithChange)
	})
	checkContains(t, outRepChange, "1 created, 7 unchanged")
	if strings.Contains(outRepChange, "already ok") {
		t.Errorf("Report summary must not contain 'already ok':\n%s", outRepChange)
	}
	if strings.Contains(outRepChange, "2 unchanged") {
		t.Errorf("Report summary must count all 7 unchanged rows, not 2:\n%s", outRepChange)
	}

	// All rows unchanged: still folded, then "Everything is up to date."
	planAllOK := &apply.Plan{Items: makeOKPlanItems(7)}
	outPlanAllOK := render(func(pr *Printer) {
		pr.Fold = true
		pr.FoldKeep = 2
		pr.Plan(planAllOK)
	})
	checkContains(t, outPlanAllOK, "  … 5 unchanged", "Everything is up to date.")
	for i := 0; i < 2; i++ {
		checkContains(t, outPlanAllOK, "✓ "+p("home", fmt.Sprintf("ok%d", i)))
	}
	for i := 2; i < 7; i++ {
		if strings.Contains(outPlanAllOK, "✓ "+p("home", fmt.Sprintf("ok%d", i))) {
			t.Errorf("folded ok rows must not appear as ✓ in Plan:\n%s", outPlanAllOK)
		}
	}

	repAllOK := &apply.Report{Items: makeOKReportItems(7)}
	outRepAllOK := render(func(pr *Printer) {
		pr.Fold = true
		pr.FoldKeep = 2
		pr.Report(repAllOK)
	})
	checkContains(t, outRepAllOK, "  … 5 unchanged", "Everything is up to date.")
	for i := 0; i < 2; i++ {
		checkContains(t, outRepAllOK, "✓ "+p("home", fmt.Sprintf("ok%d", i)))
	}
	for i := 2; i < 7; i++ {
		if strings.Contains(outRepAllOK, "✓ "+p("home", fmt.Sprintf("ok%d", i))) {
			t.Errorf("folded ok rows must not appear as ✓ in Report:\n%s", outRepAllOK)
		}
	}
}

func TestFoldArrowAlignment(t *testing.T) {
	items := makeOKPlanItems(7)
	items = append(items,
		item("create", state.NotExist, apply.ActionCreate, nil),
		item("replace", state.InvalidLink, apply.ActionReplace, nil),
	)
	plan := &apply.Plan{Items: items}

	outFolded := render(func(pr *Printer) {
		pr.Fold = true
		pr.FoldKeep = 2
		pr.Plan(plan)
	})
	outUnfolded := render(func(pr *Printer) {
		pr.Fold = false
		pr.FoldKeep = 2
		pr.Plan(plan)
	})

	// The "…" line never has "←"
	for _, l := range strings.Split(outFolded, "\n") {
		if strings.Contains(l, "…") && strings.Contains(l, "←") {
			t.Errorf("the '…' line must not have '←': %q", l)
		}
	}

	// Compare arrow columns with and without folding
	foldPlus := findItemLine(outFolded, "+")
	unfoldPlus := findItemLine(outUnfolded, "+")
	foldTilde := findItemLine(outFolded, "~")
	unfoldTilde := findItemLine(outUnfolded, "~")

	if foldPlus == "" || unfoldPlus == "" || foldTilde == "" || unfoldTilde == "" {
		t.Fatalf("could not find + or ~ lines in output:\nfolded:\n%s\nunfolded:\n%s", outFolded, outUnfolded)
	}

	colFoldPlus := arrowCol(foldPlus)
	colUnfoldPlus := arrowCol(unfoldPlus)
	colFoldTilde := arrowCol(foldTilde)
	colUnfoldTilde := arrowCol(unfoldTilde)

	if colFoldPlus == -1 || colUnfoldPlus == -1 {
		t.Fatalf("missing arrow in + line: folded=%d unfolded=%d", colFoldPlus, colUnfoldPlus)
	}
	if colFoldPlus != colUnfoldPlus {
		t.Errorf("arrow column for + changed with folding: folded col %d vs unfolded col %d", colFoldPlus, colUnfoldPlus)
	}
	if colFoldTilde != colUnfoldTilde {
		t.Errorf("arrow column for ~ changed with folding: folded col %d vs unfolded col %d", colFoldTilde, colUnfoldTilde)
	}
	if colFoldPlus != colFoldTilde {
		t.Errorf("arrow column for + and ~ not aligned: + is %d, ~ is %d", colFoldPlus, colFoldTilde)
	}
}

func TestFoldColor(t *testing.T) {
	plan := &apply.Plan{Items: makeOKPlanItems(7)}

	// Without Color: no escape sequences
	outPlain := render(func(pr *Printer) {
		pr.Fold = true
		pr.FoldKeep = 2
		pr.Color = false
		pr.Plan(plan)
	})
	checkContains(t, outPlain, "  … 5 unchanged")
	if strings.Contains(outPlain, "\x1b[") {
		t.Errorf("plain output must not have ANSI escapes, got:\n%s", outPlain)
	}

	// With Color: fold line is dim ("\x1b[2m")
	outColor := render(func(pr *Printer) {
		pr.Fold = true
		pr.FoldKeep = 2
		pr.Color = true
		pr.Plan(plan)
	})
	checkContains(t, outColor, "\x1b[2m… 5 unchanged\x1b[0m")
}

func TestFoldNoBlankLine(t *testing.T) {
	planItems := makeOKPlanItems(7)
	planItems = append(planItems,
		item("create", state.NotExist, apply.ActionCreate, nil),
		item("replace", state.FileOrDir, apply.ActionReplace, nil),
		item("problem", state.NotExist, apply.ActionError, func(r *state.Result) {
			r.Err = errors.New("something went wrong")
		}),
	)
	plan := &apply.Plan{Items: planItems}

	t.Run("Plan folded", func(t *testing.T) {
		out := render(func(pr *Printer) {
			pr.Fold = true
			pr.FoldKeep = 2
			pr.Plan(plan)
		})
		checkNoBlankLinesInItems(t, out)
	})

	t.Run("Plan unfolded", func(t *testing.T) {
		out := render(func(pr *Printer) {
			pr.Fold = false
			pr.Plan(plan)
		})
		checkNoBlankLinesInItems(t, out)
	})

	repItems := makeOKReportItems(7)
	repItems = append(repItems,
		apply.ItemReport{Item: item("create", state.NotExist, apply.ActionCreate, nil), Outcome: apply.Created},
		apply.ItemReport{Item: item("replace", state.InvalidLink, apply.ActionReplace, nil), Outcome: apply.Replaced},
		apply.ItemReport{Item: item("problem", state.NotExist, apply.ActionCreate, nil), Outcome: apply.Failed, Err: errors.New("fail")},
	)
	rep := &apply.Report{Items: repItems}

	t.Run("Report folded", func(t *testing.T) {
		out := render(func(pr *Printer) {
			pr.Fold = true
			pr.FoldKeep = 2
			pr.Report(rep)
		})
		checkNoBlankLinesInItems(t, out)
	})

	t.Run("Report unfolded", func(t *testing.T) {
		out := render(func(pr *Printer) {
			pr.Fold = false
			pr.Report(rep)
		})
		checkNoBlankLinesInItems(t, out)
	})
}

func TestApplySourceSimple(t *testing.T) {
	plan := &apply.Plan{
		Items: []apply.Item{
			item("ok_item", state.ValidLink, apply.ActionSkip, nil),
			item("create_item", state.NotExist, apply.ActionCreate, nil),
			item("replace_item", state.InvalidLink, apply.ActionReplace, nil),
		},
	}

	rep := &apply.Report{
		Items: []apply.ItemReport{
			{Item: item("ok_item", state.ValidLink, apply.ActionSkip, nil), Outcome: apply.AlreadyOK},
			{Item: item("create_item", state.NotExist, apply.ActionCreate, nil), Outcome: apply.Created},
			{Item: item("replace_item", state.InvalidLink, apply.ActionReplace, nil), Outcome: apply.Replaced},
		},
	}

	t.Run("Plan", func(t *testing.T) {
		out := render(func(pr *Printer) {
			pr.Plan(plan)
		})
		checkContains(t, out, "← "+p("src", "create_item"), "← "+p("src", "replace_item"))
		if strings.Contains(out, p("src", "ok_item")) {
			t.Errorf("Plan must not show source for ok rows:\n%s", out)
		}
		if strings.Contains(out, `dots."create_item"`) || strings.Contains(out, `dots."replace_item"`) {
			t.Errorf("Plan must not show origin rule:\n%s", out)
		}
	})

	t.Run("Report", func(t *testing.T) {
		out := render(func(pr *Printer) {
			pr.Report(rep)
		})
		checkContains(t, out, "← "+p("src", "create_item"), "← "+p("src", "replace_item"))
		if strings.Contains(out, p("src", "ok_item")) {
			t.Errorf("Report must not show source for AlreadyOK rows:\n%s", out)
		}
		if strings.Contains(out, `dots."create_item"`) || strings.Contains(out, `dots."replace_item"`) {
			t.Errorf("Report must not show origin rule:\n%s", out)
		}
	})
}

func TestStatusSourceAndOverrides(t *testing.T) {
	plan := testPlan() // has 5 items with sources and plan.Overrides

	// Status with verbose=false shows no "←"
	outDefault := render(func(pr *Printer) {
		pr.Status(plan, false)
	})
	if strings.Contains(outDefault, "←") {
		t.Errorf("Status(plan, false) must show no '←', got:\n%s", outDefault)
	}
	if strings.Contains(outDefault, "Overridden rules:") {
		t.Errorf("Status(plan, false) must not show 'Overridden rules:', got:\n%s", outDefault)
	}

	// Status with verbose=true shows "← source  rule" on every row with aligned arrows and never "Overridden rules:"
	outVerbose := render(func(pr *Printer) {
		pr.Status(plan, true)
	})
	if strings.Contains(outVerbose, "Overridden rules:") {
		t.Errorf("Status(plan, true) must not show 'Overridden rules:', got:\n%s", outVerbose)
	}

	var arrowCols []int
	for _, l := range strings.Split(outVerbose, "\n") {
		if col := arrowCol(l); col != -1 {
			arrowCols = append(arrowCols, col)
		}
	}
	if len(arrowCols) < 5 {
		t.Errorf("expected at least 5 arrow rows in verbose status, got %d:\n%s", len(arrowCols), outVerbose)
	}
	for i := 1; i < len(arrowCols); i++ {
		if arrowCols[i] != arrowCols[0] {
			t.Errorf("arrows not aligned at column %d (row %d has col %d):\n%s", arrowCols[0], i, arrowCols[i], outVerbose)
		}
	}

	checkContains(t, outVerbose,
		"← "+p("src", "ok")+`  dots."ok"`,
		"← "+p("src", "missing")+`  dots."missing"`,
		"← "+p("src", "wrong")+`  dots."wrong"`,
		"← "+p("src", "conflict")+`  dots."conflict"`,
		"← "+p("src", "bad")+`  dots."bad"`,
	)
}

// arrowCol returns the display column of "←" in l, or -1 if absent.
// Display width (not byte offset) is used because row symbols differ in UTF-8 length.
func arrowCol(l string) int {
	idx := strings.Index(l, "←")
	if idx == -1 {
		return -1
	}
	return textWidth(l[:idx])
}
