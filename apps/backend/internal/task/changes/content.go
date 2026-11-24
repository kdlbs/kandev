package changes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	DefaultContentRetention      = 30 * 24 * time.Hour
	DefaultTaskContentBytes      = 128 << 20
	DefaultInstallContentBytes   = 1 << 30
	MaxHistoricalContentResponse = 16 << 20
	maxExportedFilesPerCheckout  = 20_000
	maxTurnExportContentBytes    = 32 << 20
	maxPreservationAttempts      = 6
	preservationRetryDelay       = 250 * time.Millisecond
	preservationAttemptBudget    = 90 * time.Second
	PreservationRecoveryWindow   = 3 * time.Minute
	MaxTaskHistoryArchiveBytes   = 256 << 20
)

type ContentRepository interface {
	StoreTurnChangeFiles(context.Context, string, []models.TurnChangeFileContent) error
	StoreTurnChangeFilesWithReceipt(context.Context, string, []models.TurnChangeFileContent) (models.TurnChangeContentStoreReceipt, error)
	ReadTurnChangeContent(context.Context, string, string, models.TurnChangeContentVariant) (*models.TurnChangeContentPayload, error)
	GetTurnFileContentSource(context.Context, string, string) (*models.TurnFileContentSource, error)
	SetTurnRepositoryContentStatus(context.Context, string, bool, models.TurnChangeReason) error
	UpdateTurnRepositoryPreservation(context.Context, string, models.TurnChangeContentSource, models.TurnChangePreservation, models.TurnChangeReason, int64, int64, string, bool) error
	SetTurnRepositoryPreservationDeadline(context.Context, string, time.Time) error
	TaskPreservationBytes(context.Context, string) (int64, error)
	MarkTurnChangeFilesGitBacked(context.Context, string) (bool, error)
	AcquireTurnChangeContentLease(context.Context, string, time.Duration) (*models.TurnChangeContentLease, error)
	ReleaseTurnChangeContentLease(context.Context, string) error
	ApplyTurnChangeRetention(context.Context, models.TurnChangeRetentionPolicy, time.Time) (models.TurnChangeRetentionResult, error)
}

type Exporter interface {
	ExportTurnCheckpoint(context.Context, turnchanges.ExportRequest) (*turnchanges.CheckpointExport, error)
}

// ArchiveClient is implemented by executors that can export the accepted
// object closure for backend-owned storage.
type ArchiveClient interface {
	ArchiveTurnCheckpoint(context.Context, turnchanges.ArchiveRequest) (*turnchanges.CheckpointArchive, error)
}

type ContentService struct {
	repository ContentRepository
	history    *GitHistoryStore
	now        func() time.Time
}

func NewContentService(repository ContentRepository, now func() time.Time) *ContentService {
	if now == nil {
		now = time.Now
	}
	return &ContentService{repository: repository, now: now}
}

// SetHistoryStore enables backend-owned Git reads and preservation.
func (s *ContentService) SetHistoryStore(store *GitHistoryStore) {
	if s != nil {
		s.history = store
	}
}

// HistoryEnabled reports whether backend-owned Git preservation is active.
func (s *ContentService) HistoryEnabled() bool {
	return s != nil && s.history.Enabled()
}

type ExportReceipt struct {
	FileCount   int64
	ExportBytes int64
	Complete    bool
	Pending     bool
	Reason      turnchanges.ReasonCode
}

// PreservationReceipt reports the bounded outcome of one preservation attempt
// sequence. A pending receipt is retryable while the original source lives.
type PreservationReceipt struct {
	Complete  bool
	Bytes     int64
	Reason    turnchanges.ReasonCode
	Retryable bool
	Attempts  int64
	// Receipt is the encoded archive receipt persisted with the interval.
	Receipt string
	// Deadline is the bounded recovery window for pending work.
	Deadline *time.Time
	// Source is the durable source selected by a successful attempt.
	Source models.TurnChangeContentSource
	// Cause is diagnostic only. It is never persisted or returned to clients.
	Cause error
}

