package backup

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shiroppi/dots/internal/fs"
)

func asideNames(t *testing.T, m fs.Manager, dir string) []string {
	t.Helper()
	var out []string
	for _, n := range names(t, m, dir) {
		if strings.Contains(n, ".dots-aside-") {
			out = append(out, n)
		}
	}
	return out
}

func TestReplaceSuccess(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, m fs.Manager, target string)
		place func(t *testing.T, m fs.Manager, target string) error
	}{
		{"file by file", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, fs.WriteFile(m, target, []byte("old"), 0o644))
		}, func(t *testing.T, m fs.Manager, target string) error {
			return fs.WriteFile(m, target, []byte("new"), 0o644)
		}},
		{"dir by symlink", func(t *testing.T, m fs.Manager, target string) {
			mkTree(t, m, target)
		}, func(t *testing.T, m fs.Manager, target string) error {
			return m.Symlink(filepath.Join("..", "src"), target)
		}},
		{"symlink by symlink", func(t *testing.T, m fs.Manager, target string) {
			mustNil(t, m.Symlink("old-target", target))
		}, func(t *testing.T, m fs.Manager, target string) error {
			return m.Symlink("new-target", target)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newMem(t)
			target := p("home", "t")
			tc.setup(t, m, target)
			oldSnap := snapshot(t, m, target)
			s := newStore(m)

			called := 0
			path, err := s.Replace(target, func() error {
				called++
				if exists(t, m, target) {
					t.Error("target still exists inside place")
				}
				if len(asideNames(t, m, p("home"))) != 1 {
					t.Errorf("aside copies during place: %v", names(t, m, p("home")))
				}
				return tc.place(t, m, target)
			})
			mustNil(t, err)
			if called != 1 {
				t.Fatalf("place called %d times", called)
			}
			if want := p("backups", "t-"+fixedStamp+"-0001.tar.gz"); path != want {
				t.Fatalf("path %q want %q", path, want)
			}
			if !exists(t, m, target) {
				t.Fatal("new content missing")
			}
			if got := asideNames(t, m, p("home")); len(got) != 0 {
				t.Fatalf("aside left behind: %v", got)
			}
			// the archive holds the OLD content: restore it elsewhere to compare
			a, err := ReadArchive(m, path)
			mustNil(t, err)
			if a.Manifest.Target != target {
				t.Fatalf("manifest target %q", a.Manifest.Target)
			}
			plan, err := s.PlanRestore(path)
			mustNil(t, err)
			if !plan.Exists {
				t.Fatal("plan should see the new content")
			}
			_, err = s.Restore(plan, true)
			mustNil(t, err)
			equalSnap(t, snapshot(t, m, target), oldSnap)
		})
	}
}

func TestReplacePlaceFails(t *testing.T) {
	errPlace := errors.New("place boom")
	tests := []struct {
		name    string
		partial bool // place leaves partial content at target
		dirOld  bool
	}{
		{"nothing created", false, false},
		{"partial file created", true, false},
		{"directory original", true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newMem(t)
			target := p("home", "t")
			if tc.dirOld {
				mkTree(t, m, target)
			} else {
				mustNil(t, fs.WriteFile(m, target, []byte("old"), 0o600))
			}
			before := snapshot(t, m, target)
			s := newStore(m)
			path, err := s.Replace(target, func() error {
				if tc.partial {
					mustNil(t, fs.WriteFile(m, target, []byte("half"), 0o644))
				}
				return errPlace
			})
			if !errors.Is(err, errPlace) {
				t.Fatalf("err = %v", err)
			}
			var re *ReplaceError
			if errors.As(err, &re) {
				t.Fatalf("recovery succeeded, must not be ReplaceError: %v", err)
			}
			if path == "" || !exists(t, m, path) {
				t.Fatalf("archive path %q should be returned and exist", path)
			}
			equalSnap(t, snapshot(t, m, target), before)
			if got := asideNames(t, m, p("home")); len(got) != 0 {
				t.Fatalf("aside left: %v", got)
			}
		})
	}
}

