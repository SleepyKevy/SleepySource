package main

import (
	"os"
	"strings"
)

func main() {
	app, err := newApp()
	if err != nil {
		showFatal(appTitle, err.Error())
		return
	}
	if err := app.startServer(); err != nil {
		if restoreExistingSleepySourceWindow() {
			return
		}
		showFatal(appTitle, err.Error()+"\n\nAnother copy may already be running.")
		return
	}
	app.startDetector()

	// SleepySource 1.0 Beta uses the mature 1.3.2 backend as a temporary
	// compatibility engine behind the new C#/.NET desktop host. In this mode
	// the engine owns APIs, OBS routes, Kick/relay integration and media state,
	// while the C# host owns all visible desktop UI and lifecycle.
	if engineOnlyMode() {
		select {}
	}

	runNativeWindow(app)
	app.shutdown()
}

func engineOnlyMode() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("SLEEPYSOURCE_ENGINE_ONLY")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("SLEEPYSOURCE_ENGINE_ONLY")), "true") {
		return true
	}
	for _, arg := range os.Args[1:] {
		if strings.EqualFold(strings.TrimSpace(arg), "--engine-only") {
			return true
		}
	}
	return false
}