func (s *ContentService) StoreSummaries(
	ctx context.Context,
	repositoryChangeID, checkoutID string,
	files []turnchanges.CheckpointFile,
) error {
	if s == nil || s.repository == nil || repositoryChangeID == "" || checkoutID == "" {
		return errors.New("turn change summary storage is not configured")
	}
	summaries := make([]models.TurnChangeFileContent, 0, len(files))
	for _, file := range files {
		modelFile := checkpointFileSummary(repositoryChangeID, checkoutID, file)
		summaries = append(summaries, models.TurnChangeFileContent{File: modelFile})
	}
	return s.repository.StoreTurnChangeFiles(ctx, repositoryChangeID, summaries)
}

// Preserve stores the accepted interval in backend-owned Git storage. It
// retries only temporary failures inside one bounded call.
func (s *ContentService) Preserve(
	ctx context.Context,
	client ArchiveClient,
	row models.TurnRepositoryChangeSet,
) (PreservationReceipt, error) {
	if s == nil || s.repository == nil || !s.history.Enabled() {
		return s.preserveLegacy(ctx, client, row)
	}
	started := time.Now()
	var last PreservationReceipt
	for attempt := int64(1); attempt <= maxPreservationAttempts; attempt++ {
		receipt, err := s.PreserveOnce(ctx, client, row)
		if err != nil || receipt.Complete || !receipt.Retryable {
			return receipt, err
		}
		last = receipt
		row.PreservationAttempts = receipt.Attempts
		if receipt.Deadline != nil {
			row.PreservationDeadline = receipt.Deadline
		}
		if time.Since(started) >= preservationAttemptBudget {
			break
		}
		if !waitPreservationRetry(ctx, preservationRetryDelay<<uint(attempt-1)) {
			break
		}
	}
	return last, nil
}

// PreserveOnce attempts the accepted pair exactly once. The durable retry
// owner uses it to resume pending work without recapturing history.
func (s *ContentService) PreserveOnce(
	ctx context.Context,
	client ArchiveClient,
	row models.TurnRepositoryChangeSet,
) (PreservationReceipt, error) {
	if s == nil || s.repository == nil || !s.history.Enabled() {
		return s.preserveLegacy(ctx, client, row)
	}
	if client == nil {
		return s.settlePreservationFailure(row, row.PreservationAttempts+1, errors.New("executor is unavailable"))
	}
	if row.SourceGitDir != "" && s.history.CommonSourceUsable(ctx, row.SourceGitDir, row) {
		return s.settleCommonSource(ctx, row, row.PreservationAttempts+1)
	}
	attempts := row.PreservationAttempts + 1
	request := turnchanges.ArchiveRequest{
		ChangeSetID: row.TurnChangeSetID, CheckoutID: row.CheckoutID, Repo: row.RepositorySubpath,
		HashAlgorithm: row.HashAlgorithm, StartCommitOID: row.StartCommitOID, StartTreeOID: row.StartTreeOID,
		EndCommitOID: row.EndCommitOID, EndTreeOID: row.EndTreeOID,
		KnownObjects: s.history.SourceObjects(ctx, row.RepositoryID, row.CheckoutID),
	}
	archive, err := client.ArchiveTurnCheckpoint(ctx, request)
	if err == nil {
		err = validateArchiveForRow(archive, row)
	}
	if err == nil {
		taskBytes, budgetErr := s.repository.TaskPreservationBytes(ctx, row.TurnChangeSetID)
		if budgetErr == nil && taskBytes+int64(len(archive.Pack)) > MaxTaskHistoryArchiveBytes {
			return s.settlePreservationFailure(row, attempts, ErrHistoryContentUnavailable)
		}
	}
	if err == nil {
		receipt, importErr := s.history.ImportWithCapacity(ctx, row.RepositoryID, row.CheckoutID, archive)
		if importErr == nil {
			return s.settlePreservationSuccess(row, receipt, attempts)
		}
		err = importErr
	}
	if !isRetryablePreservationError(err) {
		return s.settlePreservationFailure(row, attempts, err)
	}
	return s.settlePreservationFailure(row, attempts, err)
}

