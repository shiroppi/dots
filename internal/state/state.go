// Package state evaluates the current file system state of resolved links
// (spec §6). Evaluation is read-only: doctor, apply --dry-run, and apply all
// share it.
package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/model"
)

// Sentinel errors wrapped by Evaluate and RelLink so callers can classify
// validation failures with errors.Is.
var (
	// ErrCrossVolume reports that source and target live on different
	// volumes (Windows drives / UNC shares), so no relative link exists.
	ErrCrossVolume = errors.New("source and target are on different volumes; a relative symlink cannot be created")
	// ErrSourceNotExist reports a missing source item.
	ErrSourceNotExist = errors.New("source does not exist")
	// ErrSourceBrokenLink reports a source symlink whose destination is missing.
	ErrSourceBrokenLink = errors.New("source symlink is broken")
	// ErrSourceLinkLoop reports a cyclic or otherwise unresolvable source symlink.
	ErrSourceLinkLoop = errors.New("source symlink is cyclic or cannot be resolved")
	// ErrUnsupportedType reports an item that is neither a regular file, a
	// directory, nor a symlink (device, socket, named pipe, ...).
	ErrUnsupportedType = errors.New("unsupported file type")
	// ErrUnsafeParent reports that a parent of the target is a regular file or
	// a symlink, so the link cannot be placed safely.
	ErrUnsafeParent = errors.New("cannot safely place the link")
	// ErrOverlap reports that source and target are the same path or one
	// contains the other.
	ErrOverlap = errors.New("source and target overlap")
)

// Status is the state of a target path (spec §6 table).
type Status int

const (
	// Unknown is the zero value, used only when the target could not be
	// inspected (Result.Err is then non-nil).
	Unknown Status = iota
	// NotExist: nothing exists at the target.
	NotExist
	// ValidLink: the target is a symlink pointing at the expected source item.
	ValidLink
	// InvalidLink: the target is a symlink pointing elsewhere (including a
	// broken link).
	InvalidLink
	// FileOrDir: a regular file or directory occupies the target.
	FileOrDir
	// ValidDir: the target is a real directory (not a symlink) for a KindDir
	// entry.
	ValidDir
)

func (s Status) String() string {
	switch s {
	case Unknown:
		return "Unknown"
	case NotExist:
		return "NotExist"
	case ValidLink:
		return "ValidLink"
	case InvalidLink:
		return "InvalidLink"
	case FileOrDir:
		return "FileOrDir"
	case ValidDir:
		return "ValidDir"
	default:
		return fmt.Sprintf("Status(%d)", int(s))
	}
}

// Result is the evaluation of one entry.
type Result struct {
	Entry model.Entry
	// Status of the target path (Lstat semantics, never following a link).
	Status Status
	// Link is the relative link text that apply writes into the symlink
	// (empty when it cannot be computed).
	Link string
	// Current is the raw text of the existing target symlink when Status is
	// ValidLink or InvalidLink.
	Current string
	// Err is non-nil when the entry fails validation; apply must not start.
	// Multiple problems are combined with errors.Join.
	Err error
}

// OK reports whether the entry is valid and already in place (a correct link
// or an existing real directory).
func (r Result) OK() bool {
	return r.Err == nil && (r.Status == ValidLink || r.Status == ValidDir)
}

// PathEqual compares two cleaned runtime paths. It is case-insensitive on
// hosts whose default file systems are case-insensitive (Windows, macOS).
// Tests may replace it.
var PathEqual = defaultPathEqual()

func defaultPathEqual() func(a, b string) bool {
	switch runtime.GOOS {
	case "windows", "darwin":
		return strings.EqualFold
	default:
		return func(a, b string) bool { return a == b }
	}
}

// RelLink returns the relative link text that, stored in a symlink at target,
// points at source. Both paths must be absolute runtime paths.
func RelLink(source, target string) (string, error) {
	source, target = filepath.Clean(source), filepath.Clean(target)
	sv, tv := filepath.VolumeName(source), filepath.VolumeName(target)
	if !strings.EqualFold(sv, tv) {
		return "", fmt.Errorf("%w (source %s, target %s)", ErrCrossVolume, source, target)
	}
	rel, err := filepath.Rel(filepath.Dir(target), source)
	if err != nil {
		return "", fmt.Errorf("cannot compute relative link from %s to %s: %w", target, source, err)
	}
	return rel, nil
}

// Evaluate inspects the source and target of e without modifying anything.
func Evaluate(fsys fs.Manager, e model.Entry) Result {
	r := Result{Entry: e}
	source, target := filepath.Clean(e.Source), filepath.Clean(e.Target)
	var errs []error

	if e.Kind == model.KindDir {
		status, err := dirStatus(fsys, target)
		r.Status = status
		if err != nil {
			errs = append(errs, err)
		}
		if err := checkParent(fsys, target); err != nil {
			errs = append(errs, err)
		}
		r.Err = errors.Join(errs...)
		return r
	}

	if err := checkSource(fsys, source); err != nil {
		errs = append(errs, err)
	}
	if err := checkOverlap(source, target); err != nil {
		errs = append(errs, err)
	}
	if link, err := RelLink(source, target); err != nil {
		errs = append(errs, err)
	} else {
		r.Link = link
	}

	status, current, err := targetStatus(fsys, source, target)
	r.Status, r.Current = status, current
	if err != nil {
		errs = append(errs, err)
	}
	if err := checkParent(fsys, target); err != nil {
		errs = append(errs, err)
	}

	r.Err = errors.Join(errs...)
	return r
}

