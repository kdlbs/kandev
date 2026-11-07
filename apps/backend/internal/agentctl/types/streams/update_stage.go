package streams

const UpdateStreamStageAction = "agent.stream.stage"

// UpdateStreamStageHeartbeat is transport metadata, not an agent turn event.
// Ages are measured at the agentctl writer so clocks need not be synchronized.
type UpdateStreamStageHeartbeat struct {
	AdapterSeen      uint64 `json:"adapter_seen"`
	AdapterAgeMillis int64  `json:"adapter_age_ms"`
	WriterSent       uint64 `json:"writer_sent"`
	WriterAgeMillis  int64  `json:"writer_age_ms"`
}
