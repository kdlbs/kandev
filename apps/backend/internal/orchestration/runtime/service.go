// Package runtime runs workspace conversations on core task execution and durable runs.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestration/personas"
	store "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	runmodels "github.com/kandev/kandev/internal/runs/models"
	runstore "github.com/kandev/kandev/internal/runs/repository/sqlite"
	runservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"strings"
	"sync"
	"unicode/utf8"
)

type Tasks interface {
	GetTask(context.Context, string) (*taskmodels.Task, error)
	ListTaskSessions(context.Context, string) ([]*taskmodels.TaskSession, error)
	GetLastAgentMessage(context.Context, string) (string, error)
	GetLastAgentMessageForTurn(context.Context, string) (string, error)
}
type Manager interface {
	CreateWorkspaceTask(context.Context, models.WorkspaceTaskSpec) (string, error)
	ManageWorkspaceTask(context.Context, models.WorkspaceTaskCommand) error
	WorkspaceTaskDetails(context.Context, string, string) (any, error)
	WorkspaceCatalog(context.Context, string) (any, error)
}
type Launch struct {
	OnSessionPrepared                                func(context.Context, string) error
	TaskID, PersonaID, ProfileID, ExecutorID, Prompt string
	Env                                              map[string]string
}
type Service struct {
	Repo         *store.Repository
	Personas     *personas.Service
	Runs         *runstore.Repository
	Queue        *runservice.Service
	Auth         *runtimeauth.AgentAuth
	Tasks        Tasks
	Manager      Manager
	Credentials  CredentialHealthReader
	Start        func(context.Context, Launch) error
	UpdateStatus func(context.Context, string, string, string) error
	APIURL, CLI  string
	mu           sync.Mutex
}

func (s *Service) QueueTurn(ctx context.Context, id, taskID, reason, key string, payload map[string]any) error {
	if _, human := authn.IdentityFromContext(ctx); human {
		if err := s.Repo.AuthorizePersona(ctx, id); err != nil {
			return err
		}
	}
	a, err := s.Personas.GetAgentInstance(ctx, id)
	if err != nil {
		return err
	}
	if paused(a) {
		return fmt.Errorf("coordinator is paused")
	}
	role, err := s.Repo.OrchestratorRoleID(ctx, id)
	if err != nil {
		return err
	}
	if role == "" {
		return fmt.Errorf("coordinator not registered")
	}
	payload, err = s.withIntentRevision(ctx, taskID, payload)
	if err != nil {
		return err
	}
	if err := s.privateTurnSource(ctx, taskID, payload); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Queue == nil {
		return fmt.Errorf("orchestration queue is not ready")
	}
	_, err = s.Queue.QueueRun(ctx, runservice.QueueRunRequest{AgentProfileID: id, TaskID: taskID, Reason: reason, IdempotencyKey: key, Payload: payload, DisableCoalescing: true})
	return err
}

