package statefile

import (
	"testing"

	"github.com/shiroppi/dots/internal/fs"
)

func TestClassify(t *testing.T) {
	m := fs.NewMem()
	mustNil(t, m.MkdirAll(p("home"), 0o755))
	repo := p("repo")

	mustNil(t, m.Symlink("rel/a", p("home", ".link")))    // matches record
	mustNil(t, m.Symlink("other", p("home", ".changed"))) // different string
	mustNil(t, fs.WriteFile(m, p("home", ".file"), nil, 0o644))
	mustNil(t, m.MkdirAll(p("home", ".dir"), 0o755))
	mustNil(t, m.Symlink("rel/a", p("home", ".current"))) // still configured
	mustNil(t, m.Symlink("rel/a", p("home", ".foreign"))) // other repository
	mustNil(t, fs.WriteFile(m, p("home", "blocker"), nil, 0o644))

	rec := func(name, text, repository string) Link {
		l := link(p("home", name), text)
		l.Repository = repository
		return l
	}
	links := []Link{
		rec(".link", "rel/a", repo),
		rec(".changed", "rel/a", repo),
		rec(".file", "rel/a", repo),
		rec(".dir", "rel/a", repo),
		rec(".gone", "rel/a", repo),
		{Target: p("home", "blocker", "child"), Link: "x", Repository: repo, Source: "s", RecordedAt: t0}, // ENOTDIR
		rec(".current", "rel/a", repo),
		rec(".foreign", "rel/a", p("elsewhere")),
	}
	got, err := Classify(m, "linux", links, repo, []string{p("home", ".current")})
	mustNil(t, err)

	want := map[string]StaleKind{
		p("home", ".link"):            StaleLink,
		p("home", ".changed"):         StaleChanged,
		p("home", ".file"):            StaleChanged,
		p("home", ".dir"):             StaleChanged,
		p("home", ".gone"):            StaleGone,
		p("home", "blocker", "child"): StaleGone,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries: %+v", len(got), got)
	}
	for _, s := range got {
		if w, ok := want[s.Record.Target]; !ok || w != s.Kind {
			t.Errorf("%s: got %v, want %v", s.Record.Target, s.Kind, w)
		}
	}
	// Input order is kept and Current is filled for symlinks.
	if got[0].Record.Target != p("home", ".link") || got[1].Current != "other" {
		t.Errorf("order/current wrong: %+v", got)
	}
}

func TestClassifyDoesNotFollowLinks(t *testing.T) {
	m := fs.NewMem()
	mustNil(t, m.MkdirAll(p("home"), 0o755))
	mustNil(t, m.Symlink("nowhere", p("home", ".broken"))) // dangling
	got, err := Classify(m, "linux", []Link{link(p("home", ".broken"), "nowhere")}, p("repo"), nil)
	mustNil(t, err)
	if len(got) != 1 || got[0].Kind != StaleLink {
		t.Fatalf("got %+v", got)
	}
}

func TestClassifyCaseFolding(t *testing.T) {
	m := fs.NewMem()
	mustNil(t, m.MkdirAll(p("home"), 0o755))
	mustNil(t, m.Symlink("a", p("home", ".x")))
	l := link(p("home", ".x"), "a")
	l.Repository = p("Repo")

	// Linux: different case means a different repository, so never stale.
	got, err := Classify(m, "linux", []Link{l}, p("repo"), nil)
	mustNil(t, err)
	if len(got) != 0 {
		t.Fatalf("linux: %+v", got)
	}
	// macOS: same repository; a target configured under a different case is
	// not stale.
	got, err = Classify(m, "darwin", []Link{l}, p("repo"), []string{p("HOME", ".X")})
	mustNil(t, err)
	if len(got) != 0 {
		t.Fatalf("darwin: %+v", got)
	}
	got, err = Classify(m, "darwin", []Link{l}, p("repo"), nil)
	mustNil(t, err)
	if len(got) != 1 || got[0].Kind != StaleLink {
		t.Fatalf("darwin stale: %+v", got)
	}
}

func TestClassifyWindowsSeparators(t *testing.T) {
	m := fs.NewMem()
	mustNil(t, m.MkdirAll(p("home"), 0o755))
	mustNil(t, m.Symlink(`..\repo\a`, p("home", ".x")))
	recSlash := link(p("home", ".x"), "../repo/a")

	got, err := Classify(m, "windows", []Link{recSlash}, p("repo"), nil)
	mustNil(t, err)
	if len(got) != 1 || got[0].Kind != StaleLink {
		t.Fatalf("windows: %+v", got)
	}
	// Other systems compare the strings exactly.
	got, err = Classify(m, "linux", []Link{recSlash}, p("repo"), nil)
	mustNil(t, err)
	if len(got) != 1 || got[0].Kind != StaleChanged {
		t.Fatalf("linux: %+v", got)
	}
}
