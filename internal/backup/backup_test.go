package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/platform"
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

// fixedNow is 2024-05-06 07:08:09 JST == 2024-05-05 22:08:09 UTC.
func fixedNow() time.Time {
	return time.Date(2024, 5, 6, 7, 8, 9, 0, time.FixedZone("JST", 9*3600))
}

const (
	fixedStamp   = "20240505T220809Z"
	fixedCreated = "2024-05-05T22:08:09Z"
)

func newStore(m fs.Manager) *Store {
	return &Store{FS: m, Dir: p("backups"), Now: fixedNow}
}

func newMem(t *testing.T) fs.Manager {
	t.Helper()
	m := fs.NewMem()
	mustNil(t, m.MkdirAll(p("home"), 0o755))
	return m
}

func ts(sec int) time.Time { return time.Date(2023, 1, 2, 3, 4, sec, 0, time.UTC) }

// mkTree builds a directory tree at base with fixed modes and mtimes.
func mkTree(t *testing.T, m fs.Manager, base string) {
	t.Helper()
	mustNil(t, m.MkdirAll(filepath.Join(base, "sub", "empty"), 0o755))
	mustNil(t, fs.WriteFile(m, filepath.Join(base, "a.txt"), []byte("hello"), 0o640))
	mustNil(t, fs.WriteFile(m, filepath.Join(base, "sub", "b.bin"), []byte("\x00\x01\x02"), 0o600))
	mustNil(t, m.Symlink("a.txt", filepath.Join(base, "link")))
	mustNil(t, m.Symlink("nowhere", filepath.Join(base, "broken")))
	set := func(rel string, perm fs.FileMode, sec int) {
		name := filepath.Join(base, rel)
		mustNil(t, m.Chmod(name, perm))
		mustNil(t, m.Chtimes(name, ts(sec), ts(sec)))
	}
	set("a.txt", 0o640, 10)
	set(filepath.Join("sub", "b.bin"), 0o600, 11)
	set(filepath.Join("sub", "empty"), 0o755, 12)
	set("sub", 0o700, 13)
	set("", 0o750, 14)
}

// snapshot describes a tree (symlink mode/mtime are ignored).
func snapshot(t *testing.T, m fs.Manager, base string) map[string]string {
	t.Helper()
	out := map[string]string{}
	var walk func(abs, rel string)
	walk = func(abs, rel string) {
		fi, err := m.Lstat(abs)
		mustNil(t, err)
		mode := fi.Mode()
		switch {
		case mode&os.ModeSymlink != 0:
			l, err := m.Readlink(abs)
			mustNil(t, err)
			out[rel] = "l:" + l
		case mode.IsDir():
			out[rel] = "d:" + mode.Perm().String() + ":" + fi.ModTime().UTC().Round(time.Second).String()
			kids, err := m.ReadDir(abs)
			mustNil(t, err)
			for _, k := range kids {
				walk(filepath.Join(abs, k.Name()), rel+"/"+k.Name())
			}
		default:
			data, err := fs.ReadFile(m, abs)
			mustNil(t, err)
			out[rel] = "f:" + mode.Perm().String() + ":" + fi.ModTime().UTC().Round(time.Second).String() + ":" + string(data)
		}
	}
	walk(base, "")
	return out
}

func equalSnap(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("snapshot size: got %d want %d\ngot=%v\nwant=%v", len(got), len(want), got, want)
	}
	for k, w := range want {
		if g, ok := got[k]; !ok || g != w {
			t.Errorf("entry %q: got %q want %q", k, g, w)
		}
	}
}

func exists(t *testing.T, m fs.Manager, name string) bool {
	t.Helper()
	ok, err := fs.Exists(m, name)
	mustNil(t, err)
	return ok
}

func names(t *testing.T, m fs.Manager, dir string) []string {
	t.Helper()
	kids, err := m.ReadDir(dir)
	mustNil(t, err)
	var out []string
	for _, k := range kids {
		out = append(out, k.Name())
	}
	return out
}

// ---- raw tar helpers -------------------------------------------------------

type rawEntry struct {
	name string
	typ  byte
	body string
	link string
	mode int64
}

func manifestJSON(kind string) string {
	return manifestJSONFor(kind, p("home", "t"))
}

func manifestJSONFor(kind, target string) string {
	b, _ := json.Marshal(Manifest{Format: FormatName, Version: FormatVersion, Target: target, Created: fixedCreated, Kind: kind})
	return string(b)
}