func (s *ContentService) settleCommonSource(
	ctx context.Context,
	row models.TurnRepositoryChangeSet,
	attempts int64,
) (PreservationReceipt, error) {
	writeCtx, cancel := turnChangePersistenceContext()
	defer cancel()
	complete, err := s.repository.MarkTurnChangeFilesGitBacked(writeCtx, row.ID)
	if err != nil {
		return PreservationReceipt{}, err
	}
	reason := models.TurnChangeReason("")
	if !complete {
		reason = models.TurnChangeReasonContentUnavailable
	}
	if err := s.repository.UpdateTurnRepositoryPreservation(writeCtx, row.ID,
		models.TurnChangeSourceGitCommon, models.TurnChangePreservationReady, reason,
		attempts, 0, "", complete); err != nil {
		return PreservationReceipt{}, err
	}
	return PreservationReceipt{Complete: complete, Attempts: attempts, Reason: reason, Source: models.TurnChangeSourceGitCommon}, nil
}

func (s *ContentService) preserveLegacy(
	ctx context.Context,
	client ArchiveClient,
	row models.TurnRepositoryChangeSet,
) (PreservationReceipt, error) {
	exporter, ok := client.(Exporter)
	if !ok {
		return PreservationReceipt{Reason: turnchanges.ReasonUnsupportedExecutor}, nil
	}
	receipt, err := s.ExportAndStore(ctx, exporter, turnchanges.ExportRequest{
		ChangeSetID: row.TurnChangeSetID, CheckoutID: row.CheckoutID, Repo: row.RepositorySubpath,
		HashAlgorithm: row.HashAlgorithm, StartCommitOID: row.StartCommitOID, StartTreeOID: row.StartTreeOID,
		EndCommitOID: row.EndCommitOID, EndTreeOID: row.EndTreeOID,
	}, row.ID)
	if err != nil {
		return PreservationReceipt{Retryable: isRetryablePreservationError(err)}, nil
	}
	return PreservationReceipt{Complete: receipt.Complete, Bytes: receipt.ExportBytes, Reason: receipt.Reason}, nil
}

func waitPreservationRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func isRetryablePreservationError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrHistoryContentUnavailable) {
		return false
	}
	if errors.Is(err, ErrHistoryReadRetryable) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var clientErr interface{ CaptureReason() turnchanges.ReasonCode }
	if errors.As(err, &clientErr) {
		switch clientErr.CaptureReason() {
		case turnchanges.ReasonSizeLimit, turnchanges.ReasonEntryLimit, turnchanges.ReasonUnsafeGitState,
			turnchanges.ReasonInvalidPathEncoding, turnchanges.ReasonNoGitRepository, turnchanges.ReasonUnsupportedExecutor:
			return false
		case turnchanges.ReasonCheckoutUnavailable, turnchanges.ReasonCaptureFailed, turnchanges.ReasonContentUnavailable, "":
			return true
		}
	}
	return true
}

func (s *ContentService) settlePreservationSuccess(
	row models.TurnRepositoryChangeSet,
	receipt ArchiveReceipt,
	attempts int64,
) (PreservationReceipt, error) {
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return PreservationReceipt{}, err
	}
	writeCtx, cancel := turnChangePersistenceContext()
	defer cancel()
	complete, err := s.repository.MarkTurnChangeFilesGitBacked(writeCtx, row.ID)
	if err != nil {
		return PreservationReceipt{}, err
	}
	reason := models.TurnChangeReason("")
	if !complete {
		reason = models.TurnChangeReasonContentUnavailable
	}
	if err := s.repository.UpdateTurnRepositoryPreservation(writeCtx, row.ID,
		models.TurnChangeSourceGitArchive, models.TurnChangePreservationReady, reason,
		attempts, receipt.StoredBytes, string(encoded), complete); err != nil {
		return PreservationReceipt{}, err
	}
	return PreservationReceipt{
		Complete: complete, Bytes: receipt.StoredBytes, Attempts: attempts,
		Receipt: string(encoded), Reason: reason, Source: models.TurnChangeSourceGitArchive,
	}, nil
}

