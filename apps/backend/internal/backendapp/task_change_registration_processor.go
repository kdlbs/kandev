package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const (
	registrationWorkerBatchSize          = 32
	registrationWorkerInterval           = time.Second
	registrationReconciliationInterval   = time.Minute
	registrationStatePending             = "pending"
	registrationStateSucceeded           = "succeeded"
	registrationStateFailed              = "failed"
	registrationStateSuppressed          = "suppressed"
	registrationStateUncertain           = "uncertain"
	registrationProviderGitHub           = "github"
	registrationProviderGitLab           = "gitlab"
	registrationOriginSelectedMention    = "selected_mention"
	registrationErrorProviderUnavailable = "provider_unavailable"
)

var errTaskChangeRegistrationCommitUncertain = errors.New("task change registration commit outcome is uncertain")
var errTaskChangeRegistrationAuthorizationRevoked = errors.New("task change registration actor is no longer authorized")
var errTaskChangeRegistrationTargetChanged = errors.New("task change registration target changed")

type githubAcceptedRegistrationProvider interface {
	RegisterExistingPRByURLForWorkspaceAtRevision(
		context.Context, string, string, string, string, string, int64, bool, string, github.TaskPRObservationCommitter,
	) (*github.TaskPR, error)
	RegisterTaskPRObservationTx(context.Context, *sqlx.Tx, *github.TaskPR, bool, int64) (*github.TaskPR, bool, error)
}

type gitlabAcceptedRegistrationProvider interface {
	RegisterExistingMRByURLAtRevision(
		context.Context, string, string, string, string, int64, bool, string, gitlab.TaskMRObservationCommitter,
	) (*gitlab.TaskMR, error)
	RegisterTaskMRObservationTx(context.Context, *sqlx.Tx, *gitlab.TaskMR, bool, int64) (*gitlab.TaskMR, bool, error)
}

type githubRegistrationAssociationReader interface {
	GetTaskPRByRegistrationIdentityIncludingDetached(context.Context, string, string, string, string, int) (*github.TaskPR, error)
}

type gitlabRegistrationAssociationReader interface {
	GetTaskMRByRegistrationIdentityIncludingDetached(context.Context, string, string, string, string, int) (*gitlab.TaskMR, error)
}

type taskChangeRegistrationProcessor struct {
	repository          *tasksqlite.Repository
	tasks               *taskservice.Service
	coordinator         taskChangeLinkCoordinator
	db                  *sqlx.DB
	logger              *commonlogger.Logger
	resolveActor        func(context.Context, string) (authn.Identity, bool)
	queueStatusNotifier func(context.Context, string, string)

	mu      sync.Mutex
	started bool
	wake    chan struct{}
	cancel  context.CancelFunc
	done    chan struct{}
}

func (p *taskChangeRegistrationProcessor) QueueAdmissionRegistrationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity messagequeue.QueueSessionIdentity,
	admissionID string,
	candidate, accepted *messagequeue.QueuedMessage,
) error {
	if candidate == nil || accepted == nil {
		return errors.New("queue registration entry is missing")
	}
	actorID, ok := registrationActorID(ctx)
	if !ok {
		return errTaskChangeRegistrationAuthorizationRevoked
	}
	references := entityrefs.NormalizePersisted(candidate.Metadata[messagequeue.MetadataEntityReferences])
	registered, originIDs, err := p.repository.CreateQueuedTaskChangeRegistrationIntentsTx(
		ctx, tx, identity.TaskID, identity.SessionID, accepted.ID, admissionID, actorID, references,
	)
	if err != nil {
		return err
	}
	accepted.Metadata = copyQueueRegistrationMetadata(accepted.Metadata)
	accepted.Metadata[messagequeue.MetadataEntityReferences] = registered
	accepted.Metadata[messagequeue.MetadataRegistrationOriginIDs] = originIDs
	return nil
}