func manifestEntry(kind string) rawEntry {
	return rawEntry{name: "manifest.json", typ: tar.TypeReg, body: manifestJSON(kind)}
}

func tarBytes(t *testing.T, entries []rawEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		h := &tar.Header{Name: e.name, Typeflag: e.typ, Mode: mode, Linkname: e.link, ModTime: ts(5)}
		if e.typ == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		mustNil(t, tw.WriteHeader(h))
		if e.typ == tar.TypeReg {
			_, err := tw.Write([]byte(e.body))
			mustNil(t, err)
		}
	}
	mustNil(t, tw.Close())
	return buf.Bytes()
}

func gz(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write(data)
	mustNil(t, err)
	mustNil(t, w.Close())
	return buf.Bytes()
}

func rawArchive(t *testing.T, entries []rawEntry) []byte { return gz(t, tarBytes(t, entries)) }

func putArchive(t *testing.T, m fs.Manager, name string, data []byte) string {
	t.Helper()
	path := p("backups", name)
	mustNil(t, m.MkdirAll(p("backups"), 0o700))
	mustNil(t, fs.WriteFile(m, path, data, 0o600))
	return path
}

type tarItem struct {
	hdr  tar.Header
	body string
}

func readTarItems(t *testing.T, m fs.Manager, path string) []tarItem {
	t.Helper()
	data, err := fs.ReadFile(m, path)
	mustNil(t, err)
	zr, err := gzip.NewReader(bytes.NewReader(data))
	mustNil(t, err)
	tr := tar.NewReader(zr)
	var out []tarItem
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		mustNil(t, err)
		b, err := io.ReadAll(tr)
		mustNil(t, err)
		out = append(out, tarItem{*h, string(b)})
	}
	return out
}

// ---- tests -----------------------------------------------------------------

