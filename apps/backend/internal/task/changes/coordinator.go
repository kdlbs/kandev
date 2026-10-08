package changes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"go.uber.org/zap"
)

type TurnReader interface {
	GetTurn(context.Context, string) (*models.Turn, error)
}

type PolicyResolver interface {
	ResolveTurnChangedFilesCapturePolicy(context.Context) (turnchanges.CapturePolicy, error)
}

type CheckpointClient interface {
	TurnCheckpointRepositoryScopes(context.Context) ([]string, error)
	CaptureTurnCheckpoint(context.Context, turnchanges.CheckpointRequest) (*turnchanges.CheckpointResult, error)
	DeleteTurnCheckpoint(context.Context, turnchanges.CheckpointDeleteRequest) error
	CompareTurnCheckpoints(context.Context, turnchanges.CompareRequest) (*turnchanges.CheckpointComparison, error)
	ExportTurnCheckpoint(context.Context, turnchanges.ExportRequest) (*turnchanges.CheckpointExport, error)
}

type Checkout struct {
	ID                     string
	EnvironmentRepoID      string
	TaskRepositoryID       string
	RepositoryID           string
	WorktreeID             string
	DisplayName            string
	RepositorySubpath      string
	RepositorySubpathKnown bool
}

type Admission struct {
	TaskID             string
	SessionID          string
	TaskEnvironmentID  string
	TurnID             string
	ExecutionID        string
	StartupAttemptID   string
	PromptGeneration   uint64
	ExecutionProfileID string
	RouteGeneration    int64
	Checkouts          []Checkout
}

type Terminal struct {
	Admission
	At                      time.Time
	Outcome                 string
	FinalAssistantMessageID string
}

type Coordinator struct {
	repository repository.TurnChangesRepository
	turns      TurnReader
	policies   PolicyResolver
	content    *ContentService
	now        func() time.Time
	overlapMu  sync.Mutex
	logger     *zap.Logger
}

const turnChangePersistenceTimeout = 3 * time.Second

func turnChangePersistenceContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), turnChangePersistenceTimeout)
}

func NewCoordinator(repo repository.TurnChangesRepository, turns TurnReader, policies PolicyResolver, content *ContentService, now func() time.Time) *Coordinator {
	if now == nil {
		now = time.Now
	}
	return &Coordinator{repository: repo, turns: turns, policies: policies, content: content, now: now, logger: zap.NewNop()}
}

// SetLogger configures structured logging for coordinator diagnostics.
func (c *Coordinator) SetLogger(logger *zap.Logger) {
	if logger == nil {
		c.logger = zap.NewNop()
		return
	}
	c.logger = logger
}

// Admit persists the exact effective policy and, when enabled, a fresh
// executor-owned start endpoint before the provider receives the prompt.
func (c *Coordinator) Admit(ctx context.Context, admission Admission, client CheckpointClient) error {
	if !c.configuredForAdmission() {
		return errors.New("turn change coordinator is not configured")
	}
	turn, admission, err := c.prepareTurnChangeAdmission(ctx, admission, client)
	if err != nil || turn == nil {
		return err
	}
	changeSetID := turnChangeSetID(turn.ID)
	existing, getErr := c.repository.GetTurnChangeSet(ctx, admission.TaskID, admission.SessionID, changeSetID)
	if getErr == nil {
		return c.admitExistingTurnChangeSet(ctx, admission, client, changeSetID, existing)
	}
	if !errors.Is(getErr, repoerrors.ErrTurnChangeSetNotFound) {
		return fmt.Errorf("read turn change set before admission: %w", getErr)
	}
	return c.createAdmittedTurnChangeSet(ctx, admission, turn, client)
}

func (c *Coordinator) configuredForAdmission() bool {
	return c != nil && c.repository != nil && c.turns != nil && c.policies != nil
}

