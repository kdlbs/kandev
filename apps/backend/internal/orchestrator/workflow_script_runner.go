package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"go.uber.org/zap"

	"github.com/google/uuid"
)

const (
	workflowScriptOutputBufferBytes = 2 * 1024 * 1024
	workflowScriptMessageBatch      = 200 * time.Millisecond
)

var (
	errWorkflowScriptInterrupted = errors.New("workflow script interrupted")
	errWorkflowScriptMessage     = errors.New("workflow script message projection failed")
)

type workflowScriptRunStore interface {
	taskrepo.WorkflowScriptRunRepository
}

type workflowScriptMessageWriter interface {
	CreateWorkflowScriptMessage(ctx context.Context, messageID, taskID, sessionID, content string, metadata map[string]interface{}) error
	UpdateWorkflowScriptMessage(ctx context.Context, messageID, content string, metadata map[string]interface{}) error
}

type workflowScriptExecutionRequest struct {
	TaskID           string
	WorkflowID       string
	WorkflowStepID   string
	WorkflowStepName string
	Trigger          taskmodels.WorkflowScriptRunTrigger
	ActionPosition   int
	OccurrenceID     string
	SessionID        string
	ExecutionID      string
	Action           wfmodels.WorkflowScriptAction
}

// WorkflowScriptBlockedError identifies the durable run that prevented a
// workflow callback from continuing. Callers use errors.As to keep the
// transition gate separate from the process and message implementations.
type WorkflowScriptBlockedError struct {
	RunID     string
	MessageID string
	Status    taskmodels.WorkflowScriptRunStatus
	Cause     error
}

func (e *WorkflowScriptBlockedError) Error() string {
	if e == nil {
		return "workflow script blocked workflow operation"
	}
	if e.Cause == nil {
		return fmt.Sprintf("workflow script %s blocked workflow operation", e.RunID)
	}
	return fmt.Sprintf("workflow script %s blocked workflow operation: %v", e.RunID, e.Cause)
}

