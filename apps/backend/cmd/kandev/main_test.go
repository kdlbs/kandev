package main

import "testing"

func TestRunDispatchesHiddenBackendMode(t *testing.T) {
	backendCalled := false
	launcherCalled := false
	oldBackend := runBackend
	oldLauncher := runLauncher
	t.Cleanup(func() {
		runBackend = oldBackend
		runLauncher = oldLauncher
	})
	runBackend = func(args []string, build buildInfo) int {
		backendCalled = true
		if len(args) != 1 || args[0] != "--version" {
			t.Fatalf("backend args = %v, want [--version]", args)
		}
		return 7
	}
	runLauncher = func(args []string, build buildInfo) int {
		launcherCalled = true
		return 0
	}

	code := run([]string{"__backend", "--version"})

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

func TestRunDefaultsToLauncherMode(t *testing.T) {
	backendCalled := false
	launcherCalled := false
	oldBackend := runBackend
	oldLauncher := runLauncher
	t.Cleanup(func() {
		runBackend = oldBackend
		runLauncher = oldLauncher
	})
	runBackend = func(args []string, build buildInfo) int {
		backendCalled = true
		return 0
	}
	runLauncher = func(args []string, build buildInfo) int {
		launcherCalled = true
		if len(args) != 1 || args[0] != "--help" {
			t.Fatalf("launcher args = %v, want [--help]", args)
		}
		return 3
	}

	code := run([]string{"--help"})

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
