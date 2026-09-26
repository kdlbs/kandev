package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestConversationForkDraftServiceCompilesEstimatesAndAuthorizes(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedConversationForkServiceSource(t, repo)
	ctx := ctxAs("user-a")
	request := models.ConversationForkCreateRequest{
		Source: models.ConversationForkSourceRequest{
			SessionID:       "session-fork-service",
			CutoffMessageID: "message-fork-service",
		},
		DraftRequestID: "draft-request-service",
		ModelID:        "target-model",
	}
	draft, err := svc.CreateConversationForkDraft(ctx, request)
	if err != nil {
		t.Fatalf("create fork draft: %v", err)
	}
	if draft.Descriptor.SourceSessionID != "session-fork-service" || draft.Descriptor.SourceMessageID != "message-fork-service" {
		t.Fatalf("draft provenance = %+v", draft.Descriptor)
	}
	if draft.Descriptor.Estimate.EstimatedTokens == 0 || draft.Descriptor.Estimate.Method != conversationForkCompilerMethod || draft.Descriptor.Estimate.ContextLimit != nil {
		t.Fatalf("estimate = %+v, want a positive informational estimate with no guessed limit", draft.Descriptor.Estimate)
	}
	if draft.Descriptor.Estimate.ModelID != "target-model" || draft.Descriptor.Estimate.AttachmentsUnmeasured {
		t.Fatalf("target model or attachment estimate = %+v", draft.Descriptor.Estimate)
	}
	if lifetime := draft.Descriptor.ExpiresAt.Sub(draft.Descriptor.CreatedAt); lifetime <= conversationForkDraftTTL-time.Second || lifetime > conversationForkDraftTTL {
		t.Fatalf("draft lifetime = %s, want approximately %s", lifetime, conversationForkDraftTTL)
	}
	if draft.Descriptor.ContentHash == "" || draft.CompiledText == "" {
		t.Fatalf("draft text or content hash is empty: %+v", draft.Descriptor)
	}
	storedMessage, err := repo.GetMessage(ctx, "message-fork-service")
	if err != nil {
		t.Fatalf("load source message for mutation: %v", err)
	}
	storedMessage.Content = "Source edited after the preview."
	storedMessage.UpdatedAt = time.Now().UTC().Add(time.Second)
	if err := repo.UpdateMessage(ctx, storedMessage); err != nil {
		t.Fatalf("edit source after preview: %v", err)
	}
	retried, err := svc.CreateConversationForkDraft(ctx, request)
	if err != nil || retried.Descriptor.ID != draft.Descriptor.ID || retried.CompiledText != draft.CompiledText {
		t.Fatalf("same-request retry changed frozen preview: id %q, text %q, err %v", retried.Descriptor.ID, retried.CompiledText, err)
	}
	reestimated, err := svc.EstimateConversationForkDraft(ctx, draft.Descriptor.ID, "target-model-b")
	if err != nil || reestimated.ModelID != "target-model-b" {
		t.Fatalf("re-estimate = %+v, %v", reestimated, err)
	}
	reloaded, err := svc.GetConversationForkDraft(ctx, draft.Descriptor.ID)
	if err != nil || reloaded.CompiledText != draft.CompiledText || reloaded.Descriptor.ContentHash != draft.Descriptor.ContentHash || reloaded.Descriptor.Estimate.ModelID != "target-model-b" {
		t.Fatalf("get fork draft = %q, %v", reloaded.CompiledText, err)
	}
	if _, err := svc.GetConversationForkDraft(ctxAs("user-b"), draft.Descriptor.ID); !errors.Is(err, models.ErrConversationForkNotFound) {
		t.Fatalf("foreign draft access = %v, want not found", err)
	}
	if err := svc.DiscardConversationForkDraft(ctx, draft.Descriptor.ID); err != nil {
		t.Fatalf("discard fork draft: %v", err)
	}
	if _, err := svc.CreateConversationForkDraft(ctx, models.ConversationForkCreateRequest{
		Source:         request.Source,
		DraftRequestID: "draft-request-attachments-unavailable-in-task-01",
		AttachmentIDs:  []string{"attachment-source"},
	}); !errors.Is(err, models.ErrConversationForkAttachmentMissing) {
		t.Fatalf("task-01 selected attachment error = %v, want explicit unavailable", err)
	}
}

