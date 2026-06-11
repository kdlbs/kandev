package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/office/costs/modelsdev"
)

// ModelInfoLookup resolves optional model metadata from the existing
// models.dev cache client.
type ModelInfoLookup interface {
	LookupModelInfo(ctx context.Context, modelID string) (modelsdev.ModelInfo, bool)
}

// SetModelInfoLookup wires optional models.dev metadata lookup for
// context-window fallback. Nil means ACP-only behavior.
func (s *Service) SetModelInfoLookup(lookup ModelInfoLookup) {
	s.modelInfoLookup = lookup
}
