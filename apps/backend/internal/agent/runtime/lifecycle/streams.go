package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// StreamCallbacks defines callbacks for stream events
type StreamCallbacks struct {
	OnAgentEvent       func(execution *AgentExecution, event agentctl.AgentEvent)
	OnStreamDisconnect func(execution *AgentExecution, err error, promptGeneration uint64)
	// Generation-aware callbacks are used for startup replacement streams. The
	// lifecycle manager owns the completion signal and callback lease for this
	// path; the legacy callbacks remain available to isolated callers and tests.
	OnAgentEventWithGeneration       func(execution *AgentExecution, event agentctl.AgentEvent, startupGeneration uint64)
	OnStreamDisconnectWithGeneration func(execution *AgentExecution, err error, promptGeneration, startupGeneration uint64)
	OnGitStatus                      func(execution *AgentExecution, update *agentctl.GitStatusUpdate)
	OnGitCommit                      func(execution *AgentExecution, commit *agentctl.GitCommitNotification)
	OnGitReset                       func(execution *AgentExecution, reset *agentctl.GitResetNotification)
	OnBranchSwitch                   func(execution *AgentExecution, branchSwitch *agentctl.GitBranchSwitchNotification)
	OnFileChange                     func(execution *AgentExecution, notification *agentctl.FileChangeNotification)
	OnShellOutput                    func(execution *AgentExecution, data string)
	OnShellExit                      func(execution *AgentExecution, code int)
	OnProcessOutput                  func(execution *AgentExecution, output *agentctl.ProcessOutput)
	OnProcessStatus                  func(execution *AgentExecution, status *agentctl.ProcessStatusUpdate)
}

// StreamManager manages WebSocket streams to agent executions
type StreamManager struct {
	builtinActionResolver          BuiltinActionResolver
	logger                         *logger.Logger
	callbacks                      StreamCallbacks
	reconciliationMu               sync.Mutex
	deliveryReconciliations        map[string]*deliveryReconciliationCycle
	stoppedReconciliations         map[string]struct{}
	reconciliationContext          context.Context
	reconciliationCancel           context.CancelFunc
	reconciliationNow              func() time.Time
	reconciliationWait             func(context.Context, time.Duration) error
	deliveryHealthClock            func() time.Time
	deliveryHealthWait             func(context.Context, time.Duration) error
	deliveryHealthProbeInterval    time.Duration
	deliveryHealthProbeTimeout     time.Duration
	deliveryHealthRecoveryCooldown time.Duration
	runtimeStorageProbeInterval    time.Duration
	runtimeStorageSlots            chan struct{}
	runtimeStorageAdvisories       sync.Map
	onRuntimeStorageProbe          func(context.Context, *AgentExecution, RuntimeStorageProbeResult)
	onRuntimeStoragePressure       func(context.Context, *AgentExecution, RuntimeStoragePressure)
	onRuntimeStorageDetached       func(*AgentExecution, *deliveryStreamAttachment)
	deliveryAttachments            sync.Map
	isExecutionCurrent             func(*AgentExecution) bool
	onDeliveryReconciliationPhase  func(*AgentExecution, DeliveryReconciliationIdentity, DeliveryReconciliationPhase)
	deliverySubmissionSettler      func(context.Context, *AgentExecution, DeliveryReconciliationIdentity, *journal.Submission) (bool, error)
	deliveryMu                     sync.RWMutex
	delivery                       AgentDeliveryRepository
	ackMu                          sync.Mutex
	ackWorkers                     map[string]*durableDeliveryAckWorker
	ackStopped                     bool
	ackWG                          sync.WaitGroup
	mcpMu                          sync.RWMutex
	mcpHandler                     agentctl.MCPHandler
	// mcpIdentityScoper scopes in-session MCP dispatches to the owner of the
	// stream's task. Nil leaves dispatch unscoped (single-user instances and
	// isolated tests); set via Manager.SetMCPIdentityScoper.
	mcpIdentityScoper MCPIdentityScoper
	// mcpPrincipalScoper attaches the trusted task/session/surface principal.
	mcpPrincipalScoper MCPPrincipalScoper
	// stopCh is the Manager-owned shutdown signal. The retry/backoff and
	// connected `<-ws.Done() / <-stop>` select read from it so they drain on
	// Manager.Stop. May be nil when isolated tests don't care about external
	// shutdown; waitCh below covers Wait-driven drains in that case.
	stopCh <-chan struct{}
	// waitCh is closed by Wait() so retry/backoff and the connected select
	// drain even when the external stopCh isn't closed by the caller (or is
	// nil). Together with stopCh this makes Wait an absolute drain barrier,
	// which goleak.VerifyTestMain depends on under CI load.
	waitCh     chan struct{}
	waitChOnce sync.Once
	wg         sync.WaitGroup
	wgMu       sync.Mutex
	stopped    bool
}

