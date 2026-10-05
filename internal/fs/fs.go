// Package fs defines the file system abstraction used by every dots
// component. Production code uses NewOS; tests inject NewMem.
//
// All paths passed to a Manager are absolute runtime paths in the host OS
// format (filepath separators). Converting configuration paths written with
// "/" into runtime paths is the responsibility of the caller.
package fs

import (
	"errors"
	"io"
	iofs "io/fs"
	"time"
)

// FileInfo and FileMode are re-exported so callers do not need to import
// io/fs alongside this package.
type (
	FileInfo = iofs.FileInfo
	FileMode = iofs.FileMode
)

// Common sentinel errors. Implementations wrap them so errors.Is works.
var (
	ErrNotExist = iofs.ErrNotExist
	ErrExist    = iofs.ErrExist
	// ErrLinkLoop is reported by Stat when symlink resolution exceeds
	// MaxLinkHops (cyclic or excessively deep symlink chains).
	ErrLinkLoop = errors.New("too many levels of symbolic links")
)

// MaxLinkHops bounds symlink resolution in Stat.
const MaxLinkHops = 40

// File is the subset of *os.File used by dots.
type File interface {
	io.Reader
	io.Writer
	io.Closer
	Name() string
}

// Manager abstracts every file system operation performed by dots.
type Manager interface {
	// Lstat returns information about the named item without following a
	// final symlink.
	Lstat(name string) (FileInfo, error)
	// Stat follows symlinks. Broken links return an error wrapping
	// ErrNotExist; cyclic links return an error wrapping ErrLinkLoop.
	Stat(name string) (FileInfo, error)
	// Readlink returns the raw link string stored in a symlink.
	Readlink(name string) (string, error)
	// Symlink creates newname as a symlink whose content is oldname.
	// oldname is stored verbatim (relative strings stay relative).
	Symlink(oldname, newname string) error

	// ReadDir returns the entries of a directory sorted by name. Returned
	// FileInfo values have Lstat semantics (symlinks are not followed).
	ReadDir(name string) ([]FileInfo, error)
	MkdirAll(path string, perm FileMode) error

	Open(name string) (File, error)
	OpenFile(name string, flag int, perm FileMode) (File, error)
	Create(name string) (File, error)

	// Rename moves an item (file, directory tree, or symlink) to a new path.
	// The destination must not exist.
	Rename(oldpath, newpath string) error
	// Remove deletes a file, a symlink, or an empty directory.
	Remove(name string) error
	// RemoveAll deletes path and any children. A missing path is not an error.
	RemoveAll(path string) error

	Chmod(name string, mode FileMode) error
	Chtimes(name string, atime, mtime time.Time) error
}

// Exists reports whether name exists according to Lstat.
func Exists(m Manager, name string) (bool, error) {
	_, err := m.Lstat(name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotExist) {
		return false, nil
	}
	return false, err
}

// ReadFile reads the whole content of name.
func ReadFile(m Manager, name string) ([]byte, error) {
	f, err := m.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read-only
	return io.ReadAll(f)
}

// WriteFile writes data to name, creating or truncating it.
func WriteFile(m Manager, name string, data []byte, perm FileMode) error {
	f, err := m.OpenFile(name, flagWriteTrunc, perm)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}