func (s *ContentService) settlePreservationFailure(
	row models.TurnRepositoryChangeSet,
	attempts int64,
	cause error,
) (PreservationReceipt, error) {
	reason := preservationFailureReason(cause)
	state := models.TurnChangePreservationUnavailable
	retryable := isRetryablePreservationError(cause)
	if retryable {
		state = models.TurnChangePreservationPending
	}
	writeCtx, cancel := turnChangePersistenceContext()
	defer cancel()
	updateErr := s.repository.UpdateTurnRepositoryPreservation(writeCtx, row.ID,
		models.TurnChangeSourceGitArchive, state, reason, attempts, 0, "", false)
	deadline := row.PreservationDeadline
	if retryable && deadline == nil {
		next := s.now().UTC().Add(PreservationRecoveryWindow)
		deadline = &next
		if err := s.repository.SetTurnRepositoryPreservationDeadline(writeCtx, row.ID, next); err != nil {
			updateErr = errors.Join(updateErr, err)
		}
	}
	receipt := PreservationReceipt{Reason: reason, Retryable: retryable, Attempts: attempts, Cause: cause, Deadline: deadline}
	if updateErr != nil && !errors.Is(updateErr, repoerrors.ErrTurnChangeRelationship) {
		return receipt, updateErr
	}
	return receipt, nil
}

func preservationFailureReason(err error) models.TurnChangeReason {
	if err == nil {
		return models.TurnChangeReasonContentUnavailable
	}
	var clientErr interface{ CaptureReason() turnchanges.ReasonCode }
	if errors.As(err, &clientErr) && clientErr.CaptureReason() != "" {
		return clientErr.CaptureReason()
	}
	if errors.Is(err, ErrHistoryContentUnavailable) {
		return models.TurnChangeReasonContentUnavailable
	}
	return models.TurnChangeReasonContentUnavailable
}

func (s *ContentService) ExportAndStore(ctx context.Context, exporter Exporter, request turnchanges.ExportRequest, repositoryChangeID string) (ExportReceipt, error) {
	if s == nil || s.repository == nil || exporter == nil {
		return ExportReceipt{}, errors.New("turn change content service is not configured")
	}
	if !validTurnChangeExportRequest(request, repositoryChangeID) {
		return ExportReceipt{}, errors.New("turn change content export requires change-set, repository-change, and checkout identities")
	}
	export, err := exporter.ExportTurnCheckpoint(ctx, request)
	if err != nil {
		return ExportReceipt{}, err
	}
	if !matchesTurnChangeExportEndpoints(export, request) {
		return ExportReceipt{}, errors.New("turn change executor returned a mismatched export endpoint pair")
	}
	if !withinTurnChangeExportBounds(export) {
		return ExportReceipt{}, errors.New("turn change executor returned an export outside accepted bounds")
	}
	files, receipt := prepareTurnChangeExportFiles(export, request.CheckoutID, repositoryChangeID)
	storeCtx, storeCancel := turnChangePersistenceContext()
	stored, err := s.repository.StoreTurnChangeFilesWithReceipt(storeCtx, repositoryChangeID, files)
	storeCancel()
	if err != nil {
		return ExportReceipt{}, err
	}
	receipt.ExportBytes = stored.StoredBytes
	receipt.Complete = receipt.Complete && stored.Complete
	receipt.Pending = stored.Pending
	if receipt.Reason == "" {
		receipt.Reason = stored.Reason
	}
	if err := persistEmptyTurnChangeExportStatus(s.repository, repositoryChangeID, files, receipt, export.Reason); err != nil {
		return ExportReceipt{}, err
	}
	if !receipt.Complete && receipt.Reason == "" {
		receipt.Reason = turnchanges.ReasonContentUnavailable
	}
	return receipt, nil
}

