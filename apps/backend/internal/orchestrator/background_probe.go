package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrator/executor"
)

// BackgroundProbe is the port task-05's parked-projection uses to sample an
// agent's background-workload liveness (spec
// docs/specs/disambiguate-waiting/spec.md, §"Probe port (backend)"). The
// production implementation is Service.ProbeBackgroundWorkloads; test
// doubles are a scripted sequence of the three literals so projection tests
// never depend on the transport or the real process walk.
type BackgroundProbe interface {
	Probe(ctx context.Context, sessionID string) (executor.ProbeResult, error)
}

// ProbeBackgroundWorkloads is Service's production BackgroundProbe
// implementation.
//
// F6 (round-5, security). The spec's original claim that this path
// inherits an authorization boundary from RespondToPermissionBySessionID's
// precedent is false in the tree — that call has no guard
// (RespondToPermissionBySessionID never calls CheckSessionAccess). This
// applies an explicit authorizeSession check before the call ever reaches
// the executor/lifecycle.Manager/agentctl chain, rather than assuming one
// is inherited.
//
// The configured probe budget bounds the call via context.WithTimeout —
// applied here, around this call, never baked into the transport itself.
func (s *Service) ProbeBackgroundWorkloads(ctx context.Context, sessionID string) (executor.ProbeResult, error) {
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return executor.ProbeResultUnknown, err
	}

	budget := s.backgroundProbeConfig.Budget
	if budget <= 0 {
		budget = defaultParkedProbeBudget
	}
	probeCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	if s.executor == nil {
		return executor.ProbeResultUnknown, nil
	}
	return s.executor.ProbeBackgroundWorkloads(probeCtx, sessionID)
}
