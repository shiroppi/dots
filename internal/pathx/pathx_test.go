package pathx

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckSyntax(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"empty", "", true},
		{"backslash", `a\b`, true},
		{"unc", "//server/share", true},
		{"abs", "/abs", true},
		{"drive upper slash", "C:/x", true},
		{"drive lower no slash", "c:x", true},
		{"dollar var", "$HOME/x", true},
		{"dollar brace", "${HOME}", true},
		{"dollar inside", "a/$b", true},
		{"percent var", "%USERPROFILE%/x", true},
		{"percent var inside", "a/%X%/b", true},
		{"control newline", "a\nb", true},
		{"control tab", "a\tb", true},
		{"control del", "a\x7fb", true},
		{"control nul", "a\x00b", true},
		{"valid relative", "config/vim", false},
		{"valid tilde", "~/.vimrc", false},
		{"valid space", "a b/c", false},
		{"valid unicode", "設定/ñandú/🙂", false},
		{"lone percent", "100%", false},
		{"percent pair across slash", "50%/x%", false},
		{"colon not drive", "ab:c", false},
		{"single letter colon is drive", "a:b", true},
		{"single letter", "a", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckSyntax(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("CheckSyntax(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
			}
		})
	}
}

func TestParseTarget(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Target
		wantErr bool
	}{
		{"tilde", "~", Target{Home: true}, false},
		{"tilde slash", "~/", Target{Home: true}, false},
		{"home nested", "~/.config/nvim", Target{Home: true, Rel: ".config/nvim"}, false},
		{"home cleaning", "~/./a/../b", Target{Home: true, Rel: "b"}, false},
		{"home double slash", "~/a//b", Target{Home: true, Rel: "a/b"}, false},
		{"home dotdot", "~/..", Target{}, true},
		{"home escape", "~/a/../../x", Target{}, true},
		{"tilde user", "~user/x", Target{}, true},
		{"tilde user only", "~user", Target{}, true},
		{"repo rel", "a/b", Target{Rel: "a/b"}, false},
		{"repo dot prefix", "./a", Target{Rel: "a"}, false},
		{"repo base via dotdot", "a/..", Target{}, false},
		{"repo dot", ".", Target{}, false},
		{"repo escape", "../x", Target{}, true},
		{"repo escape via clean", "a/../../x", Target{}, true},
		{"syntax error propagates", "/abs", Target{}, true},
		{"empty", "", Target{}, true},
		{"tilde in middle is literal", "a/~", Target{Rel: "a/~"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTarget(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseTarget(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Fatalf("ParseTarget(%q) = %+v want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestTargetIsBaseAndBaseName(t *testing.T) {
	tests := []struct {
		t        Target
		isBase   bool
		baseName string
	}{
		{Target{Home: true}, true, "the home directory"},
		{Target{Home: true, Rel: "a"}, false, "the home directory"},
		{Target{}, true, "the repository root"},
		{Target{Rel: "a/b"}, false, "the repository root"},
	}
	for _, tc := range tests {
		if got := tc.t.IsBase(); got != tc.isBase {
			t.Errorf("%+v IsBase=%v want %v", tc.t, got, tc.isBase)
		}
		if got := tc.t.BaseName(); got != tc.baseName {
			t.Errorf("%+v BaseName=%q want %q", tc.t, got, tc.baseName)
		}
	}
}

// absDir returns an absolute directory path valid on the host OS.
func absDir(parts ...string) string {
	base := "/"
	if runtime.GOOS == "windows" {
		base = `C:\`
	}
	return filepath.Join(append([]string{base}, parts...)...)
}

func TestTargetResolve(t *testing.T) {
	root, home := absDir("repo"), absDir("home")
	tests := []struct {
		name    string
		target  Target
		home    string
		want    string
		wantErr bool
	}{
		{"home base", Target{Home: true}, home, home, false},
		{"home nested", Target{Home: true, Rel: ".config/nvim"}, home, filepath.Join(home, ".config", "nvim"), false},
		{"repo base", Target{}, home, root, false},
		{"repo nested", Target{Rel: "a/b"}, home, filepath.Join(root, "a", "b"), false},
		{"repo ignores empty home", Target{Rel: "a"}, "", filepath.Join(root, "a"), false},
		{"repo ignores relative home", Target{Rel: "a"}, "rel", filepath.Join(root, "a"), false},
		{"home empty", Target{Home: true, Rel: "a"}, "", "", true},
		{"home relative", Target{Home: true, Rel: "a"}, "rel/home", "", true},
		{"home base empty", Target{Home: true}, "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.target.Resolve(root, tc.home)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestParseSource(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"simple", "config/vim", "config/vim", false},
		{"cleaning", "./a//b/../c", "a/c", false},
		{"tilde", "~", "", true},
		{"tilde slash", "~/x", "", true},
		{"tilde user", "~user", "", true},
		{"dot", ".", "", true},
		{"root via dotdot", "a/..", "", true},
		{"dotdot", "..", "", true},
		{"escape", "../x", "", true},
		{"abs", "/x", "", true},
		{"empty", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSource(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseSource(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestResolveSource(t *testing.T) {
	root := absDir("repo")
	if got, want := ResolveSource(root, "a/b/c"), filepath.Join(root, "a", "b", "c"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got, want := ResolveSource(root, "x"), filepath.Join(root, "x"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFoldsCase(t *testing.T) {
	for goos, want := range map[string]bool{
		"windows": true, "darwin": true, "linux": false, "freebsd": false, "openbsd": false, "": false,
	} {
		if got := FoldsCase(goos); got != want {
			t.Errorf("FoldsCase(%q)=%v want %v", goos, got, want)
		}
	}
}

func TestKey(t *testing.T) {
	tests := []struct {
		name string
		goos string
		in   string
		want string
	}{
		{"windows lowers", "windows", filepath.Join("A", "Bc"), filepath.Join("a", "bc")},
		{"darwin lowers", "darwin", filepath.Join("A", "Bc"), filepath.Join("a", "bc")},
		{"linux keeps", "linux", filepath.Join("A", "Bc"), filepath.Join("A", "Bc")},
		{"freebsd keeps", "freebsd", filepath.Join("A", "Bc"), filepath.Join("A", "Bc")},
		{"openbsd keeps", "openbsd", filepath.Join("A", "Bc"), filepath.Join("A", "Bc")},
		{"cleans", "linux", filepath.FromSlash("a/./b/../c/"), filepath.Join("a", "c")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Key(tc.goos, tc.in); got != tc.want {
				t.Fatalf("Key(%q,%q)=%q want %q", tc.goos, tc.in, got, tc.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	ab := absDir("a", "b")
	tests := []struct {
		name          string
		goos          string
		parent, child string
		want          bool
	}{
		{"strict child", "linux", ab, filepath.Join(ab, "c"), true},
		{"deep child", "linux", ab, filepath.Join(ab, "c", "d"), true},
		{"equal", "linux", ab, ab, false},
		{"equal unclean", "linux", ab, ab + string(filepath.Separator), false},
		{"sibling common prefix", "linux", ab, ab + "c", false},
		{"child is not parent", "linux", filepath.Join(ab, "c"), ab, false},
		{"unrelated", "linux", ab, absDir("x"), false},
		{"case differs linux", "linux", ab, strings.ToUpper(filepath.Join(ab, "c")), false},
		{"case differs windows", "windows", ab, strings.ToUpper(filepath.Join(ab, "c")), true},
		{"case differs darwin", "darwin", ab, strings.ToUpper(filepath.Join(ab, "c")), true},
		{"case equal windows", "windows", ab, strings.ToUpper(ab), false},
		{"dotdot cleaned", "linux", ab, filepath.Join(ab, "c", "..", "d"), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Contains(tc.goos, tc.parent, tc.child); got != tc.want {
				t.Fatalf("Contains(%q,%q,%q)=%v want %v", tc.goos, tc.parent, tc.child, got, tc.want)
			}
		})
	}
}

func TestOverlaps(t *testing.T) {
	ab := absDir("a", "b")
	abc := filepath.Join(ab, "c")
	tests := []struct {
		name string
		goos string
		a, b string
		want bool
	}{
		{"equal", "linux", ab, ab, true},
		{"a contains b", "linux", ab, abc, true},
		{"b contains a", "linux", abc, ab, true},
		{"siblings", "linux", ab, ab + "c", false},
		{"disjoint", "linux", ab, absDir("x"), false},
		{"equal differing case windows", "windows", ab, strings.ToUpper(ab), true},
		{"equal differing case linux", "linux", ab, strings.ToUpper(ab), false},
		{"nested differing case darwin", "darwin", strings.ToUpper(ab), abc, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Overlaps(tc.goos, tc.a, tc.b); got != tc.want {
				t.Fatalf("Overlaps(%q,%q,%q)=%v want %v", tc.goos, tc.a, tc.b, got, tc.want)
			}
		})
	}
}
