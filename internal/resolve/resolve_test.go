package resolve

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shiroppi/dots/internal/config"
	"github.com/shiroppi/dots/internal/fs"
	"github.com/shiroppi/dots/internal/model"
	"github.com/shiroppi/dots/internal/platform"
)

func root() string {
	if runtime.GOOS == "windows" {
		return `C:\repo`
	}
	return "/repo"
}

func p(parts ...string) string { return filepath.Join(append([]string{root()}, parts...)...) }
func h(parts ...string) string { return p(append([]string{"home"}, parts...)...) }

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type tree struct {
	dirs  []string // slash paths relative to repo root
	files []string
	links map[string]string // slash path -> relative link string
}

func build(t *testing.T, tr tree) fs.Manager {
	t.Helper()
	m := fs.NewMem()
	must(t, m.MkdirAll(p("home"), 0o755))
	for _, d := range tr.dirs {
		must(t, m.MkdirAll(p(filepath.FromSlash(d)), 0o755))
	}
	for _, f := range tr.files {
		full := p(filepath.FromSlash(f))
		must(t, m.MkdirAll(filepath.Dir(full), 0o755))
		must(t, fs.WriteFile(m, full, []byte("x"), 0o644))
	}
	for l, dst := range tr.links {
		full := p(filepath.FromSlash(l))
		must(t, m.MkdirAll(filepath.Dir(full), 0o755))
		must(t, m.Symlink(dst, full))
	}
	return m
}

func run(t *testing.T, m fs.Manager, goos, toml string) (*model.Resolution, error) {
	t.Helper()
	cfg, err := config.Parse([]byte(toml))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	env := platform.Env{GOOS: goos, Home: p("home"), Cwd: root()}
	return Resolve(m, env, p("dots.toml"), cfg)
}

