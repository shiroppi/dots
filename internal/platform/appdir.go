package platform

import (
	"errors"
	"fmt"
	"path/filepath"
)

// Kind selects which per-user application directory AppDir resolves.
type Kind int

const (
	// DataDir holds user data such as backups (XDG_DATA_HOME on Unix).
	DataDir Kind = iota + 1
	// StateDir holds machine-local state such as the record of created
	// links (XDG_STATE_HOME on Unix).
	StateDir
)

// purpose names what the directory is for, for error messages.
func (k Kind) purpose() string {
	if k == StateDir {
		return "state"
	}
	return "backup"
}

// AppDir returns the dots directory of the given kind for env.GOOS:
//
//   - windows: %LOCALAPPDATA%\dots (LOCALAPPDATA must be absolute)
//   - darwin:  ~/Library/Application Support/dots
//   - others:  $XDG_DATA_HOME/dots (DataDir) or $XDG_STATE_HOME/dots
//     (StateDir) when set and non-empty (it must be absolute), otherwise
//     ~/.local/share/dots or ~/.local/state/dots
func AppDir(env Env, kind Kind) (string, error) {
	what := kind.purpose()
	switch env.GOOS {
	case "windows":
		v := env.Getenv("LOCALAPPDATA")
		if v == "" {
			return "", fmt.Errorf("LOCALAPPDATA is not set; cannot determine the %s directory", what)
		}
		if !filepath.IsAbs(v) {
			return "", fmt.Errorf("LOCALAPPDATA must be an absolute path, got %q", v)
		}
		return filepath.Join(v, "dots"), nil
	case "darwin":
		if env.Home == "" {
			return "", fmt.Errorf("home directory is unknown; cannot determine the %s directory", what)
		}
		return filepath.Join(env.Home, "Library", "Application Support", "dots"), nil
	default:
		var xdg, sub string
		switch kind {
		case DataDir:
			xdg, sub = "XDG_DATA_HOME", "share"
		case StateDir:
			xdg, sub = "XDG_STATE_HOME", "state"
		default:
			return "", errors.New("unknown application directory kind")
		}
		if v := env.Getenv(xdg); v != "" {
			if !filepath.IsAbs(v) {
				return "", fmt.Errorf("%s must be an absolute path, got %q", xdg, v)
			}
			return filepath.Join(v, "dots"), nil
		}
		if env.Home == "" {
			return "", fmt.Errorf("home directory is unknown; cannot determine the %s directory", what)
		}
		return filepath.Join(env.Home, ".local", sub, "dots"), nil
	}
}