// stopChannelContext wraps a parent ctx with two auxiliary stop channels.
// Done() returns a per-instance merged channel that closes when any of
// parent.Done(), primary or secondary fires. The merge goroutine spawned by
// Done() exits as soon as any signal fires, and Wait()'s waitCh close
// guarantees that happens at teardown time.
//
// We keep merge spawn behind a sync.Once so repeated Done() calls (the runtime
// re-asks every select tick) don't pile up goroutines. The optional wg field
// lets a StreamManager track the merge goroutine so sm.wg.Wait remains a true
// drain barrier even when the outer stream goroutine returns first (the
// connectUpdatesStream path returns immediately after the dial, so without
// this the merge goroutine could outlive sm.wg.Wait and trip goleak).
type stopChannelContext struct {
	context.Context
	primary   <-chan struct{}
	secondary <-chan struct{}

	wg *sync.WaitGroup

	once   sync.Once
	merged chan struct{}
}

func (c *stopChannelContext) Done() <-chan struct{} {
	if c.primary == nil && c.secondary == nil {
		return c.Context.Done()
	}
	c.once.Do(func() {
		c.merged = make(chan struct{})
		if c.wg != nil {
			c.wg.Add(1)
		}
		go c.mergeStops()
	})
	return c.merged
}

func (c *stopChannelContext) mergeStops() {
	defer close(c.merged)
	if c.wg != nil {
		defer c.wg.Done()
	}
	switch {
	case c.primary != nil && c.secondary != nil:
		select {
		case <-c.primary:
		case <-c.secondary:
		case <-c.Context.Done():
		}
	case c.primary != nil:
		select {
		case <-c.primary:
		case <-c.Context.Done():
		}
	default:
		select {
		case <-c.secondary:
		case <-c.Context.Done():
		}
	}
}

func (c *stopChannelContext) Err() error {
	if c.primary != nil {
		select {
		case <-c.primary:
			return context.Canceled
		default:
		}
	}
	if c.secondary != nil {
		select {
		case <-c.secondary:
			return context.Canceled
		default:
		}
	}
	return c.Context.Err()
}

// NewStreamManager creates a new StreamManager.
//
// stopCh is the Manager-owned shutdown signal used by the workspace-stream
// retry backoff to drain cleanly. Pass nil from tests that exercise the
// manager in isolation; production callers wire it from Manager.stopCh.
// Either way, Wait() closes a per-StreamManager internal channel that the
// same drain sites observe — so Wait remains an absolute drain barrier.
func NewStreamManager(log *logger.Logger, callbacks StreamCallbacks, mcpHandler agentctl.MCPHandler, stopCh <-chan struct{}) *StreamManager {
	reconciliationContext, reconciliationCancel := context.WithCancel(context.Background())
	return &StreamManager{
		logger:                         log.WithFields(zap.String("component", "stream-manager")),
		callbacks:                      callbacks,
		mcpHandler:                     mcpHandler,
		stopCh:                         stopCh,
		waitCh:                         make(chan struct{}),
		ackWorkers:                     make(map[string]*durableDeliveryAckWorker),
		deliveryReconciliations:        make(map[string]*deliveryReconciliationCycle),
		stoppedReconciliations:         make(map[string]struct{}),
		reconciliationContext:          reconciliationContext,
		reconciliationCancel:           reconciliationCancel,
		reconciliationNow:              time.Now,
		reconciliationWait:             waitForReconciliation,
		deliveryHealthClock:            time.Now,
		deliveryHealthProbeInterval:    defaultDeliveryHealthProbeInterval,
		deliveryHealthProbeTimeout:     defaultDeliveryHealthProbeTimeout,
		deliveryHealthRecoveryCooldown: defaultDeliveryHealthCooldown,
		runtimeStorageSlots:            make(chan struct{}, defaultRuntimeStorageConcurrency),
	}
}

