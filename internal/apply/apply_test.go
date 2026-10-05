package apply

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/model"
	"github.com/shiroppi/dots/internal/state"
)

func root() string {
	if runtime.GOOS == "windows" {
		return `C:\repo`
	}
	return "/repo"
}

func p(parts ...string) string { return filepath.Join(append([]string{root()}, parts...)...) }

func mustNil(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newFS(t *testing.T) fs.Manager {
	t.Helper()
	m := fs.NewMem()
	mustNil(t, m.MkdirAll(p("src"), 0o755))
	mustNil(t, m.MkdirAll(p("home"), 0o755))
	for _, n := range []string{"a", "b", "c", "d"} {
		mustNil(t, fs.WriteFile(m, p("src", n), []byte(n), 0o644))
	}
	return m
}

func ent(name string) model.Entry {
	return model.Entry{
		Source: p("src", name),
		Target: p("home", "."+name),
		Origin: model.Origin{Layer: model.LayerDots, Rule: "dots." + name},
	}
}

func resolution(names ...string) *model.Resolution {
	r := &model.Resolution{Root: root()}
	for _, n := range names {
		r.Entries = append(r.Entries, ent(n))
	}
	return r
}

func kind(t *testing.T, m fs.Manager, path string) string {
	t.Helper()
	fi, err := m.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "none"
	}
	mustNil(t, err)
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		return "link"
	case fi.IsDir():
		return "dir"
	}
	return "file"
}

type fakePrompter struct {
	decisions []Decision
	asked     []string
	err       error
}

func (f *fakePrompter) Confirm(it Item) (Decision, error) {
	f.asked = append(f.asked, it.Target())
	if f.err != nil {
		return 0, f.err
	}
	d := f.decisions[0]
	if len(f.decisions) > 1 {
		f.decisions = f.decisions[1:]
	}
	return d, nil
}

// fakeReplacer mimics backup.Store.Replace on the in-memory FS.
type fakeReplacer struct {
	fsys      fs.Manager
	replaced  []string
	failPlace bool
}

func (f *fakeReplacer) Replace(target string, place func() error) (string, error) {
	f.replaced = append(f.replaced, target)
	aside := target + ".aside"
	if err := f.fsys.Rename(target, aside); err != nil {
		return "", err
	}
	if err := place(); err != nil {
		_ = f.fsys.Rename(aside, target)
		return "", err
	}
	if err := f.fsys.RemoveAll(aside); err != nil {
		return "", err
	}
	return "archive:" + target, nil
}

// failFS makes Symlink fail for chosen targets.
type failFS struct {
	fs.Manager
	fail map[string]error
}

func (f failFS) Symlink(old, newname string) error {
	if err, ok := f.fail[newname]; ok {
		return err
	}
	return f.Manager.Symlink(old, newname)
}

func TestActionString(t *testing.T) {
	for a, want := range map[Action]string{ActionCreate: "create", ActionSkip: "skip", ActionReplace: "replace", ActionError: "error"} {
		if a.String() != want {
			t.Errorf("got %s want %s", a, want)
		}
	}
}

func TestBuildPlan(t *testing.T) {
	m := newFS(t)
	rel := func(e model.Entry) string {
		l, err := state.RelLink(e.Source, e.Target)
		mustNil(t, err)
		return l
	}
	mustNil(t, m.Symlink(rel(ent("b")), ent("b").Target))    // valid
	mustNil(t, fs.WriteFile(m, ent("c").Target, nil, 0o644)) // file conflict
	mustNil(t, m.Symlink("nowhere", ent("d").Target))        // broken link conflict
	res := resolution("a", "b", "c", "d", "zz")              // zz: missing source
	res.Entries[4].Target = p("home", ".zz")
	res.Overrides = []model.Override{{Target: p("home", ".a")}}

	// Feed entries in reverse to verify sorting.
	for i, j := 0, len(res.Entries)-1; i < j; i, j = i+1, j-1 {
		res.Entries[i], res.Entries[j] = res.Entries[j], res.Entries[i]
	}
	plan := BuildPlan(m, res)

	want := []struct {
		target string
		action Action
	}{
		{p("home", ".a"), ActionCreate},
		{p("home", ".b"), ActionSkip},
		{p("home", ".c"), ActionReplace},
		{p("home", ".d"), ActionReplace},
		{p("home", ".zz"), ActionError},
	}
	if len(plan.Items) != len(want) {
		t.Fatalf("items: %d", len(plan.Items))
	}
	for i, w := range want {
		if plan.Items[i].Target() != w.target || plan.Items[i].Action != w.action {
			t.Errorf("item %d: got %s/%s want %s/%s", i, plan.Items[i].Target(), plan.Items[i].Action, w.target, w.action)
		}
	}
	if !plan.HasErrors() || plan.Conflicts() != 2 || len(plan.Errors()) != 1 || len(plan.Overrides) != 1 {
		t.Errorf("helpers: errors=%v conflicts=%d errs=%d overrides=%d", plan.HasErrors(), plan.Conflicts(), len(plan.Errors()), len(plan.Overrides))
	}
	if !strings.Contains(plan.Errors()[0].Error(), "dots.zz") {
		t.Errorf("error should name the rule: %v", plan.Errors()[0])
	}
	// No side effects.
	if kind(t, m, ent("a").Target) != "none" {
		t.Error("BuildPlan must not modify the FS")
	}
}