func (c *Coordinator) prepareTurnChangeAdmission(
	ctx context.Context,
	admission Admission,
	client CheckpointClient,
) (*models.Turn, Admission, error) {
	if admission.TaskID == "" || admission.SessionID == "" || admission.TaskEnvironmentID == "" || admission.TurnID == "" || admission.PromptGeneration == 0 {
		return nil, admission, nil
	}
	if client != nil {
		// A failed cleanup remains a durable retry intent and is retried when the
		// task executor reconnects.
		_ = c.drainSessionCheckpointRefs(admission.TaskID, admission.SessionID, client)
	}
	turn, err := c.turns.GetTurn(ctx, admission.TurnID)
	if err != nil {
		return nil, admission, fmt.Errorf("load admitted turn: %w", err)
	}
	if !admissionMatchesStoredTurn(admission, turn) {
		return nil, admission, errors.New("admitted turn ownership does not match execution")
	}
	if admission.ExecutionProfileID == "" {
		admission.ExecutionProfileID = turn.ExecutionProfileID
	}
	if admission.RouteGeneration == 0 {
		admission.RouteGeneration = turn.RouteGeneration
	}
	return turn, admission, nil
}

func admissionMatchesStoredTurn(admission Admission, turn *models.Turn) bool {
	return turn != nil && turn.ID == admission.TurnID && turn.TaskID == admission.TaskID && turn.TaskSessionID == admission.SessionID
}

func (c *Coordinator) admitExistingTurnChangeSet(
	ctx context.Context,
	admission Admission,
	client CheckpointClient,
	changeSetID string,
	existing *models.TurnChangeSet,
) error {
	if existing.TerminalAt != nil || !existing.CaptureEnabled {
		return nil
	}
	if !sameTurnChangeOwnership(existing, admission) {
		return c.settleAdmissionBoundaryLoss(ctx, admission, existing, client)
	}
	if !existing.StartAccepted {
		return c.settleAdmissionBoundaryLoss(ctx, admission, existing, client)
	}
	if !c.sameTurnChangeCheckouts(ctx, client, existing, admission.Checkouts) {
		return c.settleAdmissionBoundaryLoss(ctx, admission, existing, client)
	}
	return c.advanceTurnChangeContinuation(ctx, admission, changeSetID, existing)
}

func (c *Coordinator) settleAdmissionBoundaryLoss(
	ctx context.Context,
	admission Admission,
	existing *models.TurnChangeSet,
	client CheckpointClient,
) error {
	return c.finalizeUnavailable(ctx, existing, Terminal{Admission: admission, At: c.now().UTC(), Outcome: "boundary_lost"}, models.TurnChangeReasonCheckoutUnavailable, client)
}

func (c *Coordinator) advanceTurnChangeContinuation(
	ctx context.Context,
	admission Admission,
	changeSetID string,
	existing *models.TurnChangeSet,
) error {
	generation := int64(admission.PromptGeneration)
	if generation <= existing.PromptGeneration {
		return nil
	}
	advanced, err := c.repository.AdvanceTurnChangeSetPromptGeneration(
		ctx, existing.ID, admission.ExecutionID, admission.StartupAttemptID, admission.TaskEnvironmentID,
		existing.Revision, generation,
	)
	if err != nil {
		return fmt.Errorf("advance admitted turn generation: %w", err)
	}
	if advanced {
		return nil
	}
	latest, readErr := c.repository.GetTurnChangeSet(ctx, admission.TaskID, admission.SessionID, changeSetID)
	if readErr == nil && sameTurnChangeOwnership(latest, admission) && latest.PromptGeneration >= generation {
		return nil
	}
	return errors.Join(errors.New("turn change continuation did not own the accepted lineage"), readErr)
}

func (c *Coordinator) createAdmittedTurnChangeSet(ctx context.Context, admission Admission, turn *models.Turn, client CheckpointClient) error {
	changeSetID := turnChangeSetID(turn.ID)
	policyCtx := policyContextForTurn(ctx, turn.Metadata)
	policy, policyErr := c.policies.ResolveTurnChangedFilesCapturePolicy(policyCtx)
	changeSet := &models.TurnChangeSet{
		ID: changeSetID, TaskID: admission.TaskID, TaskSessionID: admission.SessionID,
		TurnID: admission.TurnID, TaskEnvironmentID: admission.TaskEnvironmentID,
		Revision:           1,
		RuntimeExecutionID: admission.ExecutionID, StartupAttemptID: admission.StartupAttemptID,
		PromptGeneration: int64(admission.PromptGeneration), ExecutionProfileID: turn.ExecutionProfileID,
		RouteGeneration: turn.RouteGeneration, CaptureEnabled: policyErr == nil && policy.Enabled,
		SettingsUserID: policy.SettingsUserID, ActorID: turnChangeActorID(turn.Metadata),
		SettingsRevision: policy.Revision, ResolutionKind: policy.ResolutionKind,
		Availability: models.TurnChangeAvailabilityPending,
	}
	if policyErr != nil {
		changeSet.Availability = models.TurnChangeAvailabilityUnavailable
		changeSet.Reason = models.TurnChangeReasonPolicyReadFailed
		changeSet.ResolutionKind = turnchanges.PolicyReadFailed
		changeSet.CaptureEnabled = false
	} else if !policy.Enabled {
		changeSet.Availability = models.TurnChangeAvailabilityUnavailable
		changeSet.Reason = models.TurnChangeReasonCaptureDisabled
		changeSet.CaptureEnabled = false
	}
	if err := c.repository.CreateTurnChangeSet(ctx, changeSet); err != nil {
		if errors.Is(err, repoerrors.ErrTurnChangeSetIdentityConflict) {
			return c.handleTurnChangeIdentityConflict(ctx, admission, client, changeSetID)
		}
		return fmt.Errorf("persist turn change policy: %w", err)
	}
	if !changeSet.CaptureEnabled {
		return nil
	}
	return c.captureStart(ctx, admission, client, changeSet)
}

