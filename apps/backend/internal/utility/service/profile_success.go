package service

import (
	"context"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// profileChainCompleter is an optional producer hook owned by the ordered-tier
// selection feature. When the shared resolver does not implement it, a verified
// success simply leaves the selection chain untouched.
type profileChainCompleter interface {
	OrderedTiersEnabled() bool
	ClearSelectionChain(context.Context, string, int64, string) error
}

// A completed current result ends the failure chain, independently of a parent
// task's runtime session. Partial output and an empty response do not attest it.
func completeProfileSelection(ctx context.Context, resolver ExecutionFailureResolver, route *PromptRequest,
	response *agentctlutil.PromptResponse, executeErr error,
) {
	if executeErr != nil || !response.Success || strings.TrimSpace(response.Response) == "" || route.RouteSessionID == "" || route.DeferSelectionCompletion {
		return
	}
	CompleteVerifiedProfileSelection(ctx, resolver, route)
}

// CompleteVerifiedProfileSelection ends a chain only after the caller accepts
// the current output or effects. It never invokes another provider.
func CompleteVerifiedProfileSelection(ctx context.Context, resolver ExecutionFailureResolver, route *PromptRequest) {
	if route == nil || route.RouteSessionID == "" {
		return
	}
	complete, ok := resolver.(profileChainCompleter)
	if !ok || !complete.OrderedTiersEnabled() {
		return
	}
	err := complete.ClearSelectionChain(ctx, route.RouteSessionID, route.RouteGeneration, route.ExecutionProfileID)
	if err != nil && !errors.Is(err, dynamic.ErrStaleGeneration) && !errors.Is(err, dynamic.ErrRouteStateNotFound) {
		// Match the native task success hook: retain the actual result and
		// diagnose storage failure without another provider invocation.
		logger.Default().Debug("could not end dynamic selection chain after inference success", zap.Error(err))
	}
}

func (s *Service) OrderedTiersEnabled() bool {
	complete, ok := s.executionResolver.(profileChainCompleter)
	return ok && complete.OrderedTiersEnabled()
}

func (s *Service) ClearSelectionChain(ctx context.Context, sessionID string, generation int64, candidateID string) error {
	complete, ok := s.executionResolver.(profileChainCompleter)
	if !ok || !complete.OrderedTiersEnabled() {
		return nil
	}
	return complete.ClearSelectionChain(ctx, sessionID, generation, candidateID)
}
