//go:build !windows

package launcher

import (
	"os/exec"
	"strings"
	"testing"
)

func TestSetupLivenessPipe_SetsExtraFiles(t *testing.T) {
	cmd := exec.Command("true")
	cmd.Env = []string{}

	pipeWrite, err := setupLivenessPipe(cmd)
	if err != nil {
		t.Fatalf("setupLivenessPipe: %v", err)
	}
	defer func() { _ = pipeWrite.Close() }()

	if pipeWrite == nil {
		t.Fatal("expected non-nil write-end")
	}
	if len(cmd.ExtraFiles) != 1 {
		t.Fatalf("expected 1 ExtraFile, got %d", len(cmd.ExtraFiles))
	}
	defer func() { _ = cmd.ExtraFiles[0].Close() }()

	found := false
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "KANDEV_PARENT_PIPE_FD=") {
			found = true
			if env != "KANDEV_PARENT_PIPE_FD=3" {
				t.Errorf("expected KANDEV_PARENT_PIPE_FD=3, got %s", env)
			}
		}
	}
	if !found {
		t.Error("KANDEV_PARENT_PIPE_FD not set in cmd.Env")
	}
}

func TestSetupLivenessPipe_PipeIsReadable(t *testing.T) {
	cmd := exec.Command("true")
	cmd.Env = []string{}

	pipeWrite, err := setupLivenessPipe(cmd)
	if err != nil {
		t.Fatalf("setupLivenessPipe: %v", err)
	}

	pipeRead := cmd.ExtraFiles[0]

	// Close write-end; read should return immediately (EOF).
	_ = pipeWrite.Close()

	buf := make([]byte, 1)
	_, err = pipeRead.Read(buf)
	_ = pipeRead.Close()
	if err == nil {
		t.Error("expected error (EOF) after closing write-end")
	}
}
