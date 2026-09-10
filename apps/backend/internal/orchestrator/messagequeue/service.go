// Package messagequeue persists per-session FIFO message queues and deferred
// workflow moves. The queue lets a user (or another agent via the
// message_task_kandev MCP tool) enqueue follow-up prompts while the target
// session is still busy with a turn; handleAgentReady drains one entry per
// turn after each turn completes.
//
// Storage is abstracted behind Repository (SQLite in production, in-memory
// for tests). The service enforces a per-session cap (default 10) and surfaces
// overflow as ErrQueueFull rather than silently dropping the new message.
package messagequeue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

type autoMergePolicyLoader struct {
	load func(context.Context) (bool, int64, error)
}

// Service manages queued messages for sessions, backed by Repository.
type Service struct {
	repo            Repository
	maxPerSession   atomic.Int64
	mergeEnabled    atomic.Bool
	autoMergePolicy atomic.Pointer[AutoMergePolicy]
	autoMergeLoader atomic.Pointer[autoMergePolicyLoader]
	statusEpoch     string
	logger          *logger.Logger
	// lifecycleMu fences task-wide purges against every session-scoped queue
	// admission. A task purge cannot enumerate every possibly empty session,
	// so the barrier must cover admissions before they reach the repository.
	lifecycleMu                                   sync.RWMutex
	admissionMu                                   sync.Mutex
	admissions                                    map[string]*sessionAdmission
	editLeaseMu                                   sync.Mutex
	editLeases                                    map[editLeaseKey]*QueueEditLease
	editRevisions                                 map[editLeaseKey]int64
	editRevisionTaskIDs                           map[editLeaseKey]string
	sessionTransferCompensationLeaseRenewInterval time.Duration
}

type sessionAdmission struct {
	mu   sync.Mutex
	refs int
}

type sessionAdmissionContextKey struct{}

type sessionAdmissionToken struct {
	service              *Service
	sessionID            string
	afterLifecycleUnlock *[]func()
}

type queueAttachmentRepository interface {
	InsertForSessionWithClaim(context.Context, QueueSessionIdentity, *QueuedMessage, QueueAttachmentClaim, int) error
	AutoMergeCandidateIntoAboveForSessionWithClaim(context.Context, QueueSessionIdentity, *QueuedMessage, QueueAttachmentClaim) (*QueuedMessage, bool, error)
	UpdateContentAndMetadataForSessionWithClaim(context.Context, QueueSessionIdentity, string, string, []MessageAttachment, map[string]interface{}, string, QueueAttachmentClaim) error
}

type deletedSessionRepository interface {
	PurgeDeletedSession(context.Context, QueueSessionIdentity) error
}

type autoMergeAdmissionRepository interface {
	InsertForSessionWithPolicy(
		context.Context,
		QueueSessionIdentity,
		*QueuedMessage,
		*QueueAttachmentClaim,
		int,
		AutoMergePolicy,
	) error
	AutoMergeCandidateIntoAboveForSessionWithPolicy(
		context.Context,
		QueueSessionIdentity,
		*QueuedMessage,
		*QueueAttachmentClaim,
		AutoMergePolicy,
	) (*QueuedMessage, bool, error)
	AutoMergeIntoAboveForSessionWithPolicy(
		context.Context,
		QueueSessionIdentity,
		string,
		AutoMergePolicy,
	) (*QueuedMessage, bool, error)
}

// NewService creates a Service backed by the supplied repository. maxPerSession
// is the per-session cap (entries beyond this return ErrQueueFull on insert);
// pass 0 to disable the cap.
func NewService(repo Repository, maxPerSession int, log *logger.Logger) *Service {
	service := &Service{
		repo:                repo,
		statusEpoch:         uuid.NewString(),
		logger:              log.WithFields(zap.String("component", "message-queue")),
		admissions:          make(map[string]*sessionAdmission),
		editLeases:          make(map[editLeaseKey]*QueueEditLease),
		editRevisions:       make(map[editLeaseKey]int64),
		editRevisionTaskIDs: make(map[editLeaseKey]string),
	}
	service.SetMaxPerSession(maxPerSession)
	service.mergeEnabled.Store(true)
	service.SetAutoMergePolicy(true, 0)
	return service
}

// NewServiceMemory returns a Service backed by an in-memory repository.
// Convenience constructor for tests; cap defaults to 10 for parity with prod.
func NewServiceMemory(log *logger.Logger) *Service {
	return NewService(NewMemoryRepository(), DefaultMaxPerSession, log)
}

// ResolveSessionIdentity returns the repository-authoritative immutable queue
// identity for trusted internal callers migrating legacy session snapshots.
func (s *Service) ResolveSessionIdentity(ctx context.Context, taskID, sessionID string) (QueueSessionIdentity, error) {
	return s.repo.ResolveSessionIdentity(ctx, taskID, sessionID)
}

// SupportsAtomicDeferredMoveTransition reports whether task and queue rows share
// one SQL transaction. The in-memory queue cannot participate in task storage.
func (s *Service) SupportsAtomicDeferredMoveTransition() bool {
	_, ok := s.repo.(*sqliteRepository)
	return ok
}

// MaxPerSession returns the configured per-session cap.
func (s *Service) MaxPerSession() int { return int(s.maxPerSession.Load()) }

// LifecycleGeneration returns the task archive/delete generation captured by
// send-now restoration guards.
func (s *Service) LifecycleGeneration(ctx context.Context, taskID string) (int64, error) {
	return s.repo.LifecycleGeneration(ctx, taskID)
}

// ListDurableLifecycleEntries returns lifecycle rows that remain persisted
// until executor acceptance. Startup recovery uses it to reconcile queue rows
// with their owning automation reservations.
func (s *Service) ListDurableLifecycleEntries(ctx context.Context) ([]QueuedMessage, error) {
	return s.repo.ListDurableLifecycleEntries(ctx)
}

// SessionGeneration returns the session destructive-mutation generation used
// to fence Send Now restores.
func (s *Service) SessionGeneration(ctx context.Context, sessionID string) (int64, error) {
	return s.repo.SessionGeneration(ctx, sessionID)
}

// SetMaxPerSession applies a new admission cap without pruning existing rows.
// Non-positive values disable the cap.
func (s *Service) SetMaxPerSession(maxPerSession int) {
	if maxPerSession < 0 {
		maxPerSession = 0
	}
	s.maxPerSession.Store(int64(maxPerSession))
}

// MergeEnabled reports whether MergeIntoAbove is currently allowed. Enabled
// by default; see SetMergeEnabled.
func (s *Service) MergeEnabled() bool { return s.mergeEnabled.Load() }

// SetMergeEnabled toggles whether queued messages may be folded into the
// entry above them via MergeIntoAbove. Disabling it does not affect merges
// already applied.
func (s *Service) SetMergeEnabled(enabled bool) {
	s.mergeEnabled.Store(enabled)
}

// AutoMergeEnabled reports the global admission policy used by sessions that
// do not have an explicit override.
func (s *Service) AutoMergeEnabled() bool {
	return s.globalAutoMergePolicy().Enabled
}

// SetAutoMergeEnabled updates the global policy while preserving the legacy
// boolean target used by callers that do not own the persisted revision.
func (s *Service) SetAutoMergeEnabled(enabled bool) {
	current := s.globalAutoMergePolicy()
	revision := current.Revision
	if current.Enabled != enabled {
		revision++
	}
	s.SetAutoMergePolicy(enabled, revision)
}

// SetAutoMergePolicy atomically publishes a complete global policy snapshot.
func (s *Service) SetAutoMergePolicy(enabled bool, revision int64) {
	policy := &AutoMergePolicy{
		Enabled:  enabled,
		Source:   AutoMergeSourceGlobal,
		Revision: revision,
	}
	s.autoMergePolicy.Store(policy)
}

// SetAutoMergePolicyLoader installs the shared persisted-policy reader used by
// identity-bound admissions and snapshots. The atomic cache remains available
// to legacy callers that cannot carry a context.
func (s *Service) SetAutoMergePolicyLoader(
	load func(context.Context) (bool, int64, error),
) {
	if load == nil {
		s.autoMergeLoader.Store(nil)
		return
	}
	s.autoMergeLoader.Store(&autoMergePolicyLoader{load: load})
}

func (s *Service) globalAutoMergePolicy() AutoMergePolicy {
	policy := s.autoMergePolicy.Load()
	if policy == nil {
		return AutoMergePolicy{Enabled: true, Source: AutoMergeSourceGlobal}
	}
	return *policy
}

// ResolveAutoMergePolicy returns the session override when present, otherwise
// the current global policy snapshot.
func (s *Service) ResolveAutoMergePolicy(
	ctx context.Context,
	identity QueueSessionIdentity,
) (AutoMergePolicy, error) {
	override, err := s.repo.GetAutoMergeOverride(ctx, identity)
	if err != nil {
		return AutoMergePolicy{}, err
	}
	if override == nil {
		return s.loadGlobalAutoMergePolicy(ctx)
	}
	return AutoMergePolicy{
		Enabled:  override.Enabled,
		Source:   AutoMergeSourceSession,
		Revision: override.Revision,
	}, nil
}

func (s *Service) loadGlobalAutoMergePolicy(ctx context.Context) (AutoMergePolicy, error) {
	if loader := s.autoMergeLoader.Load(); loader != nil {
		enabled, revision, err := loader.load(ctx)
		if err != nil {
			return AutoMergePolicy{}, err
		}
		s.SetAutoMergePolicy(enabled, revision)
	}
	return s.globalAutoMergePolicy(), nil
}

// SetSessionAutoMerge persists and returns an explicit per-session policy.
func (s *Service) SetSessionAutoMerge(
	ctx context.Context,
	identity QueueSessionIdentity,
	enabled bool,
) (AutoMergePolicy, error) {
	var override AutoMergeOverride
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		var setErr error
		override, setErr = s.repo.SetAutoMergeOverride(admittedCtx, identity, enabled)
		return setErr
	})
	if err != nil {
		return AutoMergePolicy{}, err
	}
	return AutoMergePolicy{
		Enabled:  override.Enabled,
		Source:   AutoMergeSourceSession,
		Revision: override.Revision,
	}, nil
}

// WithSessionAdmission runs fn under the per-session queue admission lock.
// All queue insertion and mutation paths use the same lock. The callback must
// complete synchronously; queue methods called with its context reuse the held
// lock.
func (s *Service) WithSessionAdmission(ctx context.Context, sessionID string, fn func(context.Context) error) error {
	if fn == nil {
		return errors.New("session admission callback is nil")
	}
	if token, ok := ctx.Value(sessionAdmissionContextKey{}).(sessionAdmissionToken); ok &&
		token.service == s {
		if token.sessionID == sessionID {
			return fn(ctx)
		}
		return s.withSessionAdmissionLock(ctx, sessionID, token.afterLifecycleUnlock, fn)
	}

	var afterLifecycleUnlock []func()
	s.lifecycleMu.RLock()
	defer func() {
		s.lifecycleMu.RUnlock()
		for _, callback := range afterLifecycleUnlock {
			callback()
		}
	}()
	return s.withSessionAdmissionLock(ctx, sessionID, &afterLifecycleUnlock, fn)
}

func (s *Service) withSessionAdmissionLock(
	ctx context.Context,
	sessionID string,
	afterLifecycleUnlock *[]func(),
	fn func(context.Context) error,
) error {
	s.admissionMu.Lock()
	entry := s.admissions[sessionID]
	if entry == nil {
		entry = &sessionAdmission{}
		s.admissions[sessionID] = entry
	}
	entry.refs++
	s.admissionMu.Unlock()

	entry.mu.Lock()
	defer func() {
		entry.mu.Unlock()
		s.admissionMu.Lock()
		entry.refs--
		if entry.refs == 0 && s.admissions[sessionID] == entry {
			delete(s.admissions, sessionID)
		}
		s.admissionMu.Unlock()
	}()

	admittedCtx := context.WithValue(ctx, sessionAdmissionContextKey{}, sessionAdmissionToken{
		service: s, sessionID: sessionID, afterLifecycleUnlock: afterLifecycleUnlock,
	})
	return fn(admittedCtx)
}

// withSessionAdmissions holds both queue admission locks in stable order.
// Transfer and replacement operations otherwise allow opposite-direction
// transfers to deadlock while each session is waiting for the other lock.
func (s *Service) withSessionAdmissions(
	ctx context.Context,
	firstSessionID, secondSessionID string,
	fn func(context.Context) error,
) error {
	if firstSessionID == secondSessionID {
		return s.WithSessionAdmission(ctx, firstSessionID, fn)
	}
	if firstSessionID > secondSessionID {
		firstSessionID, secondSessionID = secondSessionID, firstSessionID
	}
	return s.WithSessionAdmission(ctx, firstSessionID, func(admittedCtx context.Context) error {
		return s.WithSessionAdmission(admittedCtx, secondSessionID, fn)
	})
}

type editLeaseKey struct {
	sessionID string
	entryID   string
}

func (s *Service) editLeaseKey(sessionID, entryID string) editLeaseKey {
	return editLeaseKey{sessionID: sessionID, entryID: entryID}
}

func editOperationHash(content string, attachments []MessageAttachment, metadata map[string]interface{}) string {
	payload, _ := json.Marshal(struct {
		Content     string
		Attachments []MessageAttachment
		Metadata    map[string]interface{}
	}{content, attachments, metadata})
	return fmt.Sprintf("%x", sha256.Sum256(payload))
}

func (s *Service) expireEditLeaseLocked(key editLeaseKey, now time.Time) {
	if lease := s.editLeases[key]; lease != nil && !lease.ExpiresAt.After(now) {
		delete(s.editLeases, key)
	}
}

type sessionTransferFenceRepository interface {
	withSessionTransferFence(context.Context, string, func(context.Context) error) error
}

type editReplayValidationRepository interface {
	validateSessionEntryForEditReplay(context.Context, string, string) error
}

type editLeaseRepository interface {
	acquireEditLease(context.Context, *QueueEditLease) error
	renewEditLease(context.Context, *QueueEditLease) error
	releaseEditLease(context.Context, string, string, string) error
}

func (s *Service) validateDuplicateEditReplay(
	ctx context.Context,
	sessionID, entryID string,
) error {
	repo, ok := s.repo.(editReplayValidationRepository)
	if !ok {
		_, err := s.findQueuedMessageForEdit(ctx, sessionID, entryID)
		return err
	}
	return repo.validateSessionEntryForEditReplay(ctx, sessionID, entryID)
}

func (s *Service) withRepositorySessionTransferFence(
	ctx context.Context,
	sessionID string,
	fn func(context.Context) error,
) error {
	repo, ok := s.repo.(sessionTransferFenceRepository)
	if !ok {
		return fn(ctx)
	}
	return repo.withSessionTransferFence(ctx, sessionID, fn)
}

func (s *Service) editableEntry(
	ctx context.Context,
	sessionID, entryID string,
) (*QueuedMessage, error) {
	entries, err := s.repo.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].ID == entryID &&
			entries[i].QueuedBy == QueuedByUser &&
			!entries[i].IsReservedInFlight() {
			return &entries[i], nil
		}
	}
	return nil, ErrEditLeaseNotFound
}

// BeginEdit acquires a target-bound lease for a visible user-owned entry.
func (s *Service) BeginEdit(ctx context.Context, sessionID, entryID, connectionID string) (*QueueEditLease, error) {
	if sessionID == "" || entryID == "" || connectionID == "" {
		return nil, ErrEditLeaseNotFound
	}
	var lease *QueueEditLease
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		entry, err := s.editableEntry(admittedCtx, sessionID, entryID)
		if err != nil {
			return err
		}
		key := s.editLeaseKey(sessionID, entryID)
		now := time.Now().UTC()
		s.editLeaseMu.Lock()
		s.expireEditLeaseLocked(key, now)
		if s.editLeases[key] != nil {
			s.editLeaseMu.Unlock()
			return ErrEditConflict
		}
		lease = &QueueEditLease{
			SessionID: sessionID, EntryID: entryID, LeaseID: uuid.NewString(),
			TargetRevision: s.editRevisions[key], LeaseGeneration: 1,
			ExpiresAt: now.Add(QueueEditLeaseTTL), connectionID: connectionID,
			taskID: entry.TaskID,
		}
		s.editLeaseMu.Unlock()
		if repo, ok := s.repo.(editLeaseRepository); ok {
			if err := repo.acquireEditLease(admittedCtx, lease); err != nil {
				return err
			}
		}
		s.editLeaseMu.Lock()
		s.editLeases[key] = lease
		s.editLeaseMu.Unlock()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cloneEditLease(lease), nil
}

