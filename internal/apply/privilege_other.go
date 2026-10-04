//go:build !windows

package apply

// isPrivilegeError is only meaningful on Windows, where creating symlinks
// requires a privilege.
func isPrivilegeError(error) bool { return false }