func (p *taskChangeRegistrationProcessor) QueueEditRegistrationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	operationID string,
	previous, updated *messagequeue.QueuedMessage,
) error {
	if previous == nil || updated == nil {
		return errors.New("queue edit registration entry is missing")
	}
	actorID, ok := registrationActorID(ctx)
	if !ok {
		return errTaskChangeRegistrationAuthorizationRevoked
	}
	previousReferences := entityrefs.NormalizePersisted(previous.Metadata[messagequeue.MetadataEntityReferences])
	updatedReferences := entityrefs.NormalizePersisted(updated.Metadata[messagequeue.MetadataEntityReferences])
	registered, originIDs, err := p.repository.CreateQueueEditTaskChangeRegistrationIntentsTx(
		ctx, tx, updated.TaskID, updated.SessionID, updated.ID, operationID, actorID,
		previousReferences, updatedReferences,
	)
	if err != nil {
		return err
	}
	updated.Metadata = copyQueueRegistrationMetadata(updated.Metadata)
	updated.Metadata[messagequeue.MetadataEntityReferences] = registered
	if len(originIDs) > 0 {
		updated.Metadata[messagequeue.MetadataRegistrationOriginIDs] = originIDs
	}
	return nil
}

func registrationActorID(ctx context.Context) (string, bool) {
	identity, ok := authn.IdentityFromContext(ctx)
	return strings.TrimSpace(identity.UserID), ok && strings.TrimSpace(identity.UserID) != ""
}

func copyQueueRegistrationMetadata(metadata map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{}, len(metadata)+2)
	for key, value := range metadata {
		copy[key] = value
	}
	return copy
}

func (p *taskChangeRegistrationProcessor) SetQueueStatusNotifier(notifier func(context.Context, string, string)) {
	if p != nil {
		p.queueStatusNotifier = notifier
	}
}

func newTaskChangeRegistrationProcessor(
	repository *tasksqlite.Repository,
	tasks *taskservice.Service,
	coordinator taskChangeLinkCoordinator,
	db *sqlx.DB,
	log *commonlogger.Logger,
	resolveActor func(context.Context, string) (authn.Identity, bool),
) *taskChangeRegistrationProcessor {
	return &taskChangeRegistrationProcessor{
		repository: repository, tasks: tasks, coordinator: coordinator, db: db, logger: log, resolveActor: resolveActor,
		wake: make(chan struct{}, 1),
	}
}

func (p *taskChangeRegistrationProcessor) Start(ctx context.Context) {
	if p == nil || p.repository == nil || p.tasks == nil || p.db == nil {
		return
	}
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	p.cancel, p.done, p.started = cancel, done, true
	p.mu.Unlock()
	go p.run(workerCtx, done)
}

func (p *taskChangeRegistrationProcessor) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return
	}
	p.cancel()
	done := p.done
	<-done
	p.cancel, p.done, p.started = nil, nil, false
	p.mu.Unlock()
}

func (p *taskChangeRegistrationProcessor) NotifyAcceptedMessage(taskID, messageID string) {
	if p == nil || taskID == "" || messageID == "" {
		return
	}
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *taskChangeRegistrationProcessor) SubscribeAssociationDetachEvents(eventBus bus.EventBus) ([]bus.Subscription, error) {
	if p == nil || eventBus == nil {
		return nil, nil
	}
	subscriptions := make([]bus.Subscription, 0, 2)
	githubSubscription, err := eventBus.Subscribe(events.GitHubTaskPRDeleted, p.handleGitHubAssociationDetached)
	if err != nil {
		return nil, err
	}
	subscriptions = append(subscriptions, githubSubscription)
	gitlabSubscription, err := eventBus.Subscribe(events.GitLabTaskMRDeleted, p.handleGitLabAssociationDetached)
	if err != nil {
		_ = githubSubscription.Unsubscribe()
		return nil, err
	}
	subscriptions = append(subscriptions, gitlabSubscription)
	return subscriptions, nil
}

func (p *taskChangeRegistrationProcessor) handleGitHubAssociationDetached(ctx context.Context, event *bus.Event) error {
	if event == nil {
		return nil
	}
	payload, ok := decodeRegistrationEventData[github.TaskPRDeletedEvent](event.Data)
	if !ok || payload.TaskID == "" || payload.WorkspaceID == "" || payload.AssociationID == "" {
		return nil
	}
	return p.markAssociationUnlinked(ctx, payload.TaskID, payload.WorkspaceID, "github", payload.AssociationID,
		payload.ProviderHost, payload.ConnectionScope, payload.ProviderRepositoryID, payload.ChangeRequestID, payload.AssociationRevision)
}