// GetEditLease returns the current lease for an entry without acquiring
// session admission. Callers use it only to avoid starting cleanup while an
// editor still owns the target; the mutation itself remains admission-bound.
func (s *Service) GetEditLease(_ context.Context, sessionID, entryID string) (*QueueEditLease, error) {
	key := s.editLeaseKey(sessionID, entryID)
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()
	s.expireEditLeaseLocked(key, time.Now().UTC())
	lease := s.editLeases[key]
	if lease == nil {
		return nil, ErrEditLeaseNotFound
	}
	return cloneEditLease(lease), nil
}

// RenewEdit extends a live lease owned by connectionID.
func (s *Service) RenewEdit(ctx context.Context, sessionID, entryID, leaseID, connectionID string) (*QueueEditLease, error) {
	var renewed *QueueEditLease
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		if _, err := s.editableEntry(admittedCtx, sessionID, entryID); err != nil {
			return err
		}
		return s.withRepositorySessionTransferFence(admittedCtx, sessionID, func(context.Context) error {
			key := s.editLeaseKey(sessionID, entryID)
			now := time.Now().UTC()
			s.editLeaseMu.Lock()
			defer s.editLeaseMu.Unlock()
			s.expireEditLeaseLocked(key, now)
			lease := s.editLeases[key]
			if lease == nil || lease.LeaseID != leaseID || leaseConnection(lease) != connectionID {
				return ErrEditLeaseNotFound
			}
			lease.LeaseGeneration++
			lease.ExpiresAt = now.Add(QueueEditLeaseTTL)
			renewed = cloneEditLease(lease)
			return nil
		})
	})
	if err != nil || renewed == nil {
		return renewed, err
	}
	if repo, ok := s.repo.(editLeaseRepository); ok {
		if err := repo.renewEditLease(ctx, renewed); err != nil {
			s.editLeaseMu.Lock()
			if lease := s.editLeases[s.editLeaseKey(sessionID, entryID)]; lease != nil && lease.LeaseID == leaseID {
				delete(s.editLeases, s.editLeaseKey(sessionID, entryID))
			}
			s.editLeaseMu.Unlock()
			return nil, err
		}
	}
	return renewed, nil
}

func (s *Service) EndEdit(ctx context.Context, sessionID, entryID, leaseID, connectionID string) error {
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		return s.withRepositorySessionTransferFence(admittedCtx, sessionID, func(context.Context) error {
			key := s.editLeaseKey(sessionID, entryID)
			s.editLeaseMu.Lock()
			defer s.editLeaseMu.Unlock()
			s.expireEditLeaseLocked(key, time.Now().UTC())
			lease := s.editLeases[key]
			if lease == nil || lease.LeaseID != leaseID || leaseConnection(lease) != connectionID {
				return ErrEditLeaseNotFound
			}
			delete(s.editLeases, key)
			return nil
		})
	})
	if err != nil {
		return err
	}
	if repo, ok := s.repo.(editLeaseRepository); ok {
		return repo.releaseEditLease(ctx, sessionID, entryID, leaseID)
	}
	return nil
}

// EndEditAfterSave releases a lease and reports whether its latest update
// completed all attachment finalization. Only that state may authorize a
// post-save automatic drain.
func (s *Service) EndEditAfterSave(ctx context.Context, sessionID, entryID, leaseID, connectionID string) (bool, error) {
	saved := false
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		return s.withRepositorySessionTransferFence(admittedCtx, sessionID, func(context.Context) error {
			key := s.editLeaseKey(sessionID, entryID)
			s.editLeaseMu.Lock()
			defer s.editLeaseMu.Unlock()
			s.expireEditLeaseLocked(key, time.Now().UTC())
			lease := s.editLeases[key]
			if lease == nil || lease.LeaseID != leaseID || leaseConnection(lease) != connectionID {
				return ErrEditLeaseNotFound
			}
			saved = lease.lastOperationID != "" && lease.lastOperationFinalized
			delete(s.editLeases, key)
			return nil
		})
	})
	if err != nil {
		return false, err
	}
	if repo, ok := s.repo.(editLeaseRepository); ok {
		if err := repo.releaseEditLease(ctx, sessionID, entryID, leaseID); err != nil {
			return saved, err
		}
	}
	return saved, nil
}

func (s *Service) UpdateMessageWithLease(
	ctx context.Context,
	sessionID, entryID, leaseID, operationID, connectionID string,
	expectedRevision int64,
	content string,
	attachments []MessageAttachment,
	metadataUpdates map[string]interface{},
) (int64, error) {
	return s.UpdateMessageWithLeaseAfterValidation(
		ctx, sessionID, entryID, leaseID, operationID, connectionID,
		expectedRevision, content, attachments, metadataUpdates, nil, nil,
	)
}

// UpdateMessageWithLeaseAfterValidation runs prepare after all rejectable edit
// preconditions pass and before the queue row is changed. Both callbacks run
// inside the session admission boundary, so rejected prepared state can be
// rolled back before another editor can acquire the target.
func (s *Service) UpdateMessageWithLeaseAfterValidation(
	ctx context.Context,
	sessionID, entryID, leaseID, operationID, connectionID string,
	expectedRevision int64,
	content string,
	attachments []MessageAttachment,
	metadataUpdates map[string]interface{},
	prepare func(context.Context) error,
	rollback func(context.Context) error,
) (int64, error) {
	return s.updateMessageWithLeaseAfterValidation(
		ctx, sessionID, entryID, leaseID, operationID, connectionID,
		expectedRevision, content, attachments, metadataUpdates, prepare, rollback, nil,
	)
}

// UpdateMessageWithLeaseAfterValidationAndFinalize is the attachment-aware
// variant. finalize runs after the durable row update while session admission
// remains held, preventing a concurrent session transfer from moving the row
// before superseded external state is reconciled. A failed finalize is retained
// as part of the operation record so an identical retry can run it again with
// the original pre-update snapshot. Intervening entry lifecycle mutations
// invalidate the lease operation record, so a valid replay cannot belong to a
// newer edit.
func (s *Service) UpdateMessageWithLeaseAfterValidationAndFinalize(
	ctx context.Context,
	sessionID, entryID, leaseID, operationID, connectionID string,
	expectedRevision int64,
	content string,
	attachments []MessageAttachment,
	metadataUpdates map[string]interface{},
	prepare func(context.Context) error,
	rollback func(context.Context) error,
	finalize func(context.Context, *QueuedMessage) error,
) (int64, error) {
	return s.updateMessageWithLeaseAfterValidation(
		ctx, sessionID, entryID, leaseID, operationID, connectionID,
		expectedRevision, content, attachments, metadataUpdates, prepare, rollback, finalize,
	)
}

func (s *Service) updateMessageWithLeaseAfterValidation(
	ctx context.Context,
	sessionID, entryID, leaseID, operationID, connectionID string,
	expectedRevision int64,
	content string,
	attachments []MessageAttachment,
	metadataUpdates map[string]interface{},
	prepare func(context.Context) error,
	rollback func(context.Context) error,
	finalize func(context.Context, *QueuedMessage) error,
) (int64, error) {
	if operationID == "" {
		return 0, ErrEditLeaseNotFound
	}
	key := s.editLeaseKey(sessionID, entryID)
	operationHash := editOperationHash(content, attachments, metadataUpdates)
	var revision int64
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		initialRevision, initialOperationID, duplicate, previous, err := s.beginEditMutation(
			key, leaseID, operationID, connectionID, expectedRevision, operationHash,
		)
		if err != nil {
			return err
		}
		if duplicate {
			if err := s.validateDuplicateEditReplay(admittedCtx, sessionID, entryID); err != nil {
				return err
			}
			revision = initialRevision
			return s.finalizeDuplicateEdit(
				admittedCtx, key, operationID, operationHash, previous, finalize,
			)
		}
		revision = initialRevision
		previous, err = s.findQueuedMessageForEdit(admittedCtx, sessionID, entryID)
		if err != nil {
			return err
		}

		if prepare != nil {
			if err := prepare(admittedCtx); err != nil {
				s.rollbackPreparedEdit(admittedCtx, rollback)
				return err
			}
		}

		lease, err := s.relockEditMutation(key, leaseID, connectionID, revision, initialOperationID)
		if err != nil {
			s.rollbackPreparedEdit(admittedCtx, rollback)
			return err
		}
		if leaseRepository, ok := s.repo.(durableEditLeaseMutationRepository); ok {
			if err := leaseRepository.updateContentAndMetadataWithLease(admittedCtx, sessionID, entryID, leaseID, content, attachments, metadataUpdates, QueuedByUser); err != nil {
				s.editLeaseMu.Unlock()
				s.rollbackPreparedEdit(admittedCtx, rollback)
				return err
			}
		} else if err := s.repo.UpdateContentAndMetadata(admittedCtx, sessionID, entryID, content, attachments, metadataUpdates, QueuedByUser); err != nil {
			s.editLeaseMu.Unlock()
			s.rollbackPreparedEdit(admittedCtx, rollback)
			return err
		}
		revision++
		s.editRevisions[key] = revision
		s.editRevisionTaskIDs[key] = lease.taskID
		lease.TargetRevision = revision
		lease.lastOperationID = operationID
		lease.lastOperationHash = operationHash
		lease.lastOperationResult = revision
		lease.lastOperationPrevious = cloneQueuedMessage(previous)
		lease.lastOperationFinalized = finalize == nil
		s.editLeaseMu.Unlock()
		if finalize != nil {
			if err := finalize(admittedCtx, previous); err != nil {
				return err
			}
			s.markEditOperationFinalized(key, operationID, operationHash)
		}
		return nil
	})
	return revision, err
}

func (s *Service) finalizeDuplicateEdit(
	ctx context.Context,
	key editLeaseKey,
	operationID, operationHash string,
	previous *QueuedMessage,
	finalize func(context.Context, *QueuedMessage) error,
) error {
	if finalize == nil || s.editOperationFinalized(key, operationID, operationHash) {
		return nil
	}
	if err := finalize(ctx, previous); err != nil {
		return err
	}
	s.markEditOperationFinalized(key, operationID, operationHash)
	return nil
}

func (s *Service) findQueuedMessageForEdit(
	ctx context.Context,
	sessionID, entryID string,
) (*QueuedMessage, error) {
	entries, err := s.repo.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].ID == entryID {
			return cloneQueuedMessage(&entries[i]), nil
		}
	}
	return nil, ErrEntryNotFound
}
func (s *Service) beginEditMutation(
	key editLeaseKey,
	leaseID, operationID, connectionID string,
	expectedRevision int64,
	operationHash string,
) (int64, string, bool, *QueuedMessage, error) {
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()
	s.expireEditLeaseLocked(key, time.Now().UTC())
	lease := s.editLeases[key]
	if lease == nil || lease.LeaseID != leaseID || leaseConnection(lease) != connectionID {
		return 0, "", false, nil, ErrEditLeaseNotFound
	}
	if lease.lastOperationID == operationID {
		if lease.lastOperationHash == operationHash {
			return lease.lastOperationResult, lease.lastOperationID, true,
				cloneQueuedMessage(lease.lastOperationPrevious), nil
		}
		return 0, "", false, nil, ErrEditRevisionConflict
	}
	revision := s.editRevisions[key]
	if expectedRevision != revision {
		return 0, "", false, nil, ErrEditRevisionConflict
	}
	return revision, lease.lastOperationID, false, nil, nil
}
func (s *Service) editOperationFinalized(key editLeaseKey, operationID, operationHash string) bool {
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()
	lease := s.editLeases[key]
	return lease != nil &&
		lease.lastOperationID == operationID &&
		lease.lastOperationHash == operationHash &&
		lease.lastOperationFinalized
}

func (s *Service) markEditOperationFinalized(key editLeaseKey, operationID, operationHash string) {
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()
	lease := s.editLeases[key]
	if lease != nil && lease.lastOperationID == operationID && lease.lastOperationHash == operationHash {
		lease.lastOperationFinalized = true
	}
}

// relockEditMutation validates the lease again after attachment preparation and
// leaves editLeaseMu held for the repository update.
func (s *Service) relockEditMutation(
	key editLeaseKey,
	leaseID, connectionID string,
	revision int64,
	initialOperationID string,
) (*QueueEditLease, error) {
	s.editLeaseMu.Lock()
	s.expireEditLeaseLocked(key, time.Now().UTC())
	lease := s.editLeases[key]
	if lease == nil || lease.LeaseID != leaseID || leaseConnection(lease) != connectionID {
		s.editLeaseMu.Unlock()
		return nil, ErrEditLeaseNotFound
	}
	if s.editRevisions[key] != revision || lease.lastOperationID != initialOperationID {
		s.editLeaseMu.Unlock()
		return nil, ErrEditRevisionConflict
	}
	return lease, nil
}

func (s *Service) rollbackPreparedEdit(ctx context.Context, rollback func(context.Context) error) {
	if rollback == nil {
		return
	}
	if err := rollback(ctx); err != nil {
		s.logger.Warn("failed to roll back prepared queue edit state", zap.Error(err))
	}
}

func cloneEditLease(lease *QueueEditLease) *QueueEditLease {
	if lease == nil {
		return nil
	}
	copy := *lease
	return &copy
}

// leaseConnection is intentionally process-local. The connection binding is
// stored beside the lease rather than exposed in the wire response.
func leaseConnection(lease *QueueEditLease) string {
	return lease.connectionID
}

func (s *Service) editLeaseBlocksHead(ctx context.Context, sessionID string) (bool, error) {
	entries, err := s.repo.ListBySession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()
	now := time.Now().UTC()
	for _, entry := range entries {
		key := s.editLeaseKey(sessionID, entry.ID)
		s.expireEditLeaseLocked(key, now)
		return s.editLeases[key] != nil, nil
	}
	return false, nil
}

func (s *Service) editLeaseBlocksEntryLocked(sessionID, entryID string) bool {
	key := s.editLeaseKey(sessionID, entryID)
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()
	s.expireEditLeaseLocked(key, time.Now().UTC())
	return s.editLeases[key] != nil
}
func (s *Service) editLeaseBlocksReorderLocked(sessionID string) bool {
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()
	now := time.Now().UTC()
	for key := range s.editLeases {
		if key.sessionID != sessionID {
			continue
		}
		s.expireEditLeaseLocked(key, now)
		if s.editLeases[key] != nil {
			return true
		}
	}
	return false
}

func (s *Service) editLeaseBlocksTail(ctx context.Context, sessionID, excludedEntryID, queuedBy string) (bool, error) {
	entries, err := s.repo.ListBySession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	var tail *QueuedMessage
	for i := range entries {
		entry := &entries[i]
		if entry.ID == excludedEntryID {
			continue
		}
		if tail == nil || entry.Position > tail.Position {
			tail = entry
		}
	}
	if tail == nil {
		return false, nil
	}
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()
	key := s.editLeaseKey(sessionID, tail.ID)
	s.expireEditLeaseLocked(key, time.Now().UTC())
	return s.editLeases[key] != nil && tail.QueuedBy == queuedBy, nil
}

func (s *Service) editLeaseBlocksMerge(ctx context.Context, sessionID, sourceID string) (bool, error) {
	entries, err := s.repo.ListBySession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	var source, target *QueuedMessage
	for i := range entries {
		if entries[i].ID == sourceID {
			source = &entries[i]
			break
		}
	}
	if source == nil {
		return false, nil
	}
	for i := range entries {
		entry := &entries[i]
		if entry.IsReservedInFlight() || entry.Position >= source.Position {
			continue
		}
		if target == nil || entry.Position > target.Position {
			target = entry
		}
	}
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()
	now := time.Now().UTC()
	sourceKey := s.editLeaseKey(sessionID, source.ID)
	s.expireEditLeaseLocked(sourceKey, now)
	if s.editLeases[sourceKey] != nil {
		return true, nil
	}
	if target == nil {
		return false, nil
	}
	targetKey := s.editLeaseKey(sessionID, target.ID)
	s.expireEditLeaseLocked(targetKey, now)
	return s.editLeases[targetKey] != nil, nil
}

