package models

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestLoadSessionRuntimeConfigMapStringExtractsReservedKeys(t *testing.T) {
	cfg, ok := LoadSessionRuntimeConfig(map[string]interface{}{
		SessionMetaKeyRuntimeConfig: map[string]string{
			"model":            "gpt-5.3-codex-spark",
			"mode":             "acceptEdits",
			"reasoning_effort": "low",
		},
	})
	if !ok {
		t.Fatal("expected runtime config")
	}
	if cfg.Model != "gpt-5.3-codex-spark" {
		t.Fatalf("Model = %q", cfg.Model)
	}
	if cfg.Mode != "acceptEdits" {
		t.Fatalf("Mode = %q", cfg.Mode)
	}
	if got := cfg.ConfigOptions["reasoning_effort"]; got != "low" {
		t.Fatalf("reasoning_effort = %q", got)
	}
	if _, ok := cfg.ConfigOptions["model"]; ok {
		t.Fatal("model key should not remain in ConfigOptions")
	}
	if _, ok := cfg.ConfigOptions["mode"]; ok {
		t.Fatal("mode key should not remain in ConfigOptions")
	}
}

func TestLoadSessionRuntimeConfigOverridesUsesDedicatedKey(t *testing.T) {
	metadata := map[string]interface{}{
		SessionMetaKeyRuntimeConfig: map[string]interface{}{
			"config_options": map[string]interface{}{"effort": "medium"},
		},
		SessionMetaKeyRuntimeConfigOverrides: map[string]interface{}{
			"model":          "gpt-5.6-sol",
			"config_options": map[string]interface{}{"effort": "low"},
		},
	}
	overrides, ok := LoadSessionRuntimeConfigOverrides(metadata)
	if !ok || overrides.Model != "gpt-5.6-sol" || overrides.ConfigOptions["effort"] != "low" {
		t.Fatalf("overrides = %#v, %v", overrides, ok)
	}
}

func TestLoadSessionACPConfigBaselinePreservesEmptyJSONValues(t *testing.T) {
	baseline, ok := LoadSessionACPConfigBaseline(map[string]interface{}{
		SessionMetaKeyACPConfigBaseline: map[string]interface{}{
			"reasoning_effort": "",
			"fast_mode":        "off",
			"ignored":          float64(1),
		},
	})

	if !ok {
		t.Fatal("expected JSON-rehydrated baseline")
	}
	if value, exists := baseline["reasoning_effort"]; !exists || value != "" {
		t.Fatalf("empty baseline value = %q, exists = %v", value, exists)
	}
	if baseline["fast_mode"] != "off" {
		t.Fatalf("fast_mode = %q, want off", baseline["fast_mode"])
	}
	if _, exists := baseline["ignored"]; exists {
		t.Fatal("non-string baseline value should be ignored")
	}
}

func TestTaskStateConstants(t *testing.T) {
	tests := []struct {
		name     string
		state    v1.TaskState
		expected string
	}{
		{"CREATED state", v1.TaskStateCreated, "CREATED"},
		{"SCHEDULING state", v1.TaskStateScheduling, "SCHEDULING"},
		{"TODO state", v1.TaskStateTODO, "TODO"},
		{"IN_PROGRESS state", v1.TaskStateInProgress, "IN_PROGRESS"},
		{"REVIEW state", v1.TaskStateReview, "REVIEW"},
		{"BLOCKED state", v1.TaskStateBlocked, "BLOCKED"},
		{"WAITING_FOR_INPUT state", v1.TaskStateWaitingForInput, "WAITING_FOR_INPUT"},
		{"COMPLETED state", v1.TaskStateCompleted, "COMPLETED"},
		{"FAILED state", v1.TaskStateFailed, "FAILED"},
		{"CANCELLED state", v1.TaskStateCancelled, "CANCELLED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.state) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(tt.state))
			}
		})
	}
}