func TestConversationForkDraftIncludesPersistedParentContextOnce(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedConversationForkServiceSource(t, repo)
	ctx := ctxAs("user-a")
	original, err := svc.CreateConversationForkDraft(ctx, models.ConversationForkCreateRequest{
		Source:         models.ConversationForkSourceRequest{SessionID: "session-fork-service", CutoffMessageID: "message-fork-service"},
		DraftRequestID: "nested-original-draft",
	})
	if err != nil {
		t.Fatalf("create original fork draft: %v", err)
	}
	admission, _, err := svc.PrepareConversationForkAgentAdmission(ctx,
		"task-fork-service", original.Descriptor.ID, "nested-agent-request", "nested-agent-fingerprint")
	if err != nil {
		t.Fatalf("prepare original fork admission: %v", err)
	}
	admission.DestinationSessionID = "session-fork-origin"
	if _, err := repo.BindConversationForkToSession(ctx, admission); err != nil {
		t.Fatalf("bind original fork to source session: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-fork-origin", TaskID: "task-fork-service", State: models.TaskSessionStateCreated,
		Metadata: map[string]interface{}{models.MetaKeyConversationForkID: original.Descriptor.ID},
	}); err != nil {
		t.Fatalf("create fork destination session: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-fork-origin", TaskID: "task-fork-service", TaskSessionID: "session-fork-origin",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create fork destination turn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "message-fork-origin", TaskID: "task-fork-service", TaskSessionID: "session-fork-origin", TurnID: "turn-fork-origin",
		AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage,
		Content:   "New request after the prior fork.",
		Metadata:  map[string]interface{}{models.MetaKeyConversationForkID: original.Descriptor.ID},
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create fork destination message: %v", err)
	}

	nested, err := svc.CreateConversationForkDraft(ctx, models.ConversationForkCreateRequest{
		Source:         models.ConversationForkSourceRequest{SessionID: "session-fork-origin", CutoffMessageID: "message-fork-origin"},
		DraftRequestID: "nested-child-draft",
	})
	if err != nil {
		t.Fatalf("create nested fork draft: %v", err)
	}
	if strings.Count(nested.CompiledText, "Keep the source context intact.") != 1 {
		t.Fatalf("inherited context count = %d, want once: %q", strings.Count(nested.CompiledText, "Keep the source context intact."), nested.CompiledText)
	}
	if strings.Count(nested.CompiledText, "New request after the prior fork.") != 1 {
		t.Fatalf("new request count = %d, want once: %q", strings.Count(nested.CompiledText, "New request after the prior fork."), nested.CompiledText)
	}
}

type fixedConversationForkContextLimit struct {
	modelID string
	limit   int64
}

func (f fixedConversationForkContextLimit) LookupConversationForkContextLimit(_ context.Context, modelID string) (int64, bool) {
	return f.limit, modelID == f.modelID
}

func TestConversationForkEstimateUsesKnownSelectedModelContextLimit(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedConversationForkServiceSource(t, repo)
	svc.SetConversationForkModelLimitLookup(fixedConversationForkContextLimit{modelID: "known-model", limit: 128_000})
	ctx := ctxAs("user-a")
	draft, err := svc.CreateConversationForkDraft(ctx, models.ConversationForkCreateRequest{
		Source:         models.ConversationForkSourceRequest{SessionID: "session-fork-service", CutoffMessageID: "message-fork-service"},
		DraftRequestID: "draft-known-context-limit", ModelID: "known-model",
	})
	if err != nil {
		t.Fatalf("create fork draft: %v", err)
	}
	if draft.Descriptor.Estimate.ContextLimit == nil || *draft.Descriptor.Estimate.ContextLimit != 128_000 {
		t.Fatalf("initial model context estimate = %+v, want 128000", draft.Descriptor.Estimate)
	}
	if draft.Descriptor.Estimate.LimitSource != "models.dev" {
		t.Fatalf("initial context-limit source = %q, want models.dev", draft.Descriptor.Estimate.LimitSource)
	}

	unknown, err := svc.EstimateConversationForkDraft(ctx, draft.Descriptor.ID, "unknown-model")
	if err != nil {
		t.Fatalf("estimate unknown model: %v", err)
	}
	if unknown.ContextLimit != nil || unknown.LimitSource != "" {
		t.Fatalf("unknown model context estimate = %+v, want unknown limit", unknown)
	}
}

func TestConversationForkCandidateServiceAuthorizesBeforeSourceRead(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedConversationForkServiceSource(t, repo)
	if _, err := svc.ListConversationForkCandidates(ctxAs("user-b"), models.ConversationForkSourceRequest{
		SessionID:       "session-fork-service",
		CutoffMessageID: "message-fork-service",
	}); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign candidate access = %v, want not found", err)
	}
}

