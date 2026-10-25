package hostutility

import (
	"context"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/pkg/agent"
)

func (m *Manager) getInstance(ctx context.Context, agentType string, sandboxed ...bool) (*instance, agents.InferenceAgent, error) {
	if err := m.reconcileRuntimeEpoch(ctx); err != nil {
		return nil, nil, err
	}
	ia, ok := m.registry.GetInferenceAgent(agentType)
	if !ok {
		return nil, nil, fmt.Errorf("agent %q not found or not inference-capable", agentType)
	}
	ag, ok := ia.(agents.Agent)
	if !ok || !ag.Enabled() {
		return nil, nil, fmt.Errorf("agent %q is not a full agent type", agentType)
	}
	if rt := ag.Runtime(); rt == nil || (rt.Protocol != agent.ProtocolACP && rt.Protocol != agent.ProtocolCodexAppServer) {
		return nil, nil, fmt.Errorf("agent %q is not ACP-capable", agentType)
	}

	m.mu.RLock()
	if m.stopped {
		m.mu.RUnlock()
		return nil, nil, errManagerStopped
	}
	inst := m.instances[agentType]
	parentTmpDir := m.parentTmpDir
	m.mu.RUnlock()
	if inst != nil {
		if err := m.staleInstanceCause(ctx, inst); err == nil {
			return inst, ia, nil
		} else if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
	}

	if parentTmpDir == "" {
		return nil, nil, errors.New("host utility manager not started")
	}

	// Serialize instance creation per agent type via singleflight so two
	// concurrent callers don't both spawn a process and then race to cache
	// the result.
	v, err, _ := m.createGroup.Do(agentType, func() (interface{}, error) {
		return m.createOrReuseUtilityInstance(ctx, agentType, ag, sandboxed)
	})
	if err != nil {
		return nil, nil, err
	}
	return v.(*instance), ia, nil
}

func (m *Manager) createOrReuseUtilityInstance(ctx context.Context, agentType string, ag agents.Agent, sandboxed []bool) (*instance, error) {
	// Double-check inside the singleflight window.
	m.mu.RLock()
	existing := m.instances[agentType]
	m.mu.RUnlock()
	if existing != nil {
		if err := m.staleInstanceCause(ctx, existing); err == nil {
			return existing, nil
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		} else {
			m.dropStaleInstance(ctx, agentType, existing, err)
		}
	}
	// Pre-check installation so Refresh surfaces `not_installed`
	// instead of collapsing it into `failed` via createInstance errors.
	if err := precheckHostInstallation(ctx, ag, sandboxed); err != nil {
		return nil, err
	}
	created, cerr := m.createInstance(ctx, agentType)
	if cerr != nil {
		return nil, cerr
	}
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		deleteCtx, cancel := hostUtilityDeleteContext(ctx)
		m.deleteInstance(deleteCtx, created)
		cancel()
		return nil, errManagerStopped
	}
	m.instances[agentType] = created
	m.mu.Unlock()
	return created, nil
}

// Selected sandbox discovery must verify installation inside its owned attempt.
func precheckHostInstallation(ctx context.Context, ag agents.Agent, sandboxed []bool) error {
	if len(sandboxed) > 0 && sandboxed[0] {
		return nil
	}
	discovery, err := ag.IsInstalled(ctx)
	if err != nil || discovery == nil || !discovery.Available {
		return errAgentNotInstalled
	}
	return nil
}