func (s *Service) deleteEditStateLocked(key editLeaseKey) {
	delete(s.editLeases, key)
	delete(s.editRevisions, key)
	delete(s.editRevisionTaskIDs, key)
}

func (s *Service) invalidateEditLeasesLocked(sessionIDs ...string) {
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()
	for key := range s.editRevisions {
		for _, sessionID := range sessionIDs {
			if key.sessionID == sessionID {
				s.deleteEditStateLocked(key)
				break
			}
		}
	}
	for key := range s.editLeases {
		for _, sessionID := range sessionIDs {
			if key.sessionID == sessionID {
				s.deleteEditStateLocked(key)
				break
			}
		}
	}
	for key := range s.editRevisionTaskIDs {
		for _, sessionID := range sessionIDs {
			if key.sessionID == sessionID {
				s.deleteEditStateLocked(key)
				break
			}
		}
	}
}

// QueueMessage appends a new entry to the session's FIFO queue. Returns
// ErrQueueFull when the cap is exceeded.
func (s *Service) QueueMessage(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment) (*QueuedMessage, error) {
	return s.QueueMessageWithMetadata(ctx, sessionID, taskID, content, model, userID, planMode, attachments, nil)
}

// QueueMessageWithMetadata is like QueueMessage but stores extra metadata that
// is propagated to the resulting Message row when the queued message is
// drained (e.g. sender_task_id for messages sent via message_task_kandev).
func (s *Service) QueueMessageWithMetadata(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}) (*QueuedMessage, error) {
	identity, err := s.repo.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		// The legacy admission API predates durable session incarnations. Keep
		// it usable for callers that only have the session/task pair while the
		// identity-bound API remains strict for browser mutations.
		if errors.Is(err, ErrSessionIdentityMismatch) {
			return s.queueMessageWithMetadataAdmission(
				ctx, nil, sessionID, taskID, content, model, userID, planMode, attachments, metadata, nil, nil,
			)
		}
		return nil, err
	}
	return s.queueMessageWithMetadataAdmission(
		ctx, &identity, sessionID, taskID, content, model, userID, planMode, attachments, metadata, nil, nil,
	)
}

// QueueMessageWithMetadataAfterInsert admits an exact source row, runs
// afterInsert while the per-session admission lock remains held, then attempts
// the snapshotted automatic fold. Callers use the hook to claim staged
// attachments before folding; if it returns an error, no fold is attempted.
func (s *Service) QueueMessageWithMetadataAfterInsert(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, afterInsert func(context.Context, *QueuedMessage) error) (*QueuedMessage, error) {
	identity, err := s.repo.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		if errors.Is(err, ErrSessionIdentityMismatch) {
			return s.queueMessageWithMetadataAdmission(
				ctx, nil, sessionID, taskID, content, model, userID, planMode, attachments, metadata, nil, afterInsert,
			)
		}
		return nil, err
	}
	return s.queueMessageWithMetadataAdmission(
		ctx, &identity, sessionID, taskID, content, model, userID, planMode, attachments, metadata, nil, afterInsert,
	)
}

// QueueMessageWithMetadataForSession admits a browser message using the
// effective policy bound to the supplied immutable session identity.
func (s *Service) QueueMessageWithMetadataForSession(
	ctx context.Context,
	identity QueueSessionIdentity,
	content, model, userID string,
	planMode bool,
	attachments []MessageAttachment,
	metadata map[string]interface{},
) (*QueuedMessage, error) {
	return s.queueMessageWithMetadataAdmission(
		ctx, &identity, identity.SessionID, identity.TaskID, content, model, userID, planMode, attachments, metadata, nil, nil,
	)
}

// QueueMessageWithMetadataForSessionWithClaim atomically claims staged
// attachments when admission inserts a queue entry. An empty claim carries
// inline-only attachments and may use the compatible full-queue fold path.
func (s *Service) QueueMessageWithMetadataForSessionWithClaim(
	ctx context.Context,
	identity QueueSessionIdentity,
	content, model, userID string,
	planMode bool,
	attachments []MessageAttachment,
	metadata map[string]interface{},
	claim QueueAttachmentClaim,
) (*QueuedMessage, error) {
	return s.queueMessageWithMetadataAdmission(
		ctx, &identity, identity.SessionID, identity.TaskID, content, model, userID, planMode, attachments, metadata, &claim, nil,
	)
}

// QueueMessageWithMetadataForSessionAfterInsert is the identity-bound variant
// used while attachment claiming still participates in the admission lock.
func (s *Service) QueueMessageWithMetadataForSessionAfterInsert(
	ctx context.Context,
	identity QueueSessionIdentity,
	content, model, userID string,
	planMode bool,
	attachments []MessageAttachment,
	metadata map[string]interface{},
	afterInsert func(context.Context, *QueuedMessage) error,
) (*QueuedMessage, error) {
	return s.queueMessageWithMetadataAdmission(
		ctx, &identity, identity.SessionID, identity.TaskID, content, model, userID, planMode, attachments, metadata, nil, afterInsert,
	)
}

// queueMessageWithMetadataAdmission resolves policy and completes admission
// under one per-session lock. Identity-bound repositories compare that policy
// inside every durable mutation; an override race restarts before admission.
//
//nolint:gocognit // retry boundaries keep policy validation before each mutation.
func (s *Service) queueMessageWithMetadataAdmission(ctx context.Context, identity *QueueSessionIdentity, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, claim *QueueAttachmentClaim, afterInsert func(context.Context, *QueuedMessage) error) (*QueuedMessage, error) {
	var queued *QueuedMessage
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		for {
			policy := s.resolveAdmissionAutoMergePolicy(admittedCtx, identity, sessionID)
			source, insertErr := s.insertQueueMessageWithMetadata(
				admittedCtx, identity, sessionID, taskID, content, model, userID, planMode,
				attachments, metadata, claim, s.MaxPerSession(), policy,
			)
			if errors.Is(insertErr, ErrAutoMergePolicyChanged) {
				continue
			}
			autoMergeEnabled := policy != nil && policy.Enabled
			if insertErr != nil {
				hasStagedAttachments := claim != nil && len(claim.IDs) > 0
				if !errors.Is(insertErr, ErrQueueFull) || !autoMergeEnabled || hasStagedAttachments || afterInsert != nil {
					return insertErr
				}
				var merged *QueuedMessage
				source, merged, insertErr = s.admitQueueFullMessage(
					admittedCtx, identity, insertErr, sessionID, taskID, content, model, userID, planMode,
					attachments, metadata, claim, s.MaxPerSession(), policy,
				)
				if errors.Is(insertErr, ErrAutoMergePolicyChanged) {
					continue
				}
				if insertErr != nil {
					return insertErr
				}
				if merged != nil {
					queued = merged
					return nil
				}
			}
			if afterInsert != nil {
				if err := afterInsert(admittedCtx, source); err != nil {
					return err
				}
			}
			queued = s.finalizeAutoMerge(admittedCtx, identity, source, policy)
			return nil
		}
	})
	return queued, err
}

func (s *Service) resolveAdmissionAutoMergePolicy(
	ctx context.Context,
	identity *QueueSessionIdentity,
	sessionID string,
) *AutoMergePolicy {
	if identity == nil {
		policy := s.globalAutoMergePolicy()
		return &policy
	}
	policy, err := s.ResolveAutoMergePolicy(ctx, *identity)
	if err == nil {
		return &policy
	}
	s.logger.Warn("automatic merge policy unavailable; admitting separate message",
		zap.String("session_id", sessionID),
		zap.Error(err))
	return nil
}

func (s *Service) autoMergeCandidate(
	ctx context.Context,
	identity *QueueSessionIdentity,
	candidate *QueuedMessage,
	claim *QueueAttachmentClaim,
	policy *AutoMergePolicy,
) (*QueuedMessage, bool, error) {
	if identity == nil {
		return s.repo.AutoMergeCandidateIntoAbove(ctx, candidate)
	}
	if policyRepo, ok := s.repo.(autoMergeAdmissionRepository); ok && policy != nil {
		return policyRepo.AutoMergeCandidateIntoAboveForSessionWithPolicy(ctx, *identity, candidate, claim, *policy)
	}
	if claim == nil {
		return s.repo.AutoMergeCandidateIntoAboveForSession(ctx, *identity, candidate)
	}
	claimRepo, ok := s.repo.(queueAttachmentRepository)
	if !ok {
		return nil, false, errors.New("transactional attachment queue repository is unavailable")
	}
	return claimRepo.AutoMergeCandidateIntoAboveForSessionWithClaim(ctx, *identity, candidate, *claim)
}

// admitQueueFullMessage folds a candidate into the tail or retries insertion
// when the fold observes capacity freed by a concurrent drain.
func (s *Service) admitQueueFullMessage(
	ctx context.Context,
	identity *QueueSessionIdentity,
	queueFullErr error,
	sessionID, taskID, content, model, userID string,
	planMode bool,
	attachments []MessageAttachment,
	metadata map[string]interface{},
	claim *QueueAttachmentClaim,
	maxPerSession int,
	policy *AutoMergePolicy,
) (*QueuedMessage, *QueuedMessage, error) {
	candidate := &QueuedMessage{
		SessionID:   sessionID,
		TaskID:      taskID,
		Content:     content,
		Model:       model,
		PlanMode:    planMode,
		Attachments: attachments,
		Metadata:    copyMessageMetadata(metadata, 0),
		QueuedAt:    time.Now().UTC(),
		QueuedBy:    userID,
	}
	merged, didMerge, err := s.autoMergeCandidate(ctx, identity, candidate, claim, policy)
	if errors.Is(err, ErrAutoMergePolicyChanged) {
		return nil, nil, err
	}
	if err != nil {
		if errors.Is(err, ErrTaskInactive) {
			return nil, nil, err
		}
		s.logger.Error("automatic merge into full queue failed; preserving queue full rejection",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, nil, queueFullErr
	}
	if didMerge && merged != nil {
		s.logger.Info("automatically merged queued entry into full tail",
			zap.String("session_id", sessionID),
			zap.String("surviving_entry_id", merged.ID))
		return nil, merged, nil
	}
	source, err := s.insertQueueMessageWithMetadata(
		ctx, identity, sessionID, taskID, content, model, userID, planMode,
		attachments, metadata, claim, maxPerSession, policy,
	)
	return source, nil, err
}

// queueMessageWithMetadataSeparate is used by retry paths whose existing
// contract must not gain admission-time automatic behavior.
func (s *Service) queueMessageWithMetadataSeparate(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, maxPerSession int) (*QueuedMessage, error) {
	var queued *QueuedMessage
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		var err error
		queued, err = s.insertQueueMessageWithMetadata(
			admittedCtx, nil, sessionID, taskID, content, model, userID, planMode,
			attachments, metadata, nil, maxPerSession, nil,
		)
		return err
	})
	return queued, err
}

func (s *Service) finalizeAutoMerge(
	ctx context.Context,
	identity *QueueSessionIdentity,
	source *QueuedMessage,
	policy *AutoMergePolicy,
) *QueuedMessage {
	for policy != nil && policy.Enabled {
		blocked, leaseErr := s.editLeaseBlocksTail(ctx, source.SessionID, source.ID, source.QueuedBy)
		if leaseErr != nil {
			s.logger.Error("automatic queue merge lease check failed; preserving separate admission",
				zap.String("session_id", source.SessionID),
				zap.String("source_entry_id", source.ID),
				zap.Error(leaseErr))
			return source
		}
		if blocked {
			return source
		}
		var merged *QueuedMessage
		var didMerge bool
		var err error
		if identity == nil {
			merged, didMerge, err = s.repo.AutoMergeIntoAbove(ctx, source.SessionID, source.ID)
		} else if policyRepo, ok := s.repo.(autoMergeAdmissionRepository); ok {
			merged, didMerge, err = policyRepo.AutoMergeIntoAboveForSessionWithPolicy(ctx, *identity, source.ID, *policy)
		} else {
			merged, didMerge, err = s.repo.AutoMergeIntoAboveForSession(ctx, *identity, source.ID)
		}
		if errors.Is(err, ErrAutoMergePolicyChanged) && identity != nil {
			policy = s.resolveAdmissionAutoMergePolicy(ctx, identity, source.SessionID)
			continue
		}
		if err != nil {
			s.logger.Error("automatic queue merge failed; preserving separate admission",
				zap.String("session_id", source.SessionID),
				zap.String("source_entry_id", source.ID),
				zap.Error(err))
			return source
		}
		if !didMerge || merged == nil {
			return source
		}
		s.logger.Info("automatically merged queued entry into tail",
			zap.String("session_id", source.SessionID),
			zap.String("source_entry_id", source.ID),
			zap.String("surviving_entry_id", merged.ID))
		return merged
	}
	return source
}

func (s *Service) insertQueueMessage(
	ctx context.Context,
	identity *QueueSessionIdentity,
	msg *QueuedMessage,
	claim *QueueAttachmentClaim,
	maxPerSession int,
	policy *AutoMergePolicy,
) error {
	if identity == nil {
		return s.repo.Insert(ctx, msg, maxPerSession)
	}
	if policyRepo, ok := s.repo.(autoMergeAdmissionRepository); ok && policy != nil {
		return policyRepo.InsertForSessionWithPolicy(ctx, *identity, msg, claim, maxPerSession, *policy)
	}
	if claim == nil {
		return s.repo.InsertForSession(ctx, *identity, msg, maxPerSession)
	}
	claimRepo, ok := s.repo.(queueAttachmentRepository)
	if !ok {
		return errors.New("transactional attachment queue repository is unavailable")
	}
	return claimRepo.InsertForSessionWithClaim(ctx, *identity, msg, *claim, maxPerSession)
}

// insertQueueMessageWithMetadata inserts a message with metadata under the per-session admission lock.
func (s *Service) insertQueueMessageWithMetadata(ctx context.Context, identity *QueueSessionIdentity, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, claim *QueueAttachmentClaim, maxPerSession int, policy *AutoMergePolicy) (*QueuedMessage, error) {
	metadataCopy := copyMessageMetadata(metadata, 0)
	msg := &QueuedMessage{
		SessionID:   sessionID,
		TaskID:      taskID,
		Content:     content,
		Model:       model,
		PlanMode:    planMode,
		Attachments: attachments,
		Metadata:    metadataCopy,
		QueuedBy:    userID,
	}
	err := s.insertQueueMessage(ctx, identity, msg, claim, maxPerSession, policy)
	if err != nil {
		if errors.Is(err, ErrQueueFull) {
			s.logger.Info("queue full",
				zap.String("session_id", sessionID),
				zap.Int("max", maxPerSession))
		}
		return nil, err
	}
	s.logger.Info("message queued",
		zap.String("session_id", sessionID),
		zap.String("task_id", taskID),
		zap.String("entry_id", msg.ID),
		zap.Int64("position", msg.Position),
		zap.Int("content_length", len(content)))
	return msg, nil
}

// RestoreMessage restores a dequeued message at its original FIFO position
// after a delivery failure. It preserves the entry's identity, position,
// queued_at time, and queued_by ownership.
func (s *Service) RestoreMessage(ctx context.Context, msg *QueuedMessage) (*QueuedMessage, error) {
	if msg == nil {
		return nil, errors.New("queued message is nil")
	}
	var restored *QueuedMessage
	err := s.WithSessionAdmission(ctx, msg.SessionID, func(admittedCtx context.Context) error {
		var err error
		restored, err = s.restoreMessage(admittedCtx, msg)
		return err
	})
	return restored, err
}

