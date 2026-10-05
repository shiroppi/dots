//go:build windows

package fs

import (
	"errors"
	"syscall"
)

// errCantResolveFilename is ERROR_CANT_RESOLVE_FILENAME, which Windows
// reports for cyclic symbolic links.
const errCantResolveFilename = syscall.Errno(1921)

func isLinkLoop(err error) bool { return errors.Is(err, errCantResolveFilename) }
