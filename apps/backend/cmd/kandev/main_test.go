package main

import (
	"testing"

	"github.com/kandev/kandev/internal/backendapp"
	"github.com/kandev/kandev/internal/launcher"
)

func TestDispatchesHiddenBackendMode(t *testing.T) {
	backendCalled := false
	launcherCalled := false

	code := dispatch([]string{"__backend", "--version"}, buildInfo{Version: "test"}, noHelper, func(args []string, build backendapp.BuildInfo) int {
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

func TestDispatchDefaultsToLauncherMode(t *testing.T) {
	backendCalled := false
	launcherCalled := false

	code := dispatch([]string{"--help"}, buildInfo{Commit: "abc"}, noHelper, func(args []string, build backendapp.BuildInfo) int {
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

func TestDispatchRunsInternalHelperBeforeBackend(t *testing.T) {
	backendCalled := false
	launcherCalled := false
	helper := func(args []string) (int, bool) {
		if len(args) != 1 || args[0] != "__git-init-open-directory" {
			t.Fatalf("helper args = %v", args)
		}
		return 9, true
	}

	code := dispatch([]string{"__git-init-open-directory"}, buildInfo{}, helper, func([]string, backendapp.BuildInfo) int {
		backendCalled = true
		return 0
	}, func([]string, launcher.BuildInfo) int {
		launcherCalled = true
		return 0
	})

	if code != 9 {
		t.Fatalf("exit code = %d, want 9", code)
	}
	if backendCalled || launcherCalled {
		t.Fatalf("backend called=%t launcher called=%t, want neither", backendCalled, launcherCalled)
	}
}

func noHelper([]string) (int, bool) {
	return 0, false
}