// ConnectAll connects to all streams for an execution.
// If ready is non-nil, it is closed when the updates stream connection attempt
// completes (success or failure). Agent operations require the updates stream;
// workspace stream readiness is handled independently.
func (sm *StreamManager) ConnectAll(execution *AgentExecution, ready chan<- struct{}) {
	sm.connectUpdatesStreamAsync(execution, ready)
	sm.ConnectWorkspaceStream(execution, nil)
}

// ConnectAllAndWait waits until the updates stream connection attempt has
// finished and verifies the stream is attached before callers admit prompts.
// The stream itself keeps the execution and manager lifetime, not this waiter's
// shorter recovery-attempt context.
func (sm *StreamManager) ConnectAllAndWait(ctx context.Context, execution *AgentExecution) error {
	if sm == nil || execution == nil {
		return errors.New("stream manager or execution is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if client, releaseClient := execution.AcquireAgentCtlClient(); client != nil {
		connected := client.HasAgentStream()
		releaseClient()
		if connected {
			return nil
		}
	} else {
		return errors.New("agentctl client is unavailable")
	}

	ready := make(chan struct{})
	sm.ConnectAll(execution, ready)
	select {
	case <-ready:
	case <-ctx.Done():
		return ctx.Err()
	case <-sm.waitCh:
		return errors.New("stream manager stopped before updates stream connected")
	case <-sm.stopCh:
		return errors.New("stream manager stopped before updates stream connected")
	}

	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		return errors.New("agentctl client became unavailable before updates stream connected")
	}
	connected := client.HasAgentStream()
	releaseClient()
	if !connected {
		if err := execution.deliveryStreamFailureSnapshot(); err != nil {
			return err
		}
		return errors.New("agent updates stream did not connect")
	}
	return nil
}

func (sm *StreamManager) connectUpdatesStreamAsync(execution *AgentExecution, ready chan<- struct{}) {
	if !sm.start(func() {
		sm.connectUpdatesStream(execution, ready)
	}) && ready != nil {
		close(ready)
	}
}

// ConnectWorkspaceStream starts the workspace stream and tracks the goroutine
// so shutdown and tests can wait for it to drain after stopCh closes.
func (sm *StreamManager) ConnectWorkspaceStream(execution *AgentExecution, ready chan<- struct{}) {
	if !sm.start(func() {
		sm.connectWorkspaceStream(execution, ready)
	}) && ready != nil {
		close(ready)
	}
}

// ConnectMCPStream opens the passthrough MCP proxy stream under goroutine
// tracking so it drains cleanly on shutdown (mirrors ConnectWorkspaceStream).
func (sm *StreamManager) ConnectMCPStream(execution *AgentExecution) {
	sm.start(func() {
		sm.connectMCPStream(execution)
	})
}

// Wait blocks until all StreamManager-owned stream goroutines have exited.
// Closes the internal waitCh first so any goroutine still parked in the retry
// backoff or the connected `<-ws.Done() / <-stop>` select drains without
// depending on the caller having closed the external stopCh.
func (sm *StreamManager) Wait() {
	sm.wgMu.Lock()
	sm.stopped = true
	sm.wgMu.Unlock()
	sm.reconciliationCancel()
	sm.cancelAllDeliveryReconciliations()
	sm.waitChOnce.Do(func() { close(sm.waitCh) })
	sm.wg.Wait()
	sm.ackMu.Lock()
	sm.ackStopped = true
	for _, worker := range sm.ackWorkers {
		worker.cancel()
	}
	sm.ackMu.Unlock()
	sm.ackWG.Wait()
}

