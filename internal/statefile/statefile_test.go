package statefile

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/platform"
)

func root() string {
	if runtime.GOOS == "windows" {
		return `C:\`
	}
	return "/"
}

func p(parts ...string) string { return filepath.Join(append([]string{root()}, parts...)...) }

func mustNil(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

var t0 = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func link(target, text string) Link {
	return Link{Target: target, Link: text, Repository: p("repo"), Source: "src", RecordedAt: t0}
}

func newStore(goos string) (*Store, fs.Manager) {
	m := fs.NewMem()
	return &Store{FS: m, Path: p("state", "dots", FileName), GOOS: goos}, m
}

func TestDefaultPath(t *testing.T) {
	env := platform.Env{GOOS: "linux", Home: p("home"), Vars: map[string]string{"XDG_STATE_HOME": p("xs")}}
	got, err := DefaultPath(env)
	mustNil(t, err)
	if want := p("xs", "dots", "state.json"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if _, err := DefaultPath(platform.Env{GOOS: "linux"}); err == nil {
		t.Fatal("want error without home")
	}
}

func TestLoadMissing(t *testing.T) {
	s, _ := newStore("linux")
	st, err := s.Load()
	mustNil(t, err)
	if len(st.Links) != 0 {
		t.Fatalf("want empty, got %v", st.Links)
	}
}

func TestSaveLoadSortOverwrite(t *testing.T) {
	s, m := newStore("linux")
	st := &State{GOOS: "linux"}
	st.Record(link(p("home", ".zshrc"), "z1"))
	st.Record(link(p("home", ".bashrc"), "b"))
	st.Record(link(p("home", ".zshrc"), "z2")) // overwrite
	mustNil(t, s.Save(st))

	got, err := s.Load()
	mustNil(t, err)
	if len(got.Links) != 2 {
		t.Fatalf("want 2 links, got %v", got.Links)
	}
	if got.Links[0].Target != p("home", ".bashrc") || got.Links[1].Target != p("home", ".zshrc") {
		t.Fatalf("not sorted: %v", got.Links)
	}
	if got.Links[1].Link != "z2" {
		t.Fatalf("not overwritten: %v", got.Links[1])
	}
	if !got.Links[0].RecordedAt.Equal(t0) {
		t.Fatalf("recorded_at = %v", got.Links[0].RecordedAt)
	}

	// Saving again over an existing file works and leaves no temp files.
	st.Remove(p("home", ".bashrc"))
	mustNil(t, s.Save(st))
	got, err = s.Load()
	mustNil(t, err)
	if len(got.Links) != 1 {
		t.Fatalf("want 1 link, got %v", got.Links)
	}
	entries, err := m.ReadDir(filepath.Dir(s.Path))
	mustNil(t, err)
	if len(entries) != 1 || entries[0].Name() != FileName {
		t.Fatalf("unexpected directory content: %v", entries)
	}
	if runtime.GOOS != "windows" {
		fi, err := m.Lstat(s.Path)
		mustNil(t, err)
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("file mode %v", fi.Mode().Perm())
		}
		di, err := m.Lstat(filepath.Dir(s.Path))
		mustNil(t, err)
		if di.Mode().Perm() != 0o700 {
			t.Errorf("dir mode %v", di.Mode().Perm())
		}
	}
}

func TestSaveEmptyWritesArray(t *testing.T) {
	s, m := newStore("linux")
	mustNil(t, s.Save(&State{}))
	data, err := fs.ReadFile(m, s.Path)
	mustNil(t, err)
	if !strings.Contains(string(data), `"links": []`) || !strings.Contains(string(data), `"version": 1`) {
		t.Fatalf("unexpected content %s", data)
	}
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
}

func TestFileFormat(t *testing.T) {
	s, m := newStore("linux")
	st := &State{}
	st.Record(Link{Target: "/home/user/.vimrc", Link: "dotfiles/vimrc", Repository: "/home/user/dotfiles", Source: "vimrc", RecordedAt: t0})
	mustNil(t, s.Save(st))
	data, err := fs.ReadFile(m, s.Path)
	mustNil(t, err)
	want := `{
  "version": 1,
  "links": [
    {
      "target": "/home/user/.vimrc",
      "link": "dotfiles/vimrc",
      "repository": "/home/user/dotfiles",
      "source": "vimrc",
      "recorded_at": "2026-10-07T12:00:00Z"
    }
  ]
}
`
	if string(data) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", data, want)
	}
}

func TestTargetCaseFolding(t *testing.T) {
	for goos, folds := range map[string]bool{"linux": false, "darwin": true, "windows": true} {
		st := &State{GOOS: goos}
		st.Record(link(p("Home", ".A"), "x"))
		st.Record(link(p("home", ".a"), "y"))
		want := 2
		if folds {
			want = 1
		}
		if len(st.Links) != want {
			t.Errorf("%s: want %d record(s), got %v", goos, want, st.Links)
		}
		if _, ok := st.Find(p("HOME", ".a")); ok != folds {
			t.Errorf("%s: Find case-insensitive = %v, want %v", goos, ok, folds)
		}
	}
}

func TestLoadIgnoresUnknownKeys(t *testing.T) {
	s, m := newStore("linux")
	mustNil(t, m.MkdirAll(filepath.Dir(s.Path), 0o700))
	doc := `{"version":1,"copies":[{"x":1}],"future":true,"links":[
	  {"target":"/t","link":"l","repository":"/r","source":"s","recorded_at":"2026-10-07T12:00:00Z","extra":1}]}`
	mustNil(t, fs.WriteFile(m, s.Path, []byte(doc), 0o600))
	st, err := s.Load()
	mustNil(t, err)
	if len(st.Links) != 1 || st.Links[0].Target != "/t" {
		t.Fatalf("got %v", st.Links)
	}
}

func TestSavePreservesUnknownKeys(t *testing.T) {
	s, m := newStore("linux")
	mustNil(t, m.MkdirAll(filepath.Dir(s.Path), 0o700))
	doc := `{"version":1,"links":[],"future":true,"copies":[{"x":"a&b","n":1}]}`
	mustNil(t, fs.WriteFile(m, s.Path, []byte(doc), 0o600))
	st, err := s.Load()
	mustNil(t, err)
	st.Record(link("/t", "l"))
	mustNil(t, s.Save(st))

	data, err := fs.ReadFile(m, s.Path)
	mustNil(t, err)
	var top map[string]json.RawMessage
	mustNil(t, json.Unmarshal(data, &top))
	if string(top["future"]) != "true" {
		t.Errorf("future = %s", top["future"])
	}
	var copies []map[string]any
	mustNil(t, json.Unmarshal(top["copies"], &copies))
	if len(copies) != 1 || copies[0]["x"] != "a&b" || copies[0]["n"] != float64(1) {
		t.Errorf("copies = %s", top["copies"])
	}
	if !strings.HasPrefix(string(data), "{\n  \"version\": 1,\n  \"links\": [") {
		t.Errorf("version and links should come first:\n%s", data)
	}
	back, err := s.Load()
	mustNil(t, err)
	if len(back.Links) != 1 {
		t.Fatalf("links = %v", back.Links)
	}
	// Saving a fresh state does not invent keys.
	mustNil(t, s.Save(&State{}))
	data, _ = fs.ReadFile(m, s.Path)
	if strings.Contains(string(data), "future") {
		t.Errorf("unexpected key in a fresh state:\n%s", data)
	}
}

func TestSaveReplacesExistingAtomically(t *testing.T) {
	s, m := newStore("linux")
	for i := 0; i < 3; i++ {
		st := &State{}
		st.Record(link(p("t", string(rune('a'+i))), "l"))
		mustNil(t, s.Save(st))
	}
	back, err := s.Load()
	mustNil(t, err)
	if len(back.Links) != 1 || back.Links[0].Target != p("t", "c") {
		t.Fatalf("got %v", back.Links)
	}
	infos, err := m.ReadDir(filepath.Dir(s.Path))
	mustNil(t, err)
	if len(infos) != 1 {
		t.Errorf("leftover files in the state directory: %v", infos)
	}
}

func TestLoadBroken(t *testing.T) {
	cases := map[string]string{
		"invalid json":    `{not json`,
		"empty":           ``,
		"array":           `[]`,
		"null":            `null`,
		"no version":      `{"links":[]}`,
		"bad version":     `{"version":"1","links":[]}`,
		"zero version":    `{"version":0,"links":[]}`,
		"no links":        `{"version":1}`,
		"links not array": `{"version":1,"links":{}}`,
		"links null":      `{"version":1,"links":null}`,
		"missing key":     `{"version":1,"links":[{"target":"/t","link":"l","repository":"/r","source":"s"}]}`,
		"empty target":    `{"version":1,"links":[{"target":"","link":"l","repository":"/r","source":"s","recorded_at":"2026-10-07T12:00:00Z"}]}`,
		"bad time":        `{"version":1,"links":[{"target":"/t","link":"l","repository":"/r","source":"s","recorded_at":"yesterday"}]}`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			s, m := newStore("linux")
			mustNil(t, m.MkdirAll(filepath.Dir(s.Path), 0o700))
			mustNil(t, fs.WriteFile(m, s.Path, []byte(doc), 0o600))
			st, err := s.Load()
			if !errors.Is(err, ErrBroken) {
				t.Fatalf("want ErrBroken, got %v", err)
			}
			if errors.Is(err, ErrNewerVersion) {
				t.Fatal("must not be a version error")
			}
			if st == nil || len(st.Links) != 0 {
				t.Fatalf("want empty state, got %v", st)
			}
		})
	}
}

func TestLoadNewerVersion(t *testing.T) {
	s, m := newStore("linux")
	mustNil(t, m.MkdirAll(filepath.Dir(s.Path), 0o700))
	// A newer format may be shaped differently; the version decides first.
	mustNil(t, fs.WriteFile(m, s.Path, []byte(`{"version":2,"links":"other shape"}`), 0o600))
	st, err := s.Load()
	if !errors.Is(err, ErrNewerVersion) || errors.Is(err, ErrBroken) {
		t.Fatalf("want ErrNewerVersion only, got %v", err)
	}
	if len(st.Links) != 0 {
		t.Fatalf("want empty state, got %v", st.Links)
	}
}

func TestMoveAside(t *testing.T) {
	s, m := newStore("linux")
	mustNil(t, m.MkdirAll(filepath.Dir(s.Path), 0o700))
	mustNil(t, fs.WriteFile(m, s.Path, []byte("garbage"), 0o600))
	now := time.Date(2026, 10, 8, 9, 10, 11, 0, time.UTC)

	dest, err := s.MoveAside(now)
	mustNil(t, err)
	if want := s.Path + ".broken-20261008T091011Z"; dest != want {
		t.Fatalf("got %q want %q", dest, want)
	}
	if ok, _ := fs.Exists(m, s.Path); ok {
		t.Fatal("original still exists")
	}
	data, err := fs.ReadFile(m, dest)
	mustNil(t, err)
	if string(data) != "garbage" {
		t.Fatalf("content %q", data)
	}

	// Same second again: a different name is chosen and nothing is overwritten.
	mustNil(t, fs.WriteFile(m, s.Path, []byte("garbage2"), 0o600))
	dest2, err := s.MoveAside(now)
	mustNil(t, err)
	if dest2 == dest {
		t.Fatal("overwrote the earlier file")
	}
	data, err = fs.ReadFile(m, dest)
	mustNil(t, err)
	if string(data) != "garbage" {
		t.Fatalf("earlier content changed: %q", data)
	}
}

func TestPruneMissing(t *testing.T) {
	_, m := newStore("linux")
	mustNil(t, m.MkdirAll(p("home"), 0o755))
	mustNil(t, m.Symlink("x", p("home", ".kept")))
	st := &State{}
	st.Record(link(p("home", ".kept"), "x"))
	st.Record(link(p("home", ".gone"), "x"))
	removed, err := st.PruneMissing(m)
	mustNil(t, err)
	if len(removed) != 1 || removed[0].Target != p("home", ".gone") {
		t.Fatalf("removed %v", removed)
	}
	if len(st.Links) != 1 || st.Links[0].Target != p("home", ".kept") {
		t.Fatalf("kept %v", st.Links)
	}
}