func (s *Service) RestoreMessageForSession(
	ctx context.Context,
	identity QueueSessionIdentity,
	msg *QueuedMessage,
) (*QueuedMessage, error) {
	if msg == nil {
		return nil, errors.New("queued message is nil")
	}
	var restored *QueuedMessage
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		candidate := *msg
		candidate.Metadata = copyMessageMetadata(msg.Metadata, 0)
		if err := s.validateSessionIdentity(admittedCtx, identity); err != nil {
			return err
		}
		if err := s.repo.RestoreForSession(admittedCtx, identity, &candidate, 0); err != nil {
			return err
		}
		restored = &candidate
		return nil
	})
	return restored, err
}

// restoreMessage is the admission-checked core of RestoreMessage.
func (s *Service) restoreMessage(ctx context.Context, msg *QueuedMessage) (*QueuedMessage, error) {
	restored := *msg
	restored.Metadata = copyMessageMetadata(msg.Metadata, 0)
	if err := s.repo.Restore(ctx, &restored, 0); err != nil {
		return nil, err
	}
	s.invalidateEditLease(restored.SessionID, restored.ID)
	s.logger.Info("message restored at original queue position",
		zap.String("session_id", restored.SessionID),
		zap.String("task_id", restored.TaskID),
		zap.String("entry_id", restored.ID),
		zap.Int64("position", restored.Position))
	return &restored, nil
}

// QueueMessageWithCoalesceKey replaces an existing pending entry with the same
// coalesce key, session, and queued_by value. When no matching entry exists it
// inserts a new tail entry if allowInsert is true; otherwise ErrEntryNotFound is
// returned. The returned bool is true when an existing entry was replaced.
func (s *Service) QueueMessageWithCoalesceKey(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, coalesceKey string, allowInsert bool) (*QueuedMessage, bool, error) {
	return s.queueMessageWithCoalesceKey(
		ctx, sessionID, taskID, content, model, userID, planMode, attachments, metadata,
		coalesceKey, allowInsert, s.MaxPerSession(),
	)
}

func (s *Service) QueueMessageWithCoalesceKeyForSession(
	ctx context.Context,
	identity QueueSessionIdentity,
	content, model, userID string,
	planMode bool,
	attachments []MessageAttachment,
	metadata map[string]interface{},
	coalesceKey string,
	allowInsert bool,
) (*QueuedMessage, bool, error) {
	var queued *QueuedMessage
	var replaced bool
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		metadataCopy := copyMessageMetadata(metadata, 1)
		metadataCopy[MetadataCoalesceKey] = coalesceKey
		candidate := &QueuedMessage{
			SessionID: identity.SessionID, TaskID: identity.TaskID,
			Content: content, Model: model, PlanMode: planMode,
			Attachments: attachments, Metadata: metadataCopy, QueuedBy: userID,
		}
		var err error
		queued, replaced, err = s.repo.InsertOrReplaceByCoalesceKeyForSession(
			admittedCtx, identity, candidate, coalesceKey, s.MaxPerSession(), allowInsert,
		)
		return err
	})
	return queued, replaced, err
}

// queueMessageWithCoalesceKey is the admission-checked core of QueueMessageWithCoalesceKey.
func (s *Service) queueMessageWithCoalesceKey(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, coalesceKey string, allowInsert bool, maxPerSession int) (*QueuedMessage, bool, error) {
	var queued *QueuedMessage
	var replaced bool
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		var err error
		queued, replaced, err = s.insertQueueMessageWithCoalesceKey(
			admittedCtx, sessionID, taskID, content, model, userID, planMode, attachments, metadata,
			coalesceKey, allowInsert, maxPerSession,
		)
		return err
	})
	return queued, replaced, err
}

// insertQueueMessageWithCoalesceKey inserts or coalesce-replaces an entry under the admission lock.
func (s *Service) insertQueueMessageWithCoalesceKey(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, coalesceKey string, allowInsert bool, maxPerSession int) (*QueuedMessage, bool, error) {
	metadataCopy := copyMessageMetadata(metadata, 1)
	metadataCopy[MetadataCoalesceKey] = coalesceKey
	msg := &QueuedMessage{
		SessionID:   sessionID,
		TaskID:      taskID,
		Content:     content,
		Model:       model,
		PlanMode:    planMode,
		Attachments: attachments,
		Metadata:    metadataCopy,
		QueuedBy:    userID,
	}
	queued, replaced, err := s.repo.InsertOrReplaceByCoalesceKey(ctx, msg, coalesceKey, maxPerSession, allowInsert)
	if err != nil {
		if errors.Is(err, ErrQueueFull) {
			s.logger.Info("queue full",
				zap.String("session_id", sessionID),
				zap.Int("max", maxPerSession))
		}
		return nil, false, err
	}
	if replaced && queued != nil {
		s.invalidateEditLease(sessionID, queued.ID)
	}
	s.logger.Info("message queued with coalesce key",
		zap.String("session_id", sessionID),
		zap.String("task_id", taskID),
		zap.String("entry_id", queued.ID),
		zap.String("coalesce_key", coalesceKey),
		zap.Bool("replaced", replaced),
		zap.Int64("position", queued.Position),
		zap.Int("content_length", len(content)))
	return queued, replaced, nil
}

// RequeueMessage retries work that was already admitted. It bypasses the
// current admission cap so lowering capacity while a message is in flight
// cannot discard that message. Coalesced retries retain replacement semantics.
func (s *Service) RequeueMessage(ctx context.Context, msg *QueuedMessage, queuedBy, coalesceKey string) (*QueuedMessage, bool, error) {
	if msg == nil {
		return nil, false, errors.New("queued message is nil")
	}
	if coalesceKey != "" {
		return s.queueMessageWithCoalesceKey(
			ctx, msg.SessionID, msg.TaskID, msg.Content, msg.Model, queuedBy, msg.PlanMode,
			msg.Attachments, msg.Metadata, coalesceKey, true, 0,
		)
	}
	queued, err := s.queueMessageWithMetadataSeparate(
		ctx, msg.SessionID, msg.TaskID, msg.Content, msg.Model, queuedBy, msg.PlanMode,
		msg.Attachments, msg.Metadata, 0,
	)
	return queued, false, err
}

// RequeueAtHead re-enqueues a user message at a position strictly lower
// than the session's current head. It is the FIFO-preserving requeue
// that Service.requeueMessage invokes when an entry was superseded by a
// newer dispatch before it could be claimed. See Repository
// .RequeuePreservingFIFO for the position arithmetic and position rebasing.
//
// This method is the bug fix entry point — caller code that wants
// user-message retry should call this instead of RequeueMessage so
// the original message beats any new arrival on a busy session.
func (s *Service) RequeueAtHead(ctx context.Context, msg *QueuedMessage) error {
	if msg == nil {
		return errors.New("queued message is nil")
	}
	return s.WithSessionAdmission(ctx, msg.SessionID, func(admittedCtx context.Context) error {
		if err := s.repo.RequeuePreservingFIFO(admittedCtx, msg); err != nil {
			return err
		}
		s.invalidateEditLease(msg.SessionID, msg.ID)
		return nil
	})
}

func (s *Service) RequeueAtHeadForSession(
	ctx context.Context,
	identity QueueSessionIdentity,
	msg *QueuedMessage,
) error {
	if msg == nil {
		return errors.New("queued message is nil")
	}
	return s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		if err := s.validateSessionIdentity(admittedCtx, identity); err != nil {
			return err
		}
		return s.repo.RequeuePreservingFIFOForSession(admittedCtx, identity, msg)
	})
}

// QueueLifecycleMessageWithCoalesceKey accepts a lifecycle entry only while
// its task remains active. accepted is false for a normal archive/delete win.
func (s *Service) QueueLifecycleMessageWithCoalesceKey(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, coalesceKey string, allowInsert bool) (*QueuedMessage, bool, bool, error) {
	return s.queueLifecycleMessageWithCoalesceKey(ctx, nil, sessionID, taskID, content, model, userID, planMode, attachments, metadata, coalesceKey, allowInsert, false, nil)
}

func (s *Service) QueueLifecycleMessageWithCoalesceKeyForSession(
	ctx context.Context,
	identity QueueSessionIdentity,
	content, model, userID string,
	planMode bool,
	attachments []MessageAttachment,
	metadata map[string]interface{},
	coalesceKey string,
	allowInsert bool,
) (*QueuedMessage, bool, bool, error) {
	return s.queueLifecycleMessageWithCoalesceKey(
		ctx, &identity, identity.SessionID, identity.TaskID, content, model, userID, planMode,
		attachments, metadata, coalesceKey, allowInsert, false, nil,
	)
}

// QueueLifecycleMessageWithCoalesceKeyAfterInsert runs afterInsert while the
// queue admission lock is held. If the callback fails, the exact queue
// snapshot from before admission is restored before the error is returned.
func (s *Service) QueueLifecycleMessageWithCoalesceKeyAfterInsert(
	ctx context.Context,
	sessionID, taskID, content, model, userID string,
	planMode bool,
	attachments []MessageAttachment,
	metadata map[string]interface{},
	coalesceKey string,
	allowInsert bool,
	afterInsert func(context.Context, *QueuedMessage, bool) error,
) (*QueuedMessage, bool, bool, error) {
	return s.queueLifecycleMessageWithCoalesceKey(ctx, nil, sessionID, taskID, content, model, userID, planMode, attachments, metadata, coalesceKey, allowInsert, false, afterInsert)
}

func (s *Service) QueueLifecycleMessageWithCoalesceKeyForSessionAfterInsert(
	ctx context.Context,
	identity QueueSessionIdentity,
	content, model, userID string,
	planMode bool,
	attachments []MessageAttachment,
	metadata map[string]interface{},
	coalesceKey string,
	allowInsert bool,
	afterInsert func(context.Context, *QueuedMessage, bool) error,
) (*QueuedMessage, bool, bool, error) {
	return s.queueLifecycleMessageWithCoalesceKey(
		ctx, &identity, identity.SessionID, identity.TaskID, content, model, userID, planMode,
		attachments, metadata, coalesceKey, allowInsert, false, afterInsert,
	)
}

// RequeueLifecycleMessageWithCoalesceKey preserves the generation captured by
// an existing durable row. This prevents a stale retry from becoming a new
// prompt after the task was archived and then unarchived.
func (s *Service) RequeueLifecycleMessageWithCoalesceKey(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, coalesceKey string, allowInsert bool) (*QueuedMessage, bool, bool, error) {
	return s.queueLifecycleMessageWithCoalesceKey(ctx, nil, sessionID, taskID, content, model, userID, planMode, attachments, metadata, coalesceKey, allowInsert, true, nil)
}

func (s *Service) RequeueLifecycleMessageWithCoalesceKeyForSession(
	ctx context.Context,
	identity QueueSessionIdentity,
	content, model, userID string,
	planMode bool,
	attachments []MessageAttachment,
	metadata map[string]interface{},
	coalesceKey string,
	allowInsert bool,
) (*QueuedMessage, bool, bool, error) {
	return s.queueLifecycleMessageWithCoalesceKey(
		ctx, &identity, identity.SessionID, identity.TaskID, content, model, userID, planMode,
		attachments, metadata, coalesceKey, allowInsert, true, nil,
	)
}

// queueLifecycleMessageWithCoalesceKey is the lifecycle-guarded core of the lifecycle queue methods.
func (s *Service) queueLifecycleMessageWithCoalesceKey(ctx context.Context, identity *QueueSessionIdentity, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, coalesceKey string, allowInsert, isRetry bool, afterInsert func(context.Context, *QueuedMessage, bool) error) (*QueuedMessage, bool, bool, error) {
	var queued *QueuedMessage
	var replaced, accepted bool
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		if identity != nil {
			if err := s.validateSessionIdentity(admittedCtx, *identity); err != nil {
				return err
			}
		}
		var snapshot []QueuedMessage
		var pendingMove *PendingMove
		if afterInsert != nil {
			var snapshotErr error
			snapshot, snapshotErr = s.repo.ListBySession(admittedCtx, sessionID)
			if snapshotErr != nil {
				return snapshotErr
			}
			pendingMove, snapshotErr = s.repo.GetPendingMove(admittedCtx, sessionID)
			if snapshotErr != nil {
				return snapshotErr
			}
		}
		var err error
		queued, replaced, accepted, err = s.insertLifecycleMessageWithCoalesceKey(
			admittedCtx, identity, sessionID, taskID, content, model, userID, planMode, attachments,
			metadata, coalesceKey, allowInsert, isRetry,
		)
		if err != nil || !accepted || afterInsert == nil {
			return err
		}
		if err := afterInsert(admittedCtx, queued, replaced); err != nil {
			rollbackErr := s.restoreCoalescedAdmission(context.WithoutCancel(admittedCtx), sessionID, snapshot, pendingMove)
			queued = nil
			replaced = false
			accepted = false
			if rollbackErr != nil {
				return fmt.Errorf("coalesced lifecycle admission callback failed: %w (queue rollback failed: %v)", err, rollbackErr)
			}
			return err
		}
		return nil
	})
	return queued, replaced, accepted, err
}

func (s *Service) restoreCoalescedAdmission(ctx context.Context, sessionID string, entries []QueuedMessage, pendingMove *PendingMove) error {
	if err := s.repo.ReplaceSession(ctx, sessionID, entries, pendingMove); err != nil {
		s.logger.Error("failed to roll back lifecycle queue admission",
			zap.String("session_id", sessionID), zap.Error(err))
		return err
	}
	return nil
}

// insertLifecycleMessageWithCoalesceKey inserts or replaces a lifecycle entry under the admission lock.
func (s *Service) insertLifecycleMessageWithCoalesceKey(ctx context.Context, identity *QueueSessionIdentity, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment, metadata map[string]interface{}, coalesceKey string, allowInsert, isRetry bool) (*QueuedMessage, bool, bool, error) {
	metadataCopy := clearReservedMetadata(metadata)
	generation, err := s.repo.LifecycleGeneration(ctx, taskID)
	if err != nil {
		return nil, false, false, err
	}
	if isRetry {
		// Rows written before generation metadata existed belong to generation
		// zero. They may retry while no purge occurred, but a later archive
		// advances generation and rejects them just like newer rows.
		expected, ok := lifecycleGenerationFromMetadata(metadataCopy)
		if !ok {
			expected = 0
		}
		if expected != generation {
			return nil, false, false, nil
		}
	}
	metadataCopy[MetadataCoalesceKey] = coalesceKey
	metadataCopy[MetadataLifecycleDurable] = true
	metadataCopy[MetadataLifecycleGeneration] = generation
	msg := &QueuedMessage{SessionID: sessionID, TaskID: taskID, Content: content, Model: model, PlanMode: planMode, Attachments: attachments, Metadata: metadataCopy, QueuedBy: userID}
	maxPerSession := s.MaxPerSession()
	if isRetry {
		maxPerSession = 0
	}
	var queued *QueuedMessage
	var replaced bool
	if identity == nil {
		queued, replaced, err = s.repo.InsertOrReplaceLifecycleByCoalesceKey(ctx, msg, coalesceKey, maxPerSession, allowInsert)
	} else {
		queued, replaced, err = s.repo.InsertOrReplaceLifecycleByCoalesceKeyForSession(ctx, *identity, msg, coalesceKey, maxPerSession, allowInsert)
	}
	if errors.Is(err, ErrTaskInactive) || errors.Is(err, ErrLifecycleCancelled) {
		return nil, false, false, nil
	}
	if err != nil {
		return nil, false, false, err
	}
	return queued, replaced, true, nil
}

// InvalidateEditLeasesForTask drops process-local edit leases after the task
// repository has purged its durable queue rows in the same lifecycle operation.
func (s *Service) InvalidateEditLeasesForTask(taskID string) {
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()
	s.invalidateEditLeasesForTaskLocked(taskID)
}

func (s *Service) invalidateEditLeasesForTaskLocked(taskID string) {
	for key, lease := range s.editLeases {
		if lease.taskID == taskID {
			s.deleteEditStateLocked(key)
		}
	}
	for key, revisionTaskID := range s.editRevisionTaskIDs {
		if revisionTaskID == taskID {
			s.deleteEditStateLocked(key)
		}
	}
}