func (p *taskChangeRegistrationProcessor) handleGitLabAssociationDetached(ctx context.Context, event *bus.Event) error {
	if event == nil {
		return nil
	}
	payload, ok := decodeRegistrationEventData[gitlab.TaskMRDeletedEvent](event.Data)
	if !ok || payload.TaskID == "" || payload.WorkspaceID == "" || payload.AssociationID == "" {
		return nil
	}
	return p.markAssociationUnlinked(ctx, payload.TaskID, payload.WorkspaceID, "gitlab", payload.AssociationID,
		payload.ProviderHost, payload.ConnectionScope, payload.ProviderRepositoryID, payload.ChangeRequestID, payload.AssociationRevision)
}

func decodeRegistrationEventData[T any](data any) (*T, bool) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, false
	}
	var decoded T
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return nil, false
	}
	return &decoded, true
}

func (p *taskChangeRegistrationProcessor) markAssociationUnlinked(
	ctx context.Context, taskID, workspaceID, provider, associationID, providerHost, connectionScope, providerRepositoryID, changeRequestID string,
	revision int64,
) error {
	notifications, err := p.repository.MarkTaskChangeRegistrationUnlinked(
		ctx, taskID, workspaceID, provider, associationID, providerHost, connectionScope, providerRepositoryID, changeRequestID, revision,
	)
	if err != nil {
		p.logError("project task change registration unlink", taskID, err)
		return err
	}
	for _, messageID := range notifications.MessageIDs {
		if err := p.notifyMessage(ctx, messageID); err != nil {
			p.logError("publish unlinked task change registration", taskID, err)
		}
	}
	for _, sessionID := range notifications.QueueSessionIDs {
		p.notifyQueueStatus(ctx, taskID, sessionID)
	}
	return nil
}

func (p *taskChangeRegistrationProcessor) ActTaskChangeRegistration(
	ctx context.Context, taskID, registrationID, operationID, actorID, action string,
) error {
	if p == nil || p.repository == nil {
		return errors.New("task change registration is unavailable")
	}
	intent, _, err := p.repository.ActTaskChangeRegistration(ctx, taskID, registrationID, operationID, actorID, action)
	if err != nil {
		return err
	}
	if intent == nil {
		return tasksqlite.ErrTaskChangeRegistrationConflict
	}
	p.NotifyAcceptedMessage(taskID, intent.OriginID)
	return p.notifyRegistrationSource(ctx, intent)
}

func (p *taskChangeRegistrationProcessor) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(registrationWorkerInterval)
	defer ticker.Stop()
	reconcileTicker := time.NewTicker(registrationReconciliationInterval)
	defer reconcileTicker.Stop()
	p.reconcileSucceeded(ctx)
	p.processPending(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.wake:
			p.processPending(ctx)
		case <-ticker.C:
			p.processPending(ctx)
		case <-reconcileTicker.C:
			p.reconcileSucceeded(ctx)
		}
	}
}

func (p *taskChangeRegistrationProcessor) reconcileSucceeded(ctx context.Context) {
	if p == nil || p.repository == nil {
		return
	}
	afterID := ""
	for {
		intents, err := p.repository.ListSucceededTaskChangeRegistrationIntentsAfter(ctx, afterID, registrationWorkerBatchSize)
		if err != nil {
			p.logError("list completed task change registration intents", "", err)
			return
		}
		if len(intents) == 0 {
			return
		}
		for _, intent := range intents {
			if intent == nil {
				continue
			}
			afterID = intent.ID
			if err := p.reconcileSucceededIntent(ctx, intent); err != nil {
				p.logError("reconcile completed task change registration", intent.TaskID, err)
			}
		}
		if len(intents) < registrationWorkerBatchSize {
			return
		}
	}
}

func (p *taskChangeRegistrationProcessor) reconcileSucceededIntent(
	ctx context.Context, intent *tasksqlite.TaskChangeRegistrationIntent,
) error {
	if intent == nil || intent.State != registrationStateSucceeded || intent.AssociationRevision == nil || *intent.AssociationRevision <= 0 {
		return nil
	}
	changeNumber, err := parseRegistrationChangeNumber(intent.ChangeRequestID)
	if err != nil {
		return nil
	}
	switch intent.Provider {
	case registrationProviderGitHub:
		return p.reconcileGitHubAssociation(ctx, intent, changeNumber)
	case registrationProviderGitLab:
		return p.reconcileGitLabAssociation(ctx, intent, changeNumber)
	default:
		return nil
	}
}

