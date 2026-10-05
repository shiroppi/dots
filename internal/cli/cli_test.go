package cli

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shiroppi/dots/internal/apply"
	"github.com/shiroppi/dots/internal/backup"
	"github.com/shiroppi/dots/internal/config"
	dotsfs "github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/platform"
)

func root() string {
	if runtime.GOOS == "windows" {
		return `C:\repo`
	}
	return "/repo"
}

func p(parts ...string) string { return filepath.Join(append([]string{root()}, parts...)...) }

type fakePrompter struct {
	decisions []apply.Decision
	err       error
	called    bool
}

func (f *fakePrompter) Confirm(it apply.Item) (apply.Decision, error) {
	f.called = true
	if f.err != nil {
		return 0, f.err
	}
	if len(f.decisions) == 0 {
		return apply.No, nil
	}
	d := f.decisions[0]
	if len(f.decisions) > 1 {
		f.decisions = f.decisions[1:]
	}
	return d, nil
}

type failFS struct {
	dotsfs.Manager
	symlinkFail map[string]error
	removeFail  map[string]error
}

func (f *failFS) Symlink(oldname, newname string) error {
	if err, ok := f.symlinkFail[newname]; ok {
		return err
	}
	return f.Manager.Symlink(oldname, newname)
}

func (f *failFS) RemoveAll(path string) error {
	if err, ok := f.removeFail[path]; ok {
		return err
	}
	for k, err := range f.removeFail {
		if k == "aside" && strings.Contains(path, ".dots-aside-") {
			return err
		}
	}
	return f.Manager.RemoveAll(path)
}

type testEnv struct {
	app    *App
	fsys   *failFS
	out    *bytes.Buffer
	errOut *bytes.Buffer
	editor []string
}

func setupEnv(t *testing.T) *testEnv {
	t.Helper()
	m := dotsfs.NewMem()
	if err := m.MkdirAll(p("home"), 0o755); err != nil {
		t.Fatal(err)
	}
	ffs := &failFS{Manager: m, symlinkFail: make(map[string]error), removeFail: make(map[string]error)}
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	env := platform.Env{
		GOOS: runtime.GOOS,
		Home: p("home"),
		Cwd:  root(),
		Vars: map[string]string{"LOCALAPPDATA": p("localappdata"), "XDG_DATA_HOME": p("xdg")},
	}
	te := &testEnv{
		fsys:   ffs,
		out:    out,
		errOut: errOut,
	}
	te.app = &App{
		FS:          ffs,
		Env:         env,
		Stdin:       new(bytes.Buffer),
		Stdout:      out,
		Stderr:      errOut,
		Interactive: false,
		Color:       false,
		BackupDir:   p("backup"),
		RunEditor: func(argv []string) error {
			te.editor = argv
			return nil
		},
	}
	return te
}

func (te *testEnv) run(args ...string) int {
	te.out.Reset()
	te.errOut.Reset()
	cmd := NewRootCmd(te.app, "test")
	cmd.SetArgs(args)
	err := cmd.Execute()
	return ExitCode(err, te.errOut)
}

func snap(m dotsfs.Manager, dir string) string {
	var paths []string
	var walk func(string)
	walk = func(d string) {
		entries, err := m.ReadDir(d)
		if err != nil {
			return
		}
		for _, e := range entries {
			pp := filepath.Join(d, e.Name())
			paths = append(paths, pp)
			if e.IsDir() {
				walk(pp)
			}
		}
	}
	walk(dir)
	return strings.Join(paths, "\n")
}

func TestVersion(t *testing.T) {
	te := setupEnv(t)
	code := te.run("--version")
	if code != 0 {
		t.Errorf("want 0, got %d", code)
	}
	if !strings.Contains(te.out.String(), "dots version test") {
		t.Errorf("want 'dots version test', got %q", te.out.String())
	}
}

