package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shiroppi/dots/internal/fs"
)

const maxManifestSize = 64 << 10

// Entry describes one validated payload entry of an archive.
type Entry struct {
	// Name is the slash-separated path relative to the payload root; the
	// root itself is "".
	Name string
	// Kind is KindFile, KindDir or KindSymlink.
	Kind     string
	Mode     fs.FileMode
	ModTime  time.Time
	Size     int64
	Linkname string
}

// Archive is a fully validated backup archive.
type Archive struct {
	Path     string
	Manifest Manifest
	// Entries lists payload entries in archive order (parents first).
	Entries []Entry
}

func invalidf(format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s", ErrInvalidArchive, fmt.Sprintf(format, args...))
}

// payloadRel validates an archive entry name and returns its path relative to
// the payload root ("" for the root itself).
func payloadRel(name string, typeflag byte) (string, error) {
	n := name
	if typeflag == tar.TypeDir {
		n = strings.TrimSuffix(n, "/")
	}
	switch {
	case n == "":
		return "", errors.New("empty entry name")
	case strings.ContainsAny(n, "\\\x00"):
		return "", fmt.Errorf("entry name %q contains a backslash or NUL", name)
	case strings.HasPrefix(n, "/"):
		return "", fmt.Errorf("absolute entry name %q", name)
	}
	segs := strings.Split(n, "/")
	for _, seg := range segs {
		switch seg {
		case "", ".", "..":
			return "", fmt.Errorf("unsafe entry name %q", name)
		}
		if runtime.GOOS == "windows" && strings.Contains(seg, ":") {
			return "", fmt.Errorf("entry name %q contains ':'", name)
		}
	}
	if segs[0] != payloadName {
		return "", fmt.Errorf("entry %q is outside %s", name, payloadName)
	}
	return strings.Join(segs[1:], "/"), nil
}

type invalidReader struct{ r io.Reader }

func (e invalidReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err != nil && err != io.EOF {
		err = invalidf("read archive: %v", err)
	}
	return n, err
}

// scan reads and validates the whole archive. onManifest (optional) is called
// once the manifest was validated; handle (optional) is called for every
// payload entry with a reader for its content. The returned Archive is only
// meaningful when err is nil.
func scan(fsys fs.Manager, archivePath string,
	onManifest func(Manifest) error,
	handle func(Entry, io.Reader) error) (*Archive, error) {

	f, err := fsys.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read-only: close error is not actionable
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, invalidf("not a gzip file: %v", err)
	}
	tr := tar.NewReader(invalidReader{gz})

	a := &Archive{Path: archivePath}
	seen := map[string]bool{}
	dirs := map[string]bool{}
	haveManifest := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, invalidf("read tar: %v", err)
		}

		if !haveManifest {
			if hdr.Name != manifestName || hdr.Typeflag != tar.TypeReg {
				return nil, invalidf("the first entry must be a regular %s, got %q", manifestName, hdr.Name)
			}
			if hdr.Size > maxManifestSize {
				return nil, invalidf("%s is too large", manifestName)
			}
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, invalidf("read %s: %v", manifestName, err)
			}
			if err := json.Unmarshal(data, &a.Manifest); err != nil {
				return nil, invalidf("parse %s: %v", manifestName, err)
			}
			if err := checkManifest(a.Manifest); err != nil {
				return nil, err
			}
			haveManifest = true
			if onManifest != nil {
				if err := onManifest(a.Manifest); err != nil {
					return nil, err
				}
			}
			continue
		}

		if hdr.Name == manifestName {
			return nil, invalidf("duplicate %s", manifestName)
		}
		var kind string
		switch hdr.Typeflag {
		case tar.TypeReg:
			kind = KindFile
		case tar.TypeDir:
			kind = KindDir
		case tar.TypeSymlink:
			kind = KindSymlink
		default:
			return nil, invalidf("unsupported entry type %q for %q", string(hdr.Typeflag), hdr.Name)
		}
		if kind != KindDir && strings.HasSuffix(hdr.Name, "/") {
			return nil, invalidf("non-directory entry %q ends with '/'", hdr.Name)
		}
		rel, err := payloadRel(hdr.Name, hdr.Typeflag)
		if err != nil {
			return nil, invalidf("%v", err)
		}
		if seen[rel] {
			return nil, invalidf("duplicate entry %q", hdr.Name)
		}
		seen[rel] = true
		if rel == "" {
			if len(a.Entries) != 0 {
				return nil, invalidf("payload root must be the first payload entry")
			}
			if kind != a.Manifest.Kind {
				return nil, invalidf("manifest kind %q does not match payload entry kind %q", a.Manifest.Kind, kind)
			}
		} else {
			parent := path.Dir(rel)
			if parent == "." {
				parent = ""
			}
			if !dirs[parent] {
				return nil, invalidf("entry %q has no preceding parent directory entry", hdr.Name)
			}
		}
		e := Entry{
			Name:    rel,
			Kind:    kind,
			Mode:    hdr.FileInfo().Mode().Perm(),
			ModTime: hdr.ModTime,
		}
		switch kind {
		case KindDir:
			dirs[rel] = true
		case KindSymlink:
			if hdr.Linkname == "" || strings.ContainsRune(hdr.Linkname, 0) {
				return nil, invalidf("symlink entry %q has an invalid link target", hdr.Name)
			}
			e.Linkname = hdr.Linkname
		case KindFile:
			if hdr.Size < 0 {
				return nil, invalidf("entry %q has a negative size", hdr.Name)
			}
			e.Size = hdr.Size
		}
		a.Entries = append(a.Entries, e)
		if handle != nil {
			if err := handle(e, tr); err != nil {
				return nil, err
			}
		}
		if _, err := io.Copy(io.Discard, tr); err != nil {
			return nil, invalidf("read entry %q: %v", hdr.Name, err)
		}
	}
	// Reading to the end of the gzip stream verifies its checksum.
	if _, err := io.Copy(io.Discard, invalidReader{gz}); err != nil {
		return nil, err
	}
	if !haveManifest {
		return nil, invalidf("missing %s", manifestName)
	}
	if len(a.Entries) == 0 {
		return nil, invalidf("missing %s entry", payloadName)
	}
	return a, nil
}

