package main

import (
	"os"
	"path/filepath"
)

// chdirModuleRoot makes the module's root the tests' working directory, as
// it is the app's under mygo dev and go run ./app: resources/ (the
// sounds, the staged helper and runtimes on a Mac), testdata/, helper/
// and internal/ are found there by their paths from the root. A copy of
// the test binary started elsewhere (as a fake process, from a temporary
// directory) stays where it is.
func chdirModuleRoot() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if err := os.Chdir(dir); err != nil {
				panic(err)
			}
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}