func TestInit(t *testing.T) {
	te := setupEnv(t)
	code := te.run("init")
	if code != 0 {
		t.Errorf("want 0, got %d", code)
	}
	cfgPath := p(config.FileName)
	content, err := dotsfs.ReadFile(te.fsys, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != InitTemplate {
		t.Errorf("content mismatch")
	}
	cfg, err := config.Parse(content)
	if err != nil || len(cfg.Dots) != 0 || len(cfg.Auto) != 0 {
		t.Errorf("expected 0 rules, got err=%v cfg=%+v", err, cfg)
	}
	code2 := te.run("init")
	if code2 != 1 {
		t.Errorf("want 1, got %d", code2)
	}
	if !strings.Contains(te.errOut.String(), "refusing to overwrite") {
		t.Errorf("want refusal message, got %q", te.errOut.String())
	}
}

func TestConfigDiscoveryAndMissing(t *testing.T) {
	te := setupEnv(t)
	code := te.run("doctor")
	if code != 1 {
		t.Errorf("want 1, got %d", code)
	}
	if !strings.Contains(te.errOut.String(), "not found") {
		t.Errorf("want 'not found', got %q", te.errOut.String())
	}
	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte("[dots]\n"), 0o644)
	te.fsys.MkdirAll(p("sub", "dir"), 0o755)
	te.app.Env.Cwd = p("sub", "dir")
	code = te.run("doctor")
	if code != 0 {
		t.Errorf("want 0, got %d", code)
	}
}

func TestDoctor(t *testing.T) {
	te := setupEnv(t)
	toml := fmt.Sprintf(`[dots]
"~/.a" = "a"
"~/.b" = "b"
[dots.%s]
"~/.b" = "b2"
`, runtime.GOOS)
	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(toml), 0o644)
	dotsfs.WriteFile(te.fsys, p("a"), []byte("a"), 0o644)
	dotsfs.WriteFile(te.fsys, p("b"), []byte("b"), 0o644)
	dotsfs.WriteFile(te.fsys, p("b2"), []byte("b2"), 0o644)

	snapBefore := snap(te.fsys, p())
	code := te.run("doctor")
	if code != 1 {
		t.Errorf("want 1, got %d", code)
	}
	if !strings.Contains(te.out.String(), "MISSING") {
		t.Errorf("want MISSING, got %q", te.out.String())
	}
	outStr := strings.ToLower(te.out.String())
	if !strings.Contains(outStr, "overridden") {
		t.Errorf("want overrides section, got %q", te.out.String())
	}
	snapAfter := snap(te.fsys, p())
	if snapBefore != snapAfter {
		t.Errorf("doctor modified fs")
	}

	if c := te.run("apply"); c != 0 {
		t.Fatal("apply failed")
	}
	if c := te.run("doctor"); c != 0 {
		t.Errorf("want 0, got %d", c)
	}

	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(`[dots]`+"\n"+`"/etc/x" = "x"`), 0o644)
	code = te.run("doctor")
	if code != 1 {
		t.Errorf("want 1, got %d", code)
	}
	if !strings.Contains(te.errOut.String(), `dots."/etc/x"`) {
		t.Errorf("want rule locator dots.\"/etc/x\", got %q", te.errOut.String())
	}
}

