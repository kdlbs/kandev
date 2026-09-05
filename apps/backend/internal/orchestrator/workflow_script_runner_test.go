package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	runtimeagentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type workflowScriptRunStoreFake struct {
	mu   sync.Mutex
	runs map[string]*taskmodels.WorkflowScriptRun
	seq  int
}

func (f *workflowScriptRunStoreFake) ClaimWorkflowScriptRun(_ context.Context, run *taskmodels.WorkflowScriptRun) (*taskmodels.WorkflowScriptRun, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.runs == nil {
		f.runs = make(map[string]*taskmodels.WorkflowScriptRun)
	}
	if existing, ok := f.runs[run.OccurrenceKey]; ok {
		return cloneWorkflowScriptRun(existing), false, nil
	}
	f.seq++
	if run.ID == "" {
		run.ID = fmt.Sprintf("run-%d", f.seq)
	}
	if run.ProcessRequestID == "" {
		run.ProcessRequestID = run.ID
	}
	if run.Status == "" {
		run.Status = taskmodels.WorkflowScriptRunPending
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}
	run.UpdatedAt = run.CreatedAt
	f.runs[run.OccurrenceKey] = cloneWorkflowScriptRun(run)
	return cloneWorkflowScriptRun(run), true, nil
}

func (f *workflowScriptRunStoreFake) GetWorkflowScriptRun(_ context.Context, id string) (*taskmodels.WorkflowScriptRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, run := range f.runs {
		if run.ID == id {
			return cloneWorkflowScriptRun(run), nil
		}
	}
	return nil, taskmodels.ErrWorkflowScriptRunNotFound
}

func (f *workflowScriptRunStoreFake) GetWorkflowScriptRunByOccurrence(_ context.Context, key string) (*taskmodels.WorkflowScriptRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run, ok := f.runs[key]
	if !ok {
		return nil, taskmodels.ErrWorkflowScriptRunNotFound
	}
	return cloneWorkflowScriptRun(run), nil
}

func (f *workflowScriptRunStoreFake) ListNonTerminalWorkflowScriptRuns(_ context.Context) ([]*taskmodels.WorkflowScriptRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []*taskmodels.WorkflowScriptRun
	for _, run := range f.runs {
		if !run.Status.IsTerminal() {
			result = append(result, cloneWorkflowScriptRun(run))
		}
	}
	return result, nil
}

func (f *workflowScriptRunStoreFake) MarkWorkflowScriptRunStarting(_ context.Context, id, messageID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run := f.findLocked(id)
	if run == nil {
		return false, taskmodels.ErrWorkflowScriptRunNotFound
	}
	if run.Status != taskmodels.WorkflowScriptRunPending {
		return false, nil
	}
	now := time.Now().UTC()
	run.Status = taskmodels.WorkflowScriptRunStarting
	run.MessageID = messageID
	run.AdmissionAttemptedAt = &now
	run.StartedAt = &now
	run.UpdatedAt = now
	return true, nil
}

func (f *workflowScriptRunStoreFake) MarkWorkflowScriptRunRunning(_ context.Context, id, processID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run := f.findLocked(id)
	if run == nil {
		return false, taskmodels.ErrWorkflowScriptRunNotFound
	}
	if run.Status != taskmodels.WorkflowScriptRunStarting {
		return false, nil
	}
	run.Status = taskmodels.WorkflowScriptRunRunning
	run.ProcessID = processID
	run.UpdatedAt = time.Now().UTC()
	return true, nil
}

func (f *workflowScriptRunStoreFake) CompleteWorkflowScriptRun(_ context.Context, id string, completion taskmodels.WorkflowScriptRunCompletion) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run := f.findLocked(id)
	if run == nil {
		return false, taskmodels.ErrWorkflowScriptRunNotFound
	}
	if run.Status.IsTerminal() {
		return false, nil
	}
	run.Status = completion.Status
	run.ProcessID = completion.ProcessID
	run.ExitCode = completion.ExitCode
	run.Output = completion.Output
	run.OutputTruncated = completion.OutputTruncated
	run.FailureReason = completion.FailureReason
	run.CompletedAt = &completion.CompletedAt
	run.UpdatedAt = completion.CompletedAt
	return true, nil
}

