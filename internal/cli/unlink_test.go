package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	dotsfs "github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/statefile"
	"github.com/shiroppi/dots/internal/ui"
)

// staleSetup applies a repository with ~/.a and ~/.b, then drops the ~/.b
// rule from dots.toml, so ~/.b is a StaleLink.
func staleSetup(t *testing.T) *testEnv {
	t.Helper()
	te := stateSetup(t)
	te.write(t, p("dots.toml"), "[dots]\n\"~/.a\" = \"a\"\n\"~/.b\" = \"b\"\n")
	te.write(t, p("b"), "b")
	if c := te.run("apply"); c != 0 {
		t.Fatalf("apply: %d %s%s", c, te.out, te.errOut)
	}
	te.write(t, p("dots.toml"), "[dots]\n\"~/.a\" = \"a\"\n")
	return te
}

func (te *testEnv) write(t *testing.T, path, content string) {
	t.Helper()
	if err := dotsfs.WriteFile(te.fsys, path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (te *testEnv) isLink(path string) bool {
	_, err := te.fsys.Readlink(path)
	return err == nil
}

func (te *testEnv) recorded(t *testing.T, target string) bool {
	t.Helper()
	_, ok := te.loadState(t).Find(target)
	return ok
}

func (te *testEnv) interactive(answer int) {
	te.app.Interactive = true
	te.app.Ask = func(q ui.Question) (ui.Answer, error) {
		return ui.Answer{Choice: answer}, nil
	}
}

func TestDoctorShowsStale(t *testing.T) {
	te := staleSetup(t)
	before := snap(te.fsys, p("state"))
	if c := te.run("doctor"); c != 1 {
		t.Fatalf("want 1, got %d", c)
	}
	out := te.out.String()
	for _, want := range []string{
		"! stale ~/.b",
		"← b",
		"not in dots.toml; created by dots on 2026-10-07",
		`Run "dots unlink --stale" to remove stale links.`,
		"1 ok, 1 stale",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output lacks %q:\n%s", want, out)
		}
	}
	if before != snap(te.fsys, p("state")) || !te.recorded(t, p("home", ".b")) {
		t.Error("doctor changed the state")
	}
}

func TestDoctorStaleChangedAndGone(t *testing.T) {
	te := staleSetup(t)
	te.write(t, p("b2"), "x")
	te.write(t, p("dots.toml"), "[dots]\n\"~/.a\" = \"a\"\n\"~/.c\" = \"b2\"\n")
	if c := te.run("apply"); c != 0 {
		t.Fatalf("apply: %d %s", c, te.errOut)
	}
	te.write(t, p("dots.toml"), "[dots]\n\"~/.a\" = \"a\"\n")
	// ~/.b becomes a regular file, ~/.c vanishes.
	if err := te.fsys.Remove(p("home", ".b")); err != nil {
		t.Fatal(err)
	}
	te.write(t, p("home", ".b"), "mine")
	if err := te.fsys.Remove(p("home", ".c")); err != nil {
		t.Fatal(err)
	}
	if c := te.run("doctor"); c != 1 {
		t.Fatalf("want 1, got %d", c)
	}
	out := te.out.String()
	if !strings.Contains(out, "changed since dots created it; dots will not remove it") {
		t.Errorf("missing changed detail:\n%s", out)
	}
	if strings.Contains(out, ".c") {
		t.Errorf("a gone target must not be shown:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "stale ~/.b") && strings.Contains(line, "←") {
			t.Errorf("a changed item shows no source: %q", line)
		}
	}
}

func TestDoctorOnlyGoneIsClean(t *testing.T) {
	te := staleSetup(t)
	if err := te.fsys.Remove(p("home", ".b")); err != nil {
		t.Fatal(err)
	}
	if c := te.run("doctor"); c != 0 {
		t.Fatalf("want 0, got %d\n%s", c, te.out)
	}
	if strings.Contains(te.out.String(), "stale") {
		t.Errorf("unexpected stale output:\n%s", te.out)
	}
}

func TestDoctorIgnoresOtherRepositories(t *testing.T) {
	te := staleSetup(t)
	st := te.loadState(t)
	st.Record(statefile.Link{Target: p("home", ".other"), Link: "x", Repository: p("elsewhere"), Source: "x", RecordedAt: stateT0})
	if err := te.store().Save(st); err != nil {
		t.Fatal(err)
	}
	te.write(t, p("home", ".other"), "")
	te.run("doctor")
	if strings.Contains(te.out.String(), ".other") {
		t.Errorf("record of another repository shown:\n%s", te.out)
	}
}

func TestDoctorBrokenAndNewerState(t *testing.T) {
	te := staleSetup(t)
	if err := te.fsys.MkdirAll(filepath.Dir(te.app.StatePath), 0o700); err != nil {
		t.Fatal(err)
	}
	te.write(t, te.app.StatePath, "{garbage")
	if c := te.run("doctor"); c != 1 {
		t.Fatalf("broken: want 1, got %d", c)
	}
	if !strings.Contains(te.out.String(), "! error") || !strings.Contains(te.out.String(), "unreadable") {
		t.Errorf("broken state not reported:\n%s", te.out)
	}
	if data, _ := dotsfs.ReadFile(te.fsys, te.app.StatePath); string(data) != "{garbage" {
		t.Errorf("doctor rewrote the state: %q", data)
	}

	te.write(t, te.app.StatePath, `{"version":2,"links":[]}`)
	if c := te.run("doctor"); c != 0 {
		t.Fatalf("newer: want 0, got %d\n%s%s", c, te.out, te.errOut)
	}
	if !strings.Contains(te.errOut.String(), "warning") || !strings.Contains(te.errOut.String(), "newer dots") {
		t.Errorf("want a warning, got %q", te.errOut)
	}
}

func TestApplyReportsStale(t *testing.T) {
	te := staleSetup(t)
	if c := te.run("apply"); c != 0 {
		t.Fatalf("apply: %d %s", c, te.errOut)
	}
	out := te.out.String()
	if !strings.Contains(out, "1 stale link(s) found.") || !strings.Contains(out, `Run "dots unlink --stale" to remove them.`) {
		t.Errorf("missing stale notice:\n%s", out)
	}
	if strings.Index(out, "already ok") > strings.Index(out, "stale link(s)") {
		t.Errorf("notice must follow the summary:\n%s", out)
	}
	if !te.isLink(p("home", ".b")) || !te.recorded(t, p("home", ".b")) {
		t.Error("apply removed a stale link or its record")
	}
}

func TestApplyWithoutStaleHasNoNotice(t *testing.T) {
	te := stateSetup(t)
	te.run("apply")
	if strings.Contains(te.out.String(), "stale") {
		t.Errorf("unexpected notice:\n%s", te.out)
	}
}

func TestUnlinkStaleDryRun(t *testing.T) {
	te := staleSetup(t)
	before := snap(te.fsys, p("state")) + snap(te.fsys, p("home"))
	if c := te.run("unlink", "--stale", "--dry-run"); c != 0 {
		t.Fatalf("want 0, got %d %s", c, te.errOut)
	}
	out := te.out.String()
	for _, want := range []string{"- stale ~/.b", "← b", "1 to unlink", "dry run: nothing was changed"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	if before != snap(te.fsys, p("state"))+snap(te.fsys, p("home")) || !te.isLink(p("home", ".b")) || !te.recorded(t, p("home", ".b")) {
		t.Error("dry run changed something")
	}
}

func TestUnlinkStaleYes(t *testing.T) {
	te := staleSetup(t)
	if c := te.run("unlink", "--stale", "--yes"); c != 0 {
		t.Fatalf("want 0, got %d %s", c, te.errOut)
	}
	out := te.out.String()
	for _, want := range []string{"- stale ~/.b", "← b", "1 unlinked"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	if _, err := te.fsys.Lstat(p("home", ".b")); err == nil {
		t.Error("link still exists")
	}
	if data, err := dotsfs.ReadFile(te.fsys, p("b")); err != nil || string(data) != "b" {
		t.Errorf("source was touched: %q %v", data, err)
	}
	if !te.isLink(p("home", ".a")) || !te.recorded(t, p("home", ".a")) {
		t.Error("a current link or its record was removed")
	}
	if te.recorded(t, p("home", ".b")) {
		t.Error("record not removed")
	}
	if c := te.run("doctor"); c != 0 || strings.Contains(te.out.String(), "stale") {
		t.Errorf("doctor after unlink: %d\n%s", c, te.out)
	}
}

func TestUnlinkStaleKeepsParentDirectories(t *testing.T) {
	te := stateSetup(t)
	te.write(t, p("dots.toml"), "[dots]\n\"~/.a\" = \"a\"\n\"~/.config/x/y\" = \"a\"\n")
	if c := te.run("apply"); c != 0 {
		t.Fatalf("apply: %d %s", c, te.errOut)
	}
	te.write(t, p("dots.toml"), "[dots]\n\"~/.a\" = \"a\"\n")
	if c := te.run("unlink", "--stale", "--yes"); c != 0 {
		t.Fatalf("unlink: %d %s", c, te.errOut)
	}
	if ok, _ := dotsfs.Exists(te.fsys, p("home", ".config", "x")); !ok {
		t.Error("parent directory was removed")
	}
}

func TestUnlinkStaleNonInteractiveNeedsYes(t *testing.T) {
	te := staleSetup(t)
	if c := te.run("unlink", "--stale"); c != 1 {
		t.Fatalf("want 1, got %d", c)
	}
	if !strings.Contains(te.errOut.String(), "--yes") || !strings.Contains(te.errOut.String(), "nothing was changed") {
		t.Errorf("stderr: %q", te.errOut)
	}
	if !te.isLink(p("home", ".b")) || !te.recorded(t, p("home", ".b")) {
		t.Error("something was changed")
	}
}

func TestUnlinkStaleConfirmation(t *testing.T) {
	var asked ui.Question
	te := staleSetup(t)
	te.app.Interactive = true
	te.app.Ask = func(q ui.Question) (ui.Answer, error) {
		asked = q
		return ui.Answer{Choice: 1}, nil
	}
	if c := te.run("unlink", "--stale"); c != 0 {
		t.Fatalf("No: want 0, got %d", c)
	}
	if asked.Initial != 1 || len(asked.Options) != 2 || !strings.HasPrefix(asked.Options[0], "Yes") || !strings.HasPrefix(asked.Options[1], "No") {
		t.Errorf("question = %+v", asked)
	}
	if !strings.Contains(te.out.String(), "- stale ~/.b") {
		t.Errorf("the list must be shown before the question:\n%s", te.out)
	}
	if !te.isLink(p("home", ".b")) || !te.recorded(t, p("home", ".b")) {
		t.Error("No changed something")
	}

	te.interactive(0)
	if c := te.run("unlink", "--stale"); c != 0 {
		t.Fatalf("Yes: want 0, got %d %s", c, te.errOut)
	}
	if te.isLink(p("home", ".b")) || te.recorded(t, p("home", ".b")) {
		t.Error("Yes did not remove the link")
	}
	if strings.Count(te.out.String(), "- stale ~/.b") != 1 {
		t.Errorf("the list should not be printed again after the question:\n%s", te.out)
	}
	if !strings.Contains(te.out.String(), "1 unlinked") {
		t.Errorf("no summary:\n%s", te.out)
	}
}

func TestUnlinkStaleAbort(t *testing.T) {
	te := staleSetup(t)
	te.app.Interactive = true
	te.app.Ask = func(ui.Question) (ui.Answer, error) { return ui.Answer{}, ui.ErrAborted }
	if c := te.run("unlink", "--stale"); c != 130 {
		t.Fatalf("want 130, got %d", c)
	}
	if !te.isLink(p("home", ".b")) || !te.recorded(t, p("home", ".b")) {
		t.Error("abort changed something")
	}
}

func TestUnlinkStaleRecheckSkips(t *testing.T) {
	te := staleSetup(t)
	te.app.Interactive = true
	te.app.Ask = func(ui.Question) (ui.Answer, error) {
		// The user repoints the link after the list was shown.
		if err := te.fsys.Remove(p("home", ".b")); err != nil {
			t.Fatal(err)
		}
		if err := te.fsys.Symlink("elsewhere", p("home", ".b")); err != nil {
			t.Fatal(err)
		}
		return ui.Answer{Choice: 0}, nil
	}
	if c := te.run("unlink", "--stale"); c != 0 {
		t.Fatalf("want 0, got %d %s", c, te.errOut)
	}
	if got, _ := te.fsys.Readlink(p("home", ".b")); got != "elsewhere" {
		t.Errorf("link was touched: %q", got)
	}
	if !strings.Contains(te.out.String(), "! skipped ~/.b") {
		t.Errorf("want a skipped row:\n%s", te.out)
	}
	if te.recorded(t, p("home", ".b")) {
		t.Error("record should be dropped")
	}
}

func TestUnlinkStaleChangedAndGone(t *testing.T) {
	te := staleSetup(t)
	te.write(t, p("c"), "c")
	te.write(t, p("dots.toml"), "[dots]\n\"~/.a\" = \"a\"\n\"~/.b\" = \"b\"\n\"~/.c\" = \"c\"\n\"~/.d\" = \"c\"\n")
	if c := te.run("apply"); c != 0 {
		t.Fatalf("apply: %d %s", c, te.errOut)
	}
	te.write(t, p("dots.toml"), "[dots]\n\"~/.a\" = \"a\"\n")
	// ~/.b becomes a regular file, ~/.c vanishes, ~/.d stays a link.
	if err := te.fsys.Remove(p("home", ".b")); err != nil {
		t.Fatal(err)
	}
	te.write(t, p("home", ".b"), "mine")
	if err := te.fsys.Remove(p("home", ".c")); err != nil {
		t.Fatal(err)
	}
	te.interactive(0)
	if c := te.run("unlink", "--stale"); c != 0 {
		t.Fatalf("want 0, got %d %s", c, te.errOut)
	}
	out := te.out.String()
	if !strings.Contains(out, "! skipped ~/.b") || !strings.Contains(out, "dots will not remove it") {
		t.Errorf("changed item not reported:\n%s", out)
	}
	if strings.Contains(out, "~/.c") {
		t.Errorf("a gone target must not be shown:\n%s", out)
	}
	if data, _ := dotsfs.ReadFile(te.fsys, p("home", ".b")); string(data) != "mine" {
		t.Errorf("changed file was touched: %q", data)
	}
	if te.isLink(p("home", ".d")) {
		t.Error("the stale link remains")
	}
	for _, name := range []string{".b", ".c", ".d"} {
		if te.recorded(t, p("home", name)) {
			t.Errorf("record of %s remains", name)
		}
	}
}

func TestUnlinkStaleOnlyChangedNeedsNoConfirmation(t *testing.T) {
	te := staleSetup(t)
	if err := te.fsys.Remove(p("home", ".b")); err != nil {
		t.Fatal(err)
	}
	te.write(t, p("home", ".b"), "mine")
	if c := te.run("unlink", "--stale"); c != 0 {
		t.Fatalf("want 0, got %d %s", c, te.errOut)
	}
	if te.recorded(t, p("home", ".b")) {
		t.Error("record remains")
	}
}

func TestUnlinkStaleNothing(t *testing.T) {
	te := stateSetup(t)
	te.run("apply")
	if c := te.run("unlink", "--stale"); c != 0 || !strings.Contains(te.out.String(), "No stale links.") {
		t.Errorf("%d %q", c, te.out)
	}
}

func TestUnlinkStaleStateProblems(t *testing.T) {
	for name, tc := range map[string]struct{ doc, msg string }{
		"broken": {"{garbage", "nothing was removed"},
		"newer":  {`{"version":2,"links":[]}`, "refusing to change anything"},
	} {
		t.Run(name, func(t *testing.T) {
			te := staleSetup(t)
			te.write(t, te.app.StatePath, tc.doc)
			for _, args := range [][]string{{"unlink", "--stale", "--yes"}, {"unlink", "--stale", "--dry-run"}} {
				if c := te.run(args...); c != 1 {
					t.Fatalf("%v: want 1, got %d", args, c)
				}
				if !strings.Contains(te.errOut.String(), tc.msg) {
					t.Errorf("stderr: %q", te.errOut)
				}
			}
			if !te.isLink(p("home", ".b")) {
				t.Error("a link was removed")
			}
			if data, _ := dotsfs.ReadFile(te.fsys, te.app.StatePath); string(data) != tc.doc {
				t.Errorf("state rewritten: %q", data)
			}
		})
	}
}

func TestUnlinkStaleRemoveFailureKeepsRecord(t *testing.T) {
	te := staleSetup(t)
	te.fsys.unlinkFail = map[string]error{p("home", ".b"): errors.New("busy")}
	if c := te.run("unlink", "--stale", "--yes"); c != 1 {
		t.Fatalf("want 1, got %d", c)
	}
	if !strings.Contains(te.out.String(), "! failed ~/.b") || !strings.Contains(te.out.String(), "busy") {
		t.Errorf("failure not shown:\n%s", te.out)
	}
	if !te.isLink(p("home", ".b")) || !te.recorded(t, p("home", ".b")) {
		t.Error("link or record lost after a failed removal")
	}
}

func TestUnlinkStateWriteFailure(t *testing.T) {
	te := staleSetup(t)
	te.fsys.replaceFail = errors.New("disk full")
	if c := te.run("unlink", "--stale", "--yes"); c != 1 {
		t.Fatalf("want 1, got %d", c)
	}
	if te.isLink(p("home", ".b")) {
		t.Error("the removed link must stay removed")
	}
	if !strings.Contains(te.errOut.String(), "warning") {
		t.Errorf("stderr: %q", te.errOut)
	}
}

func TestUnlinkUsage(t *testing.T) {
	te := staleSetup(t)
	if c := te.run("unlink"); c != 2 {
		t.Errorf("no arguments: want 2, got %d", c)
	}
	if c := te.run("unlink", "~/.b", "--stale"); c != 2 {
		t.Errorf("target with --stale: want 2, got %d", c)
	}
	if c := te.run("unlink", "~/.b"); c != 1 || !strings.Contains(te.errOut.String(), "not implemented") {
		t.Errorf("target: %d %q", c, te.errOut)
	}
	if !te.isLink(p("home", ".b")) {
		t.Error("something was removed")
	}
}
