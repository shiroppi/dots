package backup

import (
	"archive/tar"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shiroppi/dots/internal/fs"
)

func TestPlanRestore(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(t *testing.T, m fs.Manager, target string)
		exists   bool
		existing string
	}{
		{"missing", func(t *testing.T, m fs.Manager, target string) { mustNil(t, m.RemoveAll(target)) }, false, ""},
		{"file", func(t *testing.T, m fs.Manager, target string) {}, true, KindFile},
		{"dir", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, m.RemoveAll(target))
			mustNil(t, m.MkdirAll(filepath.Join(target, "x"), 0o755))
		}, true, KindDir},
		{"symlink", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, m.RemoveAll(target))
			mustNil(t, m.Symlink("elsewhere", target))
		}, true, KindSymlink},
		{"symlink to existing dir is a symlink", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, m.RemoveAll(target))
			mustNil(t, m.MkdirAll(p("home", "d"), 0o755))
			mustNil(t, m.Symlink("d", target))
		}, true, KindSymlink},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newMem(t)
			target := p("home", "t")
			mustNil(t, fs.WriteFile(m, target, []byte("orig"), 0o644))
			s := newStore(m)
			path, err := s.Archive(target)
			mustNil(t, err)
			tc.mutate(t, m, target)
			plan, err := s.PlanRestore(path)
			mustNil(t, err)
			if plan.Exists != tc.exists || plan.ExistingKind != tc.existing {
				t.Fatalf("plan exists=%v kind=%q", plan.Exists, plan.ExistingKind)
			}
			if plan.Target != target || plan.ArchivePath != path || plan.Manifest.Kind != KindFile || plan.Manifest.Target != target {
				t.Fatalf("plan %+v", plan)
			}
		})
	}
}

func TestPlanRestoreOtherKind(t *testing.T) {
	m := newMem(t)
	target := p("home", "t")
	mustNil(t, fs.WriteFile(m, target, []byte("orig"), 0o644))
	path, err := newStore(m).Archive(target)
	mustNil(t, err)
	h := &hookFS{Manager: m}
	h.lstat = func(name string, fi fs.FileInfo) fs.FileInfo {
		if name == target {
			return modeInfo{fi, os.ModeNamedPipe}
		}
		return fi
	}
	plan, err := newStore(h).PlanRestore(path)
	mustNil(t, err)
	if !plan.Exists || plan.ExistingKind != KindOther {
		t.Fatalf("plan %+v", plan)
	}
}

func TestPlanRestoreDoesNotModify(t *testing.T) {
	m := newMem(t)
	target := p("home", "t")
	mkTree(t, m, target)
	s := newStore(m)
	path, err := s.Archive(target)
	mustNil(t, err)
	mustNil(t, m.RemoveAll(target))
	_, err = s.PlanRestore(path)
	mustNil(t, err)
	if exists(t, m, target) {
		t.Fatal("PlanRestore created the target")
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, m fs.Manager, target string)
	}{
		{"file", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, fs.WriteFile(m, target, []byte("content\nline2"), 0o640))
			mustNil(t, m.Chmod(target, 0o640))
			mustNil(t, m.Chtimes(target, ts(20), ts(20)))
		}},
		{"empty file", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, fs.WriteFile(m, target, nil, 0o600))
			mustNil(t, m.Chmod(target, 0o600))
			mustNil(t, m.Chtimes(target, ts(21), ts(21)))
		}},
		{"tree", mkTree},
		{"empty dir", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, m.MkdirAll(target, 0o755))
			mustNil(t, m.Chmod(target, 0o711))
			mustNil(t, m.Chtimes(target, ts(22), ts(22)))
		}},
		{"read-only dir with children", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, m.MkdirAll(filepath.Join(target, "in"), 0o755))
			mustNil(t, fs.WriteFile(m, filepath.Join(target, "in", "f"), []byte("x"), 0o444))
			mustNil(t, m.Chmod(filepath.Join(target, "in", "f"), 0o444))
			mustNil(t, m.Chmod(filepath.Join(target, "in"), 0o500))
			mustNil(t, m.Chmod(target, 0o500))
		}},
		{"symlink relative", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, m.Symlink(filepath.Join("..", "dotfiles", "x"), target))
		}},
		{"symlink broken", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, m.Symlink("missing", target))
		}},
		{"symlink to dir", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, m.MkdirAll(p("home", "d"), 0o755))
			mustNil(t, m.Symlink("d", target))
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newMem(t)
			target := p("home", "t")
			tc.setup(t, m, target)
			want := snapshot(t, m, target)
			s := newStore(m)
			path, err := s.Archive(target)
			mustNil(t, err)
			mustNil(t, m.RemoveAll(target))

			plan, err := s.PlanRestore(path)
			mustNil(t, err)
			if plan.Exists {
				t.Fatal("target should be gone")
			}
			newArchive, err := s.Restore(plan, false)
			mustNil(t, err)
			if newArchive != "" {
				t.Errorf("nothing existed, archive path should be empty: %q", newArchive)
			}
			equalSnap(t, snapshot(t, m, target), want)
			for _, n := range names(t, m, p("home")) {
				if strings.Contains(n, ".dots-restore-") || strings.Contains(n, ".dots-aside-") {
					t.Errorf("leftover %s", n)
				}
			}
			// restoring never touches the archive
			if _, err := ReadArchive(m, path); err != nil {
				t.Errorf("archive damaged: %v", err)
			}
		})
	}
}