func TestApply(t *testing.T) {
	te := setupEnv(t)
	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte("[dots]\n"), 0o644)
	if c := te.run("apply"); c != 0 || !strings.Contains(te.out.String(), "Nothing to do") {
		t.Errorf("empty config apply failed: %d, %q", c, te.out.String())
	}

	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(`[dots]`+"\n"+`"~/.a" = "a"`+"\n"), 0o644)
	dotsfs.WriteFile(te.fsys, p("a"), []byte("a_content"), 0o644)

	if c := te.run("apply"); c != 0 {
		t.Errorf("apply failed: %d", c)
	}
	link, err := te.fsys.Readlink(p("home", ".a"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(link) {
		t.Errorf("expected relative link, got %s", link)
	}

	te.out.Reset()
	if c := te.run("apply"); c != 0 || !strings.Contains(te.out.String(), "already linked") {
		t.Errorf("idempotent apply failed: %d, %q", c, te.out.String())
	}

	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(`[dots]`+"\n"+`"~/.b" = "b"`+"\n"), 0o644)
	dotsfs.WriteFile(te.fsys, p("b"), []byte("b_content"), 0o644)
	dotsfs.WriteFile(te.fsys, p("home", ".b"), []byte("conflict"), 0o644)

	snapBefore := snap(te.fsys, p())
	if c := te.run("apply", "--dry-run"); c != 0 {
		t.Errorf("dry run failed: %d", c)
	}
	if snapBefore != snap(te.fsys, p()) {
		t.Errorf("dry-run modified fs")
	}

	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(`[dots]`+"\n"+`"~/.c" = "missing"`+"\n"), 0o644)
	if c := te.run("apply", "--dry-run"); c != 1 {
		t.Errorf("want 1, got %d", c)
	}

	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(`[dots]`+"\n"+`"~/.b" = "b"`+"\n"), 0o644)

	if c := te.run("apply"); c != 1 {
		t.Errorf("want 1, got %d", c)
	}
	if archives, _ := te.fsys.ReadDir(p("backup")); len(archives) != 0 {
		t.Errorf("expected no backup, got %d", len(archives))
	}

	te.app.Interactive = true
	prompter := &fakePrompter{}
	te.app.Prompter = prompter

	prompter.decisions = []apply.Decision{apply.Yes}
	if c := te.run("apply"); c != 0 {
		t.Errorf("want 0, got %d", c)
	}
	if archives, _ := te.fsys.ReadDir(p("backup")); len(archives) == 0 {
		t.Errorf("backup not created")
	}

	dotsfs.WriteFile(te.fsys, p("home", ".c"), []byte("conflict"), 0o644)
	dotsfs.WriteFile(te.fsys, p("c"), []byte("c_content"), 0o644)
	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(`[dots]`+"\n"+`"~/.c" = "c"`+"\n"), 0o644)

	prompter.decisions = []apply.Decision{apply.No}
	if c := te.run("apply"); c != 0 {
		t.Errorf("want 0, got %d", c)
	}
	if !strings.Contains(te.out.String(), "skipped") {
		t.Errorf("expected skipped message")
	}

	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(`[dots]`+"\n"+`"~/.d" = "c"`+"\n"+`"~/.e" = "c"`+"\n"), 0o644)
	dotsfs.WriteFile(te.fsys, p("home", ".d"), []byte("conflict"), 0o644)
	dotsfs.WriteFile(te.fsys, p("home", ".e"), []byte("conflict"), 0o644)

	prompter.decisions = []apply.Decision{apply.YesToAll}
	if c := te.run("apply"); c != 0 {
		t.Errorf("want 0, got %d", c)
	}

	dotsfs.WriteFile(te.fsys, p("home", ".d"), []byte("conflict2"), 0o644)
	dotsfs.WriteFile(te.fsys, p("home", ".e"), []byte("conflict2"), 0o644)
	prompter.decisions = []apply.Decision{apply.NoToAll}
	if c := te.run("apply"); c != 0 {
		t.Errorf("want 0, got %d", c)
	}

	te.fsys.RemoveAll(p("home", ".d"))
	dotsfs.WriteFile(te.fsys, p("home", ".d"), []byte("conflict3"), 0o644)
	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(`[dots]`+"\n"+`"~/.d" = "c"`+"\n"), 0o644)
	prompter.err = errors.New("abort")
	if c := te.run("apply"); c != 1 {
		t.Errorf("want 1, got %d", c)
	}

	prompter.err = nil
	prompter.decisions = []apply.Decision{apply.Yes}
	te.fsys.removeFail["aside"] = errors.New("cleanup fail")
	if c := te.run("apply"); c != 0 {
		t.Errorf("want 0 for cleanup warning, got %d", c)
	}
	if !strings.Contains(te.out.String(), "warning") {
		t.Errorf("expected warning, got %q", te.out.String())
	}

	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(`[dots]`+"\n"+`"~/.f" = "a"`+"\n"), 0o644)
	te.fsys.symlinkFail[p("home", ".f")] = errors.New("symlink fail")
	if c := te.run("apply"); c != 1 {
		t.Errorf("want 1 for symlink fail, got %d", c)
	}
	if !strings.Contains(te.errOut.String(), "symlink fail") {
		t.Errorf("expected symlink fail error")
	}

	te.app.Env.Vars = map[string]string{}
	te.app.BackupDir = ""
	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(`[dots]`+"\n"+`"~/.g" = "a"`+"\n"), 0o644)
	if c := te.run("apply"); c != 0 {
		t.Errorf("want 0 with no backup dir on no-conflict apply, got %d", c)
	}
	if c := te.run("doctor"); c != 0 {
		t.Errorf("want 0 for doctor with no backup dir, got %d", c)
	}

	te.app.Interactive = false
	te.app.BackupDir = p("backup")
	prompter.called = false
	prompter.decisions = []apply.Decision{apply.Yes}
	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(`[dots]`+"\n"+`"~/.h" = "a"`+"\n"), 0o644)
	dotsfs.WriteFile(te.fsys, p("home", ".h"), []byte("conflict4"), 0o644)
	archivesBefore, _ := te.fsys.ReadDir(p("backup"))
	if c := te.run("apply"); c != 1 {
		t.Errorf("want 1 for non-interactive conflict, got %d", c)
	}
	if prompter.called {
		t.Errorf("Prompter called despite non-interactive")
	}
	if archivesAfter, _ := te.fsys.ReadDir(p("backup")); len(archivesAfter) != len(archivesBefore) {
		t.Errorf("expected no new backup for non-interactive conflict, before=%d after=%d", len(archivesBefore), len(archivesAfter))
	}
	if content, _ := dotsfs.ReadFile(te.fsys, p("home", ".h")); string(content) != "conflict4" {
		t.Errorf("fs changed despite non-interactive conflict")
	}
}

