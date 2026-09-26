package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/backendapp"
	"github.com/kandev/kandev/internal/launcher"
)

func TestDispatchesHiddenBackendMode(t *testing.T) {
	backendCalled := false
	launcherCalled := false

	code := dispatch([]string{"__backend", "--version"}, buildInfo{Version: "test"}, func(args []string, build backendapp.BuildInfo) int {
		backendCalled = true
		if len(args) != 1 || args[0] != "--version" {
			t.Fatalf("backend args = %v, want [--version]", args)
		}
		if build.Version != "test" {
			t.Fatalf("backend build = %+v", build)
		}
		return 7
	}, func(args []string, build launcher.BuildInfo) int {
		launcherCalled = true
		return 0
	})

	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
	if !backendCalled {
		t.Fatal("backend runner was not called")
	}
	if launcherCalled {
		t.Fatal("launcher runner was called for hidden backend mode")
	}
}

func TestDispatchWritesBackendPIDFile(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "backend.pid")
	t.Setenv(backendPIDFileEnv, pidFile)

	code := dispatch([]string{"__backend"}, buildInfo{}, func([]string, backendapp.BuildInfo) int {
		return 7
	}, func([]string, launcher.BuildInfo) int {
		t.Fatal("launcher runner called for backend mode")
		return 0
	})
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read backend pid file: %v", err)
	}
	if got, want := strings.TrimSpace(string(raw)), strconv.Itoa(os.Getpid()); got != want {
		t.Fatalf("backend pid = %q, want %q", got, want)
	}
}

func TestDispatchDefaultsToLauncherMode(t *testing.T) {
	backendCalled := false
	launcherCalled := false

	code := dispatch([]string{"--help"}, buildInfo{Commit: "abc"}, func(args []string, build backendapp.BuildInfo) int {
		backendCalled = true
		return 0
	}, func(args []string, build launcher.BuildInfo) int {
		launcherCalled = true
		if len(args) != 1 || args[0] != "--help" {
			t.Fatalf("launcher args = %v, want [--help]", args)
		}
		if build.Commit != "abc" {
			t.Fatalf("launcher build = %+v", build)
		}
		return 3
	})

	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	if backendCalled {
		t.Fatal("backend runner was called for public launcher mode")
	}
	if !launcherCalled {
		t.Fatal("launcher runner was not called")
	}
}
