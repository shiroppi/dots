//go:build windows

package apply

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"testing"
)

func TestPrivilegeErrorWindows(t *testing.T) {
	m := newFS(t)
	fm := failFS{Manager: m, fail: map[string]error{
		ent("a").Target: fmt.Errorf("symlink: %w", &wrapped{syscall.Errno(1314)}),
	}}
	rep, err := Execute(fm, BuildPlan(m, resolution("a")), Options{})
	var perr *PrivilegeError
	if !errors.As(err, &perr) || rep == nil {
		t.Fatalf("want PrivilegeError, got %v", err)
	}
	if !strings.Contains(err.Error(), "Developer Mode") || !strings.Contains(err.Error(), "SeCreateSymbolicLinkPrivilege") {
		t.Errorf("guidance missing: %v", err)
	}
	if isPrivilegeError(errors.New("other")) {
		t.Error("false positive")
	}
}

type wrapped struct{ e error }

func (w *wrapped) Error() string { return "link error: " + w.e.Error() }
func (w *wrapped) Unwrap() error { return w.e }
