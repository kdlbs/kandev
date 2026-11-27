//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestIsolatedNativeChildPreservesArgumentsLogsAndExit(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("KANDEV_WINJOB_TEST_CHILD", "1")
	t.Setenv("KANDEV_WINJOB_TEST_PID", filepath.Join(dir, "observed.pid"))
	identityPath := filepath.Join(dir, "identity.json")
	stdoutPath := filepath.Join(dir, "stdout %literal% [file].log")
	stderrPath := filepath.Join(dir, "stderr.log")
	argument := "literal %TEMP% ! & ^ [argument]"
	err = runIsolated([]string{identityPath, stdoutPath, stderrPath, executable,
		"-test.run=^TestIsolatedNativeChildHelper$", "--", argument})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("child error = %v, want exit 7", err)
	}
	stdout, err := os.ReadFile(stdoutPath)
	if err != nil || string(stdout) != argument {
		t.Fatalf("stdout = %q, %v; want literal argument", stdout, err)
	}
	stderr, err := os.ReadFile(stderrPath)
	if err != nil || string(stderr) != "child diagnostic" {
		t.Fatalf("stderr = %q, %v", stderr, err)
	}
	data, err := os.ReadFile(identityPath)
	if err != nil {
		t.Fatal(err)
	}
	var identity isolatedIdentity
	if err := json.Unmarshal(data, &identity); err != nil {
		t.Fatal(err)
	}
	observed, err := os.ReadFile(filepath.Join(dir, "observed.pid"))
	if err != nil || string(observed) != strconv.Itoa(identity.PID) {
		t.Fatalf("identity PID %d does not match actual child %q: %v", identity.PID, observed, err)
	}
	if len(identity.StartedTicks) != 18 || !strings.HasSuffix(identity.StartedTicks, "0") {
		t.Fatalf("invalid CIM-compatible creation ticks %q", identity.StartedTicks)
	}
}

func TestIsolatedNativeChildHelper(t *testing.T) {
	if os.Getenv("KANDEV_WINJOB_TEST_CHILD") != "1" {
		t.Skip("subprocess entry point")
	}
	if err := os.WriteFile(os.Getenv("KANDEV_WINJOB_TEST_PID"),
		[]byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Fprint(os.Stdout, os.Args[len(os.Args)-1])
	fmt.Fprint(os.Stderr, "child diagnostic")
	os.Exit(7)
}

func TestIsolatedNativeLaunchRejectsIncompleteArguments(t *testing.T) {
	if err := runIsolated([]string{"identity", "stdout", "stderr"}); err == nil {
		t.Fatal("incomplete launch unexpectedly succeeded")
	}
}
