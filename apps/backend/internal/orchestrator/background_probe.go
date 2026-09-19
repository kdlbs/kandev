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
// Check session access before the call reaches the executor, lifecycle
// manager, or agent control chain.
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
