package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

func TestRunContextAllowsCurrentAndScopedTaskMutations(t *testing.T) {
	ctx := RunContext{
		TaskID: "task-current",
		Capabilities: Capabilities{
			CanUpdateTaskStatus: true,
			AllowedTaskIDs:      []string{"task-extra"},
		},
	}

	for _, taskID := range []string{"task-current", "task-extra"} {
		if !ctx.CanMutateTask(taskID) {
			t.Fatalf("expected task %q to be mutable", taskID)
		}
	}

	if ctx.CanMutateTask("task-other") {
		t.Fatal("expected unrelated task to be denied")
	}
}

func TestRunContextWildcardAllowsAnyTaskMutation(t *testing.T) {
	ctx := RunContext{
		TaskID: "task-current",
		Capabilities: Capabilities{
			CanUpdateTaskStatus: true,
			AllowedTaskIDs:      []string{WildcardTaskScope},
		},
	}

	if !ctx.CanMutateTask("task-other") {
		t.Fatal("expected wildcard task scope to allow unrelated task")
	}
}

func TestActionsPostCommentDeniesMissingCapability(t *testing.T) {
	writer := &recordingCommentWriter{}
	actions := NewActions(ActionDependencies{Comments: writer})
	runCtx := RunContext{AgentID: "agent-1", TaskID: "task-1"}

	err := actions.PostComment(context.Background(), runCtx, "task-1", "hello")
	if !errors.Is(err, ErrCapabilityDenied) || !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if len(writer.comments) != 0 {
		t.Fatal("comment writer should not be called when capability is missing")
	}
}

func TestActionsPostCommentWritesAgentComment(t *testing.T) {
	writer := &recordingCommentWriter{}
	actions := NewActions(ActionDependencies{Comments: writer})
	runCtx := RunContext{
		AgentID:   "agent-1",
		TaskID:    "task-1",
		RunID:     "run-1",
		SessionID: "session-1",
		Capabilities: Capabilities{
			CanPostComments: true,
		},
	}

	if err := actions.PostComment(context.Background(), runCtx, "task-1", "hello"); err != nil {
		t.Fatalf("post comment: %v", err)
	}

	if len(writer.comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(writer.comments))
	}
	comment := writer.comments[0]
	if comment.AuthorType != "agent" || comment.AuthorID != "agent-1" {
		t.Fatalf("unexpected author: %s/%s", comment.AuthorType, comment.AuthorID)
	}
	if comment.Source != "agent" || comment.Body != "hello" || comment.TaskID != "task-1" {
		t.Fatalf("unexpected comment payload: %+v", comment)
	}
	if comment.ID == "" || comment.CreatedAt.IsZero() {
		t.Fatalf("expected generated id and timestamp: %+v", comment)
	}
}