func (c *Coordinator) handleTurnChangeIdentityConflict(
	ctx context.Context,
	admission Admission,
	client CheckpointClient,
	changeSetID string,
) error {
	existing, err := c.repository.GetTurnChangeSet(ctx, admission.TaskID, admission.SessionID, changeSetID)
	if err != nil {
		return fmt.Errorf("read existing turn change set: %w", err)
	}
	if existing.StartAccepted || existing.TerminalAt != nil || !existing.CaptureEnabled {
		return nil
	}
	return c.captureStart(ctx, admission, client, existing)
}

func (c *Coordinator) captureStart(ctx context.Context, admission Admission, client CheckpointClient, changeSet *models.TurnChangeSet) error {
	if client == nil {
		return errors.Join(errors.New("turn change checkpoint client is unavailable"),
			c.acceptUnavailableStart(ctx, admission, changeSet, nil, turnchanges.ReasonUnsupportedExecutor))
	}
	scopes, err := client.TurnCheckpointRepositoryScopes(ctx)
	if err != nil {
		return errors.Join(fmt.Errorf("load turn change repository scopes: %w", err),
			c.acceptUnavailableStart(ctx, admission, changeSet, nil, turnchanges.ReasonCheckoutUnavailable))
	}
	scopeSet := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scopeSet[scope] = struct{}{}
	}
	repositories := make([]models.TurnRepositoryChangeSet, 0, len(admission.Checkouts))
	var captureFailures []error
	for _, checkout := range admission.Checkouts {
		row := c.repositoryStartRow(changeSet.ID, checkout)
		scope, ok := checkoutScope(checkout, len(admission.Checkouts), scopeSet)
		if !ok {
			row.Availability = models.TurnChangeAvailabilityUnavailable
			row.Reason = models.TurnChangeReasonCheckoutUnavailable
			captureFailures = append(captureFailures, fmt.Errorf("checkout %q is absent from registered repository scopes", checkout.ID))
			repositories = append(repositories, row)
			continue
		}
		row.RepositorySubpath = scope
		captured, captureErr := client.CaptureTurnCheckpoint(ctx, turnchanges.CheckpointRequest{
			ChangeSetID: changeSet.ID, CheckoutID: checkout.ID, Repo: scope, Boundary: turnchanges.CheckpointStart,
		})
		if captureErr != nil || !validCheckpoint(captured, changeSet.ID, checkout.ID, turnchanges.CheckpointStart) {
			row.Availability = models.TurnChangeAvailabilityUnavailable
			row.Reason = checkpointReason(captureErr, turnchanges.ReasonCaptureFailed)
			if captureErr == nil {
				captureErr = errors.New("checkpoint response did not match the admitted start boundary")
			}
			captureFailures = append(captureFailures, fmt.Errorf("capture start for checkout %q: %w", checkout.ID, captureErr))
			repositories = append(repositories, row)
			continue
		}
		row.StartCommitOID = captured.CommitOID
		row.StartTreeOID = captured.TreeOID
		row.HashAlgorithm = captured.HashAlgorithm
		row.StartReachabilityRef = captured.ReachabilityRef
		row.CleanupPending = captured.ReachabilityRef != ""
		capturedAt := captured.CapturedAt.UTC()
		row.StartCapturedAt = &capturedAt
		row.Availability = models.TurnChangeAvailabilityPending
		repositories = append(repositories, row)
	}
	accepted, err := c.repository.AcceptTurnChangeSetStart(ctx, changeSet.ID, changeSet.Revision, repositories)
	if err != nil {
		return fmt.Errorf("persist turn change start endpoints: %w", err)
	}
	if !accepted {
		return nil
	}
	overlapErr := c.recordAdmittedOverlaps(changeSet.ID)
	return errors.Join(errors.Join(captureFailures...), overlapErr)
}

