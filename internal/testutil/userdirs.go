package testutil

import (
	"errors"
	"fmt"
	"os"
)

// userDirEnv lists every variable that os.UserHomeDir, os.UserConfigDir and
// os.UserCacheDir (and Zero's own config.UserConfigDir) read, across Linux,
// macOS and Windows. Setting only the XDG ones still leaves macOS resolving
// from HOME and Windows from USERPROFILE, APPDATA and LOCALAPPDATA.
var userDirEnv = []string{
	"HOME",
	"USERPROFILE",
	"APPDATA",
	"LOCALAPPDATA",
	"XDG_CONFIG_HOME",
	"XDG_CACHE_HOME",
	"XDG_DATA_HOME",
	"XDG_STATE_HOME",
}

// IsolateUserDirs points every per-user directory root at root so code under
// test cannot read the developer's real config, cache or home directory, or
// write to them. It is meant for a package's TestMain, which has no *testing.T
// (tests that need their own layout still use t.Setenv, which wins while it
// is active). The returned function restores the previous environment.
//
// It fails closed: if any variable cannot be redirected, the variables it had
// already changed are restored and an error is returned, so the caller must
// not run the tests against a partially isolated environment.
func IsolateUserDirs(root string) (restore func() error, err error) {
	type saved struct {
		name  string
		value string
		set   bool
	}
	previous := make([]saved, 0, len(userDirEnv))
	restore = func() error {
		var errs []error
		for _, prior := range previous {
			var err error
			if prior.set {
				err = os.Setenv(prior.name, prior.value)
			} else {
				err = os.Unsetenv(prior.name)
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("restore %s: %w", prior.name, err))
			}
		}
		return errors.Join(errs...)
	}
	for _, name := range userDirEnv {
		value, set := os.LookupEnv(name)
		if err := os.Setenv(name, root); err != nil {
			setErr := fmt.Errorf("isolate %s: %w", name, err)
			return nil, errors.Join(setErr, restore())
		}
		previous = append(previous, saved{name: name, value: value, set: set})
	}
	return restore, nil
}
