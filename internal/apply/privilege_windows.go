//go:build windows

package apply

import (
	"errors"
	"syscall"
)

// errorPrivilegeNotHeld is ERROR_PRIVILEGE_NOT_HELD: the caller lacks
// SeCreateSymbolicLinkPrivilege.
const errorPrivilegeNotHeld syscall.Errno = 1314

func isPrivilegeError(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == errorPrivilegeNotHeld
}
