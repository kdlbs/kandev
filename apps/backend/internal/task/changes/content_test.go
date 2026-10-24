package changes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
)

func TestContentServiceExportsExactFileIdentityAndPayloadVariants(t *testing.T) {
	repository := &fakeContentRepository{}
	exporter := &fakeContentExporter{result: &turnchanges.CheckpointExport{
		ChangeSetID: "change-set", CheckoutID: "checkout", HashAlgorithm: "sha1",
		StartCommitOID: "start", StartTreeOID: "start-tree", EndCommitOID: "end", EndTreeOID: "end-tree",
		Complete: true, ExportBytes: 24,
		Files: []turnchanges.CheckpointExportFile{{
			File: turnchanges.CheckpointFile{PathBytes: []byte("src/a.go"), OldPathBytes: []byte("src/old.go"),
				Kind: "renamed", OldOID: "old", NewOID: "new", OldMode: "100644", NewMode: "100755", Added: int64Pointer(2), Deleted: int64Pointer(1)},
			CanonicalPatch: []byte("patch"), FilteredPatch: []byte("filtered"), OldRendering: []byte("before"), NewRendering: []byte("after"),
			ContentAvailability: turnchanges.Ready, CanonicalBytes: 5,
		}},
	}}
	service := NewContentService(repository, func() time.Time { return time.Unix(10, 0) })
	repository.storeReceipt = models.TurnChangeContentStoreReceipt{StoredBytes: 20, Complete: true}
	receipt, err := service.ExportAndStore(context.Background(), exporter,
		turnchanges.ExportRequest{ChangeSetID: "change-set", CheckoutID: "checkout", Repo: "app", HashAlgorithm: "sha1",
			StartCommitOID: "start", StartTreeOID: "start-tree", EndCommitOID: "end", EndTreeOID: "end-tree"}, "repository-change")
	if err != nil {
		t.Fatalf("ExportAndStore: %v", err)
	}
	if !receipt.Complete || receipt.FileCount != 1 || receipt.ExportBytes != 20 {
		t.Fatalf("receipt = %+v", receipt)
	}
	if exporter.request.Repo != "app" || repository.repositoryChangeID != "repository-change" || len(repository.files) != 1 {
		t.Fatalf("export/store routing = request %+v, change %q, files %d", exporter.request, repository.repositoryChangeID, len(repository.files))
	}
	file := repository.files[0]
	if file.File.Path != "src/a.go" || file.File.OldPath == nil || *file.File.OldPath != "src/old.go" || file.File.CheckoutID != "checkout" ||
		file.File.Kind != "renamed" || file.File.OldMode != "100644" || file.File.NewMode != "100755" || string(file.CanonicalPatch) != "patch" {
		t.Fatalf("stored export = %+v", file)
	}
}

func TestContentServiceReturnsStorageCompletenessAndPersistsFileSummaries(t *testing.T) {
	repository := &fakeContentRepository{storeReceipt: models.TurnChangeContentStoreReceipt{
		StoredBytes: 7, Complete: false, Reason: models.TurnChangeReasonSizeLimit,
	}}
	service := NewContentService(repository, nil)
	file := turnchanges.CheckpointFile{
		PathBytes: []byte("src/large.go"), OldPathBytes: []byte("src/old.go"), Kind: "renamed",
		OldOID: "old-object", NewOID: "new-object", OldMode: "100644", NewMode: "100755",
		Added: int64Pointer(3), Deleted: int64Pointer(1),
	}
	if err := service.StoreSummaries(context.Background(), "repository-change", "checkout", []turnchanges.CheckpointFile{file}); err != nil {
		t.Fatalf("StoreSummaries: %v", err)
	}
	if len(repository.files) != 1 || repository.files[0].File.Path != "src/large.go" ||
		repository.files[0].File.OldPath == nil || *repository.files[0].File.OldPath != "src/old.go" ||
		repository.files[0].File.ContentAvailability != models.TurnChangeAvailabilityUnavailable {
		t.Fatalf("stored comparison summary = %+v", repository.files)
	}
	repository.files = nil
	repository.repositoryChangeID = ""
	receipt, err := service.ExportAndStore(context.Background(), &fakeContentExporter{result: &turnchanges.CheckpointExport{
		ChangeSetID: "change-set", CheckoutID: "checkout", Complete: true, ExportBytes: 24,
		HashAlgorithm: "sha1", StartCommitOID: "start", StartTreeOID: "start-tree", EndCommitOID: "end", EndTreeOID: "end-tree",
		Files: []turnchanges.CheckpointExportFile{{
			File: file, CanonicalPatch: []byte("patch"), FilteredPatch: []byte("patch"),
			ContentAvailability: turnchanges.Ready, CanonicalBytes: 5,
		}},
	}}, turnchanges.ExportRequest{ChangeSetID: "change-set", CheckoutID: "checkout", HashAlgorithm: "sha1",
		StartCommitOID: "start", StartTreeOID: "start-tree", EndCommitOID: "end", EndTreeOID: "end-tree"}, "repository-change")
	if err != nil {
		t.Fatalf("ExportAndStore: %v", err)
	}
	if receipt.Complete || receipt.ExportBytes != 7 || receipt.Reason != turnchanges.ReasonSizeLimit {
		t.Fatalf("receipt = %+v; want actual partial storage receipt", receipt)
	}
}

