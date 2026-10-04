package state

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/model"
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
	mustNil(t, fs.WriteFile(m, p("src", "vimrc"), []byte("x"), 0o644))
	mustNil(t, m.MkdirAll(p("src", "nvim"), 0o755))
	return m
}

func entry(source, target string) model.Entry {
	return model.Entry{Source: source, Target: target, Origin: model.Origin{Layer: model.LayerDots, Rule: "test"}}
}

func TestStatusString(t *testing.T) {
	for s, want := range map[Status]string{NotExist: "NotExist", ValidLink: "ValidLink", InvalidLink: "InvalidLink", FileOrDir: "FileOrDir"} {
		if s.String() != want {
			t.Errorf("%d: got %s want %s", int(s), s, want)
		}
	}
}

func TestRelLink(t *testing.T) {
	got, err := RelLink(p("src", "vimrc"), p("home", ".vimrc"))
	mustNil(t, err)
	want := filepath.Join("..", "src", "vimrc")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got, err = RelLink(p("src", "vimrc"), p("src", "link"))
	mustNil(t, err)
	if got != "vimrc" {
		t.Fatalf("same dir: got %q", got)
	}
	got, err = RelLink(p("src", "vimrc"), p("a", "b", "c", "l"))
	mustNil(t, err)
	if want := filepath.Join("..", "..", "..", "src", "vimrc"); got != want {
		t.Fatalf("deep: got %q want %q", got, want)
	}
}

func TestRelLinkCrossVolume(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("volumes only exist on Windows")
	}
	_, err := RelLink(`C:\repo\x`, `D:\home\.x`)
	if !errors.Is(err, ErrCrossVolume) {
		t.Fatalf("want ErrCrossVolume, got %v", err)
	}
	if _, err := RelLink(`C:\repo\x`, `c:\home\.x`); err != nil {
		t.Fatalf("drive letter case must not matter: %v", err)
	}
}