func TestBuildPlanNil(t *testing.T) {
	if plan := BuildPlan(fs.NewMem(), nil); len(plan.Items) != 0 {
		t.Fatal("nil resolution must give an empty plan")
	}
}

func TestExecuteValidationErrorStopsBeforeChange(t *testing.T) {
	m := newFS(t)
	res := resolution("a", "b")
	res.Entries[1].Source = p("src", "missing")
	plan := BuildPlan(m, res)
	rep, err := Execute(m, plan, Options{})
	if err == nil || rep != nil {
		t.Fatalf("want error and nil report, got %v %v", rep, err)
	}
	if !errors.Is(err, state.ErrSourceNotExist) {
		t.Errorf("error should wrap cause: %v", err)
	}
	if kind(t, m, ent("a").Target) != "none" {
		t.Error("nothing may be created")
	}
}

func TestExecuteNoTTYConflict(t *testing.T) {
	m := newFS(t)
	mustNil(t, fs.WriteFile(m, ent("b").Target, []byte("old"), 0o644))
	plan := BuildPlan(m, resolution("a", "b"))
	rep, err := Execute(m, plan, Options{Replacer: &fakeReplacer{fsys: m}})
	if err == nil || rep != nil {
		t.Fatalf("want error, got %v %v", rep, err)
	}
	if !strings.Contains(err.Error(), "interactive terminal") || !strings.Contains(err.Error(), "--dry-run") {
		t.Errorf("message: %v", err)
	}
	if kind(t, m, ent("a").Target) != "none" || kind(t, m, ent("b").Target) != "file" {
		t.Error("FS must be untouched")
	}
}

func TestExecuteCreateAndSkip(t *testing.T) {
	m := newFS(t)
	e := ent("b")
	l, _ := state.RelLink(e.Source, e.Target)
	mustNil(t, m.Symlink(l, e.Target))
	res := resolution("a", "b")
	res.Entries[0].Target = p("home", "deep", "er", ".a") // parents are created
	plan := BuildPlan(m, res)
	rep, err := Execute(m, plan, Options{})
	mustNil(t, err)
	if rep.Created() != 1 || rep.AlreadyOK() != 1 || rep.Failed() != 0 || rep.NotProcessed() != 0 {
		t.Fatalf("counts: %+v", rep.Items)
	}
	r := state.Evaluate(m, res.Entries[0])
	if r.Err != nil || r.Status != state.ValidLink {
		t.Fatalf("after apply: %v %v", r.Status, r.Err)
	}
	// Idempotent.
	rep, err = Execute(m, BuildPlan(m, res), Options{})
	mustNil(t, err)
	if rep.AlreadyOK() != 2 {
		t.Fatalf("second run: %+v", rep.Items)
	}
}