func (f *workflowScriptRunStoreFake) InterruptWorkflowScriptRuns(_ context.Context, reason string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, run := range f.runs {
		if run.Status == taskmodels.WorkflowScriptRunStarting || run.Status == taskmodels.WorkflowScriptRunRunning {
			run.Status = taskmodels.WorkflowScriptRunInterrupted
			run.FailureReason = reason
			count++
		}
	}
	return count, nil
}

func (f *workflowScriptRunStoreFake) findLocked(id string) *taskmodels.WorkflowScriptRun {
	for _, run := range f.runs {
		if run.ID == id {
			return run
		}
	}
	return nil
}

type workflowScriptMessageFake struct {
	mu       sync.Mutex
	created  []string
	updated  []string
	content  map[string]string
	metadata map[string]map[string]interface{}
}

func (f *workflowScriptMessageFake) CreateWorkflowScriptMessage(_ context.Context, messageID, _ string, _ string, content string, metadata map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.content == nil {
		f.content = make(map[string]string)
		f.metadata = make(map[string]map[string]interface{})
	}
	f.created = append(f.created, messageID)
	f.content[messageID] = content
	f.metadata[messageID] = cloneWorkflowScriptMetadata(metadata)
	return nil
}

func (f *workflowScriptMessageFake) UpdateWorkflowScriptMessage(_ context.Context, messageID, content string, metadata map[string]interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updated = append(f.updated, messageID)
	f.content[messageID] = content
	f.metadata[messageID] = cloneWorkflowScriptMetadata(metadata)
	return nil
}

type workflowScriptProcessFake struct {
	mu      sync.Mutex
	starts  []agentruntime.WorkspaceProcessRequest
	gets    int
	stops   int
	process *runtimeagentctl.ProcessInfo
}

func (f *workflowScriptProcessFake) Start(_ context.Context, request agentruntime.WorkspaceProcessRequest) (*runtimeagentctl.ProcessInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, request)
	if f.process == nil {
		f.process = &runtimeagentctl.ProcessInfo{
			ID: "process-1", SessionID: request.SessionID, Status: "exited",
			ExitCode: ptr(0), Output: []runtimeagentctl.ProcessOutputChunk{
				{Stream: "stdout", Data: "ok\n"},
				{Stream: "stderr", Data: "warning\n"},
			},
		}
	}
	return cloneWorkflowScriptProcess(f.process), nil
}

func (f *workflowScriptProcessFake) Get(_ context.Context, _ string, _ string, _ bool) (*runtimeagentctl.ProcessInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	if f.process == nil {
		return nil, errors.New("process not found")
	}
	return cloneWorkflowScriptProcess(f.process), nil
}

func (f *workflowScriptProcessFake) Stop(_ context.Context, _ string, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
	return nil
}

