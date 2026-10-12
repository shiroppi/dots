package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	dotsfs "github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/statefile"
)

var stateT0 = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func stateSetup(t *testing.T) *testEnv {
	t.Helper()
	te := setupEnv(t)
	te.app.Now = func() time.Time { return stateT0 }
	write := func(path, content string) {
		if err := dotsfs.WriteFile(te.fsys, path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(p("dots.toml"), "[dots]\n\"~/.a\" = \"a\"\n")
	write(p("a"), "a")
	return te
}

func (te *testEnv) store() *statefile.Store {
	return &statefile.Store{FS: te.fsys, Path: te.app.StatePath, GOOS: te.app.Env.GOOS}
}

func (te *testEnv) loadState(t *testing.T) *statefile.State {
	t.Helper()
	st, err := te.store().Load()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestApplyRecordsLinks(t *testing.T) {
	te := stateSetup(t)
	if c := te.run("apply"); c != 0 {
		t.Fatalf("apply: %d %s", c, te.errOut)
	}
	st := te.loadState(t)
	if len(st.Links) != 1 {
		t.Fatalf("want 1 record, got %v", st.Links)
	}
	l := st.Links[0]
	want, err := te.fsys.Readlink(p("home", ".a"))
	if err != nil {
		t.Fatal(err)
	}
	if l.Target != p("home", ".a") || l.Link != want || l.Repository != root() || l.Source != "a" || !l.RecordedAt.Equal(stateT0) {
		t.Errorf("unexpected record %+v", l)
	}

	// A later apply keeps the original recording time.
	later := stateT0.Add(time.Hour)
	te.app.Now = func() time.Time { return later }
	if c := te.run("apply"); c != 0 {
		t.Fatalf("apply: %d", c)
	}
	if got := te.loadState(t).Links[0].RecordedAt; !got.Equal(stateT0) {
		t.Errorf("recorded_at changed to %v", got)
	}
}

func TestApplyRecordsExistingValidLink(t *testing.T) {
	te := stateSetup(t)
	if err := te.fsys.MkdirAll(p("home"), 0o755); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(p("home"), p("a"))
	if err != nil {
		t.Fatal(err)
	}
	if err := te.fsys.Symlink(rel, p("home", ".a")); err != nil {
		t.Fatal(err)
	}
	if c := te.run("apply"); c != 0 {
		t.Fatalf("apply: %d", c)
	}
	st := te.loadState(t)
	if len(st.Links) != 1 || st.Links[0].Link != rel {
		t.Fatalf("got %v", st.Links)
	}
}

func TestApplyDropsRecordsOfMissingTargets(t *testing.T) {
	te := stateSetup(t)
	seed := &statefile.State{GOOS: te.app.Env.GOOS}
	seed.Record(statefile.Link{Target: p("home", ".old"), Link: "x", Repository: root(), Source: "old", RecordedAt: stateT0})
	if err := te.store().Save(seed); err != nil {
		t.Fatal(err)
	}
	if c := te.run("apply"); c != 0 {
		t.Fatalf("apply: %d", c)
	}
	st := te.loadState(t)
	if len(st.Links) != 1 || st.Links[0].Target != p("home", ".a") {
		t.Fatalf("got %v", st.Links)
	}
}

func TestDryRunAndStatusDoNotTouchState(t *testing.T) {
	te := stateSetup(t)
	te.run("apply", "--dry-run")
	te.run("status")
	if ok, _ := dotsfs.Exists(te.fsys, te.app.StatePath); ok {
		t.Fatal("state file was created")
	}

	// With a broken file in place neither command moves or rewrites it.
	if err := te.fsys.MkdirAll(filepath.Dir(te.app.StatePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := dotsfs.WriteFile(te.fsys, te.app.StatePath, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snap(te.fsys, p("state"))
	te.run("apply", "--dry-run")
	te.run("status")
	if before != snap(te.fsys, p("state")) {
		t.Fatal("state directory changed")
	}
}

func TestApplyBrokenStateMovedAside(t *testing.T) {
	te := stateSetup(t)
	if err := te.fsys.MkdirAll(filepath.Dir(te.app.StatePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := dotsfs.WriteFile(te.fsys, te.app.StatePath, []byte("{garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c := te.run("apply"); c != 0 {
		t.Fatalf("apply: %d %s", c, te.errOut)
	}
	aside := te.app.StatePath + ".broken-20261007T120000Z"
	data, err := dotsfs.ReadFile(te.fsys, aside)
	if err != nil || string(data) != "{garbage" {
		t.Fatalf("aside file: %q, %v", data, err)
	}
	if !strings.Contains(te.errOut.String(), ".broken-20261007T120000Z") {
		t.Errorf("stderr should name the aside file: %q", te.errOut.String())
	}
	if st := te.loadState(t); len(st.Links) != 1 {
		t.Fatalf("new state: %v", st.Links)
	}
}

func TestApplyNewerStateRefusesBeforeChanges(t *testing.T) {
	te := stateSetup(t)
	if err := te.fsys.MkdirAll(filepath.Dir(te.app.StatePath), 0o700); err != nil {
		t.Fatal(err)
	}
	const doc = `{"version":2,"links":[]}`
	if err := dotsfs.WriteFile(te.fsys, te.app.StatePath, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	if c := te.run("apply"); c != 1 {
		t.Fatalf("want exit 1, got %d", c)
	}
	if ok, _ := dotsfs.Exists(te.fsys, p("home", ".a")); ok {
		t.Error("link was created despite the newer state file")
	}
	data, _ := dotsfs.ReadFile(te.fsys, te.app.StatePath)
	if string(data) != doc {
		t.Errorf("state file rewritten: %q", data)
	}
}

func TestApplyStateWriteFailureKeepsLinks(t *testing.T) {
	te := stateSetup(t)
	// The parent of the state directory is a regular file.
	te.app.StatePath = filepath.Join(p("a"), "dots", "state.json")
	if c := te.run("apply"); c != 1 {
		t.Fatalf("want exit 1, got %d", c)
	}
	if _, err := te.fsys.Readlink(p("home", ".a")); err != nil {
		t.Errorf("link was rolled back: %v", err)
	}
	if !strings.Contains(te.errOut.String(), "warning") {
		t.Errorf("want a warning, got %q", te.errOut.String())
	}
}