func TestExecuteDecisions(t *testing.T) {
	tests := []struct {
		name      string
		decisions []Decision
		asked     int
		replaced  int
		skipped   int
	}{
		{"yes each", []Decision{Yes}, 3, 3, 0},
		{"yes to all sticky", []Decision{YesToAll}, 1, 3, 0},
		{"no each", []Decision{No}, 3, 0, 3},
		{"no to all sticky", []Decision{NoToAll}, 1, 0, 3},
		{"mixed", []Decision{Yes, No, YesToAll}, 3, 2, 1},
		{"no then yes to all", []Decision{No, YesToAll}, 2, 2, 1},
		{"yes then no to all", []Decision{Yes, NoToAll}, 2, 1, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newFS(t)
			mustNil(t, fs.WriteFile(m, ent("a").Target, []byte("old"), 0o644))
			mustNil(t, m.Symlink("elsewhere", ent("b").Target))
			mustNil(t, m.MkdirAll(ent("c").Target, 0o755))
			mustNil(t, fs.WriteFile(m, filepath.Join(ent("c").Target, "child"), []byte("c"), 0o644))
			pr := &fakePrompter{decisions: tt.decisions}
			rp := &fakeReplacer{fsys: m}
			plan := BuildPlan(m, resolution("a", "b", "c"))
			rep, err := Execute(m, plan, Options{Prompter: pr, Replacer: rp})
			mustNil(t, err)
			if len(pr.asked) != tt.asked || rep.Replaced() != tt.replaced || rep.SkippedByUser() != tt.skipped {
				t.Fatalf("asked=%d replaced=%d skipped=%d", len(pr.asked), rep.Replaced(), rep.SkippedByUser())
			}
			if len(rp.replaced) != tt.replaced {
				t.Fatalf("replacer calls: %v", rp.replaced)
			}
			for _, ir := range rep.Items {
				tgt := ir.Item.Target()
				switch ir.Outcome {
				case Replaced:
					if ir.ArchivePath != "archive:"+tgt {
						t.Errorf("archive path: %q", ir.ArchivePath)
					}
					if kind(t, m, tgt) != "link" {
						t.Errorf("%s should now be a link", tgt)
					}
					if r := state.Evaluate(m, ir.Item.Result.Entry); r.Status != state.ValidLink {
						t.Errorf("%s: %s", tgt, r.Status)
					}
				case SkippedByUser:
					if kind(t, m, tgt) == "none" || kind(t, m, tgt+".aside") != "none" {
						t.Errorf("%s must be left untouched", tgt)
					}
					if r := state.Evaluate(m, ir.Item.Result.Entry); r.Status == state.ValidLink {
						t.Errorf("%s must not be linked", tgt)
					}
				}
			}
		})
	}
}

func TestExecuteReplacerFailureRestores(t *testing.T) {
	m := newFS(t)
	mustNil(t, fs.WriteFile(m, ent("a").Target, []byte("old"), 0o644))
	fm := failFS{Manager: m, fail: map[string]error{ent("a").Target: errors.New("boom")}}
	plan := BuildPlan(m, resolution("a", "b"))
	rep, err := Execute(fm, plan, Options{Prompter: &fakePrompter{decisions: []Decision{Yes}}, Replacer: &fakeReplacer{fsys: m}})
	var pe *PartialError
	if !errors.As(err, &pe) || rep == nil {
		t.Fatalf("want PartialError with report, got %v", err)
	}
	if pe.Target != ent("a").Target || pe.Remaining != 1 {
		t.Errorf("partial: %+v", pe)
	}
	if kind(t, m, ent("a").Target) != "file" {
		t.Error("original must be restored by the replacer")
	}
	if rep.Items[0].Outcome != Failed || rep.Items[0].Err == nil || rep.Items[1].Outcome != NotProcessed {
		t.Errorf("report: %+v", rep.Items)
	}
}

func TestExecutePartialFailure(t *testing.T) {
	m := newFS(t)
	boom := errors.New("disk on fire")
	fm := failFS{Manager: m, fail: map[string]error{ent("b").Target: boom}}
	plan := BuildPlan(m, resolution("a", "b", "c", "d"))
	rep, err := Execute(fm, plan, Options{})
	var pe *PartialError
	if !errors.As(err, &pe) || !errors.Is(err, boom) {
		t.Fatalf("want PartialError wrapping cause, got %v", err)
	}
	if rep == nil {
		t.Fatal("report required")
	}
	got := []Outcome{}
	for _, it := range rep.Items {
		got = append(got, it.Outcome)
	}
	want := []Outcome{Created, Failed, NotProcessed, NotProcessed}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("outcomes: %v want %v", got, want)
		}
	}
	if pe.Done != 1 || pe.Remaining != 2 || pe.Target != ent("b").Target {
		t.Errorf("partial: %+v", pe)
	}
	// No rollback.
	if kind(t, m, ent("a").Target) != "link" || kind(t, m, ent("c").Target) != "none" {
		t.Error("completed changes are kept, later items untouched")
	}
}

