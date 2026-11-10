package changes

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func coordinatorAdmission() Admission {
	return Admission{
		TaskID: "task-1", SessionID: "session-1", TaskEnvironmentID: "env-1", TurnID: "turn-1",
		ExecutionID: "execution-1", StartupAttemptID: "startup-1", PromptGeneration: 4,
		Checkouts: []Checkout{{
			ID: "checkout-1", EnvironmentRepoID: "env-repo-1", TaskRepositoryID: "task-repo-1",
			RepositoryID: "repo-1", DisplayName: "frontend", RepositorySubpath: "frontend", RepositorySubpathKnown: true,
		}},
	}
}

type coordinatorTurnReader struct{}

func (coordinatorTurnReader) GetTurn(context.Context, string) (*models.Turn, error) {
	return &models.Turn{
		ID: "turn-1", TaskID: "task-1", TaskSessionID: "session-1",
		ExecutionProfileID: "profile-routed", RouteGeneration: 9,
		Metadata: map[string]interface{}{models.TurnMetaKeyTurnChangeActorUserID: "user-initiator"},
	}, nil
}

type coordinatorTurnReaderMap map[string]*models.Turn

func (r coordinatorTurnReaderMap) GetTurn(_ context.Context, turnID string) (*models.Turn, error) {
	turn := r[turnID]
	if turn == nil {
		return nil, repoerrors.ErrTurnChangeSetNotFound
	}
	copy := *turn
	return &copy, nil
}

type coordinatorPolicy struct {
	result turnchanges.CapturePolicy
	err    error
	userID string
	calls  int64
}

func (p *coordinatorPolicy) ResolveTurnChangedFilesCapturePolicy(ctx context.Context) (turnchanges.CapturePolicy, error) {
	p.calls++
	identity, _ := authn.IdentityFromContext(ctx)
	p.userID = identity.UserID
	return p.result, p.err
}

type coordinatorCheckpointClient struct {
	scopes              []string
	scopeCalls          int
	startCalls          int
	endCalls            int
	compareCalls        int
	exportCalls         int
	checkpointRequests  []turnchanges.CheckpointRequest
	comparisonFiles     []turnchanges.CheckpointFile
	comparisonRequests  []turnchanges.CompareRequest
	provenance          bool
	exportErr           error
	deleteRequests      []turnchanges.CheckpointDeleteRequest
	deleteErr           error
	waitCaptureBoundary turnchanges.CheckpointBoundary
	waitCompare         bool
	waitExport          bool
	capturedAt          *time.Time
	exportPages         []turnchanges.CheckpointExport
	exportErrors        []error
	exportPageIndex     int
	exportPageBuilder   func(turnchanges.ExportRequest) *turnchanges.CheckpointExport
}

func (c *coordinatorCheckpointClient) TurnCheckpointRepositoryScopes(context.Context) ([]string, error) {
	c.scopeCalls++
	if c.scopes == nil {
		return []string{"frontend"}, nil
	}
	return c.scopes, nil
}