func (p *taskChangeRegistrationProcessor) reconcileGitHubAssociation(
	ctx context.Context, intent *tasksqlite.TaskChangeRegistrationIntent, changeNumber int,
) error {
	reader, ok := p.coordinator.github.(githubRegistrationAssociationReader)
	if !ok {
		return nil
	}
	association, err := reader.GetTaskPRByRegistrationIdentityIncludingDetached(
		ctx, intent.TaskID, intent.ProviderHost, intent.ConnectionScope, intent.ProviderRepositoryID, changeNumber,
	)
	if err != nil {
		return err
	}
	if association == nil || association.DetachedAt == nil || association.AssociationRevision <= *intent.AssociationRevision {
		return nil
	}
	return p.markIntentAssociationUnlinked(ctx, intent, association.ID, association.AssociationRevision)
}

func (p *taskChangeRegistrationProcessor) reconcileGitLabAssociation(
	ctx context.Context, intent *tasksqlite.TaskChangeRegistrationIntent, changeNumber int,
) error {
	reader, ok := p.coordinator.gitlab.(gitlabRegistrationAssociationReader)
	if !ok {
		return nil
	}
	association, err := reader.GetTaskMRByRegistrationIdentityIncludingDetached(
		ctx, intent.TaskID, intent.ConnectionScope, intent.ProviderRepositoryID, intent.ProviderHost, changeNumber,
	)
	if err != nil {
		return err
	}
	if association == nil || association.DetachedAt == nil || association.AssociationRevision <= *intent.AssociationRevision {
		return nil
	}
	return p.markIntentAssociationUnlinked(ctx, intent, association.ID, association.AssociationRevision)
}

func (p *taskChangeRegistrationProcessor) markIntentAssociationUnlinked(
	ctx context.Context,
	intent *tasksqlite.TaskChangeRegistrationIntent,
	associationID string,
	revision int64,
) error {
	return p.markAssociationUnlinked(
		ctx, intent.TaskID, intent.WorkspaceID, intent.Provider, associationID,
		intent.ProviderHost, intent.ConnectionScope, intent.ProviderRepositoryID, intent.ChangeRequestID, revision,
	)
}

func (p *taskChangeRegistrationProcessor) processPending(ctx context.Context) {
	intents, err := p.repository.ListPendingTaskChangeRegistrationIntents(ctx, registrationWorkerBatchSize)
	if err != nil {
		p.logError("list pending task change registration intents", "", err)
		return
	}
	for _, intent := range intents {
		if ctx.Err() != nil {
			return
		}
		if !registrationIntentHasMessageSource(intent) {
			continue
		}
		if err := p.processIntent(ctx, intent); err != nil {
			p.logError("process task change registration intent", intent.TaskID, err)
		}
	}
}

func registrationIntentHasMessageSource(intent *tasksqlite.TaskChangeRegistrationIntent) bool {
	return intent != nil && (intent.SourceKind == "message" ||
		intent.SourceKind == tasksqlite.TaskChangeRegistrationSourceQueueAdmission ||
		intent.SourceKind == tasksqlite.TaskChangeRegistrationSourceQueueEdit)
}

func (p *taskChangeRegistrationProcessor) processIntent(
	ctx context.Context, intent *tasksqlite.TaskChangeRegistrationIntent,
) error {
	if intent.ExpectedRevision == nil || intent.RepositoryID == "" || intent.ProviderRepositoryID == "" ||
		intent.ConnectionScope != "builtin:"+intent.WorkspaceID {
		return p.finishIntent(ctx, intent, "failed", "identity_unresolved", nil, "")
	}
	if intent.ActorID == "" {
		return p.finishIntent(ctx, intent, "failed", "identity_unresolved", nil, "")
	}
	var reference v1.EntityReference
	if err := json.Unmarshal([]byte(intent.ReferenceSnapshot), &reference); err != nil {
		return p.finishIntent(ctx, intent, "failed", "reference_invalid", nil, "")
	}
	actorCtx, task, repository, err := p.authorizedRegistrationTarget(ctx, intent)
	if errors.Is(err, errTaskChangeRegistrationAuthorizationRevoked) {
		return p.finishIntent(ctx, intent, "failed", "authorization_revoked", nil, "")
	}
	if err != nil || task == nil || repository == nil {
		return p.finishIntent(ctx, intent, registrationStateSuppressed, "repository_changed", nil, "")
	}
	return p.processAuthorizedIntent(ctx, actorCtx, intent, task, repository)
}