// InvalidateEditLeasesForSession drops process-local edit leases after a task
// repository deletes that session's durable queue rows.
func (s *Service) InvalidateEditLeasesForSession(sessionID string) {
	s.invalidateEditLeasesLocked(sessionID)
}

func (s *Service) invalidateEditLease(sessionID, entryID string) {
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()
	s.deleteEditStateLocked(s.editLeaseKey(sessionID, entryID))
}

// ReleaseEditLeasesForConnection drops every lease owned by a disconnected
// WebSocket connection. Durable release is attempted before local removal,
// but a failed durable release cannot preserve a disconnected owner's lease.
func (s *Service) ReleaseEditLeasesForConnection(connectionID string) int {
	if connectionID == "" {
		return 0
	}
	s.editLeaseMu.Lock()
	leases := make([]*QueueEditLease, 0)
	for _, lease := range s.editLeases {
		if lease.connectionID == connectionID {
			leases = append(leases, cloneEditLease(lease))
		}
	}
	s.editLeaseMu.Unlock()
	released := 0
	repo, persistent := s.repo.(editLeaseRepository)
	for _, lease := range leases {
		if persistent {
			if err := repo.releaseEditLease(context.Background(), lease.SessionID, lease.EntryID, lease.LeaseID); err != nil {
				s.logger.Warn("failed to release durable queue edit lease",
					zap.String("session_id", lease.SessionID),
					zap.String("entry_id", lease.EntryID),
					zap.Error(err))
			}
		}
		s.editLeaseMu.Lock()
		key := s.editLeaseKey(lease.SessionID, lease.EntryID)
		if current := s.editLeases[key]; current != nil && current.LeaseID == lease.LeaseID {
			delete(s.editLeases, key)
			released++
		}
		s.editLeaseMu.Unlock()
	}
	return released
}

// PurgeTask is a backend-only task lifecycle operation. Client deletion APIs
// retain their reserved-entry protections.
func (s *Service) PurgeTask(ctx context.Context, taskID string) (int, error) {
	if token, ok := ctx.Value(sessionAdmissionContextKey{}).(sessionAdmissionToken); ok &&
		token.service == s && token.afterLifecycleUnlock != nil {
		purgeCtx := context.WithoutCancel(ctx)
		*token.afterLifecycleUnlock = append(*token.afterLifecycleUnlock, func() {
			if _, err := s.purgeTask(purgeCtx, taskID); err != nil {
				s.logger.Error("failed to purge task queue after admission",
					zap.String("task_id", taskID), zap.Error(err))
			}
		})
		return 0, nil
	}
	return s.purgeTask(ctx, taskID)
}

func (s *Service) purgeTask(ctx context.Context, taskID string) (int, error) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.editLeaseMu.Lock()
	defer s.editLeaseMu.Unlock()

	// The lifecycle barrier above serializes the persistent purge with all
	// queue admissions, including writes for sessions absent from the purge.
	removed, err := s.repo.PurgeTask(ctx, taskID)
	if err != nil {
		return 0, err
	}
	s.invalidateEditLeasesForTaskLocked(taskID)
	return removed, nil
}

// lifecycleGenerationFromMetadata reads the lifecycle generation captured on an entry.
func lifecycleGenerationFromMetadata(metadata map[string]interface{}) (int64, bool) {
	if metadata == nil {
		return 0, false
	}
	switch value := metadata[MetadataLifecycleGeneration].(type) {
	case int64:
		return value, true
	case int:
		return int64(value), true
	case float64:
		return int64(value), true
	default:
		return 0, false
	}
}

// ReserveQueued atomically takes an ordinary head entry or reserves a durable
// lifecycle head entry. The admission lock keeps drains from observing an
// insert before its automatic-merge finalization completes. A reserved
// lifecycle row survives until acknowledged. Auto-run OFF leaves the head in
// place and reports no reserved entry.
func (s *Service) ReserveQueued(ctx context.Context, sessionID string) (*QueuedMessage, bool) {
	msg, exists, _ := s.ReserveQueuedWithAutoRun(ctx, sessionID)
	return msg, exists
}

// ReserveQueuedWithAutoRun also reports the policy decision. Nil/false/true
// means enabled but empty (or a logged storage error); nil/false/false means
// Auto-run is OFF.
func (s *Service) ReserveQueuedWithAutoRun(ctx context.Context, sessionID string) (*QueuedMessage, bool, bool) {
	var msg *QueuedMessage
	autoRun := true
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		blocked, err := s.editLeaseBlocksHead(admittedCtx, sessionID)
		if err != nil {
			return err
		}
		if blocked {
			autoRun, err = s.repo.GetAutoRun(admittedCtx, sessionID)
			return err
		}
		msg, autoRun, err = s.repo.ReserveHeadIfAutoRun(admittedCtx, sessionID)
		return err
	})
	if err != nil {
		s.logger.Error("reserve head failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, false, true
	}
	return msg, msg != nil, autoRun
}

// ReserveQueuedWithAutoRunForSession reserves the FIFO head only for the exact session incarnation.
func (s *Service) ReserveQueuedWithAutoRunForSession(ctx context.Context, identity QueueSessionIdentity) (*QueuedMessage, bool, bool, error) {
	var msg *QueuedMessage
	var autoRun bool
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		var err error
		msg, autoRun, err = s.repo.ReserveHeadIfAutoRunForSession(admittedCtx, identity)
		return err
	})
	if err != nil {
		return nil, false, true, err
	}
	return msg, msg != nil, autoRun, nil
}

// ReserveQueuedForDeliveryWithAutoRunForSession retains the FIFO head until
// explicit acknowledgement, regardless of producer type.
func (s *Service) ReserveQueuedForDeliveryWithAutoRunForSession(
	ctx context.Context,
	identity QueueSessionIdentity,
) (*QueuedMessage, bool, bool, error) {
	var msg *QueuedMessage
	var autoRun bool
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		var err error
		msg, autoRun, err = s.repo.ReserveHeadForDeliveryIfAutoRunForSession(admittedCtx, identity)
		return err
	})
	if err != nil {
		return nil, false, true, err
	}
	return msg, msg != nil, autoRun, nil
}

// SetAutoRun persists automatic-drain policy through the queue admission gate.
func (s *Service) SetAutoRun(ctx context.Context, sessionID string, enabled bool) error {
	return s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		return s.repo.SetAutoRun(admittedCtx, sessionID, enabled)
	})
}

// SetAutoRunForSession persists automatic-drain policy only for the exact session incarnation.
func (s *Service) SetAutoRunForSession(ctx context.Context, identity QueueSessionIdentity, enabled bool) error {
	return s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		return s.repo.SetAutoRunForSession(admittedCtx, identity, enabled)
	})
}

// PauseAutoRunIfPending atomically parks a visible backlog, if one exists.
func (s *Service) PauseAutoRunIfPending(ctx context.Context, sessionID string) (bool, error) {
	var paused bool
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		var err error
		paused, err = s.repo.PauseAutoRunIfPending(admittedCtx, sessionID)
		return err
	})
	return paused, err
}
func (s *Service) PauseAutoRunIfPendingForSession(
	ctx context.Context,
	identity QueueSessionIdentity,
) (bool, error) {
	var paused bool
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		var err error
		paused, err = s.repo.PauseAutoRunIfPendingForSession(admittedCtx, identity)
		return err
	})
	return paused, err
}

// AcknowledgeQueued settles only the exact server reservation carried by msg.
func (s *Service) AcknowledgeQueued(ctx context.Context, msg *QueuedMessage) error {
	if msg == nil {
		return errors.New("queued message is nil")
	}
	return s.WithSessionAdmission(ctx, msg.SessionID, func(admittedCtx context.Context) error {
		err := s.repo.AcknowledgeReserved(admittedCtx, msg)
		if err != nil && !errors.Is(err, ErrEntryNotFound) {
			return err
		}
		return s.deletePendingQueueDispatch(admittedCtx, msg)
	})
}

func (s *Service) AcknowledgeQueuedForSession(
	ctx context.Context,
	identity QueueSessionIdentity,
	entryID string,
) error {
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		if err := s.validateSessionIdentity(admittedCtx, identity); err != nil {
			return err
		}
		return s.repo.AcknowledgeByIDForSession(admittedCtx, identity, entryID)
	})
	if errors.Is(err, ErrEntryNotFound) {
		return nil
	}
	return err
}

// ReleaseQueuedDeliveryForSession makes an unaccepted retained entry visible
// again for the same session incarnation.
func (s *Service) ReleaseQueuedDeliveryForSession(
	ctx context.Context,
	identity QueueSessionIdentity,
	entryID string,
) error {
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		return s.repo.ReleaseDeliveryReservationForSession(admittedCtx, identity, entryID)
	})
	if errors.Is(err, ErrEntryNotFound) {
		return nil
	}
	return err
}

// DiscardLifecycleReservation removes a reservation still owned by a replaced
// session incarnation. The repository compares persisted ownership, so this
// cannot delete a reservation subsequently claimed by another incarnation.
func (s *Service) DiscardLifecycleReservation(
	ctx context.Context,
	identity QueueSessionIdentity,
	entryID string,
) error {
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		return s.repo.DiscardLifecycleReservation(admittedCtx, identity, entryID)
	})
	if errors.Is(err, ErrEntryNotFound) {
		return nil
	}
	return err
}

// IsCurrentLifecycleReservation verifies that msg is still the durable row
// accepted for the task's current lifecycle generation. A task archive/delete
// removes that row and advances the generation, so callers can discard a
// deferred reservation before it performs any prompt side effects.
func (s *Service) IsCurrentLifecycleReservation(ctx context.Context, msg *QueuedMessage) bool {
	if msg == nil || !msg.IsDurableLifecycle() {
		return false
	}
	expectedGeneration, ok := lifecycleGenerationFromMetadata(msg.Metadata)
	if !ok {
		expectedGeneration = 0
	}
	generation, err := s.repo.LifecycleGeneration(ctx, msg.TaskID)
	if err != nil || generation != expectedGeneration {
		return false
	}
	if msg.reservationIdentity.SessionIncarnationID != "" {
		current, err := s.repo.ResolveSessionIdentity(
			ctx,
			msg.reservationIdentity.TaskID,
			msg.reservationIdentity.SessionID,
		)
		if err != nil || current != msg.reservationIdentity {
			return false
		}
	}
	entries, err := s.repo.ListBySession(ctx, msg.SessionID)
	if err != nil {
		return false
	}
	for i := range entries {
		entry := &entries[i]
		if entry.ID != msg.ID || entry.TaskID != msg.TaskID || !entry.IsDurableLifecycle() {
			continue
		}
		if msg.reservationIdentity.SessionIncarnationID != "" &&
			lifecycleReservationIncarnation(entry.Metadata) != msg.reservationIdentity.SessionIncarnationID {
			return false
		}
		return true
	}
	return false
}

// copyMessageMetadata clones metadata, optionally growing the map capacity.
func copyMessageMetadata(metadata map[string]interface{}, extraCapacity int) map[string]interface{} {
	if len(metadata) == 0 && extraCapacity == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(metadata)+extraCapacity)
	for key, value := range metadata {
		out[key] = copyMessageMetadataValue(value)
	}
	return out
}

func copyMessageMetadataValue(value interface{}) interface{} {
	cloned := copyMessageMetadataReflect(reflect.ValueOf(value))
	if !cloned.IsValid() {
		return nil
	}
	return cloned.Interface()
}

func copyMessageMetadataReflect(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := copyMessageMetadataReflect(value.Elem())
		out := reflect.New(value.Type()).Elem()
		out.Set(cloned)
		return out
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), copyMessageMetadataReflect(iter.Value()))
		}
		return out
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := range value.Len() {
			out.Index(i).Set(copyMessageMetadataReflect(value.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(value.Type()).Elem()
		for i := range value.Len() {
			out.Index(i).Set(copyMessageMetadataReflect(value.Index(i)))
		}
		return out
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.New(value.Type().Elem())
		out.Elem().Set(copyMessageMetadataReflect(value.Elem()))
		return out
	default:
		return value
	}
}

// AppendContent appends content onto the session's tail entry when the tail's
// queued_by matches userID. Otherwise inserts a new entry. Returns
// ErrQueueFull when an insert would exceed the cap.
func (s *Service) AppendContent(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []MessageAttachment) (*QueuedMessage, bool, error) {
	identity, err := s.repo.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		if errors.Is(err, ErrSessionIdentityMismatch) {
			var msg *QueuedMessage
			var appended bool
			err = s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
				blocked, blockErr := s.editLeaseBlocksTail(admittedCtx, sessionID, "", userID)
				if blockErr != nil {
					return blockErr
				}
				if blocked {
					return ErrEditConflict
				}
				msg, appended, err = s.repo.AppendOrInsertTail(
					admittedCtx, sessionID, taskID, content, model, userID, planMode, attachments, nil, s.MaxPerSession(),
				)
				return err
			})
			if err != nil {
				return nil, false, err
			}
			s.logger.Info("queue append-or-insert",
				zap.String("session_id", sessionID),
				zap.String("entry_id", msg.ID),
				zap.Bool("appended", appended))
			return msg, appended, nil
		}
		return nil, false, err
	}
	return s.AppendContentForSession(ctx, identity, content, model, userID, planMode, attachments)
}

// AppendContentForSession admits content only for the exact session incarnation.
func (s *Service) AppendContentForSession(ctx context.Context, identity QueueSessionIdentity, content, model, userID string, planMode bool, attachments []MessageAttachment) (*QueuedMessage, bool, error) {
	var msg *QueuedMessage
	var appended bool
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		blocked, err := s.editLeaseBlocksTail(admittedCtx, identity.SessionID, "", userID)
		if err != nil {
			return err
		}
		if blocked {
			return ErrEditConflict
		}
		msg, appended, err = s.repo.AppendOrInsertTailForSession(admittedCtx, identity, content, model, userID, planMode, attachments, nil, s.MaxPerSession())
		return err
	})
	if err != nil {
		return nil, false, err
	}
	s.logger.Info("queue append-or-insert",
		zap.String("session_id", identity.SessionID),
		zap.String("entry_id", msg.ID),
		zap.Bool("appended", appended))
	return msg, appended, nil
}

// TakeQueued atomically removes and returns the head entry. Returns nil, false
func (s *Service) TakeQueued(ctx context.Context, sessionID string) (*QueuedMessage, bool) {
	var msg *QueuedMessage
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		// TakeQueued is a destructive legacy cleanup path. Never let it
		// consume a durable lifecycle row that is already reserved for an
		// in-flight dispatch; that row must remain available for ack/retry.
		entries, err := s.repo.ListBySession(admittedCtx, sessionID)
		if err != nil {
			return err
		}
		if len(entries) > 0 && entries[0].IsReservedInFlight() {
			return nil
		}
		blocked, err := s.editLeaseBlocksHead(admittedCtx, sessionID)
		if err != nil || blocked {
			return err
		}
		msg, err = s.repo.TakeHead(admittedCtx, sessionID)
		if err == nil && msg != nil {
			s.editLeaseMu.Lock()
			s.deleteEditStateLocked(s.editLeaseKey(sessionID, msg.ID))
			s.editLeaseMu.Unlock()
		}
		return err
	})
	if err != nil {
		s.logger.Error("take head failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, false
	}
	if msg == nil {
		return nil, false
	}
	s.logger.Info("message dequeued",
		zap.String("session_id", sessionID),
		zap.String("entry_id", msg.ID))
	return msg, true
}

