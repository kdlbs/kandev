package review

import "errors"

// Error codes recorded on a failed run and surfaced to the client. The Review
// surface branches on these, so the strings are a contract — see the spec's
// Failure modes table.
const (
	CodeAgentUnavailable     = "review_agent_unavailable"
	CodeWorkspaceUnavailable = "review_workspace_unavailable"
	CodeNoChanges            = "review_no_changes"
	CodeUnparseableResponse  = "review_unparseable_response"
	CodeExecutionFailed      = "review_execution_failed"
	CodeCancelled            = "review_cancelled"
)

var (
	// ErrAgentUnavailable means no inference-capable agent and model could be
	// resolved for the run: nothing configured, no usable model, or the named
	// agent profile is CLI-passthrough only. Fails closed; never retried.
	ErrAgentUnavailable = errors.New(CodeAgentUnavailable)

	// ErrWorkspaceUnavailable means the task's workspace or agentctl could not
	// be reached, so the diff could not be read. Existing findings are left
	// untouched.
	ErrWorkspaceUnavailable = errors.New(CodeWorkspaceUnavailable)

	// ErrNoChanges means the task has no reviewable changed files. No run row is
	// created for this case.
	ErrNoChanges = errors.New(CodeNoChanges)

	// ErrUnparseableResponse means the reviewer replied with text containing no
	// recoverable findings array.
	ErrUnparseableResponse = errors.New(CodeUnparseableResponse)

	// ErrExecutionFailed wraps a transport or provider failure from the
	// inference call itself.
	ErrExecutionFailed = errors.New(CodeExecutionFailed)
)

// CodeFor maps an error to the run error code the client branches on.
func CodeFor(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrAgentUnavailable):
		return CodeAgentUnavailable
	case errors.Is(err, ErrWorkspaceUnavailable):
		return CodeWorkspaceUnavailable
	case errors.Is(err, ErrNoChanges):
		return CodeNoChanges
	case errors.Is(err, ErrUnparseableResponse):
		return CodeUnparseableResponse
	default:
		return CodeExecutionFailed
	}
}
