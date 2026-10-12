package platform

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestAppDir(t *testing.T) {
	base := "/"
	if runtime.GOOS == "windows" {
		base = `C:\`
	}
	abs := filepath.Join(base, "data")
	rel := filepath.Join("rel", "dir")
	home := filepath.Join(base, "home")
	tests := []struct {
		name    string
		env     Env
		kind    Kind
		want    string
		wantErr bool
	}{
		{"windows data", Env{GOOS: "windows", Home: home, Vars: map[string]string{"LOCALAPPDATA": abs}}, DataDir, filepath.Join(abs, "dots"), false},
		{"windows state", Env{GOOS: "windows", Home: home, Vars: map[string]string{"LOCALAPPDATA": abs}}, StateDir, filepath.Join(abs, "dots"), false},
		{"windows unset", Env{GOOS: "windows", Home: home}, StateDir, "", true},
		{"windows relative", Env{GOOS: "windows", Home: home, Vars: map[string]string{"LOCALAPPDATA": rel}}, StateDir, "", true},
		{"darwin state", Env{GOOS: "darwin", Home: home, Vars: map[string]string{"XDG_STATE_HOME": abs}}, StateDir, filepath.Join(home, "Library", "Application Support", "dots"), false},
		{"darwin no home", Env{GOOS: "darwin"}, StateDir, "", true},
		{"linux state xdg", Env{GOOS: "linux", Home: home, Vars: map[string]string{"XDG_STATE_HOME": abs}}, StateDir, filepath.Join(abs, "dots"), false},
		{"linux state empty", Env{GOOS: "linux", Home: home, Vars: map[string]string{"XDG_STATE_HOME": ""}}, StateDir, filepath.Join(home, ".local", "state", "dots"), false},
		{"linux state unset", Env{GOOS: "linux", Home: home}, StateDir, filepath.Join(home, ".local", "state", "dots"), false},
		{"linux state relative", Env{GOOS: "linux", Home: home, Vars: map[string]string{"XDG_STATE_HOME": rel}}, StateDir, "", true},
		{"linux state ignores data var", Env{GOOS: "linux", Home: home, Vars: map[string]string{"XDG_DATA_HOME": abs}}, StateDir, filepath.Join(home, ".local", "state", "dots"), false},
		{"linux data xdg", Env{GOOS: "linux", Home: home, Vars: map[string]string{"XDG_DATA_HOME": abs}}, DataDir, filepath.Join(abs, "dots"), false},
		{"linux data fallback", Env{GOOS: "linux", Home: home}, DataDir, filepath.Join(home, ".local", "share", "dots"), false},
		{"freebsd state fallback", Env{GOOS: "freebsd", Home: home}, StateDir, filepath.Join(home, ".local", "state", "dots"), false},
		{"openbsd state xdg", Env{GOOS: "openbsd", Home: home, Vars: map[string]string{"XDG_STATE_HOME": abs}}, StateDir, filepath.Join(abs, "dots"), false},
		{"linux no home", Env{GOOS: "linux"}, StateDir, "", true},
		{"unknown kind", Env{GOOS: "linux", Home: home}, Kind(0), "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AppDir(tc.env, tc.kind)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
