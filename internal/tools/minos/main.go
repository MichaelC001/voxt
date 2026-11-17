// Command minos checks that a build runs on the macOS it says it needs
// (mygo.json's macos.minimumSystemVersion): no Mach-O file (executable,
// library, framework, any architecture of a universal one) may need a newer
// macOS (its LC_BUILD_VERSION or LC_VERSION_MIN_MACOSX "minos"), and no
// Metal library (.metallib) may target a newer macOS or use a Metal
// language newer than that macOS compiles. scripts/release-verify.sh runs it
// over the app (issue #162: an MLX built for macOS 26 shipped in a build for
// macOS 15).
//
//	go run ./internal/tools/minos -mygo mygo.json build/darwin-arm64/VoxtMini.app
//	go run ./internal/tools/minos -min 15.0 FILE_OR_DIR...
//
// It prints a line per file it checked; exit status 0 when every one is
// fine, 1 when one needs a newer macOS (or cannot be read), 2 for usage
// errors.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func main() {
	min := flag.String("min", "", "the oldest macOS the build supports, e.g. 15.0")
	mygo := flag.String("mygo", "", "read -min from this mygo.json (macos.minimumSystemVersion)")
	flag.Parse()
	if (*min == "") == (*mygo == "") || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: minos (-min 15.0 | -mygo mygo.json) FILE_OR_DIR...")
		os.Exit(2)
	}
	if *mygo != "" {
		v, err := mygoMinimum(*mygo)
		if err != nil {
			fmt.Fprintln(os.Stderr, "minos:", err)
			os.Exit(2)
		}
		*min = v
	}
	limit, err := parseVersion(*min)
	if err != nil {
		fmt.Fprintln(os.Stderr, "minos:", err)
		os.Exit(2)
	}
	if run(os.Stdout, limit, flag.Args()) > 0 {
		os.Exit(1)
	}
}

// run checks every file under paths against limit and prints a line each;
// it returns the number of failures.
func run(w io.Writer, limit version, paths []string) int {
	fails, checked := 0, 0
	for _, root := range paths {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.Type().IsRegular() {
				return nil
			}
			res, err := checkFile(path, limit)
			if err != nil {
				fails++
				fmt.Fprintf(w, "FAIL %s: %v\n", path, err)
				return nil
			}
			for _, r := range res {
				checked++
				if r.ok {
					fmt.Fprintf(w, "ok   %s: %s\n", path, r.what)
				} else {
					fails++
					fmt.Fprintf(w, "FAIL %s: %s, newer than macOS %s\n", path, r.what, limit)
				}
			}
			return nil
		})
		if err != nil {
			fails++
			fmt.Fprintf(w, "FAIL %v\n", err)
		}
	}
	if checked == 0 && fails == 0 {
		fails++
		fmt.Fprintln(w, "FAIL no Mach-O file or Metal library found")
	}
	return fails
}

// mygoMinimum is mygo.json's macos.minimumSystemVersion.
func mygoMinimum(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var cfg struct {
		MacOS struct {
			MinimumSystemVersion string `json:"minimumSystemVersion"`
		} `json:"macos"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if cfg.MacOS.MinimumSystemVersion == "" {
		return "", fmt.Errorf("%s has no macos.minimumSystemVersion", path)
	}
	return cfg.MacOS.MinimumSystemVersion, nil
}
