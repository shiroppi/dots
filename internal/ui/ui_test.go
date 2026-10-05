package ui

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/backup"
	"github.com/shiroppi/dots/internal/model"
	"github.com/shiroppi/dots/internal/state"
)

func root() string {
	if filepath.Separator == '\\' {
		return `C:\repo`
	}
	return "/repo"
}

func p(parts ...string) string { return filepath.Join(append([]string{root()}, parts...)...) }

func entry(name, rule string) model.Entry {
	return model.Entry{
		Target: p("home", name),
		Source: p("src", name),
		Origin: model.Origin{Layer: model.LayerDots, Rule: rule},
	}
}

func item(name string, st state.Status, act apply.Action, mod func(*state.Result)) apply.Item {
	r := state.Result{Entry: entry(name, fmt.Sprintf("dots.%q", name)), Status: st}
	if mod != nil {
		mod(&r)
	}
	return apply.Item{Result: r, Action: act}
}

func testPlan() *apply.Plan {
	return &apply.Plan{
		Items: []apply.Item{
			item("ok", state.ValidLink, apply.ActionSkip, nil),
			item("missing", state.NotExist, apply.ActionCreate, nil),
			item("wrong", state.InvalidLink, apply.ActionReplace, func(r *state.Result) { r.Current = "../elsewhere" }),
			item("conflict", state.FileOrDir, apply.ActionReplace, nil),
			item("bad", state.NotExist, apply.ActionError, func(r *state.Result) {
				r.Err = errors.Join(errors.New("source does not exist"), errors.New("overlap"))
			}),
		},
		Overrides: []model.Override{{
			Target: p("home", "ov"),
			Winner: model.Origin{Layer: model.LayerDots, Rule: `dots."win"`},
			Loser:  model.Entry{Source: p("src", "lose"), Origin: model.Origin{Layer: model.LayerAuto, Rule: "auto[0]"}},
		}},
	}
}

func render(f func(*Printer)) string {
	var b bytes.Buffer
	f(&Printer{W: &b})
	return b.String()
}

func checkContains(t *testing.T, out string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(out, s) {
			t.Errorf("output missing %q:\n%s", s, out)
		}
	}
}

func TestDoctor(t *testing.T) {
	var ok bool
	out := render(func(pr *Printer) { ok = pr.Doctor(testPlan()) })
	if ok {
		t.Error("ok = true, want false")
	}
	checkContains(t, out,
		"[OK]", "[MISSING]", "[WRONG LINK]", "[CONFLICT]", "[ERROR]",
		p("home", "ok")+" -> "+p("src", "ok"), `(dots."ok")`,
		"current link: ../elsewhere",
		"! source does not exist", "! overlap",
		"Overridden rules:", `winner: dots."win"`, "loser:  auto[0] -> "+p("src", "lose"),
		"5 link(s): 1 ok, 1 missing, 1 wrong link, 1 conflict, 1 error, 1 overridden rule(s)",
		"Not OK",
	)
}

func TestDoctorOK(t *testing.T) {
	plan := &apply.Plan{Items: []apply.Item{item("ok", state.ValidLink, apply.ActionSkip, nil)}}
	var ok bool
	out := render(func(pr *Printer) { ok = pr.Doctor(plan) })
	if !ok {
		t.Error("ok = false, want true")
	}
	checkContains(t, out, "1 link(s): 1 ok, 0 missing", "Everything is in order.")
	if strings.Contains(out, "Overridden") {
		t.Error("unexpected overrides section")
	}
	empty := render(func(pr *Printer) { ok = pr.Doctor(&apply.Plan{}) })
	if !ok {
		t.Error("empty plan should be ok")
	}
	checkContains(t, empty, "0 link(s)")
}

func TestPlan(t *testing.T) {
	out := render(func(pr *Printer) { pr.Plan(testPlan()) })
	checkContains(t, out,
		"[create]", "[skip]", "[replace]", "[error]",
		"already linked",
		"existing symbolic link -> ../elsewhere (backed up first)",
		"existing file or directory (backed up first)",
		"! source does not exist",
		"Overridden rules:",
		"5 link(s): 1 to create, 1 already linked, 2 to replace, 1 error",
		"2 conflict(s) will ask for confirmation",
	)
	noConf := render(func(pr *Printer) {
		pr.Plan(&apply.Plan{Items: []apply.Item{item("m", state.NotExist, apply.ActionCreate, nil)}})
	})
	if strings.Contains(noConf, "conflict(s)") {
		t.Errorf("unexpected conflict footer:\n%s", noConf)
	}
}