func checkManifest(m Manifest) error {
	if m.Format != FormatName {
		return invalidf("unexpected format %q", m.Format)
	}
	if m.Version != FormatVersion {
		return invalidf("unsupported version %d", m.Version)
	}
	switch m.Kind {
	case KindFile, KindDir, KindSymlink:
	default:
		return invalidf("invalid kind %q", m.Kind)
	}
	if !filepath.IsAbs(m.Target) {
		return invalidf("target %q is not an absolute path on this system", m.Target)
	}
	if _, err := time.Parse(time.RFC3339, m.Created); err != nil {
		return invalidf("invalid created time %q", m.Created)
	}
	return nil
}

// ReadArchive opens the archive at path and validates it completely (manifest,
// every entry name and type, structure and gzip integrity) without touching
// the file system. Any problem is reported as ErrInvalidArchive.
func ReadArchive(fsys fs.Manager, path string) (*Archive, error) {
	return scan(fsys, path, nil, nil)
}

// RestorePlan describes what restoring an archive would do. It is produced
// by PlanRestore without modifying anything.
type RestorePlan struct {
	ArchivePath string
	Manifest    Manifest
	// Target is the absolute path the archive will be restored to.
	Target string
	// Exists reports whether something exists at Target (Lstat).
	Exists bool
	// ExistingKind is KindFile, KindDir, KindSymlink or KindOther; it is
	// empty when Exists is false.
	ExistingKind string
}

func (s *Store) inspectTarget(target string) (exists bool, kind string, err error) {
	info, err := s.FS.Lstat(target)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, "", nil
		}
		return false, "", err
	}
	k, kerr := kindOf(info)
	if kerr != nil {
		k = KindOther
	}
	return true, k, nil
}

// PlanRestore validates the archive and reports what Restore would do. It is
// read-only and suitable for --dry-run and for prompting.
func (s *Store) PlanRestore(archivePath string) (*RestorePlan, error) {
	a, err := ReadArchive(s.FS, archivePath)
	if err != nil {
		return nil, err
	}
	exists, kind, err := s.inspectTarget(a.Manifest.Target)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", a.Manifest.Target, err)
	}
	return &RestorePlan{
		ArchivePath:  archivePath,
		Manifest:     a.Manifest,
		Target:       a.Manifest.Target,
		Exists:       exists,
		ExistingKind: kind,
	}, nil
}

type dirFixup struct {
	path  string
	mode  fs.FileMode
	mtime time.Time
}