func (e *WorkflowScriptBlockedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type workflowScriptRunner struct {
	runs      workflowScriptRunStore
	processes agentruntime.WorkspaceProcessRunner
	messages  workflowScriptMessageWriter
	logger    *logger.Logger

	locks sync.Map

	activeMu sync.Mutex
	active   map[string]context.CancelFunc
}

func newWorkflowScriptRunner(
	runs workflowScriptRunStore,
	processes agentruntime.WorkspaceProcessRunner,
	messages workflowScriptMessageWriter,
	log *logger.Logger,
) *workflowScriptRunner {
	return &workflowScriptRunner{
		runs: runs, processes: processes, messages: messages, logger: log,
		active: make(map[string]context.CancelFunc),
	}
}

func (r *workflowScriptRunner) Execute(ctx context.Context, request workflowScriptExecutionRequest) error {
	if err := validateWorkflowScriptExecutionRequest(request); err != nil {
		return err
	}
	occurrenceKey := taskmodels.NewWorkflowScriptOccurrenceKey(
		request.Trigger, request.OccurrenceID, request.WorkflowStepID, request.ActionPosition,
	)
	run, _, err := r.runs.ClaimWorkflowScriptRun(ctx, &taskmodels.WorkflowScriptRun{
		OccurrenceKey:    occurrenceKey,
		TaskID:           request.TaskID,
		WorkflowID:       request.WorkflowID,
		WorkflowStepID:   request.WorkflowStepID,
		WorkflowStepName: request.WorkflowStepName,
		Trigger:          request.Trigger,
		ActionPosition:   request.ActionPosition,
		SessionID:        request.SessionID,
		ExecutionID:      request.ExecutionID,
		Command:          request.Action.Command,
		TimeoutSeconds:   request.Action.TimeoutSeconds,
		FailurePolicy:    string(request.Action.FailurePolicy),
	})
	if err != nil {
		return fmt.Errorf("claim workflow script run: %w", err)
	}
	return r.withRunLock(ctx, run, func() error {
		return r.executeClaimed(ctx, run)
	})
}

func validateWorkflowScriptExecutionRequest(request workflowScriptExecutionRequest) error {
	if request.TaskID == "" || request.WorkflowStepID == "" || request.SessionID == "" {
		return errors.New("workflow script task, step, and session are required")
	}
	if request.WorkflowStepName == "" {
		return errors.New("workflow script step name is required")
	}
	if !request.Trigger.IsValid() || request.OccurrenceID == "" {
		return errors.New("workflow script trigger occurrence is required")
	}
	if request.ActionPosition < 0 || request.Action.Command == "" || request.Action.TimeoutSeconds <= 0 {
		return errors.New("workflow script action is invalid")
	}
	if !taskmodels.IsValidWorkflowScriptFailurePolicy(string(request.Action.FailurePolicy)) {
		return errors.New("workflow script failure policy is invalid")
	}
	return nil
}

func (r *workflowScriptRunner) withRunLock(ctx context.Context, run *taskmodels.WorkflowScriptRun, fn func() error) error {
	_ = ctx
	if run == nil || run.ID == "" {
		return errors.New("workflow script claim returned no run identity")
	}
	value, _ := r.locks.LoadOrStore(run.ID, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer func() {
		lock.Unlock()
		r.locks.Delete(run.ID)
	}()
	return fn()
}

func (r *workflowScriptRunner) executeClaimed(ctx context.Context, run *taskmodels.WorkflowScriptRun) error {
	if run.Status.IsTerminal() {
		return workflowScriptPolicyOutcome(run)
	}
	if run.Status == taskmodels.WorkflowScriptRunPending {
		return r.startPendingRun(ctx, run)
	}
	return r.resumeAdmittedRun(ctx, run)
}

func (r *workflowScriptRunner) startPendingRun(ctx context.Context, run *taskmodels.WorkflowScriptRun) error {
	messageID, err := r.ensureMessage(ctx, run)
	if err != nil {
		return r.failBeforeAdmission(ctx, run, err)
	}
	started, err := r.runs.MarkWorkflowScriptRunStarting(ctx, run.ID, messageID)
	if err != nil {
		return fmt.Errorf("mark workflow script starting: %w", err)
	}
	fresh, err := r.runs.GetWorkflowScriptRun(ctx, run.ID)
	if err != nil {
		return err
	}
	if !started && fresh.Status.IsTerminal() {
		return workflowScriptPolicyOutcome(fresh)
	}
	if fresh.Status == taskmodels.WorkflowScriptRunStarting {
		if err := r.updateRunMessage(ctx, fresh, nil, fresh.Status, ""); err != nil {
			return r.failBeforeAdmission(ctx, fresh, err)
		}
		return r.admitAndWait(ctx, fresh)
	}
	return r.resumeAdmittedRun(ctx, fresh)
}

func (r *workflowScriptRunner) resumeAdmittedRun(ctx context.Context, run *taskmodels.WorkflowScriptRun) error {
	if run.Status == taskmodels.WorkflowScriptRunStarting {
		return r.interruptAmbiguousRun(ctx, run, "process admission was ambiguous")
	}
	if run.Status == taskmodels.WorkflowScriptRunRunning {
		return r.reconcileRunning(ctx, run)
	}
	return fmt.Errorf("workflow script run %s has unsupported status %s", run.ID, run.Status)
}

func (r *workflowScriptRunner) ensureMessage(ctx context.Context, run *taskmodels.WorkflowScriptRun) (string, error) {
	if run.MessageID != "" {
		return run.MessageID, nil
	}
	if r.messages == nil {
		return "", errors.New("workflow script message writer is unavailable")
	}
	messageID := workflowScriptMessageID(run.ID)
	if err := r.messages.CreateWorkflowScriptMessage(
		ctx, messageID, run.TaskID, run.SessionID, "", r.messageMetadata(run, nil, taskmodels.WorkflowScriptRunPending, ""),
	); err != nil {
		return "", fmt.Errorf("create workflow script message: %w", err)
	}
	return messageID, nil
}

func workflowScriptMessageID(runID string) string {
	return uuid.NewSHA1(uuid.Nil, []byte("workflow-script-message/"+runID)).String()
}

func (r *workflowScriptRunner) admitAndWait(ctx context.Context, run *taskmodels.WorkflowScriptRun) error {
	r.recordStarted(run)
	processCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.registerActive(run.ID, cancel)
	defer func() {
		cancel()
		r.unregisterActive(run.ID)
	}()
	if err := processCtx.Err(); err != nil {
		return r.interruptRun(ctx, run, nil, errWorkflowScriptInterrupted)
	}
	process, err := r.processes.Start(processCtx, agentruntime.WorkspaceProcessRequest{
		RunID: run.ProcessRequestID, SessionID: run.SessionID, ExecutionID: run.ExecutionID,
		Command: run.Command, Timeout: time.Duration(run.TimeoutSeconds) * time.Second,
		Kind: "workflow_script", BufferMaxBytes: workflowScriptOutputBufferBytes,
	})
	if err != nil {
		return r.completeFailure(ctx, run, nil, fmt.Errorf("start workflow script process: %w", err))
	}
	if process == nil || process.ID == "" {
		return r.completeFailure(ctx, run, process, errors.New("workflow script process returned no identity"))
	}
	if process.SessionID != run.SessionID {
		return r.completeFailure(ctx, run, process, errors.New("workflow script process session does not match run"))
	}
	running, err := r.runs.MarkWorkflowScriptRunRunning(ctx, run.ID, process.ID)
	if err != nil {
		return r.completeFailure(ctx, run, process, fmt.Errorf("mark workflow script running: %w", err))
	}
	if !running {
		fresh, loadErr := r.runs.GetWorkflowScriptRun(ctx, run.ID)
		if loadErr != nil {
			return loadErr
		}
		run = fresh
		if run.Status.IsTerminal() {
			return workflowScriptPolicyOutcome(run)
		}
	}
	return r.waitForProcess(processCtx, run, process)
}

func (r *workflowScriptRunner) reconcileRunning(ctx context.Context, run *taskmodels.WorkflowScriptRun) error {
	if run.ProcessID == "" {
		return r.interruptAmbiguousRun(ctx, run, "running workflow script has no process identity")
	}
	if r.processes == nil {
		return r.interruptAmbiguousRun(ctx, run, "workspace process runner is unavailable")
	}
	process, err := r.processes.Get(ctx, run.ExecutionID, run.ProcessID, true)
	if err != nil || process == nil {
		if err == nil {
			err = errors.New("workspace process lookup returned no process")
		}
		return r.interruptRun(ctx, run, nil, fmt.Errorf("reconcile workflow script process: %w", err))
	}
	if process.SessionID != run.SessionID {
		return r.interruptRun(ctx, run, process, errors.New("reconciled process session does not match run"))
	}
	processCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.registerActive(run.ID, cancel)
	defer func() {
		cancel()
		r.unregisterActive(run.ID)
	}()
	return r.waitForProcess(processCtx, run, process)
}

func (r *workflowScriptRunner) waitForProcess(ctx context.Context, run *taskmodels.WorkflowScriptRun, process *agentruntime.WorkspaceProcessInfo) error {
	if r.processes == nil {
		return r.interruptRun(context.Background(), run, process, errors.New("workspace process runner is unavailable"))
	}
	lastContent, lastStatus := "", taskmodels.WorkflowScriptRunStatus("")
	ticker := time.NewTicker(workflowScriptMessageBatch)
	defer ticker.Stop()
	for {
		if isTerminalWorkspaceProcess(process.Status) {
			return r.completeFromProcess(context.Background(), run, process)
		}
		if err := r.updateProcessMessage(run, process, &lastContent, &lastStatus); err != nil {
			_ = r.processes.Stop(context.Background(), run.ExecutionID, process.ID)
			return r.completeFailure(context.Background(), run, process, err)
		}
		select {
		case <-ctx.Done():
			_ = r.processes.Stop(context.Background(), run.ExecutionID, process.ID)
			return r.interruptRun(context.Background(), run, process, errWorkflowScriptInterrupted)
		case <-ticker.C:
		}
		fresh, err := r.processes.Get(context.Background(), run.ExecutionID, process.ID, true)
		if err != nil || fresh == nil {
			if err == nil {
				err = errors.New("workspace process lookup returned no process")
			}
			return r.interruptRun(context.Background(), run, process, fmt.Errorf("workflow script process disappeared: %w", err))
		}
		if fresh.SessionID != run.SessionID {
			return r.interruptRun(context.Background(), run, fresh, errors.New("workflow script process session changed"))
		}
		process = fresh
	}
}

func (r *workflowScriptRunner) updateProcessMessage(
	run *taskmodels.WorkflowScriptRun,
	process *agentruntime.WorkspaceProcessInfo,
	lastContent *string,
	lastStatus *taskmodels.WorkflowScriptRunStatus,
) error {
	status := workflowScriptRunStatus(process.Status)
	content := workflowScriptOutput(process.Output)
	if content == *lastContent && status == *lastStatus {
		return nil
	}
	if err := r.updateRunMessage(context.Background(), run, process, status, ""); err != nil {
		return fmt.Errorf("%w: %v", errWorkflowScriptMessage, err)
	}
	*lastContent, *lastStatus = content, status
	return nil
}

func (r *workflowScriptRunner) completeFromProcess(ctx context.Context, run *taskmodels.WorkflowScriptRun, process *agentruntime.WorkspaceProcessInfo) error {
	status, reason := workflowScriptResult(process)
	return r.completeProcess(ctx, run, process, status, reason)
}

func workflowScriptResult(process *agentruntime.WorkspaceProcessInfo) (taskmodels.WorkflowScriptRunStatus, string) {
	switch string(process.Status) {
	case "exited":
		if process.ExitCode != nil && *process.ExitCode == 0 {
			return taskmodels.WorkflowScriptRunSucceeded, ""
		}
		return taskmodels.WorkflowScriptRunFailed, "workflow script exited with a non-zero status"
	case "timed_out":
		return taskmodels.WorkflowScriptRunTimedOut, "workflow script timed out"
	case agentEventFailed, "stopped":
		return taskmodels.WorkflowScriptRunFailed, "workflow script process failed"
	default:
		return taskmodels.WorkflowScriptRunInterrupted, "workflow script process ended with an unknown status"
	}
}

func (r *workflowScriptRunner) completeProcess(
	ctx context.Context,
	run *taskmodels.WorkflowScriptRun,
	process *agentruntime.WorkspaceProcessInfo,
	status taskmodels.WorkflowScriptRunStatus,
	reason string,
) error {
	if err := r.updateRunMessage(ctx, run, process, status, reason); err != nil {
		status = taskmodels.WorkflowScriptRunFailed
		reason = fmt.Sprintf("%s: %v", errWorkflowScriptMessage, err)
	}
	completion := taskmodels.WorkflowScriptRunCompletion{
		Status: status, Output: workflowScriptProcessOutput(process),
		OutputTruncated: workflowScriptProcessOutputTruncated(process), FailureReason: reason,
		CompletedAt: time.Now().UTC(),
	}
	if process != nil {
		completion.ProcessID = process.ID
		completion.ExitCode = process.ExitCode
	}
	if _, err := r.runs.CompleteWorkflowScriptRun(ctx, run.ID, completion); err != nil {
		return fmt.Errorf("complete workflow script run: %w", err)
	}
	fresh, err := r.runs.GetWorkflowScriptRun(ctx, run.ID)
	if err != nil {
		return err
	}
	if err := r.updateRunMessage(ctx, fresh, process, fresh.Status, fresh.FailureReason); err != nil {
		return fmt.Errorf("project terminal workflow script run %s: %w", run.ID, err)
	}
	r.logCompletion(fresh)
	return workflowScriptPolicyOutcome(fresh)
}

func (r *workflowScriptRunner) completeFailure(ctx context.Context, run *taskmodels.WorkflowScriptRun, process *agentruntime.WorkspaceProcessInfo, err error) error {
	if err == nil {
		err = errors.New("workflow script failed")
	}
	return r.completeProcess(ctx, run, process, taskmodels.WorkflowScriptRunFailed, err.Error())
}

func (r *workflowScriptRunner) failBeforeAdmission(ctx context.Context, run *taskmodels.WorkflowScriptRun, err error) error {
	return r.completeFailure(ctx, run, nil, err)
}

func (r *workflowScriptRunner) interruptAmbiguousRun(ctx context.Context, run *taskmodels.WorkflowScriptRun, reason string) error {
	return r.interruptRun(ctx, run, nil, errors.New(reason))
}

func (r *workflowScriptRunner) interruptRun(ctx context.Context, run *taskmodels.WorkflowScriptRun, process *agentruntime.WorkspaceProcessInfo, reason error) error {
	if reason == nil {
		reason = errWorkflowScriptInterrupted
	}
	if process != nil && r.processes != nil && !isTerminalWorkspaceProcess(process.Status) {
		_ = r.processes.Stop(context.Background(), run.ExecutionID, process.ID)
	}
	return r.completeProcess(ctx, run, process, taskmodels.WorkflowScriptRunInterrupted, reason.Error())
}

func (r *workflowScriptRunner) updateRunMessage(
	ctx context.Context,
	run *taskmodels.WorkflowScriptRun,
	process *agentruntime.WorkspaceProcessInfo,
	status taskmodels.WorkflowScriptRunStatus,
	reason string,
) error {
	if r.messages == nil {
		return errors.New("workflow script message writer is unavailable")
	}
	messageID := run.MessageID
	if messageID == "" {
		messageID = workflowScriptMessageID(run.ID)
		if err := r.messages.CreateWorkflowScriptMessage(ctx, messageID, run.TaskID, run.SessionID, "", r.messageMetadata(run, process, status, reason)); err != nil {
			return err
		}
		return nil
	}
	return r.messages.UpdateWorkflowScriptMessage(ctx, messageID, workflowScriptProcessOutput(process), r.messageMetadata(run, process, status, reason))
}

func (r *workflowScriptRunner) messageMetadata(
	run *taskmodels.WorkflowScriptRun,
	process *agentruntime.WorkspaceProcessInfo,
	status taskmodels.WorkflowScriptRunStatus,
	reason string,
) map[string]interface{} {
	metadata := map[string]interface{}{
		"script_type":            "workflow_step",
		"workflow_script_run_id": run.ID,
		"workflow_id":            run.WorkflowID,
		"workflow_step_id":       run.WorkflowStepID,
		"workflow_step_name":     run.WorkflowStepName,
		"trigger":                string(run.Trigger),
		"action_position":        run.ActionPosition,
		"command":                run.Command,
		"timeout_seconds":        run.TimeoutSeconds,
		"failure_policy":         run.FailurePolicy,
		"status":                 string(status),
		"output_truncated":       workflowScriptProcessOutputTruncated(process),
	}
	if run.MessageID != "" {
		metadata["message_id"] = run.MessageID
	}
	if process != nil {
		metadata["process_id"] = process.ID
		metadata["started_at"] = process.StartedAt
		if process.ExitCode != nil {
			metadata["exit_code"] = *process.ExitCode
		}
	}
	if run.StartedAt != nil {
		metadata["started_at"] = *run.StartedAt
	}
	if run.CompletedAt != nil {
		metadata["completed_at"] = *run.CompletedAt
	}
	if reason != "" {
		metadata["error"] = reason
	}
	if run.StartedAt != nil {
		end := time.Now().UTC()
		if run.CompletedAt != nil {
			end = *run.CompletedAt
		}
		metadata["duration_ms"] = end.Sub(*run.StartedAt).Milliseconds()
	}
	return metadata
}

func workflowScriptProcessOutput(process *agentruntime.WorkspaceProcessInfo) string {
	if process == nil {
		return ""
	}
	return workflowScriptOutput(process.Output)
}

func workflowScriptProcessOutputTruncated(process *agentruntime.WorkspaceProcessInfo) bool {
	return process != nil && process.OutputTruncated
}

func workflowScriptOutput(chunks []agentruntime.WorkspaceProcessOutputChunk) string {
	var builder strings.Builder
	for _, chunk := range chunks {
		builder.WriteString(chunk.Data)
	}
	return builder.String()
}

func workflowScriptRunStatus(status agentruntime.WorkspaceProcessStatus) taskmodels.WorkflowScriptRunStatus {
	switch string(status) {
	case "starting":
		return taskmodels.WorkflowScriptRunStarting
	case "running":
		return taskmodels.WorkflowScriptRunRunning
	default:
		return taskmodels.WorkflowScriptRunInterrupted
	}
}

func isTerminalWorkspaceProcess(status agentruntime.WorkspaceProcessStatus) bool {
	switch string(status) {
	case "exited", agentEventFailed, "stopped", "timed_out":
		return true
	default:
		return false
	}
}

func workflowScriptPolicyOutcome(run *taskmodels.WorkflowScriptRun) error {
	if run == nil || run.Status == taskmodels.WorkflowScriptRunSucceeded ||
		run.FailurePolicy == taskmodels.WorkflowScriptFailurePolicyContinue {
		return nil
	}
	return &WorkflowScriptBlockedError{
		RunID: run.ID, MessageID: run.MessageID, Status: run.Status,
		Cause: errors.New(run.FailureReason),
	}
}

func (r *workflowScriptRunner) registerActive(runID string, cancel context.CancelFunc) {
	r.activeMu.Lock()
	r.active[runID] = cancel
	r.activeMu.Unlock()
}

func (r *workflowScriptRunner) unregisterActive(runID string) {
	r.activeMu.Lock()
	delete(r.active, runID)
	r.activeMu.Unlock()
}

func (r *workflowScriptRunner) recordStarted(run *taskmodels.WorkflowScriptRun) {
	workflowScriptStartedTotal.Add(metricLabel("trigger", string(run.Trigger)), 1)
}

func (r *workflowScriptRunner) logCompletion(run *taskmodels.WorkflowScriptRun) {
	if r.logger == nil || run == nil {
		return
	}
	fields := make([]zap.Field, 0, 12)
	fields = append(fields,
		zap.String("task_id", run.TaskID), zap.String("workflow_id", run.WorkflowID),
		zap.String("workflow_step_id", run.WorkflowStepID), zap.String("trigger", string(run.Trigger)),
		zap.Int("action_position", run.ActionPosition), zap.String("run_id", run.ID),
		zap.String("session_id", run.SessionID), zap.String("execution_id", run.ExecutionID),
		zap.String("process_id", run.ProcessID), zap.String("status", string(run.Status)),
		zap.String("failure_policy", run.FailurePolicy), zap.Bool("output_truncated", run.OutputTruncated))
	if run.ExitCode != nil {
		fields = append(fields, zap.Int("exit_code", *run.ExitCode))
	}
	if run.StartedAt != nil && run.CompletedAt != nil {
		fields = append(fields, zap.Duration("duration", run.CompletedAt.Sub(*run.StartedAt)))
	}
	r.logger.Info("workflow script completed", fields...)
	workflowScriptTerminalTotal.Add(metricLabel("trigger", string(run.Trigger), "outcome", string(run.Status)), 1)
}

// Stop interrupts all admitted workflow scripts. Pending rows are left for
// startup recovery because no process admission crossed the at-most-once
// boundary yet.
func (r *workflowScriptRunner) Stop(ctx context.Context) error {
	if r == nil || r.runs == nil {
		return nil
	}
	r.activeMu.Lock()
	for _, cancel := range r.active {
		cancel()
	}
	r.activeMu.Unlock()
	runs, err := r.runs.ListNonTerminalWorkflowScriptRuns(ctx)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.Status != taskmodels.WorkflowScriptRunStarting && run.Status != taskmodels.WorkflowScriptRunRunning {
			continue
		}
		var process *agentruntime.WorkspaceProcessInfo
		if run.ProcessID != "" && r.processes != nil {
			process, _ = r.processes.Get(ctx, run.ExecutionID, run.ProcessID, true)
			if process == nil {
				// Keep the shutdown stop best-effort when observation fails. The
				// durable run still records an interruption and recovery will not
				// admit a replacement process for this occurrence.
				_ = r.processes.Stop(ctx, run.ExecutionID, run.ProcessID)
			}
		}
		_ = r.interruptRun(ctx, run, process, errors.New("workflow service stopped"))
	}
	return nil
}

// Reconcile resumes pending runs and observes admitted runs without creating a
// replacement process. Starting rows are interrupted because this state cannot
// prove whether the process-start boundary was crossed.
func (r *workflowScriptRunner) Reconcile(ctx context.Context) error {
	if r == nil || r.runs == nil {
		return nil
	}
	runs, err := r.runs.ListNonTerminalWorkflowScriptRuns(ctx)
	if err != nil {
		return err
	}
	for _, run := range runs {
		switch run.Status {
		case taskmodels.WorkflowScriptRunPending:
			go r.resumeDetachedRun(run)
		case taskmodels.WorkflowScriptRunStarting:
			completed, completeErr := r.runs.CompleteWorkflowScriptRun(ctx, run.ID, taskmodels.WorkflowScriptRunCompletion{
				Status:        taskmodels.WorkflowScriptRunInterrupted,
				FailureReason: "workflow service restarted during process admission",
				CompletedAt:   time.Now().UTC(),
			})
			if completeErr != nil {
				return fmt.Errorf("reconcile workflow script run %s: %w", run.ID, completeErr)
			}
			if !completed {
				continue
			}
			fresh, getErr := r.runs.GetWorkflowScriptRun(ctx, run.ID)
			if getErr != nil {
				return fmt.Errorf("reload reconciled workflow script run %s: %w", run.ID, getErr)
			}
			if updateErr := r.updateRunMessage(ctx, fresh, nil, fresh.Status, fresh.FailureReason); updateErr != nil {
				return fmt.Errorf("project reconciled workflow script run %s: %w", run.ID, updateErr)
			}
			r.logCompletion(fresh)
		case taskmodels.WorkflowScriptRunRunning:
			go r.resumeDetachedRun(run)
		}
	}
	return nil
}

func (r *workflowScriptRunner) resumeDetachedRun(run *taskmodels.WorkflowScriptRun) {
	ctx := context.Background()
	_ = r.withRunLock(ctx, run, func() error {
		fresh, err := r.runs.GetWorkflowScriptRun(ctx, run.ID)
		if err != nil {
			return err
		}
		return r.executeClaimed(ctx, fresh)
	})
}