func TestContentServiceMarksEmptyIncompleteExportUnavailable(t *testing.T) {
	repository := &fakeContentRepository{}
	service := NewContentService(repository, nil)
	_, err := service.ExportAndStore(context.Background(), &fakeContentExporter{result: &turnchanges.CheckpointExport{
		ChangeSetID: "change-set", CheckoutID: "checkout", Complete: false, Reason: turnchanges.ReasonContentUnavailable,
		HashAlgorithm: "sha1", StartCommitOID: "start", StartTreeOID: "start-tree", EndCommitOID: "end", EndTreeOID: "end-tree",
	}}, turnchanges.ExportRequest{ChangeSetID: "change-set", CheckoutID: "checkout", HashAlgorithm: "sha1",
		StartCommitOID: "start", StartTreeOID: "start-tree", EndCommitOID: "end", EndTreeOID: "end-tree"}, "repository-change")
	if err != nil {
		t.Fatalf("ExportAndStore: %v", err)
	}
	if repository.statusComplete || repository.statusReason != models.TurnChangeReasonContentUnavailable {
		t.Fatalf("stored status = complete %t, reason %q", repository.statusComplete, repository.statusReason)
	}
}

func TestContentServiceRejectsMismatchedExportAndReleasesReadLease(t *testing.T) {
	repository := &fakeContentRepository{payload: &models.TurnChangeContentPayload{Content: []byte("bytes")}}
	service := NewContentService(repository, nil)
	_, err := service.ExportAndStore(context.Background(), &fakeContentExporter{result: &turnchanges.CheckpointExport{
		ChangeSetID: "other", CheckoutID: "checkout",
	}}, turnchanges.ExportRequest{ChangeSetID: "expected", CheckoutID: "checkout"}, "repository-change")
	if err == nil || len(repository.files) != 0 {
		t.Fatalf("mismatched export result = %v, files=%d", err, len(repository.files))
	}
	payload, err := service.Read(context.Background(), "change-set", "file", models.TurnChangeContentCanonicalPatch)
	if err != nil || string(payload.Content) != "bytes" || repository.leaseID == "" || repository.releasedLease != repository.leaseID {
		t.Fatalf("Read = %+v, %v, lease=%q released=%q", payload, err, repository.leaseID, repository.releasedLease)
	}
}

func TestContentServiceRejectsOversizedHistoricalResponse(t *testing.T) {
	repository := &fakeContentRepository{payload: &models.TurnChangeContentPayload{Content: make([]byte, MaxHistoricalContentResponse+1)}}
	service := NewContentService(repository, nil)
	if _, err := service.Read(context.Background(), "change-set", "file", models.TurnChangeContentCanonicalPatch); err == nil {
		t.Fatal("oversized content was returned")
	}
}

func int64Pointer(value int64) *int64 { return &value }

type fakeContentExporter struct {
	request turnchanges.ExportRequest
	result  *turnchanges.CheckpointExport
}

func (f *fakeContentExporter) ExportTurnCheckpoint(_ context.Context, request turnchanges.ExportRequest) (*turnchanges.CheckpointExport, error) {
	f.request = request
	return f.result, nil
}

type fakeContentRepository struct {
	repositoryChangeID string
	files              []models.TurnChangeFileContent
	statusComplete     bool
	statusReason       models.TurnChangeReason
	leaseID            string
	releasedLease      string
	payload            *models.TurnChangeContentPayload
	storeReceipt       models.TurnChangeContentStoreReceipt
}

func (f *fakeContentRepository) StoreTurnChangeFiles(_ context.Context, id string, files []models.TurnChangeFileContent) error {
	f.repositoryChangeID, f.files = id, files
	return nil
}

func (f *fakeContentRepository) StoreTurnChangeFilesWithReceipt(ctx context.Context, id string, files []models.TurnChangeFileContent) (models.TurnChangeContentStoreReceipt, error) {
	if err := f.StoreTurnChangeFiles(ctx, id, files); err != nil {
		return models.TurnChangeContentStoreReceipt{}, err
	}
	receipt := f.storeReceipt
	if receipt == (models.TurnChangeContentStoreReceipt{}) {
		receipt.Complete = true
		for _, file := range files {
			receipt.StoredBytes += int64(len(file.CanonicalPatch) + len(file.FilteredPatch) + len(file.OldRendering) + len(file.NewRendering))
		}
	}
	return receipt, nil
}

func (f *fakeContentRepository) ReadTurnChangeContent(context.Context, string, string, models.TurnChangeContentVariant) (*models.TurnChangeContentPayload, error) {
	return f.payload, nil
}

func (f *fakeContentRepository) SetTurnRepositoryContentStatus(_ context.Context, _ string, complete bool, reason models.TurnChangeReason) error {
	f.statusComplete, f.statusReason = complete, reason
	return nil
}

func (f *fakeContentRepository) AcquireTurnChangeContentLease(_ context.Context, changeSetID string, duration time.Duration) (*models.TurnChangeContentLease, error) {
	if changeSetID == "" || duration <= 0 {
		return nil, errors.New("invalid lease request")
	}
	f.leaseID = "lease"
	return &models.TurnChangeContentLease{ID: f.leaseID}, nil
}

func (f *fakeContentRepository) ReleaseTurnChangeContentLease(_ context.Context, leaseID string) error {
	f.releasedLease = leaseID
	return nil
}

func (f *fakeContentRepository) ApplyTurnChangeRetention(_ context.Context, policy models.TurnChangeRetentionPolicy, now time.Time) (models.TurnChangeRetentionResult, error) {
	if policy.RetainFor != DefaultContentRetention || now.IsZero() {
		return models.TurnChangeRetentionResult{}, errors.New("unexpected retention policy")
	}
	return models.TurnChangeRetentionResult{}, nil
}
