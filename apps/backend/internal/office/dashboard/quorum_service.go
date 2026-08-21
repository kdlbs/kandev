package dashboard

import (
	"context"
	"fmt"
)

// GetTaskQuorum is the AC-24b/24c/54 read-only diagnostic entry point
// backing GET /workspaces/:wsId/tasks/:taskId/quorum. Per AC-24b, a step with no guarded
// transition returns an empty list, never an error; per AC-24c, so does a
// task with no bound workflow_step_id. Per AC-57c/57d, an engine dispatcher
// that hasn't been wired with quorum-evaluation support is a different case
// entirely: the read side carries the same no-fallback rule as the write
// side (decisionRecordingDispatcher below), and errors rather than
// reporting a healthy empty result.
func (s *DashboardService) GetTaskQuorum(ctx context.Context, taskID string) (QuorumResponseDTO, error) {
	qd, ok := s.engineDispatcher.(quorumEvaluatingDispatcher)
	if !ok {
		return QuorumResponseDTO{}, fmt.Errorf("%s", decisionStoreNotWiredErr)
	}
	snapshot, err := qd.EvaluateStepQuorum(ctx, taskID)
	if err != nil {
		return QuorumResponseDTO{}, err
	}
	return QuorumResponseDTO{
		Guards:              guardStateDTOsFromSnapshot(snapshot.Guards),
		ReevaluationBlocked: snapshot.ReevaluationBlocked,
	}, nil
}