// TakeQueuedIfAutoRun atomically checks the session policy and removes the
// FIFO head only while Auto-run is enabled. The admission gate serializes the
// policy read with every queue mutation made through this service.
func (s *Service) TakeQueuedIfAutoRun(ctx context.Context, sessionID string) (*QueuedMessage, bool) {
	var msg *QueuedMessage
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		autoRun, err := s.repo.GetAutoRun(admittedCtx, sessionID)
		if err != nil || !autoRun {
			return err
		}
		entries, err := s.repo.ListBySession(admittedCtx, sessionID)
		if err != nil {
			return err
		}
		if len(entries) == 0 || entries[0].IsDurableLifecycle() || entries[0].IsReservedInFlight() {
			return nil
		}
		blocked, err := s.editLeaseBlocksHead(admittedCtx, sessionID)
		if err != nil || blocked {
			return err
		}
		msg, err = s.repo.TakeHead(admittedCtx, sessionID)
		if err == nil && msg != nil {
			s.editLeaseMu.Lock()
			s.deleteEditStateLocked(s.editLeaseKey(sessionID, msg.ID))
			s.editLeaseMu.Unlock()
		}
		return err
	})
	if err != nil {
		s.logger.Error("take Auto-run head failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, false
	}
	if msg == nil {
		return nil, false
	}
	s.logger.Info("Auto-run message dequeued",
		zap.String("session_id", sessionID),
		zap.String("entry_id", msg.ID))
	return msg, true
}

// TakeQueuedEntry atomically removes and returns the entry identified by
// entryID, regardless of its FIFO position. Returns nil, false, nil when
// the entry no longer exists (already drained or removed by a concurrent
// path); returns a non-nil error on a genuine repository failure — the two
// are deliberately distinguished so InterruptForPeerMessage can treat "not
// found" (safe to fall back to a FIFO-head drain) differently from "the
// repository call itself failed" (unsafe to fall back: a transient error
// here says nothing about which entry is actually at the FIFO head, and
// falling back anyway risks dispatching the wrong message while still
// reporting "sent" for the parent's — see InterruptForPeerMessage). Used by
// InterruptForPeerMessage to dispatch the specific message that triggered
// the interrupt instead of whatever happens to be at the FIFO head.
func (s *Service) TakeQueuedEntry(ctx context.Context, sessionID, entryID string) (*QueuedMessage, bool, error) {
	var msg *QueuedMessage
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		if s.editLeaseBlocksEntryLocked(sessionID, entryID) {
			return ErrEditConflict
		}
		var err error
		msg, err = s.repo.TakeByID(admittedCtx, sessionID, entryID)
		if err == nil && msg != nil {
			s.editLeaseMu.Lock()
			s.deleteEditStateLocked(s.editLeaseKey(sessionID, msg.ID))
			s.editLeaseMu.Unlock()
		}
		return err
	})
	if err != nil {
		s.logger.Error("take by id failed",
			zap.String("session_id", sessionID),
			zap.String("entry_id", entryID),
			zap.Error(err))
		return nil, false, err
	}
	if msg == nil {
		return nil, false, nil
	}
	s.logger.Info("message dequeued by id",
		zap.String("session_id", sessionID),
		zap.String("entry_id", msg.ID))
	return msg, true, nil
}

func (s *Service) TakeQueuedEntryForSession(
	ctx context.Context,
	identity QueueSessionIdentity,
	entryID string,
) (*QueuedMessage, bool, error) {
	var msg *QueuedMessage
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		var err error
		msg, err = s.repo.TakeByIDForSession(admittedCtx, identity, entryID)
		return err
	})
	if err != nil {
		s.logger.Error("take by id for session failed",
			zap.String("session_id", identity.SessionID),
			zap.String("entry_id", entryID),
			zap.Error(err))
		return nil, false, err
	}
	if msg == nil {
		return nil, false, nil
	}
	s.logger.Info("message dequeued by id",
		zap.String("session_id", identity.SessionID),
		zap.String("entry_id", msg.ID))
	return msg, true, nil
}

// UpdateMessage replaces the content (and optionally attachments) of a queued
// entry. The sessionID scope is mandatory — callers can't update an entry by
// guessing its UUID across sessions. Returns ErrEntryNotFound when the entry
// was already drained, the session doesn't own it, or the queuedBy guard
// rejects the caller.
func (s *Service) UpdateMessage(ctx context.Context, sessionID, entryID, content string, attachments []MessageAttachment, queuedBy string) error {
	return s.UpdateMessageWithMetadata(ctx, sessionID, entryID, content, attachments, nil, queuedBy)
}

// GetEntry returns a pending queue entry scoped to its session. It is used by
// lifecycle-aware callers that must inspect the trusted task ID and previous
// attachment descriptors before replacing an entry.
func (s *Service) GetEntry(ctx context.Context, sessionID, entryID string) (*QueuedMessage, error) {
	var entry *QueuedMessage
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		entries, err := s.repo.ListBySession(admittedCtx, sessionID)
		if err != nil {
			return err
		}
		for i := range entries {
			if entries[i].ID == entryID {
				found := entries[i]
				entry = &found
				return nil
			}
		}
		return ErrEntryNotFound
	})
	return entry, err
}

type entryByIDRepository interface {
	FindByID(context.Context, string) (*QueuedMessage, error)
}

// FindEntryByID locates a queue entry after a session transfer has moved it
// out of its original session.
func (s *Service) FindEntryByID(ctx context.Context, entryID string) (*QueuedMessage, error) {
	repository, ok := s.repo.(entryByIDRepository)
	if !ok {
		return nil, ErrEntryNotFound
	}
	return repository.FindByID(ctx, entryID)
}

// ReferencedQueueAttachmentIDs returns attachment IDs referenced by pending
// entries in a session, excluding one entry. It is admission-aware so callers
// can use it while already holding the session admission lock.
func (s *Service) ReferencedQueueAttachmentIDs(
	ctx context.Context, sessionID, excludedEntryID string, attachmentIDs []string,
) (map[string]struct{}, error) {
	referenced := make(map[string]struct{}, len(attachmentIDs))
	if len(attachmentIDs) == 0 {
		return referenced, nil
	}
	read := func(admittedCtx context.Context) error {
		entries, err := s.repo.ListBySession(admittedCtx, sessionID)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.ID == excludedEntryID {
				continue
			}
			for _, attachment := range entry.Attachments {
				if attachment.AttachmentID != "" {
					referenced[attachment.AttachmentID] = struct{}{}
				}
			}
		}
		return nil
	}
	if err := s.WithSessionAdmission(ctx, sessionID, read); err != nil {
		return nil, err
	}
	return referenced, nil
}

// ClaimSendNow atomically claims the exact pending source snapshot for an
// interrupt-and-replace dispatch. The repository orders the retained sources
// by FIFO position and constructs the synthetic dispatch envelope before
// mutating any row, so aggregate validation failures or click-time edits leave
// the queue untouched.
func (s *Service) ClaimSendNow(ctx context.Context, sessionID string, expected []QueuedMessage) (*SendNowClaim, error) {
	var claim *SendNowClaim
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		for _, entry := range expected {
			if s.editLeaseBlocksEntryLocked(sessionID, entry.ID) {
				return ErrEditConflict
			}
		}
		var err error
		claim, err = s.repo.ClaimSendNow(admittedCtx, sessionID, expected)
		if err == nil && claim != nil {
			s.editLeaseMu.Lock()
			for _, source := range claim.Sources {
				if !source.IsDurableLifecycle() {
					s.deleteEditStateLocked(s.editLeaseKey(sessionID, source.ID))
				}
			}
			s.editLeaseMu.Unlock()
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info("claimed queued messages for send now",
		zap.String("session_id", sessionID),
		zap.Int("source_count", len(claim.Sources)))
	return claim, nil
}

// ClaimSendNowForSession claims the exact pending snapshot only for the exact session incarnation.
func (s *Service) ClaimSendNowForSession(ctx context.Context, identity QueueSessionIdentity, expected []QueuedMessage) (*SendNowClaim, error) {
	var claim *SendNowClaim
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		var err error
		claim, err = s.repo.ClaimSendNowForSession(admittedCtx, identity, expected)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info("claimed queued messages for send now",
		zap.String("session_id", identity.SessionID),
		zap.Int("source_count", len(claim.Sources)))
	return claim, nil
}

// RestoreSendNowClaim restores every source from an interrupted replacement
// dispatch. Durable lifecycle reservations are cleared as part of the same
// repository operation.
func sendNowClaimSessionID(claim *SendNowClaim) (string, error) {
	if claim == nil || len(claim.Sources) == 0 {
		return "", ErrSendNowEmpty
	}
	sessionID := claim.Dispatch.SessionID
	for _, source := range claim.Sources {
		if source.SessionID == "" {
			return "", ErrSendNowClaimChanged
		}
		if sessionID == "" {
			sessionID = source.SessionID
		}
		if source.SessionID != sessionID {
			return "", ErrSendNowClaimChanged
		}
	}
	return sessionID, nil
}

func (s *Service) RestoreSendNowClaim(ctx context.Context, claim *SendNowClaim) error {
	if claim == nil {
		return s.repo.RestoreSendNowClaim(ctx, claim)
	}
	sessionID, err := sendNowClaimSessionID(claim)
	if err != nil {
		return err
	}
	if err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		return s.repo.RestoreSendNowClaim(admittedCtx, claim)
	}); err != nil {
		return err
	}
	s.logger.Info("restored send-now queue claim",
		zap.String("session_id", sessionID),
		zap.Int("source_count", len(claim.Sources)))
	return nil
}

// AcknowledgeSendNowClaim removes durable lifecycle sources after the
// replacement prompt has been accepted. Ordinary sources were deleted at
// claim time and therefore need no second acknowledgement.
func (s *Service) AcknowledgeSendNowClaim(ctx context.Context, claim *SendNowClaim) error {
	if claim == nil {
		return s.repo.AcknowledgeSendNowClaim(ctx, claim)
	}
	sessionID, err := sendNowClaimSessionID(claim)
	if err != nil {
		return err
	}
	if err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		return s.repo.AcknowledgeSendNowClaim(admittedCtx, claim)
	}); err != nil {
		return err
	}
	s.logger.Info("acknowledged send-now queue claim",
		zap.String("session_id", sessionID),
		zap.Int("source_count", len(claim.Sources)))
	return nil
}

type pendingSendNowClaimRepository interface {
	ListPendingSendNowClaims(context.Context) ([]PendingSendNowClaim, error)
	MarkPendingSendNowClaimAccepted(context.Context, *SendNowClaim) error
	DeletePendingSendNowClaim(context.Context, *SendNowClaim) error
}

// PendingSendNowClaimPersistenceAvailable reports whether ordinary claimed
// prompts can be recovered after the owning process exits.
func (s *Service) PendingSendNowClaimPersistenceAvailable() bool {
	_, ok := s.repo.(pendingSendNowClaimRepository)
	return ok
}

// ListPendingSendNowClaims reloads interrupted Send Now claims after restart.
func (s *Service) ListPendingSendNowClaims(ctx context.Context) ([]PendingSendNowClaim, error) {
	repo, ok := s.repo.(pendingSendNowClaimRepository)
	if !ok {
		return nil, errors.New("pending Send Now claim persistence unavailable")
	}
	return repo.ListPendingSendNowClaims(ctx)
}

// MarkPendingSendNowClaimAccepted records the executor acceptance boundary
// before the potentially long-running prompt call returns.
func (s *Service) MarkPendingSendNowClaimAccepted(ctx context.Context, claim *SendNowClaim) error {
	repo, ok := s.repo.(pendingSendNowClaimRepository)
	if !ok {
		return errors.New("pending Send Now claim persistence unavailable")
	}
	sessionID, err := sendNowClaimSessionID(claim)
	if err != nil {
		return err
	}
	return s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		return repo.MarkPendingSendNowClaimAccepted(admittedCtx, claim)
	})
}

// DeletePendingSendNowClaim discards only the exact recovery record that can
// no longer be applied because a destructive queue generation change
// superseded it.
func (s *Service) DeletePendingSendNowClaim(ctx context.Context, claim *SendNowClaim) error {
	repo, ok := s.repo.(pendingSendNowClaimRepository)
	if !ok {
		return errors.New("pending Send Now claim persistence unavailable")
	}
	sessionID, err := sendNowClaimSessionID(claim)
	if err != nil {
		return err
	}
	return s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		return repo.DeletePendingSendNowClaim(admittedCtx, claim)
	})
}

type pendingQueueDispatchRepository interface {
	ListPendingQueueDispatches(context.Context) ([]PendingQueueDispatch, error)
	MarkPendingQueueDispatchAccepted(context.Context, *QueuedMessage) error
	DeletePendingQueueDispatch(context.Context, *QueuedMessage) error
}

// PendingQueueDispatchPersistenceAvailable reports whether ordinary dequeues
// survive a process exit before executor acceptance.
func (s *Service) PendingQueueDispatchPersistenceAvailable() bool {
	_, ok := s.repo.(pendingQueueDispatchRepository)
	return ok
}

func (s *Service) ListPendingQueueDispatches(ctx context.Context) ([]PendingQueueDispatch, error) {
	repo, ok := s.repo.(pendingQueueDispatchRepository)
	if !ok {
		return nil, errors.New("pending queue dispatch persistence unavailable")
	}
	return repo.ListPendingQueueDispatches(ctx)
}

func (s *Service) MarkPendingQueueDispatchAccepted(
	ctx context.Context,
	msg *QueuedMessage,
) error {
	repo, ok := s.repo.(pendingQueueDispatchRepository)
	if !ok {
		return nil
	}
	return s.WithSessionAdmission(ctx, msg.SessionID, func(admittedCtx context.Context) error {
		return repo.MarkPendingQueueDispatchAccepted(admittedCtx, msg)
	})
}

// DeletePendingQueueDispatch acknowledges the exact recovered or accepted
// ordinary dispatch attempt carried by msg.
func (s *Service) DeletePendingQueueDispatch(ctx context.Context, msg *QueuedMessage) error {
	return s.WithSessionAdmission(ctx, msg.SessionID, func(admittedCtx context.Context) error {
		return s.deletePendingQueueDispatch(admittedCtx, msg)
	})
}

func (s *Service) deletePendingQueueDispatch(ctx context.Context, msg *QueuedMessage) error {
	repo, ok := s.repo.(pendingQueueDispatchRepository)
	if !ok || msg.IsDurableLifecycle() {
		return nil
	}
	return repo.DeletePendingQueueDispatch(ctx, msg)
}

// GetEntryForSession returns an entry from an identity-bound repository
// snapshot, preventing edits from observing a reincarnated session.
func (s *Service) GetEntryForSession(ctx context.Context, identity QueueSessionIdentity, entryID string) (*QueuedMessage, error) {
	snapshot, err := s.repo.Snapshot(ctx, identity)
	if err != nil {
		return nil, err
	}
	for index := range snapshot.Entries {
		if snapshot.Entries[index].ID == entryID {
			entry := snapshot.Entries[index]
			return &entry, nil
		}
	}
	return nil, ErrEntryNotFound
}

// UpdateMessageWithMetadata atomically edits queue content and applies
// metadata replacements while retaining unrelated metadata keys.
func (s *Service) UpdateMessageWithMetadata(ctx context.Context, sessionID, entryID, content string, attachments []MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string) error {
	if err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		if s.editLeaseBlocksEntryLocked(sessionID, entryID) {
			return ErrEditConflict
		}

		return s.repo.UpdateContentAndMetadata(admittedCtx, sessionID, entryID, content, attachments, metadataUpdates, queuedBy)
	}); err != nil {
		return err
	}
	s.logger.Info("queued entry updated",
		zap.String("session_id", sessionID),
		zap.String("entry_id", entryID))
	return nil
}

// RemoveEntryWithEntry deletes a single entry and returns the exact snapshot
// removed under the same session admission lock. Callers that release
// entry-owned resources must use this method instead of GetEntry followed by
// RemoveEntry, because an edit between those calls can change attachments.
func (s *Service) RemoveEntryWithEntry(ctx context.Context, sessionID, entryID string) (*QueuedMessage, error) {
	var removed *QueuedMessage
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		entries, err := s.repo.ListBySession(admittedCtx, sessionID)
		if err != nil {
			return err
		}
		for i := range entries {
			if entries[i].ID != entryID {
				continue
			}
			entry := entries[i]
			if err := s.repo.DeleteByID(admittedCtx, sessionID, entryID); err != nil {
				return err
			}
			removed = &entry
			s.editLeaseMu.Lock()
			s.deleteEditStateLocked(s.editLeaseKey(sessionID, entryID))
			s.editLeaseMu.Unlock()
			return nil
		}
		return ErrEntryNotFound
	})
	if err != nil {
		return nil, err
	}
	return removed, nil
}

