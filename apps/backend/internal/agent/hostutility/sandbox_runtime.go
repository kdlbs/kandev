package hostutility

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/agent/runtime"
	"slices"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/sandbox"
)

type SandboxAttempt interface {
	Launch() *sandbox.Launch
	ValidateCommand([]string, map[string]string) error
	MarkStart(context.Context) error
	BindOwner(context.Context, string, *sandbox.ProcessOwner) error
	ApplyStartReceipt(context.Context, string, *sandbox.StartReceipt) error
	Close() error
}

type SandboxPrepare func(context.Context, *ProfileProbeContext, *utility.InferenceConfigDTO, *runtime.AgentCtlClient) (SandboxAttempt, error)

func (m *Manager) SetSandboxRuntime(prepare SandboxPrepare) { m.sandboxPrepare = prepare }

func (m *Manager) runSandboxUtility(ctx context.Context, inst *instance, profile *ProfileProbeContext, cfg *utility.InferenceConfigDTO, dispatch func(context.Context) error) (err error) {
	if profile == nil || profile.SandboxSelection == nil {
		if cfg != nil && cfg.Sandbox != nil {
			return sandbox.ErrInvalidPolicy
		}
		return dispatch(ctx)
	}
	if m.sandboxPrepare == nil || cfg == nil {
		return errors.New("sandbox runtime is unavailable")
	}
	attempt, err := m.sandboxPrepare(ctx, profile, cfg, inst.client)
	if err != nil {
		return err
	}
	if attempt == nil {
		return errors.New("selected sandbox is unavailable")
	}
	dispatched := false
	defer func() { err = errors.Join(err, closeSandboxUtility(ctx, inst, attempt, dispatched)) }()
	if err = attempt.ValidateCommand(append(slices.Clone(cfg.Command), cfg.CLIFlags...), cfg.Env); err != nil {
		return err
	}
	cfg.Sandbox = attempt.Launch()
	if err = inst.client.ValidateSandboxLaunch(ctx, cfg.Sandbox); err != nil {
		return err
	}
	if err = attempt.MarkStart(ctx); err != nil {
		return err
	}
	dispatched = true
	return inst.client.WithSandboxOwner(ctx, cfg.Sandbox, func(ctx context.Context, owner *sandbox.ProcessOwner) error {
		return attempt.BindOwner(ctx, inst.instanceID, owner)
	}, dispatch)
}

func closeSandboxUtility(parent context.Context, inst *instance, attempt SandboxAttempt, dispatched bool) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Second)
	defer cancel()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		closeErr := attempt.Close()
		if closeErr == nil {
			return nil
		}
		if dispatched {
			receipt, err := inst.client.UtilitySandboxReceipt(ctx, attempt.Launch())
			if err != nil {
				return errors.Join(closeErr, err)
			}
			if receipt != nil {
				if err = attempt.ApplyStartReceipt(ctx, inst.instanceID, receipt); err != nil {
					return errors.Join(closeErr, err)
				}
				return attempt.Close()
			}
		}
		select {
		case <-ctx.Done():
			return closeErr
		case <-tick.C:
		}
	}
}
