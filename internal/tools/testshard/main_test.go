package main

import (
	"slices"
	"testing"
)

func TestSplitBalances(t *testing.T) {
	times := map[string]float64{"A": 7, "B": 3, "C": 3, "D": 1, "E": 1, "F": 1}
	shards := split([]string{"F", "E", "D", "C", "B", "A", "New"}, times, 3)
	if len(shards) != 3 {
		t.Fatalf("%d shards", len(shards))
	}
	var all []string
	load := make([]float64, len(shards))
	for i, s := range shards {
		all = append(all, s...)
		for _, n := range s {
			load[i] += times[n] // New counts 0 here, its guess is the median (2)
		}
	}
	slices.Sort(all)
	if !slices.Equal(all, []string{"A", "B", "C", "D", "E", "F", "New"}) {
		t.Fatalf("tests lost or doubled: %v", all)
	}
	if !slices.Equal(shards[0], []string{"A"}) {
		t.Errorf("the longest test should run alone: %v", shards)
	}
	if slices.Max(load) != 7 {
		t.Errorf("loads %v, want the longest shard 7", load)
	}
}

func TestSplitFewTests(t *testing.T) {
	if s := split([]string{"A"}, nil, 8); len(s) != 1 {
		t.Fatalf("one test, %d shards", len(s))
	}
}

func TestTestNames(t *testing.T) {
	got := testNames("TestA\nBenchmarkB\nExampleC\nFuzzD\n\n")
	if !slices.Equal(got, []string{"TestA", "ExampleC", "FuzzD"}) {
		t.Fatal(got)
	}
}

func TestResultLine(t *testing.T) {
	for line, want := range map[string]string{
		"--- PASS: TestA (1.23s)":        "TestA",
		"--- FAIL: TestB (0.00s)":        "TestB",
		"--- SKIP: TestC (0.01s)":        "TestC",
		"    --- PASS: TestA/sub (0.1s)": "",
	} {
		m := resultLine.FindStringSubmatch(line)
		got := ""
		if m != nil {
			got = m[2]
		}
		if got != want {
			t.Errorf("%q: %q, want %q", line, got, want)
		}
	}
}