func TestWorkflowScriptRunnerExecutesOnceAndProjectsTerminalOutput(t *testing.T) {
	runs := &workflowScriptRunStoreFake{}
	messages := &workflowScriptMessageFake{}
	processes := &workflowScriptProcessFake{}
	runner := newWorkflowScriptRunner(runs, processes, messages, testLogger())
	request := workflowScriptExecutionRequest{
		TaskID: "task-1", WorkflowID: "workflow-1", WorkflowStepID: "step-1", WorkflowStepName: "Build",
		Trigger: taskmodels.WorkflowScriptRunTriggerOnEnter, ActionPosition: 0, OccurrenceID: "entry-1",
		SessionID: "session-destination", ExecutionID: "execution-destination",
		Action: wfmodels.WorkflowScriptAction{Command: "./verify.sh", TimeoutSeconds: 600, FailurePolicy: wfmodels.WorkflowScriptFailurePolicyBlock},
	}

	if err := runner.Execute(context.Background(), request); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	if err := runner.Execute(context.Background(), request); err != nil {
		t.Fatalf("replayed Execute: %v", err)
	}

	if got := len(processes.starts); got != 1 {
		t.Fatalf("process starts = %d, want 1", got)
	}
	if got := len(messages.created); got != 1 {
		t.Fatalf("message creates = %d, want 1", got)
	}
	if len(messages.updated) == 0 {
		t.Fatal("expected terminal message update")
	}
	messageMetadata := messages.metadata[messages.created[0]]
	if got := messageMetadata["workflow_step_name"]; got != "Build" {
		t.Fatalf("workflow step name metadata = %#v, want Build", got)
	}
	if _, ok := messageMetadata["completed_at"]; !ok {
		t.Fatal("workflow script message is missing completed_at")
	}
	if _, ok := messageMetadata["duration_ms"]; !ok {
		t.Fatal("workflow script message is missing duration_ms")
	}
	run, err := runs.GetWorkflowScriptRunByOccurrence(context.Background(), "on_enter/entry-1/step-1/0")
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.Status != taskmodels.WorkflowScriptRunSucceeded || run.Output != "ok\nwarning\n" {
		t.Fatalf("run = %+v, want succeeded with combined output", run)
	}
	if processes.starts[0].SessionID != "session-destination" || processes.starts[0].ExecutionID != "execution-destination" {
		t.Fatalf("process binding = %+v", processes.starts[0])
	}
}

func TestWorkflowScriptRunnerBlockAndContinuePolicies(t *testing.T) {
	for _, test := range []struct {
		name    string
		policy  wfmodels.WorkflowScriptFailurePolicy
		blocked bool
	}{
		{name: "block", policy: wfmodels.WorkflowScriptFailurePolicyBlock, blocked: true},
		{name: "continue", policy: wfmodels.WorkflowScriptFailurePolicyContinue, blocked: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			runs := &workflowScriptRunStoreFake{}
			messages := &workflowScriptMessageFake{}
			processes := &workflowScriptProcessFake{process: &runtimeagentctl.ProcessInfo{
				ID: "process-failed", SessionID: "session-source", Status: "failed", ExitCode: ptr(7),
				Output: []runtimeagentctl.ProcessOutputChunk{{Stream: "stderr", Data: "nope"}},
			}}
			runner := newWorkflowScriptRunner(runs, processes, messages, testLogger())
			err := runner.Execute(context.Background(), workflowScriptExecutionRequest{
				TaskID: "task-1", WorkflowID: "workflow-1", WorkflowStepID: "step-1", WorkflowStepName: "Build",
				Trigger: taskmodels.WorkflowScriptRunTriggerOnExit, ActionPosition: 1, OccurrenceID: "transition-1",
				SessionID: "session-source", ExecutionID: "execution-source",
				Action: wfmodels.WorkflowScriptAction{Command: "./verify.sh", TimeoutSeconds: 5, FailurePolicy: test.policy},
			})
			var blocked *WorkflowScriptBlockedError
			if test.blocked != errors.As(err, &blocked) {
				t.Fatalf("error = %v, blocked = %v, want blocked = %v", err, errors.As(err, &blocked), test.blocked)
			}
			run, loadErr := runs.GetWorkflowScriptRunByOccurrence(context.Background(), "on_exit/transition-1/step-1/1")
			if loadErr != nil {
				t.Fatalf("load run: %v", loadErr)
			}
			if run.Status != taskmodels.WorkflowScriptRunFailed || run.ExitCode == nil || *run.ExitCode != 7 {
				t.Fatalf("run = %+v, want failed exit 7", run)
			}
		})
	}
}