func validTurnChangeExportRequest(request turnchanges.ExportRequest, repositoryChangeID string) bool {
	return request.ChangeSetID != "" && repositoryChangeID != "" && request.CheckoutID != ""
}

func matchesTurnChangeExportEndpoints(export *turnchanges.CheckpointExport, request turnchanges.ExportRequest) bool {
	return export != nil && export.ChangeSetID == request.ChangeSetID && export.CheckoutID == request.CheckoutID &&
		export.HashAlgorithm == request.HashAlgorithm && export.StartCommitOID == request.StartCommitOID &&
		export.StartTreeOID == request.StartTreeOID && export.EndCommitOID == request.EndCommitOID &&
		export.EndTreeOID == request.EndTreeOID
}

func withinTurnChangeExportBounds(export *turnchanges.CheckpointExport) bool {
	return len(export.Files) <= maxExportedFilesPerCheckout && export.ExportBytes >= 0 && export.ExportBytes <= maxTurnExportContentBytes
}

func prepareTurnChangeExportFiles(
	export *turnchanges.CheckpointExport,
	checkoutID, repositoryChangeID string,
) ([]models.TurnChangeFileContent, ExportReceipt) {
	files := make([]models.TurnChangeFileContent, 0, len(export.Files))
	receipt := ExportReceipt{FileCount: int64(len(export.Files)), Complete: export.Complete, Reason: export.Reason}
	for _, exported := range export.Files {
		file := checkpointFileSummary(repositoryChangeID, checkoutID, exported.File)
		file.ContentAvailability, file.ContentReason = exported.ContentAvailability, exported.Reason
		file.ContentTruncated, file.CanonicalContentBytes = exported.Truncated, exported.CanonicalBytes
		invalidPath := !utf8.Valid(file.PathBytes) || len(file.OldPathBytes) > 0 && !utf8.Valid(file.OldPathBytes)
		if !utf8.Valid(file.PathBytes) {
			receipt.Complete = false
		}
		if invalidPath {
			file.ContentAvailability = models.TurnChangeAvailabilityUnavailable
			file.ContentReason = models.TurnChangeReasonInvalidPathEncoding
		}
		if !file.ContentAvailability.Valid() {
			file.ContentAvailability, file.ContentReason = models.TurnChangeAvailabilityUnavailable, models.TurnChangeReasonContentUnavailable
		}
		if file.ContentAvailability != models.TurnChangeAvailabilityReady && file.ContentReason == "" {
			file.ContentReason = models.TurnChangeReasonContentUnavailable
		}
		if exported.ContentAvailability != turnchanges.Ready || exported.Truncated {
			receipt.Complete = false
		}
		files = append(files, models.TurnChangeFileContent{
			File:           file,
			CanonicalPatch: copyNullableBytes(exported.CanonicalPatch),
			FilteredPatch:  copyNullableBytes(exported.FilteredPatch),
			OldRendering:   copyNullableBytes(exported.OldRendering),
			NewRendering:   copyNullableBytes(exported.NewRendering),
		})
	}
	return files, receipt
}

func persistEmptyTurnChangeExportStatus(
	repository ContentRepository,
	repositoryChangeID string,
	files []models.TurnChangeFileContent,
	receipt ExportReceipt,
	reason turnchanges.ReasonCode,
) error {
	if len(files) != 0 || receipt.Complete {
		return nil
	}
	if reason == "" {
		reason = models.TurnChangeReasonContentUnavailable
	}
	statusCtx, statusCancel := turnChangePersistenceContext()
	defer statusCancel()
	return repository.SetTurnRepositoryContentStatus(statusCtx, repositoryChangeID, false, reason)
}

