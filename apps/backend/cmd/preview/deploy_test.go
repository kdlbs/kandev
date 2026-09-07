package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDeployRejectsMissingExplicitArtifactBeforeCredentials(t *testing.T) {
	t.Setenv("SPRITES_API_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	artifact := filepath.Join(t.TempDir(), "missing.tar.gz")
	stderr, restoreStderr := capturePreviewStderr(t)

	code := runDeploy(context.Background(), []string{
		"--pr", "3456",
		"--repo", "kdlbs/kandev",
		"--artifact", artifact,
		"--skip-description",
	})
	restoreStderr()

	if code != 2 {
		t.Fatalf("runDeploy() = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "prebuilt artifact") {
		t.Fatalf("stderr = %q, want a prebuilt artifact error", stderr.String())
	}
}

func TestDeployArtifactsDoesNotBuildExistingArtifact(t *testing.T) {
	artifact := filepath.Join(t.TempDir(), "preview.tar.gz")
	if err := os.WriteFile(artifact, []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}

	built := false
	deployed := false
	got, err := deployArtifactsWithHooks(
		context.Background(), artifact, "test-token", "kandev-pr-3456", 3000, true, false,
		func(context.Context, string, string, bool, bool) error {
			built = true
			return nil
		},
		func(_ context.Context, tarPath, _, _ string, _ int) (string, error) {
			deployed = true
			if tarPath != artifact {
				t.Fatalf("deployed artifact = %q, want %q", tarPath, artifact)
			}
			return "https://preview.example", nil
		},
	)
	if err != nil {
		t.Fatalf("deployArtifactsWithHooks() error = %v", err)
	}
	if got != "https://preview.example" {
		t.Fatalf("deployArtifactsWithHooks() = %q, want preview URL", got)
	}
	if built {
		t.Fatal("deployArtifactsWithHooks() built an existing artifact")
	}
	if !deployed {
		t.Fatal("deployArtifactsWithHooks() did not deploy the existing artifact")
	}
}

func TestDeployArtifactsBuildsWhenArtifactIsNotProvided(t *testing.T) {
	artifact := filepath.Join(t.TempDir(), "preview.tar.gz")

	built := false
	got, err := deployArtifactsWithHooks(
		context.Background(), artifact, "test-token", "kandev-pr-3456", 3000, true, true,
		func(_ context.Context, _, tarPath string, _, _ bool) error {
			built = true
			return os.WriteFile(tarPath, []byte("bundle"), 0o600)
		},
		func(_ context.Context, tarPath, _, _ string, _ int) (string, error) {
			if tarPath != artifact {
				t.Fatalf("deployed artifact = %q, want %q", tarPath, artifact)
			}
			return "https://preview.example", nil
		},
	)
	if err != nil {
		t.Fatalf("deployArtifactsWithHooks() error = %v", err)
	}
	if got != "https://preview.example" {
		t.Fatalf("deployArtifactsWithHooks() = %q, want preview URL", got)
	}
	if !built {
		t.Fatal("deployArtifactsWithHooks() did not build the missing artifact")
	}
}

func TestDeployArtifactsRejectsMissingArtifactWhenBuildDisabled(t *testing.T) {
	artifact := filepath.Join(t.TempDir(), "preview.tar.gz")
	built := false
	deployed := false

	_, err := deployArtifactsWithHooks(
		context.Background(), artifact, "test-token", "kandev-pr-3456", 3000, true, false,
		func(context.Context, string, string, bool, bool) error {
			built = true
			return nil
		},
		func(context.Context, string, string, string, int) (string, error) {
			deployed = true
			return "", nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "prebuilt artifact") {
		t.Fatalf("deployArtifactsWithHooks() error = %v, want missing prebuilt artifact error", err)
	}
	if built {
		t.Fatal("deployArtifactsWithHooks() built an artifact when build was disabled")
	}
	if deployed {
		t.Fatal("deployArtifactsWithHooks() deployed a missing artifact")
	}
}

func capturePreviewStderr(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()

	oldStderr := os.Stderr
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stderr = writeEnd

	var output bytes.Buffer
	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&output, readEnd)
		close(copyDone)
	}()

	restore := func() {
		_ = writeEnd.Close()
		<-copyDone
		_ = readEnd.Close()
		os.Stderr = oldStderr
	}
	t.Cleanup(restore)

	return &output, restore
}