func (c *Coordinator) acceptUnavailableStart(ctx context.Context, admission Admission, changeSet *models.TurnChangeSet, rows []models.TurnRepositoryChangeSet, reason turnchanges.ReasonCode) error {
	if len(rows) == 0 {
		for _, checkout := range admission.Checkouts {
			row := c.repositoryStartRow(changeSet.ID, checkout)
			row.Availability = models.TurnChangeAvailabilityUnavailable
			row.Reason = reason
			rows = append(rows, row)
		}
	}
	accepted, err := c.repository.AcceptTurnChangeSetStart(ctx, changeSet.ID, changeSet.Revision, rows)
	if err != nil {
		return fmt.Errorf("persist unavailable turn change starts: %w", err)
	}
	if !accepted {
		return nil
	}
	return nil
}

func validComparison(comparison *turnchanges.CheckpointComparison, changeSetID string, row models.TurnRepositoryChangeSet) bool {
	return comparison != nil && comparison.ChangeSetID == changeSetID && comparison.CheckoutID == row.CheckoutID &&
		comparison.HashAlgorithm == row.HashAlgorithm && comparison.StartCommitOID == row.StartCommitOID &&
		comparison.StartTreeOID == row.StartTreeOID && comparison.EndCommitOID == row.EndCommitOID &&
		comparison.EndTreeOID == row.EndTreeOID
}

func terminalChangeClaimMatches(changeSet *models.TurnChangeSet, claim models.TurnChangeSetTerminalClaim) bool {
	return changeSet != nil && changeSet.TerminalCaptureExecutionID == claim.ExecutionID &&
		changeSet.TerminalCaptureStartupAttemptID == claim.StartupAttemptID &&
		changeSet.TerminalCapturePromptGeneration == claim.PromptGeneration &&
		changeSet.TerminalCaptureEnvironmentID == claim.TaskEnvironmentID
}

func (c *Coordinator) finalizeUnavailable(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	terminal Terminal,
	reason models.TurnChangeReason,
	clients ...CheckpointClient,
) error {
	if terminal.At.IsZero() {
		terminal.At = c.now().UTC()
	}
	readCtx, readCancel := turnChangePersistenceContext()
	repositories, listErr := c.repository.ListTurnRepositoryChanges(readCtx, changeSet.ID)
	readCancel()
	if listErr != nil && !errors.Is(listErr, repoerrors.ErrTurnChangeSetNotFound) {
		return fmt.Errorf("list unavailable turn repository checkpoints: %w", listErr)
	}
	finalizedRepositories := make([]models.TurnRepositoryChangeSet, 0, len(repositories))
	for _, row := range repositories {
		if row == nil {
			continue
		}
		row.Availability = models.TurnChangeAvailabilityUnavailable
		row.Reason = reason
		row.ContentComplete = false
		finalizedRepositories = append(finalizedRepositories, *row)
	}
	finalization := models.TurnChangeSetFinalization{
		Availability: models.TurnChangeAvailabilityUnavailable, Reason: reason,
		TerminalAt: terminal.At, TerminalOutcome: terminal.Outcome,
		FinalAssistantMessageID: terminal.FinalAssistantMessageID,
		RetainUntil:             timePtr(terminal.At.Add(DefaultContentRetention)),
		SummaryComplete:         false, ContentComplete: false, Complete: false,
		RepositoryCount: int64(len(finalizedRepositories)), Repositories: finalizedRepositories,
		OverlapIntervals: changeSet.OverlapIntervals,
	}
	if finalization.Reason == "" {
		finalization.Reason = models.TurnChangeReasonCheckoutUnavailable
	}
	if changeSet.TerminalCaptureStartedAt != nil {
		finalization.TerminalAt = *changeSet.TerminalCaptureStartedAt
		finalization.TerminalOutcome = changeSet.TerminalCaptureOutcome
		finalization.FinalAssistantMessageID = changeSet.TerminalCaptureFinalMessageID
	}
	writeCtx, writeCancel := turnChangePersistenceContext()
	finalized, err := c.repository.FinalizeTurnChangeSet(writeCtx, changeSet.ID, changeSet.Revision, finalization)
	writeCancel()
	if err != nil {
		return fmt.Errorf("finalize unavailable turn change set: %w", err)
	}
	if !finalized {
		checkCtx, checkCancel := turnChangePersistenceContext()
		latest, readErr := c.repository.GetTurnChangeSet(checkCtx, changeSet.TaskID, changeSet.TaskSessionID, changeSet.ID)
		checkCancel()
		if readErr != nil {
			return fmt.Errorf("verify unavailable turn change finalization: %w", readErr)
		}
		if latest.TerminalAt == nil {
			return errors.New("unavailable turn change finalization lost its ownership revision")
		}
	}
	if len(clients) > 0 {
		_ = c.cleanupRepositoryCheckpointRefs(finalizedRepositories, clients[0])
	}
	return nil
}

