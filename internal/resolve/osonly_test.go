package resolve

import (
	"strings"
	"testing"
)

func TestOSOnlyReadmeExample(t *testing.T) {
	m := build(t, tree{
		files: []string{
			"config/starship.toml",
			"config/darwin/karabiner/karabiner.json",
		},
	})

	toml := `
[[auto]]
source = "config"
target = "~/.config"

[[auto.darwin]]
source = "config/darwin"
target = "~/.config"
os_only = true
`

	// For darwin
	res, err := run(t, m, "darwin", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/karabiner<-config/darwin/karabiner[auto[0].darwin[0]]`,
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})

	// For linux
	res, err = run(t, m, "linux", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
	res, err = run(t, m, "openbsd", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})

	// Without os_only
	tomlNoOSOnly := `
[[auto]]
source = "config"
target = "~/.config"

[[auto.darwin]]
source = "config/darwin"
target = "~/.config"
`
	res, err = run(t, m, "darwin", tomlNoOSOnly)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/darwin<-config/darwin[auto[0]]`,
		`home/.config/karabiner<-config/darwin/karabiner[auto[0].darwin[0]]`,
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
}

func TestOSOnlyInactiveOSExcludes(t *testing.T) {
	m := build(t, tree{
		files: []string{
			"config/starship.toml",
			"config/windows/win.ini",
		},
	})
	toml := `
[[auto]]
source = "config"
target = "~/.config"

[[auto.windows]]
source = "config/windows"
target = "~/.config"
os_only = true
`
	res, err := run(t, m, "linux", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
	res, err = run(t, m, "openbsd", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
}

func TestOSOnlyOutsideCommonSource(t *testing.T) {
	m := build(t, tree{
		files: []string{
			"config/starship.toml",
			"other/darwin/karabiner.json",
		},
	})
	toml := `
[[auto]]
source = "config"
target = "~/.config"

[[auto.darwin]]
source = "other/darwin"
target = "~/.config"
os_only = true
`
	res, err := run(t, m, "linux", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
	res, err = run(t, m, "openbsd", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
}

func TestOSOnlyOverlapping(t *testing.T) {
	m := build(t, tree{
		files: []string{
			"config/starship.toml",
			"config/darwin/a",
			"config/darwin/karabiner/b",
		},
	})
	toml := `
[[auto]]
source = "config"
target = "~/.config"

[[auto.darwin]]
source = "config/darwin"
target = "~/.config"
os_only = true

[[auto.windows]]
source = "config/darwin/karabiner"
target = "~/.config"
os_only = true
`
	res, err := run(t, m, "linux", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
	res, err = run(t, m, "openbsd", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
}

func TestOSOnlyEqualToCommon(t *testing.T) {
	m := build(t, tree{
		files: []string{
			"config/darwin/karabiner.json",
		},
	})
	toml := `
[[auto]]
source = "config/darwin"
target = "~/.config"

[[auto.darwin]]
source = "config/darwin"
target = "~/.config"
os_only = true
`
	// The common rule contributes nothing on every OS; that is not an error.
	for _, goos := range []string{"linux", "openbsd"} {
		res, err := run(t, m, goos, toml)
		must(t, err)
		if len(res.Entries) != 0 {
			t.Fatalf("%s: expected no entries, got: %v", goos, summary(res))
		}
	}
	// On darwin the os_only rule itself still applies.
	res, err := run(t, m, "darwin", toml)
	must(t, err)
	eq(t, summary(res), []string{`home/.config/karabiner.json<-config/darwin/karabiner.json[auto[0].darwin[0]]`})
}

func TestOSOnlyDirectChildOnlyThatItemExcluded(t *testing.T) {
	m := build(t, tree{
		files: []string{
			"config/starship.toml",
			"config/os/darwin/karabiner.json",
			"config/os/linux/i3.conf",
		},
	})
	toml := `
[[auto]]
source = "config"
target = "~/.config"

[[auto.darwin]]
source = "config/os"
target = "~/.config"
os_only = true
`
	// config/os is a direct child of the common source: only it is excluded.
	for _, goos := range []string{"linux", "openbsd"} {
		res, err := run(t, m, goos, toml)
		must(t, err)
		eq(t, summary(res), []string{`home/.config/starship.toml<-config/starship.toml[auto[0]]`})
	}
}

func TestOSOnlyNestedCovered(t *testing.T) {
	m := build(t, tree{
		files: []string{
			"config/starship.toml",
			"config/os/darwin/a",
			"config/os/linux/b",
		},
	})
	want := []string{`home/.config/starship.toml<-config/starship.toml[auto[0]]`}

	// Covered by an os_only rule on the first-level item itself.
	covered := `
[[auto]]
source = "config"
target = "~/.config"

[[auto.darwin]]
source = "config/os/darwin"
target = "~/.config"
os_only = true

[[auto.linux]]
source = "config/os"
target = "~/.config"
os_only = true
`
	for _, goos := range []string{"linux", "openbsd"} {
		res, err := run(t, m, goos, covered)
		must(t, err)
		if goos == "openbsd" {
			eq(t, summary(res), want)
		}
	}

	// Covered by the common rule's ignore.
	ignored := `
[[auto]]
source = "config"
target = "~/.config"
ignore = ["os"]

[[auto.darwin]]
source = "config/os/darwin"
target = "~/.config"
os_only = true
`
	res, err := run(t, m, "openbsd", ignored)
	must(t, err)
	eq(t, summary(res), want)

	// Covered by a different os_only rule whose source equals the common source
	// makes the common rule empty.
	whole := `
[[auto]]
source = "config"
target = "~/.config"

[[auto.linux]]
source = "config"
target = "~/.config"
os_only = true

[[auto.darwin]]
source = "config/os/darwin"
target = "~/.config"
os_only = true
`
	if _, err := run(t, m, "openbsd", whole); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestOSOnlyNestedUncovered(t *testing.T) {
	m := build(t, tree{
		files: []string{
			"config/starship.toml",
			"config/os/darwin/a",
			"config/os/linux/b",
		},
	})
	toml := `
[[auto]]
source = "config"
target = "~/.config"

[[auto]]

[[auto.darwin]]
source = "config/os/darwin"
target = "~/.config"
os_only = true
`
	// A configuration error regardless of the running OS.
	for _, goos := range []string{"linux", "darwin", "openbsd"} {
		res, err := run(t, m, goos, toml)
		if err == nil || res != nil {
			t.Fatalf("%s: expected error, got %v", goos, res)
		}
		for _, w := range []string{"auto[1].darwin[0]", "auto[0]", "config/os/darwin", "nested more than one level", "config/os"} {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s: %q lacks %q", goos, err, w)
			}
		}
	}
}

func TestOSOnlyNestedCaseInsensitive(t *testing.T) {
	m := build(t, tree{
		files: []string{
			"config/starship.toml",
			"config/os/darwin/a",
		},
	})
	toml := `
[[auto]]
source = "config"
target = "~/.config"

[[auto.windows]]
source = "config/Os"
target = "~/.config"
os_only = true

[[auto.freebsd]]
source = "config/os/darwin"
target = "~/.config"
os_only = true
`
	// Folded on darwin/windows: config/Os covers config/os/darwin.
	res, err := run(t, m, "darwin", toml)
	must(t, err)
	eq(t, summary(res), []string{`home/.config/starship.toml<-config/starship.toml[auto[0]]`})
	// Case-sensitive on linux: config/Os is unrelated, so the nested one errors.
	if _, err := run(t, m, "linux", toml); err == nil {
		t.Fatal("expected error on linux")
	}
}

func TestOSOnlyDotsInsideResolves(t *testing.T) {
	m := build(t, tree{
		files: []string{
			"config/darwin/karabiner.json",
			"config/starship.toml",
		},
	})
	toml := `
[dots]
"~/.config/karabiner.json" = "config/darwin/karabiner.json"

[[auto]]
source = "config"
target = "~/.config"

[[auto.darwin]]
source = "config/darwin"
target = "~/.config"
os_only = true
`
	res, err := run(t, m, "linux", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/karabiner.json<-config/darwin/karabiner.json[dots."~/.config/karabiner.json"]`,
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
	res, err = run(t, m, "openbsd", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/karabiner.json<-config/darwin/karabiner.json[dots."~/.config/karabiner.json"]`,
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
}

func TestOSOnlyCaseInsensitive(t *testing.T) {
	m := build(t, tree{
		files: []string{
			"config/darwin/karabiner.json",
			"config/starship.toml",
		},
	})
	toml := `
[[auto]]
source = "config"
target = "~/.config"

[[auto.windows]]
source = "config/Darwin"
target = "~/.config"
os_only = true
`
	// Running on darwin: memfs is case-sensitive, but os_only matching folds case.
	// We use auto.windows to avoid expanding the missing "config/Darwin" dir on memfs.
	res, err := run(t, m, "darwin", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
}

func TestOSOnlyBaseTargetExcluded(t *testing.T) {
	m := build(t, tree{
		files: []string{"home/f"},
	})
	toml := `
[[auto]]
source = "home"
target = "~"

[[auto.darwin]]
source = "home"
target = "~/.config"
os_only = true
`
	// On linux, common is excluded, yielding no entries. auto.darwin doesn't run.
	res, err := run(t, m, "linux", toml)
	must(t, err)
	if len(res.Entries) != 0 {
		t.Errorf("expected no entries on linux, got: %v", summary(res))
	}
	// On openbsd, common is excluded, yielding no entries. auto.darwin doesn't run.
	res, err = run(t, m, "openbsd", toml)
	must(t, err)
	if len(res.Entries) != 0 {
		t.Errorf("expected no entries on openbsd, got: %v", summary(res))
	}

	// On darwin, common is excluded, yielding no entries. auto.darwin deploys "home" to "~/.config".
	res, err = run(t, m, "darwin", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/f<-home/f[auto[0].darwin[0]]`,
	})
}

func TestBaseTargetFullyIgnored(t *testing.T) {
	m := build(t, tree{
		files: []string{"home/f"},
	})
	toml := `
[[auto]]
source = "home"
target = "~"
ignore = ["*"]
`
	res, err := run(t, m, "linux", toml)
	must(t, err)
	if len(res.Entries) != 0 {
		t.Errorf("expected no entries, got: %v", summary(res))
	}
}
