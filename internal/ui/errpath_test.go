package ui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewritePaths(t *testing.T) {
	sep := string(filepath.Separator)
	home := filepath.Join("home", "dev")
	root := filepath.Join(home, "dev", "repo")

	tests := []struct {
		name string
		s    string
		home string
		root string
		goos string
		want string
	}{
		{"home to tilde", "in " + home + sep + "file", home, root, "linux", "in ~" + sep + "file"},
		{"root relative", "in " + root + sep + "src", home, root, "linux", "in src"},
		{"root preferred over home", "in " + root + sep + "src", home, root, "linux", "in src"},
		{"boundary check left", "in /no" + home + sep + "file", home, root, "linux", "in /no" + home + sep + "file"},
		{"boundary check right", "in " + home + "eloper" + sep + "file", home, root, "linux", "in " + home + "eloper" + sep + "file"},
		{"path equal to home", "at " + home, home, root, "linux", "at ~"},
		{"multiple paths", home + sep + "a and " + root + sep + "b", home, root, "linux", "~" + sep + "a and b"},
		{"case insensitive windows", "in " + strings.ToUpper(home) + sep + "a", home, root, "windows", "in ~" + sep + "a"},
		{"case insensitive darwin", "in " + strings.ToUpper(home) + sep + "a", home, root, "darwin", "in ~" + sep + "a"},
		{"case sensitive linux", "in " + strings.ToUpper(home) + sep + "a", home, root, "linux", "in " + strings.ToUpper(home) + sep + "a"},
		{"empty home and root no change", "in " + home + sep + "a", "", "", "linux", "in " + home + sep + "a"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rewritePaths(tc.s, tc.home, tc.root, tc.goos)
			if got != tc.want {
				t.Errorf("rewritePaths(%q) = %q, want %q", tc.s, got, tc.want)
			}
		})
	}
}

func TestWrapError(t *testing.T) {
	sep := string(filepath.Separator)
	pr := &Printer{Home: filepath.Join("home", "dev"), Root: filepath.Join("home", "dev", "repo")}

	if pr.WrapError(nil) != nil {
		t.Error("expected nil to stay nil")
	}

	orig := errors.New("err at " + filepath.Join("home", "dev", "file"))
	wrapped := pr.WrapError(orig)
	wantStr := "err at ~" + sep + "file"
	if wrapped.Error() != wantStr {
		t.Errorf("expected %q, got %q", wantStr, wrapped.Error())
	}
	if !errors.Is(wrapped, orig) {
		t.Error("errors.Is not preserved")
	}

	// Test errors.Join
	err1 := errors.New("err1 at " + filepath.Join("home", "dev", "repo", "a"))
	err2 := errors.New("err2 at " + filepath.Join("home", "dev", "b"))
	joined := errors.Join(err1, err2)

	wrappedJoin := pr.WrapError(joined)
	if !errors.Is(wrappedJoin, err1) || !errors.Is(wrappedJoin, err2) {
		t.Error("errors.Join does not preserve Is/As")
	}

	u, ok := wrappedJoin.(interface{ Unwrap() []error })
	if !ok {
		t.Fatal("expected wrapped join to implement Unwrap() []error")
	}
	kids := u.Unwrap()
	if len(kids) != 2 {
		t.Fatalf("expected 2 kids, got %d", len(kids))
	}
	if kids[0].Error() != "err1 at a" {
		t.Errorf("kid 0 error: %q", kids[0].Error())
	}
	if kids[1].Error() != "err2 at ~"+sep+"b" {
		t.Errorf("kid 1 error: %q", kids[1].Error())
	}
}

func TestDisplayText(t *testing.T) {
	sep := string(filepath.Separator)
	pr := &Printer{Home: filepath.Join("home", "dev")}
	got := pr.DisplayText("at " + filepath.Join("home", "dev", "x"))
	want := "at ~" + sep + "x"
	if got != want {
		t.Errorf("DisplayText() = %q, want %q", got, want)
	}
}