func TestRestore(t *testing.T) {
	te := setupEnv(t)
	if c := te.run("restore", "arc"); c != 1 {
		t.Errorf("want 1, got %d", c)
	}

	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte("[dots]\n"), 0o644)
	dotsfs.WriteFile(te.fsys, p("home", "r"), []byte("original"), 0o644)

	store := &backup.Store{FS: te.fsys, Dir: te.app.BackupDir}
	arc, err := store.Archive(p("home", "r"))
	if err != nil {
		t.Fatal(err)
	}
	te.fsys.RemoveAll(p("home", "r"))

	snapBefore := snap(te.fsys, p())
	if c := te.run("restore", arc, "--dry-run"); c != 0 {
		t.Errorf("want 0, got %d", c)
	}
	if snapBefore != snap(te.fsys, p()) {
		t.Errorf("dry-run modified fs")
	}

	if c := te.run("restore", arc); c != 0 {
		t.Errorf("want 0, got %d", c)
	}
	if !strings.Contains(te.out.String(), "restored") {
		t.Errorf("expected restored message, got %q", te.out.String())
	}
	content, _ := dotsfs.ReadFile(te.fsys, p("home", "r"))
	if string(content) != "original" {
		t.Errorf("content mismatch")
	}

	te.out.Reset()
	if c := te.run("restore", arc); c != 1 {
		t.Errorf("want 1, got %d", c)
	}

	te.app.Interactive = true
	confirmRestoreValue := false
	te.app.ConfirmRestore = func(plan *backup.RestorePlan) (bool, error) {
		return confirmRestoreValue, nil
	}
	if c := te.run("restore", arc); c != 0 {
		t.Errorf("want 0, got %d", c)
	}
	if !strings.Contains(te.out.String(), "skipped") {
		t.Errorf("expected skipped message")
	}

	confirmRestoreValue = true
	dotsfs.WriteFile(te.fsys, p("home", "r"), []byte("changed"), 0o644)
	te.out.Reset()
	if c := te.run("restore", arc); c != 0 {
		t.Errorf("want 0, got %d", c)
	}
	if !strings.Contains(te.out.String(), "backed up") {
		t.Errorf("expected backed up message")
	}
	content, _ = dotsfs.ReadFile(te.fsys, p("home", "r"))
	if string(content) != "original" {
		t.Errorf("expected original content, got %s", string(content))
	}

	te.app.Env.Cwd = filepath.Dir(arc)
	if c := te.run("restore", filepath.Base(arc)); c != 0 {
		t.Errorf("want 0 for relative arc, got %d", c)
	}

	dotsfs.WriteFile(te.fsys, p("corrupt.tar.gz"), []byte("bad"), 0o644)
	if c := te.run("restore", p("corrupt.tar.gz")); c != 1 {
		t.Errorf("want 1, got %d", c)
	}

	if c := te.run("restore"); c != 1 {
		t.Errorf("want 1, got %d", c)
	}

	te.app.Interactive = false
	confirmRestoreCalled := false
	te.app.ConfirmRestore = func(plan *backup.RestorePlan) (bool, error) {
		confirmRestoreCalled = true
		return true, nil
	}
	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte("[dots]\n"), 0o644)
	dotsfs.WriteFile(te.fsys, p("home", "r"), []byte("conflict_restore"), 0o644)
	if c := te.run("restore", arc); c != 1 {
		t.Errorf("want 1 for non-interactive restore conflict, got %d", c)
	}
	if confirmRestoreCalled {
		t.Errorf("ConfirmRestore called despite non-interactive")
	}
	if content, _ := dotsfs.ReadFile(te.fsys, p("home", "r")); string(content) != "conflict_restore" {
		t.Errorf("fs changed despite non-interactive restore conflict")
	}
}

