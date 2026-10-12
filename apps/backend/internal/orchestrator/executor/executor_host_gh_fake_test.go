package executor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// The copied test executable acts as gh without a shell or host credentials.
	if filepath.Base(os.Args[0]) == "gh.exe" {
		os.Exit(runHostGHFakeProcess(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func runHostGHFakeProcess(args []string) int {
	if len(args) < 2 || args[0] != "auth" {
		return 2
	}
	switch args[1] {
	case "token":
		return 0
	case "git-credential":
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			return 2
		}
		if _, err := fmt.Fprint(os.Stdout, "username=x-access-token\npassword=host-token\n"); err != nil {
			return 2
		}
		return 0
	default:
		return 2
	}
}

func writeHostGHFakeExecutable(t *testing.T, dir string) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	source, err := os.Open(executable)
	if err != nil {
		t.Fatalf("open test executable: %v", err)
	}
	defer func() {
		if err := source.Close(); err != nil {
			t.Errorf("close test executable: %v", err)
		}
	}()

	ghPath := filepath.Join(dir, "gh.exe")
	destination, err := os.OpenFile(ghPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o700)
	if err != nil {
		t.Fatalf("create fake gh: %v", err)
	}
	t.Cleanup(func() { _ = destination.Close() })
	if _, err := io.Copy(destination, source); err != nil {
		t.Fatalf("copy test executable: %v", err)
	}
	if err := destination.Close(); err != nil {
		t.Fatalf("close fake gh: %v", err)
	}
	return ghPath
}
