//go:build windows

package launcher

import (
	"os/exec"
	"testing"
)

func TestSetupLivenessPipe_NoopOnWindows(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo", "test")
	cmd.Env = []string{}

	pipeWrite, err := setupLivenessPipe(cmd)
	if err != nil {
		t.Fatalf("setupLivenessPipe: %v", err)
	}
	if pipeWrite != nil {
		t.Error("expected nil write-end on Windows")
	}
	if len(cmd.ExtraFiles) != 0 {
		t.Errorf("expected no ExtraFiles on Windows, got %d", len(cmd.ExtraFiles))
	}
}
