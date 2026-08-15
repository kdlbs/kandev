package main

import (
	"fmt"
	"os"
	"strconv"

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

const backendPIDFileEnv = "KANDEV_BACKEND_PID_FILE"

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
		if err := writeBackendPIDFile(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write backend pid file: %v\n", err)
			return 1
		}
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

func writeBackendPIDFile() error {
	path := os.Getenv(backendPIDFileEnv)
	if path == "" {
		return nil
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
}
