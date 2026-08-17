//go:build !windows

package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

func prepareBackgroundCommand(cmd *exec.Cmd) {}

func restoreExistingSleepySourceWindow() bool { return false }

func runNativeWindow(app *App) {
	fmt.Println(appTitle)
	fmt.Println("Designer:", dashboardURL)
	fmt.Println("OBS overlay:", overlayURL)
	select {}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func openFolder(path string)          { openBrowser("file://" + path) }
func showFatal(title, message string) { fmt.Println(title + ": " + message) }

func runPlatformMediaDetector(ctx context.Context, app *App) {
	app.updateTrack(Track{}, "Windows media-session detection is available in the Windows build.")
	<-ctx.Done()
}