// Restore extracts the archive of plan to plan.Target.
//
// The archive is extracted (and re-validated) into a temporary path next to
// the target, so the target is only touched once all content has been
// written. If the target does not exist the temporary item is renamed into
// place. If it exists, backupExisting must be true (otherwise ErrConflict is
// returned and nothing changes); the existing content is then archived by
// Replace, which also restores it if placing fails. newArchive is the path of
// that backup ("" when nothing existed). A *CleanupWarning may accompany a
// successful restore, see Replace.
func (s *Store) Restore(plan *RestorePlan, backupExisting bool) (newArchive string, err error) {
	if plan == nil {
		return "", errors.New("restore plan is nil")
	}
	target := plan.Manifest.Target
	if plan.Target != "" && plan.Target != target {
		return "", fmt.Errorf("%w: plan target %q differs from manifest target %q", ErrStateChanged, plan.Target, target)
	}
	if !filepath.IsAbs(target) {
		return "", invalidf("target %q is not an absolute path", target)
	}
	target = filepath.Clean(target)

	exists, kind, err := s.inspectTarget(target)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", target, err)
	}
	if exists != plan.Exists || kind != plan.ExistingKind {
		return "", fmt.Errorf("%w: %s", ErrStateChanged, target)
	}
	if exists && !backupExisting {
		return "", fmt.Errorf("%w: %s", ErrConflict, target)
	}

	parent := filepath.Dir(target)
	if err := s.FS.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("create parent directory %s: %w", parent, err)
	}
	tmp := filepath.Join(parent, "."+filepath.Base(target)+".dots-restore-"+randHex())
	if ok, err := fs.Exists(s.FS, tmp); err != nil || ok {
		return "", fmt.Errorf("temporary restore path %s is not usable (%v)", tmp, err)
	}
	if err := s.extract(plan, tmp); err != nil {
		_ = s.FS.RemoveAll(tmp)
		return "", err
	}

	if !exists {
		if err := s.FS.Rename(tmp, target); err != nil {
			_ = s.FS.RemoveAll(tmp)
			return "", fmt.Errorf("move restored content into place: %w", err)
		}
		return "", nil
	}
	archive, err := s.Replace(target, func() error { return s.FS.Rename(tmp, target) })
	var cw *CleanupWarning
	if err != nil && !errors.As(err, &cw) {
		// Either nothing was placed or the original was restored; in both
		// cases the temporary tree is still ours to delete.
		_ = s.FS.RemoveAll(tmp)
	}
	return archive, err
}

// extract writes the archive payload to tmp and applies modes and times.
// Directories are created owner-writable first and receive their final
// permissions and mtimes after all children exist.
func (s *Store) extract(plan *RestorePlan, tmp string) error {
	var fixups []dirFixup
	onManifest := func(m Manifest) error {
		if m != plan.Manifest {
			return fmt.Errorf("%w: archive %s was modified", ErrStateChanged, plan.ArchivePath)
		}
		return nil
	}
	handle := func(e Entry, r io.Reader) error {
		dest := tmp
		if e.Name != "" {
			dest = filepath.Join(tmp, filepath.FromSlash(e.Name))
		}
		switch e.Kind {
		case KindDir:
			if err := s.FS.MkdirAll(dest, 0o700); err != nil {
				return err
			}
			fixups = append(fixups, dirFixup{dest, e.Mode, e.ModTime})
		case KindSymlink:
			return s.FS.Symlink(e.Linkname, dest)
		case KindFile:
			f, err := s.FS.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				return err
			}
			_, cerr := io.Copy(f, io.LimitReader(r, e.Size))
			if err := f.Close(); cerr == nil {
				cerr = err
			}
			if cerr != nil {
				return cerr
			}
			if err := s.FS.Chmod(dest, e.Mode); err != nil {
				return err
			}
			return s.FS.Chtimes(dest, e.ModTime, e.ModTime)
		}
		return nil
	}
	if _, err := scan(s.FS, plan.ArchivePath, onManifest, handle); err != nil {
		return err
	}
	for i := len(fixups) - 1; i >= 0; i-- {
		d := fixups[i]
		if err := s.FS.Chmod(d.path, d.mode); err != nil {
			return err
		}
		if err := s.FS.Chtimes(d.path, d.mtime, d.mtime); err != nil {
			return err
		}
	}
	return nil
}