func TestActionsCreateSubtaskDeniesWithoutCapability(t *testing.T) {
	creator := &recordingTaskCreator{}
	actions := NewActions(ActionDependencies{Tasks: creator})
	runCtx := RunContext{AgentID: "agent-1", WorkspaceID: "ws-1", TaskID: "task-1"}

	_, err := actions.CreateSubtask(context.Background(), runCtx, CreateSubtaskInput{
		Title: "child",
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if len(creator.calls) != 0 {
		t.Fatal("task creator should not be called when capability is missing")
	}
}

func TestActionsCreateSubtaskPreservesCallerIdentity(t *testing.T) {
	creator := &recordingTaskCreator{taskID: "created-task"}
	actions := NewActions(ActionDependencies{Tasks: creator})
	runCtx := RunContext{
		AgentID:     "agent-1",
		WorkspaceID: "ws-1",
		TaskID:      "task-parent",
		Capabilities: Capabilities{
			CanCreateSubtasks: true,
		},
	}

	taskID, err := actions.CreateSubtask(context.Background(), runCtx, CreateSubtaskInput{
		AssigneeAgentID: "agent-2",
		Title:           "child",
		Description:     "details",
	})
	if err != nil {
		t.Fatalf("create subtask: %v", err)
	}
	if taskID != "created-task" {
		t.Fatalf("task id = %q, want created-task", taskID)
	}
	if len(creator.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(creator.calls))
	}
	call := creator.calls[0]
	if call.CallerAgentID != "agent-1" {
		t.Fatalf("unexpected caller: %+v", call)
	}
	if call.ParentTaskID != "task-parent" || call.AssigneeAgentID != "agent-2" {
		t.Fatalf("unexpected task routing: %+v", call)
	}
}

func TestActionsUpdateTaskStatusDeniesUnscopedTask(t *testing.T) {
	updater := &recordingStatusUpdater{}
	actions := NewActions(ActionDependencies{TaskStatus: updater})
	runCtx := RunContext{
		AgentID: "agent-1",
		TaskID:  "task-1",
		Capabilities: Capabilities{
			CanUpdateTaskStatus: true,
		},
	}

	err := actions.UpdateTaskStatus(context.Background(), runCtx, "task-2", "done", "")
	if !errors.Is(err, ErrTaskOutOfScope) || !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if len(updater.calls) != 0 {
		t.Fatal("status updater should not be called for an unscoped task")
	}
}

func TestCapabilitiesAllowsByKey(t *testing.T) {
	caps := Capabilities{CanCreateAgents: true, CanReadMemory: true}

	if !caps.Allows(CapabilityCreateAgent) {
		t.Fatal("expected create_agent to be allowed")
	}
	if !caps.Allows(CapabilityReadMemory) {
		t.Fatal("expected read_memory to be allowed")
	}
	if caps.Allows(CapabilityDeleteSkills) {
		t.Fatal("expected delete_skills to be denied")
	}
}

func TestFromAgentMapsExistingPermissions(t *testing.T) {
	ceo := &models.AgentInstance{
		Role: models.AgentRoleCEO,
	}
	worker := &models.AgentInstance{
		Role: models.AgentRoleWorker,
	}

	if !FromAgent(ceo).Allows(CapabilityCreateAgent) {
		t.Fatal("CEO should be allowed to create agents")
	}
	if FromAgent(worker).Allows(CapabilityCreateAgent) {
		t.Fatal("worker should not be allowed to create agents")
	}
}

func TestActionsCreateAgentUsesCallerAndReportsToDefault(t *testing.T) {
	creator := &recordingAgentCreator{}
	actions := NewActions(ActionDependencies{Agents: creator})
	runCtx := RunContext{
		AgentID:     "ceo-1",
		WorkspaceID: "ws-1",
		Capabilities: Capabilities{
			CanCreateAgents: true,
		},
	}
	caller := &models.AgentInstance{ID: "ceo-1", Name: "CEO"}

	agent, err := actions.CreateAgent(context.Background(), runCtx, caller, CreateAgentInput{
		Name:   "Builder",
		Role:   string(models.AgentRoleWorker),
		Reason: "need implementation help",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if agent.ReportsTo != "ceo-1" || agent.WorkspaceID != "ws-1" {
		t.Fatalf("unexpected created agent: %+v", agent)
	}
	if len(creator.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(creator.calls))
	}
	if creator.calls[0].Caller != caller || creator.calls[0].Reason != "need implementation help" {
		t.Fatalf("unexpected creator call: %+v", creator.calls[0])
	}
}

func TestActionsUpdateTaskStatusPreservesRunIdentity(t *testing.T) {
	updater := &recordingStatusUpdater{}
	actions := NewActions(ActionDependencies{TaskStatus: updater})
	runCtx := RunContext{
		AgentID:   "agent-1",
		TaskID:    "task-1",
		RunID:     "run-1",
		SessionID: "session-1",
		Capabilities: Capabilities{
			CanUpdateTaskStatus: true,
		},
	}

	if err := actions.UpdateTaskStatus(context.Background(), runCtx, "task-1", "done", "complete"); err != nil {
		t.Fatalf("update status: %v", err)
	}
	if len(updater.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(updater.calls))
	}
	call := updater.calls[0]
	if call.TaskID != "task-1" || call.NewStatus != "done" || call.Comment != "complete" {
		t.Fatalf("unexpected status call: %+v", call)
	}
	if call.ActorAgentID != "agent-1" || call.RunID != "run-1" || call.SessionID != "session-1" {
		t.Fatalf("missing run identity: %+v", call)
	}
}

func TestActionsRequestApprovalCreatesRunScopedPayload(t *testing.T) {
	requester := &recordingApprovalRequester{}
	actions := NewActions(ActionDependencies{Approvals: requester})
	runCtx := RunContext{
		AgentID:     "agent-1",
		WorkspaceID: "ws-1",
		TaskID:      "task-1",
		RunID:       "run-1",
		SessionID:   "sess-1",
		Capabilities: Capabilities{
			CanRequestApproval: true,
		},
	}

	approval, err := actions.RequestApproval(context.Background(), runCtx, RequestApprovalInput{
		Type:       models.ApprovalTypeTaskReview,
		TargetType: "task",
		TargetID:   "task-1",
		Reason:     "needs human signoff",
		Payload:    map[string]interface{}{"extra": "value"},
	})
	if err != nil {
		t.Fatalf("request approval: %v", err)
	}
	if approval.WorkspaceID != "ws-1" || approval.RequestedByAgentProfileID != "agent-1" {
		t.Fatalf("unexpected approval identity: %+v", approval)
	}
	if len(requester.approvals) != 1 {
		t.Fatalf("expected 1 approval, got %d", len(requester.approvals))
	}
	if !strings.Contains(approval.Payload, `"run_id":"run-1"`) ||
		!strings.Contains(approval.Payload, `"reason":"needs human signoff"`) {
		t.Fatalf("payload missing run context: %s", approval.Payload)
	}
}

func TestActionsSpawnAgentRunDeniesCrossWorkspaceTarget(t *testing.T) {
	agents := &recordingAgentModifier{
		agents: map[string]*models.AgentInstance{
			"agent-2": {ID: "agent-2", WorkspaceID: "ws-2"},
		},
	}
	runs := &recordingRunSpawner{}
	actions := NewActions(ActionDependencies{Runs: runs, AgentModifier: agents})
	runCtx := RunContext{
		WorkspaceID: "ws-1",
		Capabilities: Capabilities{
			CanSpawnAgentRun: true,
		},
	}

	err := actions.SpawnAgentRun(context.Background(), runCtx, SpawnAgentRunInput{
		AgentID: "agent-2",
		Reason:  "heartbeat",
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if len(runs.calls) != 0 {
		t.Fatal("run spawner should not be called for cross-workspace target")
	}
}

func TestActionsModifyAgentUpdatesSameWorkspaceAgent(t *testing.T) {
	name := "Runtime QA"
	agents := &recordingAgentModifier{
		agents: map[string]*models.AgentInstance{
			"agent-2": {ID: "agent-2", WorkspaceID: "ws-1", Name: "Old", Role: models.AgentRoleWorker},
		},
	}
	actions := NewActions(ActionDependencies{AgentModifier: agents})
	runCtx := RunContext{
		WorkspaceID: "ws-1",
		Capabilities: Capabilities{
			CanModifyAgents: true,
		},
	}

	agent, err := actions.ModifyAgent(context.Background(), runCtx, "agent-2", ModifyAgentInput{Name: &name})
	if err != nil {
		t.Fatalf("modify agent: %v", err)
	}
	if agent.Name != "Runtime QA" || len(agents.updated) != 1 {
		t.Fatalf("agent not updated: %+v updated=%d", agent, len(agents.updated))
	}
}

func TestActionsDeleteSkillRequiresWorkspaceScope(t *testing.T) {
	skills := &recordingSkillManager{
		skills: map[string]*models.Skill{
			"skill-1": {ID: "skill-1", WorkspaceID: "ws-1"},
		},
	}
	actions := NewActions(ActionDependencies{Skills: skills})
	runCtx := RunContext{
		WorkspaceID: "ws-1",
		Capabilities: Capabilities{
			CanDeleteSkills: true,
		},
	}

	if err := actions.DeleteSkill(context.Background(), runCtx, "skill-1"); err != nil {
		t.Fatalf("delete skill: %v", err)
	}
	if len(skills.deleted) != 1 || skills.deleted[0] != "skill-1" {
		t.Fatalf("unexpected deleted skills: %+v", skills.deleted)
	}
}

type recordingCommentWriter struct {
	comments []*models.TaskComment
}

func (w *recordingCommentWriter) CreateComment(_ context.Context, comment *models.TaskComment) error {
	w.comments = append(w.comments, comment)
	return nil
}

type recordingTaskCreator struct {
	calls  []createTaskCall
	taskID string
}

type createTaskCall struct {
	CallerAgentID   string
	ParentTaskID    string
	AssigneeAgentID string
	Title           string
	Description     string
}

func (c *recordingTaskCreator) CreateOfficeSubtaskAsAgent(
	_ context.Context,
	callerAgentID string,
	parentTaskID string,
	assigneeAgentID string,
	title string,
	description string,
) (string, error) {
	c.calls = append(c.calls, createTaskCall{
		CallerAgentID:   callerAgentID,
		ParentTaskID:    parentTaskID,
		AssigneeAgentID: assigneeAgentID,
		Title:           title,
		Description:     description,
	})
	if c.taskID != "" {
		return c.taskID, nil
	}
	return "task-id", nil
}

type recordingStatusUpdater struct {
	calls []TaskStatusUpdate
}

func (u *recordingStatusUpdater) UpdateTaskStatusAsAgent(
	_ context.Context,
	update TaskStatusUpdate,
) error {
	u.calls = append(u.calls, update)
	return nil
}

type recordingAgentCreator struct {
	calls []agentCreateCall
}

type agentCreateCall struct {
	Agent  *models.AgentInstance
	Caller *models.AgentInstance
	Reason string
}

func (c *recordingAgentCreator) CreateAgentInstanceWithCaller(
	_ context.Context,
	agent *models.AgentInstance,
	caller *models.AgentInstance,
	reason string,
) error {
	c.calls = append(c.calls, agentCreateCall{Agent: agent, Caller: caller, Reason: reason})
	return nil
}

type recordingApprovalRequester struct {
	approvals []*models.Approval
}

func (r *recordingApprovalRequester) CreateApprovalWithActivity(
	_ context.Context,
	approval *models.Approval,
) error {
	r.approvals = append(r.approvals, approval)
	return nil
}

type recordingRunSpawner struct {
	calls []spawnRunCall
}

type spawnRunCall struct {
	AgentID        string
	Reason         string
	Payload        string
	IdempotencyKey string
}

func (r *recordingRunSpawner) QueueRun(
	_ context.Context,
	agentInstanceID, reason, payload, idempotencyKey string,
) error {
	r.calls = append(r.calls, spawnRunCall{
		AgentID:        agentInstanceID,
		Reason:         reason,
		Payload:        payload,
		IdempotencyKey: idempotencyKey,
	})
	return nil
}

type recordingAgentModifier struct {
	agents  map[string]*models.AgentInstance
	updated []*models.AgentInstance
}

func (r *recordingAgentModifier) GetAgentInstance(
	_ context.Context,
	idOrName string,
) (*models.AgentInstance, error) {
	if agent, ok := r.agents[idOrName]; ok {
		return agent, nil
	}
	return nil, errors.New("agent not found")
}

func (r *recordingAgentModifier) UpdateAgentInstance(
	_ context.Context,
	agent *models.AgentInstance,
) error {
	r.updated = append(r.updated, agent)
	return nil
}

type recordingSkillManager struct {
	skills  map[string]*models.Skill
	deleted []string
}

func (r *recordingSkillManager) GetSkill(_ context.Context, id string) (*models.Skill, error) {
	if skill, ok := r.skills[id]; ok {
		return skill, nil
	}
	return nil, errors.New("skill not found")
}

func (r *recordingSkillManager) DeleteSkill(_ context.Context, id string) error {
	r.deleted = append(r.deleted, id)
	return nil
}