func TestConversationForkAgentAdmissionBindsDestinationAndSurvivesSourceDeletion(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedConversationForkServiceSource(t, repo)
	ctx := ctxAs("user-a")
	fork, err := svc.CreateConversationForkDraft(ctx, models.ConversationForkCreateRequest{
		Source:         models.ConversationForkSourceRequest{SessionID: "session-fork-service", CutoffMessageID: "message-fork-service"},
		DraftRequestID: "draft-agent-destination",
	})
	if err != nil {
		t.Fatalf("create fork draft: %v", err)
	}
	admission, draft, err := svc.PrepareConversationForkAgentAdmission(ctx,
		"task-fork-service", fork.Descriptor.ID, "agent-fork-request", "agent-fork-fingerprint")
	if err != nil || draft.Descriptor.ID != fork.Descriptor.ID || admission.DestinationKind != "agent" {
		t.Fatalf("prepare agent fork admission = %+v, %+v, %v", admission, draft.Descriptor, err)
	}
	retryAdmission, existing, err := svc.PrepareConversationForkAgentAdmission(ctx,
		"task-fork-service", fork.Descriptor.ID, "agent-fork-request", "agent-fork-fingerprint")
	if err != nil || retryAdmission.ForkID != admission.ForkID || existing.Descriptor.State != "draft" {
		t.Fatalf("idempotent pre-admission retry = %+v, %+v, %v", retryAdmission, existing.Descriptor, err)
	}
	session := &models.TaskSession{
		ID: "session-fork-agent-destination", TaskID: "task-fork-service", State: models.TaskSessionStateCreated,
		ConversationForkAdmission: &admission,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create destination session: %v", err)
	}
	attached, err := svc.GetConversationForkForDestination(ctx, session.TaskID, session.ID)
	if err != nil || attached.Descriptor.State != "attached" || attached.CompiledText != fork.CompiledText {
		t.Fatalf("destination fork read = %+v, %v", attached.Descriptor, err)
	}
	if _, _, err := svc.PrepareConversationForkAgentAdmission(ctx,
		"task-fork-service", fork.Descriptor.ID, "agent-fork-request", "changed-fingerprint"); !errors.Is(err, models.ErrConversationForkConflict) {
		t.Fatalf("changed agent request fingerprint error = %v, want conflict", err)
	}
	source, err := repo.GetTaskSession(ctx, "session-fork-service")
	if err != nil {
		t.Fatalf("load source session for deletion: %v", err)
	}
	if err := repo.DeleteTaskSession(ctx, source); err != nil {
		t.Fatalf("delete source session: %v", err)
	}
	retained, err := svc.GetConversationForkDraft(ctx, fork.Descriptor.ID)
	if err != nil || retained.CompiledText != fork.CompiledText {
		t.Fatalf("attached fork after source deletion = %+v, %v", retained.Descriptor, err)
	}
}

func TestConversationForkAgentAdmissionRequiresCurrentSourceAccess(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedConversationForkServiceSource(t, repo)
	ownerCtx := ctxAs("user-a")
	fork, err := svc.CreateConversationForkDraft(ownerCtx, models.ConversationForkCreateRequest{
		Source:         models.ConversationForkSourceRequest{SessionID: "session-fork-service", CutoffMessageID: "message-fork-service"},
		DraftRequestID: "draft-agent-access",
	})
	if err != nil {
		t.Fatalf("create fork draft: %v", err)
	}
	if _, _, err := svc.PrepareConversationForkAgentAdmission(ctxAs("user-b"),
		"task-fork-service", fork.Descriptor.ID, "inaccessible-agent-fork", "fingerprint"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign agent admission error = %v, want not found", err)
	}
}

func seedConversationForkServiceSource(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateTask(context.Context, *models.Task) error
	CreateTaskSession(context.Context, *models.TaskSession) error
	CreateTurn(context.Context, *models.Turn) error
	CreateMessage(context.Context, *models.Message) error
}) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-fork-service", Name: "Fork source", OwnerID: "user-a"}); err != nil {
		t.Fatalf("create source workspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-fork-service", WorkspaceID: "workspace-fork-service", Title: "Fork source"}); err != nil {
		t.Fatalf("create source task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-fork-service", TaskID: "task-fork-service", State: models.TaskSessionStateCreated}); err != nil {
		t.Fatalf("create source session: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-fork-service", TaskSessionID: "session-fork-service", TaskID: "task-fork-service", StartedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create source turn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "message-fork-service", TaskSessionID: "session-fork-service", TaskID: "task-fork-service", TurnID: "turn-fork-service",
		AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage, Content: "Keep the source context intact.", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create source message: %v", err)
	}
}