func TestWorkflowScriptRunnerStopObservesAdmittedProcessBeforeInterrupting(t *testing.T) {
	runs := &workflowScriptRunStoreFake{
		runs: map[string]*taskmodels.WorkflowScriptRun{
			"on_exit/transition-1/step-1/0": {
				ID: "run-1", OccurrenceKey: "on_exit/transition-1/step-1/0",
				TaskID: "task-1", WorkflowID: "workflow-1", WorkflowStepID: "step-1",
				WorkflowStepName: "Build", Trigger: taskmodels.WorkflowScriptRunTriggerOnExit,
				ActionPosition: 0, SessionID: "session-source", ExecutionID: "execution-source",
				MessageID: "message-1", ProcessID: "process-1", Status: taskmodels.WorkflowScriptRunRunning,
				Command: "./verify.sh", TimeoutSeconds: 600,
				FailurePolicy: string(taskmodels.WorkflowScriptFailurePolicyBlock),
			},
		},
	}
	messages := &workflowScriptMessageFake{
		content:  map[string]string{"message-1": ""},
		metadata: map[string]map[string]interface{}{"message-1": {}},
	}
	processes := &workflowScriptProcessFake{process: &runtimeagentctl.ProcessInfo{
		ID: "process-1", SessionID: "session-source", Status: "running",
		Output: []runtimeagentctl.ProcessOutputChunk{{Stream: "stdout", Data: "partial output\n"}},
	}}
	runner := newWorkflowScriptRunner(runs, processes, messages, testLogger())

	if err := runner.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if processes.gets != 1 {
		t.Fatalf("process gets = %d, want 1", processes.gets)
	}
	if processes.stops != 1 {
		t.Fatalf("process stops = %d, want 1", processes.stops)
	}
	if got := messages.content["message-1"]; got != "partial output\n" {
		t.Fatalf("projected output = %q, want latest process output", got)
	}
	run, err := runs.GetWorkflowScriptRun(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("load interrupted run: %v", err)
	}
	if run.Status != taskmodels.WorkflowScriptRunInterrupted {
		t.Fatalf("run status = %s, want interrupted", run.Status)
	}
}

func TestWorkflowScriptRunnerReconcileProjectsInterruptedAdmission(t *testing.T) {
	runs := &workflowScriptRunStoreFake{
		runs: map[string]*taskmodels.WorkflowScriptRun{
			"on_enter/entry-1/step-1/0": {
				ID: "run-1", OccurrenceKey: "on_enter/entry-1/step-1/0",
				TaskID: "task-1", WorkflowID: "workflow-1", WorkflowStepID: "step-1",
				WorkflowStepName: "Build", Trigger: taskmodels.WorkflowScriptRunTriggerOnEnter,
				ActionPosition: 0, SessionID: "session-destination", ExecutionID: "execution-destination",
				MessageID: "message-1", Status: taskmodels.WorkflowScriptRunStarting,
				Command: "./verify.sh", TimeoutSeconds: 600,
				FailurePolicy: string(taskmodels.WorkflowScriptFailurePolicyBlock),
			},
		},
	}
	messages := &workflowScriptMessageFake{
		content:  map[string]string{"message-1": ""},
		metadata: map[string]map[string]interface{}{"message-1": {"status": "starting"}},
	}
	runner := newWorkflowScriptRunner(runs, &workflowScriptProcessFake{}, messages, testLogger())

	if err := runner.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	run, err := runs.GetWorkflowScriptRun(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("load interrupted run: %v", err)
	}
	if run.Status != taskmodels.WorkflowScriptRunInterrupted {
		t.Fatalf("run status = %s, want interrupted", run.Status)
	}
	if len(messages.updated) != 1 || messages.updated[0] != "message-1" {
		t.Fatalf("message updates = %#v, want one update for message-1", messages.updated)
	}
	metadata := messages.metadata["message-1"]
	if metadata["status"] != string(taskmodels.WorkflowScriptRunInterrupted) {
		t.Fatalf("message status = %#v, want interrupted", metadata["status"])
	}
	if metadata["error"] != "workflow service restarted during process admission" {
		t.Fatalf("message error = %#v, want restart reason", metadata["error"])
	}
}

func cloneWorkflowScriptRun(run *taskmodels.WorkflowScriptRun) *taskmodels.WorkflowScriptRun {
	copy := *run
	return &copy
}

func cloneWorkflowScriptProcess(info *runtimeagentctl.ProcessInfo) *runtimeagentctl.ProcessInfo {
	copy := *info
	copy.Output = append([]runtimeagentctl.ProcessOutputChunk(nil), info.Output...)
	return &copy
}

func cloneWorkflowScriptMetadata(metadata map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		copy[key] = value
	}
	return copy
}

func ptr[T any](value T) *T {
	return &value
}
