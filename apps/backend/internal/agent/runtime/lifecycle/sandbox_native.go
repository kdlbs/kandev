package lifecycle

import (
	"context"
	"errors"
	"time"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

type sandboxNativeReceipt interface {
	SandboxStartReceipt(string) (*contract.StartReceipt, error)
}

type sandboxNativeOwner interface {
	SandboxOwner(string) (*contract.ProcessOwner, error)
}

func (m *Manager) startSandboxInteractive(ctx context.Context, execution *AgentExecution, runner executionPassthroughRunner, request process.InteractiveStartRequest) (*process.InteractiveProcessInfo, error) {
	if execution.SandboxSelection == nil {
		if err := m.captureExecutionSnapshot(ctx, execution, "native_terminal", request.Command, request.LaunchWrapper, nil); err != nil {
			return nil, err
		}
		return runner.Start(ctx, request)
	}
	ownership, ok := runner.(sandboxNativeOwner)
	if !ok {
		return nil, errors.New("terminal transport cannot verify sandbox ownership")
	}
	transport, release := execution.AcquireAgentCtlClient()
	defer release()
	if transport == nil {
		return nil, errors.New("sandbox control transport is unavailable")
	}
	attempt, err := m.prepareExecutionSandbox(ctx, execution, request.Env, transport)
	if err != nil {
		return nil, err
	}
	if err = attempt.ValidateCommand(request.Command, request.Env); err != nil {
		return nil, err
	}
	if err = m.captureExecutionSnapshot(ctx, execution, "native_terminal", request.Command, request.LaunchWrapper, attempt); err != nil {
		return nil, err
	}
	request.Sandbox = attempt.Launch()
	request.LogCommand = []string{"sandboxed CLI", "<arguments omitted>"}
	if err = transport.ConfigureAgentWithSandbox(ctx, "", request.Command, request.Env, "", nil, request.Sandbox, request.LaunchWrapper); err != nil {
		return nil, err
	}
	if err = execution.markSandboxStart(ctx, attempt); err != nil {
		return nil, err
	}
	info, err := runner.Start(ctx, request)
	if err != nil {
		var failure *contract.StartFailure
		if errors.As(err, &failure) {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			defer cancel()
			return nil, errors.Join(err, attempt.ApplyStartReceipt(cleanup, execution.ID, failure.Receipt))
		}
		return nil, err
	}
	if err = bindNativeSandbox(ctx, execution.ID, info.ID, ownership, transport, attempt); err != nil {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		return nil, errors.Join(err, runner.Stop(stopCtx, info.ID), attempt.Close())
	}
	return info, nil
}

func bindNativeSandbox(parent context.Context, executionID, processID string, ownership sandboxNativeOwner, transport *client.Client, attempt SandboxAttempt) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		owner, err := ownership.SandboxOwner(processID)
		if err == nil && attempt.BindOwner(ctx, executionID, owner) == nil {
			return transport.SetSandboxOwner(ctx, owner)
		}
		if receipts, ok := ownership.(sandboxNativeReceipt); ok {
			if receipt, receiptErr := receipts.SandboxStartReceipt(attempt.Launch().AttemptID); receiptErr == nil {
				return errors.Join(errors.New("native sandbox exited before readiness"), attempt.ApplyStartReceipt(ctx, executionID, receipt))
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("native sandbox process ownership could not be verified")
		case <-tick.C:
		}
	}
}

func (m *Manager) sandboxMCPRoute(ctx context.Context, execution *AgentExecution, automatic string) (string, error) {
	if execution.SandboxSelection == nil {
		return automatic, nil
	}
	transport, release := execution.AcquireAgentCtlClient()
	defer release()
	attempt, err := m.prepareExecutionSandbox(ctx, execution, execution.RuntimeEnvironment(), transport)
	if err != nil {
		return "", err
	}
	return attempt.Launch().MCPURL, nil
}