func TestIsTerminalTaskState(t *testing.T) {
	tests := []struct {
		state v1.TaskState
		want  bool
	}{
		{v1.TaskStateCompleted, true},
		{v1.TaskStateFailed, true},
		{v1.TaskStateCancelled, true},
		{v1.TaskStateTODO, false},
		{v1.TaskStateCreated, false},
		{v1.TaskStateScheduling, false},
		{v1.TaskStateInProgress, false},
		{v1.TaskStateReview, false},
		{v1.TaskStateBlocked, false},
		{v1.TaskStateWaitingForInput, false},
	}
	for _, tt := range tests {
		if got := IsTerminalTaskState(tt.state); got != tt.want {
			t.Errorf("IsTerminalTaskState(%s) = %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestTaskStructInitialization(t *testing.T) {
	now := time.Now().UTC()
	repos := []*TaskRepository{
		{
			ID:           "task-repo-1",
			TaskID:       "task-123",
			RepositoryID: "repo-123",
			BaseBranch:   "main",
			Position:     0,
			Metadata:     map[string]interface{}{"role": "primary"},
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
	task := Task{
		ID:             "task-123",
		WorkspaceID:    "workspace-001",
		WorkflowID:     "workflow-456",
		WorkflowStepID: "workflow-step-789",
		Title:          "Test Task",
		Description:    "A test task description",
		State:          v1.TaskStateTODO,
		Priority:       "high",
		Position:       1,
		Metadata:       map[string]interface{}{"key": "value"},
		Repositories:   repos,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if task.ID != "task-123" {
		t.Errorf("expected ID task-123, got %s", task.ID)
	}
	if task.WorkspaceID != "workspace-001" {
		t.Errorf("expected WorkspaceID workspace-001, got %s", task.WorkspaceID)
	}
	if task.WorkflowID != "workflow-456" {
		t.Errorf("expected WorkflowID workflow-456, got %s", task.WorkflowID)
	}
	if task.WorkflowStepID != "workflow-step-789" {
		t.Errorf("expected WorkflowStepID workflow-step-789, got %s", task.WorkflowStepID)
	}
	if task.Title != "Test Task" {
		t.Errorf("expected Title 'Test Task', got %s", task.Title)
	}
	if task.Description != "A test task description" {
		t.Errorf("expected Description 'A test task description', got %s", task.Description)
	}
	if task.State != v1.TaskStateTODO {
		t.Errorf("expected State TODO, got %s", task.State)
	}
	if task.Priority != "high" {
		t.Errorf("expected Priority high, got %s", task.Priority)
	}
	if len(task.Repositories) != 1 {
		t.Fatalf("expected 1 repository, got %d", len(task.Repositories))
	}
	if task.Repositories[0].RepositoryID != "repo-123" {
		t.Errorf("expected RepositoryID 'repo-123', got %s", task.Repositories[0].RepositoryID)
	}
	if task.Repositories[0].BaseBranch != "main" {
		t.Errorf("expected BaseBranch 'main', got %s", task.Repositories[0].BaseBranch)
	}
	if task.Position != 1 {
		t.Errorf("expected Position 1, got %d", task.Position)
	}
	if task.Metadata["key"] != "value" {
		t.Errorf("expected Metadata key 'value', got %v", task.Metadata["key"])
	}
}

func TestWorkflowStructInitialization(t *testing.T) {
	now := time.Now().UTC()
	wf := Workflow{
		ID:          "workflow-123",
		WorkspaceID: "workspace-001",
		Name:        "Test Workflow",
		Description: "A test workflow",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if wf.ID != "workflow-123" {
		t.Errorf("expected ID workflow-123, got %s", wf.ID)
	}
	if wf.WorkspaceID != "workspace-001" {
		t.Errorf("expected WorkspaceID workspace-001, got %s", wf.WorkspaceID)
	}
	if wf.Name != "Test Workflow" {
		t.Errorf("expected Name 'Test Workflow', got %s", wf.Name)
	}
	if wf.Description != "A test workflow" {
		t.Errorf("expected Description 'A test workflow', got %s", wf.Description)
	}
}

func TestTaskToAPI(t *testing.T) {
	now := time.Now().UTC()
	task := &Task{
		ID:             "task-123",
		WorkspaceID:    "workspace-001",
		WorkflowID:     "workflow-456",
		WorkflowStepID: "step-789",
		Title:          "Test Task",
		Description:    "A test task description",
		State:          v1.TaskStateInProgress,
		Priority:       "medium",
		Repositories: []*TaskRepository{
			{
				ID:           "task-repo-1",
				TaskID:       "task-123",
				RepositoryID: "repo-123",
				BaseBranch:   "main",
				Position:     0,
				Metadata:     map[string]interface{}{"role": "primary"},
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
		Position:  2,
		Metadata:  map[string]interface{}{"key": "value"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	apiTask := task.ToAPI()

	if apiTask.ID != task.ID {
		t.Errorf("expected ID %s, got %s", task.ID, apiTask.ID)
	}
	if apiTask.WorkspaceID != task.WorkspaceID {
		t.Errorf("expected WorkspaceID %s, got %s", task.WorkspaceID, apiTask.WorkspaceID)
	}
	if apiTask.WorkflowID != task.WorkflowID {
		t.Errorf("expected WorkflowID %s, got %s", task.WorkflowID, apiTask.WorkflowID)
	}
	if apiTask.Title != task.Title {
		t.Errorf("expected Title %s, got %s", task.Title, apiTask.Title)
	}
	if apiTask.Description != task.Description {
		t.Errorf("expected Description %s, got %s", task.Description, apiTask.Description)
	}
	if apiTask.State != task.State {
		t.Errorf("expected State %s, got %s", task.State, apiTask.State)
	}
	if apiTask.Priority != task.Priority {
		t.Errorf("expected Priority %s, got %s", task.Priority, apiTask.Priority)
	}
	if len(apiTask.Repositories) != 1 {
		t.Fatalf("expected 1 repository, got %d", len(apiTask.Repositories))
	}
	if apiTask.Repositories[0].RepositoryID != "repo-123" {
		t.Errorf("expected RepositoryID repo-123, got %s", apiTask.Repositories[0].RepositoryID)
	}
	if apiTask.Repositories[0].BaseBranch != "main" {
		t.Errorf("expected BaseBranch main, got %s", apiTask.Repositories[0].BaseBranch)
	}
	if apiTask.Metadata["key"] != "value" {
		t.Errorf("expected Metadata key 'value', got %v", apiTask.Metadata["key"])
	}
}

func TestTaskToAPIWithEmptyOptionalFields(t *testing.T) {
	now := time.Now().UTC()
	task := &Task{
		ID:             "task-123",
		WorkspaceID:    "workspace-001",
		WorkflowID:     "workflow-456",
		WorkflowStepID: "step-789",
		Title:          "Test Task",
		Description:    "A test task description",
		State:          v1.TaskStateTODO,
		Priority:       "medium",
		Position:       0,
		Metadata:       nil,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	apiTask := task.ToAPI()

	if len(apiTask.Repositories) != 0 {
		t.Errorf("expected no repositories, got %d", len(apiTask.Repositories))
	}
}

func TestTaskEnvironmentRepoToAPIIncludesBranchSlug(t *testing.T) {
	now := time.Now().UTC()
	repo := &TaskEnvironmentRepo{
		ID:                "ter-1",
		TaskEnvironmentID: "env-1",
		RepositoryID:      "repo-1",
		BranchSlug:        "branch-5hn",
		WorktreeID:        "wt-1",
		Position:          1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	api := repo.ToAPI()

	if api["branch_slug"] != "branch-5hn" {
		t.Fatalf("branch_slug = %v, want branch-5hn", api["branch_slug"])
	}
}

// TestTaskIsFromOfficeField verifies the IsFromOffice field round-trips
// through the model. The actual office-vs-kanban predicate is computed in
// SQL by isFromOfficeProjection (see repository/sqlite/task.go) so the
// scan layer is the only thing that sets the field. A round-trip test is
// the right scope at this layer; the SQL projection itself is covered by
// TestIsFromOfficeProjection_RealWorkspaceWorkflow in
// repository/sqlite/is_from_office_test.go, which exercises all three
// branches (office workflow, project link, neither) against a real DB.
func TestTaskIsFromOfficeField(t *testing.T) {
	tests := []struct {
		name string
		task Task
		want bool
	}{
		{"default zero value", Task{}, false},
		{"explicit false", Task{IsFromOffice: false}, false},
		{"explicit true", Task{IsFromOffice: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.task.IsFromOffice; got != tt.want {
				t.Errorf("IsFromOffice = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskIsOfficeOwnedAndAssigned(t *testing.T) {
	tests := []struct {
		name string
		task *Task
		want bool
	}{
		{name: "nil task", task: nil, want: false},
		{name: "kanban runner", task: &Task{AssigneeAgentProfileID: "runner"}, want: false},
		{name: "unassigned office task", task: &Task{IsFromOffice: true}, want: false},
		{name: "assigned office task", task: &Task{IsFromOffice: true, AssigneeAgentProfileID: "runner"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.task.IsOfficeOwnedAndAssigned(); got != tt.want {
				t.Errorf("IsOfficeOwnedAndAssigned() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExecutorTypeRuntime pins the ExecutorType → agentruntime.Runtime
// mapping table. Every executor variant the codebase ships must
// declare its runtime explicitly here — a missing case falls through
// to RuntimeStandalone, which silently mislabels container executors
// as host-side. That's the exact failure mode MockAgent.BuildCommand
// guards against via opts.Runtime.IsContainerized(); if Runtime()
// reports the wrong value here, the guard is bypassed and docker e2e
// fails with `exec: "<host-abs-path>": not found` (the bug fixed in
// commit 8518f65).
//
// When you add a new ExecutorType, add a row here and a matching
// case in the Runtime() switch. The default-fallthrough probe at the
// end keeps the implementation honest: it exercises an unknown value
// and asserts the host-side default, so introducing a new container
// type without registering it surfaces as a real mismatch upstream
// rather than as a quiet wrong answer.
func TestExecutorTypeRuntime(t *testing.T) {
	cases := []struct {
		in   ExecutorType
		want agentruntime.Runtime
	}{
		{ExecutorTypeLocal, agentruntime.RuntimeStandalone},
		{ExecutorTypeWorktree, agentruntime.RuntimeStandalone},
		{ExecutorTypeMockRemote, agentruntime.RuntimeStandalone},
		{ExecutorTypeLocalDocker, agentruntime.RuntimeDocker},
		{ExecutorTypeRemoteDocker, agentruntime.RuntimeRemoteDocker},
		{ExecutorTypeSprites, agentruntime.RuntimeSprites},
	}
	for _, tc := range cases {
		t.Run(string(tc.in), func(t *testing.T) {
			if got := tc.in.Runtime(); got != tc.want {
				t.Errorf("ExecutorType(%q).Runtime() = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	// Unknown ExecutorType falls back to standalone — documented in the
	// switch's default branch. Pin it so a future "treat unknown as
	// docker" refactor can't happen silently.
	t.Run("unknown_falls_back_to_standalone", func(t *testing.T) {
		got := ExecutorType("not-a-real-type").Runtime()
		if got != agentruntime.RuntimeStandalone {
			t.Errorf("unknown ExecutorType.Runtime() = %q, want %q (host-side fallback)",
				got, agentruntime.RuntimeStandalone)
		}
	})
}
