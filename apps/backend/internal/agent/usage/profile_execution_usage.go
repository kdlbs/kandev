package usage

// ProfileExecutionUsage identifies an actual, already tracked execution. An
// absent execution is unknown; a profile definition is not an account source.
type ProfileExecutionUsage struct {
	ExecutionID string                `json:"execution_id"`
	SessionID   string                `json:"session_id"`
	Report      *ExecutionUsageReport `json:"report"`
}
