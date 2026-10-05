package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	dfs "github.com/shiroppi/dots/internal/fs"
)

// managers returns both implementations rooted at an absolute directory so the
// same scenario verifies memfs parity with the real OS.
func managers(t *testing.T) map[string]struct {
	m    dfs.Manager
	root string
} {
	t.Helper()
	memRoot := `/root`
	if runtime.GOOS == "windows" {
		memRoot = `C:\root`
	}
	mem := dfs.NewMem()
	if err := mem.MkdirAll(memRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	return map[string]struct {
		m    dfs.Manager
		root string
	}{
		"os":  {dfs.NewOS(), t.TempDir()},
		"mem": {mem, memRoot},
	}
}

func skipIfNoSymlink(t *testing.T, m dfs.Manager, root string) {
	t.Helper()
	probe := filepath.Join(root, "probe-link")
	if err := m.Symlink("x", probe); err != nil {
		t.Skipf("symlinks not permitted on this host: %v", err)
	}
	_ = m.Remove(probe)
}

func TestParity(t *testing.T) {
	for name, tc := range managers(t) {
		m, root := tc.m, tc.root
		t.Run(name, func(t *testing.T) {
			skipIfNoSymlink(t, m, root)

			// Create requires an existing parent.
			if _, err := m.Create(filepath.Join(root, "missing", "f")); err == nil {
				t.Fatal("Create without parent should fail")
			}
			if err := m.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := dfs.WriteFile(m, filepath.Join(root, "a", "b", "f.txt"), []byte("hi"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := dfs.WriteFile(m, filepath.Join(root, "a", "bc"), []byte("sibling"), 0o644); err != nil {
				t.Fatal(err)
			}

			// Relative symlink resolves against the link's directory.
			link := filepath.Join(root, "link")
			if err := m.Symlink(filepath.Join("a", "b", "f.txt"), link); err != nil {
				t.Fatal(err)
			}
			if got, _ := m.Readlink(link); got != filepath.Join("a", "b", "f.txt") {
				t.Fatalf("Readlink = %q", got)
			}
			if fi, err := m.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("Lstat link: %v %v", fi, err)
			}
			if fi, err := m.Stat(link); err != nil || !fi.Mode().IsRegular() {
				t.Fatalf("Stat link: %v %v", fi, err)
			}
			if err := m.Symlink("x", link); !errors.Is(err, dfs.ErrExist) {
				t.Fatalf("Symlink over existing: %v", err)
			}

			// Broken and cyclic links.
			broken := filepath.Join(root, "broken")
			_ = m.Symlink("nowhere", broken)
			if _, err := m.Stat(broken); !errors.Is(err, dfs.ErrNotExist) {
				t.Fatalf("Stat broken: %v", err)
			}
			if _, err := m.Lstat(broken); err != nil {
				t.Fatalf("Lstat broken: %v", err)
			}
			c1, c2 := filepath.Join(root, "c1"), filepath.Join(root, "c2")
			_ = m.Symlink("c2", c1)
			_ = m.Symlink("c1", c2)
			if _, err := m.Stat(c1); err == nil {
				t.Fatal("Stat cyclic should fail")
			}

			// Rename tree must not move siblings that share a prefix.
			if err := m.Rename(filepath.Join(root, "a", "b"), filepath.Join(root, "moved")); err != nil {
				t.Fatal(err)
			}
			if data, err := dfs.ReadFile(m, filepath.Join(root, "moved", "f.txt")); err != nil || string(data) != "hi" {
				t.Fatalf("moved content: %q %v", data, err)
			}
			if ok, _ := dfs.Exists(m, filepath.Join(root, "a", "bc")); !ok {
				t.Fatal("sibling with shared prefix was moved")
			}
			if err := m.Rename(filepath.Join(root, "moved"), filepath.Join(root, "a", "bc")); err == nil {
				t.Fatal("Rename onto existing should fail")
			}

			// ReadDir is sorted and uses Lstat semantics.
			infos, err := m.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for i := 1; i < len(infos); i++ {
				if infos[i-1].Name() >= infos[i].Name() {
					t.Fatalf("ReadDir not sorted: %s >= %s", infos[i-1].Name(), infos[i].Name())
				}
			}
			for _, fi := range infos {
				if fi.Name() == "link" && fi.Mode()&os.ModeSymlink == 0 {
					t.Fatal("ReadDir followed symlink")
				}
			}
			if _, err := m.ReadDir(filepath.Join(root, "a", "bc")); err == nil {
				t.Fatal("ReadDir on file should fail")
			}

			// Remove refuses non-empty dirs; RemoveAll does not.
			if err := m.Remove(filepath.Join(root, "moved")); err == nil {
				t.Fatal("Remove non-empty dir should fail")
			}
			if err := m.RemoveAll(filepath.Join(root, "moved")); err != nil {
				t.Fatal(err)
			}
			if ok, _ := dfs.Exists(m, filepath.Join(root, "moved")); ok {
				t.Fatal("RemoveAll left data")
			}
			if err := m.RemoveAll(filepath.Join(root, "moved")); err != nil {
				t.Fatalf("RemoveAll missing: %v", err)
			}

			// MkdirAll through a regular file fails.
			if err := m.MkdirAll(filepath.Join(root, "a", "bc", "x"), 0o755); err == nil {
				t.Fatal("MkdirAll through file should fail")
			}
		})
	}
}

func TestOSStatLinkLoop(t *testing.T) {
	d := t.TempDir()
	a, b := filepath.Join(d, "a"), filepath.Join(d, "b")
	if err := os.Symlink("b", a); err != nil {
		t.Skipf("symlinks not permitted: %v", err)
	}
	if err := os.Symlink("a", b); err != nil {
		t.Skipf("symlinks not permitted: %v", err)
	}
	_, err := dfs.NewOS().Stat(a)
	if !errors.Is(err, dfs.ErrLinkLoop) {
		t.Fatalf("want ErrLinkLoop, got %v", err)
	}
	var pe *os.PathError
	if !errors.As(err, &pe) {
		t.Fatalf("original PathError not inspectable: %v", err)
	}
}
