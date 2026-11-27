package lifecycle

import (
	"context"
	"errors"
	"time"
)

var errReadinessClientDetached = errors.New("agentctl readiness client is detached")

// Readiness retries do not retain a client lease between health probes.
// Cleanup can detach an unavailable runtime without waiting for startup to expire.
func (ae *AgentExecution) waitForAgentctlReady(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := ae.probeAgentctlHealth(ctx); err == nil {
				return nil
			} else if errors.Is(err, errReadinessClientDetached) {
				return err
			}
		}
	}
}

func (ae *AgentExecution) probeAgentctlHealth(ctx context.Context) error {
	client, release := ae.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return errReadinessClientDetached
	}
	probeCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return client.Health(probeCtx)
}