func (c *Coordinator) cleanupRepositoryCheckpointRefs(rows []models.TurnRepositoryChangeSet, client CheckpointClient) error {
	if client == nil {
		return nil
	}
	var failures []error
	for _, row := range rows {
		if !row.CleanupPending {
			continue
		}
		cleanupFailed := false
		for _, endpoint := range []struct {
			boundary turnchanges.CheckpointBoundary
			commit   string
			ref      string
		}{
			{boundary: turnchanges.CheckpointStart, commit: row.StartCommitOID, ref: row.StartReachabilityRef},
			{boundary: turnchanges.CheckpointEnd, commit: row.EndCommitOID, ref: row.EndReachabilityRef},
		} {
			if endpoint.ref == "" || endpoint.commit == "" {
				continue
			}
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := client.DeleteTurnCheckpoint(cleanupCtx, turnchanges.CheckpointDeleteRequest{
				ChangeSetID: row.TurnChangeSetID, CheckoutID: row.CheckoutID, Repo: row.RepositorySubpath,
				Boundary: endpoint.boundary, CommitOID: endpoint.commit,
			})
			cleanupCancel()
			if err != nil {
				cleanupFailed = true
				failures = append(failures, fmt.Errorf("delete %s checkpoint for checkout %q: %w", endpoint.boundary, row.CheckoutID, err))
				break
			}
		}
		if cleanupFailed {
			continue
		}
		persistCtx, persistCancel := turnChangePersistenceContext()
		err := c.repository.MarkTurnRepositoryCheckpointRefsCleaned(persistCtx, row.ID)
		persistCancel()
		if err != nil {
			failures = append(failures, fmt.Errorf("persist checkpoint cleanup receipt for checkout %q: %w", row.CheckoutID, err))
		}
	}
	return errors.Join(failures...)
}

func (c *Coordinator) drainSessionCheckpointRefs(taskID, sessionID string, client CheckpointClient) error {
	if client == nil {
		return nil
	}
	var failures []error
	for offset := 0; ; {
		ctx, cancel := turnChangePersistenceContext()
		sets, total, err := c.repository.ListTurnChangeSets(ctx, taskID, sessionID, offset, 100)
		cancel()
		if err != nil {
			return errors.Join(append(failures, fmt.Errorf("list checkpoint cleanup intents: %w", err))...)
		}
		for _, set := range sets {
			if set == nil || set.TerminalAt == nil {
				continue
			}
			ctx, cancel := turnChangePersistenceContext()
			rows, listErr := c.repository.ListTurnRepositoryChanges(ctx, set.ID)
			cancel()
			if listErr != nil {
				failures = append(failures, listErr)
				continue
			}
			var values []models.TurnRepositoryChangeSet
			for _, row := range rows {
				if row != nil {
					values = append(values, *row)
				}
			}
			if cleanupErr := c.cleanupRepositoryCheckpointRefs(values, client); cleanupErr != nil {
				failures = append(failures, cleanupErr)
			}
		}
		offset += len(sets)
		if offset >= total || len(sets) == 0 {
			break
		}
	}
	return errors.Join(failures...)
}