// summary renders entries as "target<-source[rule]" with paths relative to the repo root.
func summary(res *model.Resolution) []string {
	rel := func(s string) string {
		r, _ := filepath.Rel(root(), s)
		return filepath.ToSlash(r)
	}
	var out []string
	for _, e := range res.Entries {
		out = append(out, fmt.Sprintf("%s<-%s[%s]", rel(e.Target), rel(e.Source), e.Origin.Rule))
	}
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestFlat(t *testing.T) {
	m := build(t, tree{files: []string{"vimrc", "a/b"}})
	res, err := run(t, m, "linux", `
[dots]
"~/.vimrc" = "vimrc"
"out/link" = "a/b"

[dots.linux]
"~/.vimrc" = "a/b"
"~/.extra" = "vimrc"

[dots.darwin]
"~/.mac" = "vimrc"
`)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.extra<-vimrc[dots.linux."~/.extra"]`,
		`home/.vimrc<-a/b[dots.linux."~/.vimrc"]`,
		`out/link<-a/b[dots."out/link"]`,
	})
	if res.ConfigPath != p("dots.toml") || res.Root != root() {
		t.Errorf("paths: %+v", res)
	}
	if len(res.Overrides) != 1 {
		t.Fatalf("overrides: %+v", res.Overrides)
	}
	o := res.Overrides[0]
	if o.Target != h(".vimrc") || o.Winner.Layer != model.LayerDotsOS || o.Loser.Origin.Layer != model.LayerDots || o.Loser.Source != p("vimrc") {
		t.Errorf("override: %+v", o)
	}
}

func TestFlatDuplicateSpellings(t *testing.T) {
	m := build(t, tree{files: []string{"a", "b"}})
	_, err := run(t, m, "linux", "[dots]\n\"~/.vim\" = \"a\"\n\"~/./.vim\" = \"b\"\n")
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	for _, w := range []string{"duplicate target " + h(".vim"), `dots."~/.vim"`, `dots."~/./.vim"`, "same priority", "dots"} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("%q lacks %q", err, w)
		}
	}
	// Same layer in dots.<os>.
	_, err = run(t, m, "linux", "[dots.linux]\n\"~/.vim\" = \"a\"\n\"~/./.vim\" = \"b\"\n")
	if err == nil || !strings.Contains(err.Error(), "dots.linux") {
		t.Fatalf("dots.linux dup: %v", err)
	}
}

func TestCaseFolding(t *testing.T) {
	m := build(t, tree{files: []string{"a", "b"}})
	toml := "[dots]\n\"~/.Vimrc\" = \"a\"\n\"~/.vimrc\" = \"b\"\n"
	for goos, wantErr := range map[string]bool{"windows": true, "darwin": true, "linux": false, "freebsd": false} {
		_, err := run(t, m, goos, toml)
		if (err != nil) != wantErr {
			t.Errorf("%s: err=%v wantErr=%v", goos, err, wantErr)
		}
	}
}

func TestPriority(t *testing.T) {
	m := build(t, tree{
		files: []string{"dotsf", "dotsos", "auto/common/keep", "auto/common/x", "autoos/x", "autoos/only"},
	})
	res, err := run(t, m, "linux", `
[dots]
"~/.cfg/common/x" = "dotsf"

[dots.linux]
"~/.cfg/only" = "dotsos"

[[auto]]
source = "auto/common"
target = "~/.cfg/common"

[[auto.linux]]
source = "autoos"
target = "~/.cfg/common"
`)
	must(t, err)
	// auto.linux/x is beaten by dots; auto/x is beaten by dots too; keep stays; only comes from auto.linux.
	eq(t, summary(res), []string{
		`home/.cfg/common/keep<-auto/common/keep[auto[0]]`,
		`home/.cfg/common/only<-autoos/only[auto[0].linux[0]]`,
		`home/.cfg/common/x<-dotsf[dots."~/.cfg/common/x"]`,
		`home/.cfg/only<-dotsos[dots.linux."~/.cfg/only"]`,
	})
	if len(res.Overrides) != 2 {
		t.Fatalf("overrides: %+v", res.Overrides)
	}
	if res.Overrides[0].Loser.Origin.Rule != "auto[0]" || res.Overrides[1].Loser.Origin.Rule != "auto[0].linux[0]" {
		t.Errorf("override order: %+v", res.Overrides)
	}
	for _, o := range res.Overrides {
		if o.Winner.Layer != model.LayerDots || o.Target != h(".cfg", "common", "x") {
			t.Errorf("override: %+v", o)
		}
	}
}

func TestAutoOSReplacesOnlyCollidingFile(t *testing.T) {
	m := build(t, tree{files: []string{"c/a", "c/b", "c/sub/d", "o/b", "o/sub/d", "o/new"}})
	res, err := run(t, m, "darwin", `
[[auto]]
source = "c"
target = "~/.config"

[[auto.darwin]]
source = "o"
target = "~/.config"
`)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/a<-c/a[auto[0]]`,
		`home/.config/b<-o/b[auto[0].darwin[0]]`,
		`home/.config/new<-o/new[auto[0].darwin[0]]`,
		`home/.config/sub/d<-o/sub/d[auto[0].darwin[0]]`,
	})
	if len(res.Overrides) != 2 {
		t.Fatalf("overrides: %+v", res.Overrides)
	}
	// Other OS: only the common rule applies.
	res, err = run(t, m, "linux", `
[[auto]]
source = "c"
target = "~/.config"

[[auto.darwin]]
source = "o"
target = "~/.config"
`)
	must(t, err)
	if len(res.Entries) != 3 || len(res.Overrides) != 0 {
		t.Fatalf("linux: %v", summary(res))
	}
}

func TestAutoSameLayerDuplicate(t *testing.T) {
	m := build(t, tree{files: []string{"c1/a", "c2/a"}})
	_, err := run(t, m, "linux", `
[[auto]]
source = "c1"
target = "~/.config"

[[auto]]
source = "c2"
target = "~/.config"
`)
	if err == nil || !strings.Contains(err.Error(), "auto[0], auto[1]") || !strings.Contains(err.Error(), "duplicate target "+h(".config", "a")) {
		t.Fatalf("got %v", err)
	}
}