func TestReplaceRecoveryFails(t *testing.T) {
	errPlace := errors.New("place boom")
	errMove := errors.New("cannot move back")
	m := newMem(t)
	target := p("home", "t")
	mustNil(t, fs.WriteFile(m, target, []byte("old"), 0o644))
	h := &hookFS{Manager: m}
	h.rename = func(o, n string) error {
		if strings.Contains(o, ".dots-aside-") && n == target {
			return errMove
		}
		return nil
	}
	s := newStore(h)
	path, err := s.Replace(target, func() error {
		mustNil(t, fs.WriteFile(m, target, []byte("half"), 0o644))
		return errPlace
	})
	var re *ReplaceError
	if !errors.As(err, &re) {
		t.Fatalf("want *ReplaceError, got %T %v", err, err)
	}
	if !errors.Is(err, errPlace) || !errors.Is(err, errMove) {
		t.Errorf("Unwrap must expose both errors: %v", err)
	}
	if !errors.Is(re.Err, errPlace) || !errors.Is(re.RecoverErr, errMove) {
		t.Errorf("fields: Err=%v RecoverErr=%v", re.Err, re.RecoverErr)
	}
	if re.Target != target || re.ArchivePath != path || path == "" {
		t.Errorf("target=%q archive=%q path=%q", re.Target, re.ArchivePath, path)
	}
	if !strings.Contains(re.AsidePath, ".dots-aside-") || filepath.Dir(re.AsidePath) != p("home") {
		t.Errorf("aside %q", re.AsidePath)
	}
	data, err := fs.ReadFile(m, re.AsidePath)
	mustNil(t, err)
	if string(data) != "old" {
		t.Errorf("original not preserved at aside path: %q", data)
	}
	msg := re.Error()
	for _, want := range []string{target, re.AsidePath, re.ArchivePath, "place boom", "cannot move back"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %q", msg, want)
		}
	}
	if !exists(t, m, path) {
		t.Error("archive must still exist")
	}
}

func TestReplaceReplaceErrorNilFields(t *testing.T) {
	e := &ReplaceError{Err: errors.New("a")}
	if got := e.Unwrap(); len(got) != 1 {
		t.Errorf("unwrap %v", got)
	}
	e = &ReplaceError{RecoverErr: errors.New("b")}
	if got := e.Unwrap(); len(got) != 1 {
		t.Errorf("unwrap %v", got)
	}
}

func TestReplaceArchiveFails(t *testing.T) {
	tests := []struct {
		name string
		hook func(h *hookFS)
		path func() string
	}{
		{"cannot write archive", func(h *hookFS) {
			h.openFile = func(name string) error {
				if strings.HasSuffix(name, ".partial") {
					return errors.New("disk full")
				}
				return nil
			}
		}, func() string { return p("home", "t") }},
		{"target missing", func(*hookFS) {}, func() string { return p("home", "missing") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newMem(t)
			target := p("home", "t")
			mustNil(t, fs.WriteFile(m, target, []byte("old"), 0o644))
			h := &hookFS{Manager: m}
			tc.hook(h)
			s := newStore(h)
			called := false
			path, err := s.Replace(tc.path(), func() error { called = true; return nil })
			if err == nil || path != "" {
				t.Fatalf("path=%q err=%v", path, err)
			}
			if called {
				t.Fatal("place called although archiving failed")
			}
			data, _ := fs.ReadFile(m, target)
			if string(data) != "old" {
				t.Fatalf("target changed: %q", data)
			}
			if got := asideNames(t, m, p("home")); len(got) != 0 {
				t.Fatalf("aside: %v", got)
			}
		})
	}
}

func TestReplaceMoveAsideFails(t *testing.T) {
	errMove := errors.New("busy")
	m := newMem(t)
	target := p("home", "t")
	mustNil(t, fs.WriteFile(m, target, []byte("old"), 0o644))
	h := &hookFS{Manager: m}
	h.rename = func(o, n string) error {
		if o == target {
			return errMove
		}
		return nil
	}
	called := false
	path, err := newStore(h).Replace(target, func() error { called = true; return nil })
	if !errors.Is(err, errMove) {
		t.Fatalf("err = %v", err)
	}
	if called {
		t.Fatal("place called")
	}
	if path == "" {
		t.Error("archive path should be reported (archive was created)")
	}
	data, _ := fs.ReadFile(m, target)
	if string(data) != "old" {
		t.Fatalf("target changed: %q", data)
	}
}

func TestReplaceCleanupWarning(t *testing.T) {
	errRm := errors.New("locked")
	m := newMem(t)
	target := p("home", "t")
	mustNil(t, fs.WriteFile(m, target, []byte("old"), 0o644))
	h := &hookFS{Manager: m}
	h.removeAll = func(path string) error {
		if strings.Contains(path, ".dots-aside-") {
			return errRm
		}
		return nil
	}
	path, err := newStore(h).Replace(target, func() error {
		return fs.WriteFile(m, target, []byte("new"), 0o644)
	})
	var cw *CleanupWarning
	if !errors.As(err, &cw) {
		t.Fatalf("want *CleanupWarning, got %v", err)
	}
	if !errors.Is(err, errRm) {
		t.Error("CleanupWarning must unwrap to the cause")
	}
	if path == "" || !exists(t, m, path) {
		t.Errorf("archive path must accompany the warning: %q", path)
	}
	if !strings.Contains(cw.AsidePath, ".dots-aside-") || !exists(t, m, cw.AsidePath) {
		t.Errorf("aside %q should remain", cw.AsidePath)
	}
	if !strings.Contains(cw.Error(), cw.AsidePath) {
		t.Errorf("message %q", cw.Error())
	}
	data, _ := fs.ReadFile(m, target)
	if string(data) != "new" {
		t.Errorf("new content = %q", data)
	}
	var re *ReplaceError
	if errors.As(err, &re) {
		t.Error("warning must not be ReplaceError")
	}
}
