package fs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
)

// memManager adapts go-billy's memfs to Manager and corrects behaviours where
// memfs differs from a real OS file system:
//
//   - Stat resolves symlink chains itself with a hop limit (memfs recurses
//     forever on cyclic links).
//   - Create/OpenFile/Symlink require an existing parent directory (memfs
//     silently creates parents).
//   - ReadDir fails on non-directories.
//   - Rename moves whole trees without the memfs prefix-matching bug that
//     also moves siblings sharing a name prefix.
//   - Chmod and Chtimes are emulated with a metadata overlay.
//
// Known limitation: symlinks in intermediate path components are not
// resolved. Code should only rely on final-component symlink semantics,
// checking parent components explicitly with Lstat.
type memManager struct {
	mu    sync.Mutex
	b     billy.Filesystem
	perm  map[string]FileMode
	mtime map[string]time.Time
}

// NewMem returns an in-memory Manager for tests. Paths must be absolute host
// paths (for example "/home/u" on Unix or `C:\home\u` on Windows).
func NewMem() Manager {
	return &memManager{
		b:     memfs.New(),
		perm:  map[string]FileMode{},
		mtime: map[string]time.Time{},
	}
}

func clean(p string) string { return filepath.Clean(p) }

func pathErr(op, path string, err error) error {
	return &os.PathError{Op: op, Path: path, Err: err}
}

type overlayInfo struct {
	FileInfo
	mode  FileMode
	mtime time.Time
}

func (o overlayInfo) Mode() FileMode     { return o.mode }
func (o overlayInfo) ModTime() time.Time { return o.mtime }
func (o overlayInfo) IsDir() bool        { return o.mode.IsDir() }

func (m *memManager) wrap(p string, fi FileInfo) FileInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	mode := fi.Mode()
	if perm, ok := m.perm[p]; ok {
		mode = (mode &^ os.ModePerm) | (perm & os.ModePerm)
	}
	mt := fi.ModTime()
	if t, ok := m.mtime[p]; ok {
		mt = t
	}
	return overlayInfo{FileInfo: fi, mode: mode, mtime: mt}
}

func (m *memManager) Lstat(name string) (FileInfo, error) {
	p := clean(name)
	fi, err := m.b.Lstat(p)
	if err != nil {
		return nil, pathErr("lstat", name, ErrNotExist)
	}
	return m.wrap(p, fi), nil
}

// resolve follows symlinks on the final component and returns the resolved
// path of an existing non-symlink item.
func (m *memManager) resolve(name string) (string, error) {
	p := clean(name)
	for i := 0; i <= MaxLinkHops; i++ {
		fi, err := m.b.Lstat(p)
		if err != nil {
			return "", pathErr("stat", name, ErrNotExist)
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			return p, nil
		}
		target, err := m.b.Readlink(p)
		if err != nil {
			return "", pathErr("stat", name, err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(p), target)
		}
		p = clean(target)
	}
	return "", pathErr("stat", name, ErrLinkLoop)
}

func (m *memManager) Stat(name string) (FileInfo, error) {
	p, err := m.resolve(name)
	if err != nil {
		return nil, err
	}
	fi, err := m.b.Lstat(p)
	if err != nil {
		return nil, pathErr("stat", name, ErrNotExist)
	}
	return m.wrap(p, renamed{fi, filepath.Base(clean(name))}), nil
}

type renamed struct {
	FileInfo
	name string
}

func (r renamed) Name() string { return r.name }

func (m *memManager) Readlink(name string) (string, error) {
	p := clean(name)
	fi, err := m.b.Lstat(p)
	if err != nil {
		return "", pathErr("readlink", name, ErrNotExist)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return "", pathErr("readlink", name, errors.New("not a symlink"))
	}
	return m.b.Readlink(p)
}

func (m *memManager) requireParentDir(op, name string) error {
	parent := filepath.Dir(clean(name))
	if parent == clean(name) {
		return nil
	}
	fi, err := m.b.Lstat(parent)
	if err != nil {
		return pathErr(op, name, ErrNotExist)
	}
	if !fi.IsDir() {
		return pathErr(op, name, errors.New("parent is not a directory"))
	}
	return nil
}

func (m *memManager) Symlink(oldname, newname string) error {
	p := clean(newname)
	if _, err := m.b.Lstat(p); err == nil {
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: ErrExist}
	}
	if err := m.requireParentDir("symlink", p); err != nil {
		return err
	}
	return m.b.Symlink(oldname, p)
}

func (m *memManager) ReadDir(name string) ([]FileInfo, error) {
	p, err := m.resolve(name)
	if err != nil {
		return nil, pathErr("readdir", name, err)
	}
	fi, err := m.b.Lstat(p)
	if err != nil {
		return nil, pathErr("readdir", name, ErrNotExist)
	}
	if !fi.IsDir() {
		return nil, pathErr("readdir", name, errors.New("not a directory"))
	}
	entries, err := m.b.ReadDir(p)
	if err != nil {
		return nil, err
	}
	out := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, m.wrap(filepath.Join(p, e.Name()), e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func (m *memManager) MkdirAll(path string, perm FileMode) error {
	p := clean(path)
	// Reject non-directory components explicitly so behaviour matches the OS.
	for cur := p; ; cur = filepath.Dir(cur) {
		if fi, err := m.b.Lstat(cur); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				if _, err := m.resolveDir(cur); err != nil {
					return pathErr("mkdir", path, err)
				}
			} else if !fi.IsDir() {
				return pathErr("mkdir", path, errors.New("not a directory"))
			}
			break
		}
		if filepath.Dir(cur) == cur {
			break
		}
	}
	return m.b.MkdirAll(p, perm)
}