func TestAutoEmptyDirAndIgnore(t *testing.T) {
	m := build(t, tree{
		dirs: []string{"s/emptydir", "s/onlyignored", "s/skipme/deep", "r"},
		files: []string{
			"s/README.md", "s/top/README.md", "s/top/keep", "s/onlyignored/.DS_Store",
			"s/skipme/deep/f", "s/skipme/g",
		},
	})
	res, err := run(t, m, "linux", `
[[auto]]
source = "s"
target = "~/.cfg"
ignore = ["README.md", "**/.DS_Store", "skipme"]
`)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.cfg/emptydir<-s/emptydir[auto[0]]`,
		`home/.cfg/top/README.md<-s/top/README.md[auto[0]]`, // root-only pattern does not match nested
		`home/.cfg/top/keep<-s/top/keep[auto[0]]`,
	})

	// "**/README.md" matches at every depth including the root.
	res, err = run(t, m, "linux", `
[[auto]]
source = "s"
target = "~/.cfg"
ignore = ["**/README.md", "**/.DS_Store", "skipme"]
`)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.cfg/emptydir<-s/emptydir[auto[0]]`,
		`home/.cfg/top/keep<-s/top/keep[auto[0]]`,
	})

	// Physically empty source root and fully-ignored root produce nothing.
	res, err = run(t, m, "linux", "[[auto]]\nsource = \"r\"\ntarget = \"~/.cfg\"\n")
	must(t, err)
	if len(res.Entries) != 0 {
		t.Errorf("empty root: %v", summary(res))
	}
	res, err = run(t, m, "linux", "[[auto]]\nsource = \"s/onlyignored\"\ntarget = \"~/.cfg\"\nignore = [\".DS_Store\"]\n")
	must(t, err)
	if len(res.Entries) != 0 {
		t.Errorf("ignored root: %v", summary(res))
	}
}

func TestAutoTargetHome(t *testing.T) {
	m := build(t, tree{files: []string{"s/.zshrc"}})
	res, err := run(t, m, "linux", "[[auto]]\nsource = \"s\"\ntarget = \"~\"\n")
	must(t, err)
	eq(t, summary(res), []string{`home/.zshrc<-s/.zshrc[auto[0]]`})
}

func TestAutoSymlinks(t *testing.T) {
	m := build(t, tree{
		files: []string{"s/real"},
		links: map[string]string{
			"s/good":   "real",
			"s/broken": "nowhere",
			"s/loopa":  "loopb",
			"s/loopb":  "loopa",
		},
	})
	_, err := run(t, m, "linux", "[[auto]]\nsource = \"s\"\ntarget = \"~/.cfg\"\n")
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, w := range []string{"auto[0]", "broken symlink", filepath.Join("s", "broken"), "symlink loop", filepath.Join("s", "loopa"), filepath.Join("s", "loopb")} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("%q lacks %q", err, w)
		}
	}
	if strings.Contains(err.Error(), filepath.Join("s", "good")) {
		t.Errorf("good link reported: %v", err)
	}

	// Ignoring the bad links yields the link item itself, never its target.
	res, err := run(t, m, "linux", `
[[auto]]
source = "s"
target = "~/.cfg"
ignore = ["broken", "loopa", "loopb"]
`)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.cfg/good<-s/good[auto[0]]`,
		`home/.cfg/real<-s/real[auto[0]]`,
	})
}

func TestAutoSourceErrors(t *testing.T) {
	m := build(t, tree{files: []string{"file"}, dirs: []string{"d"}, links: map[string]string{"dlink": "d"}})
	tests := []struct{ name, toml, want string }{
		{"missing", "[[auto]]\nsource = \"nope\"\ntarget = \"~/x\"\n", "does not exist"},
		{"file", "[[auto]]\nsource = \"file\"\ntarget = \"~/x\"\n", "must be a directory"},
		{"symlink to dir", "[[auto]]\nsource = \"dlink\"\ntarget = \"~/x\"\n", "must be a directory"},
		{"os rule missing", "[[auto]]\n[[auto.linux]]\nsource = \"nope\"\ntarget = \"~/x\"\n", "auto[0].linux[0]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := run(t, m, "linux", tt.toml)
			if err == nil || res != nil {
				t.Fatalf("expected error, got %v", res)
			}
			if !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "auto[0]") {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestParentChildConflict(t *testing.T) {
	m := build(t, tree{files: []string{"auto/x"}, dirs: []string{"config/vim"}})
	_, err := run(t, m, "linux", `