func (c *coordinatorCheckpointClient) CaptureTurnCheckpoint(ctx context.Context, request turnchanges.CheckpointRequest) (*turnchanges.CheckpointResult, error) {
	c.checkpointRequests = append(c.checkpointRequests, request)
	if request.Boundary == turnchanges.CheckpointStart {
		c.startCalls++
	} else {
		c.endCalls++
	}
	if request.Boundary == c.waitCaptureBoundary {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	capturedAt := time.Now().UTC()
	if c.capturedAt != nil {
		capturedAt = c.capturedAt.UTC()
	}
	result := &turnchanges.CheckpointResult{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID, Boundary: request.Boundary,
		CommitOID: "commit-" + string(request.Boundary), TreeOID: "tree-" + string(request.Boundary),
		HashAlgorithm: "sha1", ReachabilityRef: "refs/kandev/turn-changes/test", CapturedAt: capturedAt,
	}
	if c.provenance {
		result.Provenance = &turnchanges.Provenance{
			Version: turnchanges.ProvenanceVersion2, ChangeSetID: request.ChangeSetID,
			CheckoutID: request.CheckoutID, Boundary: request.Boundary, HashAlgorithm: "sha1",
			HeadCommitOID: "commit-" + string(request.Boundary), HeadRef: "refs/heads/main",
		}
	}
	return result, nil
}

func (c *coordinatorCheckpointClient) DeleteTurnCheckpoint(_ context.Context, request turnchanges.CheckpointDeleteRequest) error {
	c.deleteRequests = append(c.deleteRequests, request)
	return c.deleteErr
}

func (c *coordinatorCheckpointClient) CompareTurnCheckpoints(ctx context.Context, request turnchanges.CompareRequest) (*turnchanges.CheckpointComparison, error) {
	c.compareCalls++
	c.comparisonRequests = append(c.comparisonRequests, request)
	if c.waitCompare {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	added, deleted := int64(2), int64(1)
	comparison := &turnchanges.CheckpointComparison{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID,
		HashAlgorithm: request.HashAlgorithm, StartCommitOID: request.StartCommitOID, StartTreeOID: request.StartTreeOID,
		EndCommitOID: request.EndCommitOID, EndTreeOID: request.EndTreeOID, FileCount: 1,
		Files: c.comparisonFiles, AddedLines: &added, DeletedLines: &deleted, Complete: true,
	}
	if request.ComparisonVersion == turnchanges.ComparisonVersion2 {
		comparison.ComparisonVersion = turnchanges.ComparisonVersion2
		comparison.ComparisonBasis = turnchanges.ComparisonDirect
		comparison.EffectiveStartCommitOID = request.StartCommitOID
		comparison.EffectiveStartTreeOID = request.StartTreeOID
		comparison.EffectiveEndCommitOID = request.EndCommitOID
		comparison.EffectiveEndTreeOID = request.EndTreeOID
		comparison.ComparisonRef = "refs/kandev/turn-comparisons/test"
	}
	return comparison, nil
}

func (c *coordinatorCheckpointClient) ExportTurnCheckpoint(ctx context.Context, request turnchanges.ExportRequest) (*turnchanges.CheckpointExport, error) {
	index := c.exportPageIndex
	c.exportCalls++
	c.exportPageIndex++
	if c.waitExport {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if index < len(c.exportErrors) && c.exportErrors[index] != nil {
		return nil, fmt.Errorf("export checkpoint: %w", c.exportErrors[index])
	}
	if c.exportPageBuilder != nil {
		return c.exportPageBuilder(request), nil
	}
	if index < len(c.exportPages) {
		page := c.exportPages[index]
		return &page, nil
	}
	if c.exportErr != nil {
		return nil, fmt.Errorf("export checkpoint: %w", c.exportErr)
	}
	return &turnchanges.CheckpointExport{
		ChangeSetID: request.ChangeSetID, CheckoutID: request.CheckoutID,
		HashAlgorithm: request.HashAlgorithm, StartCommitOID: request.StartCommitOID, StartTreeOID: request.StartTreeOID,
		EndCommitOID: request.EndCommitOID, EndTreeOID: request.EndTreeOID, Complete: true, ExportBytes: 10,
	}, nil
}

type coordinatorStore struct {
	repository.TurnChangesRepository
	changeSets                map[string]*models.TurnChangeSet
	repositoryRows            map[string][]*models.TurnRepositoryChangeSet
	files                     []models.TurnChangeFileContent
	contentStatuses           map[string]bool
	finalizeFailuresRemaining int
	unfinishedListErr         error
	writerIntervalErr         error
	completedAtByTurn         map[string]*time.Time
	forceExportClaimLost      bool
	exportStoreDelay          time.Duration
	leaseReleaseDelay         time.Duration
	leaseReleaseDeadline      time.Time
	overlapCASConflicts       int
	singleListCalls           int
	batchListCalls            int
}

func newCoordinatorStore() *coordinatorStore {
	return &coordinatorStore{
		changeSets: map[string]*models.TurnChangeSet{}, repositoryRows: map[string][]*models.TurnRepositoryChangeSet{},
		contentStatuses: map[string]bool{},
	}
}

func (s *coordinatorStore) CreateTurnChangeSet(_ context.Context, changeSet *models.TurnChangeSet) error {
	if _, exists := s.changeSets[changeSet.ID]; exists {
		return repoerrors.ErrTurnChangeSetIdentityConflict
	}
	copy := *changeSet
	copy.Revision = 1
	if copy.TurnOrdinal == 0 {
		copy.TurnOrdinal = int64(len(s.changeSets) + 1)
	}
	s.changeSets[changeSet.ID] = &copy
	return nil
}

func (s *coordinatorStore) GetTurnChangeSet(_ context.Context, taskID, sessionID, id string) (*models.TurnChangeSet, error) {
	changeSet, ok := s.changeSets[id]
	if !ok || changeSet.TaskID != taskID || changeSet.TaskSessionID != sessionID {
		return nil, repoerrors.ErrTurnChangeSetNotFound
	}
	copy := *changeSet
	return &copy, nil
}

func (s *coordinatorStore) AcceptTurnChangeSetStart(_ context.Context, id string, revision int64, rows []models.TurnRepositoryChangeSet) (bool, error) {
	changeSet := s.changeSets[id]
	if changeSet == nil || changeSet.Revision != revision || changeSet.StartAccepted {
		return false, nil
	}
	changeSet.StartAccepted = true
	changeSet.Revision++
	s.repositoryRows[id] = make([]*models.TurnRepositoryChangeSet, 0, len(rows))
	for i := range rows {
		row := rows[i]
		copy := row
		s.repositoryRows[id] = append(s.repositoryRows[id], &copy)
	}
	return true, nil
}

func (s *coordinatorStore) AdvanceTurnChangeSetPromptGeneration(_ context.Context, id, executionID, startupAttemptID, taskEnvironmentID string, revision, generation int64) (bool, error) {
	changeSet := s.changeSets[id]
	if changeSet == nil || changeSet.Revision != revision || !changeSet.StartAccepted || changeSet.TerminalAt != nil ||
		changeSet.RuntimeExecutionID != executionID || changeSet.StartupAttemptID != startupAttemptID ||
		changeSet.TaskEnvironmentID != taskEnvironmentID || generation <= changeSet.PromptGeneration {
		return false, nil
	}
	changeSet.PromptGeneration = generation
	changeSet.Revision++
	return true, nil
}

func (s *coordinatorStore) ListTurnRepositoryChanges(_ context.Context, id string) ([]*models.TurnRepositoryChangeSet, error) {
	s.singleListCalls++
	rows := s.repositoryRows[id]
	result := make([]*models.TurnRepositoryChangeSet, 0, len(rows))
	for _, row := range rows {
		copy := *row
		result = append(result, &copy)
	}
	return result, nil
}

func (s *coordinatorStore) ListTurnRepositoryChangesForSets(_ context.Context, ids []string) (map[string][]*models.TurnRepositoryChangeSet, error) {
	s.batchListCalls++
	result := make(map[string][]*models.TurnRepositoryChangeSet, len(ids))
	for _, id := range ids {
		for _, row := range s.repositoryRows[id] {
			copy := *row
			result[id] = append(result[id], &copy)
		}
	}
	return result, nil
}

func (s *coordinatorStore) ListUnfinishedTurnChangeSets(_ context.Context, _ int) ([]*models.TurnChangeSet, error) {
	if s.unfinishedListErr != nil {
		return nil, s.unfinishedListErr
	}
	var result []*models.TurnChangeSet
	for _, changeSet := range s.changeSets {
		if changeSet.TerminalAt == nil {
			copy := *changeSet
			result = append(result, &copy)
		}
	}
	return result, nil
}

func (s *coordinatorStore) ListTurnChangeSets(_ context.Context, taskID, sessionID string, offset, limit int) ([]*models.TurnChangeSet, int, error) {
	var result []*models.TurnChangeSet
	for _, changeSet := range s.changeSets {
		if changeSet.TaskID != taskID || changeSet.TaskSessionID != sessionID {
			continue
		}
		copy := *changeSet
		result = append(result, &copy)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TurnOrdinal > result[j].TurnOrdinal })
	total := len(result)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []*models.TurnChangeSet{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return result[offset:end], total, nil
}

func (s *coordinatorStore) ClaimTurnChangeSetTerminal(_ context.Context, id string, revision int64, claim models.TurnChangeSetTerminalClaim) (bool, error) {
	changeSet := s.changeSets[id]
	if changeSet == nil || changeSet.TerminalAt != nil || changeSet.Revision != revision ||
		changeSet.RuntimeExecutionID != claim.ExecutionID || changeSet.StartupAttemptID != claim.StartupAttemptID ||
		changeSet.PromptGeneration != claim.PromptGeneration || changeSet.TaskEnvironmentID != claim.TaskEnvironmentID {
		return false, nil
	}
	if changeSet.TerminalCaptureStartedAt != nil {
		return changeSet.TerminalCaptureExecutionID == claim.ExecutionID &&
			changeSet.TerminalCaptureStartupAttemptID == claim.StartupAttemptID &&
			changeSet.TerminalCapturePromptGeneration == claim.PromptGeneration &&
			changeSet.TerminalCaptureEnvironmentID == claim.TaskEnvironmentID, nil
	}
	changeSet.Revision++
	changeSet.TerminalCaptureStartedAt = &claim.At
	changeSet.TerminalCaptureExecutionID = claim.ExecutionID
	changeSet.TerminalCaptureStartupAttemptID = claim.StartupAttemptID
	changeSet.TerminalCapturePromptGeneration = claim.PromptGeneration
	changeSet.TerminalCaptureEnvironmentID = claim.TaskEnvironmentID
	changeSet.TerminalCaptureOutcome = claim.Outcome
	changeSet.TerminalCaptureFinalMessageID = claim.FinalAssistantMessageID
	return true, nil
}

func (s *coordinatorStore) AcceptTurnRepositoryEnd(_ context.Context, changeSetID, repositoryChangeID, startCommitOID, startTreeOID string, end models.TurnRepositoryChangeSet) (bool, error) {
	for _, row := range s.repositoryRows[changeSetID] {
		if row.ID != repositoryChangeID || row.StartCommitOID != startCommitOID || row.StartTreeOID != startTreeOID {
			continue
		}
		if row.EndCommitOID != "" && (row.EndCommitOID != end.EndCommitOID || row.EndTreeOID != end.EndTreeOID) {
			return false, nil
		}
		row.EndCommitOID, row.EndTreeOID, row.EndCapturedAt, row.EndReachabilityRef = end.EndCommitOID, end.EndTreeOID, end.EndCapturedAt, end.EndReachabilityRef
		row.EndProvenanceJSON = end.EndProvenanceJSON
		return true, nil
	}
	return false, nil
}

func (s *coordinatorStore) UpdateTurnChangeSetOverlaps(_ context.Context, changeSetID string, overlaps []models.TurnChangeOverlap) error {
	if changeSet := s.changeSets[changeSetID]; changeSet != nil {
		changeSet.OverlapIntervals = append([]models.TurnChangeOverlap(nil), overlaps...)
		changeSet.Revision++
	}
	return nil
}

func (s *coordinatorStore) MarkTurnRepositoryCheckpointRefsCleaned(_ context.Context, repositoryChangeID string) error {
	for _, rows := range s.repositoryRows {
		for _, row := range rows {
			if row.ID == repositoryChangeID {
				row.CleanupPending = false
			}
		}
	}
	return nil
}

func (s *coordinatorStore) FinalizeTurnChangeSet(_ context.Context, id string, revision int64, finalization models.TurnChangeSetFinalization) (bool, error) {
	if s.finalizeFailuresRemaining > 0 {
		s.finalizeFailuresRemaining--
		return false, context.DeadlineExceeded
	}
	changeSet := s.changeSets[id]
	if changeSet == nil || changeSet.Revision != revision || changeSet.TerminalAt != nil {
		return false, nil
	}
	changeSet.Revision++
	changeSet.Availability = finalization.Availability
	changeSet.Reason = finalization.Reason
	changeSet.Complete = finalization.Complete
	changeSet.SummaryComplete = finalization.SummaryComplete
	changeSet.ContentComplete = finalization.ContentComplete
	changeSet.TerminalAt = &finalization.TerminalAt
	changeSet.TerminalOutcome = finalization.TerminalOutcome
	changeSet.FinalAssistantMessageID = finalization.FinalAssistantMessageID
	changeSet.FileCount = finalization.FileCount
	changeSet.AddedLines = finalization.AddedLines
	changeSet.DeletedLines = finalization.DeletedLines
	changeSet.BinaryFileCount = finalization.BinaryFileCount
	changeSet.UnknownCountFileCount = finalization.UnknownCountFileCount
	changeSet.RepositoryCount = finalization.RepositoryCount
	changeSet.ContentBytes = finalization.ContentBytes
	for index := range finalization.Repositories {
		repositoryChange := finalization.Repositories[index]
		for rowIndex, row := range s.repositoryRows[id] {
			if row.ID != repositoryChange.ID {
				continue
			}
			s.repositoryRows[id][rowIndex] = &repositoryChange
		}
	}
	return true, nil
}

func (s *coordinatorStore) StoreTurnChangeFiles(_ context.Context, _ string, files []models.TurnChangeFileContent) error {
	s.files = append(s.files, files...)
	return nil
}

func (s *coordinatorStore) StoreTurnChangeFilesWithReceipt(ctx context.Context, id string, files []models.TurnChangeFileContent) (models.TurnChangeContentStoreReceipt, error) {
	if err := s.StoreTurnChangeFiles(ctx, id, files); err != nil {
		return models.TurnChangeContentStoreReceipt{}, err
	}
	return models.TurnChangeContentStoreReceipt{StoredBytes: 10, Complete: true}, nil
}

func (s *coordinatorStore) SetTurnRepositoryContentStatus(_ context.Context, id string, complete bool, _ models.TurnChangeReason) error {
	s.contentStatuses[id] = complete
	return nil
}

func (s *coordinatorStore) ReadTurnChangeContent(context.Context, string, string, models.TurnChangeContentVariant) (*models.TurnChangeContentPayload, error) {
	return nil, nil
}

func (s *coordinatorStore) AcquireTurnChangeContentLease(_ context.Context, id string, _ time.Duration) (*models.TurnChangeContentLease, error) {
	return &models.TurnChangeContentLease{ID: "lease", ChangeSetID: id}, nil
}

func (s *coordinatorStore) ReleaseTurnChangeContentLease(ctx context.Context, _ string) error {
	if deadline, ok := ctx.Deadline(); ok {
		s.leaseReleaseDeadline = deadline
	}
	if s.leaseReleaseDelay <= 0 {
		return nil
	}
	timer := time.NewTimer(s.leaseReleaseDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *coordinatorStore) ApplyTurnChangeRetention(context.Context, models.TurnChangeRetentionPolicy, time.Time) (models.TurnChangeRetentionResult, error) {
	return models.TurnChangeRetentionResult{}, nil
}

func (s *coordinatorStore) ListTurnRepositoryPreservationRows(context.Context, []models.TurnChangePreservation, int) ([]*models.TurnRepositoryChangeSet, error) {
	return nil, nil
}