// Process claims no rows itself: the core queue dispatcher owns the single claim loop.
func (s *Service) Process(ctx context.Context, run *runmodels.Run) (bool, error) {
	role, err := s.Repo.OrchestratorRoleID(ctx, run.AgentProfileID)
	if err != nil || role == "" {
		return false, err
	}
	err = s.launch(ctx, run)
	if err != nil {
		_ = s.Runs.RecordFailure(ctx, run.ID, err.Error())
		_ = s.Runs.FinishRun(ctx, run.ID, statusFailed, nil)
		_ = s.Repo.SetRuntimeWorking(ctx, run.AgentProfileID, false)
	}
	return true, err
}
func (s *Service) launch(ctx context.Context, run *runmodels.Run) error {
	a, err := s.Personas.GetAgentInstance(ctx, run.AgentProfileID)
	if err != nil {
		return err
	}
	if paused(a) {
		return s.Runs.FinishRun(ctx, run.ID, "finished", nil)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Payload), &payload); err != nil {
		return err
	}
	taskID, _ := payload["task_id"].(string)
	owner, ws, err := s.Repo.ConversationOwner(ctx, taskID)
	if err != nil || owner != a.ID || ws != a.WorkspaceID {
		return fmt.Errorf("run must belong to the coordinator conversation")
	}
	if err := s.validateBindingSnapshot(ctx, taskID, run.Payload); err != nil {
		return fmt.Errorf("conversation binding is no longer current: %w", err)
	}
	profile, executorID, err := s.executionSelection(ctx, a)
	if err != nil {
		return err
	}

	token, err := s.Auth.MintRuntimeJWT(a.ID, taskID, ws, run.ID, "", "workspace_coordinator")
	if err != nil {
		return err
	}
	prompt, err := s.prompt(ctx, a, taskID, payload)
	if err != nil {
		return err
	}
	env := map[string]string{"KANDEV_API_URL": s.APIURL, "KANDEV_API_KEY": token, "KANDEV_RUN_TOKEN": token, "KANDEV_AGENT_ID": a.ID, "KANDEV_AGENT_NAME": a.Name, "KANDEV_WORKSPACE_ID": ws, "KANDEV_RUN_ID": run.ID, "KANDEV_TASK_ID": taskID, "KANDEV_WAKE_REASON": run.Reason, "KANDEV_CLI": s.CLI, "KANDEV_RUNTIME_API_PREFIX": "/api/v1/orchestration"}
	env["KANDEV_INTENT_REVISION"] = fmt.Sprint(payload["intent_revision"])
	_ = s.Runs.UpdateRunPromptArtifacts(ctx, run.ID, prompt, "")
	if err := s.Repo.SetRuntimeWorking(ctx, a.ID, true); err != nil {
		return err
	}
	return s.Start(ctx, Launch{TaskID: taskID, PersonaID: a.ID, ProfileID: profile, ExecutorID: executorID, Prompt: prompt, Env: env, OnSessionPrepared: func(ctx context.Context, sessionID string) error {
		token, err := s.Auth.MintRuntimeJWT(a.ID, taskID, ws, run.ID, sessionID, "workspace_coordinator")
		if err != nil {
			return err
		}
		env["KANDEV_API_KEY"], env["KANDEV_RUN_TOKEN"] = token, token
		return s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, "workspace_coordinator", run.Payload, sessionID)
	}})
}
func clip(value string, max int) string {
	if len(value) <= max {
		return value
	}
	for max > 0 && !utf8.RuneStart(value[max]) {
		max--
	}
	return value[:max] + "\n[Excerpt]"
}
func (s *Service) prompt(ctx context.Context, a *models.AgentInstance, taskID string, payload map[string]any) (string, error) {
	files, err := s.Repo.ListInstructions(ctx, a.ID)
	if err != nil {
		return "", err
	}
	var text strings.Builder
	for _, f := range files {
		if f.IsEntry && f.Filename != "ROLE.md" {
			fmt.Fprintf(&text, "%s\n%s\n", f.Filename, f.Content)
		}
	}
	role, err := s.Repo.AssignedRole(ctx, a.ID)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&text, "Role: %s\n%s\n", role.Name, role.Instructions)
	fmt.Fprintf(&text, "\nWorkspace: %s\nPersona: %s\nConversation task: %s\nRouting context: %s\n", a.WorkspaceID, a.ID, taskID, models.DelegationContext(a))
	memory, err := s.Repo.MemoryContext(ctx, a.ID)
	if err != nil {
		return "", err
	}
	selected, err := s.promptMemory(ctx, a, taskID, memory)
	if err != nil {
		return "", err
	}
	for _, entry := range selected.Memory {
		fmt.Fprintf(&text, "\nMemory %s (scope=%s/%s, revision=%d, confirmed=%t, source=%s): %s\n", entry.ID, entry.Scope, entry.ScopeID, entry.Revision, entry.Confirmed, entry.SourceCommentID, entry.Content)
	}
	if selected.OmittedMemory > 0 {
		fmt.Fprintf(&text, "\n%d optional memories omitted; use scoped memory lookup when needed.\n", selected.OmittedMemory)
	}
	comments, err := s.Repo.ListComments(ctx, taskID, 4)
	if err != nil {
		return "", err
	}
	for i := len(comments) - 1; i >= 0; i-- {
		fmt.Fprintf(&text, "\n%s: %s\n", comments[i].AuthorType, clip(comments[i].Body, 1000))
	}
	if id, _ := payload["comment_id"].(string); id != "" {
		comment, err := s.Repo.GetCommentByID(ctx, taskID, id)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&text, "\nCurrent user message (comment_id=%s, intent_revision=%v): %s\n", comment.ID, payload["intent_revision"], comment.Body)
	}
	if callback, ok := payload["callback"]; ok {
		data, _ := json.Marshal(callback)
		fmt.Fprintf(&text, "\nTask update: %s\nInspect this task's current state/result and post only new information in this chat. Review is not completion; do not repeat an answered question or restart work.\n", data)
	}
	text.WriteString("\nUse agentctl kandev (or $KANDEV_CLI kandev) for workspace, task, and memory commands. Your final reply appears in this conversation. Retrieve older comments only when needed.\n")
	return text.String(), nil
}

func (s *Service) executionSelection(ctx context.Context, a *models.AgentInstance) (string, string, error) {
	profile, err := personas.ExecutionProfileID(a.Settings)
	if err != nil || profile == "" {
		return "", "", fmt.Errorf("execution profile unavailable")
	}
	if err := s.Personas.ConfigurePinnedProfile(ctx, a, profile); err != nil {
		return "", "", err
	}
	var executor struct {
		ID string `json:"executor_profile_id"`
	}
	if err := json.Unmarshal([]byte(a.ExecutorPreference), &executor); err != nil || executor.ID == "" {
		return "", "", fmt.Errorf("executor profile unavailable")
	}
	return profile, executor.ID, nil
}

func paused(a *models.AgentInstance) bool {
	return a.Status == models.AgentStatusPaused || a.Status == "stopped"
}