func TestDefaultDir(t *testing.T) {
	abs := p("data")
	rel := filepath.Join("rel", "dir")
	home := p("home")
	tests := []struct {
		name    string
		env     platform.Env
		want    string
		wantErr bool
	}{
		{"windows ok", platform.Env{GOOS: "windows", Home: home, Vars: map[string]string{"LOCALAPPDATA": abs}}, filepath.Join(abs, "dots", "backups"), false},
		{"windows unset", platform.Env{GOOS: "windows", Home: home}, "", true},
		{"windows empty", platform.Env{GOOS: "windows", Home: home, Vars: map[string]string{"LOCALAPPDATA": ""}}, "", true},
		{"windows relative", platform.Env{GOOS: "windows", Home: home, Vars: map[string]string{"LOCALAPPDATA": rel}}, "", true},
		{"darwin", platform.Env{GOOS: "darwin", Home: home, Vars: map[string]string{"XDG_DATA_HOME": abs}}, filepath.Join(home, "Library", "Application Support", "dots", "backups"), false},
		{"darwin no home", platform.Env{GOOS: "darwin"}, "", true},
		{"linux xdg", platform.Env{GOOS: "linux", Home: home, Vars: map[string]string{"XDG_DATA_HOME": abs}}, filepath.Join(abs, "dots", "backups"), false},
		{"linux xdg empty", platform.Env{GOOS: "linux", Home: home, Vars: map[string]string{"XDG_DATA_HOME": ""}}, filepath.Join(home, ".local", "share", "dots", "backups"), false},
		{"linux xdg unset", platform.Env{GOOS: "linux", Home: home}, filepath.Join(home, ".local", "share", "dots", "backups"), false},
		{"linux xdg relative", platform.Env{GOOS: "linux", Home: home, Vars: map[string]string{"XDG_DATA_HOME": rel}}, "", true},
		{"freebsd xdg", platform.Env{GOOS: "freebsd", Home: home, Vars: map[string]string{"XDG_DATA_HOME": abs}}, filepath.Join(abs, "dots", "backups"), false},
		{"freebsd fallback", platform.Env{GOOS: "freebsd", Home: home}, filepath.Join(home, ".local", "share", "dots", "backups"), false},
		{"linux no home no xdg", platform.Env{GOOS: "linux"}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DefaultDir(tc.env)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestSanitizeBase(t *testing.T) {
	tests := map[string]string{
		"plain.txt":              "plain.txt",
		".vimrc":                 "vimrc",
		"...hidden":              "hidden",
		"my file (1).txt":        "my_file__1_.txt",
		"...":                    "target",
		"___":                    "target",
		"日本語":                    "target",
		"a-b_c.D9":               "a-b_c.D9",
		strings.Repeat("x", 100): strings.Repeat("x", 64),
	}
	for in, want := range tests {
		if got := sanitizeBase(filepath.Join(root(), in)); got != want {
			t.Errorf("sanitizeBase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestArchiveRegularFile(t *testing.T) {
	m := newMem(t)
	target := p("home", ".vimrc")
	mustNil(t, fs.WriteFile(m, target, []byte("set nu\n"), 0o640))
	mustNil(t, m.Chmod(target, 0o640))
	mustNil(t, m.Chtimes(target, ts(30), ts(30)))
	s := newStore(m)

	path, err := s.Archive(target)
	mustNil(t, err)
	if want := p("backups", "vimrc-"+fixedStamp+"-0001.tar.gz"); path != want {
		t.Fatalf("path = %q want %q", path, want)
	}
	items := readTarItems(t, m, path)
	if len(items) != 2 {
		t.Fatalf("got %d entries", len(items))
	}
	if items[0].hdr.Name != "manifest.json" {
		t.Fatalf("first entry %q", items[0].hdr.Name)
	}
	var mf Manifest
	mustNil(t, json.Unmarshal([]byte(items[0].body), &mf))
	want := Manifest{Format: "dots-backup", Version: 1, Target: target, Created: fixedCreated, Kind: KindFile}
	if mf != want {
		t.Fatalf("manifest %+v want %+v", mf, want)
	}
	pl := items[1]
	if pl.hdr.Name != "payload" || pl.hdr.Typeflag != tar.TypeReg || pl.body != "set nu\n" {
		t.Fatalf("payload %+v body=%q", pl.hdr, pl.body)
	}
	if pl.hdr.Mode != 0o640 {
		t.Errorf("mode %o", pl.hdr.Mode)
	}
	if !pl.hdr.ModTime.Equal(ts(30)) {
		t.Errorf("mtime %v", pl.hdr.ModTime)
	}

	a, err := ReadArchive(m, path)
	mustNil(t, err)
	if len(a.Entries) != 1 || a.Entries[0].Name != "" || a.Entries[0].Kind != KindFile || a.Entries[0].Size != 7 {
		t.Fatalf("entries %+v", a.Entries)
	}
	// target untouched
	data, _ := fs.ReadFile(m, target)
	if string(data) != "set nu\n" {
		t.Fatal("target modified")
	}
}

func TestArchiveDirectoryTree(t *testing.T) {
	m := newMem(t)
	target := p("home", "cfg")
	mkTree(t, m, target)
	before := snapshot(t, m, target)
	s := newStore(m)

	path, err := s.Archive(target)
	mustNil(t, err)
	a, err := ReadArchive(m, path)
	mustNil(t, err)
	if a.Manifest.Kind != KindDir || a.Manifest.Target != target {
		t.Fatalf("manifest %+v", a.Manifest)
	}
	got := map[string]Entry{}
	var order []string
	for _, e := range a.Entries {
		got[e.Name] = e
		order = append(order, e.Name)
	}
	wantNames := []string{"", "a.txt", "broken", "link", "sub", "sub/b.bin", "sub/empty"}
	if len(order) != len(wantNames) {
		t.Fatalf("entries %v", order)
	}
	sorted := append([]string(nil), order...)
	sort.Strings(sorted)
	for i := range wantNames {
		if sorted[i] != wantNames[i] {
			t.Fatalf("entries %v", order)
		}
	}
	if order[0] != "" {
		t.Fatalf("root not first: %v", order)
	}
	if e := got["a.txt"]; e.Kind != KindFile || e.Size != 5 || e.Mode != 0o640 || !e.ModTime.Equal(ts(10)) {
		t.Errorf("a.txt %+v", e)
	}
	if e := got["sub"]; e.Kind != KindDir || e.Mode != 0o700 {
		t.Errorf("sub %+v", e)
	}
	if e := got["link"]; e.Kind != KindSymlink || e.Linkname != "a.txt" {
		t.Errorf("link %+v", e)
	}
	if e := got["broken"]; e.Kind != KindSymlink || e.Linkname != "nowhere" {
		t.Errorf("broken %+v", e)
	}
	// parents precede children
	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	if pos["sub"] > pos["sub/b.bin"] || pos["sub"] > pos["sub/empty"] {
		t.Errorf("order %v", order)
	}
	// target untouched
	equalSnap(t, snapshot(t, m, target), before)
}

func TestArchiveSymlinkItem(t *testing.T) {
	tests := []struct {
		name string
		link string
		mk   func(t *testing.T, m fs.Manager)
	}{
		{"relative to file", filepath.Join("..", "dotfiles", "vimrc"), func(t *testing.T, m fs.Manager) {
			mustNil(t, m.MkdirAll(p("dotfiles"), 0o755))
		}},
		{"broken", "does-not-exist", func(*testing.T, fs.Manager) {}},
		{"to directory", "..", func(*testing.T, fs.Manager) {}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newMem(t)
			target := p("home", ".link")
			tc.mk(t, m)
			mustNil(t, m.Symlink(tc.link, target))
			path, err := newStore(m).Archive(target)
			mustNil(t, err)
			items := readTarItems(t, m, path)
			if len(items) != 2 || items[1].hdr.Typeflag != tar.TypeSymlink || items[1].hdr.Linkname != tc.link {
				t.Fatalf("items %+v", items)
			}
			var mf Manifest
			mustNil(t, json.Unmarshal([]byte(items[0].body), &mf))
			if mf.Kind != KindSymlink {
				t.Fatalf("kind %q", mf.Kind)
			}
			a, err := ReadArchive(m, path)
			mustNil(t, err)
			if a.Entries[0].Linkname != tc.link {
				t.Fatalf("linkname %q", a.Entries[0].Linkname)
			}
			// the link itself still exists
			got, err := m.Readlink(target)
			mustNil(t, err)
			if got != tc.link {
				t.Fatalf("link changed to %q", got)
			}
		})
	}
}

func TestArchiveSymlinkNotFollowed(t *testing.T) {
	m := newMem(t)
	mustNil(t, m.MkdirAll(p("real"), 0o755))
	mustNil(t, fs.WriteFile(m, p("real", "f"), []byte("secret"), 0o644))
	target := p("home", "dirlink")
	mustNil(t, m.Symlink(filepath.Join("..", "real"), target))
	path, err := newStore(m).Archive(target)
	mustNil(t, err)
	a, err := ReadArchive(m, path)
	mustNil(t, err)
	if len(a.Entries) != 1 || a.Entries[0].Kind != KindSymlink {
		t.Fatalf("entries %+v", a.Entries)
	}
}

// hookFS wraps a Manager and lets tests inject failures.
type hookFS struct {
	fs.Manager
	lstat     func(name string, fi fs.FileInfo) fs.FileInfo
	readDir   func(name string, in []fs.FileInfo) []fs.FileInfo
	openFile  func(name string) error
	rename    func(oldpath, newpath string) error
	removeAll func(path string) error
}

type modeInfo struct {
	fs.FileInfo
	mode fs.FileMode
}

func (m modeInfo) Mode() fs.FileMode { return m.mode }

func (h *hookFS) Lstat(name string) (fs.FileInfo, error) {
	fi, err := h.Manager.Lstat(name)
	if err == nil && h.lstat != nil {
		fi = h.lstat(name, fi)
	}
	return fi, err
}

func (h *hookFS) ReadDir(name string) ([]fs.FileInfo, error) {
	in, err := h.Manager.ReadDir(name)
	if err == nil && h.readDir != nil {
		in = h.readDir(name, in)
	}
	return in, err
}

func (h *hookFS) OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error) {
	if h.openFile != nil {
		if err := h.openFile(name); err != nil {
			return nil, err
		}
	}
	return h.Manager.OpenFile(name, flag, perm)
}

func (h *hookFS) Open(name string) (fs.File, error) {
	if h.openFile != nil {
		if err := h.openFile(name); err != nil {
			return nil, err
		}
	}
	return h.Manager.Open(name)
}

func (h *hookFS) Rename(o, n string) error {
	if h.rename != nil {
		if err := h.rename(o, n); err != nil {
			return err
		}
	}
	return h.Manager.Rename(o, n)
}

func (h *hookFS) RemoveAll(path string) error {
	if h.removeAll != nil {
		if err := h.removeAll(path); err != nil {
			return err
		}
	}
	return h.Manager.RemoveAll(path)
}

func TestArchiveUnsupportedType(t *testing.T) {
	tests := []struct {
		name string
		mode fs.FileMode
		nest bool
	}{
		{"fifo", os.ModeNamedPipe, false},
		{"socket", os.ModeSocket, false},
		{"device", os.ModeDevice, false},
		{"nested fifo", os.ModeNamedPipe, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := newMem(t)
			target := p("home", "thing")
			bad := target
			if tc.nest {
				mustNil(t, base.MkdirAll(target, 0o755))
				bad = filepath.Join(target, "bad")
			}
			mustNil(t, fs.WriteFile(base, bad, []byte("x"), 0o644))
			h := &hookFS{Manager: base}
			h.lstat = func(name string, fi fs.FileInfo) fs.FileInfo {
				if name == bad {
					return modeInfo{fi, tc.mode}
				}
				return fi
			}
			h.readDir = func(name string, in []fs.FileInfo) []fs.FileInfo {
				out := make([]fs.FileInfo, len(in))
				for i, fi := range in {
					out[i] = fi
					if filepath.Join(name, fi.Name()) == bad {
						out[i] = modeInfo{fi, tc.mode}
					}
				}
				return out
			}
			s := newStore(h)
			path, err := s.Archive(target)
			if !errors.Is(err, ErrUnsupportedType) {
				t.Fatalf("err = %v", err)
			}
			if path != "" {
				t.Fatalf("path %q", path)
			}
			// no archive and no temp file left behind
			if exists(t, base, s.Dir) {
				if got := names(t, base, s.Dir); len(got) != 0 {
					t.Fatalf("leftovers %v", got)
				}
			}
			if !exists(t, base, bad) {
				t.Fatal("target removed")
			}
		})
	}
}

func TestArchiveErrors(t *testing.T) {
	m := newMem(t)
	s := newStore(m)
	if _, err := s.Archive(filepath.Join("rel", "x")); err == nil {
		t.Error("relative target accepted")
	}
	_, err := s.Archive(p("home", "missing"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing: %v", err)
	}
	// backup dir inside the target directory
	target := p("home", "tree")
	mustNil(t, m.MkdirAll(target, 0o755))
	s2 := &Store{FS: m, Dir: filepath.Join(target, "backups"), Now: fixedNow}
	if _, err := s2.Archive(target); err == nil {
		t.Error("backup dir inside target accepted")
	}
	if exists(t, m, filepath.Join(target, "backups")) {
		t.Error("backup dir created inside target")
	}
	// Dir equal to the target is also inside it
	s3 := &Store{FS: m, Dir: target, Now: fixedNow}
	if _, err := s3.Archive(target); err == nil {
		t.Error("backup dir == target accepted")
	}
	// a sibling with a common prefix is fine
	s4 := &Store{FS: m, Dir: target + "-backups", Now: fixedNow}
	if _, err := s4.Archive(target); err != nil {
		t.Errorf("sibling dir: %v", err)
	}
}

func TestArchiveNamingAndSequence(t *testing.T) {
	m := newMem(t)
	target := p("home", "f")
	mustNil(t, fs.WriteFile(m, target, []byte("1"), 0o644))
	s := newStore(m)
	seen := map[string]bool{}
	for i := 1; i <= 3; i++ {
		path, err := s.Archive(target)
		mustNil(t, err)
		want := p("backups", "f-"+fixedStamp+"-000"+string(rune('0'+i))+".tar.gz")
		if path != want {
			t.Fatalf("path %q want %q", path, want)
		}
		if seen[path] {
			t.Fatal("duplicate path")
		}
		seen[path] = true
	}
	// a different time gives a different stamp and restarts the sequence
	s.Now = func() time.Time { return fixedNow().Add(time.Second) }
	path, err := s.Archive(target)
	mustNil(t, err)
	if want := p("backups", "f-20240505T220810Z-0001.tar.gz"); path != want {
		t.Fatalf("path %q want %q", path, want)
	}
	// existing archives are never overwritten: contents stay intact
	mustNil(t, fs.WriteFile(m, target, []byte("2"), 0o644))
	first := p("backups", "f-"+fixedStamp+"-0001.tar.gz")
	before, _ := fs.ReadFile(m, first)
	_, err = s.Archive(target)
	mustNil(t, err)
	after, _ := fs.ReadFile(m, first)
	if !bytes.Equal(before, after) {
		t.Fatal("existing archive modified")
	}
	// no temp files remain
	for _, n := range names(t, m, p("backups")) {
		if strings.HasPrefix(n, ".tmp-") || strings.HasSuffix(n, ".partial") {
			t.Errorf("temp file left: %s", n)
		}
	}
}

func TestArchiveDefaultClock(t *testing.T) {
	m := newMem(t)
	target := p("home", "f")
	mustNil(t, fs.WriteFile(m, target, []byte("1"), 0o644))
	s := &Store{FS: m, Dir: p("backups")}
	path, err := s.Archive(target)
	mustNil(t, err)
	if !strings.HasSuffix(path, ".tar.gz") || !strings.Contains(filepath.Base(path), "f-") {
		t.Fatalf("path %q", path)
	}
}

func TestArchiveContentSafety(t *testing.T) {
	m := newMem(t)
	target := p("home", "cfg")
	mkTree(t, m, target)
	path, err := newStore(m).Archive(target)
	mustNil(t, err)
	items := readTarItems(t, m, path)
	if items[0].hdr.Name != "manifest.json" {
		t.Fatalf("first %q", items[0].hdr.Name)
	}
	for _, it := range items[1:] {
		n := it.hdr.Name
		if !(n == "payload" || strings.HasPrefix(n, "payload/")) {
			t.Errorf("entry outside payload: %q", n)
		}
		if strings.HasPrefix(n, "/") || strings.Contains(n, "..") || strings.Contains(n, `\`) || strings.Contains(n, ":") {
			t.Errorf("unsafe name %q", n)
		}
	}
}

func TestArchivePermissionsAndDirCreation(t *testing.T) {
	m := newMem(t)
	target := p("home", "f")
	mustNil(t, fs.WriteFile(m, target, []byte("1"), 0o644))
	s := &Store{FS: m, Dir: p("deep", "er", "backups"), Now: fixedNow}
	path, err := s.Archive(target)
	mustNil(t, err)
	di, err := m.Lstat(s.Dir)
	mustNil(t, err)
	if !di.IsDir() || di.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v", di.Mode())
	}
	fi, err := m.Lstat(path)
	mustNil(t, err)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("archive mode %v", fi.Mode())
	}
}

func TestArchiveRealOSPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are best effort on Windows")
	}
	m := fs.NewOS()
	tmp := t.TempDir()
	target := filepath.Join(tmp, "f")
	mustNil(t, fs.WriteFile(m, target, []byte("1"), 0o644))
	s := &Store{FS: m, Dir: filepath.Join(tmp, "bk"), Now: fixedNow}
	path, err := s.Archive(target)
	mustNil(t, err)
	di, _ := os.Stat(s.Dir)
	fi, _ := os.Stat(path)
	if di.Mode().Perm() != 0o700 || fi.Mode().Perm() != 0o600 {
		t.Errorf("modes dir=%v file=%v", di.Mode(), fi.Mode())
	}
}

func TestArchiveFailureLeavesNoTrace(t *testing.T) {
	tests := []struct {
		name string
		hook func(h *hookFS, target string)
	}{
		{"cannot create temp file", func(h *hookFS, _ string) {
			h.openFile = func(name string) error {
				if strings.HasSuffix(name, ".partial") {
					return errors.New("disk full")
				}
				return nil
			}
		}},
		{"cannot read source file", func(h *hookFS, target string) {
			h.openFile = func(name string) error {
				if name == filepath.Join(target, "a.txt") {
					return errors.New("permission denied")
				}
				return nil
			}
		}},
		{"rename into place fails", func(h *hookFS, _ string) {
			h.rename = func(o, n string) error {
				if strings.HasSuffix(o, ".partial") {
					return errors.New("rename boom")
				}
				return nil
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := newMem(t)
			target := p("home", "cfg")
			mkTree(t, base, target)
			before := snapshot(t, base, target)
			h := &hookFS{Manager: base}
			tc.hook(h, target)
			s := newStore(h)
			path, err := s.Archive(target)
			if err == nil || path != "" {
				t.Fatalf("path=%q err=%v", path, err)
			}
			if exists(t, base, s.Dir) {
				if got := names(t, base, s.Dir); len(got) != 0 {
					t.Fatalf("leftovers %v", got)
				}
			}
			equalSnap(t, snapshot(t, base, target), before)
		})
	}
}

// ---- ReadArchive validation ------------------------------------------------

func TestReadArchiveValid(t *testing.T) {
	m := newMem(t)
	path := putArchive(t, m, "ok.tar.gz", rawArchive(t, []rawEntry{
		manifestEntry(KindDir),
		{name: "payload/", typ: tar.TypeDir, mode: 0o755},
		{name: "payload/f", typ: tar.TypeReg, body: "abc"},
		{name: "payload/d/", typ: tar.TypeDir, mode: 0o700},
		{name: "payload/d/l", typ: tar.TypeSymlink, link: "../f"},
	}))
	a, err := ReadArchive(m, path)
	mustNil(t, err)
	if a.Path != path || a.Manifest.Target != p("home", "t") || a.Manifest.Created != fixedCreated {
		t.Fatalf("archive %+v", a)
	}
	var got []string
	for _, e := range a.Entries {
		got = append(got, e.Kind+":"+e.Name)
	}
	want := []string{"dir:", "file:f", "dir:d", "symlink:d/l"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestReadArchiveMissingFile(t *testing.T) {
	m := newMem(t)
	_, err := ReadArchive(m, p("backups", "nope.tar.gz"))
	if err == nil || errors.Is(err, ErrInvalidArchive) {
		t.Fatalf("want plain not-exist error, got %v", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("want ErrNotExist, got %v", err)
	}
}

func TestReadArchiveInvalid(t *testing.T) {
	file := func(name, body string) rawEntry { return rawEntry{name: name, typ: tar.TypeReg, body: body} }
	dir := func(name string) rawEntry { return rawEntry{name: name, typ: tar.TypeDir, mode: 0o755} }
	man := func(mut func(*Manifest)) rawEntry {
		mf := Manifest{Format: FormatName, Version: FormatVersion, Target: p("home", "t"), Created: fixedCreated, Kind: KindFile}
		mut(&mf)
		b, _ := json.Marshal(mf)
		return file("manifest.json", string(b))
	}
	okMan := manifestEntry(KindFile)
	dirMan := manifestEntry(KindDir)

	tests := []struct {
		name    string
		entries []rawEntry
		skipWin bool
	}{
		{"no entries at all", nil, false},
		{"missing manifest", []rawEntry{file("payload", "x")}, false},
		{"manifest only", []rawEntry{okMan}, false},
		{"manifest not first", []rawEntry{file("payload", "x"), okMan}, false},
		{"manifest is a dir", []rawEntry{{name: "manifest.json", typ: tar.TypeDir}, file("payload", "x")}, false},
		{"duplicate manifest", []rawEntry{okMan, okMan, file("payload", "x")}, false},
		{"manifest not json", []rawEntry{file("manifest.json", "{nope"), file("payload", "x")}, false},
		{"wrong format", []rawEntry{man(func(m *Manifest) { m.Format = "tar" }), file("payload", "x")}, false},
		{"wrong version", []rawEntry{man(func(m *Manifest) { m.Version = 2 }), file("payload", "x")}, false},
		{"version zero", []rawEntry{man(func(m *Manifest) { m.Version = 0 }), file("payload", "x")}, false},
		{"bad kind", []rawEntry{man(func(m *Manifest) { m.Kind = "fifo" }), file("payload", "x")}, false},
		{"relative target", []rawEntry{man(func(m *Manifest) { m.Target = "rel/x" }), file("payload", "x")}, false},
		{"bad created", []rawEntry{man(func(m *Manifest) { m.Created = "yesterday" }), file("payload", "x")}, false},
		{"oversized manifest", []rawEntry{file("manifest.json", strings.Repeat(" ", maxManifestSize+1)), file("payload", "x")}, false},
		{"kind mismatch", []rawEntry{dirMan, file("payload", "x")}, false},
		{"payload name outside", []rawEntry{okMan, file("other", "x")}, false},
		{"payload prefix lookalike", []rawEntry{dirMan, dir("payload/"), file("payload2/x", "x")}, false},
		{"absolute entry", []rawEntry{dirMan, dir("payload/"), file("/etc/passwd", "x")}, false},
		{"absolute payload", []rawEntry{okMan, file("/payload", "x")}, false},
		{"dotdot entry", []rawEntry{dirMan, dir("payload/"), file("payload/../evil", "x")}, false},
		{"dotdot start", []rawEntry{dirMan, dir("payload/"), file("../evil", "x")}, false},
		{"dotdot middle", []rawEntry{dirMan, dir("payload/"), dir("payload/a/"), file("payload/a/../../evil", "x")}, false},
		{"dot segment", []rawEntry{dirMan, dir("payload/"), file("payload/./x", "x")}, false},
		{"empty segment", []rawEntry{dirMan, dir("payload/"), file("payload//x", "x")}, false},
		{"backslash name", []rawEntry{dirMan, dir("payload/"), file(`payload/a\b`, "x")}, false},
		{"colon name on windows", []rawEntry{dirMan, dir("payload/"), file("payload/C:evil", "x")}, true},
		{"duplicate file", []rawEntry{dirMan, dir("payload/"), file("payload/a", "1"), file("payload/a", "2")}, false},
		{"duplicate root", []rawEntry{dirMan, dir("payload/"), dir("payload/")}, false},
		{"duplicate dir and file", []rawEntry{dirMan, dir("payload/"), dir("payload/a/"), file("payload/a", "x")}, false},
		{"missing parent entry", []rawEntry{dirMan, dir("payload/"), file("payload/a/b", "x")}, false},
		{"missing root dir entry", []rawEntry{dirMan, file("payload/a", "x")}, false},
		{"entry under file", []rawEntry{okMan, file("payload", "x"), file("payload/a", "x")}, false},
		{"root not first payload entry", []rawEntry{dirMan, dir("payload/a/"), dir("payload/")}, false},
		{"symlink empty target", []rawEntry{okMan, {name: "payload", typ: tar.TypeSymlink}}, false},
		{"char device", []rawEntry{dirMan, dir("payload/"), {name: "payload/c", typ: tar.TypeChar}}, false},
		{"block device", []rawEntry{dirMan, dir("payload/"), {name: "payload/b", typ: tar.TypeBlock}}, false},
		{"fifo", []rawEntry{dirMan, dir("payload/"), {name: "payload/f", typ: tar.TypeFifo}}, false},
		{"hardlink", []rawEntry{dirMan, dir("payload/"), file("payload/a", "x"), {name: "payload/h", typ: tar.TypeLink, link: "payload/a"}}, false},
		{"hardlink root", []rawEntry{okMan, {name: "payload", typ: tar.TypeLink, link: "/etc/passwd"}}, false},
		{"root is fifo", []rawEntry{okMan, {name: "payload", typ: tar.TypeFifo}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipWin && runtime.GOOS != "windows" {
				t.Skip("windows only rule")
			}
			m := newMem(t)
			path := putArchive(t, m, "bad.tar.gz", rawArchive(t, tc.entries))
			a, err := ReadArchive(m, path)
			if !errors.Is(err, ErrInvalidArchive) {
				t.Fatalf("err = %v (archive %+v)", err, a)
			}
			if _, err := newStore(m).PlanRestore(path); !errors.Is(err, ErrInvalidArchive) {
				t.Fatalf("PlanRestore err = %v", err)
			}
		})
	}
}

func TestReadArchiveCorruptContainers(t *testing.T) {
	valid := []rawEntry{
		manifestEntry(KindFile),
		{name: "payload", typ: tar.TypeReg, body: strings.Repeat("0123456789", 500)},
	}
	rawTar := tarBytes(t, valid)
	goodGz := gz(t, rawTar)

	flip := func(b []byte, idx int) []byte {
		c := append([]byte(nil), b...)
		c[idx] ^= 0xff
		return c
	}
	tests := []struct {
		name string
		data []byte
	}{
		{"empty file", nil},
		{"not gzip", []byte("this is not gzip at all")},
		{"gzip of garbage", gz(t, []byte("garbage that is not a tar archive, 512+ bytes expected "+strings.Repeat("z", 600)))},
		{"truncated gzip", goodGz[:len(goodGz)/2]},
		{"truncated gzip trailer", goodGz[:len(goodGz)-4]},
		{"corrupt gzip crc", flip(goodGz, len(goodGz)-6)},
		{"corrupt gzip body", flip(goodGz, len(goodGz)/2)},
		{"truncated tar header", gz(t, rawTar[:700])},
		{"truncated tar body", gz(t, rawTar[:1024+300])},
		{"truncated tar without end blocks", gz(t, rawTar[:len(rawTar)-1024-200])},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newMem(t)
			path := putArchive(t, m, "bad.tar.gz", tc.data)
			_, err := ReadArchive(m, path)
			if !errors.Is(err, ErrInvalidArchive) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	// sanity: the untouched archive is valid
	m := newMem(t)
	path := putArchive(t, m, "good.tar.gz", goodGz)
	if _, err := ReadArchive(m, path); err != nil {
		t.Fatalf("control archive invalid: %v", err)
	}
}

func TestReadArchiveTrailingGarbageAfterTar(t *testing.T) {
	// Extra bytes after a complete tar stream inside the gzip must not make
	// ReadArchive panic; either outcome must be a clean (nil or invalid) result.
	m := newMem(t)
	data := gz(t, append(tarBytes(t, []rawEntry{manifestEntry(KindFile), {name: "payload", typ: tar.TypeReg, body: "x"}}), []byte("junk")...))
	path := putArchive(t, m, "t.tar.gz", data)
	if _, err := ReadArchive(m, path); err != nil && !errors.Is(err, ErrInvalidArchive) {
		t.Fatalf("unexpected error type: %v", err)
	}
}
