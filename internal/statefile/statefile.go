// Package statefile reads and writes the state file that records the symlinks
// dots created (v0.2 spec §5). It is unrelated to package state, which
// evaluates the current status of link targets.
//
// The file is UTF-8 JSON:
//
//	{"version": 1, "links": [{"target": ..., "link": ..., "repository": ...,
//	 "source": ..., "recorded_at": ...}]}
//
// Unknown keys are ignored on load so later versions can add keys (for
// example "copies") without bumping the version.
//
// Permissions: the directory is created with mode 0700 and the file with
// mode 0600. On Windows these bits are only best-effort; the default location
// lives under %LOCALAPPDATA%, which is private to the current user.
package statefile

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/pathx"
	"github.com/shiroppi/dots/internal/platform"
)

// FileName is the base name of the state file.
const FileName = "state.json"

// Version is the only format version this package understands.
const Version = 1

// Sentinel errors. Returned errors wrap them so errors.Is works.
var (
	// ErrBroken reports a state file that is not valid JSON or lacks a
	// required key. Load returns it together with an empty State; callers
	// warn and treat the state as empty.
	ErrBroken = errors.New("state file is unreadable")
	// ErrNewerVersion reports a state file written by a newer dots. It must
	// not be rewritten; callers that write refuse before changing anything.
	ErrNewerVersion = errors.New("state file was written by a newer dots")
)

// DefaultPath returns the default state file path for env.
func DefaultPath(env platform.Env) (string, error) {
	dir, err := platform.AppDir(env, platform.StateDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FileName), nil
}

// Link records one symlink created by dots.
type Link struct {
	// Target is the absolute runtime path of the symlink.
	Target string `json:"target"`
	// Link is the link string as returned by Readlink.
	Link string `json:"link"`
	// Repository is the absolute repository root that created the link.
	Repository string `json:"repository"`
	// Source is the slash-separated source path relative to Repository
	// (display only).
	Source string `json:"source"`
	// RecordedAt is the time of recording (stored as RFC 3339 UTC).
	RecordedAt time.Time `json:"recorded_at"`
}

// State is the in-memory content of the state file. GOOS selects the target
// comparison rules (case-insensitive on windows and darwin); an empty GOOS
// compares exactly.
type State struct {
	GOOS  string
	Links []Link
}

type fileDoc struct {
	Version int    `json:"version"`
	Links   []Link `json:"links"`
}

func (s *State) index(target string) int {
	k := pathx.Key(s.GOOS, target)
	for i, l := range s.Links {
		if pathx.Key(s.GOOS, l.Target) == k {
			return i
		}
	}
	return -1
}

// Find returns the record of target.
func (s *State) Find(target string) (Link, bool) {
	if i := s.index(target); i >= 0 {
		return s.Links[i], true
	}
	return Link{}, false
}

// Record stores l, replacing an existing record of the same target.
func (s *State) Record(l Link) {
	l.RecordedAt = l.RecordedAt.UTC().Truncate(time.Second)
	if i := s.index(l.Target); i >= 0 {
		s.Links[i] = l
		return
	}
	s.Links = append(s.Links, l)
}

// Remove deletes the record of target and reports whether there was one.
func (s *State) Remove(target string) bool {
	i := s.index(target)
	if i < 0 {
		return false
	}
	s.Links = append(s.Links[:i], s.Links[i+1:]...)
	return true
}

// PruneMissing deletes the records whose target no longer exists (Lstat) and
// returns the removed records. Any failure other than "does not exist" is
// returned and leaves the record in place.
func (s *State) PruneMissing(fsys fs.Manager) ([]Link, error) {
	var removed []Link
	var errs []error
	kept := s.Links[:0:0]
	for _, l := range s.Links {
		_, err := fsys.Lstat(l.Target)
		switch {
		case err == nil:
			kept = append(kept, l)
		case isGone(err):
			removed = append(removed, l)
		default:
			kept = append(kept, l)
			errs = append(errs, fmt.Errorf("cannot inspect %s: %w", l.Target, err))
		}
	}
	s.Links = kept
	return removed, errors.Join(errs...)
}

func (s *State) sort() {
	sort.SliceStable(s.Links, func(i, j int) bool {
		a, b := pathx.Key(s.GOOS, s.Links[i].Target), pathx.Key(s.GOOS, s.Links[j].Target)
		if a != b {
			return a < b
		}
		return s.Links[i].Target < s.Links[j].Target
	})
}

// Store reads and writes one state file.
type Store struct {
	FS   fs.Manager
	Path string
	// GOOS selects the target comparison rules; see State.GOOS.
	GOOS string
}