func TestEvaluate(t *testing.T) {
	src := p("src", "vimrc")
	tests := []struct {
		name    string
		setup   func(t *testing.T, m fs.Manager)
		entry   model.Entry
		status  Status
		wantErr error
	}{
		{name: "not exist", entry: entry(src, p("home", ".vimrc")), status: NotExist},
		{
			name: "valid relative link",
			setup: func(t *testing.T, m fs.Manager) {
				mustNil(t, m.Symlink(filepath.Join("..", "src", "vimrc"), p("home", ".vimrc")))
			},
			entry: entry(src, p("home", ".vimrc")), status: ValidLink,
		},
		{
			name: "valid relative link with dots",
			setup: func(t *testing.T, m fs.Manager) {
				mustNil(t, m.Symlink(filepath.Join("..", "home", "..", "src", "vimrc"), p("home", ".vimrc")))
			},
			entry: entry(src, p("home", ".vimrc")), status: ValidLink,
		},
		{
			name: "valid absolute link",
			setup: func(t *testing.T, m fs.Manager) {
				if runtime.GOOS == "windows" {
					t.Skip("memfs mangles absolute symlink targets on Windows; covered by TestEvaluateRealOS")
				}
				mustNil(t, m.Symlink(src, p("home", ".vimrc")))
			},
			entry: entry(src, p("home", ".vimrc")), status: ValidLink,
		},
		{
			name: "link to other source",
			setup: func(t *testing.T, m fs.Manager) {
				mustNil(t, m.Symlink(filepath.Join("..", "src", "nvim"), p("home", ".vimrc")))
			},
			entry: entry(src, p("home", ".vimrc")), status: InvalidLink,
		},
		{
			name: "broken target link is invalid not notexist",
			setup: func(t *testing.T, m fs.Manager) {
				mustNil(t, m.Symlink("nowhere", p("home", ".vimrc")))
			},
			entry: entry(src, p("home", ".vimrc")), status: InvalidLink,
		},
		{
			name: "target link to source dir resolves via parent not cwd",
			setup: func(t *testing.T, m fs.Manager) {
				// "src/vimrc" relative to home/ is home/src/vimrc, not the source.
				mustNil(t, m.Symlink(filepath.Join("src", "vimrc"), p("home", ".vimrc")))
			},
			entry: entry(src, p("home", ".vimrc")), status: InvalidLink,
		},
		{
			name: "regular file",
			setup: func(t *testing.T, m fs.Manager) {
				mustNil(t, fs.WriteFile(m, p("home", ".vimrc"), []byte("old"), 0o644))
			},
			entry: entry(src, p("home", ".vimrc")), status: FileOrDir,
		},
		{
			name: "directory",
			setup: func(t *testing.T, m fs.Manager) {
				mustNil(t, m.MkdirAll(p("home", ".vimrc"), 0o755))
			},
			entry: entry(src, p("home", ".vimrc")), status: FileOrDir,
		},
		{
			name: "directory source valid",
			setup: func(t *testing.T, m fs.Manager) {
				mustNil(t, m.Symlink(filepath.Join("..", "src", "nvim"), p("home", "nvim")))
			},
			entry: entry(p("src", "nvim"), p("home", "nvim")), status: ValidLink,
		},
		{
			name: "source symlink is compared as link item",
			setup: func(t *testing.T, m fs.Manager) {
				mustNil(t, m.Symlink("vimrc", p("src", "vimrc-link")))
				// Target points at the resolved destination, not at the link item.
				mustNil(t, m.Symlink(filepath.Join("..", "src", "vimrc"), p("home", ".vimrc")))
			},
			entry: entry(p("src", "vimrc-link"), p("home", ".vimrc")), status: InvalidLink,
		},
		{
			name: "source symlink valid when target points at link item",
			setup: func(t *testing.T, m fs.Manager) {
				mustNil(t, m.Symlink("vimrc", p("src", "vimrc-link")))
				mustNil(t, m.Symlink(filepath.Join("..", "src", "vimrc-link"), p("home", ".vimrc")))
			},
			entry: entry(p("src", "vimrc-link"), p("home", ".vimrc")), status: ValidLink,
		},
		{
			name:    "missing source",
			entry:   entry(p("src", "missing"), p("home", ".x")),
			status:  NotExist,
			wantErr: ErrSourceNotExist,
		},
		{
			name: "broken source link",
			setup: func(t *testing.T, m fs.Manager) {
				mustNil(t, m.Symlink("gone", p("src", "broken")))
			},
			entry: entry(p("src", "broken"), p("home", ".x")), status: NotExist, wantErr: ErrSourceBrokenLink,
		},
		{
			name: "cyclic source link",
			setup: func(t *testing.T, m fs.Manager) {
				mustNil(t, m.Symlink("b", p("src", "a")))
				mustNil(t, m.Symlink("a", p("src", "b")))
			},
			entry: entry(p("src", "a"), p("home", ".x")), status: NotExist, wantErr: ErrSourceLinkLoop,
		},
		{
			name: "unsafe parent is a file",
			setup: func(t *testing.T, m fs.Manager) {
				mustNil(t, fs.WriteFile(m, p("home", "cfg"), []byte("f"), 0o644))
			},
			entry: entry(src, p("home", "cfg", "sub", ".x")), status: NotExist, wantErr: ErrUnsafeParent,
		},
		{
			name: "unsafe parent is a symlink",
			setup: func(t *testing.T, m fs.Manager) {
				mustNil(t, m.Symlink(filepath.Join("..", "src"), p("home", "cfg")))
			},
			entry: entry(src, p("home", "cfg", "sub", ".x")), status: NotExist, wantErr: ErrUnsafeParent,
		},
		{
			name:   "missing ancestors are fine (nearest existing is a dir)",
			entry:  entry(src, p("home", "a", "b", "c", ".x")),
			status: NotExist,
		},
		{
			name:    "target is the source",
			entry:   entry(src, src),
			status:  FileOrDir,
			wantErr: ErrOverlap,
		},
		{
			name:    "target inside source dir",
			entry:   entry(p("src", "nvim"), p("src", "nvim", "x")),
			status:  NotExist,
			wantErr: ErrOverlap,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newFS(t)
			if tt.setup != nil {
				tt.setup(t, m)
			}
			r := Evaluate(m, tt.entry)
			if tt.wantErr == nil {
				if r.Err != nil {
					t.Fatalf("unexpected error: %v", r.Err)
				}
				if r.Link == "" {
					t.Error("Link not set")
				}
			} else if !errors.Is(r.Err, tt.wantErr) {
				t.Fatalf("want %v, got %v", tt.wantErr, r.Err)
			}
			if r.Status != tt.status {
				t.Errorf("status: got %s want %s", r.Status, tt.status)
			}
			if r.Entry != tt.entry {
				t.Errorf("entry not carried through")
			}
		})
	}
}

