package config

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shiroppi/dots/internal/fs"
)

func root() string {
	if runtime.GOOS == "windows" {
		return `C:\repo`
	}
	return "/repo"
}

func p(parts ...string) string { return filepath.Join(append([]string{root()}, parts...)...) }

func TestParseValid(t *testing.T) {
	src := `
[dots]
"~/.vimrc" = "vimrc"
"~/.vim" = "config/vim"
"out/x" = "./x"

[dots.darwin]
"~/.config/karabiner" = "config/darwin/karabiner"

[dots.linux]
"~/.zshrc" = "zshrc"

[[auto]]
source = "config/auto"
target = "~/.config"
ignore = ["README.md", ".DS_Store"]

[[auto.darwin]]
source = "config/darwin-auto"
target = "~/.config"

[[auto]]

[[auto.linux]]
source = "config/linux-only"
target = "~"
`
	cfg, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var raws []string
	for _, l := range cfg.Dots {
		raws = append(raws, l.RawTarget)
	}
	if strings.Join(raws, ",") != "out/x,~/.vim,~/.vimrc" {
		t.Errorf("dots order: %v", raws)
	}
	if cfg.Dots[0].Source != "x" || cfg.Dots[0].Target.Home {
		t.Errorf("bad link: %+v", cfg.Dots[0])
	}
	if len(cfg.DotsOS["darwin"]) != 1 || len(cfg.DotsOS["linux"]) != 1 {
		t.Errorf("dotsOS: %+v", cfg.DotsOS)
	}
	if len(cfg.Auto) != 2 {
		t.Fatalf("auto: %+v", cfg.Auto)
	}
	g0, g1 := cfg.Auto[0], cfg.Auto[1]
	if g0.Index != 0 || g0.Common == nil || g0.Common.Source != "config/auto" || !g0.Common.Target.Home {
		t.Errorf("g0: %+v", g0)
	}
	if !g0.Common.Ignore.Match("README.md") || !g0.Common.Ignore.Match(".DS_Store") || g0.Common.Ignore.Match("README.txt") {
		t.Error("ignore matcher wrong")
	}
	if len(g0.OS["darwin"]) != 1 || g0.OS["darwin"][0].Ignore == nil || g0.OS["darwin"][0].Ignore.Match("x") {
		t.Errorf("g0 darwin: %+v", g0.OS)
	}
	if g1.Index != 1 || g1.Common != nil || len(g1.OS["linux"]) != 1 || !g1.OS["linux"][0].Target.IsBase() {
		t.Errorf("g1: %+v", g1)
	}
}

