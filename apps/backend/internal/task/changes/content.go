package changes

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	DefaultContentRetention      = 30 * 24 * time.Hour
	DefaultTaskContentBytes      = 128 << 20
	DefaultInstallContentBytes   = 1 << 30
	MaxHistoricalContentResponse = 16 << 20
	maxExportedFilesPerCheckout  = 20_000
	maxTurnExportContentBytes    = 32 << 20
)

type ContentRepository interface {
	StoreTurnChangeFiles(context.Context, string, []models.TurnChangeFileContent) error
	StoreTurnChangeFilesWithReceipt(context.Context, string, []models.TurnChangeFileContent) (models.TurnChangeContentStoreReceipt, error)
	ReadTurnChangeContent(context.Context, string, string, models.TurnChangeContentVariant) (*models.TurnChangeContentPayload, error)
	SetTurnRepositoryContentStatus(context.Context, string, bool, models.TurnChangeReason) error
	AcquireTurnChangeContentLease(context.Context, string, time.Duration) (*models.TurnChangeContentLease, error)
	ReleaseTurnChangeContentLease(context.Context, string) error
	ApplyTurnChangeRetention(context.Context, models.TurnChangeRetentionPolicy, time.Time) (models.TurnChangeRetentionResult, error)
}

type Exporter interface {
	ExportTurnCheckpoint(context.Context, turnchanges.ExportRequest) (*turnchanges.CheckpointExport, error)
}

type ContentService struct {
	repository ContentRepository
	now        func() time.Time
}

func NewContentService(repository ContentRepository, now func() time.Time) *ContentService {
	if now == nil {
		now = time.Now
	}
	return &ContentService{repository: repository, now: now}
}

type ExportReceipt struct {
	FileCount   int64
	ExportBytes int64
	Complete    bool
	Reason      turnchanges.ReasonCode
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
		ContentAvailability: models.TurnChangeAvailabilityUnavailable,
		ContentReason:       models.TurnChangeReasonContentUnavailable,
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
	payload, err := s.repository.ReadTurnChangeContent(ctx, changeSetID, fileChangeID, variant)
	if err != nil {
		return nil, err
	}
	if payload == nil || len(payload.Content) > MaxHistoricalContentResponse {
		return nil, fmt.Errorf("historical content exceeds %d-byte response limit", MaxHistoricalContentResponse)
	}
	return payload, nil
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
