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

type backendRunner func(args []string, build backendapp.BuildInfo) int
type launcherRunner func(args []string, build launcher.BuildInfo) int

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	build := buildInfo{Version: Version, Commit: Commit, BuildTime: BuildTime}
	return dispatch(args, build, backendapp.Run, launcher.Run)
}

func dispatch(args []string, build buildInfo, backend backendRunner, launch launcherRunner) int {
	if len(args) > 0 && args[0] == "__backend" {
		return backend(args[1:], backendapp.BuildInfo{
			Version:   build.Version,
			Commit:    build.Commit,
			BuildTime: build.BuildTime,
		})
	}
	return launch(args, launcher.BuildInfo{
		Version:   build.Version,
		Commit:    build.Commit,
		BuildTime: build.BuildTime,
	})
}