// UpdateMessageWithMetadataForSession edits content only for the exact session incarnation.
func (s *Service) UpdateMessageWithMetadataForSession(ctx context.Context, identity QueueSessionIdentity, entryID, content string, attachments []MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string) error {
	if err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		return s.repo.UpdateContentAndMetadataForSession(admittedCtx, identity, entryID, content, attachments, metadataUpdates, queuedBy)
	}); err != nil {
		return err
	}
	s.logger.Info("queued entry updated",
		zap.String("session_id", identity.SessionID),
		zap.String("entry_id", entryID))
	return nil
}

// UpdateMessageWithMetadataForSessionWithClaim claims newly referenced staged
// attachments and updates the exact queue row atomically.
func (s *Service) UpdateMessageWithMetadataForSessionWithClaim(ctx context.Context, identity QueueSessionIdentity, entryID, content string, attachments []MessageAttachment, metadataUpdates map[string]interface{}, queuedBy string, claim QueueAttachmentClaim) error {
	if err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		claimRepo, ok := s.repo.(queueAttachmentRepository)
		if !ok {
			return errors.New("transactional attachment queue repository is unavailable")
		}
		return claimRepo.UpdateContentAndMetadataForSessionWithClaim(admittedCtx, identity, entryID, content, attachments, metadataUpdates, queuedBy, claim)
	}); err != nil {
		return err
	}
	s.logger.Info("queued entry updated",
		zap.String("session_id", identity.SessionID),
		zap.String("entry_id", entryID))
	return nil
}

// RemoveEntry deletes a single entry. The sessionID scope is mandatory — see
// the rationale on the Repository.DeleteByID contract for why. Returns
// ErrEntryNotFound when no entry matches.
func (s *Service) RemoveEntry(ctx context.Context, sessionID, entryID string) error {
	_, err := s.RemoveEntryWithEntry(ctx, sessionID, entryID)
	if err != nil {
		return err
	}
	s.logger.Info("queued entry removed",
		zap.String("session_id", sessionID),
		zap.String("entry_id", entryID))
	return nil
}

// RemoveEntryForSession deletes an entry only for the exact session incarnation
// and returns the removed entry plus every retained row from the same transaction.
func (s *Service) RemoveEntryForSession(ctx context.Context, identity QueueSessionIdentity, entryID string) (*QueueRemovalResult, error) {
	var result *QueueRemovalResult
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		var err error
		result, err = s.repo.DeleteByIDForSession(admittedCtx, identity, entryID)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info("queued entry removed",
		zap.String("session_id", identity.SessionID),
		zap.String("entry_id", entryID))
	return result, nil
}

// MergeIntoAbove folds the entry identified by entryID into the entry directly
// above it within the same session. See Repository.MergeIntoAbove for the merge
// rules and error mapping (ErrEntryNotFound / ErrNoMergeTarget).
func (s *Service) MergeIntoAbove(ctx context.Context, sessionID, entryID, queuedBy string) (*QueuedMessage, error) {
	if !s.MergeEnabled() {
		return nil, ErrMergeDisabled
	}
	var merged *QueuedMessage
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		blocked, err := s.editLeaseBlocksMerge(admittedCtx, sessionID, entryID)
		if err != nil {
			return err
		}
		if blocked {
			return ErrEditConflict
		}
		var mergeErr error
		merged, mergeErr = s.repo.MergeIntoAbove(admittedCtx, sessionID, entryID, queuedBy)
		if mergeErr == nil {
			s.invalidateEditLease(sessionID, entryID)
		}
		return mergeErr
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info("queued entry merged into entry above",
		zap.String("session_id", sessionID),
		zap.String("entry_id", merged.ID))
	return merged, nil
}

// MergeIntoAboveForSession merges an entry only for the exact session incarnation.
func (s *Service) MergeIntoAboveForSession(ctx context.Context, identity QueueSessionIdentity, entryID, queuedBy string) (*QueuedMessage, error) {
	if !s.MergeEnabled() {
		return nil, ErrMergeDisabled
	}
	var merged *QueuedMessage
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		var err error
		merged, err = s.repo.MergeIntoAboveForSession(admittedCtx, identity, entryID, queuedBy)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info("queued entry merged into entry above",
		zap.String("session_id", identity.SessionID),
		zap.String("entry_id", merged.ID))
	return merged, nil
}

// ReorderEntries rewrites the pending FIFO order of a session's queue to
// match orderedIDs. Returns ErrQueueChanged when the submitted order is empty,
// contains duplicates, or does not match the current visible pending set (a
// drain/remove/merge raced the request); callers refetch the authoritative
// queue.
func (s *Service) ReorderEntries(ctx context.Context, sessionID string, orderedIDs []string) error {
	if err := validateReorderInput(orderedIDs); err != nil {
		return err
	}
	if err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		if s.editLeaseBlocksReorderLocked(sessionID) {
			return ErrEditConflict
		}
		return s.repo.ReorderEntries(admittedCtx, sessionID, orderedIDs)
	}); err != nil {
		return err
	}
	s.logger.Info("queued entries reordered",
		zap.String("session_id", sessionID),
		zap.Int("entry_count", len(orderedIDs)))
	return nil
}

// ReorderEntriesForSession reorders pending entries only for the exact session incarnation.
func (s *Service) ReorderEntriesForSession(ctx context.Context, identity QueueSessionIdentity, orderedIDs []string) error {
	if err := validateReorderInput(orderedIDs); err != nil {
		return err
	}
	if err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		return s.repo.ReorderEntriesForSession(admittedCtx, identity, orderedIDs)
	}); err != nil {
		return err
	}
	s.logger.Info("queued entries reordered",
		zap.String("session_id", identity.SessionID),
		zap.Int("entry_count", len(orderedIDs)))
	return nil
}

// CancelAll clears every queued entry for a session. Returns the number of
// rows removed.
func (s *Service) CancelAll(ctx context.Context, sessionID string) (int, error) {
	var n int
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		var err error
		n, err = s.repo.DeleteAllBySession(admittedCtx, sessionID)
		if err == nil {
			s.invalidateEditLeasesLocked(sessionID)
		}
		return err
	})
	if err != nil {
		return 0, err
	}
	s.logger.Info("queue cancel-all",
		zap.String("session_id", sessionID),
		zap.Int("removed", n))
	return n, nil
}

// PurgeSession removes all queue rows for a deleted session, including
// reserved lifecycle deliveries. It is intentionally distinct from
// CancelAll, which preserves in-flight durable rows for executor recovery.
func (s *Service) PurgeSession(ctx context.Context, sessionID string) (int, error) {
	var removed int
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		var err error
		removed, err = s.repo.PurgeSession(admittedCtx, sessionID)
		if err == nil {
			s.invalidateEditLeasesLocked(sessionID)
		}
		return err
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
}

// CancelAllWithEntries clears a session queue and returns the deleted rows so
// callers can release resources owned by those entries.
func (s *Service) CancelAllWithEntries(ctx context.Context, sessionID string) ([]QueuedMessage, error) {
	var entries []QueuedMessage
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		var err error
		entries, err = s.repo.ListBySession(admittedCtx, sessionID)
		if err != nil {
			return err
		}
		_, err = s.repo.DeleteAllBySession(admittedCtx, sessionID)
		if err == nil {
			deleted := entries[:0]
			for i := range entries {
				if !entries[i].IsReservedInFlight() {
					deleted = append(deleted, entries[i])
				}
			}
			entries = deleted
			s.invalidateEditLeasesLocked(sessionID)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// CancelAllForSession clears pending entries only for the exact session
// incarnation and returns both removed and retained rows atomically.
func (s *Service) CancelAllForSession(ctx context.Context, identity QueueSessionIdentity) (*QueueRemovalResult, error) {
	var result *QueueRemovalResult
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		var err error
		result, err = s.repo.DeleteAllBySessionForIdentity(admittedCtx, identity)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info("queue cancel-all",
		zap.String("session_id", identity.SessionID),
		zap.Int("removed", len(result.Removed)))
	return result, nil
}

// PurgeDeletedSession removes all ephemeral queue state after the authoritative
// task-session row is gone. It never matches a replacement incarnation.
func (s *Service) PurgeDeletedSession(ctx context.Context, identity QueueSessionIdentity) error {
	purger, ok := s.repo.(deletedSessionRepository)
	if !ok {
		return nil
	}
	return s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		return purger.PurgeDeletedSession(admittedCtx, identity)
	})
}

// GetStatus returns the full pending list and capacity info for a session.
func (s *Service) GetStatus(ctx context.Context, sessionID string) *QueueStatus {
	maxPerSession := s.MaxPerSession()
	mergeEnabled := s.MergeEnabled()
	status := &QueueStatus{
		Entries:      []QueuedMessage{},
		Count:        0,
		Max:          maxPerSession,
		AutoRun:      true,
		MergeEnabled: mergeEnabled,
	}
	_ = s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		autoRun, autoRunErr := s.repo.GetAutoRun(admittedCtx, sessionID)
		if autoRunErr != nil {
			s.logger.Error("get queue auto-run failed",
				zap.String("session_id", sessionID),
				zap.Error(autoRunErr))
			// Preserve pre-policy behavior if status storage is temporarily unreadable.
			autoRun = true
		}
		entries, err := s.repo.ListBySession(admittedCtx, sessionID)
		if err != nil {
			s.logger.Error("list queued failed",
				zap.String("session_id", sessionID),
				zap.Error(err))
			status = &QueueStatus{
				Entries:      []QueuedMessage{},
				Count:        0,
				Max:          maxPerSession,
				AutoRun:      autoRun,
				MergeEnabled: mergeEnabled,
			}
			return nil
		}
		pending := make([]QueuedMessage, 0, len(entries))
		for _, entry := range entries {
			// A reserved lifecycle row is already being delivered; listing it next
			// to the message it produced reads as a stuck duplicate.
			if entry.IsReservedInFlight() {
				continue
			}
			// Repositories may return shallow copies. The status is also handed to
			// asynchronous event publishers, so detach maps and slices before the
			// admission lock is released.
			pending = append(pending, *cloneQueuedMessage(&entry))
		}
		status = &QueueStatus{
			Entries:      pending,
			Count:        len(pending),
			Max:          maxPerSession,
			AutoRun:      autoRun,
			MergeEnabled: mergeEnabled,
		}
		return nil
	})
	return status
}

// Snapshot returns an ordered status bound to one immutable session identity.
func (s *Service) Snapshot(ctx context.Context, identity QueueSessionIdentity) (*QueueStatus, error) {
	snapshot, err := s.repo.Snapshot(ctx, identity)
	if err != nil {
		return nil, err
	}
	pending := make([]QueuedMessage, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		if !entry.IsReservedInFlight() {
			pending = append(pending, entry)
		}
	}
	status := &QueueStatus{
		Entries:              pending,
		Count:                len(pending),
		Max:                  s.MaxPerSession(),
		TaskID:               identity.TaskID,
		SessionID:            identity.SessionID,
		SessionIncarnationID: identity.SessionIncarnationID,
		StatusEpoch:          s.statusEpoch,
		StatusGeneration:     snapshot.StatusGeneration,
		AutoRun:              snapshot.AutoRun,
		MergeEnabled:         s.MergeEnabled(),
	}
	if snapshot.AutoMergeError != nil {
		s.logger.Warn("automatic merge policy unavailable while reading queue status",
			zap.String("session_id", identity.SessionID),
			zap.Error(snapshot.AutoMergeError))
		return status, nil
	}
	var policy AutoMergePolicy
	if snapshot.AutoMergeOverride != nil {
		policy = AutoMergePolicy{
			Enabled: snapshot.AutoMergeOverride.Enabled, Source: AutoMergeSourceSession,
			Revision: snapshot.AutoMergeOverride.Revision,
		}
	} else {
		policy, err = s.loadGlobalAutoMergePolicy(ctx)
		if err != nil {
			s.logger.Warn("global automatic merge policy unavailable while reading queue status",
				zap.String("session_id", identity.SessionID),
				zap.Error(err))
			return status, nil
		}
	}
	status.AutoMergeAvailable = true
	status.AutoMergeEnabled = &policy.Enabled
	status.AutoMergeSource = policy.Source
	status.AutoMergeRevision = &policy.Revision
	return status, nil
}

// CountPendingByTaskIDs returns the pending prompt count per task, keyed by
// task_id, for every requested task. Reserved in-flight lifecycle rows are
// excluded, matching GetStatus. Used by task-list assembly and the status
// summary projector to render per-task queued-prompt badges.
func (s *Service) CountPendingByTaskIDs(ctx context.Context, taskIDs []string) (map[string]int, error) {
	counts, err := s.repo.CountPendingByTaskIDs(ctx, taskIDs)
	if err != nil {
		s.logger.Error("count pending by task ids failed",
			zap.Int("task_count", len(taskIDs)),
			zap.Error(err))
		return nil, err
	}
	return counts, nil
}

// CountPendingByTask returns the pending prompt count for one task.
func (s *Service) CountPendingByTask(ctx context.Context, taskID string) (int, error) {
	counts, err := s.CountPendingByTaskIDs(ctx, []string{taskID})
	if err != nil {
		return 0, err
	}
	return counts[taskID], nil
}

// SnapshotSession returns the complete persisted queue state for rollback.
// Unlike GetStatus, it includes durable lifecycle rows reserved by an in-flight
// delivery so a later restore cannot erase them before acknowledgement.
func (s *Service) SnapshotSession(ctx context.Context, sessionID string) ([]QueuedMessage, *PendingMove, error) {
	var entries []QueuedMessage
	var move *PendingMove
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		rawEntries, err := s.repo.ListBySession(admittedCtx, sessionID)
		if err != nil {
			return fmt.Errorf("snapshot queued messages: %w", err)
		}
		entries = make([]QueuedMessage, 0, len(rawEntries))
		for i := range rawEntries {
			entries = append(entries, *cloneQueuedMessage(&rawEntries[i]))
		}
		move, err = s.repo.GetPendingMove(admittedCtx, sessionID)
		if err != nil {
			return fmt.Errorf("snapshot pending move: %w", err)
		}
		if move != nil {
			moveCopy := *move
			move = &moveCopy
		}
		return nil
	})
	return entries, move, err
}

type ownedSessionTransferRepository interface {
	transferSessionOwned(context.Context, string, string, string) error
}

func (s *Service) transferRepositorySession(
	ctx context.Context,
	oldSessionID, newSessionID, operationID string,
) error {
	if operationID == "" {
		return s.repo.TransferSession(ctx, oldSessionID, newSessionID)
	}
	repo, ok := s.repo.(ownedSessionTransferRepository)
	if !ok {
		return errors.New("owned session transfer unavailable")
	}
	return repo.transferSessionOwned(ctx, oldSessionID, newSessionID, operationID)
}

// SnapshotSessionForIdentity captures queue and deferred-move state for one exact incarnation.
func (s *Service) SnapshotSessionForIdentity(ctx context.Context, identity QueueSessionIdentity) ([]QueuedMessage, *PendingMove, error) {
	snapshot, err := s.repo.Snapshot(ctx, identity)
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot session queue: %w", err)
	}
	return snapshot.Entries, snapshot.PendingMove, nil
}

// TransferSession moves any queued messages and pending move from one session
// to another. Used by workflow session switches. Returns an error so the
// caller can fail closed instead of silently leaving entries orphaned on the
// old session — a transfer that no-ops without a signal would let the workflow
// step move forward while the queue sticks behind.
func (s *Service) TransferSession(ctx context.Context, oldSessionID, newSessionID string) error {
	return s.transferSession(ctx, "", oldSessionID, newSessionID, "", nil, nil)
}

// TransferSessionWithPreparation runs preparation while both session
// admissions are held, then transfers queued state. If the queue transfer
// fails, rollback runs under the same admissions before they are released.
// Callers use this to keep external state, such as attachment ownership,
// synchronized with the queue move.
func (s *Service) TransferSessionWithPreparation(
	ctx context.Context,
	oldSessionID, newSessionID string,
	prepare func(context.Context) error,
	rollback func(context.Context) error,
) error {
	return s.transferSession(ctx, "", oldSessionID, newSessionID, "", prepare, rollback)
}