func TestEdit(t *testing.T) {
	te := setupEnv(t)
	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte("[dots]\n"), 0o644)
	te.app.Env.Vars["EDITOR"] = "myedit --wait"
	if c := te.run("edit"); c != 0 {
		t.Errorf("want 0, got %d", c)
	}
	if len(te.editor) != 3 || te.editor[0] != "myedit" || te.editor[1] != "--wait" || te.editor[2] != p("dots.toml") {
		t.Errorf("editor args mismatch: %v", te.editor)
	}

	te.app.Env.Vars["EDITOR"] = ""
	if c := te.run("edit"); c != 1 {
		t.Errorf("want 1, got %d", c)
	}
	if !strings.Contains(te.errOut.String(), "EDITOR is not set") {
		t.Errorf("want 'EDITOR is not set', got %q", te.errOut.String())
	}

	te.app.Env.Vars["EDITOR"] = "badedit"
	te.app.RunEditor = func(argv []string) error { return errors.New("editor fail") }
	if c := te.run("edit"); c != 1 {
		t.Errorf("want 1, got %d", c)
	}
}

func TestExitCodeFunction(t *testing.T) {
	buf := new(bytes.Buffer)
	if c := ExitCode(nil, buf); c != 0 || buf.Len() != 0 {
		t.Errorf("want 0, got %d output %q", c, buf.String())
	}

	errs := errors.Join(errors.New("a"), errors.New("b"))
	buf.Reset()
	if c := ExitCode(errs, buf); c != 1 {
		t.Errorf("want 1, got %d", c)
	}
	if out := buf.String(); !strings.Contains(out, "dots: a") || !strings.Contains(out, "dots: b") {
		t.Errorf("want dots: a and dots: b, got %q", out)
	}

	buf.Reset()
	if c := ExitCode(silentError{}, buf); c != 1 || buf.Len() != 0 {
		t.Errorf("want 1 and no output, got %d output %q", c, buf.String())
	}
}

