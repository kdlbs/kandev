package backendapp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/logger"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type nativeInputFixture struct {
	ctx         context.Context
	adapter     *taskCreatorAdapter
	resolver    *assistantInputResolver
	questions   *clarification.Store
	permissions *attentionTestPermissions
	binding     *shared.AssistantBinding
	row         shared.Attention
}

func newNativeInputFixture(t *testing.T, prompt string, delegable bool) nativeInputFixture {
	t.Helper()
	adapter, svc := newOfficeTaskAdapterHarness(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner", Role: authn.RoleMember})
	require.NoError(t, adapter.taskRepo.CreateTask(ctx, &taskmodels.Task{ID: "worker", WorkspaceID: "ws-1", Title: "Synthetic worker"}))
	require.NoError(t, adapter.taskRepo.CreateTaskSession(ctx, &taskmodels.TaskSession{ID: "session", TaskID: "worker", State: taskmodels.TaskSessionStateWaitingForInput}))
	require.NoError(t, adapter.taskRepo.CreateTurn(ctx, &taskmodels.Turn{ID: "turn", TaskID: "worker", TaskSessionID: "session", StartedAt: time.Now(), CreatedAt: time.Now()}))
	questions := clarification.NewStore(time.Minute)
	question := clarification.Question{ID: "color", Prompt: prompt, AssistantDelegable: delegable, Options: []clarification.Option{{ID: "blue", Label: "Blue"}}}
	questions.CreateRequest(&clarification.Request{PendingID: "pending", SessionID: "session", TaskID: "worker", Questions: []clarification.Question{question}})
	creator := &messageCreatorAdapter{svc: svc, logger: logger.Default()}
	_, err := creator.CreateClarificationRequestMessages(ctx, "worker", "session", "pending", []clarification.Question{question}, "")
	require.NoError(t, err)
	permissions := &attentionTestPermissions{}
	reader := &assistantAttentionReader{tasks: svc, questions: questions, permissions: permissions}
	binding := &shared.AssistantBinding{OwnerUserID: "owner", OrchestratorID: "assistant", WorkspaceID: "ws-1"}
	rows, err := reader.ReadAttention(ctx, binding, "worker", nil)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := shared.Attention{TaskID: "worker", AttentionSource: rows[0]}
	resolver := &assistantInputResolver{attention: reader, sessions: svc, permissions: &recordingPermissionResolver{}, clarifications: clarification.NewResolver(questions, adapter.taskRepo, creator, svc, nil, nil, nil, logger.Default())}
	return nativeInputFixture{ctx, adapter, resolver, questions, permissions, binding, row}
}
func TestAssistantInputNativeQuestionAndAttribution(t *testing.T) {
	for _, actor := range []string{"user", "agent"} {
		t.Run(actor, func(t *testing.T) {
			f := newNativeInputFixture(t, "Choose a sample color", true)
			input, err := f.resolver.ReadInput(f.ctx, f.binding, f.row)
			require.NoError(t, err)
			require.True(t, input.Questions[0].AssistantDelegable)
			require.Equal(t, "blue", input.Questions[0].Options[0].ID)
			entered := make(chan struct{}, 1)
			f.questions.SetOnWaitEntered(func(string) { entered <- struct{}{} })
			ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
			defer cancel()
			delivered := make(chan error, 1)
			go func() { _, err := f.questions.WaitForResponse(ctx, "pending"); delivered <- err }()
			<-entered
			response := shared.InputResponse{ActorType: actor, ActorID: "owner", Answers: []shared.InputAnswer{{QuestionID: "color", SelectedOptions: []string{"blue"}}}}
			if actor == "agent" {
				response.ActorID = "assistant"
				response.SourceMemoryIDs = []string{"confirmed-color"}
			}
			_, err = f.resolver.ResolveInput(ctx, f.binding, f.row, response)
			require.NoError(t, err)
			require.NoError(t, <-delivered)
			messages, err := f.adapter.taskRepo.FindMessagesByPendingID(ctx, "pending")
			require.NoError(t, err)
			require.Len(t, messages, 1)
			require.Equal(t, "answered", messages[0].Metadata["status"])
			if actor == "agent" {
				require.Equal(t, "assistant", messages[0].Metadata["response_author_id"])
				require.Equal(t, []any{"confirmed-color"}, messages[0].Metadata["response_memory_ids"])
			} else {
				require.NotContains(t, messages[0].Metadata, "response_author_type")
			}
			_, err = f.resolver.ResolveInput(ctx, f.binding, f.row, response)
			require.Error(t, err)
		})
	}
}
func TestAssistantInputNativeDenialsAndFullPrompt(t *testing.T) {
	prompt := strings.Repeat("Example requirement. ", 40) + "Final condition must remain visible."
	f := newNativeInputFixture(t, prompt, false)
	input, err := f.resolver.ReadInput(f.ctx, f.binding, f.row)
	require.NoError(t, err)
	require.Equal(t, prompt, input.Questions[0].Prompt)
	response := shared.InputResponse{ActorType: "agent", ActorID: "assistant", SourceMemoryIDs: []string{"memory"}, Answers: []shared.InputAnswer{{QuestionID: "color", SelectedOptions: []string{"blue"}}}}
	_, err = f.resolver.ResolveInput(f.ctx, f.binding, f.row, response)
	require.ErrorContains(t, err, "question_requires_human")
	response.ActorType, response.ActorID = "user", "owner"
	response.Answers[0].SelectedOptions = []string{"not-offered"}
	_, err = f.resolver.ResolveInput(f.ctx, f.binding, f.row, response)
	require.ErrorContains(t, err, "invalid_native_answer")
	_, err = f.resolver.ReadInput(context.Background(), f.binding, f.row)
	require.Error(t, err)
	f.questions.CancelRequest("pending")
	input, err = f.resolver.ReadInput(f.ctx, f.binding, f.row)
	require.NoError(t, err)
	require.Equal(t, "expired", input.State)
}
func TestAssistantInputNativePermissionIdentity(t *testing.T) {
	f := newNativeInputFixture(t, "Choose a sample color", false)
	for _, generation := range []string{"old", "current"} {
		require.NoError(t, f.adapter.taskRepo.CreateMessage(f.ctx, &taskmodels.Message{ID: generation, TaskID: "worker", TaskSessionID: "session", TurnID: "turn", Type: taskmodels.MessageTypePermissionRequest, AuthorType: taskmodels.MessageAuthorAgent, Metadata: map[string]any{"pending_id": "reused", "request_id": generation}}))
	}
	f.permissions.rows = []streams.PendingAgentPermission{{TaskID: "worker", SessionID: "session", PendingID: "reused", RequestID: "current", Options: []streams.PermissionChoice{{OptionID: "deny", Name: "Deny", Kind: streams.PermissionOptionKindRejectOnce}}}}
	rows, err := f.resolver.attention.ReadAttention(f.ctx, f.binding, "worker", nil)
	require.NoError(t, err)
	for _, source := range rows {
		if source.Kind == "permission" && source.State == "pending" {
			f.row = shared.Attention{TaskID: "worker", AttentionSource: source}
		}
	}
	input, err := f.resolver.ReadInput(f.ctx, f.binding, f.row)
	require.NoError(t, err)
	require.Equal(t, "current", input.RequestID)
	response := shared.InputResponse{ActorType: "agent", ActorID: "assistant", OptionID: "deny"}
	_, err = f.resolver.ResolveInput(f.ctx, f.binding, f.row, response)
	require.ErrorContains(t, err, "permission_requires_human")
	response.ActorType, response.ActorID = "user", "owner"
	response.OptionID = "invented"
	_, err = f.resolver.ResolveInput(f.ctx, f.binding, f.row, response)
	require.ErrorContains(t, err, "permission_option_not_offered")
	response.OptionID = "deny"
	_, err = f.resolver.ResolveInput(f.ctx, f.binding, f.row, response)
	require.NoError(t, err)
	resolved := f.resolver.permissions.(*recordingPermissionResolver).request
	require.Equal(t, "current", resolved.RequestID)
	require.Equal(t, "session", resolved.SessionID)
	require.Equal(t, taskmodels.PermissionSourceWeb, resolved.Source)
	f.permissions.rows = nil
	_, err = f.resolver.ResolveInput(f.ctx, f.binding, f.row, response)
	require.ErrorContains(t, err, "native_input_expired_or_superseded")
}

func TestAssistantInputOversizedQuestionUsesNativeView(t *testing.T) {
	f := newNativeInputFixture(t, strings.Repeat("Example. ", 5000), false)
	_, err := f.resolver.ReadInput(f.ctx, f.binding, f.row)
	require.ErrorContains(t, err, "open_native_task_for_full_input")
}
