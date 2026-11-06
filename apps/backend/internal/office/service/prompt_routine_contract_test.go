package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/stretchr/testify/require"
)

func TestRoutinePromptPreservesWorkContract(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	routine := &models.Routine{
		WorkspaceID: "ws-1", Name: "Quality review", Status: "active",
		Description: "Review the current release against its evidence; save findings and proposal references in the existing handoff.",
	}
	require.NoError(t, svc.CreateRoutine(ctx, routine))
	snapshot := fmt.Sprintf(`{"routine_id":%q,"variables":{"target":"current release"}}`, routine.ID)
	for _, reason := range routineCatchUpReasons {
		t.Run(reason, func(t *testing.T) {
			pc := service.BuildPromptContextWithSnapshotForTest(svc, ctx, reason, "{}", snapshot)
			prompt := service.BuildPrompt(pc)
			require.Contains(t, prompt, routine.ID)
			require.Contains(t, prompt, routine.Name)
			require.Contains(t, prompt, routine.Description)
		})
	}
	pc := service.BuildPromptContextWithSnapshotForTest(svc, ctx, service.RunReasonTaskAssigned, "{}", snapshot)
	require.NotContains(t, service.BuildPrompt(pc), routine.Description)
}
