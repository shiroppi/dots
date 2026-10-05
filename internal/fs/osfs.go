package fs

import (
	"errors"
	"os"
	"sort"
	"time"
)

const flagWriteTrunc = os.O_WRONLY | os.O_CREATE | os.O_TRUNC

type osManager struct{}

// NewOS returns a Manager backed by the os package.
func NewOS() Manager { return osManager{} }

func (osManager) Lstat(name string) (FileInfo, error) { return os.Lstat(name) }
func (osManager) Stat(name string) (FileInfo, error) {
	fi, err := os.Stat(name)
	if err != nil && isLinkLoop(err) && !errors.Is(err, ErrLinkLoop) {
		return nil, &linkLoopError{err: err}
	}
	return fi, err
}
func (osManager) Readlink(name string) (string, error) { return os.Readlink(name) }

// Symlink pre-checks the destination because on Windows os.Symlink onto an
// existing path retries without the unprivileged flag and reports a
// misleading "privilege not held" error instead of ErrExist.
func (osManager) Symlink(oldname, newname string) error {
	if _, err := os.Lstat(newname); err == nil {
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: os.ErrExist}
	}
	return os.Symlink(oldname, newname)
}
func (osManager) MkdirAll(path string, perm FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osManager) ReadDir(name string) ([]FileInfo, error) {
	entries, err := os.ReadDir(name)
	if err != nil {
		return nil, err
	}
	infos := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name() < infos[j].Name() })
	return infos, nil
}

func (osManager) Open(name string) (File, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (osManager) OpenFile(name string, flag int, perm FileMode) (File, error) {
	f, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (osManager) Create(name string) (File, error) {
	f, err := os.Create(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (osManager) Rename(oldpath, newpath string) error {
	if _, err := os.Lstat(newpath); err == nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: os.ErrExist}
	}
	return os.Rename(oldpath, newpath)
}

func (osManager) Remove(name string) error    { return os.Remove(name) }
func (osManager) RemoveAll(path string) error { return os.RemoveAll(path) }
func (osManager) Chmod(name string, mode FileMode) error {
	return os.Chmod(name, mode)
}
func (osManager) Chtimes(name string, atime, mtime time.Time) error {
	return os.Chtimes(name, atime, mtime)
}

// linkLoopError marks a cyclic symlink error so that both ErrLinkLoop and the
// original OS error remain inspectable with errors.Is / errors.As.
type linkLoopError struct{ err error }

func (e *linkLoopError) Error() string   { return e.err.Error() }
func (e *linkLoopError) Unwrap() []error { return []error{ErrLinkLoop, e.err} }