// isNotExist treats ENOTDIR (a path component is a file) like a missing path;
// the parent check reports the offending component.
func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

func checkSource(fsys fs.Manager, source string) error {
	fi, err := fsys.Lstat(source)
	if err != nil {
		if isNotExist(err) {
			return fmt.Errorf("%w: %s", ErrSourceNotExist, source)
		}
		return fmt.Errorf("cannot inspect source %s: %w", source, err)
	}
	mode := fi.Mode()
	switch {
	case mode&os.ModeSymlink != 0:
		// The link item itself is the source; resolving it only validates it.
		if _, err := fsys.Stat(source); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("%w: %s", ErrSourceBrokenLink, source)
			}
			return fmt.Errorf("%w: %s: %v", ErrSourceLinkLoop, source, err)
		}
	case mode.IsRegular(), mode.IsDir():
	default:
		return fmt.Errorf("%w: source %s (%s)", ErrUnsupportedType, source, mode.Type())
	}
	return nil
}

// within reports whether p equals base or lies below it.
func within(p, base string) bool {
	if PathEqual(p, base) {
		return true
	}
	prefix := base
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return len(p) > len(prefix) && PathEqual(p[:len(prefix)], prefix)
}

func checkOverlap(source, target string) error {
	switch {
	case PathEqual(source, target):
		return fmt.Errorf("%w: target %s is the source itself", ErrOverlap, target)
	case within(source, target):
		return fmt.Errorf("%w: target %s contains source %s", ErrOverlap, target, source)
	case within(target, source):
		return fmt.Errorf("%w: target %s is inside source %s", ErrOverlap, target, source)
	}
	return nil
}

// resolveLinkText turns raw symlink text stored at target into a cleaned
// absolute path.
func resolveLinkText(target, text string) string {
	switch {
	case filepath.IsAbs(text):
		return filepath.Clean(text)
	case filepath.VolumeName(text) == "" && text != "" && os.IsPathSeparator(text[0]):
		// Windows root-relative path (`\x`): relative to the target's volume.
		return filepath.Clean(filepath.VolumeName(target) + text)
	default:
		return filepath.Clean(filepath.Join(filepath.Dir(target), text))
	}
}

func targetStatus(fsys fs.Manager, source, target string) (Status, string, error) {
	fi, err := fsys.Lstat(target)
	if err != nil {
		if isNotExist(err) {
			return NotExist, "", nil
		}
		return Unknown, "", fmt.Errorf("cannot inspect target %s: %w", target, err)
	}
	mode := fi.Mode()
	switch {
	case mode&os.ModeSymlink != 0:
		text, err := fsys.Readlink(target)
		if err != nil {
			return Unknown, "", fmt.Errorf("cannot read target link %s: %w", target, err)
		}
		if PathEqual(resolveLinkText(target, text), source) {
			return ValidLink, text, nil
		}
		return InvalidLink, text, nil
	case mode.IsRegular(), mode.IsDir():
		return FileOrDir, "", nil
	default:
		return FileOrDir, "", fmt.Errorf("%w: target %s (%s) cannot be backed up or replaced", ErrUnsupportedType, target, mode.Type())
	}
}

// dirStatus classifies the target of a KindDir entry.
func dirStatus(fsys fs.Manager, target string) (Status, error) {
	fi, err := fsys.Lstat(target)
	if err != nil {
		if isNotExist(err) {
			return NotExist, nil
		}
		return Unknown, fmt.Errorf("cannot inspect target %s: %w", target, err)
	}
	mode := fi.Mode()
	switch {
	case mode&os.ModeSymlink != 0:
		return InvalidLink, nil
	case mode.IsDir():
		return ValidDir, nil
	case mode.IsRegular():
		return FileOrDir, nil
	default:
		return FileOrDir, fmt.Errorf("%w: target %s (%s) cannot be backed up or replaced", ErrUnsupportedType, target, mode.Type())
	}
}

// checkParent finds the nearest existing ancestor of target. It must be a
// real directory; ancestors above it are not inspected (system symlinks such
// as FreeBSD's /home -> /usr/home are fine). Missing ancestors are created by
// apply.
func checkParent(fsys fs.Manager, target string) error {
	for cur := filepath.Dir(target); ; {
		fi, err := fsys.Lstat(cur)
		if err == nil {
			mode := fi.Mode()
			switch {
			case mode&os.ModeSymlink != 0:
				return fmt.Errorf("%w: parent %s of target %s is a symlink", ErrUnsafeParent, cur, target)
			case mode.IsDir():
				return nil
			default:
				return fmt.Errorf("%w: parent %s of target %s is not a directory", ErrUnsafeParent, cur, target)
			}
		}
		if !isNotExist(err) {
			return fmt.Errorf("cannot inspect parent %s of target %s: %w", cur, target, err)
		}
		next := filepath.Dir(cur)
		if next == cur {
			return fmt.Errorf("%w: no existing ancestor directory for target %s", ErrUnsafeParent, target)
		}
		cur = next
	}
}