func TestAutoDirIgnored(t *testing.T) {
	te := setupEnv(t)
	// Create an auto source that only has ignored files.
	te.fsys.MkdirAll(p("auto-src", "ign"), 0o755)
	dotsfs.WriteFile(te.fsys, p("auto-src", "ign", "ignored.txt"), []byte("ign"), 0o644)

	toml := `
[[auto]]
source = "auto-src"
target = "~/.cfg"
ignore = ["**/*.txt"]
`
	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(toml), 0o644)

	// dry-run apply
	if c := te.run("apply", "--dry-run"); c != 0 {
		t.Errorf("dry run failed: %d", c)
	}
	if !strings.Contains(te.out.String(), "create") || !strings.Contains(te.out.String(), "ign (directory)") {
		t.Errorf("dry-run missing dir create: %q", te.out.String())
	}
	if _, err := te.fsys.Stat(p("home", ".cfg", "ign")); err == nil {
		t.Errorf("dry-run should not create dir")
	}

	// first apply
	if c := te.run("apply"); c != 0 {
		t.Errorf("apply failed: %d", c)
	}
	info, err := te.fsys.Lstat(p("home", ".cfg", "ign"))
	if err != nil || !info.IsDir() {
		t.Errorf("expected directory created: %v", err)
	}

	// doctor
	if c := te.run("doctor"); c != 0 {
		t.Errorf("doctor failed: %d", c)
	}
	if !strings.Contains(te.out.String(), "(directory)") || !strings.Contains(te.out.String(), "[OK]") {
		t.Errorf("doctor output incorrect: %q", te.out.String())
	}

	// second apply
	te.out.Reset()
	if c := te.run("apply"); c != 0 {
		t.Errorf("second apply failed: %d", c)
	}
	if !strings.Contains(te.out.String(), "already ok") {
		t.Errorf("second apply should be ok, got %q", te.out.String())
	}
}

const expectedRootHelp = `
A brief, declarative, flexible dotfiles manager.

Usage:
  dots <command> [options]

Core:
  apply       Apply dotfiles
  doctor      Check configuration and managed files

Usability:
  init        Initialize a dotfiles repository
  restore     Restore files from backups
  edit        Edit dots.toml with $EDITOR

Options:
  -h, --help      Show help
  -v, --version   Show version

Use "dots <command> --help" for more information.
`

func TestHelp(t *testing.T) {
	te := setupEnv(t)

	for _, arg := range []string{"--help", "-h", "help", ""} {
		name := arg
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			var args []string
			if arg != "" {
				args = []string{arg}
			}
			if code := te.run(args...); code != 0 {
				t.Errorf("want 0, got %d", code)
			}
			if te.errOut.Len() != 0 {
				t.Errorf("stderr not empty: %q", te.errOut.String())
			}
			if out := te.out.String(); out != expectedRootHelp {
				t.Errorf("stdout mismatch:\nwant:\n%q\ngot:\n%q", expectedRootHelp, out)
			}
		})
	}

	te.run("--help")
	out := te.out.String()
	for _, word := range []string{"completion", "status", "Plugin"} {
		if strings.Contains(out, word) {
			t.Errorf("root help should not contain %q", word)
		}
	}
	idx1 := strings.Index(out, "apply")
	idx2 := strings.Index(out, "doctor")
	idx3 := strings.Index(out, "init")
	idx4 := strings.Index(out, "restore")
	idx5 := strings.Index(out, "edit")
	if !(idx1 < idx2 && idx2 < idx3 && idx3 < idx4 && idx4 < idx5) {
		t.Errorf("wrong command order in root help")
	}

	for _, arg := range []string{"-v", "--version"} {
		t.Run(arg, func(t *testing.T) {
			if code := te.run(arg); code != 0 {
				t.Errorf("want 0, got %d", code)
			}
			if out := te.out.String(); out != "dots version test\n" {
				t.Errorf("want 'dots version test\\n', got %q", out)
			}
		})
	}

	applyHelp := "Apply dotfiles\n\nUsage:\n  dots apply [options]\n\nOptions:\n      --dry-run   Show what would be done without changing anything\n  -h, --help      Show help\n"
	te.run("apply", "--help")
	if out := te.out.String(); out != applyHelp {
		t.Errorf("apply help mismatch:\nwant:\n%q\ngot:\n%q", applyHelp, out)
	}

	restoreHelp := "Restore files from backups\n\nUsage:\n  dots restore <archive> [options]\n\nOptions:\n      --dry-run   Show what would be restored without changing anything\n  -h, --help      Show help\n"
	te.run("restore", "--help")
	if out := te.out.String(); out != restoreHelp {
		t.Errorf("restore help mismatch:\nwant:\n%q\ngot:\n%q", restoreHelp, out)
	}

	te.run("help", "apply")
	if out := te.out.String(); out != applyHelp {
		t.Errorf("help apply mismatch:\nwant:\n%q\ngot:\n%q", applyHelp, out)
	}
}

