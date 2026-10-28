package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/admission"
	"github.com/kandev/kandev/internal/testutil"
	userstore "github.com/kandev/kandev/internal/user/store"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestAcceptedMessagePersistsOnlySelectedRegistrationIntents(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	ctx := context.Background()
	seedMetadataCASTask(t, repo, nil)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-registration-message", TaskID: casTaskID,
		State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-registration-message", TaskID: casTaskID, TaskSessionID: "session-registration-message"}); err != nil {
		t.Fatalf("create turn: %v", err)
	}

	linked := selectedPRReference("selection-accepted", apiv1.EntityReferenceModeLinkOnSend)
	contextOnly := selectedPRReference("selection-context", apiv1.EntityReferenceModeContextOnly)
	contextOnly.Provider = "github"
	contextOnly.Kind = "issue"
	contextOnly.ID = "issue-13"
	contextOnly.Ref = entityrefs.CanonicalRef("github", "issue", contextOnly.Scope, contextOnly.ID)
	contextOnly.Key = "acme/app#13"
	contextOnly.Title = "Related discussion"
	contextOnly.URL = "https://github.com/acme/app/issues/13"
	message := &models.Message{
		ID: "message-registration-accepted", TaskID: casTaskID, TaskSessionID: "session-registration-message", TurnID: "turn-registration-message",
		AuthorType: models.MessageAuthorUser, Content: "Check these changes",
		Metadata: map[string]any{"entity_references": []apiv1.EntityReference{linked, contextOnly}},
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("accept message: %v", err)
	}
	intents, err := repo.ListPendingTaskChangeRegistrationIntents(ctx, 20)
	if err != nil {
		t.Fatalf("list pending intents: %v", err)
	}
	if len(intents) != 1 || intents[0].OriginID != message.ID || intents[0].SelectionID != linked.SelectionID ||
		intents[0].ReferenceRef != linked.Ref || intents[0].SourceRevision != "1" || intents[0].State != "pending" ||
		intents[0].ActorID != userstore.DefaultUserID {
		t.Fatalf("accepted registration intents = %+v, want the selected link only", intents)
	}
}

