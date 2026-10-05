package ui

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/model"
	"github.com/shiroppi/dots/internal/state"
)

func fp(parts ...string) string { return filepath.Join(parts...) }

func TestOutputPlanPlain(t *testing.T) {
	var buf bytes.Buffer
	pr := &Printer{W: &buf, Color: false}

	plan := &apply.Plan{
		Items: []apply.Item{
			{Action: apply.ActionCreate, Result: state.Result{Entry: model.Entry{Target: fp("t", "c"), Kind: model.KindLink}}},
			{Action: apply.ActionReplace, Result: state.Result{Entry: model.Entry{Target: fp("t", "r"), Kind: model.KindLink}, Status: state.FileOrDir}},
			{Action: apply.ActionSkip, Result: state.Result{Entry: model.Entry{Target: fp("t", "s"), Kind: model.KindLink}}},
			{Action: apply.ActionError, Result: state.Result{Entry: model.Entry{Target: fp("t", "e"), Kind: model.KindLink}, Err: errors.New("fail")}},
		},
	}
	pr.Plan(plan)
	out := buf.String()

	if strings.Contains(out, "\x1b[") {
		t.Error("plain mode should not have ANSI escapes")
	}

	wantLines := []string{
		"  + " + fp("t", "c"),
		"  ~ " + fp("t", "r"),
		"    existing file or directory; will be backed up and replaced after confirmation",
		"  ✓ " + fp("t", "s"),
		"  ! error " + fp("t", "e"),
		"    fail",
		"1 to create, 1 to replace, 1 already ok, 1 error(s)",
	}

	for _, w := range wantLines {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\ngot:\n%s", w, out)
		}
	}
}

func TestOutputPlanUpToDate(t *testing.T) {
	var buf bytes.Buffer
	pr := &Printer{W: &buf, Color: false}
	plan := &apply.Plan{
		Items: []apply.Item{
			{Action: apply.ActionSkip, Result: state.Result{Entry: model.Entry{Target: fp("t", "s"), Kind: model.KindLink}}},
		},
	}
	pr.Plan(plan)
	if !strings.Contains(buf.String(), "Everything is up to date.") {
		t.Errorf("missing 'Everything is up to date.'\ngot:\n%s", buf.String())
	}
}

func TestOutputKindDir(t *testing.T) {
	var buf bytes.Buffer
	pr := &Printer{W: &buf, Color: false}
	plan := &apply.Plan{
		Items: []apply.Item{
			{Action: apply.ActionCreate, Result: state.Result{Entry: model.Entry{Target: fp("t", "dir"), Kind: model.KindDir}}},
		},
	}
	pr.Plan(plan)
	if !strings.Contains(buf.String(), fp("t", "dir")+string(filepath.Separator)) {
		t.Errorf("missing trailing separator for KindDir\ngot:\n%s", buf.String())
	}
}

func TestOutputDoctorVsPlan(t *testing.T) {
	var bufDoc, bufPlan bytes.Buffer
	prDoc := &Printer{W: &bufDoc, Color: false, Root: fp("root")}
	prPlan := &Printer{W: &bufPlan, Color: false, Root: fp("root")}

	plan := &apply.Plan{
		Items: []apply.Item{
			{Action: apply.ActionCreate, Result: state.Result{Entry: model.Entry{Target: fp("t"), Source: fp("root", "src"), Origin: model.Origin{Rule: "rule"}}}},
		},
	}
	prDoc.Doctor(plan)
	prPlan.Plan(plan)

	if !strings.Contains(bufDoc.String(), " -> src  rule") {
		t.Errorf("doctor missing mapping and origin\ngot:\n%s", bufDoc.String())
	}
	if strings.Contains(bufPlan.String(), " -> src") || strings.Contains(bufPlan.String(), "rule") {
		t.Errorf("plan should not have mapping and origin\ngot:\n%s", bufPlan.String())
	}
}

