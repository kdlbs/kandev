package acp

import (
	"errors"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// ProjectSessionOperationFailure projects only the ACP authentication error
// code emitted by a supported session operation. Provider prose and generic
// JSON-RPC failures cannot establish this cause.
func ProjectSessionOperationFailure(err error, methods []streams.AuthMethodInfo) *types.SessionOperationFailure {
	var requestErr *acpsdk.RequestError
	if !errors.As(err, &requestErr) || requestErr.Code != -32000 {
		return nil
	}
	return types.NormalizeSessionOperationFailure(&types.SessionOperationFailure{
		Code:        types.SessionOperationFailureCodeAuthenticationRequired,
		AuthMethods: methods,
	})
}

// GetSessionOperationAuthMethods returns the initialize snapshot used by the
// synchronous session-operation failure response.
func (a *Adapter) GetSessionOperationAuthMethods() []streams.AuthMethodInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]streams.AuthMethodInfo(nil), a.availableAuthMethods...)
}
