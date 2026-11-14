// Command fakehelper is helpertest's fake voxtmini-helper as a program,
// to try the app without the Swift helper (on Linux, or on a Mac without
// Xcode): build it and point VOXTMINI_HELPER at it,
//
//	go build -o /tmp/fakehelper ./internal/helper/helpertest/cmd/fakehelper
//	VOXTMINI_HELPER=/tmp/fakehelper go tool mygo dev
//
// It takes the options of VOXTMINI_FAKE_HELPER (helpertest) from its
// arguments or that variable; it records a tone, not the microphone.
package main

import (
	"os"
	"strings"

	"github.com/hehehai/VoxtMini/internal/helper/helpertest"
)

func main() {
	opts := os.Getenv("VOXTMINI_FAKE_HELPER")
	if len(os.Args) > 1 {
		opts = strings.Join(os.Args[1:], ",")
	}
	helpertest.Run(opts)
}
