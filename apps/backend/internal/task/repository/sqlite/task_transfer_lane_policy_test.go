package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestTransferTaskRejectsDifferentCurrentLanePolicies(t *testing.T) {
	for _, assignment := range []string{
		"profile_session_start_policy = 'fresh'",
		"profile_session_end_policy = 'stop'",
		"disable_unclassified_fallback = 1",
		"complete_task_on_enter = 1",
		`session_target = '{"kind":"primary"}'`,
	} {
		t.Run(assignment, func(t *testing.T) {
			repo, task := seedTaskTransferFixture(t)
			mustExecTransferTest(t, repo, `UPDATE workflow_steps SET `+assignment+` WHERE id = 'step-destination-work'`)
			if _, err := repo.TransferTask(context.Background(), taskTransferCommand(task)); !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
				t.Fatalf("different lane policy: %v, want conflict", err)
			}
		})
	}
}