func TestParseEmpty(t *testing.T) {
	for _, src := range []string{"", "# comment only\n", "[dots]\n"} {
		cfg, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if len(cfg.Dots)+len(cfg.Auto)+len(cfg.DotsOS) != 0 {
			t.Errorf("%q: not empty: %+v", src, cfg)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string // substrings that must all appear
	}{
		{"syntax", "[dots\n\"a\" = \"b\"\n", []string{"TOML syntax error at line 1"}},
		{"syntax line 3", "[dots]\n\"a\" = \"b\"\nfoo = = 1\n", []string{"line 3"}},
		{"unknown top-level", "foo = 1\n", []string{`unknown top-level key "foo"`}},
		{"dots not table", "dots = 3\n", []string{"dots: must be a table"}},
		{"auto single table", "[auto]\nsource = \"a\"\ntarget = \"~/b\"\n", []string{"auto:", "use [[auto]]"}},
		{"auto wrong type", "auto = 3\n", []string{"auto: must be an array of tables"}},
		{"auto.darwin single table", "[[auto]]\nsource = \"a\"\ntarget = \"~/b\"\n[auto.darwin]\nsource = \"c\"\ntarget = \"~/d\"\n", []string{"auto[0].darwin", "use [[auto.darwin]]"}},
		{"unknown os table", "[dots.bogus]\n\"~/a\" = \"b\"\n", []string{`dots.bogus: unknown OS name "bogus"`, "quote target paths"}},
		{"unquoted dotted key", "[dots]\n~/a.b = \"x\"\n", []string{"TOML syntax error"}},
		{"unquoted dotted key valid toml", "[dots]\na.b = \"x\"\n", []string{`dots.a: unknown OS name "a"`, "quote target paths"}},
		{"dots non-string", "[dots]\n\"~/a\" = 1\n", []string{`dots."~/a"`, "must be a string", "an integer"}},
		{"dots os non-string", "[dots.linux]\n\"~/a\" = [\"x\"]\n", []string{`dots.linux."~/a"`, "must be a string", "an array"}},
		{"dots os nested table", "[dots.linux.sub]\n\"~/a\" = \"x\"\n", []string{`dots.linux."sub"`, "must be a string"}},
		{"dots bad target", "[dots]\n\"/abs\" = \"x\"\n", []string{`dots."/abs"`, "invalid target", "absolute"}},
		{"dots bad source", "[dots]\n\"~/a\" = \"../x\"\n", []string{`dots."~/a"`, "invalid source", "escapes"}},
		{"auto element not table", "auto = [1]\n", []string{"auto[0]: must be a table"}},
		{"source without target", "[[auto]]\nsource = \"a\"\n", []string{"auto[0]", `missing required key "target"`}},
		{"target without source", "[[auto]]\ntarget = \"~/a\"\n", []string{"auto[0]", `missing required key "source"`}},
		{"source non-string", "[[auto]]\nsource = 1\ntarget = \"~/a\"\n", []string{"auto[0].source: must be a string"}},
		{"ignore without rule", "[[auto]]\nignore = [\"a\"]\n", []string{"auto[0]: ignore requires source and target"}},
		{"empty group", "[[auto]]\n", []string{"auto[0]: empty [[auto]] group"}},
		{"unknown group key", "[[auto]]\nsource = \"a\"\ntarget = \"~/a\"\nfoo = 1\n", []string{"auto[0]", `unknown key "foo"`}},
		{"ignore not array", "[[auto]]\nsource = \"a\"\ntarget = \"~/a\"\nignore = \"x\"\n", []string{"auto[0].ignore: must be an array of strings"}},
		{"ignore non-string element", "[[auto]]\nsource = \"a\"\ntarget = \"~/a\"\nignore = [\"x\", 2]\n", []string{"auto[0].ignore[1]: must be a string"}},
		{"ignore invalid pattern", "[[auto]]\nsource = \"a\"\ntarget = \"~/a\"\nignore = [\"/abs\"]\n", []string{"auto[0].", "ignore[0]", "/abs"}},
		{"ignore path pattern", "[[auto]]\nsource = \"a\"\ntarget = \"~/a\"\nignore = [\"nvim/README.md\"]\n", []string{"auto[0].", "ignore[0]", "single top-level name"}},
		{"ignore doublestar", "[[auto]]\nsource = \"a\"\ntarget = \"~/a\"\nignore = [\"**/.DS_Store\"]\n", []string{"auto[0].", "ignore[0]", "**/.DS_Store"}},
		{"ignore question mark", "[[auto]]\nsource = \"a\"\ntarget = \"~/a\"\nignore = [\"?.txt\"]\n", []string{"auto[0].", "ignore[0]", "is supported as a wildcard"}},
		{"ignore class", "[[auto]]\nsource = \"a\"\ntarget = \"~/a\"\nignore = [\"[ab]\"]\n", []string{"auto[0].", "ignore[0]", "is supported as a wildcard"}},
		{"os rule missing source", "[[auto]]\nsource = \"a\"\ntarget = \"~/a\"\n[[auto.linux]]\ntarget = \"~/b\"\n", []string{"auto[0].linux[0]", `missing required key "source"`}},
		{"os rule unknown key", "[[auto]]\n[[auto.linux]]\nsource = \"a\"\ntarget = \"~/b\"\nfoo = 1\n", []string{"auto[0].linux[0]", `unknown key "foo"`}},
		{"os rule bad target", "[[auto]]\n[[auto.linux]]\nsource = \"a\"\ntarget = \"C:/x\"\n", []string{"auto[0].linux[0].target", "drive"}},
		{"os rule bad source", "[[auto]]\n[[auto.linux]]\nsource = \"\"\ntarget = \"~/x\"\n", []string{"auto[0].linux[0].source", "empty"}},
		{"os key wrong type", "[[auto]]\nsource = \"a\"\ntarget = \"~/a\"\nlinux = 3\n", []string{"auto[0].linux: must be an array of tables"}},
		{"os element not table", "[[auto]]\nsource = \"a\"\ntarget = \"~/a\"\nlinux = [1]\n", []string{"auto[0].linux[0]: must be a table"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse([]byte(tt.src))
			if err == nil {
				t.Fatalf("expected error, got config %+v", cfg)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err.Error(), w)
				}
			}
		})
	}
}