func (p *taskChangeRegistrationProcessor) processAuthorizedIntent(
	ctx, actorCtx context.Context,
	intent *tasksqlite.TaskChangeRegistrationIntent,
	task *models.Task,
	repository *models.Repository,
) error {
	switch intent.Provider {
	case registrationProviderGitHub:
		result, err := p.processGitHubIntent(actorCtx, intent, task, repository)
		if err != nil {
			return p.finishProviderError(ctx, intent, err)
		}
		return p.finishAcceptedIntent(ctx, intent, result)
	case registrationProviderGitLab:
		result, err := p.processGitLabIntent(actorCtx, intent, task, repository)
		if err != nil {
			return p.finishProviderError(ctx, intent, err)
		}
		return p.finishAcceptedIntent(ctx, intent, result)
	default:
		return p.finishIntent(ctx, intent, registrationStateFailed, registrationErrorProviderUnavailable, nil, "")
	}
}

type taskChangeRegistrationResult struct {
	identity json.RawMessage
	revision int64
	receipt  string
}

func (p *taskChangeRegistrationProcessor) processGitHubIntent(
	actorCtx context.Context,
	intent *tasksqlite.TaskChangeRegistrationIntent,
	task *models.Task,
	repository *models.Repository,
) (*taskChangeRegistrationResult, error) {
	provider, ok := p.coordinator.github.(githubAcceptedRegistrationProvider)
	if !ok {
		return nil, fmt.Errorf("GitHub registration provider is unavailable")
	}
	changeNumber, err := parseRegistrationChangeNumber(intent.ChangeRequestID)
	if err != nil {
		return nil, fmt.Errorf("invalid registration change request number: %w", err)
	}
	changeURL, err := githubChangeURL(
		repository.ProviderHost, repository.ProviderOwner, repository.ProviderName, changeNumber,
	)
	if err != nil {
		return nil, err
	}
	fence := *intent.ExpectedRevision
	registered, err := provider.RegisterExistingPRByURLForWorkspaceAtRevision(
		actorCtx, task.WorkspaceID, intent.ActorID, task.ID, repository.ID, changeURL, fence,
		fence > 0, intent.OperationID,
		func(commitCtx context.Context, registration *github.TaskPR, restore bool, expected int64) (*github.TaskPR, bool, error) {
			registration.Source = github.TaskPRSourceSelectedMention
			registration.FirstOriginKind = registrationOriginSelectedMention
			registration.FirstOriginID = intent.OriginID
			registration.FirstOriginRevision = intent.SourceRevision
			return commitAcceptedRegistration(
				p, commitCtx, intent, expected,
				func(ctx context.Context, tx *sqlx.Tx) (*github.TaskPR, bool, error) {
					return provider.RegisterTaskPRObservationTx(ctx, tx, registration, restore, expected)
				},
				registrationResultFromGitHub,
			)
		},
	)
	if err != nil {
		return nil, err
	}
	return &taskChangeRegistrationResult{
		identity: taskChangeRegistrationIdentityFromGitHub(registered),
		revision: registered.AssociationRevision,
		receipt:  registered.ID,
	}, nil
}

func (p *taskChangeRegistrationProcessor) processGitLabIntent(
	actorCtx context.Context,
	intent *tasksqlite.TaskChangeRegistrationIntent,
	task *models.Task,
	repository *models.Repository,
) (*taskChangeRegistrationResult, error) {
	provider, ok := p.coordinator.gitlab.(gitlabAcceptedRegistrationProvider)
	if !ok {
		return nil, fmt.Errorf("GitLab registration provider is unavailable")
	}
	changeNumber, err := parseRegistrationChangeNumber(intent.ChangeRequestID)
	if err != nil {
		return nil, fmt.Errorf("invalid registration change request number: %w", err)
	}
	changeURL, err := gitlabChangeURL(
		repository.ProviderHost, repository.ProviderOwner, repository.ProviderName, changeNumber,
	)
	if err != nil {
		return nil, err
	}
	fence := *intent.ExpectedRevision
	registered, err := provider.RegisterExistingMRByURLAtRevision(
		actorCtx, task.WorkspaceID, task.ID, repository.ID, changeURL, fence, fence > 0, intent.OperationID,
		func(commitCtx context.Context, registration *gitlab.TaskMR, restore bool, expected int64) (*gitlab.TaskMR, bool, error) {
			registration.Source = gitlab.TaskMRSourceSelectedMention
			registration.FirstOriginKind = registrationOriginSelectedMention
			registration.FirstOriginID = intent.OriginID
			registration.FirstOriginRevision = intent.SourceRevision
			return commitAcceptedRegistration(
				p, commitCtx, intent, expected,
				func(ctx context.Context, tx *sqlx.Tx) (*gitlab.TaskMR, bool, error) {
					return provider.RegisterTaskMRObservationTx(ctx, tx, registration, restore, expected)
				},
				registrationResultFromGitLab,
			)
		},
	)
	if err != nil {
		return nil, err
	}
	return &taskChangeRegistrationResult{
		identity: taskChangeRegistrationIdentityFromGitLab(registered),
		revision: registered.AssociationRevision,
		receipt:  registered.ID,
	}, nil
}