func checkpointFileSummary(repositoryChangeID, checkoutID string, file turnchanges.CheckpointFile) models.TurnFileChange {
	modelFile := models.TurnFileChange{
		RepositoryChangeID: repositoryChangeID, CheckoutID: checkoutID,
		PathBytes: append([]byte(nil), file.PathBytes...),
		Kind:      file.Kind, OldBlobOID: file.OldOID, NewBlobOID: file.NewOID,
		OldMode: file.OldMode, NewMode: file.NewMode, Submodule: file.Submodule, Binary: file.Binary,
		AddedLines: file.Added, DeletedLines: file.Deleted,
		ContentAvailability: models.TurnChangeAvailabilityPending,
	}
	if utf8.Valid(modelFile.PathBytes) {
		modelFile.Path = string(modelFile.PathBytes)
	} else {
		modelFile.Path = "\uFFFD"
		modelFile.ContentReason = models.TurnChangeReasonInvalidPathEncoding
	}
	if len(file.OldPathBytes) > 0 {
		modelFile.OldPathBytes = append([]byte(nil), file.OldPathBytes...)
		if utf8.Valid(modelFile.OldPathBytes) {
			oldPath := string(modelFile.OldPathBytes)
			modelFile.OldPath = &oldPath
		} else {
			modelFile.ContentReason = models.TurnChangeReasonInvalidPathEncoding
		}
	}
	if file.Reason != "" {
		modelFile.ContentReason = file.Reason
	}
	return modelFile
}

func copyNullableBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append(make([]byte, 0, len(value)), value...)
}

func (s *ContentService) Read(ctx context.Context, changeSetID, fileChangeID string, variant models.TurnChangeContentVariant) (*models.TurnChangeContentPayload, error) {
	if s == nil || s.repository == nil {
		return nil, errors.New("turn change content service is not configured")
	}
	lease, err := s.repository.AcquireTurnChangeContentLease(ctx, changeSetID, time.Minute)
	if err != nil {
		return nil, err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.repository.ReleaseTurnChangeContentLease(releaseCtx, lease.ID)
	}()
	source, err := s.repository.GetTurnFileContentSource(ctx, changeSetID, fileChangeID)
	if err != nil {
		return nil, err
	}
	payload, err := s.readSource(ctx, source, variant)
	if err != nil {
		return nil, err
	}
	if payload == nil || len(payload.Content) > MaxHistoricalContentResponse {
		return nil, fmt.Errorf("historical content exceeds %d-byte response limit", MaxHistoricalContentResponse)
	}
	return payload, nil
}

func (s *ContentService) readSource(
	ctx context.Context,
	source *models.TurnFileContentSource,
	variant models.TurnChangeContentVariant,
) (*models.TurnChangeContentPayload, error) {
	if source == nil || source.ChangeSet == nil || source.Repository == nil || source.File == nil {
		return nil, repoerrors.ErrTurnChangeContentNotFound
	}
	if source.ChangeSet.Availability == models.TurnChangeAvailabilityExpired ||
		source.File.ContentAvailability == models.TurnChangeAvailabilityExpired {
		return nil, repoerrors.ErrTurnChangeContentNotFound
	}
	if source.Repository.ContentSource == models.TurnChangeSourceGitCommon {
		return s.readCommon(ctx, source, variant)
	}
	if source.Repository.ContentSource == models.TurnChangeSourceGitArchive {
		return s.readArchived(ctx, source, variant)
	}
	return s.repository.ReadTurnChangeContent(ctx, source.ChangeSet.ID, source.File.ID, variant)
}

