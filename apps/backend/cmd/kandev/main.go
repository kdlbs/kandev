package main

import (
	"os"

	"github.com/kandev/kandev/internal/backendapp"
	"github.com/kandev/kandev/internal/launcher"
)

// Build-time variables injected via -ldflags "-X main.Version=... -X main.Commit=... -X main.BuildTime=..."
// (see apps/backend/Makefile). Defaults apply when running un-stamped builds.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

type buildInfo struct {
	Version   string
	Commit    string
	BuildTime string
}

var runBackend = func(args []string, build buildInfo) int {
	return backendapp.Run(args, backendapp.BuildInfo{
		Version:   build.Version,
		Commit:    build.Commit,
		BuildTime: build.BuildTime,
	})
}

var runLauncher = func(args []string, build buildInfo) int {
	return launcher.Run(args, launcher.BuildInfo{
		Version:   build.Version,
		Commit:    build.Commit,
		BuildTime: build.BuildTime,
	})
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	build := buildInfo{Version: Version, Commit: Commit, BuildTime: BuildTime}
	if len(args) > 0 && args[0] == "__backend" {
		return runBackend(args[1:], build)
	}
	return runLauncher(args, build)
}
