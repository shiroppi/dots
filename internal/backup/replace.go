package backup

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/shiroppi/dots/internal/fs"
)

// ReplaceError reports that placing new content failed and the original
// content could not be put back. The original content is still available at
// AsidePath (when it exists) and in the archive at ArchivePath.
type ReplaceError struct {
	Target      string
	AsidePath   string
	ArchivePath string
	// Err is the error returned by the place function.
	Err error
	// RecoverErr is the error that prevented restoring the original.
	RecoverErr error
}

func (e *ReplaceError) Error() string {
	return fmt.Sprintf("replacing %s failed (%v) and the original could not be restored (%v); "+
		"recover it manually from %s or from the backup archive %s",
		e.Target, e.Err, e.RecoverErr, e.AsidePath, e.ArchivePath)
}

// Unwrap exposes both underlying errors to errors.Is and errors.As.
func (e *ReplaceError) Unwrap() []error {
	var errs []error
	if e.Err != nil {
		errs = append(errs, e.Err)
	}
	if e.RecoverErr != nil {
		errs = append(errs, e.RecoverErr)
	}
	return errs
}

// CleanupWarning is returned by Replace (together with the archive path)
// when the replacement succeeded but the moved-aside original could not be
// deleted. It is not a failure: callers should detect it with errors.As,
// treat the operation as successful and report AsidePath as a leftover.
type CleanupWarning struct {
	AsidePath string
	Err       error
}

func (w *CleanupWarning) Error() string {
	return fmt.Sprintf("replacement succeeded but the temporary copy of the original at %s could not be removed: %v", w.AsidePath, w.Err)
}

func (w *CleanupWarning) Unwrap() error { return w.Err }

// ReplaceSucceeded reports that the replacement itself succeeded; callers
// that must not import this package can detect it through an interface.
func (w *CleanupWarning) ReplaceSucceeded() bool { return true }

// Replace archives target, moves it aside, calls place to create the new
// content at target and then deletes the moved-aside original.
//
//   - Archiving fails: the target is untouched and the error is returned.
//   - place fails: whatever place created at target is removed and the
//     original is moved back; the returned error wraps place's error. If this
//     recovery fails a *ReplaceError is returned.
//   - Success: the archive path is returned. If deleting the aside copy
//     fails, the error is a *CleanupWarning (still a success).
//
// *Store satisfies apply.Replacer.
func (s *Store) Replace(target string, place func() error) (string, error) {
	archivePath, err := s.Archive(target)
	if err != nil {
		return "", err
	}
	target = filepath.Clean(target)
	aside := filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".dots-aside-"+randHex())
	if err := s.FS.Rename(target, aside); err != nil {
		return archivePath, fmt.Errorf("move %s aside: %w", target, err)
	}
	if perr := place(); perr != nil {
		if rerr := s.recoverAside(target, aside); rerr != nil {
			return archivePath, &ReplaceError{
				Target: target, AsidePath: aside, ArchivePath: archivePath,
				Err: perr, RecoverErr: rerr,
			}
		}
		return archivePath, fmt.Errorf("place new content at %s (original restored, backup %s): %w", target, archivePath, perr)
	}
	if err := s.FS.RemoveAll(aside); err != nil {
		return archivePath, &CleanupWarning{AsidePath: aside, Err: err}
	}
	return archivePath, nil
}

func (s *Store) recoverAside(target, aside string) error {
	if _, err := s.FS.Lstat(target); err == nil {
		if err := s.FS.RemoveAll(target); err != nil {
			return fmt.Errorf("remove partial content at %s: %w", target, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect %s: %w", target, err)
	}
	if err := s.FS.Rename(aside, target); err != nil {
		return fmt.Errorf("move original back: %w", err)
	}
	return nil
}
