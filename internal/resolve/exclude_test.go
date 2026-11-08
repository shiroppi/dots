package resolve

import (
	"strings"
	"testing"
)

func TestAutoOSExcludeReadmeExample(t *testing.T) {
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

	// Exception: also linking ~/.config/darwin itself via [dots]
	tomlWithDots := `
[dots]
"~/.config/darwin" = "config/darwin"

[[auto]]
source = "config"
target = "~/.config"

[[auto.darwin]]
source = "config/darwin"
target = "~/.config"
`
	res, err = run(t, m, "darwin", tomlWithDots)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/darwin<-config/darwin[dots."~/.config/darwin"]`,
		`home/.config/karabiner<-config/darwin/karabiner[auto[0].darwin[0]]`,
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
}

func TestAutoOSExcludeInactiveOS(t *testing.T) {
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

func TestAutoOSExcludeOutsideCommonSource(t *testing.T) {
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

func TestAutoOSExcludeOverlapping(t *testing.T) {
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

[[auto.windows]]
source = "config/darwin/karabiner"
target = "~/.config"
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

func TestAutoOSExcludeEqualToCommon(t *testing.T) {
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
`
	// The common rule contributes nothing on every OS; that is not an error.
	for _, goos := range []string{"linux", "openbsd"} {
		res, err := run(t, m, goos, toml)
		must(t, err)
		if len(res.Entries) != 0 {
			t.Fatalf("%s: expected no entries, got: %v", goos, summary(res))
		}
	}
	// On darwin the auto.darwin rule itself still applies.
	res, err := run(t, m, "darwin", toml)
	must(t, err)
	eq(t, summary(res), []string{`home/.config/karabiner.json<-config/darwin/karabiner.json[auto[0].darwin[0]]`})
}

func TestAutoOSExcludeDirectChild(t *testing.T) {
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
`
	// config/os is a direct child of the common source: only it is excluded.
	for _, goos := range []string{"linux", "openbsd"} {
		res, err := run(t, m, goos, toml)
		must(t, err)
		eq(t, summary(res), []string{`home/.config/starship.toml<-config/starship.toml[auto[0]]`})
	}
}

func TestAutoOSExcludeNestedCovered(t *testing.T) {
	m := build(t, tree{
		files: []string{
			"config/starship.toml",
			"config/os/darwin/a",
			"config/os/linux/b",
		},
	})
	want := []string{`home/.config/starship.toml<-config/starship.toml[auto[0]]`}

	// Covered by an auto.<os> rule on the first-level item itself.
	covered := `
[[auto]]
source = "config"
target = "~/.config"

[[auto.darwin]]
source = "config/os/darwin"
target = "~/.config"

[[auto.linux]]
source = "config/os"
target = "~/.config"
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
`
	res, err := run(t, m, "openbsd", ignored)
	must(t, err)
	eq(t, summary(res), want)

	// Covered by a different auto.<os> rule whose source equals the common source
	// makes the common rule empty.
	whole := `
[[auto]]
source = "config"
target = "~/.config"

[[auto.linux]]
source = "config"
target = "~/.config"

[[auto.darwin]]
source = "config/os/darwin"
target = "~/.config"
`
	if _, err := run(t, m, "openbsd", whole); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestAutoOSExcludeNestedUncovered(t *testing.T) {
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
		if strings.Contains(err.Error(), "os_only") {
			t.Errorf("%s: error must not mention os_only: %q", goos, err)
		}
	}
}

func TestAutoOSExcludeNestedCaseInsensitive(t *testing.T) {
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

[[auto.freebsd]]
source = "config/os/darwin"
target = "~/.config"
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

func TestAutoOSExcludeDotsInsideResolves(t *testing.T) {
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

func TestAutoOSExcludeCaseInsensitive(t *testing.T) {
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
`
	// Running on darwin: memfs is case-sensitive, but auto.<os> matching folds case.
	// We use auto.windows to avoid expanding the missing "config/Darwin" dir on memfs.
	res, err := run(t, m, "darwin", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
}

func TestAutoOSExcludeBaseTarget(t *testing.T) {
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