func waitForReconciliation(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (sm *StreamManager) start(fn func()) bool {
	sm.wgMu.Lock()
	defer sm.wgMu.Unlock()
	if sm.stopped {
		return false
	}
	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()
		fn()
	}()
	return true
}

// ReconnectAll reconnects to all streams (used after backend restart).
// This waits for agentctl to be ready before connecting to streams.
func (sm *StreamManager) ReconnectAll(execution *AgentExecution) {
	sm.logger.Debug("reconnecting to agent streams after recovery",
		zap.String("instance_id", execution.ID),
		zap.String("task_id", execution.TaskID))

	// Wait a moment for any startup operations to settle. Selecting on
	// stopCh lets shutdown drain this goroutine without burning the full
	// 500ms when the manager is already stopping.
	if !sm.sleepOrStop(500 * time.Millisecond) {
		return
	}

	// Check if agentctl is responsive
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		sm.logger.Warn("agentctl unavailable for stream reconnection", zap.String("instance_id", execution.ID))
		return
	}
	err := client.WaitForReady(ctx, 10*time.Second)
	releaseClient()
	if err != nil {
		sm.logger.Warn("agentctl not ready for stream reconnection",
			zap.String("instance_id", execution.ID),
			zap.Error(err))
		// Don't return - still try to connect to streams
	}

	// Reconnect to WebSocket streams
	sm.ConnectAll(execution, nil)

	sm.logger.Debug("agent streams reconnected",
		zap.String("instance_id", execution.ID),
		zap.String("task_id", execution.TaskID))
}

// sleepOrStop blocks for d or until the Manager begins shutting down.
// Returns true when the timer fires, false when either the external stopCh
// or the internal Wait-driven waitCh fires first.
func (sm *StreamManager) sleepOrStop(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	if sm.stopCh == nil {
		select {
		case <-timer.C:
			return true
		case <-sm.waitCh:
			return false
		}
	}
	select {
	case <-timer.C:
		return true
	case <-sm.stopCh:
		return false
	case <-sm.waitCh:
		return false
	}
}

// streamContext preserves the execution's session trace values while making
// in-flight WebSocket dials cancellable by either the external Manager
// shutdown signal or StreamManager.Wait's internal drain signal.
func (sm *StreamManager) streamContext(execution *AgentExecution) context.Context {
	return &stopChannelContext{
		Context:   execution.SessionTraceContext(),
		primary:   sm.stopCh,
		secondary: sm.waitCh,
		wg:        &sm.wg,
	}
}