func (s *Service) transferSession(
	ctx context.Context,
	taskID string,
	oldSessionID, newSessionID, operationID string,
	prepare func(context.Context) error,
	rollback func(context.Context) error,
) error {
	err := s.withSessionAdmissions(ctx, oldSessionID, newSessionID, func(admittedCtx context.Context) error {
		rollbackPreparation := func(transferErr error) error {
			if rollback == nil {
				return transferErr
			}
			rollbackErr := rollback(context.WithoutCancel(admittedCtx))
			if rollbackErr != nil {
				s.logger.Warn("failed to roll back transfer preparation",
					zap.String("from_session_id", oldSessionID),
					zap.String("to_session_id", newSessionID),
					zap.Error(rollbackErr))
			}
			return errors.Join(transferErr, rollbackErr)
		}
		if prepare != nil {
			if err := prepare(admittedCtx); err != nil {
				return rollbackPreparation(err)
			}
		}
		if err := s.transferRepositorySessionForTask(
			admittedCtx, taskID, oldSessionID, newSessionID, operationID,
		); err != nil {
			return rollbackPreparation(err)
		}
		s.invalidateEditLeasesLocked(oldSessionID, newSessionID)
		return nil
	})
	if err != nil {
		s.logger.Error("transfer session failed",
			zap.String("from_session_id", oldSessionID),
			zap.String("to_session_id", newSessionID),
			zap.Error(err))
		return err
	}
	s.logger.Info("transferred queue between sessions",
		zap.String("from_session_id", oldSessionID),
		zap.String("to_session_id", newSessionID))
	return nil
}

func (s *Service) transferRepositorySessionForTask(
	ctx context.Context,
	taskID, oldSessionID, newSessionID, operationID string,
) error {
	// Transfers that do not have a durable compensation record can use the
	// immutable session identities. This fences a workflow handoff to one task
	// and lets repositories reject a stale or cross-task destination. Durable
	// transfers keep the owned-operation path because it also carries the
	// compensation lease and operation token.
	if taskID != "" && operationID == "" {
		source, sourceErr := s.repo.ResolveSessionIdentity(ctx, taskID, oldSessionID)
		if sourceErr != nil && !errors.Is(sourceErr, ErrSessionIdentityMismatch) {
			return sourceErr
		}
		destination, destinationErr := s.repo.ResolveSessionIdentity(ctx, taskID, newSessionID)
		if destinationErr != nil && !errors.Is(destinationErr, ErrSessionIdentityMismatch) {
			return destinationErr
		}
		if sourceErr == nil && destinationErr == nil {
			return s.repo.TransferSessionIdentities(ctx, source, destination)
		}
	}
	return s.transferRepositorySession(ctx, oldSessionID, newSessionID, operationID)
}

type attachmentCleanupRepository interface {
	UpsertAttachmentCleanup(context.Context, AttachmentCleanup) error
	DeleteAttachmentCleanup(context.Context, string, string, string) error
	ListAttachmentCleanups(context.Context) ([]AttachmentCleanup, error)
}

// AttachmentCleanupPersistenceAvailable reports whether this service's
// repository can preserve cleanup work across handler restarts.
func (s *Service) AttachmentCleanupPersistenceAvailable() bool {
	_, ok := s.repo.(attachmentCleanupRepository)
	return ok
}

// UpsertAttachmentCleanup persists a cleanup obligation before its retry
// worker is allowed to outlive the current handler.
func (s *Service) UpsertAttachmentCleanup(ctx context.Context, cleanup AttachmentCleanup) error {
	repo, ok := s.repo.(attachmentCleanupRepository)
	if !ok {
		return errors.New("attachment cleanup persistence unavailable")
	}
	return repo.UpsertAttachmentCleanup(ctx, cleanup)
}

// DeleteAttachmentCleanup acknowledges one completed cleanup obligation.
func (s *Service) DeleteAttachmentCleanup(
	ctx context.Context,
	sessionID, entryID, operationID string,
) error {
	repo, ok := s.repo.(attachmentCleanupRepository)
	if !ok {
		return errors.New("attachment cleanup persistence unavailable")
	}
	return repo.DeleteAttachmentCleanup(ctx, sessionID, entryID, operationID)
}

// ListAttachmentCleanups reloads cleanup obligations after process restart.
func (s *Service) ListAttachmentCleanups(ctx context.Context) ([]AttachmentCleanup, error) {
	repo, ok := s.repo.(attachmentCleanupRepository)
	if !ok {
		return nil, errors.New("attachment cleanup persistence unavailable")
	}
	return repo.ListAttachmentCleanups(ctx)
}

type attachmentCleanupLocatorRepository interface {
	GetAttachmentCleanup(context.Context, string, string, string) (*AttachmentCleanup, error)
}

// GetAttachmentCleanup reloads one cleanup by its immutable acknowledgement key.
func (s *Service) GetAttachmentCleanup(
	ctx context.Context,
	sessionID, entryID, operationID string,
) (*AttachmentCleanup, error) {
	repo, ok := s.repo.(attachmentCleanupLocatorRepository)
	if !ok {
		return nil, errors.New("attachment cleanup persistence unavailable")
	}
	return repo.GetAttachmentCleanup(ctx, sessionID, entryID, operationID)
}

type sessionTransferCompensationRepository interface {
	UpsertSessionTransferCompensation(context.Context, SessionTransferCompensation) error
	DeleteSessionTransferCompensation(context.Context, string, string, string, string) error
	ListSessionTransferCompensations(context.Context) ([]SessionTransferCompensation, error)
}

type sessionTransferCompensationRecoveryRepository interface {
	claimSessionTransferCompensationRecovery(context.Context, SessionTransferCompensation) (string, error)
	renewSessionTransferCompensationLease(context.Context, SessionTransferCompensation, string) error
	deleteSessionTransferCompensationWithOwner(context.Context, string, string, string, string, string) error
}

// SessionTransferCompensationPersistenceAvailable reports whether interrupted
// queue and attachment transfers can be reconciled after process restart.
func (s *Service) SessionTransferCompensationPersistenceAvailable() bool {
	_, ok := s.repo.(sessionTransferCompensationRepository)
	return ok
}

// UpsertSessionTransferCompensation persists intent before attachment ownership
// changes outside the queue repository transaction.
func (s *Service) UpsertSessionTransferCompensation(
	ctx context.Context,
	compensation SessionTransferCompensation,
) error {
	repo, ok := s.repo.(sessionTransferCompensationRepository)
	if !ok {
		return errors.New("session transfer compensation persistence unavailable")
	}
	return repo.UpsertSessionTransferCompensation(ctx, compensation)
}

// DeleteSessionTransferCompensation acknowledges a reconciled transfer.
func (s *Service) DeleteSessionTransferCompensation(
	ctx context.Context,
	operationID, taskID, fromSessionID, toSessionID string,
) error {
	repo, ok := s.repo.(sessionTransferCompensationRepository)
	if !ok {
		return errors.New("session transfer compensation persistence unavailable")
	}
	return repo.DeleteSessionTransferCompensation(
		ctx, operationID, taskID, fromSessionID, toSessionID,
	)
}

// RenewSessionTransferCompensationLease keeps external transfer work owned
// until its queue mutation or recovery acknowledgement completes.
func (s *Service) RenewSessionTransferCompensationLease(
	ctx context.Context,
	compensation SessionTransferCompensation,
	ownerID string,
) error {
	repo, ok := s.repo.(sessionTransferCompensationRecoveryRepository)
	if !ok {
		return errors.New("session transfer compensation recovery unavailable")
	}
	return repo.renewSessionTransferCompensationLease(ctx, compensation, ownerID)
}

// ClaimSessionTransferCompensationRecovery claims an expired transfer
// compensation before its external attachment recovery starts.
func (s *Service) ClaimSessionTransferCompensationRecovery(
	ctx context.Context,
	compensation SessionTransferCompensation,
) (string, error) {
	repo, ok := s.repo.(sessionTransferCompensationRecoveryRepository)
	if !ok {
		return "", errors.New("session transfer compensation recovery unavailable")
	}
	return repo.claimSessionTransferCompensationRecovery(ctx, compensation)
}

// DeleteClaimedSessionTransferCompensation acknowledges recovered attachment
// state only while the caller owns the compensation recovery lease.

// MaintainSessionTransferCompensationLease renews an owner-scoped lease until
// the caller completes its external attachment operation. Renewal failure
// cancels the returned context and is returned by the stop function.
func (s *Service) MaintainSessionTransferCompensationLease(
	ctx context.Context,
	compensation SessionTransferCompensation,
	ownerID string,
) (context.Context, func() error) {
	return s.maintainSessionTransferCompensationLease(
		ctx, compensation, ownerID, nil, sessionTransferCompensationLeaseDuration/2,
	)
}

// MaintainSessionTransferCompensationLeaseWithCancel is the recovery variant
// that also cancels the owning operation when lease renewal fails.
func (s *Service) MaintainSessionTransferCompensationLeaseWithCancel(
	ctx context.Context,
	compensation SessionTransferCompensation,
	ownerID string,
	cancelOwner context.CancelFunc,
) (context.Context, func() error) {
	return s.maintainSessionTransferCompensationLease(
		ctx, compensation, ownerID, cancelOwner, sessionTransferCompensationLeaseDuration/2,
	)
}

func (s *Service) maintainSessionTransferCompensationLease(
	ctx context.Context,
	compensation SessionTransferCompensation,
	ownerID string,
	cancelOwner context.CancelFunc,
	renewInterval time.Duration,
) (context.Context, func() error) {
	operationCtx, cancel := context.WithCancel(ctx)
	stop := make(chan struct{})
	done := make(chan error, 1)
	var stopOnce sync.Once
	var stopErr error
	go func() {
		ticker := time.NewTicker(renewInterval)
		defer ticker.Stop()
		var err error
		for {
			select {
			case <-stop:
				done <- nil
				return
			case <-operationCtx.Done():
				done <- nil
				return
			case <-ticker.C:
				renewCtx, renewCancel := context.WithTimeout(
					operationCtx, sessionTransferCompensationLeaseDuration/3,
				)
				err = s.RenewSessionTransferCompensationLease(
					renewCtx, compensation, ownerID,
				)
				renewCancel()
				if err != nil {
					cancel()
					if cancelOwner != nil {
						cancelOwner()
					}
					done <- err
					return
				}
			}
		}
	}()
	return operationCtx, func() error {
		stopOnce.Do(func() {
			close(stop)
			stopErr = <-done
			cancel()
		})
		return stopErr
	}
}

func (s *Service) DeleteClaimedSessionTransferCompensation(
	ctx context.Context,
	compensation SessionTransferCompensation,
	ownerID string,
) error {
	repo, ok := s.repo.(sessionTransferCompensationRecoveryRepository)
	if !ok {
		return errors.New("session transfer compensation recovery unavailable")
	}
	return repo.deleteSessionTransferCompensationWithOwner(
		ctx,
		compensation.OperationID,
		ownerID,
		compensation.TaskID,
		compensation.FromSessionID,
		compensation.ToSessionID,
	)
}

// ListSessionTransferCompensations reloads interrupted transfers after restart.
func (s *Service) ListSessionTransferCompensations(
	ctx context.Context,
) ([]SessionTransferCompensation, error) {
	repo, ok := s.repo.(sessionTransferCompensationRepository)
	if !ok {
		return nil, errors.New("session transfer compensation persistence unavailable")
	}
	return repo.ListSessionTransferCompensations(ctx)
}

func (s *Service) validateSessionIdentity(ctx context.Context, identity QueueSessionIdentity) error {
	current, err := s.repo.ResolveSessionIdentity(ctx, identity.TaskID, identity.SessionID)
	if err != nil {
		return err
	}
	if current != identity {
		return ErrSessionIdentityMismatch
	}
	return nil
}

// TransferSessionIdentities moves queue state between exact live incarnations of the same task.
func (s *Service) TransferSessionIdentities(ctx context.Context, source, destination QueueSessionIdentity) error {
	if source.TaskID != destination.TaskID {
		return ErrSessionIdentityMismatch
	}
	if err := s.validateSessionIdentity(ctx, source); err != nil {
		return err
	}
	if err := s.validateSessionIdentity(ctx, destination); err != nil {
		return err
	}
	if err := s.repo.TransferSessionIdentities(ctx, source, destination); err != nil {
		s.logger.Error("transfer session failed",
			zap.String("from_session_id", source.SessionID),
			zap.String("to_session_id", destination.SessionID),
			zap.Error(err))
		return err
	}
	s.logger.Info("transferred queue between sessions",
		zap.String("from_session_id", source.SessionID),
		zap.String("to_session_id", destination.SessionID))
	return nil
}

// RestoreSession replaces a session's queue and pending move from a snapshot,
// preserving queued-message identity fields.
func (s *Service) RestoreSession(ctx context.Context, sessionID string, entries []QueuedMessage, pendingMove *PendingMove) error {
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		if err := s.repo.ReplaceSession(admittedCtx, sessionID, entries, pendingMove); err != nil {
			return err
		}
		s.invalidateEditLeasesLocked(sessionID)
		return nil
	})
	if err != nil {
		s.logger.Error("restore session queue failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return err
	}
	s.logger.Info("restored session queue",
		zap.String("session_id", sessionID),
		zap.Int("entries", len(entries)),
		zap.Bool("pending_move", pendingMove != nil))
	return nil
}

// RestoreSessionForIdentity replaces queue state only for the captured session incarnation.
func (s *Service) RestoreSessionForIdentity(ctx context.Context, identity QueueSessionIdentity, entries []QueuedMessage, pendingMove *PendingMove) error {
	if err := s.validateSessionIdentity(ctx, identity); err != nil {
		return err
	}
	if err := s.repo.ReplaceSessionForIdentity(ctx, identity, entries, pendingMove); err != nil {
		s.logger.Error("restore session queue failed",
			zap.String("session_id", identity.SessionID),
			zap.Error(err))
		return err
	}
	s.logger.Info("restored session queue",
		zap.String("session_id", identity.SessionID),
		zap.Int("entries", len(entries)),
		zap.Bool("pending_move", pendingMove != nil))
	return nil
}

// SetPendingMove records a pending move for a session (replaces any existing one).
// The move is applied by handleAgentReady when the agent's current turn completes.
func (s *Service) SetPendingMove(ctx context.Context, sessionID string, move *PendingMove) error {
	if err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		return s.repo.SetPendingMove(admittedCtx, sessionID, move)
	}); err != nil {
		s.logger.Error("set pending move failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return err
	}
	s.logger.Info("pending move recorded",
		zap.String("session_id", sessionID),
		zap.String("task_id", move.TaskID),
		zap.String("workflow_step_id", move.WorkflowStepID))
	return nil
}

// GetPendingMoveWithError retrieves the pending move for a session without
// removing it. It distinguishes an empty queue from a storage failure so
// callers that must preserve deferred workflow state can fail closed.
func (s *Service) GetPendingMoveWithError(ctx context.Context, sessionID string) (*PendingMove, bool, error) {
	var move *PendingMove
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		var err error
		move, err = s.repo.GetPendingMove(admittedCtx, sessionID)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	if move == nil {
		return nil, false, nil
	}
	return move, true, nil
}

// GetPendingMove retrieves the pending move for a session without removing it.
func (s *Service) GetPendingMove(ctx context.Context, sessionID string) (*PendingMove, bool) {
	move, exists, err := s.GetPendingMoveWithError(ctx, sessionID)
	if err != nil {
		s.logger.Error("get pending move failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, false
	}
	return move, exists
}

// TakePendingMove retrieves and removes the pending move for a session.
func (s *Service) TakePendingMove(ctx context.Context, sessionID string) (*PendingMove, bool) {
	var move *PendingMove
	err := s.WithSessionAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		var err error
		move, err = s.repo.TakePendingMove(admittedCtx, sessionID)
		return err
	})
	if err != nil {
		s.logger.Error("take pending move failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, false
	}
	if move == nil {
		return nil, false
	}
	return move, true
}