func (m *memManager) resolveDir(p string) (string, error) {
	r, err := m.resolve(p)
	if err != nil {
		return "", err
	}
	fi, err := m.b.Lstat(r)
	if err != nil || !fi.IsDir() {
		return "", errors.New("not a directory")
	}
	return r, nil
}

func (m *memManager) Open(name string) (File, error) {
	return m.OpenFile(name, os.O_RDONLY, 0)
}

func (m *memManager) Create(name string) (File, error) {
	return m.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

func (m *memManager) OpenFile(name string, flag int, perm FileMode) (File, error) {
	p := clean(name)
	_, err := m.b.Lstat(p)
	exists := err == nil
	if exists && flag&os.O_CREATE != 0 && flag&os.O_EXCL != 0 {
		return nil, pathErr("open", name, ErrExist)
	}
	if exists {
		r, err := m.resolve(p)
		if err != nil {
			return nil, pathErr("open", name, err)
		}
		p = r
		fi, _ := m.b.Lstat(p)
		if fi.IsDir() && flag&(os.O_WRONLY|os.O_RDWR) != 0 {
			return nil, pathErr("open", name, errors.New("is a directory"))
		}
	} else {
		if flag&os.O_CREATE == 0 {
			return nil, pathErr("open", name, ErrNotExist)
		}
		if err := m.requireParentDir("open", p); err != nil {
			return nil, err
		}
	}
	f, err := m.b.OpenFile(p, flag, perm)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (m *memManager) Rename(oldpath, newpath string) error {
	from, to := clean(oldpath), clean(newpath)
	if _, err := m.b.Lstat(from); err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: ErrNotExist}
	}
	if _, err := m.b.Lstat(to); err == nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: ErrExist}
	}
	if from == to {
		return nil
	}
	if rel, err := filepath.Rel(from, to); err == nil && rel != ".." && !startsWithDotDot(rel) {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errors.New("cannot move a directory into itself")}
	}
	if err := m.requireParentDir("rename", to); err != nil {
		return err
	}
	if err := m.copyTree(from, to); err != nil {
		return err
	}
	return m.RemoveAll(from)
}

func startsWithDotDot(rel string) bool {
	return len(rel) >= 3 && rel[:2] == ".." && os.IsPathSeparator(rel[2])
}

func (m *memManager) copyTree(from, to string) error {
	fi, err := m.b.Lstat(from)
	if err != nil {
		return err
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		target, err := m.b.Readlink(from)
		if err != nil {
			return err
		}
		if err := m.b.Symlink(target, to); err != nil {
			return err
		}
	case fi.IsDir():
		if err := m.b.MkdirAll(to, fi.Mode().Perm()); err != nil {
			return err
		}
		entries, err := m.b.ReadDir(from)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := m.copyTree(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
				return err
			}
		}
	default:
		src, err := m.b.Open(from)
		if err != nil {
			return err
		}
		dst, err := m.b.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
		if err != nil {
			_ = src.Close() // read-only
			return err
		}
		_, cerr := io.Copy(dst, src)
		_ = src.Close() // read-only
		if err := dst.Close(); cerr == nil {
			cerr = err
		}
		if cerr != nil {
			return cerr
		}
	}
	m.mu.Lock()
	if v, ok := m.perm[from]; ok {
		m.perm[to] = v
	}
	if v, ok := m.mtime[from]; ok {
		m.mtime[to] = v
	}
	m.mu.Unlock()
	return nil
}

func (m *memManager) forget(p string) {
	m.mu.Lock()
	delete(m.perm, p)
	delete(m.mtime, p)
	m.mu.Unlock()
}

func (m *memManager) Remove(name string) error {
	p := clean(name)
	fi, err := m.b.Lstat(p)
	if err != nil {
		return pathErr("remove", name, ErrNotExist)
	}
	if fi.IsDir() {
		entries, err := m.b.ReadDir(p)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return pathErr("remove", name, errors.New("directory not empty"))
		}
	}
	if err := m.b.Remove(p); err != nil {
		return err
	}
	m.forget(p)
	return nil
}

func (m *memManager) RemoveAll(path string) error {
	p := clean(path)
	fi, err := m.b.Lstat(p)
	if err != nil {
		return nil
	}
	if fi.IsDir() {
		entries, err := m.b.ReadDir(p)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := m.RemoveAll(filepath.Join(p, e.Name())); err != nil {
				return err
			}
		}
	}
	return m.Remove(p)
}

func (m *memManager) Chmod(name string, mode FileMode) error {
	p, err := m.resolve(name)
	if err != nil {
		return pathErr("chmod", name, err)
	}
	m.mu.Lock()
	m.perm[p] = mode & os.ModePerm
	m.mu.Unlock()
	return nil
}

func (m *memManager) Chtimes(name string, _, mtime time.Time) error {
	p, err := m.resolve(name)
	if err != nil {
		return pathErr("chtimes", name, err)
	}
	m.mu.Lock()
	m.mtime[p] = mtime
	m.mu.Unlock()
	return nil
}