func TestOutputColor(t *testing.T) {
	var buf bytes.Buffer
	pr := &Printer{W: &buf, Color: true}

	plan := &apply.Plan{
		Items: []apply.Item{
			{Action: apply.ActionSkip, Result: state.Result{Entry: model.Entry{Target: "skip"}, Status: state.ValidLink}},
			{Action: apply.ActionCreate, Result: state.Result{Entry: model.Entry{Target: "create"}, Status: state.NotExist}},
			{Action: apply.ActionReplace, Result: state.Result{Entry: model.Entry{Target: "replace"}, Status: state.FileOrDir}},
			{Action: apply.ActionError, Result: state.Result{Entry: model.Entry{Target: "err"}, Status: state.InvalidLink, Err: errors.New("e")}},
		},
	}
	pr.Plan(plan)
	out := buf.String()

	if !strings.Contains(out, "\x1b[32m✓\x1b[0m") {
		t.Errorf("missing green check: %q", out)
	}
	if !strings.Contains(out, "\x1b[34m+\x1b[0m") {
		t.Errorf("missing blue plus: %q", out)
	}
	if !strings.Contains(out, "\x1b[35m~\x1b[0m") {
		t.Errorf("missing magenta tilde: %q", out)
	}
	if !strings.Contains(out, "\x1b[33m! error\x1b[0m") {
		t.Errorf("missing yellow error: %q", out)
	}

	buf.Reset()
	pr.Doctor(&apply.Plan{Items: []apply.Item{{Result: state.Result{Entry: model.Entry{Target: "miss"}, Status: state.NotExist}}}})
	if !strings.Contains(buf.String(), "\x1b[31m×\x1b[0m") {
		t.Errorf("missing red cross in doctor: %q", buf.String())
	}
}

func TestOutputBlocked(t *testing.T) {
	var buf bytes.Buffer
	pr := &Printer{W: &buf, Color: false}
	plan := &apply.Plan{
		Items: []apply.Item{
			{Action: apply.ActionSkip, Result: state.Result{Entry: model.Entry{Target: "s"}}},
			{Action: apply.ActionCreate, Result: state.Result{Entry: model.Entry{Target: "c"}}},
			{Action: apply.ActionReplace, Result: state.Result{Entry: model.Entry{Target: "r"}, Status: state.FileOrDir}},
			{Action: apply.ActionError, Result: state.Result{Entry: model.Entry{Target: "e"}, Err: errors.New("err")}},
		},
	}
	pr.Blocked(plan)
	out := buf.String()
	want := []string{
		"  ✓ s",
		"  ! not processed c",
		"  ! conflict r",
		"    existing file or directory",
		"  ! error e",
		"    err",
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in blocked output\ngot:\n%s", w, out)
		}
	}
}

func TestOutputReport(t *testing.T) {
	var buf bytes.Buffer
	pr := &Printer{W: &buf, Color: false}

	rep := &apply.Report{
		Items: []apply.ItemReport{
			{Outcome: apply.Created, Item: apply.Item{Result: state.Result{Entry: model.Entry{Target: "c"}}}},
			{Outcome: apply.AlreadyOK, Item: apply.Item{Result: state.Result{Entry: model.Entry{Target: "ok"}}}},
			{Outcome: apply.Replaced, Item: apply.Item{Result: state.Result{Entry: model.Entry{Target: "r"}}}, ArchivePath: "arc"},
			{Outcome: apply.SkippedByUser, Item: apply.Item{Result: state.Result{Entry: model.Entry{Target: "s"}}}},
			{Outcome: apply.Failed, Item: apply.Item{Result: state.Result{Entry: model.Entry{Target: "f"}}}, Err: errors.New("fail")},
			{Outcome: apply.NotProcessed, Item: apply.Item{Result: state.Result{Entry: model.Entry{Target: "n"}}}},
		},
	}
	pr.Report(rep)
	out := buf.String()

	want := []string{
		"  + c",
		"  ✓ ok",
		"  ~ r",
		"    backup: arc",
		"  ! skipped s",
		"  ! failed f",
		"    fail",
		"  ! not processed n",
		"1 created, 1 replaced, 1 already ok, 1 skipped, 1 failed, 1 not processed",
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in report output\ngot:\n%s", w, out)
		}
	}
}