func TestReport(t *testing.T) {
	mk := func(name string, o apply.Outcome, mod func(*apply.ItemReport)) apply.ItemReport {
		ir := apply.ItemReport{Item: item(name, state.NotExist, apply.ActionCreate, nil), Outcome: o}
		if mod != nil {
			mod(&ir)
		}
		return ir
	}
	arch := p("backups", "a.tar.gz")
	rep := &apply.Report{Items: []apply.ItemReport{
		mk("c", apply.Created, nil),
		mk("a", apply.AlreadyOK, nil),
		mk("r", apply.Replaced, func(i *apply.ItemReport) { i.ArchivePath = arch }),
		mk("s1", apply.SkippedByUser, nil),
		mk("s2", apply.SkippedByUser, nil),
		mk("f", apply.Failed, func(i *apply.ItemReport) { i.Err = errors.New("boom") }),
		mk("n", apply.NotProcessed, nil),
	}}
	out := render(func(pr *Printer) { pr.Report(rep) })
	checkContains(t, out,
		"[created]", "[ok]", "[replaced]", "[skipped]", "[failed]", "[not processed]",
		"archive: "+arch, "! boom",
		"1 created, 1 already ok, 1 replaced, 2 skipped by user, 1 failed, 1 not processed",
	)
}

func TestRestorePlan(t *testing.T) {
	base := backup.RestorePlan{
		ArchivePath: p("b", "x.tar.gz"),
		Target:      p("home", ".vimrc"),
		Manifest:    backup.Manifest{Kind: backup.KindFile, Created: "2026-01-02T03:04:05Z"},
	}
	exists := base
	exists.Exists, exists.ExistingKind = true, backup.KindDir

	tests := []struct {
		name   string
		plan   backup.RestorePlan
		dryRun bool
		want   []string
		not    []string
	}{
		{"missing", base, false, []string{"Archive:  " + base.ArchivePath, "Target:   " + base.Target, "Archived: file (created 2026-01-02T03:04:05Z)", "does not exist", "Action:   restore the archive"}, []string{"would", "dry run"}},
		{"exists", exists, false, []string{"exists (dir)", "Action:   back up the current content, then restore"}, []string{"would"}},
		{"dry exists", exists, true, []string{"Action:   would back up the current content", "dry run: nothing was changed"}, nil},
		{"dry missing", base, true, []string{"Action:   would restore the archive"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan := tc.plan
			out := render(func(pr *Printer) { pr.RestorePlan(&plan, tc.dryRun) })
			checkContains(t, out, tc.want...)
			for _, s := range tc.not {
				if strings.Contains(out, s) {
					t.Errorf("output has %q:\n%s", s, out)
				}
			}
		})
	}
}