// connectUpdatesStream handles the updates WebSocket stream with ready signaling
func (sm *StreamManager) connectUpdatesStream(execution *AgentExecution, ready chan<- struct{}) {
	ctx := sm.streamContext(execution)
	startupGeneration, attempt := execution.deliveryStreamStartupSnapshot()
	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		if ready != nil {
			close(ready)
		}
		return
	}
	delivery := sm.deliveryRepository()
	streamID := execution.DeliveryStreamID
	if streamID == "" {
		streamID = execution.SessionID
	}
	after, cursorErr := sm.initialDeliveryReplayCursor(ctx, execution, delivery, client, streamID)
	execution.recordDeliveryStreamFailure(startupGeneration, cursorErr)
	if cursorErr != nil {
		sm.logger.Error("failed to load durable delivery replay cursor",
			zap.String("instance_id", execution.ID),
			zap.String("stream_id", streamID),
			zap.Error(cursorErr))
		releaseClient()
		if ready != nil {
			close(ready)
		}
		return
	}

	if execution.DeliveryMode == DurableDeliveryV1 && after > 0 {
		sm.scheduleDurableDeliveryAck(execution, client, agentctl.AgentEvent{DeliveryStreamID: streamID, DeliverySequence: after})
	}

	// Bind this connection to the startup attempt captured before the replay
	// RPC. Reconnects and replacement attempts must never complete this gate.
	processor := newStreamEventProcessor(sm, ctx, execution, client, delivery, startupGeneration, attempt)
	monitorCtx, monitorCancel := context.WithCancel(ctx)
	attachment := sm.startDeliveryHealthMonitor(monitorCtx, monitorCancel, execution, client)
	// Processor failure cancels only this connection, including when no later
	// frame arrives. Disconnect handling reports the original processing error.
	err := client.StreamUpdatesFromWithOwner(processor.ctx, func(event agentctl.AgentEvent) {
		processor.enqueue(event)
	}, sm.mcpHandlerFor(execution), func(disconnectErr error) {
		attachment.close()
		monitorCancel()
		processErr := processor.close()
		if processErr != nil {
			sm.logger.Error("durable agent event processing failed",
				zap.String("instance_id", execution.ID), zap.Error(processErr))
			if disconnectErr == nil {
				disconnectErr = processErr
			} else {
				disconnectErr = errors.Join(disconnectErr, processErr)
			}
		}
		if disconnectErr != nil {
			if errors.Is(disconnectErr, errDeliveryReplayReconciled) {
				sm.reconnectAfterExpiredDeliveryReplay(execution)
				return
			}
			overloaded := errors.Is(disconnectErr, agentctl.ErrAgentEventQueueFull) ||
				errors.Is(disconnectErr, errStreamEventProcessorQueueFull)
			if overloaded && execution.DeliveryMode != DurableDeliveryV1 {
				disconnectErr = fmt.Errorf("%w: %v", ErrUncertainPromptDelivery, disconnectErr)
				sm.handleUpdatesDisconnectWithGeneration(execution, disconnectErr, startupGeneration)
				return
			}
			if sm.shouldReconnectAfterStreamOverload(execution, disconnectErr) {
				return
			}
			if execution.DeliveryMode == DurableDeliveryV1 && execution.deliverySubmissionIDSnapshot() != "" {
				result := sm.reconcileDisconnectedSubmission(ctx, execution)
				switch result.Outcome {
				case DeliveryReconciliationRunningAttached, DeliveryReconciliationTerminalSettled,
					DeliveryReconciliationOwnerMismatch:
					return
				default:
					disconnectErr = errors.Join(ErrUncertainPromptDelivery, result.Err)
				}
			}
			sm.handleUpdatesDisconnectWithGeneration(execution, disconnectErr, startupGeneration)
		}
	}, after, func(ownerID uint64) {
		processor.progress.ownerID = ownerID
		stage, ok := client.UpdateStreamStage()
		if ok && stage.OwnerID == ownerID {
			execution.installStreamProgress(processor.progress)
		}
	})
	execution.recordDeliveryStreamFailure(startupGeneration, err)
	if err != nil {
		attachment.close()
		monitorCancel()
	}
	releaseClient()

	// Signal that the stream connection attempt is complete (success or failure)
	// StreamUpdates returns immediately after establishing the WebSocket connection
	// and starting the read goroutine, so this signals that we're ready to receive updates
	if ready != nil {
		close(ready)
	}

	if err != nil {
		processor.fail(err)
		_ = processor.close()
		sm.logger.Error("failed to connect to updates stream",
			zap.String("instance_id", execution.ID),
			zap.Error(err))
	}
}

func (sm *StreamManager) reconnectAfterExpiredDeliveryReplay(execution *AgentExecution) {
	if execution == nil || execution.DeliveryMode != DurableDeliveryV1 {
		return
	}
	if !sm.start(func() {
		if sm.sleepOrStop(20 * time.Millisecond) {
			sm.connectUpdatesStream(execution, nil)
		}
	}) {
		sm.logger.Debug("skipping expired delivery replay reconnect during shutdown",
			zap.String("execution_id", execution.ID))
	}
}