func (c *Coordinator) recordAdmittedOverlaps(currentID string) error {
	c.overlapMu.Lock()
	defer c.overlapMu.Unlock()
	ctx, cancel := turnChangePersistenceContext()
	defer cancel()
	sets, err := c.repository.ListUnfinishedTurnChangeSets(ctx, 500)
	if err != nil {
		return err
	}
	byID := indexUnfinishedTurnChangeSets(sets)
	current := byID[currentID]
	if current == nil {
		return nil
	}
	rowsBySet, err := c.loadUnfinishedRepositoryRows(ctx, sets)
	if err != nil {
		return err
	}
	changed := map[string]bool{}
	for _, currentRow := range rowsBySet[currentID] {
		if currentRow == nil || currentRow.StartCapturedAt == nil {
			continue
		}
		recordOverlapsForCurrentRow(currentRow, sets, rowsBySet, current, currentID, changed)
	}
	for id := range changed {
		if err := c.repository.UpdateTurnChangeSetOverlaps(ctx, id, byID[id].OverlapIntervals); err != nil {
			return err
		}
	}
	return nil
}

func indexUnfinishedTurnChangeSets(sets []*models.TurnChangeSet) map[string]*models.TurnChangeSet {
	byID := make(map[string]*models.TurnChangeSet, len(sets))
	for _, set := range sets {
		if set != nil {
			byID[set.ID] = set
		}
	}
	return byID
}

func (c *Coordinator) loadUnfinishedRepositoryRows(
	ctx context.Context,
	sets []*models.TurnChangeSet,
) (map[string][]*models.TurnRepositoryChangeSet, error) {
	ids := make([]string, 0, len(sets))
	seen := make(map[string]struct{}, len(sets))
	for _, set := range sets {
		if set != nil && set.ID != "" {
			if _, exists := seen[set.ID]; exists {
				continue
			}
			seen[set.ID] = struct{}{}
			ids = append(ids, set.ID)
		}
	}
	return c.repository.ListTurnRepositoryChangesForSets(ctx, ids)
}

func recordOverlapsForCurrentRow(
	currentRow *models.TurnRepositoryChangeSet,
	sets []*models.TurnChangeSet,
	rowsBySet map[string][]*models.TurnRepositoryChangeSet,
	current *models.TurnChangeSet,
	currentID string,
	changed map[string]bool,
) {
	for _, otherSet := range sets {
		if otherSet == nil || otherSet.ID == currentID || otherSet.TerminalAt != nil {
			continue
		}
		for _, otherRow := range rowsBySet[otherSet.ID] {
			if otherRow == nil || otherRow.StartCapturedAt == nil || !sameTurnChangeCheckoutIdentity(*currentRow, *otherRow) {
				continue
			}
			recordTurnChangeOverlapPair(current, currentID, currentRow, otherSet, otherRow)
			changed[currentID], changed[otherSet.ID] = true, true
		}
	}
}

func recordTurnChangeOverlapPair(
	current *models.TurnChangeSet,
	currentID string,
	currentRow *models.TurnRepositoryChangeSet,
	otherSet *models.TurnChangeSet,
	otherRow *models.TurnRepositoryChangeSet,
) {
	startedAt := currentRow.StartCapturedAt.UTC()
	if otherRow.StartCapturedAt.After(startedAt) {
		startedAt = otherRow.StartCapturedAt.UTC()
	}
	current.OverlapIntervals = mergeTurnChangeOverlap(current.OverlapIntervals, models.TurnChangeOverlap{
		ChangeSetID: otherSet.ID, CheckoutID: otherRow.CheckoutID, StartedAt: startedAt,
	})
	otherSet.OverlapIntervals = mergeTurnChangeOverlap(otherSet.OverlapIntervals, models.TurnChangeOverlap{
		ChangeSetID: currentID, CheckoutID: currentRow.CheckoutID, StartedAt: startedAt,
	})
}

func (c *Coordinator) closeAdmittedOverlaps(currentID string, endedAt time.Time) ([]models.TurnChangeOverlap, error) {
	c.overlapMu.Lock()
	defer c.overlapMu.Unlock()
	ctx, cancel := turnChangePersistenceContext()
	defer cancel()
	sets, err := c.repository.ListUnfinishedTurnChangeSets(ctx, 500)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*models.TurnChangeSet, len(sets))
	for _, set := range sets {
		if set != nil {
			byID[set.ID] = set
		}
	}
	current := byID[currentID]
	if current == nil {
		return nil, nil
	}
	changed := map[string]bool{}
	for _, set := range sets {
		if set == nil {
			continue
		}
		for index := range set.OverlapIntervals {
			overlap := &set.OverlapIntervals[index]
			if overlap.EndedAt == nil && (set.ID == currentID || overlap.ChangeSetID == currentID) {
				at := endedAt.UTC()
				overlap.EndedAt = &at
				changed[set.ID] = true
			}
		}
	}
	for id := range changed {
		if err := c.repository.UpdateTurnChangeSetOverlaps(ctx, id, byID[id].OverlapIntervals); err != nil {
			return current.OverlapIntervals, err
		}
	}
	return current.OverlapIntervals, nil
}