func TestExecutePrompterError(t *testing.T) {
	m := newFS(t)
	mustNil(t, fs.WriteFile(m, ent("b").Target, []byte("old"), 0o644))
	abort := errors.New("aborted")
	plan := BuildPlan(m, resolution("a", "b", "c"))
	rep, err := Execute(m, plan, Options{Prompter: &fakePrompter{err: abort}, Replacer: &fakeReplacer{fsys: m}})
	var pe *PartialError
	if !errors.As(err, &pe) || !errors.Is(err, abort) || rep == nil {
		t.Fatalf("got %v", err)
	}
	if rep.Created() != 1 || rep.NotProcessed() != 2 || rep.Failed() != 0 {
		t.Errorf("report: %+v", rep.Items)
	}
}

func TestExecuteConflictWithoutReplacer(t *testing.T) {
	m := newFS(t)
	mustNil(t, fs.WriteFile(m, ent("a").Target, []byte("old"), 0o644))
	rep, err := Execute(m, BuildPlan(m, resolution("a")), Options{Prompter: &fakePrompter{decisions: []Decision{Yes}}})
	if err == nil || rep != nil {
		t.Fatalf("got %v %v", rep, err)
	}
}

func TestOutcomeAndDecisionStrings(t *testing.T) {
	for _, s := range []string{Created.String(), AlreadyOK.String(), Replaced.String(), SkippedByUser.String(), Failed.String(), NotProcessed.String(), Yes.String(), YesToAll.String(), No.String(), NoToAll.String()} {
		if s == "" || strings.Contains(s, "(") {
			t.Errorf("bad string %q", s)
		}
	}
}

func TestExecuteRealOS(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("probe", filepath.Join(dir, "probe")); err != nil {
		t.Skipf("symlinks not permitted: %v", err)
	}
	src := filepath.Join(dir, "repo", "src")
	mustNil(t, os.MkdirAll(filepath.Join(src, "nvim"), 0o755))
	mustNil(t, os.WriteFile(filepath.Join(src, "vimrc"), []byte("x"), 0o644))
	res := &model.Resolution{Entries: []model.Entry{
		{Source: filepath.Join(src, "nvim"), Target: filepath.Join(dir, "home", ".config", "nvim")},
		{Source: filepath.Join(src, "vimrc"), Target: filepath.Join(dir, "home", ".vimrc")},
	}}
	m := fs.NewOS()
	plan := BuildPlan(m, res)
	if plan.HasErrors() {
		t.Fatal(plan.Errors())
	}
	rep, err := Execute(m, plan, Options{})
	mustNil(t, err)
	if rep.Created() != 2 {
		t.Fatalf("report: %+v", rep.Items)
	}
	for _, e := range res.Entries {
		text, err := os.Readlink(e.Target)
		mustNil(t, err)
		if filepath.IsAbs(text) {
			t.Errorf("link must be relative: %s", text)
		}
		if r := state.Evaluate(m, e); r.Err != nil || r.Status != state.ValidLink {
			t.Errorf("%s: %s %v", e.Target, r.Status, r.Err)
		}
		if _, err := os.Stat(e.Target); err != nil {
			t.Errorf("link must resolve: %v", err)
		}
	}
	rep, err = Execute(m, BuildPlan(m, res), Options{})
	mustNil(t, err)
	if rep.AlreadyOK() != 2 {
		t.Fatalf("second run: %+v", rep.Items)
	}
}

type warnErr struct{}

func (warnErr) Error() string          { return "cleanup failed" }
func (warnErr) ReplaceSucceeded() bool { return true }

// warnReplacer behaves like fakeReplacer but reports a non-fatal warning.
type warnReplacer struct{ fakeReplacer }

func (w *warnReplacer) Replace(target string, place func() error) (string, error) {
	a, err := w.fakeReplacer.Replace(target, place)
	if err != nil {
		return a, err
	}
	return a, fmt.Errorf("wrapped: %w", warnErr{})
}

func TestExecuteReplaceWarningIsNotFailure(t *testing.T) {
	m := newFS(t)
	mustNil(t, fs.WriteFile(m, ent("a").Target, []byte("old"), 0o644))
	mustNil(t, fs.WriteFile(m, ent("b").Target, []byte("old"), 0o644))
	rp := &warnReplacer{fakeReplacer{fsys: m}}
	plan := BuildPlan(m, resolution("a", "b", "c"))
	rep, err := Execute(m, plan, Options{Prompter: &fakePrompter{decisions: []Decision{Yes}}, Replacer: rp})
	mustNil(t, err)
	if rep.Replaced() != 2 || rep.Created() != 1 || rep.Failed() != 0 || rep.NotProcessed() != 0 {
		t.Fatalf("report: %+v", rep.Items)
	}
	for _, ir := range rep.Items {
		if ir.Outcome == Replaced && (ir.Warning == nil || ir.Err != nil || ir.ArchivePath == "") {
			t.Errorf("replaced item: %+v", ir)
		}
		if ir.Outcome == Created && ir.Warning != nil {
			t.Errorf("unexpected warning on created item")
		}
	}
}

