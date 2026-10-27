package lifecycle

import (
	"context"
	"errors"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

// SandboxAttempt is the retained core owner of a single subprocess launch.
type SandboxAttempt interface {
	Launch() *contract.Launch
	ValidateCommand([]string, map[string]string) error
	MarkStart(context.Context) error
	BindOwner(context.Context, string, *contract.ProcessOwner) error
	ApplyStartReceipt(context.Context, string, *contract.StartReceipt) error
	Close() error
}

type SandboxPrepare func(context.Context, *AgentExecution, uint64, string, map[string]string, *client.Client) (SandboxAttempt, error)

func (m *Manager) SetSandboxRuntime(prepare SandboxPrepare) { m.sandboxPrepare = prepare }

func (m *Manager) configureAndStartProcess(ctx context.Context, execution *AgentExecution, transport *client.Client, environment map[string]string) (string, error) {
	attempt, err := m.prepareExecutionSandbox(ctx, execution, environment, transport)
	if err != nil {
		return "", err
	}
	if transport == nil {
		return "", errors.New("agentctl is unavailable")
	}
	if err = m.captureExecutionSnapshot(ctx, execution, "chat", execution.AgentArgs, execution.LaunchWrapper, attempt); err != nil {
		return "", err
	}
	if attempt == nil {
		if err = transport.ConfigureAgent(ctx, execution.AgentCommand, execution.AgentArgs, environment, execution.ContinueCommand, execution.ContinueArgs, execution.LaunchWrapper); err != nil {
			return "", err
		}
		return transport.Start(ctx)
	}
	if err = attempt.ValidateCommand(execution.AgentArgs, environment); err != nil {
		return "", err
	}
	if err = transport.ConfigureAgentWithSandbox(ctx, execution.AgentCommand, execution.AgentArgs, environment, execution.ContinueCommand, execution.ContinueArgs, attempt.Launch(), execution.LaunchWrapper); err != nil {
		return "", err
	}
	if err = execution.markSandboxStart(ctx, attempt); err != nil {
		return "", err
	}
	command, err := transport.Start(ctx)
	if err != nil {
		return "", errors.Join(err, resolveFailedSandboxStart(ctx, execution.ID, transport, attempt))
	}
	if err = bindExecutionSandbox(ctx, execution.ID, transport, attempt); err != nil {
		return "", errors.Join(err, resolveFailedSandboxStart(ctx, execution.ID, transport, attempt))
	}
	return command, nil
}

func (m *Manager) prepareExecutionSandbox(ctx context.Context, execution *AgentExecution, environment map[string]string, transport *client.Client) (SandboxAttempt, error) {
	execution.sandboxMu.Lock()
	defer execution.sandboxMu.Unlock()
	if execution.SandboxSelection == nil {
		return nil, nil
	}
	if execution.sandboxAttempt != nil {
		if !execution.sandboxDispatched {
			return execution.sandboxAttempt, nil
		}
		if err := execution.sandboxAttempt.Close(); err != nil {
			return nil, err
		}
		execution.sandboxPreviousAttempt = execution.sandboxAttempt.Launch().AttemptID
		execution.sandboxAttempt = nil
	}
	if m.sandboxPrepare == nil {
		return nil, errors.New("sandbox runtime is unavailable")
	}
	execution.sandboxGeneration++
	attempt, err := m.sandboxPrepare(ctx, execution, execution.sandboxGeneration, execution.sandboxPreviousAttempt, environment, transport)
	if err != nil {
		return nil, err
	}
	if attempt == nil {
		return nil, errors.New("selected sandbox is unavailable")
	}
	execution.sandboxAttempt = attempt
	execution.sandboxDispatched = false
	return attempt, nil
}

func bindExecutionSandbox(parent context.Context, executionID string, transport *client.Client, attempt SandboxAttempt) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	launch := attempt.Launch()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		owner, err := transport.SandboxOwner(ctx, launch.AttemptID, launch.Generation, launch.PolicyDigest)
		if err == nil && attempt.BindOwner(ctx, executionID, owner) == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("sandbox process ownership could not be verified")
		case <-tick.C:
		}
	}
}

func (execution *AgentExecution) closeSandbox() error {
	execution.sandboxMu.Lock()
	defer execution.sandboxMu.Unlock()
	if execution.sandboxAttempt == nil {
		return nil
	}
	if err := execution.sandboxAttempt.Close(); err != nil {
		return err
	}
	execution.sandboxPreviousAttempt = execution.sandboxAttempt.Launch().AttemptID
	execution.sandboxAttempt = nil
	return nil
}

func (execution *AgentExecution) markSandboxStart(ctx context.Context, attempt SandboxAttempt) error {
	execution.sandboxMu.Lock()
	defer execution.sandboxMu.Unlock()
	if execution.sandboxDispatched {
		return errors.New("sandbox launch was already dispatched")
	}
	if err := attempt.MarkStart(ctx); err != nil {
		return err
	}
	execution.sandboxDispatched = true
	return nil
}

func resolveFailedSandboxStart(parent context.Context, executionID string, transport *client.Client, attempt SandboxAttempt) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Second)
	defer cancel()
	launch := attempt.Launch()
	receipt, err := transport.SandboxStartReceipt(ctx, launch)
	if err == nil && receipt != nil {
		return attempt.ApplyStartReceipt(ctx, executionID, receipt)
	}
	owner, ownerErr := transport.SandboxOwner(ctx, launch.AttemptID, launch.Generation, launch.PolicyDigest)
	if ownerErr == nil {
		return attempt.BindOwner(ctx, executionID, owner)
	}
	return errors.New("sandbox startup completion could not be verified")
}
