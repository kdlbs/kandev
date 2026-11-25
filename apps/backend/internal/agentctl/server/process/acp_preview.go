package process

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"

	wire "github.com/coder/acp-go-sdk/experimental/v2"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/pkg/agent"
	"go.uber.org/zap"
)

func (m *Manager) requestedACPProtocolVersion() uint16 {
	if m.cfg.PreferACPV2 && m.cfg.Protocol == agent.ProtocolACP &&
		m.cfg.ProviderGatewayAuth == nil && m.cfg.AuthPolicy == nil {
		return 2
	}
	return 0
}

func (m *Manager) ACPProtocolVersion() uint16 {
	return adapter.ACPProtocolVersion(m.GetAdapter())
}

func (m *Manager) publishProcessExitError(event adapter.AgentEvent) {
	if owner, ok := m.adapter.(interface{ OwnsPromptDisconnect() bool }); ok &&
		(owner.OwnsPromptDisconnect() || m.activeDeliverySubmissionID() != "") {
		return
	}
	m.recordTerminalOutcome(&event)
	persisted, err := m.persistDeliveryEvent(event)
	if err != nil {
		m.logger.Error("failed to commit durable exit error event", zap.Error(err))
		return
	}
	m.sendUpdateBlockingRecorded(persisted)
}

// InitializeAdapter allows one fresh ACP v1 connection after a validated
// compatibility result. Session traffic can only see the resulting adapter.
func (m *Manager) InitializeAdapter(ctx context.Context, adpt adapter.AgentAdapter, generation uint64) (adapter.AgentAdapter, uint64, error) {
	err := adpt.Initialize(ctx)
	var legacy *wire.LegacyPeerError
	if !errors.As(err, &legacy) {
		return adpt, generation, err
	}
	replacement, current, err := m.restartACPWithV1(ctx, adpt, generation)
	if err != nil {
		return adpt, current, err
	}
	if err := replacement.Initialize(ctx); err != nil {
		return replacement, current, err
	}
	version := ""
	if info := replacement.GetAgentInfo(); info != nil {
		version = info.Version
	}
	m.logger.Info("ACP initialized with fallback", zap.String("agent_id", m.cfg.AgentType),
		zap.String("agent_version", version), zap.Uint16("requested_version", 2),
		zap.Uint16("selected_version", 1), zap.String("fallback_reason", "validated_legacy_peer"))
	return replacement, current, nil
}

func (m *Manager) restartACPWithV1(
	ctx context.Context, original adapter.AgentAdapter, generation uint64,
) (adapter.AgentAdapter, uint64, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, generation, err
	}
	release, err := m.admitStart()
	if err != nil {
		return nil, generation, err
	}
	defer release()
	current := m.ProcessGeneration()
	if m.GetAdapter() != original || (generation != 0 && generation != current) ||
		m.stopInProgress.Load() > 0 || m.adapterCfg.ACPProtocolVersion != 2 || m.cmd == nil {
		return nil, generation, errors.New("ACP v2 process changed before fallback")
	}
	// Reuse the exact resolved command, including any managed runtime rewrite.
	command := exec.Command(m.cmd.Path, m.cmd.Args[1:]...)
	command.Args = slices.Clone(m.cmd.Args)
	command.Env, command.Dir = slices.Clone(m.cmd.Env), m.cmd.Dir
	setAgentProcGroup(command)
	if err := m.retireACPPreviewProcess(ctx); err != nil {
		return nil, current, fmt.Errorf("retire ACP v2 process before fallback: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, current, err
	}
	m.adapterCfg.ACPProtocolVersion = 1
	if err := m.createAdapter(); err != nil {
		return nil, current, err
	}
	m.cmd = command
	m.status.Store(StatusStarting)
	m.exitCode.Store(-1)
	m.exitErr.Store(errorWrapper{})
	current, err = m.startManagedProcess()
	return m.GetAdapter(), current, err
}

func (m *Manager) retireACPPreviewProcess(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := m.Status()
	if status != StatusStopped {
		m.status.Store(StatusStopping)
	}
	m.closeAdapterAndStdin()
	if status == StatusStopped && !m.mainReapPending.Load() {
		done := make(chan struct{})
		go func() {
			m.wg.Wait()
			close(done)
		}()
		if !m.waitForManagerDone(ctx, done, m.processExitGrace(ctx)) {
			return errors.New("ACP v2 process workers did not stop")
		}
		return nil
	}
	err := m.waitForProcessExit(ctx)
	m.mainReapPending.Store(err != nil)
	m.status.Store(StatusStopped)
	return err
}