func TestParseCollectsAllProblemsDeterministically(t *testing.T) {
	src := "zzz = 1\n[dots]\n\"/a\" = \"x\"\n\"~/b\" = 3\n[[auto]]\n"
	var first string
	for i := 0; i < 20; i++ {
		_, err := Parse([]byte(src))
		if err == nil {
			t.Fatal("expected error")
		}
		if i == 0 {
			first = err.Error()
			for _, w := range []string{`unknown top-level key "zzz"`, `dots."/a"`, `dots."~/b"`, "empty [[auto]] group"} {
				if !strings.Contains(first, w) {
					t.Errorf("missing %q in %q", w, first)
				}
			}
		} else if err.Error() != first {
			t.Fatalf("non-deterministic:\n%s\nvs\n%s", first, err.Error())
		}
	}
}

func TestLoad(t *testing.T) {
	m := fs.NewMem()
	if err := m.MkdirAll(root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile(m, p("dots.toml"), []byte("[dots]\n\"~/.a\" = \"a\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(m, p("dots.toml"))
	if err != nil || len(cfg.Dots) != 1 {
		t.Fatalf("%v %+v", err, cfg)
	}
	if err := fs.WriteFile(m, p("bad.toml"), []byte("foo = 1\nbar = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Load(m, p("bad.toml"))
	if err == nil || strings.Count(err.Error(), p("bad.toml")+": ") != 2 {
		t.Fatalf("want path prefix on every problem, got %v", err)
	}
	if _, err = Load(m, p("missing.toml")); err == nil || !strings.Contains(err.Error(), p("missing.toml")) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
}

func TestFind(t *testing.T) {
	m := fs.NewMem()
	if err := m.MkdirAll(p("a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile(m, p("dots.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile(m, p("a", "b", "dots.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Run("same dir", func(t *testing.T) {
		got, err := Find(m, root())
		if err != nil || got != p("dots.toml") {
			t.Fatalf("%q %v", got, err)
		}
	})
	t.Run("nested subdir picks nearest", func(t *testing.T) {
		got, err := Find(m, p("a", "b", "c"))
		if err != nil || got != p("a", "b", "dots.toml") {
			t.Fatalf("%q %v", got, err)
		}
	})
	t.Run("parent of nested", func(t *testing.T) {
		got, err := Find(m, p("a"))
		if err != nil || got != p("dots.toml") {
			t.Fatalf("%q %v", got, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		m2 := fs.NewMem()
		if err := m2.MkdirAll(p("x", "y"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := Find(m2, p("x", "y"))
		if !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), p("x", "y")) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("directory", func(t *testing.T) {
		m2 := fs.NewMem()
		if err := m2.MkdirAll(p("d", "dots.toml"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := Find(m2, p("d"))
		if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "is a directory") {
			t.Fatalf("got %v", err)
		}
	})
}