// shouldReconnectAfterStreamOverload handles bounded queue pressure without
// converting a recoverable durable stream into a failed prompt. The next
// connection replays from the SQL projected cursor. Legacy streams have no
// retained cursor contract, so their overflow is classified as uncertain.
func (sm *StreamManager) shouldReconnectAfterStreamOverload(
	execution *AgentExecution,
	err error,
) bool {
	if execution == nil {
		return false
	}
	readerOverflow := errors.Is(err, agentctl.ErrAgentEventQueueFull)
	processorOverflow := errors.Is(err, errStreamEventProcessorQueueFull)
	if !readerOverflow && !processorOverflow {
		return false
	}
	if execution.DeliveryMode != DurableDeliveryV1 {
		return false
	}
	if !sm.start(func() {
		if !sm.sleepOrStop(20 * time.Millisecond) {
			return
		}
		sm.connectUpdatesStream(execution, nil)
	}) {
		sm.logger.Debug("skipping durable stream overload reconnect during shutdown",
			zap.String("execution_id", execution.ID))
	}
	return true
}

func (sm *StreamManager) handleUpdatesDisconnect(execution *AgentExecution, disconnectErr error) {
	sm.handleUpdatesDisconnectWithGeneration(execution, disconnectErr, execution.startupAttemptSnapshot())
}

func (sm *StreamManager) handleUpdatesDisconnectWithGeneration(
	execution *AgentExecution,
	disconnectErr error,
	startupGeneration uint64,
) {
	promptGeneration := execution.promptGenerationSnapshot()
	if sm.callbacks.OnStreamDisconnectWithGeneration != nil {
		// The lifecycle callback holds the startup lease across both the signal
		// and disconnect mutation. Signalling here first would leave a window in
		// which replacement startup could advance the generation before the old
		// callback reached its validation guard.
		sm.callbacks.OnStreamDisconnectWithGeneration(execution, disconnectErr, promptGeneration, startupGeneration)
		return
	}
	uncertainSubmissionID := execution.deliverySubmissionIDSnapshot()
	uncertain := uncertainSubmissionID != "" || errors.Is(disconnectErr, ErrUncertainPromptDelivery)
	signalError := "agent stream disconnected: " + disconnectErr.Error()
	if uncertain {
		signalError = fmt.Sprintf(
			"%s: %s; reconcile submission %q before retrying",
			ErrUncertainPromptDelivery,
			disconnectErr,
			uncertainSubmissionID,
		)
	}
	if !execution.signalPromptCompletionForStartupGeneration(
		startupGeneration,
		PromptCompletionSignal{
			IsError:          true,
			Uncertain:        uncertain,
			Error:            signalError,
			PromptGeneration: promptGeneration,
		},
	) {
		sm.logger.Debug("ignoring stale updates stream disconnect",
			zap.String("execution_id", execution.ID),
			zap.Uint64("stream_startup_generation", startupGeneration),
			zap.Uint64("current_startup_generation", execution.startupAttemptSnapshot()))
		return
	}
	if sm.callbacks.OnStreamDisconnect != nil {
		sm.callbacks.OnStreamDisconnect(execution, disconnectErr, promptGeneration)
	}
}

// applyCanonicalCompactionResult attaches committed count/reporting metadata to
// the live notification. It never recomputes the count: the projector is the
// only authority, and a lost publish is repaired by normal hydration.
func applyCanonicalCompactionResult(
	event *agentctl.AgentEvent,
	result models.CanonicalAgentDeliveryProjectionResult,
) {
	applyCanonicalCompactionMetadata(event, result.ContextCompactionCount, result.ContextCompactionReporting)
}

// applyCanonicalCompactionMetadata attaches committed count/reporting metadata
// to the live notification. It never recomputes the count: the projector is
// the only authority, and a lost publish is repaired by normal hydration.
func applyCanonicalCompactionMetadata(
	event *agentctl.AgentEvent,
	count *int64,
	reporting *streams.ContextCompactionReporting,
) {
	if event == nil || (count == nil && reporting == nil) {
		return
	}
	if event.Data == nil {
		event.Data = make(map[string]any)
	}
	if count != nil {
		event.Data[models.SessionMetaKeyContextCompactionCount] = *count
	}
	if reporting != nil {
		event.Data[models.SessionMetaKeyContextCompactionReporting] = reporting
	}
}
