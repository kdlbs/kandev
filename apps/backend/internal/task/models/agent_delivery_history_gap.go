package models

import "fmt"

// AgentDeliveryHistoryGap records unavailable output independently of the
// contiguous receipt and projection cursors. It never authorizes prompt replay.
type AgentDeliveryHistoryGap struct {
	FirstSequence     int64 `json:"first_sequence"`
	LastSequence      int64 `json:"last_sequence"`
	ReceivedSequence  int64 `json:"received_sequence"`
	ProjectedSequence int64 `json:"projected_sequence"`
}

const SessionMetaKeyAgentDeliveryHistoryGaps = "agent_delivery_history_gaps"

// AgentDeliveryHistoryGapRecord remains in session metadata when a subsequent
// prompt replaces the current recovery view.
type AgentDeliveryHistoryGapRecord struct {
	StreamID          string                  `json:"stream_id"`
	IncarnationID     string                  `json:"incarnation_id"`
	HarnessGeneration int64                   `json:"harness_generation"`
	SubmissionID      string                  `json:"submission_id"`
	PayloadHash       string                  `json:"payload_hash"`
	NativeSessionID   string                  `json:"native_session_id"`
	Gap               AgentDeliveryHistoryGap `json:"gap"`
}

// MatchesPrunedStream permits only a fully acknowledged, empty retained
// stream whose earlier SQL projection is complete. Unacknowledged output and
// locally received but unprojected events must be replayed first.
func (g AgentDeliveryHistoryGap) MatchesPrunedStream(received, projected, highWater, acknowledged, firstRetained int64) bool {
	return received >= 0 && received == projected && received < highWater && highWater < 1<<63-1 &&
		highWater == acknowledged && firstRetained == highWater+1 &&
		g.FirstSequence == received+1 && g.LastSequence == highWater &&
		g.ReceivedSequence == received && g.ProjectedSequence == projected
}

func SameAgentDeliveryHistoryGap(a, b *AgentDeliveryHistoryGap) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

const AgentDeliveryHistoryGapFailureCode = "delivery_history_gap"

// AgentDeliveryHistoryGapDetails keeps startup diagnostics and exact restored
// error matching on the same stream/range representation.
func AgentDeliveryHistoryGapDetails(streamID string, first, last int64) string {
	return fmt.Sprintf("stream %s, missing event range %d..%d", streamID, first, last)
}