func (s *ContentService) readCommon(
	ctx context.Context,
	source *models.TurnFileContentSource,
	variant models.TurnChangeContentVariant,
) (*models.TurnChangeContentPayload, error) {
	if !s.history.Enabled() {
		return nil, fmt.Errorf("%w: common Git history source is unavailable", ErrHistoryReadRetryable)
	}
	commonDir := source.Repository.SourceGitDir
	if !s.history.CommonSourceUsable(ctx, commonDir, *source.Repository) {
		// A restored backup may have moved a common interval into backend
		// storage; read the verified archive fallback before reporting loss.
		commonDir = s.history.repositoryPath(s.history.sourceKey(source.Repository.RepositoryID, source.Repository.CheckoutID))
		if !s.history.CommonSourceUsable(ctx, commonDir, *source.Repository) {
			return nil, fmt.Errorf("%w: common Git history source is unavailable", ErrHistoryContentUnavailable)
		}
	}
	archive := turnchanges.CheckpointArchive{
		HashAlgorithm: source.Repository.HashAlgorithm,
		StartTreeOID:  source.Repository.StartTreeOID, EndTreeOID: source.Repository.EndTreeOID,
	}
	file := HistoricalFile{
		PathBytes: source.File.PathBytes, OldPathBytes: source.File.OldPathBytes,
		OldBlobOID: source.File.OldBlobOID, NewBlobOID: source.File.NewBlobOID,
	}
	payload := &models.TurnChangeContentPayload{FileChangeID: source.File.ID, Variant: variant}
	var err error
	switch variant {
	case models.TurnChangeContentCanonicalPatch:
		payload.Content, err = s.history.readPatchAt(ctx, commonDir, archive, file, false)
	case models.TurnChangeContentFilteredPatch:
		payload.Content, err = s.history.readPatchAt(ctx, commonDir, archive, file, true)
	case models.TurnChangeContentOldRendering:
		payload.Content, err = s.history.ReadBlobAt(ctx, commonDir, source.File.OldBlobOID, source.Repository.HashAlgorithm)
	case models.TurnChangeContentNewRendering:
		payload.Content, err = s.history.ReadBlobAt(ctx, commonDir, source.File.NewBlobOID, source.Repository.HashAlgorithm)
	default:
		return nil, repoerrors.ErrTurnChangeContentNotFound
	}
	if err != nil {
		return nil, err
	}
	if payload.Content == nil {
		payload.Content = []byte{}
	}
	return payload, nil
}

func (s *ContentService) readArchived(
	ctx context.Context,
	source *models.TurnFileContentSource,
	variant models.TurnChangeContentVariant,
) (*models.TurnChangeContentPayload, error) {
	if !s.history.Enabled() {
		return nil, fmt.Errorf("%w: Git history store is not configured", ErrHistoryReadRetryable)
	}
	switch source.Repository.PreservationState {
	case models.TurnChangePreservationPending:
		return nil, fmt.Errorf("%w: historical content is still being prepared", ErrHistoryReadRetryable)
	case models.TurnChangePreservationReady:
	case models.TurnChangePreservationUnavailable:
		return nil, repoerrors.ErrTurnChangeContentNotFound
	default:
		if source.File.ContentAvailability == models.TurnChangeAvailabilityReady {
			return s.repository.ReadTurnChangeContent(ctx, source.ChangeSet.ID, source.File.ID, variant)
		}
		return nil, repoerrors.ErrTurnChangeContentNotFound
	}
	receipt, err := decodeArchiveReceipt(source.Repository.PreservationReceipt)
	if err != nil {
		return nil, fmt.Errorf("%w: retained archive receipt is invalid", ErrHistoryContentUnavailable)
	}
	archive := turnchanges.CheckpointArchive{
		HashAlgorithm: receipt.HashAlgorithm, StartCommitOID: receipt.StartCommitOID, EndCommitOID: receipt.EndCommitOID,
		StartTreeOID: source.Repository.StartTreeOID, EndTreeOID: source.Repository.EndTreeOID,
	}
	payload := &models.TurnChangeContentPayload{FileChangeID: source.File.ID, Variant: variant}
	file := HistoricalFile{
		PathBytes: source.File.PathBytes, OldPathBytes: source.File.OldPathBytes,
		OldBlobOID: source.File.OldBlobOID, NewBlobOID: source.File.NewBlobOID,
	}
	switch variant {
	case models.TurnChangeContentCanonicalPatch:
		payload.Content, err = s.history.ReadPatch(ctx, receipt, archive, file, false)
	case models.TurnChangeContentFilteredPatch:
		payload.Content, err = s.history.ReadPatch(ctx, receipt, archive, file, true)
	case models.TurnChangeContentOldRendering:
		payload.Content, err = s.history.ReadBlob(ctx, receipt, source.File.OldBlobOID, receipt.HashAlgorithm)
	case models.TurnChangeContentNewRendering:
		payload.Content, err = s.history.ReadBlob(ctx, receipt, source.File.NewBlobOID, receipt.HashAlgorithm)
	default:
		return nil, repoerrors.ErrTurnChangeContentNotFound
	}
	if err != nil {
		return nil, err
	}
	if payload.Content == nil {
		payload.Content = []byte{}
	}
	return payload, nil
}

