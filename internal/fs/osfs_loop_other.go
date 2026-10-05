//go:build !windows

package fs

import (
	"errors"
	"syscall"
)

func isLinkLoop(err error) bool { return errors.Is(err, syscall.ELOOP) }