// Load reads the state file. A missing file yields an empty State and no
// error. A broken file yields an empty State and an error wrapping
// ErrBroken; a file with a version above Version yields an empty State and an
// error wrapping ErrNewerVersion. Other errors are I/O failures.
func (s *Store) Load() (*State, error) {
	st := &State{GOOS: s.GOOS}
	data, err := fs.ReadFile(s.FS, s.Path)
	if err != nil {
		if isGone(err) {
			return st, nil
		}
		return st, fmt.Errorf("cannot read state file %s: %w", s.Path, err)
	}
	links, err := parse(data)
	if err != nil {
		return st, fmt.Errorf("%s: %w", s.Path, err)
	}
	st.Links = links
	st.sort()
	return st, nil
}

// parse validates data and returns its link records.
func parse(data []byte) ([]Link, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil || top == nil {
		if err == nil {
			err = errors.New("not a JSON object")
		}
		return nil, fmt.Errorf("%w: %v", ErrBroken, err)
	}
	rawVer, ok := top["version"]
	if !ok {
		return nil, fmt.Errorf("%w: missing \"version\"", ErrBroken)
	}
	var ver int
	if err := json.Unmarshal(rawVer, &ver); err != nil || ver < 1 {
		return nil, fmt.Errorf("%w: invalid \"version\"", ErrBroken)
	}
	if ver > Version {
		return nil, fmt.Errorf("%w (version %d, this dots understands up to %d)", ErrNewerVersion, ver, Version)
	}
	rawLinks, ok := top["links"]
	if !ok {
		return nil, fmt.Errorf("%w: missing \"links\"", ErrBroken)
	}
	var recs []map[string]json.RawMessage
	if err := json.Unmarshal(rawLinks, &recs); err != nil || recs == nil {
		return nil, fmt.Errorf("%w: \"links\" is not an array", ErrBroken)
	}
	links := make([]Link, 0, len(recs))
	for i, rec := range recs {
		for _, key := range []string{"target", "link", "repository", "source", "recorded_at"} {
			if _, ok := rec[key]; !ok {
				return nil, fmt.Errorf("%w: links[%d] lacks %q", ErrBroken, i, key)
			}
		}
		raw, err := json.Marshal(rec)
		if err != nil {
			return nil, fmt.Errorf("%w: links[%d]: %v", ErrBroken, i, err)
		}
		var l Link
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("%w: links[%d]: %v", ErrBroken, i, err)
		}
		if l.Target == "" || l.Link == "" || l.Repository == "" {
			return nil, fmt.Errorf("%w: links[%d] has an empty value", ErrBroken, i)
		}
		links = append(links, l)
	}
	return links, nil
}

// Save writes st: links sorted by target, one record per target. The data
// goes to a temporary file in the same directory first and is renamed into
// place afterwards. The directory is created with mode 0700 and the file with
// mode 0600.
func (s *Store) Save(st *State) error {
	out := &State{GOOS: s.GOOS}
	for _, l := range st.Links {
		out.Record(l)
	}
	out.sort()
	doc := fileDoc{Version: Version, Links: out.Links}
	if doc.Links == nil {
		doc.Links = []Link{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("cannot encode state file: %w", err)
	}

	dir := filepath.Dir(s.Path)
	if err := s.FS.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	if err := s.FS.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("restrict state directory permissions: %w", err)
	}
	tmp := filepath.Join(dir, ".tmp-"+randHex()+".partial")
	f, err := s.FS.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	_, werr := f.Write(buf.Bytes())
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = s.FS.RemoveAll(tmp)
		return fmt.Errorf("write state file: %w", werr)
	}
	_ = s.FS.Chmod(tmp, 0o600) // best effort on platforms without mode bits

	// fs.Manager.Rename refuses an existing destination, so the previous
	// file is removed first.
	err = s.FS.Rename(tmp, s.Path)
	if errors.Is(err, fs.ErrExist) {
		if rerr := s.FS.Remove(s.Path); rerr != nil && !isGone(rerr) {
			_ = s.FS.RemoveAll(tmp)
			return fmt.Errorf("replace state file: %w", rerr)
		}
		err = s.FS.Rename(tmp, s.Path)
	}
	if err != nil {
		_ = s.FS.RemoveAll(tmp)
		return fmt.Errorf("replace state file: %w", err)
	}
	return nil
}

// MoveAside renames the state file to <path>.broken-<UTC yyyymmddTHHMMSSZ>
// (with a numeric suffix when that name is taken) and returns the new path.
func (s *Store) MoveAside(now time.Time) (string, error) {
	base := s.Path + ".broken-" + now.UTC().Format("20060102T150405Z")
	for i := 1; i <= 1000; i++ {
		dest := base
		if i > 1 {
			dest = fmt.Sprintf("%s-%d", base, i)
		}
		err := s.FS.Rename(s.Path, dest)
		if err == nil {
			return dest, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("move state file aside: %w", err)
		}
	}
	return "", fmt.Errorf("no free name for the broken state file beside %s", s.Path)
}

func randHex() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%012x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
