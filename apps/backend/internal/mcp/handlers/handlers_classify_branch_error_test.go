package handlers

import (
	"errors"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestClassifyAddBranchError_WorktreeAdoptionExecutorIsValidation(t *testing.T) {
	err := errors.New(`worktree adoption is only supported on host Worktree and Local executors; this task uses "docker"`)
	if got := classifyAddBranchError(err); got != ws.ErrorCodeValidation {
		t.Errorf("expected ErrorCodeValidation, got %q", got)
	}
}