func TestEvaluateLinkText(t *testing.T) {
	m := newFS(t)
	r := Evaluate(m, entry(p("src", "vimrc"), p("home", ".vimrc")))
	if want := filepath.Join("..", "src", "vimrc"); r.Link != want {
		t.Fatalf("Link: got %q want %q", r.Link, want)
	}
}

func TestEvaluateCaseInsensitive(t *testing.T) {
	old := PathEqual
	t.Cleanup(func() { PathEqual = old })
	m := newFS(t)
	mustNil(t, m.Symlink(filepath.Join("..", "SRC", "VIMRC"), p("home", ".vimrc")))
	e := entry(p("src", "vimrc"), p("home", ".vimrc"))

	PathEqual = func(a, b string) bool { return a == b }
	if r := Evaluate(m, e); r.Status != InvalidLink {
		t.Fatalf("case-sensitive: got %s", r.Status)
	}
	PathEqual = strings.EqualFold
	if r := Evaluate(m, e); r.Status != ValidLink {
		t.Fatalf("case-insensitive: got %s", r.Status)
	}
}

func TestEvaluateRealOS(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("probe-target", filepath.Join(dir, "probe")); err != nil {
		t.Skipf("symlinks not permitted: %v", err)
	}
	m := fs.NewOS()
	realDir := filepath.Join(dir, "real")
	mustNil(t, os.MkdirAll(filepath.Join(realDir, "u"), 0o755))
	mustNil(t, os.Symlink("real", filepath.Join(dir, "home"))) // like /home -> /usr/home
	src := filepath.Join(dir, "src")
	mustNil(t, os.MkdirAll(src, 0o755))
	mustNil(t, os.WriteFile(filepath.Join(src, "f"), []byte("x"), 0o644))

	// Ancestors above the nearest existing directory are not inspected.
	e := entry(filepath.Join(src, "f"), filepath.Join(dir, "home", "u", "new", ".f"))
	if r := Evaluate(m, e); r.Err != nil || r.Status != NotExist {
		t.Fatalf("got %v %v", r.Status, r.Err)
	}
	// A symlink that is the nearest existing ancestor is unsafe.
	mustNil(t, os.Symlink("src", filepath.Join(dir, "lnk")))
	e = entry(filepath.Join(src, "f"), filepath.Join(dir, "lnk", "x", ".f"))
	if r := Evaluate(m, e); !errors.Is(r.Err, ErrUnsafeParent) {
		t.Fatalf("want unsafe parent, got %v", r.Err)
	}

	// Cyclic source link.
	mustNil(t, os.Symlink("cb", filepath.Join(src, "ca")))
	mustNil(t, os.Symlink("ca", filepath.Join(src, "cb")))
	e = entry(filepath.Join(src, "ca"), filepath.Join(dir, "home", "u", ".c"))
	if r := Evaluate(m, e); r.Err == nil {
		t.Fatal("cyclic source must error")
	}

	// Existing absolute and relative links to the source are ValidLink.
	abs := filepath.Join(dir, "abs")
	mustNil(t, os.Symlink(filepath.Join(src, "f"), abs))
	if r := Evaluate(m, entry(filepath.Join(src, "f"), abs)); r.Err != nil || r.Status != ValidLink {
		t.Fatalf("absolute: %s %v", r.Status, r.Err)
	}
	relTarget := filepath.Join(dir, "rel")
	mustNil(t, os.Symlink(filepath.Join("src", "f"), relTarget))
	if r := Evaluate(m, entry(filepath.Join(src, "f"), relTarget)); r.Err != nil || r.Status != ValidLink {
		t.Fatalf("relative: %s %v", r.Status, r.Err)
	}
	// Broken target link is InvalidLink, not NotExist.
	broken := filepath.Join(dir, "broken")
	mustNil(t, os.Symlink("missing", broken))
	if r := Evaluate(m, entry(filepath.Join(src, "f"), broken)); r.Err != nil || r.Status != InvalidLink {
		t.Fatalf("broken: %s %v", r.Status, r.Err)
	}
}
