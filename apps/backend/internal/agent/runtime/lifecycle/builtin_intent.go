package lifecycle

import (
	"context"
	builtinintent "github.com/kandev/kandev/internal/mcp/intent"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

func (e *AgentExecution) validateBuiltinIntent(ctx context.Context) error {
	if _, requested := builtinintent.FromContext(ctx); !requested {
		return nil
	}
	manifest := e.builtinMCPManifestSnapshot()
	if _, present := e.metadataValue(p.BuiltinBindingMetadataKey); present && manifest == nil {
		return builtinintent.ErrUnavailable
	}
	return builtinintent.ValidateExecution(ctx, e.TaskID, e.SessionID, e.ID, manifest)
}
