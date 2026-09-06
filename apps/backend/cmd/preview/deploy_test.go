package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunDeployAcceptsSkipDescription(t *testing.T) {
	t.Setenv("SPRITES_API_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")

	oldStderr := os.Stderr
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = writePipe
	t.Cleanup(func() {
		os.Stderr = oldStderr
		_ = readPipe.Close()
		_ = writePipe.Close()
	})

	code := runDeploy(context.Background(), []string{
		"--pr", "123",
		"--repo", "owner/repo",
		"--skip-description",
	})
	if err := writePipe.Close(); err != nil {
		t.Fatalf("close stderr pipe: %v", err)
	}
	output, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	if code != 2 {
		t.Fatalf("runDeploy exit code = %d, want missing token validation (2)", code)
	}
	if strings.Contains(string(output), "flag provided but not defined") {
		t.Fatalf("skip-description was rejected as an unknown flag: %s", output)
	}
	if !strings.Contains(string(output), "SPRITES_API_TOKEN is required") {
		t.Fatalf("runDeploy output = %q, want missing token validation", output)
	}
}
