package config

import (
	"strings"
	"testing"
)

func TestParseOSOnly(t *testing.T) {
	src := `
[[auto]]
source = "a"
target = "~"

[[auto.darwin]]
source = "b"
target = "~"
os_only = true

[[auto.linux]]
source = "c"
target = "~"
os_only = false

[[auto.windows]]
source = "d"
target = "~"
`
	cfg, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Auto) != 1 {
		t.Fatalf("auto: %+v", cfg.Auto)
	}
	g := cfg.Auto[0]
	if len(g.OS["darwin"]) != 1 || !g.OS["darwin"][0].OSOnly {
		t.Errorf("darwin os_only should be true")
	}
	if len(g.OS["linux"]) != 1 || g.OS["linux"][0].OSOnly {
		t.Errorf("linux os_only should be false")
	}
	if len(g.OS["windows"]) != 1 || g.OS["windows"][0].OSOnly {
		t.Errorf("windows os_only should be false (absent)")
	}
}

func TestParseOSOnlyErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "os_only on common auto",
			src:  "[[auto]]\nsource = \"a\"\ntarget = \"~\"\nos_only = true\n",
			want: "auto[0]: os_only is only allowed in auto.<os> rules",
		},
		{
			name: "non-boolean os_only",
			src:  "[[auto]]\n[[auto.darwin]]\nsource = \"b\"\ntarget = \"~\"\nos_only = \"true\"\n",
			want: "auto[0].darwin[0].os_only: must be a boolean",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.src))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q lacks %q", err.Error(), tt.want)
			}
		})
	}
}
