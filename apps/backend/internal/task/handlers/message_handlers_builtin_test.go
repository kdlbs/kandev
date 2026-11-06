package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type builtinMessagePolicyReader struct {
	reads int
	err   error
}

func (r *builtinMessagePolicyReader) GetBuiltinPolicySnapshot(_ context.Context, workspaceID, taskID string) (*mcpconfig.BuiltinPolicySnapshot, error) {
	r.reads++
	if r.err != nil {
		return nil, r.err
	}
	policy := mcpconfig.DefaultBuiltinWorkspacePolicy(workspaceID)
	for id := range policy.Packages {
		policy.Packages[id] = mcpconfig.BuiltinPolicyAvailable
	}
	return &mcpconfig.BuiltinPolicySnapshot{Policy: policy, Preferences: &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: workspaceID, TaskID: taskID, Packages: mcpconfig.BuiltinPackageChoices{p.PackageConfiguration: mcpconfig.BuiltinPreferenceOn}}}, nil
}

type builtinMessageLaunch struct {
	content string
	profile *p.Context
}

type builtinMessageOrchestrator struct {
	firstTurnCaptureOrchestrator
	executor *executor.Executor
	session  *models.TaskSession
	reader   *builtinMessagePolicyReader
	launched chan builtinMessageLaunch
}

func (o *builtinMessageOrchestrator) PrepareTaskSessionMCPPrompt(ctx context.Context, taskID, _ string) (context.Context, error) {
	prepared, err := o.executor.PrepareMCPLaunch(ctx, taskID, o.session, true)
	if err == nil {
		o.reader.err = errors.New("preferences changed after prompt acceptance")
	}
	return prepared, err
}

func (o *builtinMessageOrchestrator) StartCreatedSessionWithDeliverySubmission(ctx context.Context, taskID, sessionID, _, content string, _ orchestrator.DirectPromptStartOptions) (*executor.TaskExecution, error) {
	o.launched <- builtinMessageLaunch{content: content, profile: executor.PreparedMCPProfile(ctx, taskID, sessionID)}
	return &executor.TaskExecution{}, nil
}

func (o *builtinMessageOrchestrator) StartCreatedSessionWithPromptContextAndCanvasGuidancePreservingDirectPrompt(ctx context.Context, taskID, sessionID, profileID, content string, _, _, _ bool, _ []v1.MessageAttachment, _ []v1.EntityReference, _ string, _, _, _ bool) (*executor.TaskExecution, error) {
	return o.StartCreatedSessionWithDeliverySubmission(ctx, taskID, sessionID, profileID, content, orchestrator.DirectPromptStartOptions{})
}

// @covers AC-AGENTS-BUILTIN-MCP-003.2
func TestBuiltinMessageStartPreservesAcceptedPackageContext(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "policy unavailable"}[denied], func(t *testing.T) {
			ctx := context.Background()
			svc, taskHandlers, repo := newQueuedTaskDTOBuilder(t)
			task := &models.Task{ID: "task", WorkspaceID: "ws-1", Title: "Task", Description: "original task brief", State: v1.TaskStateInProgress}
			session := &models.TaskSession{ID: "session", TaskID: task.ID, State: models.TaskSessionStateCreated, IsPrimary: true, AgentProfileID: "profile"}
			require.NoError(t, repo.CreateTask(ctx, task))
			require.NoError(t, repo.CreateTaskSession(ctx, session))
			exec := executor.NewExecutor(nil, repo, taskHandlers.logger, executor.ExecutorConfig{})
			reader := &builtinMessagePolicyReader{}
			if denied {
				reader.err = errors.New("policy unavailable")
			}
			exec.SetBuiltinPolicyReader(reader)
			orch := &builtinMessageOrchestrator{executor: exec, session: session, reader: reader, launched: make(chan builtinMessageLaunch, 1)}
			h := NewMessageHandlers(svc, orch, taskHandlers.logger)
			request, err := ws.NewRequest("message", ws.ActionMessageAdd, map[string]any{"task_id": task.ID, "session_id": session.ID, "content": "first instruction"})
			require.NoError(t, err)
			response, err := h.wsAddMessage(ctx, request)
			require.NoError(t, err)
			if denied {
				require.Equal(t, ws.MessageTypeError, response.Type)
				messages, readErr := repo.ListMessages(ctx, session.ID)
				require.NoError(t, readErr)
				require.Empty(t, messages, "failed preparation must not persist misleading guidance")
				return
			}
			require.Equal(t, ws.MessageTypeResponse, response.Type)
			select {
			case launch := <-orch.launched:
				messages, readErr := repo.ListMessages(ctx, session.ID)
				require.NoError(t, readErr)
				require.Len(t, messages, 1)
				require.Equal(t, messages[0].Content, launch.content)
				require.NotNil(t, launch.profile, "async dispatch must retain the acceptance snapshot")
				require.ElementsMatch(t, []p.PackageID{p.PackageCore, p.PackageConfiguration}, launch.profile.BuiltinPackages.Packages)
				require.Contains(t, launch.content, "kandev-configuration")
				require.NotContains(t, launch.content, "call show_rich_output_kandev")
				require.Contains(t, launch.content, "original task brief")
				require.Contains(t, launch.content, "first instruction")
				require.Equal(t, 1, reader.reads)
			case <-time.After(5 * time.Second):
				t.Fatal("created session was not dispatched")
			}
		})
	}
}

