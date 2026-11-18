package process

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"go.uber.org/zap"
)

func (m *Manager) startGeneration(
	ctx context.Context,
	preparation *types.ManagedRuntimePreparationDirective,
	prepared *managedruntime.PreparedRuntime,
	expectedRevision *uint64,
	expectedStopGeneration *uint64,
	expectedRepairContext *managedruntime.RepairContext,
	workspace bool,
) (uint64, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	release, err := m.admitStart()
	if err != nil {
		return 0, err
	}
	defer release()
	if workspace && m.cfg.ProviderMCP != nil {
		return 0, errors.New("scoped provider restart requires a new binding")
	}
	if err := m.validateProviderStart(prepared, expectedRevision, expectedStopGeneration); err != nil {
		return 0, err
	}
	// A scoped attempt consumes its prospective generation even when spawn fails.
	// A later retry must never reuse a failed scope's process generation.
	var reservedGeneration uint64
	if !workspace {
		reservedGeneration = m.beginManagedStartupGeneration()
	}
	prepared, err = m.selectPreparedProviderForStart(preparation, prepared, expectedRepairContext)
	if err != nil {
		return 0, err
	}

	// A previous lifecycle may still be live: an agent that exited on its own
	// never ran teardown, and Start is reachable without an intervening Stop.
	m.finishPreviousLifecycle(ctx)
	if prepared != nil {
		if preparation != nil {
			if _, err := m.validateManagedRuntimePreparationDirective(*preparation); err != nil {
				return 0, err
			}
		}
		if err := m.installPreparedRuntime(prepared); err != nil {
			return 0, err
		}
	}
	// Stop leaves every workspace tracker terminal. A retained manager that
	// starts a replacement agent must replace them before the process reads or
	// streams workspace state again.
	if workspace {
		m.renewWorkspaceTrackersForStart(ctx)
	}

	m.logger.Info("starting agent process",
		zap.String("protocol", string(m.cfg.Protocol)),
		zap.Strings("args", m.cfg.AgentArgs),
		zap.String("workdir", m.cfg.WorkDir),
		zap.Int("mcp_servers", len(m.cfg.McpServers)))

	m.status.Store(StatusStarting)
	m.exitCode.Store(-1)
	m.exitErr.Store(errorWrapper{err: nil})

	if err := config.ValidateCommandArgs(m.cfg.AgentArgs); err != nil {
		m.status.Store(StatusError)
		return 0, err
	}

	// Build adapter config and create protocol adapter
	if err := m.buildAdapterConfig(); err != nil {
		m.status.Store(StatusError)
		return 0, err
	}

	return m.startConfiguredProcess(workspace, reservedGeneration)
}

func (m *Manager) startManagedProcessWithWorkspace(workspace bool, reservedGeneration uint64) (uint64, error) {
	// Set up stdin/stdout/stderr pipes (must happen before process starts)
	if err := m.startProcessPipes(); err != nil {
		m.status.Store(StatusError)
		return 0, err
	}

	m.ClearStderrBuffer()
	// Start the subprocess now that pipes are connected
	if err := m.cmd.Start(); err != nil {
		_ = m.closeStderrPipe()
		m.status.Store(StatusError)
		if !m.cfg.LaunchWrapper.IsZero() {
			return 0, errors.New("wrapped agent process failed to start")
		}
		return 0, formatAgentStartError(err, m.cfg.AgentEnv)
	}
	processGeneration := reservedGeneration
	if processGeneration == 0 {
		processGeneration = m.beginManagedStartupGeneration()
	}
	m.watchOpenCodePluginReadiness()
	if err := m.closeStderrWriter(); err != nil {
		m.logger.Debug("failed to close parent stderr pipe", zap.Error(err))
	}
	processLifecycle, err := installProcessLifecycle(m.cmd)
	if err != nil {
		reapErr := killAndWaitStartedCommand(m.cmd)
		_ = m.closeStderrReader()
		m.status.Store(StatusError)
		return 0, errors.Join(fmt.Errorf("failed to install agent process lifecycle: %w", err), reapErr)
	}
	m.processLifecycle = processLifecycle

	m.initializeProviderChannels()

	// Connect adapter to the process stdin/stdout pipes
	if err := m.adapter.Connect(m.stdin, m.stdout); err != nil {
		reapErr, owned := m.reapMainProcessLifecycle()
		switch {
		case !owned:
			reapErr = killAndWaitStartedCommand(m.cmd)
		case reapErr != nil:
			fallbackErr := killAndWaitStartedCommand(m.cmd)
			reapErr = errors.Join(reapErr, fallbackErr)
		default:
			reapErr = killAndWaitStartedCommand(m.cmd)
		}
		_ = m.closeStderrReader()
		m.status.Store(StatusError)
		return 0, errors.Join(fmt.Errorf("failed to connect adapter: %w", err), reapErr)
	}

	m.recordAgentProcessIdentity(m.cmd.Process.Pid)

	// Start stderr reader and exit waiter. Keep the completion channel local to
	// this process generation so a delayed reader cannot signal a replacement.
	stderrDone := make(chan stderrReadResult, 1)
	m.wg.Add(2)
	go m.readStderr(stderrDone)
	go m.waitForExitGeneration(stderrDone, processGeneration)

	// Forward adapter updates to our channel
	m.wg.Add(1)
	go m.forwardUpdates(m.adapter, m.stopCh)

	m.startWorkspaceResources(workspace)

	m.status.Store(StatusRunning)
	m.logger.Info("agent process started", zap.Int("pid", m.cmd.Process.Pid))

	return processGeneration, nil
}

