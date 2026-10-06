package resolve

import (
	"testing"

	"github.com/shiroppi/dots/internal/model"
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
		`home/.config/karabiner/karabiner.json<-config/darwin/karabiner/karabiner.json[auto[0].darwin[0]]`,
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
		`home/.config/darwin/karabiner/karabiner.json<-config/darwin/karabiner/karabiner.json[auto[0]]`,
		`home/.config/karabiner/karabiner.json<-config/darwin/karabiner/karabiner.json[auto[0].darwin[0]]`,
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
	res, err := run(t, m, "linux", toml)
	must(t, err)
	// For linux, auto[0] is completely excluded, so we get a single real-directory entry for target root
	if len(res.Entries) != 1 || res.Entries[0].Kind != model.KindDir || res.Entries[0].Target != h(".config") {
		t.Fatalf("expected real directory for excluded root, got: %v", summary(res))
	}
	res, err = run(t, m, "openbsd", toml)
	must(t, err)
	// For openbsd, auto[0] is completely excluded, so we get a single real-directory entry for target root
	if len(res.Entries) != 1 || res.Entries[0].Kind != model.KindDir || res.Entries[0].Target != h(".config") {
		t.Fatalf("expected real directory for excluded root on openbsd, got: %v", summary(res))
	}
}

func TestOSOnlyEmptiedDirBecomesDir(t *testing.T) {
	m := build(t, tree{
		dirs: []string{"config/os/darwin"},
		files: []string{
			"config/starship.toml",
			"config/os/darwin/karabiner.json",
		},
	})
	toml := `
[[auto]]
source = "config"
target = "~/.config"

[[auto.darwin]]
source = "config/os/darwin"
target = "~/.config"
os_only = true
`
	res, err := run(t, m, "linux", toml)
	must(t, err)
	// config/os is emptied by the exclusion of config/os/darwin, so it becomes a KindDir
	eq(t, summary(res), []string{
		`home/.config/os<-config/os[auto[0]]`,
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
	if res.Entries[0].Kind != model.KindDir {
		t.Errorf("expected config/os to be KindDir, got %v", res.Entries[0].Kind)
	}
	res, err = run(t, m, "openbsd", toml)
	must(t, err)
	// config/os is emptied by the exclusion of config/os/darwin, so it becomes a KindDir
	eq(t, summary(res), []string{
		`home/.config/os<-config/os[auto[0]]`,
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
	if res.Entries[0].Kind != model.KindDir {
		t.Errorf("expected config/os to be KindDir on openbsd, got %v", res.Entries[0].Kind)
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
	// Running on darwin: memfs is case-sensitive, but os_only subtrees matching folds case.
	// We use auto.windows to avoid expanding the missing "config/Darwin" dir on memfs.
	res, err := run(t, m, "darwin", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config/starship.toml<-config/starship.toml[auto[0]]`,
	})
}

func TestOSOnlySubtrees(t *testing.T) {
	tests := []struct {
		name      string
		goos      string
		commonSrc string
		sources   []string
		want      []string
	}{
		{
			name:      "exact match",
			goos:      "linux",
			commonSrc: "config/darwin",
			sources:   []string{"config/darwin"},
			want:      []string{""},
		},
		{
			name:      "inside",
			goos:      "linux",
			commonSrc: "config",
			sources:   []string{"config/darwin", "config/windows/app"},
			want:      []string{"darwin", "windows/app"},
		},
		{
			name:      "outside",
			goos:      "linux",
			commonSrc: "config",
			sources:   []string{"other", "confignot/darwin"},
			want:      nil,
		},
		{
			name:      "case insensitive exact",
			goos:      "darwin",
			commonSrc: "config/darwin",
			sources:   []string{"Config/Darwin"},
			want:      []string{""},
		},
		{
			name:      "case insensitive inside",
			goos:      "windows",
			commonSrc: "Config",
			sources:   []string{"config/Darwin"},
			want:      []string{"Darwin"},
		},
		{
			name:      "case sensitive miss",
			goos:      "linux",
			commonSrc: "config",
			sources:   []string{"Config/darwin"},
			want:      nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := osOnlySubtrees(tt.goos, tt.commonSrc, tt.sources)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
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

func TestNonBaseTargetFullyIgnored(t *testing.T) {
	m := build(t, tree{
		files: []string{"home/f"},
	})
	toml := `
[[auto]]
source = "home"
target = "~/.config"
ignore = ["*"]
`
	res, err := run(t, m, "linux", toml)
	must(t, err)
	eq(t, summary(res), []string{
		`home/.config<-home[auto[0]]`,
	})
	if res.Entries[0].Kind != model.KindDir {
		t.Errorf("expected KindDir, got %v", res.Entries[0].Kind)
	}
}

func TestBaseTargetPhysicallyEmptyError(t *testing.T) {
	m := build(t, tree{
		dirs: []string{"empty_home"},
	})
	toml := `
[[auto]]
source = "empty_home"
target = "~"
`
	_, err := run(t, m, "linux", toml)
	if err == nil {
		t.Errorf("expected error for empty source root with base target, got nil")
	}
}