func TestInitialTaskBriefMessageSharesRegistrationIntentBoundary(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	ctx := context.Background()
	seedMetadataCASTask(t, repo, nil)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET description = ? WHERE id = ?`), "Brief", casTaskID); err != nil {
		t.Fatalf("set task description: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-registration-initial", TaskID: casTaskID,
		State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-registration-initial", TaskID: casTaskID, TaskSessionID: "session-registration-initial"}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	ref := selectedPRReference("selection-initial", apiv1.EntityReferenceModeLinkOnSend)
	message := &models.Message{
		ID: "message-registration-initial", TaskID: casTaskID, TaskSessionID: "session-registration-initial", TurnID: "turn-registration-initial",
		AuthorType: models.MessageAuthorUser, Content: "Initial message",
		Metadata: map[string]any{"entity_references": []apiv1.EntityReference{ref}},
	}
	if err := repo.CreateMessageWithInitialTaskBrief(ctx, message, &admission.InitialTaskBriefCandidate{
		DescriptionSnapshot: "Brief", Content: "Initial task brief",
	}); err != nil {
		t.Fatalf("accept initial brief message: %v", err)
	}
	intent, err := repo.GetTaskChangeRegistrationIntentBySelection(ctx, casTaskID, ref.SelectionID)
	if err != nil || intent == nil || intent.OriginID != message.ID || message.PromptIndex != 1 {
		t.Fatalf("initial message intent=%+v prompt=%d err=%v", intent, message.PromptIndex, err)
	}
}

func TestAcceptedMessageCapturesRegistrationFenceSQLite(t *testing.T) {
	runAcceptedMessageRegistrationFence(t, newRepoForMetadataCASTests(t))
}

func TestAcceptedMessageCapturesRegistrationFencePostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	runAcceptedMessageRegistrationFence(t, newPostgresMetadataCASRepo(t, db))
}

func runAcceptedMessageRegistrationFence(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	seedMetadataCASTask(t, repo, nil)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-registration-fence", TaskID: casTaskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-registration-fence", TaskID: casTaskID, TaskSessionID: "session-registration-fence",
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repository-registration-fence", WorkspaceID: "ws-cas", Name: "app", Provider: "github",
		ProviderRepoID: "42", ProviderHost: "github.com", ProviderOwner: "acme", ProviderName: "app",
		RemoteURL: "https://github.com/acme/app.git",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	created := time.Now().UTC()
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO task_repositories (id, task_id, repository_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`), "task-repository-registration-fence", casTaskID, "repository-registration-fence", created, created); err != nil {
		t.Fatalf("attach repository: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `CREATE TABLE github_task_prs (
		task_id TEXT NOT NULL, provider_host TEXT NOT NULL, connection_scope TEXT NOT NULL,
		provider_repository_id TEXT NOT NULL, pr_number INTEGER NOT NULL, association_revision BIGINT NOT NULL
	)`); err != nil {
		t.Fatalf("create provider association table: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO github_task_prs (task_id, provider_host, connection_scope, provider_repository_id, pr_number, association_revision)
		VALUES (?, ?, ?, ?, ?, ?)`), casTaskID, "https://github.com", "builtin:ws-cas", "42", 12, 7); err != nil {
		t.Fatalf("seed provider association: %v", err)
	}
	reference := selectedPRReference("selection-registration-fence", apiv1.EntityReferenceModeLinkOnSend)
	message := &models.Message{
		ID: "message-registration-fence", TaskID: casTaskID, TaskSessionID: "session-registration-fence",
		TurnID: "turn-registration-fence", AuthorType: models.MessageAuthorUser, AuthorID: "user-cas",
		Content: "Review this change", Metadata: map[string]any{"entity_references": []apiv1.EntityReference{reference}},
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("accept message: %v", err)
	}
	intent, err := repo.GetTaskChangeRegistrationIntentBySelection(ctx, casTaskID, reference.SelectionID)
	if err != nil || intent == nil || intent.RepositoryID != "repository-registration-fence" ||
		intent.ProviderRepositoryID != "42" || intent.ConnectionScope != "builtin:ws-cas" ||
		intent.ChangeRequestID != "12" || intent.ExpectedRevision == nil || *intent.ExpectedRevision != 7 {
		t.Fatalf("captured registration intent = %+v, err=%v", intent, err)
	}
	stored, err := repo.GetMessage(ctx, message.ID)
	if err != nil {
		t.Fatalf("read accepted message: %v", err)
	}
	encoded, err := json.Marshal(stored.Metadata["entity_references"])
	if err != nil {
		t.Fatalf("encode accepted references: %v", err)
	}
	var references []apiv1.EntityReference
	if err := json.Unmarshal(encoded, &references); err != nil || len(references) != 1 ||
		references[0].RegistrationStatus != apiv1.EntityReferenceRegistrationPending {
		t.Fatalf("accepted reference status = %+v, err=%v", references, err)
	}
	if updated, err := repo.SetTaskChangeRegistrationIntentResult(ctx, intent.ID, "pending", "failed",
		"provider_unavailable", nil, intent.ExpectedRevision, nil, ""); err != nil || !updated {
		t.Fatalf("record retryable provider failure = (%v, %v)", updated, err)
	}
	retried, replayed, err := repo.ActTaskChangeRegistration(ctx, casTaskID, reference.SelectionID,
		"registration-retry-1", "user-cas", "retry")
	if err != nil || retried == nil || retried.State != "retryable" || retried.OperationID != intent.OperationID ||
		retried.ExpectedRevision == nil || *retried.ExpectedRevision != 7 || replayed {
		t.Fatalf("retry changed the accepted fence: intent=%+v replayed=%v err=%v", retried, replayed, err)
	}
	replayedIntent, replayed, err := repo.ActTaskChangeRegistration(ctx, casTaskID, reference.SelectionID,
		"registration-retry-1", "user-cas", "retry")
	if err != nil || !replayed || replayedIntent == nil || replayedIntent.ID != retried.ID {
		t.Fatalf("retry operation replay = intent=%+v replayed=%v err=%v", replayedIntent, replayed, err)
	}
	stored, err = repo.GetMessage(ctx, message.ID)
	if err != nil {
		t.Fatalf("read retried message: %v", err)
	}
	encoded, err = json.Marshal(stored.Metadata["entity_references"])
	if err != nil {
		t.Fatalf("encode retried references: %v", err)
	}
	if err := json.Unmarshal(encoded, &references); err != nil || len(references) != 1 ||
		references[0].RegistrationStatus != apiv1.EntityReferenceRegistrationPending {
		t.Fatalf("retried reference status = %+v, err=%v", references, err)
	}
}

func selectedPRReference(selectionID string, mode apiv1.EntityReferenceAssociationMode) apiv1.EntityReference {
	return apiv1.EntityReference{
		Version:  apiv1.EntityReferenceVersion,
		Ref:      entityrefs.CanonicalRef("github", "pull_request", "github.com/acme/app", "PR_kwDOAA"),
		Provider: "github", Kind: "pull_request", ID: "PR_kwDOAA", Key: "acme/app#12",
		Title: "Add authentication", URL: "https://github.com/acme/app/pull/12", Scope: "github.com/acme/app",
		SelectionID: selectionID, AssociationMode: mode,
	}
}