func TestRestoreCreatesParent(t *testing.T) {
	m := newMem(t)
	target := p("home", "t")
	mustNil(t, fs.WriteFile(m, target, []byte("x"), 0o644))
	s := newStore(m)
	path, err := s.Archive(target)
	mustNil(t, err)
	mustNil(t, m.RemoveAll(p("home")))
	plan, err := s.PlanRestore(path)
	mustNil(t, err)
	_, err = s.Restore(plan, false)
	mustNil(t, err)
	data, err := fs.ReadFile(m, target)
	mustNil(t, err)
	if string(data) != "x" {
		t.Fatalf("data %q", data)
	}
}

func TestRestoreConflict(t *testing.T) {
	m := newMem(t)
	target := p("home", "t")
	mkTree(t, m, target)
	s := newStore(m)
	path, err := s.Archive(target)
	mustNil(t, err)
	// change the target afterwards and plan
	mustNil(t, fs.WriteFile(m, filepath.Join(target, "new"), []byte("n"), 0o644))
	before := snapshot(t, m, target)
	backupsBefore := names(t, m, s.Dir)
	plan, err := s.PlanRestore(path)
	mustNil(t, err)
	if !plan.Exists || plan.ExistingKind != KindDir {
		t.Fatalf("plan %+v", plan)
	}
	out, err := s.Restore(plan, false)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v", err)
	}
	if out != "" {
		t.Errorf("archive path %q", out)
	}
	equalSnap(t, snapshot(t, m, target), before)
	if got := names(t, m, s.Dir); strings.Join(got, ",") != strings.Join(backupsBefore, ",") {
		t.Errorf("backup dir changed: %v -> %v", backupsBefore, got)
	}
	if got := names(t, m, p("home")); len(got) != 1 {
		t.Errorf("home entries %v", got)
	}
}

func TestRestoreBackupExisting(t *testing.T) {
	tests := []struct {
		name     string
		existing func(t *testing.T, m fs.Manager, target string)
	}{
		{"file", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, fs.WriteFile(m, target, []byte("current"), 0o644))
		}},
		{"dir", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, m.MkdirAll(filepath.Join(target, "deep"), 0o755))
			mustNil(t, fs.WriteFile(m, filepath.Join(target, "deep", "f"), []byte("current"), 0o644))
		}},
		{"symlink", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, m.Symlink("current-link", target))
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newMem(t)
			target := p("home", "t")
			mkTree(t, m, target)
			want := snapshot(t, m, target)
			s := newStore(m)
			orig, err := s.Archive(target)
			mustNil(t, err)

			mustNil(t, m.RemoveAll(target))
			tc.existing(t, m, target)
			current := snapshot(t, m, target)
			plan, err := s.PlanRestore(orig)
			mustNil(t, err)
			if !plan.Exists {
				t.Fatal("expected existing target")
			}
			newArchive, err := s.Restore(plan, true)
			mustNil(t, err)
			if newArchive == "" || newArchive == orig {
				t.Fatalf("new archive path %q (orig %q)", newArchive, orig)
			}
			if !exists(t, m, newArchive) {
				t.Fatal("new archive missing")
			}
			equalSnap(t, snapshot(t, m, target), want)
			// the previous content is saved in the new archive
			plan2, err := s.PlanRestore(newArchive)
			mustNil(t, err)
			_, err = s.Restore(plan2, true)
			mustNil(t, err)
			equalSnap(t, snapshot(t, m, target), current)
			for _, n := range names(t, m, p("home")) {
				if strings.Contains(n, ".dots-restore-") || strings.Contains(n, ".dots-aside-") {
					t.Errorf("leftover %s", n)
				}
			}
		})
	}
}