func TestExecuteDirEntry(t *testing.T) {
	m := newFS(t)
	e := ent("a")
	e.Kind = model.KindDir
	e.Target = p("home", ".cfg", "sub")
	res := &model.Resolution{Root: root(), Entries: []model.Entry{e}}

	plan := BuildPlan(m, res)
	if plan.Items[0].Action != ActionCreate {
		t.Fatalf("action %v", plan.Items[0].Action)
	}
	rep, err := Execute(m, plan, Options{})
	mustNil(t, err)
	if rep.Created() != 1 || kind(t, m, e.Target) != "dir" {
		t.Fatalf("created=%d kind=%s", rep.Created(), kind(t, m, e.Target))
	}
	if plan = BuildPlan(m, res); plan.Items[0].Action != ActionSkip {
		t.Fatalf("action %v", plan.Items[0].Action)
	}
}

func TestExecuteDirEntryConflicts(t *testing.T) {
	m := newFS(t)
	e := ent("a")
	e.Kind = model.KindDir
	e.Target = p("home", "conflict")
	res := &model.Resolution{Root: root(), Entries: []model.Entry{e}}

	// Setup conflict (file)
	mustNil(t, fs.WriteFile(m, e.Target, []byte("old"), 0o644))
	plan := BuildPlan(m, res)
	if plan.Items[0].Action != ActionReplace {
		t.Fatalf("action %v", plan.Items[0].Action)
	}

	// Non-interactive rejection
	rep, err := Execute(m, plan, Options{Replacer: &fakeReplacer{fsys: m}})
	if err == nil || rep != nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("want non-interactive error, got %v %v", rep, err)
	}
	if kind(t, m, e.Target) != "file" {
		t.Error("FS must be untouched")
	}

	// Interactive replace
	rep, err = Execute(m, plan, Options{Prompter: &fakePrompter{decisions: []Decision{Yes}}, Replacer: &fakeReplacer{fsys: m}})
	mustNil(t, err)
	if rep.Replaced() != 1 || kind(t, m, e.Target) != "dir" {
		t.Fatalf("replaced=%d kind=%s", rep.Replaced(), kind(t, m, e.Target))
	}

	// Setup conflict (invalid link)
	e.Target = p("home", "conflict2")
	res.Entries[0].Target = e.Target
	mustNil(t, m.Symlink("nowhere", e.Target))
	plan = BuildPlan(m, res)
	if plan.Items[0].Action != ActionReplace {
		t.Fatalf("action %v", plan.Items[0].Action)
	}

	rep, err = Execute(m, plan, Options{Prompter: &fakePrompter{decisions: []Decision{Yes}}, Replacer: &fakeReplacer{fsys: m}})
	mustNil(t, err)
	if rep.Replaced() != 1 || kind(t, m, e.Target) != "dir" {
		t.Fatalf("replaced=%d kind=%s", rep.Replaced(), kind(t, m, e.Target))
	}
}

type failMkdirFS struct {
	fs.Manager
}

func (f failMkdirFS) MkdirAll(path string, perm os.FileMode) error {
	if strings.HasSuffix(path, "faildir") {
		return errors.New("mkdir fail")
	}
	return f.Manager.MkdirAll(path, perm)
}

func TestExecuteDirEntryMkdirAllFailure(t *testing.T) {
	m := newFS(t)
	e := ent("a")
	e.Kind = model.KindDir
	e.Target = p("home", "faildir")
	res := &model.Resolution{Root: root(), Entries: []model.Entry{e}}

	plan := BuildPlan(m, res)
	fm := failMkdirFS{Manager: m}
	rep, err := Execute(fm, plan, Options{})
	var pe *PartialError
	if !errors.As(err, &pe) || !strings.Contains(err.Error(), "mkdir fail") {
		t.Fatalf("want PartialError with mkdir fail, got %v", err)
	}
	if rep == nil || rep.Failed() != 1 {
		t.Fatalf("want failed report, got %+v", rep)
	}
}
