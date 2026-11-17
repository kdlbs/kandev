package runtime

import (
	"context"
	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

type ProviderMCPControl = lifecycle.ProviderMCPControl

// ProviderMCPReplacement is optional. Unsupported runtimes keep settings pending.
type ProviderMCPReplacement interface {
	WithProviderMCPReplacement(context.Context, string, string, func(ProviderMCPControl) error) error
}

var ErrProviderMCPApplicationPending = lifecycle.ErrProviderMCPApplicationPending

var ErrProviderReplacementUnsupported = client.ErrProviderReplacementUnsupported