func TestRestoreStatePlanStale(t *testing.T) {
	setup := func(t *testing.T) (fs.Manager, *Store, string, string) {
		m := newMem(t)
		target := p("home", "t")
		mustNil(t, fs.WriteFile(m, target, []byte("orig"), 0o644))
		s := newStore(m)
		path, err := s.Archive(target)
		mustNil(t, err)
		return m, s, target, path
	}
	t.Run("target appeared", func(t *testing.T) {
		m, s, target, path := setup(t)
		mustNil(t, m.RemoveAll(target))
		plan, err := s.PlanRestore(path)
		mustNil(t, err)
		mustNil(t, fs.WriteFile(m, target, []byte("racer"), 0o644))
		_, err = s.Restore(plan, true)
		if !errors.Is(err, ErrStateChanged) {
			t.Fatalf("err = %v", err)
		}
		data, _ := fs.ReadFile(m, target)
		if string(data) != "racer" {
			t.Fatalf("target modified: %q", data)
		}
	})
	t.Run("target vanished", func(t *testing.T) {
		m, s, target, path := setup(t)
		plan, err := s.PlanRestore(path)
		mustNil(t, err)
		mustNil(t, m.RemoveAll(target))
		_, err = s.Restore(plan, true)
		if !errors.Is(err, ErrStateChanged) {
			t.Fatalf("err = %v", err)
		}
		if exists(t, m, target) {
			t.Fatal("target created")
		}
	})
	t.Run("target kind changed", func(t *testing.T) {
		m, s, target, path := setup(t)
		plan, err := s.PlanRestore(path)
		mustNil(t, err)
		mustNil(t, m.RemoveAll(target))
		mustNil(t, m.MkdirAll(target, 0o755))
		_, err = s.Restore(plan, true)
		if !errors.Is(err, ErrStateChanged) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("archive replaced after plan", func(t *testing.T) {
		m, s, target, path := setup(t)
		mustNil(t, m.RemoveAll(target))
		plan, err := s.PlanRestore(path)
		mustNil(t, err)
		// a different (but valid) archive appears under the same name
		other := rawArchive(t, []rawEntry{
			{name: "manifest.json", typ: tar.TypeReg, body: manifestJSONFor(KindFile, target)[:0] + strings.Replace(manifestJSONFor(KindFile, target), fixedCreated, "2030-01-01T00:00:00Z", 1)},
			{name: "payload", typ: tar.TypeReg, body: "evil"},
		})
		mustNil(t, m.Remove(path))
		mustNil(t, fs.WriteFile(m, path, other, 0o600))
		_, err = s.Restore(plan, false)
		if !errors.Is(err, ErrStateChanged) {
			t.Fatalf("err = %v", err)
		}
		if exists(t, m, target) {
			t.Fatal("target created from replaced archive")
		}
		for _, n := range names(t, m, p("home")) {
			t.Errorf("leftover %s", n)
		}
	})
	t.Run("nil plan", func(t *testing.T) {
		_, s, _, _ := setup(t)
		if _, err := s.Restore(nil, false); err == nil {
			t.Fatal("nil plan accepted")
		}
	})
	t.Run("plan target differs from manifest", func(t *testing.T) {
		m, s, target, path := setup(t)
		mustNil(t, m.RemoveAll(target))
		plan, err := s.PlanRestore(path)
		mustNil(t, err)
		plan.Target = p("home", "other")
		_, err = s.Restore(plan, true)
		if !errors.Is(err, ErrStateChanged) {
			t.Fatalf("err = %v", err)
		}
		if exists(t, m, plan.Target) || exists(t, m, target) {
			t.Fatal("something was created")
		}
	})
}

func TestRestoreRejectsInvalidArchiveWithoutTrace(t *testing.T) {
	target := p("home", "t")
	file := func(name, body string) rawEntry { return rawEntry{name: name, typ: tar.TypeReg, body: body} }
	dirE := func(name string) rawEntry { return rawEntry{name: name, typ: tar.TypeDir, mode: 0o755} }
	man := rawEntry{name: "manifest.json", typ: tar.TypeReg, body: manifestJSONFor(KindDir, target)}

	// Archives that pass PlanRestore's own checks are impossible (it
	// validates everything), so craft a valid plan first and then swap the
	// archive for one with an identical manifest but a bad payload; the
	// extraction pass must reject it and leave nothing behind.
	tests := []struct {
		name    string
		entries []rawEntry
	}{
		{"duplicate after written files", []rawEntry{man, dirE("payload/"), file("payload/a", "1"), file("payload/a", "2")}},
		{"dotdot after written files", []rawEntry{man, dirE("payload/"), file("payload/a", "1"), file("payload/../escape", "x")}},
		{"device after written files", []rawEntry{man, dirE("payload/"), file("payload/a", "1"), {name: "payload/dev", typ: tar.TypeChar}}},
		{"hardlink after written files", []rawEntry{man, dirE("payload/"), file("payload/a", "1"), {name: "payload/hl", typ: tar.TypeLink, link: "payload/a"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newMem(t)
			s := newStore(m)
			good := rawArchive(t, []rawEntry{man, dirE("payload/"), file("payload/a", "1")})
			path := putArchive(t, m, "x.tar.gz", good)
			plan, err := s.PlanRestore(path)
			mustNil(t, err)
			mustNil(t, m.Remove(path))
			mustNil(t, fs.WriteFile(m, path, rawArchive(t, tc.entries), 0o600))

			_, err = s.Restore(plan, true)
			if !errors.Is(err, ErrInvalidArchive) {
				t.Fatalf("err = %v", err)
			}
			if got := names(t, m, p("home")); len(got) != 0 {
				t.Fatalf("leftovers in home: %v", got)
			}
			if exists(t, m, filepath.Join(p("home"), "escape")) || exists(t, m, p("escape")) {
				t.Fatal("escaped file created")
			}
		})
	}
}

func TestRestoreExistingTargetKeptWhenExtractionFails(t *testing.T) {
	m := newMem(t)
	target := p("home", "t")
	mustNil(t, fs.WriteFile(m, target, []byte("orig"), 0o644))
	s := newStore(m)
	path, err := s.Archive(target)
	mustNil(t, err)
	mustNil(t, fs.WriteFile(m, target, []byte("current"), 0o644))
	plan, err := s.PlanRestore(path)
	mustNil(t, err)
	// fail extraction by making tmp file creation fail
	h := &hookFS{Manager: m}
	h.openFile = func(name string) error {
		if strings.Contains(name, ".dots-restore-") {
			return errors.New("disk full")
		}
		return nil
	}
	s2 := &Store{FS: h, Dir: s.Dir, Now: fixedNow}
	out, err := s2.Restore(plan, true)
	if err == nil || out != "" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	data, _ := fs.ReadFile(m, target)
	if string(data) != "current" {
		t.Fatalf("target changed: %q", data)
	}
	if got := names(t, m, p("home")); len(got) != 1 {
		t.Fatalf("leftovers: %v", got)
	}
	if got := names(t, m, s.Dir); len(got) != 1 {
		t.Fatalf("a backup was made although extraction failed: %v", got)
	}
}

func TestRestorePlaceFailureRestoresOriginal(t *testing.T) {
	m := newMem(t)
	target := p("home", "t")
	mustNil(t, fs.WriteFile(m, target, []byte("orig"), 0o644))
	s := newStore(m)
	path, err := s.Archive(target)
	mustNil(t, err)
	mustNil(t, fs.WriteFile(m, target, []byte("current"), 0o644))
	plan, err := s.PlanRestore(path)
	mustNil(t, err)

	errPlace := errors.New("cannot place")
	h := &hookFS{Manager: m}
	h.rename = func(o, n string) error {
		if strings.Contains(o, ".dots-restore-") && n == target {
			return errPlace
		}
		return nil
	}
	s2 := &Store{FS: h, Dir: s.Dir, Now: fixedNow}
	_, err = s2.Restore(plan, true)
	if !errors.Is(err, errPlace) {
		t.Fatalf("err = %v", err)
	}
	data, _ := fs.ReadFile(m, target)
	if string(data) != "current" {
		t.Fatalf("target = %q", data)
	}
	for _, n := range names(t, m, p("home")) {
		if n != "t" {
			t.Errorf("leftover %s", n)
		}
	}
}

func TestRestoreCleanupWarning(t *testing.T) {
	m := newMem(t)
	target := p("home", "t")
	mustNil(t, fs.WriteFile(m, target, []byte("orig"), 0o644))
	s := newStore(m)
	path, err := s.Archive(target)
	mustNil(t, err)
	mustNil(t, fs.WriteFile(m, target, []byte("current"), 0o644))
	plan, err := s.PlanRestore(path)
	mustNil(t, err)
	h := &hookFS{Manager: m}
	h.removeAll = func(path string) error {
		if strings.Contains(path, ".dots-aside-") {
			return errors.New("locked")
		}
		return nil
	}
	s2 := &Store{FS: h, Dir: s.Dir, Now: fixedNow}
	out, err := s2.Restore(plan, true)
	var cw *CleanupWarning
	if !errors.As(err, &cw) {
		t.Fatalf("err = %v", err)
	}
	if out == "" {
		t.Error("archive path missing")
	}
	data, _ := fs.ReadFile(m, target)
	if string(data) != "orig" {
		t.Fatalf("target = %q", data)
	}
}

func TestRestoreRealOS(t *testing.T) {
	m := fs.NewOS()
	tmp := t.TempDir()
	target := filepath.Join(tmp, "tree")
	mustNil(t, os.MkdirAll(filepath.Join(target, "sub"), 0o755))
	mustNil(t, os.WriteFile(filepath.Join(target, "a.txt"), []byte("hello"), 0o640))
	mustNil(t, os.WriteFile(filepath.Join(target, "sub", "b"), []byte("bee"), 0o600))
	abs := filepath.Join(tmp, "elsewhere")
	if err := os.Symlink(abs, filepath.Join(target, "abs-link")); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
	mustNil(t, os.Symlink("a.txt", filepath.Join(target, "rel-link")))
	mt := ts(40)
	mustNil(t, os.Chtimes(filepath.Join(target, "a.txt"), mt, mt))
	mustNil(t, os.Chtimes(filepath.Join(target, "sub"), mt, mt))

	s := &Store{FS: m, Dir: filepath.Join(tmp, "backups"), Now: fixedNow}
	path, err := s.Archive(target)
	mustNil(t, err)
	mustNil(t, os.RemoveAll(target))

	plan, err := s.PlanRestore(path)
	mustNil(t, err)
	_, err = s.Restore(plan, false)
	mustNil(t, err)

	if l, err := os.Readlink(filepath.Join(target, "abs-link")); err != nil || l != abs {
		t.Errorf("abs-link = %q, %v", l, err)
	}
	if l, err := os.Readlink(filepath.Join(target, "rel-link")); err != nil || l != "a.txt" {
		t.Errorf("rel-link = %q, %v", l, err)
	}
	data, _ := os.ReadFile(filepath.Join(target, "sub", "b"))
	if string(data) != "bee" {
		t.Errorf("sub/b = %q", data)
	}
	fi, err := os.Stat(filepath.Join(target, "a.txt"))
	mustNil(t, err)
	if !fi.ModTime().Equal(mt) {
		t.Errorf("mtime %v want %v", fi.ModTime(), mt)
	}
	di, err := os.Stat(filepath.Join(target, "sub"))
	mustNil(t, err)
	if !di.ModTime().Equal(mt) {
		t.Errorf("dir mtime %v want %v", di.ModTime(), mt)
	}
	if runtime.GOOS != "windows" {
		if fi.Mode().Perm() != 0o640 {
			t.Errorf("mode %v", fi.Mode())
		}
		if bi, _ := os.Stat(filepath.Join(target, "sub", "b")); bi.Mode().Perm() != 0o600 {
			t.Errorf("mode %v", bi.Mode())
		}
	}
}