func TestUsageErrors(t *testing.T) {
	te := setupEnv(t)

	for _, args := range [][]string{
		{"nosuch"},
		{"apply", "--bad"},
		{"apply", "extra"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if code := te.run(args...); code != 1 {
				t.Errorf("want 1, got %d", code)
			}
			errStr := te.errOut.String()
			if !strings.HasSuffix(errStr, "Run \"dots --help\" for usage.\n") {
				t.Errorf("missing help hint: %q", errStr)
			}
			if strings.Contains(errStr, "Usage:") {
				t.Errorf("stderr contains 'Usage:': %q", errStr)
			}
		})
	}

	te.app.Env.Cwd = p("empty")
	te.fsys.MkdirAll(p("empty"), 0o755)
	if code := te.run("doctor"); code != 1 {
		t.Errorf("want 1, got %d", code)
	}
	errStr := te.errOut.String()
	if strings.Contains(errStr, "Run \"dots --help\" for usage.") {
		t.Errorf("unexpected help hint: %q", errStr)
	}
	if errStr == "" {
		t.Errorf("expected error message")
	}

	te.app.Env.Cwd = root()
	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte("[dots]\n\"~/.x\" = \"x\"\n"), 0o644)
	dotsfs.WriteFile(te.fsys, p("x"), []byte("x"), 0o644)
	te.out.Reset()
	te.errOut.Reset()
	if code := te.run("doctor"); code != 1 {
		t.Errorf("want 1, got %d", code)
	}
	if te.errOut.Len() != 0 {
		t.Errorf("expected no stderr for silent error, got %q", te.errOut.String())
	}
}

func TestCLIHeadersAndRelativeSources(t *testing.T) {
	te := setupEnv(t)
	dotsfs.WriteFile(te.fsys, p("dots.toml"), []byte(`[dots]`+"\n"+`"~/.a" = "a/b"`+"\n"), 0o644)
	te.fsys.MkdirAll(p("a"), 0o755)
	dotsfs.WriteFile(te.fsys, p("a", "b"), []byte("content"), 0o644)

	relSrc := filepath.Join("a", "b")

	// Doctor
	if c := te.run("doctor"); c != 1 {
		t.Errorf("want 1, got %d", c)
	}
	out := te.out.String()
	if !strings.Contains(out, "Repository: "+root()) {
		t.Errorf("missing Repository header in doctor: %q", out)
	}
	if !strings.Contains(out, "-> "+relSrc+" ") {
		t.Errorf("missing relative source in doctor: %q", out)
	}

	// Apply dry-run
	if c := te.run("apply", "--dry-run"); c != 0 {
		t.Errorf("want 0, got %d", c)
	}
	out = te.out.String()
	if !strings.Contains(out, "Repository: "+root()) {
		t.Errorf("missing Repository header in apply --dry-run: %q", out)
	}
	if !strings.Contains(out, "-> "+relSrc+" ") {
		t.Errorf("missing relative source in apply --dry-run: %q", out)
	}

	// Apply report
	if c := te.run("apply"); c != 0 {
		t.Errorf("want 0, got %d", c)
	}
	out = te.out.String()
	if !strings.Contains(out, "Repository: "+root()) {
		t.Errorf("missing Repository header in apply: %q", out)
	}
	if !strings.Contains(out, "-> "+relSrc+" ") {
		t.Errorf("missing relative source in apply: %q", out)
	}

	// Restore dry-run
	store := &backup.Store{FS: te.fsys, Dir: te.app.BackupDir}
	arc, err := store.Archive(p("home", ".a"))
	if err != nil {
		t.Fatal(err)
	}
	if c := te.run("restore", arc, "--dry-run"); c != 0 {
		t.Errorf("want 0, got %d", c)
	}
	out = te.out.String()
	if strings.Contains(out, "Repository:") {
		t.Errorf("unexpected Repository header in restore --dry-run: %q", out)
	}
}