func decodeArchiveReceipt(raw string) (ArchiveReceipt, error) {
	if raw == "" {
		return ArchiveReceipt{}, errors.New("empty archive receipt")
	}
	var receipt ArchiveReceipt
	if err := json.Unmarshal([]byte(raw), &receipt); err != nil {
		return ArchiveReceipt{}, err
	}
	if receipt.SourceID == "" || receipt.StartCommitOID == "" || receipt.EndCommitOID == "" ||
		receipt.HashAlgorithm == "" {
		return ArchiveReceipt{}, errors.New("incomplete archive receipt")
	}
	return receipt, nil
}

func (s *ContentService) Retain(ctx context.Context) (models.TurnChangeRetentionResult, error) {
	if s == nil || s.repository == nil {
		return models.TurnChangeRetentionResult{}, errors.New("turn change content service is not configured")
	}
	return s.repository.ApplyTurnChangeRetention(ctx, models.TurnChangeRetentionPolicy{
		RetainFor: DefaultContentRetention, TaskBytes: DefaultTaskContentBytes,
		InstallationBytes: DefaultInstallContentBytes,
	}, s.now().UTC())
}

// validateArchiveForRow binds an executor response to the exact accepted
// interval before any archive or SQL acknowledgement.
func validateArchiveForRow(archive *turnchanges.CheckpointArchive, row models.TurnRepositoryChangeSet) error {
	if archive == nil || !archive.Complete {
		return fmt.Errorf("%w: archive response is incomplete", ErrHistoryContentUnavailable)
	}
	if archive.ChangeSetID != row.TurnChangeSetID || archive.CheckoutID != row.CheckoutID ||
		archive.HashAlgorithm != row.HashAlgorithm ||
		archive.StartCommitOID != row.StartCommitOID || archive.StartTreeOID != row.StartTreeOID ||
		archive.EndCommitOID != row.EndCommitOID || archive.EndTreeOID != row.EndTreeOID {
		return fmt.Errorf("%w: archive response does not match the accepted interval", ErrHistoryContentUnavailable)
	}
	required := map[string]struct{}{
		row.StartCommitOID: {}, row.StartTreeOID: {}, row.EndCommitOID: {}, row.EndTreeOID: {},
	}
	for _, oid := range archive.Objects {
		delete(required, oid)
	}
	if len(required) != 0 {
		return fmt.Errorf("%w: archive omits accepted endpoint objects", ErrHistoryContentUnavailable)
	}
	return nil
}

// SettlePreservationExpired records the explicit final result for work whose
// bounded recovery window ended without surviving source evidence.
func (s *ContentService) SettlePreservationExpired(ctx context.Context, row models.TurnRepositoryChangeSet) error {
	if s == nil || s.repository == nil {
		return errors.New("turn change content service is not configured")
	}
	writeCtx, cancel := turnChangePersistenceContext()
	defer cancel()
	err := s.repository.UpdateTurnRepositoryPreservation(writeCtx, row.ID,
		models.TurnChangeSourceGitArchive, models.TurnChangePreservationUnavailable,
		models.TurnChangeReasonContentUnavailable, row.PreservationAttempts, row.PreservationBytes, "", false)
	if err != nil && !errors.Is(err, repoerrors.ErrTurnChangeRelationship) {
		return err
	}
	return nil
}
