package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

// A recoverable agent failure must attach its recovery entry to the turn that
// failed (marking it error-terminated) instead of completing the turn and then
// lazily opening a second empty turn to hold the recovery message. Otherwise
// the frontend shows the "agent finished without producing any output" notice
// twice: once for the empty failed turn and once for the empty recovery turn.
func TestHandleRecoverableFailure_AttachesRecoveryToFailedTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	const failedTurnID = "failed-turn-s1"
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID:            failedTurnID,
		TaskSessionID: "s1",
		TaskID:        "t1",
		StartedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
	}
	svc, _ := newAgentErrorTestService(t, repo, stepGetter, nil)
	svc.turnService = &repoBackedTurnService{repo: repo}
	mc := &mockMessageCreator{}
	svc.messageCreator = mc

	svc.handleRecoverableFailureLocked(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom",
	})

	// The failed turn is marked error-terminated so its completion reports
	// had_output=true (verified in the task-service package).
	turn, err := repo.GetTurn(ctx, failedTurnID)
	require.NoError(t, err)
	if terminated, _ := turn.Metadata[models.TurnMetaKeyErrorTerminated].(bool); !terminated {
		t.Fatalf("failed turn metadata = %#v, want error_terminated=true", turn.Metadata)
	}

	// The recovery message attaches to the failed turn, not a lazily-started
	// second turn.
	require.Len(t, mc.sessionMessages, 1)
	if got := mc.sessionMessages[0].turnID; got != failedTurnID {
		t.Fatalf("recovery message turnID = %q, want %q (must attach to the failed turn)", got, failedTurnID)
	}
}