func sameTurnChangeCheckoutIdentity(left, right models.TurnRepositoryChangeSet) bool {
	if left.WorktreeID != "" && right.WorktreeID != "" {
		return left.WorktreeID == right.WorktreeID
	}
	if left.TaskEnvironmentRepoID != "" && right.TaskEnvironmentRepoID != "" {
		return left.TaskEnvironmentRepoID == right.TaskEnvironmentRepoID
	}
	return left.CheckoutID != "" && left.CheckoutID == right.CheckoutID
}

func mergeTurnChangeOverlap(overlaps []models.TurnChangeOverlap, incoming models.TurnChangeOverlap) []models.TurnChangeOverlap {
	for _, current := range overlaps {
		if current.ChangeSetID == incoming.ChangeSetID && current.CheckoutID == incoming.CheckoutID {
			return overlaps
		}
	}
	return append(overlaps, incoming)
}

// ReconcileUnfinished settles rows left by a process restart. It never calls an
// executor: a new process cannot prove that the old checkout boundary survived.
func (c *Coordinator) ReconcileUnfinished(ctx context.Context) error {
	if c == nil || c.repository == nil {
		return errors.New("turn change coordinator is not configured")
	}
	readCtx, readCancel := turnChangePersistenceContext()
	changeSets, err := c.repository.ListUnfinishedTurnChangeSets(readCtx, 500)
	readCancel()
	if err != nil {
		return fmt.Errorf("list unfinished turn change sets: %w", err)
	}
	var failures []error
	for _, changeSet := range changeSets {
		if changeSet == nil || changeSet.TerminalAt != nil {
			continue
		}
		terminalAt := c.now().UTC()
		terminal := Terminal{Admission: Admission{
			TaskID: changeSet.TaskID, SessionID: changeSet.TaskSessionID,
			TaskEnvironmentID: changeSet.TaskEnvironmentID, TurnID: changeSet.TurnID,
			ExecutionID: changeSet.RuntimeExecutionID, StartupAttemptID: changeSet.StartupAttemptID,
			PromptGeneration: uint64(changeSet.PromptGeneration),
		}, At: terminalAt, Outcome: "restart_unavailable"}
		if changeSet.TerminalCaptureStartedAt != nil {
			terminal.At = *changeSet.TerminalCaptureStartedAt
			terminal.Outcome = changeSet.TerminalCaptureOutcome
			terminal.FinalAssistantMessageID = changeSet.TerminalCaptureFinalMessageID
		}
		if err := c.finalizeUnavailable(ctx, changeSet, terminal, models.TurnChangeReasonCheckoutUnavailable); err != nil {
			failures = append(failures, fmt.Errorf("reconcile turn change set %q: %w", changeSet.ID, err))
		}
	}
	return errors.Join(failures...)
}

func sameTurnChangeOwnership(changeSet *models.TurnChangeSet, admission Admission) bool {
	return changeSet != nil && changeSet.TaskID == admission.TaskID && changeSet.TaskSessionID == admission.SessionID &&
		changeSet.TurnID == admission.TurnID && changeSet.TaskEnvironmentID == admission.TaskEnvironmentID &&
		changeSet.RuntimeExecutionID == admission.ExecutionID && changeSet.StartupAttemptID == admission.StartupAttemptID
}

func sameTurnChangeTerminalOwner(changeSet *models.TurnChangeSet, admission Admission) bool {
	return sameTurnChangeOwnership(changeSet, admission) && changeSet.PromptGeneration == int64(admission.PromptGeneration)
}

func (c *Coordinator) sameTurnChangeCheckouts(ctx context.Context, client CheckpointClient, changeSet *models.TurnChangeSet, checkouts []Checkout) bool {
	if client == nil {
		return false
	}
	scopes, err := client.TurnCheckpointRepositoryScopes(ctx)
	if err != nil {
		return false
	}
	rows, err := c.repository.ListTurnRepositoryChanges(ctx, changeSet.ID)
	return err == nil && sameTurnChangeCheckoutsWithScopes(rows, checkouts, scopes)
}

