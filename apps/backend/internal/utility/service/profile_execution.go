package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
)

const maxProfileRouteAttempts = 8

type profileRouteResumer interface {
	ResumePendingRoute(context.Context, string, int64) (agentruntime.ProfileExecution, error)
}

// ExecuteProfilePrompt shares the bounded utility route across inference callers.
// The callback owns actual child execution, admission and settlement.
func ExecuteProfilePrompt(ctx context.Context, resolver ExecutionFailureResolver, route *PromptRequest,
	execute func(context.Context, string) (*agentctlutil.PromptResponse, error),
) (*agentctlutil.PromptResponse, error) {
	if route.ExecutionProfileID == "" {
		route.ExecutionProfileID = route.AgentProfileID
	}
	var response *agentctlutil.PromptResponse
	var executeErr error
	for attempt := 0; attempt < maxProfileRouteAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return response, err
		}
		identity := agentruntime.InferenceIdentityFromContext(ctx)
		identity.ExecutionProfileID = route.ExecutionProfileID
		response, executeErr = execute(agentruntime.WithInferenceIdentity(ctx, identity), route.ExecutionProfileID)
		if err := ctx.Err(); err != nil {
			return response, err
		}
		if agentruntime.IsInferenceCapacityError(executeErr) {
			return response, executeErr
		}
		if response != nil && (response.Success || strings.TrimSpace(response.Response) != "") {
			return response, executeErr
		}
		failure := profilePromptFailure(executeErr, response, route.AgentCLI)
		if !profileFallbackAllowed(resolver, route, failure) {
			return response, executeErr
		}
		cause := profilePromptError(response, executeErr)
		next, err := resolver.ResolveExecutionAfterFailure(ctx, route.RouteSessionID, route.AgentProfileID,
			route.ExecutionProfileID, route.RouteGeneration, failure)
		if attempt+1 == maxProfileRouteAttempts {
			return response, errors.Join(cause, err, errors.New("profile route attempt limit reached"))
		}
		if errors.Is(err, dynamic.ErrRecoveryPending) {
			next, err = awaitProfileRoute(ctx, resolver, next, err)
		}
		if err != nil {
			return response, errors.Join(cause, fmt.Errorf("resolve next execution profile: %w", err))
		}
		if next.ExecutionProfileID == "" {
			return response, errors.Join(cause, ErrExecutionRouting)
		}
		applyProfileRoute(route, next)
	}
	return response, executeErr
}

func profileFallbackAllowed(resolver ExecutionFailureResolver, route *PromptRequest, failure *routingerr.Error) bool {
	return resolver != nil && route.RouteSessionID != "" && route.AgentProfileID != route.ExecutionProfileID && failure.FallbackAllowed
}

func profilePromptFailure(err error, response *agentctlutil.PromptResponse, provider string) *routingerr.Error {
	if errors.Is(err, context.DeadlineExceeded) {
		// A live parent does not prove the child stopped: an HTTP transport
		// deadline can return while its provider operation remains active.
		// Preserve the original cause without authorizing another invocation.
		return &routingerr.Error{Code: routingerr.CodeUnknownProvider, Class: routingerr.ClassUnclassified,
			CatalogueVersion: routingerr.CatalogueVersion, Confidence: routingerr.ConfLow,
			Phase: routingerr.PhasePromptSend, ClassifierRule: "utility.unconfirmed_request_deadline",
			RawExcerpt: routingerr.Sanitize(err.Error())}
	}
	message := ""
	if response != nil {
		message = response.Error
	}
	if err != nil {
		message = err.Error()
	}
	return routingerr.Classify(routingerr.Input{Phase: routingerr.PhasePromptSend, ProviderID: provider, Stderr: message})
}

func profilePromptError(response *agentctlutil.PromptResponse, err error) error {
	if err != nil {
		return err
	}
	if response != nil && response.Error != "" {
		return errors.New(response.Error)
	}
	return errors.New("inference returned no successful result")
}

func applyProfileRoute(route *PromptRequest, next agentruntime.ProfileExecution) {
	route.ExecutionProfileID, route.RouteGeneration = next.ExecutionProfileID, next.Generation
	if next.Profile != nil {
		route.AgentCLI, route.Model = next.AgentName, next.Profile.Model
		if route.AgentCLI == "" {
			route.AgentCLI = next.Profile.AgentID
		}
	}
}

func awaitProfileRoute(ctx context.Context, resolver ExecutionFailureResolver, next agentruntime.ProfileExecution, pending error) (agentruntime.ProfileExecution, error) {
	resumer, ok := resolver.(profileRouteResumer)
	if !ok || next.Decision.Deadline == nil {
		return next, pending
	}
	timer := time.NewTimer(max(time.Until(*next.Decision.Deadline), 0))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return next, ctx.Err()
	case <-timer.C:
	}
	return resumer.ResumePendingRoute(ctx, next.RouteSessionID, next.Generation)
}