func TestErrorsFlatten(t *testing.T) {
	a, b, c := errors.New("a"), errors.New("b"), errors.New("c")
	tree := errors.Join(a, errors.Join(b, nil, c))
	wrapped := fmt.Errorf("ctx: %w", errors.Join(errors.New("d"), errors.New("e")))
	out := render(func(pr *Printer) { pr.Errors([]error{tree, nil, wrapped}) })
	// fmt.Errorf wraps a Join: only the top-level Unwrap() []error is flattened,
	// so a %w wrapper stays a single multi-line leaf collapsed to one line.
	want := "error: a\nerror: b\nerror: c\nerror: ctx: d e\n"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestDisplayPath(t *testing.T) {
	sep := string(filepath.Separator)
	home := p("home")
	tests := []struct {
		name, home, path, goos, want string
	}{
		{"under", home, home + sep + ".vimrc", "linux", "~" + sep + ".vimrc"},
		{"nested", home, home + sep + "a" + sep + "b", "linux", "~" + sep + "a" + sep + "b"},
		{"home itself", home, home, "linux", "~"},
		{"sibling prefix", home, home + "2" + sep + "x", "linux", home + "2" + sep + "x"},
		{"outside", home, p("other", "x"), "linux", p("other", "x")},
		{"case sensitive linux", home, strings.ToUpper(home) + sep + "x", "linux", strings.ToUpper(home) + sep + "x"},
		{"case insensitive windows", home, strings.ToUpper(home) + sep + "x", "windows", "~" + sep + "x"},
		{"case insensitive darwin", home, strings.ToLower(home) + sep + "x", "darwin", "~" + sep + "x"},
		{"empty home", "", home, "linux", home},
		{"trailing sep home", home + sep, home + sep + "x", "linux", "~" + sep + "x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := displayPath(tc.home, tc.path, tc.goos); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestPrinterHome(t *testing.T) {
	var b bytes.Buffer
	pr := &Printer{W: &b, Home: p("home")}
	pr.Plan(&apply.Plan{Items: []apply.Item{item("m", state.NotExist, apply.ActionCreate, nil)}})
	checkContains(t, b.String(), "~"+string(filepath.Separator)+"m ->")
}

func TestNoColorHasNoEscapes(t *testing.T) {
	plan := testPlan()
	rep := &apply.Report{Items: []apply.ItemReport{{Item: plan.Items[0], Outcome: apply.Failed, Err: errors.New("x")}}}
	rp := &backup.RestorePlan{Exists: true, ExistingKind: backup.KindFile}
	out := render(func(pr *Printer) {
		pr.Doctor(plan)
		pr.Plan(plan)
		pr.Report(rep)
		pr.RestorePlan(rp, true)
		pr.Errors([]error{errors.New("e")})
	})
	if strings.Contains(out, "\x1b[") {
		t.Errorf("plain output contains ANSI escapes: %q", out)
	}
	var b bytes.Buffer
	(&Printer{W: &b, Color: true}).Doctor(plan)
	if !strings.Contains(b.String(), "\x1b[") {
		t.Error("color output has no escapes")
	}
}

func TestDecisionFor(t *testing.T) {
	tests := []struct {
		choice string
		want   apply.Decision
	}{
		{choiceYes, apply.Yes}, {choiceYesAll, apply.YesToAll}, {choiceNo, apply.No}, {choiceNoAll, apply.NoToAll},
	}
	for _, tc := range tests {
		got, err := decisionFor(tc.choice)
		if err != nil || got != tc.want {
			t.Errorf("decisionFor(%q) = %v, %v; want %v", tc.choice, got, err, tc.want)
		}
	}
	if _, err := decisionFor("nope"); err == nil {
		t.Error("want error for unknown choice")
	}
	if len(conflictChoices) != 4 {
		t.Errorf("want 4 choices, got %d", len(conflictChoices))
	}
	for _, c := range conflictChoices {
		if _, err := decisionFor(c); err != nil {
			t.Errorf("choice %q unmapped", c)
		}
	}
}

func TestRestoreFor(t *testing.T) {
	if v, err := restoreFor(choiceRestore); err != nil || !v {
		t.Errorf("restore: %v %v", v, err)
	}
	if v, err := restoreFor(choiceSkip); err != nil || v {
		t.Errorf("skip: %v %v", v, err)
	}
	if _, err := restoreFor("x"); err == nil {
		t.Error("want error")
	}
}

func TestConflictTitle(t *testing.T) {
	it := item("w", state.InvalidLink, apply.ActionReplace, func(r *state.Result) { r.Current = "../z" })
	title := conflictTitle(it, p("home"))
	checkContains(t, title, "~"+string(filepath.Separator)+"w", "symbolic link -> ../z")
}

func TestReportWarning(t *testing.T) {
	ir := apply.ItemReport{Item: item("w", state.NotExist, apply.ActionCreate, nil), Outcome: apply.Replaced,
		ArchivePath: p("backups", "a.tar.gz"), Warning: errors.New("leftover not removed")}
	rep := &apply.Report{Items: []apply.ItemReport{ir}}
	out := render(func(pr *Printer) { pr.Report(rep) })
	checkContains(t, out, "[replaced]", "warning: leftover not removed", "0 failed, 0 not processed, 1 warning(s)")
}