func (p *taskChangeRegistrationProcessor) finishAcceptedIntent(
	ctx context.Context,
	intent *tasksqlite.TaskChangeRegistrationIntent,
	result *taskChangeRegistrationResult,
) error {
	current, readErr := p.repository.GetTaskChangeRegistrationIntent(ctx, intent.TaskID, intent.ID)
	if readErr != nil || current == nil || current.State == registrationStateSucceeded {
		return p.notifyRegistrationSource(ctx, intent)
	}
	return p.finishIntentWithAssociationRevision(
		ctx, intent, registrationStateSucceeded, "", result.identity,
		receiptWithRevision(result.receipt, result.revision), int64Pointer(result.revision),
	)
}

func commitAcceptedRegistration[T any](
	p *taskChangeRegistrationProcessor,
	ctx context.Context,
	intent *tasksqlite.TaskChangeRegistrationIntent,
	expected int64,
	register func(context.Context, *sqlx.Tx) (*T, bool, error),
	result func(*T) taskChangeRegistrationResult,
) (*T, bool, error) {
	authorizedCtx, _, _, err := p.authorizedRegistrationTarget(ctx, intent)
	if err != nil {
		return nil, false, err
	}
	tx, err := p.db.BeginTxx(authorizedCtx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	stored, changed, err := register(authorizedCtx, tx)
	if err != nil {
		return nil, false, err
	}
	registrationResult := result(stored)
	if err := p.persistSuccessTx(
		authorizedCtx, tx, intent, registrationResult.identity, expected,
		registrationResult.revision, registrationResult.receipt,
	); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("%w: %v", errTaskChangeRegistrationCommitUncertain, err)
	}
	return stored, changed, nil
}

func registrationResultFromGitHub(registration *github.TaskPR) taskChangeRegistrationResult {
	return taskChangeRegistrationResult{
		identity: taskChangeRegistrationIdentityFromGitHub(registration),
		revision: registration.AssociationRevision,
		receipt:  registration.ID,
	}
}

func registrationResultFromGitLab(registration *gitlab.TaskMR) taskChangeRegistrationResult {
	return taskChangeRegistrationResult{
		identity: taskChangeRegistrationIdentityFromGitLab(registration),
		revision: registration.AssociationRevision,
		receipt:  registration.ID,
	}
}

func (p *taskChangeRegistrationProcessor) persistSuccessTx(
	ctx context.Context,
	tx *sqlx.Tx,
	intent *tasksqlite.TaskChangeRegistrationIntent,
	resolvedIdentity json.RawMessage,
	expectedRevision, associationRevision int64,
	receipt string,
) error {
	updated, err := p.repository.SetTaskChangeRegistrationIntentResultTx(
		ctx, tx, intent.ID, intent.State, "succeeded", "", resolvedIdentity,
		int64Pointer(expectedRevision), int64Pointer(associationRevision), receipt,
	)
	if err != nil {
		return err
	}
	if !updated {
		return tasksqlite.ErrTaskChangeRegistrationConflict
	}
	current, err := p.repository.GetTaskChangeRegistrationIntentTx(ctx, tx, intent.TaskID, intent.ID)
	if err != nil || current == nil {
		return err
	}
	_, err = p.repository.ProjectTaskChangeRegistrationStatusTx(ctx, tx, current, "succeeded", "")
	return err
}

func (p *taskChangeRegistrationProcessor) finishIntent(
	ctx context.Context,
	intent *tasksqlite.TaskChangeRegistrationIntent,
	state, errorCode string,
	resolvedIdentity json.RawMessage,
	operationReceipt string,
) error {
	return p.finishIntentWithAssociationRevision(ctx, intent, state, errorCode, resolvedIdentity, operationReceipt, nil)
}

