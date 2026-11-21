package main

// Developer overrides: VOXTMINI_HELPER, VOXTMINI_LLM, VOXTMINI_OMNI_RUNTIME
// (other executables to run) and VOXTMINI_DEV_LLM (models from any
// folder) work in development builds only. In a published build any
// process of the user could set them (launchctl setenv) and have VoxtMini
// run its code, which would inherit VoxtMini's microphone and
// Accessibility permissions without a prompt (docs/audit
// security-privacy.md, M2). A published build is one `mygo build` linked
// an update feed into (scripts/release-config.sh; mygo.Updater.Configured).

import (
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/hehehai/VoxtMini/internal/helper"
	"github.com/hehehai/VoxtMini/internal/llmrt"
	"github.com/hehehai/VoxtMini/internal/omni"
)

// devOverrides reports whether this build honours the developer
// overrides (a test seam).
var devOverrides = func() bool { return !mygo.Updater.Configured() }

// overrideEnvs are the developer overrides' variables.
var overrideEnvs = []string{helper.EnvBin, llmrt.EnvBin, omni.EnvRuntime, llmrt.DevEnv}

// logIgnoredOverrides says, once at start, which overrides a published
// build ignores, so that a developer running one is not puzzled.
func logIgnoredOverrides() {
	if devOverrides() {
		return
	}
	for _, k := range overrideEnvs {
		if os.Getenv(k) != "" {
			log.Printf("%s is ignored: a published build runs only what it ships", k)
		}
	}
}
