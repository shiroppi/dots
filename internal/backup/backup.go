// Package backup implements the dots backup store (spec §7.1) and restore
// (spec §7.2).
//
// Before dots replaces an existing target it archives it into a gzip
// compressed tar file inside the backup directory. Archives are never
// overwritten and never deleted automatically. Restore validates a whole
// archive before touching the file system and never follows symlinks stored
// in an archive.
//
// Permissions: the backup directory is created with mode 0700 and archives
// with mode 0600. On Windows these bits are only best-effort; the default
// backup directory lives under %LOCALAPPDATA%, which is private to the
// current user.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/platform"
)

// Manifest format identification.
const (
	// FormatName is the value of Manifest.Format.
	FormatName = "dots-backup"
	// FormatVersion is the only manifest version understood by this package.
	FormatVersion = 1

	manifestName = "manifest.json"
	payloadName  = "payload"
)

// Target kinds recorded in the manifest and reported by RestorePlan.
const (
	KindFile    = "file"
	KindDir     = "dir"
	KindSymlink = "symlink"
	// KindOther is only used for RestorePlan.ExistingKind.
	KindOther = "other"
)

// Sentinel errors. Returned errors wrap them so errors.Is works.
var (
	// ErrInvalidArchive reports a backup archive that is corrupt, malformed
	// or unsafe to extract.
	ErrInvalidArchive = errors.New("invalid backup archive")
	// ErrUnsupportedType reports a file system item that is neither a
	// regular file, a directory nor a symlink.
	ErrUnsupportedType = errors.New("unsupported file type")
	// ErrConflict reports that the restore target exists and the caller did
	// not allow backing it up and replacing it.
	ErrConflict = errors.New("restore target already exists")
	// ErrStateChanged reports that the target or the archive changed after
	// the restore plan was made.
	ErrStateChanged = errors.New("state changed since the restore plan was made")
)

// Manifest is the manifest.json document stored first in every archive.
type Manifest struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	// Target is the original absolute runtime path of the archived item.
	Target string `json:"target"`
	// Created is the creation time in RFC 3339 (UTC).
	Created string `json:"created"`
	// Kind is KindFile, KindDir or KindSymlink.
	Kind string `json:"kind"`
}

// DefaultDir returns the default backup directory for env.GOOS:
//
//   - windows: %LOCALAPPDATA%\dots\backups (LOCALAPPDATA must be absolute)
//   - darwin:  ~/Library/Application Support/dots/backups
//   - others:  $XDG_DATA_HOME/dots/backups when set and non-empty (it must
//     be absolute), otherwise ~/.local/share/dots/backups
func DefaultDir(env platform.Env) (string, error) {
	switch env.GOOS {
	case "windows":
		v := env.Getenv("LOCALAPPDATA")
		if v == "" {
			return "", errors.New("LOCALAPPDATA is not set; cannot determine the backup directory")
		}
		if !filepath.IsAbs(v) {
			return "", fmt.Errorf("LOCALAPPDATA must be an absolute path, got %q", v)
		}
		return filepath.Join(v, "dots", "backups"), nil
	case "darwin":
		if env.Home == "" {
			return "", errors.New("home directory is unknown; cannot determine the backup directory")
		}
		return filepath.Join(env.Home, "Library", "Application Support", "dots", "backups"), nil
	default:
		if v := env.Getenv("XDG_DATA_HOME"); v != "" {
			if !filepath.IsAbs(v) {
				return "", fmt.Errorf("XDG_DATA_HOME must be an absolute path, got %q", v)
			}
			return filepath.Join(v, "dots", "backups"), nil
		}
		if env.Home == "" {
			return "", errors.New("home directory is unknown; cannot determine the backup directory")
		}
		return filepath.Join(env.Home, ".local", "share", "dots", "backups"), nil
	}
}

// Store creates and restores backup archives inside Dir.
type Store struct {
	FS  fs.Manager
	Dir string
	// Now returns the current time; nil means time.Now.
	Now func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func randHex() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely; fall back to the clock.
		return fmt.Sprintf("%012x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// sanitizeBase turns the base name of target into a file name fragment that
// is safe on every supported OS.
func sanitizeBase(target string) string {
	base := filepath.Base(target)
	var sb strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			sb.WriteRune(r)
		default:
			sb.WriteByte('_')
		}
	}
	out := strings.TrimLeft(sb.String(), ".")
	if len(out) > 64 {
		out = out[:64]
	}
	if out == "" || strings.Trim(out, "_") == "" {
		return "target"
	}
	return out
}

func kindOf(info fs.FileInfo) (string, error) {
	mode := info.Mode()
	switch {
	case mode&os.ModeSymlink != 0:
		return KindSymlink, nil
	case mode.IsDir():
		return KindDir, nil
	case mode.IsRegular():
		return KindFile, nil
	}
	return "", fmt.Errorf("%w: %s (mode %s)", ErrUnsupportedType, info.Name(), mode.Type())
}

func isWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// Archive stores target (a file, a directory tree or a symlink; symlinks are
// never followed) in a new archive and returns the archive path. The archive
// is written to a temporary file first and renamed into place only after it
// was completely written, so a failure leaves neither an archive nor a
// temporary file behind. The target itself is never modified.
//
// Archives are named <base>-<UTC yyyymmddTHHMMSSZ>-<seq>.tar.gz; seq makes
// the name unique and existing archives are never overwritten.
func (s *Store) Archive(target string) (string, error) {
	if !filepath.IsAbs(target) {
		return "", fmt.Errorf("backup target must be an absolute path, got %q", target)
	}
	target = filepath.Clean(target)
	info, err := s.FS.Lstat(target)
	if err != nil {
		return "", fmt.Errorf("back up %s: %w", target, err)
	}
	kind, err := kindOf(info)
	if err != nil {
		return "", fmt.Errorf("back up %s: %w", target, err)
	}
	if filepath.IsAbs(s.Dir) && isWithin(target, filepath.Clean(s.Dir)) {
		return "", fmt.Errorf("back up %s: the backup directory %s is inside the target", target, s.Dir)
	}

	if err := s.FS.MkdirAll(s.Dir, 0o700); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	if err := s.FS.Chmod(s.Dir, 0o700); err != nil {
		return "", fmt.Errorf("restrict backup directory permissions: %w", err)
	}

	tmp := filepath.Join(s.Dir, ".tmp-"+randHex()+".partial")
	f, err := s.FS.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create temporary archive: %w", err)
	}
	now := s.now().UTC()
	werr := s.write(f, target, info, kind, now)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = s.FS.RemoveAll(tmp)
		return "", fmt.Errorf("back up %s: %w", target, werr)
	}
	_ = s.FS.Chmod(tmp, 0o600) // best effort on platforms without mode bits

	prefix := fmt.Sprintf("%s-%s-", sanitizeBase(target), now.Format("20060102T150405Z"))
	const maxSeq = 10000
	for seq := 1; seq <= maxSeq; seq++ {
		final := filepath.Join(s.Dir, fmt.Sprintf("%s%04d.tar.gz", prefix, seq))
		if _, err := s.FS.Lstat(final); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			_ = s.FS.RemoveAll(tmp)
			return "", fmt.Errorf("inspect %s: %w", final, err)
		}
		// Rename refuses an existing destination, so a concurrent writer
		// can never be overwritten.
		err := s.FS.Rename(tmp, final)
		if err == nil {
			return final, nil
		}
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		_ = s.FS.RemoveAll(tmp)
		return "", fmt.Errorf("finalize archive: %w", err)
	}
	_ = s.FS.RemoveAll(tmp)
	return "", fmt.Errorf("no free archive name with prefix %s in %s", prefix, s.Dir)
}

func (s *Store) write(w io.Writer, target string, info fs.FileInfo, kind string, now time.Time) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	err := func() error {
		m := Manifest{
			Format:  FormatName,
			Version: FormatVersion,
			Target:  target,
			Created: now.Format(time.RFC3339),
			Kind:    kind,
		}
		data, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return err
		}
		hdr := &tar.Header{Typeflag: tar.TypeReg, Name: manifestName, Mode: 0o600, Size: int64(len(data)), ModTime: now}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
		return s.addEntry(tw, target, payloadName, info)
	}()
	if err != nil {
		_ = tw.Close()
		_ = gz.Close()
		return err
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return err
	}
	return gz.Close()
}

func (s *Store) addEntry(tw *tar.Writer, src, name string, info fs.FileInfo) error {
	if _, err := payloadRel(name, tar.TypeReg); err != nil {
		return fmt.Errorf("%w: %s", ErrUnsupportedType, err)
	}
	mode := info.Mode()
	hdr := &tar.Header{Name: name, Mode: int64(mode.Perm()), ModTime: info.ModTime()}
	switch {
	case mode&os.ModeSymlink != 0:
		link, err := s.FS.Readlink(src)
		if err != nil {
			return err
		}
		hdr.Typeflag = tar.TypeSymlink
		hdr.Linkname = link
		return tw.WriteHeader(hdr)
	case mode.IsDir():
		hdr.Typeflag = tar.TypeDir
		hdr.Name = name + "/"
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		children, err := s.FS.ReadDir(src)
		if err != nil {
			return err
		}
		for _, c := range children {
			if err := s.addEntry(tw, filepath.Join(src, c.Name()), name+"/"+c.Name(), c); err != nil {
				return err
			}
		}
		return nil
	case mode.IsRegular():
		hdr.Typeflag = tar.TypeReg
		hdr.Size = info.Size()
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := s.FS.Open(src)
		if err != nil {
			return err
		}
		_, cerr := io.CopyN(tw, f, hdr.Size)
		f.Close()
		return cerr
	}
	return fmt.Errorf("%w: %s (mode %s)", ErrUnsupportedType, src, mode.Type())
}