func (p *taskChangeRegistrationProcessor) finishIntentWithAssociationRevision(
	ctx context.Context,
	intent *tasksqlite.TaskChangeRegistrationIntent,
	state, errorCode string,
	resolvedIdentity json.RawMessage,
	operationReceipt string,
	associationRevision *int64,
) error {
	tx, err := p.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	updated, err := p.repository.SetTaskChangeRegistrationIntentResultTx(
		ctx, tx, intent.ID, intent.State, state, errorCode, resolvedIdentity,
		intent.ExpectedRevision, associationRevision, operationReceipt,
	)
	if err != nil {
		return err
	}
	if !updated {
		current, readErr := p.repository.GetTaskChangeRegistrationIntentTx(ctx, tx, intent.TaskID, intent.ID)
		if readErr == nil && current != nil && current.State == state {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				return rollbackErr
			}
			return p.notifyRegistrationSource(ctx, current)
		}
		return tasksqlite.ErrTaskChangeRegistrationConflict
	}
	current, err := p.repository.GetTaskChangeRegistrationIntentTx(ctx, tx, intent.TaskID, intent.ID)
	if err != nil || current == nil {
		return err
	}
	if _, err := p.repository.ProjectTaskChangeRegistrationStatusTx(ctx, tx, current, state, errorCode); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return p.notifyRegistrationSource(ctx, intent)
}

func (p *taskChangeRegistrationProcessor) notifyRegistrationSource(
	ctx context.Context, fallback *tasksqlite.TaskChangeRegistrationIntent,
) error {
	if fallback == nil || p == nil || p.repository == nil {
		return nil
	}
	intent, err := p.repository.GetTaskChangeRegistrationIntent(ctx, fallback.TaskID, fallback.ID)
	if err != nil {
		return err
	}
	if intent == nil {
		intent = fallback
	}
	messageID := intent.MessageID
	if intent.SourceKind == "message" && messageID == "" {
		messageID = intent.OriginID
	}
	if messageID != "" {
		return p.notifyMessage(ctx, messageID)
	}
	if intent.QueueEntryID == "" || p.queueStatusNotifier == nil {
		return nil
	}
	sessionID, found, err := p.repository.GetQueuedRegistrationSession(ctx, intent.TaskID, intent.SelectionID)
	if err != nil {
		return err
	}
	if found {
		p.notifyQueueStatus(ctx, intent.TaskID, sessionID)
	}
	return nil
}

func (p *taskChangeRegistrationProcessor) notifyQueueStatus(ctx context.Context, taskID, sessionID string) {
	if p != nil && p.queueStatusNotifier != nil && taskID != "" && sessionID != "" {
		p.queueStatusNotifier(context.WithoutCancel(ctx), taskID, sessionID)
	}
}

func (p *taskChangeRegistrationProcessor) notifyMessage(ctx context.Context, messageID string) error {
	notifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := p.tasks.NotifyMessageUpdated(notifyCtx, messageID); err != nil {
		p.logError("publish task change registration message status", "", err)
		return err
	}
	return nil
}

func (p *taskChangeRegistrationProcessor) logError(operation, taskID string, err error) {
	if p.logger == nil || err == nil {
		return
	}
	p.logger.Warn("task change registration processing failed",
		zap.String("operation", operation), zap.String("task_id", taskID), zap.Error(err))
}

func taskChangeRegistrationIdentityFromGitHub(pr *github.TaskPR) json.RawMessage {
	encoded, _ := json.Marshal(map[string]string{
		"provider": "github", "host": pr.ProviderHost, "scope": pr.ConnectionScope,
		"repository_id": pr.ProviderRepositoryID, "change_request_id": fmt.Sprint(pr.PRNumber),
	})
	return encoded
}

func taskChangeRegistrationIdentityFromGitLab(mr *gitlab.TaskMR) json.RawMessage {
	encoded, _ := json.Marshal(map[string]string{
		"provider": "gitlab", "host": mr.Host, "scope": mr.ConnectionScope,
		"repository_id": mr.ProviderRepositoryID, "change_request_id": fmt.Sprint(mr.MRIID),
	})
	return encoded
}

func parseRegistrationChangeNumber(value string) (int, error) {
	var number int
	if _, err := fmt.Sscanf(value, "%d", &number); err != nil || number <= 0 {
		return 0, fmt.Errorf("invalid registration change request number")
	}
	return number, nil
}

func receiptWithRevision(receipt string, revision int64) string {
	return fmt.Sprintf("%s:%d", receipt, revision)
}