func TestBuiltinMessageRejectsUnknownIntentBeforePersistence(t *testing.T) {
	ctx := context.Background()
	svc, taskHandlers, repo := newQueuedTaskDTOBuilder(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "ws-1", Title: "Task"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", State: models.TaskSessionStateCreated, IsPrimary: true}))
	h := NewMessageHandlers(svc, nil, taskHandlers.logger)
	request, err := ws.NewRequest("intent", ws.ActionMessageAdd, map[string]any{"task_id": "task", "session_id": "session", "content": "request", "builtin_intent": "arbitrary-tool"})
	require.NoError(t, err)
	response, err := h.wsAddMessage(ctx, request)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	messages, err := repo.ListMessages(ctx, "session")
	require.NoError(t, err)
	require.Empty(t, messages)
}

type builtinIntentMessageOrchestrator struct {
	firstTurnCaptureOrchestrator
	allow   bool
	checked []string
}

func (o *builtinIntentMessageOrchestrator) ValidateBuiltinIntent(_ context.Context, taskID, sessionID, intent string) error {
	o.checked = []string{taskID, sessionID, intent}
	if !o.allow {
		return errors.New("built-in tools unavailable")
	}
	return nil
}

func TestBuiltinMessageChecksIntentBeforePersistence(t *testing.T) {
	for _, allow := range []bool{false, true} {
		ctx := context.Background()
		svc, taskHandlers, repo := newQueuedTaskDTOBuilder(t)
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "ws-1", Title: "Task"}))
		require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", State: models.TaskSessionStateWaitingForInput, IsPrimary: true}))
		checker := &builtinIntentMessageOrchestrator{allow: allow}
		h := NewMessageHandlers(svc, checker, taskHandlers.logger)
		request, err := ws.NewRequest("intent", ws.ActionMessageAdd, map[string]any{"task_id": "task", "session_id": "session", "content": "user supplied text", "builtin_intent": "walkthrough"})
		require.NoError(t, err)
		response, err := h.wsAddMessage(ctx, request)
		require.NoError(t, err)
		require.Equal(t, []string{"task", "session", "walkthrough"}, checker.checked)
		messages, err := repo.ListMessages(ctx, "session")
		require.NoError(t, err)
		if allow {
			require.Equal(t, ws.MessageTypeResponse, response.Type)
			require.Len(t, messages, 1)
			require.EqualValues(t, "walkthrough", messages[0].Metadata["builtin_intent"])
		} else {
			require.Equal(t, ws.MessageTypeError, response.Type)
			require.Empty(t, messages)
		}
	}
}

func TestBuiltinMessageReplayIdentityIncludesIntent(t *testing.T) {
	request := wsAddMessageRequest{TaskID: "task", TaskSessionID: "session", Content: "request"}
	ordinary, err := addMessageRequestFingerprint(request)
	require.NoError(t, err)
	request.BuiltinIntent = "walkthrough"
	structured, err := addMessageRequestFingerprint(request)
	require.NoError(t, err)
	require.NotEqual(t, ordinary, structured)
}

func (o *builtinIntentMessageOrchestrator) PrepareBuiltinIntent(ctx context.Context, taskID, sessionID, intent string) (context.Context, error) {
	return ctx, o.ValidateBuiltinIntent(ctx, taskID, sessionID, intent)
}
