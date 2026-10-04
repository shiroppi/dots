// Package platform captures the runtime environment (OS name, home
// directory, environment variables, working directory) so it can be injected
// in tests instead of being read from global process state.
package platform

import (
	"os"
	"runtime"
)

// Env is an injectable snapshot of the process environment.
type Env struct {
	// GOOS is the Go runtime OS name (runtime.GOOS), e.g. "darwin",
	// "windows", "linux", "freebsd".
	GOOS string
	// Home is the absolute home directory of the current user.
	Home string
	// Cwd is the absolute current working directory.
	Cwd string
	// Vars holds environment variables. A missing key means unset.
	Vars map[string]string
}

// Getenv returns the value of key, or "" when unset.
func (e Env) Getenv(key string) string { return e.Vars[key] }

// LookupEnv returns the value of key and whether it is set.
func (e Env) LookupEnv(key string) (string, bool) {
	v, ok := e.Vars[key]
	return v, ok
}

// Current reads the real process environment.
func Current() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return Env{}, err
	}
	vars := map[string]string{}
	for _, key := range []string{"XDG_DATA_HOME", "LOCALAPPDATA", "EDITOR", "VISUAL", "HOME", "USERPROFILE"} {
		if v, ok := os.LookupEnv(key); ok {
			vars[key] = v
		}
	}
	return Env{GOOS: runtime.GOOS, Home: home, Cwd: cwd, Vars: vars}, nil
}