func (m *Manager) startOneShotWithWorkspace(workspace bool) error {
	m.cfg.AgentEnv = launchwrapper.StripControlCredentials(m.cfg.AgentEnv)
	if err := m.prepareOneShotArgs(); err != nil {
		m.status.Store(StatusError)
		return err
	}
	if m.adapterCfg != nil && m.adapterCfg.OneShotConfig != nil {
		m.adapterCfg.OneShotConfig.Env = m.cfg.AgentEnv
	}

	m.stopCh = make(chan struct{})
	m.stopChSnapshot.Store(m.stopCh)
	m.doneCh = make(chan struct{})
	m.stopChClosed.Store(false)
	m.shutdownIntent.Store(&atomic.Bool{})

	// Forward adapter updates to our channel
	m.wg.Add(1)
	go m.forwardUpdates(m.adapter, m.stopCh)

	m.startWorkspaceResources(workspace)

	m.status.Store(StatusRunning)
	m.logger.Info("one-shot adapter started (no persistent subprocess)")
	return nil
}

// validateProviderStart runs under startMu before any process or adapter changes.
func (m *Manager) validateProviderStart(prepared *managedruntime.PreparedRuntime, expectedRevision, expectedStopGeneration *uint64) error {
	if m.passthrough.Live() {
		return fmt.Errorf("passthrough agent is already running")
	}

	if m.Status() == StatusRunning || m.Status() == StatusStarting {
		return fmt.Errorf("agent is already running")
	}
	if expectedRevision != nil && m.configRevision != *expectedRevision {
		if prepared != nil {
			_ = prepared.Close()
		}
		return errors.New("managed runtime configuration changed during recovery")
	}
	if expectedStopGeneration != nil && m.stopGeneration != *expectedStopGeneration {
		if prepared != nil {
			_ = prepared.Close()
		}
		return errors.New("managed runtime start was retired by a concurrent stop")
	}
	return nil
}

func (m *Manager) startWorkspaceResources(workspace bool) {
	if !workspace {
		return
	}
	root, trackers := m.snapshotTrackers()
	root.Start(context.Background())
	for _, tracker := range trackers {
		tracker.Start(context.Background())
	}
	m.startAgentShell()
}

func (m *Manager) selectPreparedProviderForStart(preparation *types.ManagedRuntimePreparationDirective, prepared *managedruntime.PreparedRuntime, expectedRepairContext *managedruntime.RepairContext) (*managedruntime.PreparedRuntime, error) {
	if prepared != nil && preparation == nil {
		if expectedRepairContext == nil || m.repairStore == nil ||
			!m.repairStore.MatchesRepairContext(prepared, *expectedRepairContext) {
			if err := prepared.Close(); err != nil {
				m.retainPreparedRuntime(prepared)
			}
			m.detachPreparedRuntimeLocked()
			prepared = nil
		}
	}
	if m.cfg.DurableJournalPath != "" && m.deliveryJournalErr != nil {
		m.status.Store(StatusError)
		return nil, fmt.Errorf("durable delivery journal unavailable: %w", m.deliveryJournalErr)
	}
	// Only an admitted stopped-process start may release an incompatible
	// selected or pending handle. Every earlier rejection keeps the current
	// process, publication ticket, deadline, and ledger ownership intact.
	if prepared == nil && preparation == nil {
		m.detachPreparedRuntimeLocked()
	}

	return prepared, nil
}

func (m *Manager) initializeProviderChannels() {
	m.stopCh = make(chan struct{})
	m.stopChSnapshot.Store(m.stopCh)
	m.doneCh = make(chan struct{})
	m.stopChClosed.Store(false)
	m.shutdownIntent.Store(&atomic.Bool{})

}
