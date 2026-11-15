package streams

import "github.com/kandev/kandev/internal/agent/runtime/routingerr"

// RoutingInput retains the safe diagnostic and its structured signals together.
// agentID is the classifier catalogue key, not a model-provider/account ID.
// Legacy message-only errors remain valid inputs; incomplete diagnostics never
// lend their metadata authority to an unrelated terminal message.
func (e *ProviderError) RoutingInput(phase routingerr.Phase, agentID, message string) routingerr.Input {
	in := routingerr.Input{Phase: phase, ProviderID: agentID, Stderr: message}
	if e == nil {
		return in
	}
	if e.Message != "" {
		in.Stderr = e.Message
	}
	in.ResetHint, in.OccurredAt = e.ResetAt, e.OccurredAt
	if e.Valid() {
		in.HTTPStatus, in.ACPCode = e.HTTPStatus, e.RPCCode
		in.ErrorKind, in.ProviderCode = e.ErrorKind, e.ProviderCode
	}
	return in
}
