// Command testshard runs one package's tests in N processes at once, the
// tests split so that each process takes about as long: the app package's
// end-to-end tests wait in real time and none of them is parallel, so one
// process takes a minute where eight take ten seconds. scripts/test-main.sh
// runs it for the app package (app/).
//
//	go run ./internal/tools/testshard [-n 8] [-race] [-run RE] [-v] [-times FILE] [-timeout 10m] [PKG]
//
// It builds the package's test binary once (go test -c), lists its tests
// (-test.list), splits them by the durations of the last run (FILE,
// default build/test-times.json; tests it does not know count as the
// median), runs the shards from the package's directory and writes the
// durations of this run back. A shard that fails prints all its output; -v
// prints every shard's. Every listed test must report PASS, FAIL or SKIP:
// one that never ran (a shard that crashed or timed out) is a failure.
//
// Exit status: 0 all passed, 1 a test failed or did not run, 2 usage or
// build errors.
package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	n := flag.Int("n", 8, "processes")
	race := flag.Bool("race", false, "build with -race (needs cgo)")
	run := flag.String("run", ".", "only the top-level tests that match")
	verbose := flag.Bool("v", false, "print every shard's output")
	timesFile := flag.String("times", "build/test-times.json", "durations of the last run")
	timeout := flag.Duration("timeout", 10*time.Minute, "each shard's -test.timeout")
	flag.Parse()
	pkg := "."
	if flag.NArg() > 0 {
		pkg = flag.Arg(0)
	}
	if *n < 1 {
		*n = 1
	}
	os.Exit(runAll(*n, *race, *run, *verbose, *timesFile, *timeout, pkg))
}

func runAll(n int, race bool, run string, verbose bool, timesFile string, timeout time.Duration, pkg string) int {
	tmp, err := os.MkdirTemp("", "testshard-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer os.RemoveAll(tmp)
	bin := filepath.Join(tmp, "pkg.test")
	args := []string{"test", "-c", "-o", bin}
	if race {
		args = append(args, "-race")
	}
	build := exec.Command("go", append(args, pkg)...)
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if race {
		build.Env = append(os.Environ(), "CGO_ENABLED=1")
	}
	if err := build.Run(); err != nil {
		return 2
	}
	dir, err := pkgDir(pkg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	list := exec.Command(bin, "-test.list", run)
	list.Dir = dir
	out, err := list.Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "listing the tests:", err)
		return 2
	}
	tests := testNames(string(out))
	if len(tests) == 0 {
		fmt.Fprintln(os.Stderr, "no tests match", run)
		return 0
	}
	old := readTimes(timesFile)
	shards := split(tests, old, n)

	start := time.Now()
	results := make([]shardResult, len(shards))
	var wg sync.WaitGroup
	for i, s := range shards {
		wg.Go(func() { results[i] = runShard(bin, dir, s, timeout) })
	}
	wg.Wait()

	status := 0
	now := map[string]float64{}
	for i, r := range results {
		for name, d := range r.times {
			now[name] = d
		}
		missing := 0
		for _, t := range shards[i] {
			if _, ok := r.done[t]; !ok {
				missing++
			}
		}
		failed := r.err != nil || missing > 0
		if failed || verbose {
			os.Stdout.Write(r.out)
		}
		mark := "ok  "
		if failed {
			mark, status = "FAIL", 1
		}
		fmt.Printf("%s shard %d/%d: %d tests, %.1fs", mark, i+1, len(shards), len(shards[i]), r.wall.Seconds())
		if missing > 0 {
			fmt.Printf(", %d did not run", missing)
		}
		fmt.Println()
	}
	for _, r := range results {
		for _, t := range r.failed {
			fmt.Println("--- FAIL:", t)
		}
	}
	fmt.Printf("%d tests in %d processes: %.1fs\n", len(tests), len(shards), time.Since(start).Seconds())
	writeTimes(timesFile, old, now)
	return status
}

// pkgDir is the directory of the package, where its tests run.
func pkgDir(pkg string) (string, error) {
	out, err := exec.Command("go", "list", "-f", "{{.Dir}}", pkg).Output()
	if err != nil {
		return "", fmt.Errorf("go list %s: %w", pkg, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// testNames keeps the tests, fuzz targets (their seeds run as tests) and
// examples of -test.list's output, not the benchmarks.
func testNames(out string) []string {
	var names []string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "Benchmark") || strings.HasPrefix(l, "ok ") {
			continue
		}
		names = append(names, l)
	}
	return names
}

// split deals the tests out to n shards, the longest first, each to the
// shard with the least time so far (the longest-processing-time rule).
// Unknown tests count as the median of the known ones.
func split(tests []string, times map[string]float64, n int) [][]string {
	n = min(n, len(tests))
	known := []float64{}
	for _, t := range tests {
		if d, ok := times[t]; ok {
			known = append(known, d)
		}
	}
	guess := 0.1
	if len(known) > 0 {
		slices.Sort(known)
		guess = known[len(known)/2]
	}
	cost := func(t string) float64 {
		if d, ok := times[t]; ok {
			return d
		}
		return guess
	}
	sorted := slices.Clone(tests)
	slices.SortStableFunc(sorted, func(a, b string) int {
		if c := cmp.Compare(cost(b), cost(a)); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	shards := make([][]string, n)
	load := make([]float64, n)
	for _, t := range sorted {
		i := slices.Index(load, slices.Min(load))
		shards[i] = append(shards[i], t)
		load[i] += cost(t)
	}
	return shards
}

type shardResult struct {
	out    []byte
	err    error
	wall   time.Duration
	done   map[string]bool    // top-level tests that reported a result
	times  map[string]float64 // their durations, seconds
	failed []string
}

var resultLine = regexp.MustCompile(`^--- (PASS|FAIL|SKIP): (\S+) \(([0-9.]+)s\)`)

func runShard(bin, dir string, tests []string, timeout time.Duration) shardResult {
	pattern := "^(" + strings.Join(quoted(tests), "|") + ")$"
	cmd := exec.Command(bin, "-test.v", "-test.run", pattern, "-test.timeout", timeout.String())
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	start := time.Now()
	err := cmd.Run()
	r := shardResult{out: buf.Bytes(), err: err, wall: time.Since(start),
		done: map[string]bool{}, times: map[string]float64{}}
	sc := bufio.NewScanner(bytes.NewReader(r.out))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		m := resultLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue // subtests are indented
		}
		r.done[m[2]] = true
		if d, err := strconv.ParseFloat(m[3], 64); err == nil {
			r.times[m[2]] = d
		}
		if m[1] == "FAIL" {
			r.failed = append(r.failed, m[2])
		}
	}
	// Examples report no duration line in every Go version: count a clean
	// exit as their result.
	if err == nil {
		for _, t := range tests {
			if strings.HasPrefix(t, "Example") {
				r.done[t] = true
			}
		}
	}
	return r
}

func quoted(names []string) []string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = regexp.QuoteMeta(n)
	}
	return q
}

func readTimes(file string) map[string]float64 {
	m := map[string]float64{}
	b, err := os.ReadFile(file)
	if err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

// writeTimes keeps the old durations of the tests this run left out (-run).
func writeTimes(file string, old, now map[string]float64) {
	if len(now) == 0 {
		return
	}
	for k, v := range now {
		old[k] = v
	}
	b, err := json.MarshalIndent(old, "", "\t")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err == nil {
		_ = os.WriteFile(file, append(b, '\n'), 0o644)
	}
}