func sameTurnChangeCheckoutsWithScopes(rows []*models.TurnRepositoryChangeSet, checkouts []Checkout, scopes []string) bool {
	if len(rows) != len(checkouts) {
		return false
	}
	scopeSet := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scopeSet[scope] = struct{}{}
	}
	checkoutByID := make(map[string]Checkout, len(checkouts))
	for _, checkout := range checkouts {
		checkoutByID[checkout.ID] = checkout
	}
	for _, row := range rows {
		if row == nil {
			return false
		}
		checkout, exists := checkoutByID[row.CheckoutID]
		if !exists || row.TaskEnvironmentRepoID != checkout.EnvironmentRepoID || row.TaskRepositoryID != checkout.TaskRepositoryID ||
			row.RepositoryID != checkout.RepositoryID || row.WorktreeID != checkout.WorktreeID {
			return false
		}
		scope, ok := checkoutScope(checkout, len(checkouts), scopeSet)
		if !ok || scope != row.RepositorySubpath {
			return false
		}
	}
	return true
}

func (c *Coordinator) repositoryStartRow(changeSetID string, checkout Checkout) models.TurnRepositoryChangeSet {
	return models.TurnRepositoryChangeSet{
		ID: repositoryChangeID(changeSetID, checkout.ID), TurnChangeSetID: changeSetID,
		CheckoutID: checkout.ID, TaskEnvironmentRepoID: checkout.EnvironmentRepoID,
		TaskRepositoryID: checkout.TaskRepositoryID, RepositoryID: checkout.RepositoryID,
		WorktreeID: checkout.WorktreeID, DisplayName: checkout.DisplayName,
		RepositorySubpath: checkout.RepositorySubpath,
	}
}

func checkoutScope(checkout Checkout, manifestCount int, scopes map[string]struct{}) (string, bool) {
	if manifestCount == 1 {
		if _, ok := scopes[""]; ok {
			return "", true
		}
	}
	if !checkout.RepositorySubpathKnown {
		if manifestCount != 1 {
			return "", false
		}
		if _, ok := scopes[""]; ok {
			return "", true
		}
		return "", false
	}
	if _, ok := scopes[checkout.RepositorySubpath]; !ok {
		return "", false
	}
	return checkout.RepositorySubpath, true
}

func validCheckpoint(result *turnchanges.CheckpointResult, changeSetID, checkoutID string, boundary turnchanges.CheckpointBoundary) bool {
	return result != nil && result.ChangeSetID == changeSetID && result.CheckoutID == checkoutID &&
		result.Boundary == boundary && result.CommitOID != "" && result.TreeOID != "" &&
		result.HashAlgorithm != "" && result.ReachabilityRef != ""
}

func checkpointReason(err error, fallback turnchanges.ReasonCode) turnchanges.ReasonCode {
	if err != nil {
		var reason interface{ CaptureReason() turnchanges.ReasonCode }
		if errors.As(err, &reason) && reason.CaptureReason().Valid() {
			return reason.CaptureReason()
		}
	}
	return fallback
}

func policyContextForTurn(ctx context.Context, metadata map[string]interface{}) context.Context {
	identity := authn.Identity{}
	if userID := turnChangeActorID(metadata); userID != "" {
		identity.UserID = userID
	} else if synthetic, _ := metadata[models.TurnMetaKeyTurnChangeSyntheticActor].(bool); synthetic {
		identity.Synthetic = true
	}
	return authn.WithIdentity(ctx, identity)
}

func turnChangeActorID(metadata map[string]interface{}) string {
	if metadata == nil {
		return ""
	}
	userID, _ := metadata[models.TurnMetaKeyTurnChangeActorUserID].(string)
	return userID
}

func turnChangeSetID(turnID string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("kandev:turn-change-set:"+turnID)).String()
}

func ChangeSetIDForTurn(turnID string) string {
	if turnID == "" {
		return ""
	}
	return turnChangeSetID(turnID)
}

func repositoryChangeID(changeSetID, checkoutID string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("kandev:turn-repository-change:"+changeSetID+":"+checkoutID)).String()
}

func timePtr(value time.Time) *time.Time { return &value }