[dots]
"~/.vim" = "config/vim"

[[auto]]
source = "auto"
target = "~/.vim"
`)
	if err == nil {
		t.Fatal("expected conflict")
	}
	want := fmt.Sprintf("target %s (auto[0]) is inside target %s (dots.%q)", h(".vim", "x"), h(".vim"), "~/.vim")
	if !strings.Contains(err.Error(), want) {
		t.Errorf("got %v\nwant substring %q", err, want)
	}
	if strings.Count(err.Error(), "is inside target") != 1 {
		t.Errorf("pair reported more than once: %v", err)
	}
	// An overridden loser does not conflict: the winner replaces the file.
	m2 := build(t, tree{files: []string{"auto/x", "f"}, dirs: []string{"config/vim"}})
	if _, err = run(t, m2, "linux", `
[dots]
"~/.vim/x" = "f"

[[auto]]
source = "auto"
target = "~/.vim"
`); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestOtherOSIgnored(t *testing.T) {
	m := build(t, tree{files: []string{"a", "o/f"}})
	res, err := run(t, m, "linux", `
[dots]
"~/.a" = "a"

[dots.darwin]
"~/.a" = "a"
"~/.bad" = "missing"

[dots.windows]
"~/.w" = "a"

[[auto]]

[[auto.windows]]
source = "does-not-exist"
target = "~/x"
`)
	must(t, err)
	eq(t, summary(res), []string{`home/.a<-a[dots."~/.a"]`})
	if len(res.Overrides) != 0 {
		t.Errorf("overrides: %+v", res.Overrides)
	}
}

func TestBaseTarget(t *testing.T) {
	m := build(t, tree{files: []string{"a"}})
	_, err := run(t, m, "linux", "[dots]\n\"~\" = \"a\"\n\".\" = \"a\"\n")
	if err == nil || strings.Count(err.Error(), "cannot be a symlink") != 2 || !strings.Contains(err.Error(), `dots."~"`) || !strings.Contains(err.Error(), `dots."."`) {
		t.Fatalf("got %v", err)
	}
}

func TestMultipleErrorsJoined(t *testing.T) {
	m := build(t, tree{files: []string{"a", "file"}})
	res, err := run(t, m, "linux", `
[dots]
"~" = "a"
"~/.x" = "a"
"~/./.x" = "a"

[[auto]]
source = "nope"
target = "~/n"

[[auto]]
source = "file"
target = "~/f"
`)
	if err == nil || res != nil {
		t.Fatal("expected error")
	}
	for _, w := range []string{"cannot be a symlink", "duplicate target", "does not exist", "must be a directory"} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("%q lacks %q", err, w)
		}
	}
}

func TestDeterministicOrdering(t *testing.T) {
	m := build(t, tree{files: []string{"a", "c1/z", "c1/b", "c2/z", "c2/b"}})
	toml := `
[dots]
"~/.z" = "a"
"~/.a" = "a"
"~/.m" = "a"

[[auto]]
source = "c1"
target = "~/.cfg"

[[auto]]
source = "c2"
target = "~/.cfg"
`
	// c1 and c2 collide at the same layer -> error; make c2 an OS rule instead.
	toml = strings.Replace(toml, "[[auto]]\nsource = \"c2\"", "[[auto.linux]]\nsource = \"c2\"", 1)
	var first []string
	for i := 0; i < 10; i++ {
		res, err := run(t, m, "linux", toml)
		must(t, err)
		got := summary(res)
		for _, o := range res.Overrides {
			got = append(got, "ov:"+o.Target+":"+o.Loser.Origin.Rule)
		}
		if i == 0 {
			first = got
			for j := 1; j < len(res.Entries); j++ {
				if res.Entries[j-1].Target >= res.Entries[j].Target {
					t.Errorf("entries unsorted: %v", got)
				}
			}
			for j := 1; j < len(res.Overrides); j++ {
				if res.Overrides[j-1].Target > res.Overrides[j].Target {
					t.Errorf("overrides unsorted: %v", got)
				}
			}
			continue
		}
		eq(t, got, first)
	}
}
