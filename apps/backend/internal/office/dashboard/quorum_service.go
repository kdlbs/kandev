package dashboard

import "context"

// GetTaskQuorum is the AC-24b/24c/54 read-only diagnostic entry point
// backing GET /tasks/:id/quorum. Per AC-24b, a step with no guarded
// transition returns an empty list, never an error; per AC-24c, so does a
// task with no bound workflow_step_id. An engine dispatcher that hasn't
// been wired with quorum-evaluation support gets the same treatment: a
// diagnostic read must never fail on the state it exists to diagnose.
func (s *DashboardService) GetTaskQuorum(ctx context.Context, taskID string) (QuorumResponseDTO, error) {
	qd, ok := s.engineDispatcher.(quorumEvaluatingDispatcher)
	if !ok {
		return QuorumResponseDTO{Guards: []GuardStateDTO{}}, nil
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
