package changes

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
)

func TestTurnGitHistoryContentServiceReadsArchiveWithoutSQLPayloads(t *testing.T) {
	archive, _, startTree, endTree := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	repository := &fakeContentRepository{}
	repositoryChangeID := uuid.NewString()
	receipt, err := store.Import(context.Background(), uuid.NewString(), uuid.NewString(), &turnchanges.CheckpointArchive{
		ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(), HashAlgorithm: archive.HashAlgorithm,
		StartCommitOID: archive.StartCommitOID, StartTreeOID: archive.StartTreeOID,
		EndCommitOID: archive.EndCommitOID, EndTreeOID: archive.EndTreeOID,
		Pack: archive.Pack, Objects: archive.Objects, ObjectCount: archive.ObjectCount,
		UncompressedBytes: archive.UncompressedBytes, Complete: true,
	})
	if err != nil {
		t.Fatalf("import archive: %v", err)
	}
	encoded, _ := json.Marshal(receipt)
	repository.source = &models.TurnFileContentSource{
		ChangeSet: &models.TurnChangeSet{ID: uuid.NewString(), Availability: models.TurnChangeAvailabilityReady},
		Repository: &models.TurnRepositoryChangeSet{
			ID: repositoryChangeID, ContentSource: models.TurnChangeSourceGitArchive,
			PreservationState: models.TurnChangePreservationReady, PreservationReceipt: string(encoded),
			StartTreeOID: startTree, EndTreeOID: endTree,
		},
		File: &models.TurnFileChange{
			ID: uuid.NewString(), PathBytes: []byte("src/app.txt"),
			ContentAvailability: models.TurnChangeAvailabilityReady,
		},
	}
	service := NewContentService(repository, nil)
	service.SetHistoryStore(store)
	payload, err := service.Read(context.Background(), repository.source.ChangeSet.ID, repository.source.File.ID, models.TurnChangeContentCanonicalPatch)
	if err != nil {
		t.Fatalf("read archived content: %v", err)
	}
	if !strings.Contains(string(payload.Content), "-old") || !strings.Contains(string(payload.Content), "+new") {
		t.Fatalf("archived patch = %q", payload.Content)
	}
	if repository.legacyReads != 0 {
		t.Fatalf("Git-backed read fell back to SQL %d times", repository.legacyReads)
	}
}

func TestTurnChangePreservationRetriesAcceptedObjects(t *testing.T) {
	archive, _, _, _ := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	repository := &fakeContentRepository{}
	row := models.TurnRepositoryChangeSet{
		ID: uuid.NewString(), TurnChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(),
		RepositoryID: uuid.NewString(), HashAlgorithm: "sha1",
		StartCommitOID: archive.StartCommitOID, StartTreeOID: archive.StartTreeOID,
		EndCommitOID: archive.EndCommitOID, EndTreeOID: archive.EndTreeOID,
	}
	archive.ChangeSetID, archive.CheckoutID = row.TurnChangeSetID, row.CheckoutID
	archive.StartCommitOID, archive.StartTreeOID = row.StartCommitOID, row.StartTreeOID
	archive.EndCommitOID, archive.EndTreeOID = row.EndCommitOID, row.EndTreeOID
	client := &flakyArchiveClient{archive: archive, failures: 1}
	service := NewContentService(repository, nil)
	service.SetHistoryStore(store)
	receipt, err := service.Preserve(context.Background(), client, row)
	if err != nil {
		t.Fatalf("preserve accepted objects: %v", err)
	}
	if !receipt.Complete || receipt.Attempts != 2 || client.calls != 2 {
		t.Fatalf("preservation receipt = %+v, calls=%d", receipt, client.calls)
	}
	if client.lastRequest.StartCommitOID != archive.StartCommitOID || client.lastRequest.EndCommitOID != archive.EndCommitOID {
		t.Fatal("preservation retry changed the accepted endpoint pair")
	}
	if receipt.Receipt == "" {
		t.Fatal("preservation receipt was not returned for durable finalization")
	}
	if repository.preservationState != models.TurnChangePreservationReady || !repository.contentComplete {
		t.Fatalf("durable preservation state = %q complete=%t", repository.preservationState, repository.contentComplete)
	}
	if repository.gitBackedID != row.ID {
		t.Fatalf("Git-backed files were not acknowledged for %q", repository.gitBackedID)
	}
}

func TestTurnChangePreservationSettlesPermanentFailureWithoutRetry(t *testing.T) {
	store := NewGitHistoryStore(t.TempDir(), nil)
	repository := &fakeContentRepository{}
	row := models.TurnRepositoryChangeSet{
		ID: uuid.NewString(), TurnChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(),
		RepositoryID: uuid.NewString(), HashAlgorithm: "sha1",
		StartCommitOID: "a", StartTreeOID: "b", EndCommitOID: "c", EndTreeOID: "d",
	}
	client := &permanentFailureArchiveClient{}
	service := NewContentService(repository, nil)
	service.SetHistoryStore(store)
	receipt, err := service.Preserve(context.Background(), client, row)
	if err != nil {
		t.Fatalf("settle permanent preservation failure: %v", err)
	}
	if receipt.Complete || receipt.Retryable || client.calls != 1 {
		t.Fatalf("permanent preservation receipt = %+v, calls=%d", receipt, client.calls)
	}
	if repository.preservationState != models.TurnChangePreservationUnavailable {
		t.Fatalf("permanent preservation state = %q", repository.preservationState)
	}
}

func TestTurnChangePreservationRetainsCompletedRepositoriesOnAnotherFailure(t *testing.T) {
	archive, _, _, _ := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	repository := &fakeContentRepository{}
	service := NewContentService(repository, nil)
	service.SetHistoryStore(store)
	first := models.TurnRepositoryChangeSet{
		ID: uuid.NewString(), TurnChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(),
		RepositoryID: uuid.NewString(), HashAlgorithm: "sha1",
		StartCommitOID: archive.StartCommitOID, StartTreeOID: archive.StartTreeOID,
		EndCommitOID: archive.EndCommitOID, EndTreeOID: archive.EndTreeOID,
	}
	archive.ChangeSetID, archive.CheckoutID = first.TurnChangeSetID, first.CheckoutID
	archive.StartCommitOID, archive.StartTreeOID = first.StartCommitOID, first.StartTreeOID
	archive.EndCommitOID, archive.EndTreeOID = first.EndCommitOID, first.EndTreeOID
	if receipt, err := service.Preserve(context.Background(), &flakyArchiveClient{archive: archive}, first); err != nil || !receipt.Complete {
		t.Fatalf("first repository preservation = %+v, %v", receipt, err)
	}
	second := first
	second.ID = uuid.NewString()
	second.CheckoutID = uuid.NewString()
	second.RepositoryID = uuid.NewString()
	if receipt, err := service.Preserve(context.Background(), &permanentFailureArchiveClient{}, second); err != nil || receipt.Complete {
		t.Fatalf("second repository preservation = %+v, %v", receipt, err)
	}
	if repository.gitBackedID == second.ID {
		t.Fatal("failed repository replaced the completed repository's durable state")
	}
}

type flakyArchiveClient struct {
	archive     *turnchanges.CheckpointArchive
	failures    int
	calls       int
	lastRequest turnchanges.ArchiveRequest
}

func (f *flakyArchiveClient) ArchiveTurnCheckpoint(_ context.Context, request turnchanges.ArchiveRequest) (*turnchanges.CheckpointArchive, error) {
	f.calls++
	f.lastRequest = request
	if f.calls <= f.failures {
		return nil, ErrHistoryReadRetryable
	}
	return f.archive, nil
}

type permanentFailureArchiveClient struct {
	calls int
}

func (f *permanentFailureArchiveClient) ArchiveTurnCheckpoint(context.Context, turnchanges.ArchiveRequest) (*turnchanges.CheckpointArchive, error) {
	f.calls++
	return nil, &fakeCaptureError{reason: turnchanges.ReasonSizeLimit}
}

type fakeCaptureError struct {
	reason turnchanges.ReasonCode
}

func (e *fakeCaptureError) Error() string { return "archive failed" }

func (e *fakeCaptureError) CaptureReason() turnchanges.ReasonCode { return e.reason }

func TestTurnChangePreservationRejectsMismatchedArchiveIdentity(t *testing.T) {
	archive, _, _, _ := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	repository := &fakeContentRepository{}
	row := models.TurnRepositoryChangeSet{
		ID: uuid.NewString(), TurnChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(),
		RepositoryID: uuid.NewString(), HashAlgorithm: "sha1",
		StartCommitOID: archive.StartCommitOID, StartTreeOID: archive.StartTreeOID,
		EndCommitOID: archive.EndCommitOID, EndTreeOID: archive.EndTreeOID,
	}
	service := NewContentService(repository, nil)
	service.SetHistoryStore(store)
	receipt, err := service.Preserve(context.Background(), &flakyArchiveClient{archive: archive}, row)
	if err != nil || receipt.Complete || receipt.Retryable {
		t.Fatalf("mismatched archive receipt = %+v, %v", receipt, err)
	}
	if repository.preservationState == models.TurnChangePreservationReady {
		t.Fatal("mismatched archive was recorded as preservation proof")
	}
}

func TestTurnChangePreservationRejectsArchiveWithoutEndpointObjects(t *testing.T) {
	archive, _, _, _ := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	repository := &fakeContentRepository{}
	row := models.TurnRepositoryChangeSet{
		ID: uuid.NewString(), TurnChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(),
		RepositoryID: uuid.NewString(), HashAlgorithm: "sha1",
		StartCommitOID: archive.StartCommitOID, StartTreeOID: archive.StartTreeOID,
		EndCommitOID: archive.EndCommitOID, EndTreeOID: archive.EndTreeOID,
	}
	archive.ChangeSetID, archive.CheckoutID = row.TurnChangeSetID, row.CheckoutID
	archive.Objects = archive.Objects[:len(archive.Objects)-1]
	archive.ObjectCount = int64(len(archive.Objects))
	service := NewContentService(repository, nil)
	service.SetHistoryStore(store)
	receipt, err := service.Preserve(context.Background(), &flakyArchiveClient{archive: archive}, row)
	if err != nil || receipt.Complete || receipt.Retryable {
		t.Fatalf("truncated archive receipt = %+v, %v", receipt, err)
	}
}

func TestTurnGitHistoryUsesSurvivingCommonSourceWithoutArchiving(t *testing.T) {
	archive, repoDir, startTree, endTree := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	repository := &fakeContentRepository{}
	service := NewContentService(repository, nil)
	service.SetHistoryStore(store)
	row := models.TurnRepositoryChangeSet{
		ID: uuid.NewString(), TurnChangeSetID: archive.ChangeSetID, CheckoutID: archive.CheckoutID,
		RepositoryID: uuid.NewString(), HashAlgorithm: "sha1", SourceGitDir: filepath.Join(repoDir, ".git"),
		StartCommitOID: archive.StartCommitOID, StartTreeOID: startTree,
		EndCommitOID: archive.EndCommitOID, EndTreeOID: endTree,
	}
	client := &flakyArchiveClient{archive: archive}
	receipt, err := service.PreserveOnce(context.Background(), client, row)
	if err != nil || !receipt.Complete {
		t.Fatalf("common-source preservation = %+v, %v", receipt, err)
	}
	if client.calls != 0 {
		t.Fatalf("common-source preservation transferred %d archives", client.calls)
	}
	if repository.preservationSource != models.TurnChangeSourceGitCommon {
		t.Fatalf("preservation source = %q", repository.preservationSource)
	}
	if receipt.Source != models.TurnChangeSourceGitCommon || receipt.Receipt != "" {
		t.Fatalf("preservation receipt = %+v, want the settled common source", receipt)
	}
	source := &models.TurnFileContentSource{
		ChangeSet: &models.TurnChangeSet{ID: row.TurnChangeSetID, Availability: models.TurnChangeAvailabilityReady},
		Repository: &models.TurnRepositoryChangeSet{
			ContentSource: models.TurnChangeSourceGitCommon, SourceGitDir: filepath.Join(repoDir, ".git"),
			HashAlgorithm: "sha1", StartCommitOID: archive.StartCommitOID, StartTreeOID: startTree,
			EndCommitOID: archive.EndCommitOID, EndTreeOID: endTree,
		},
		File: &models.TurnFileChange{ID: uuid.NewString(), PathBytes: []byte("src/app.txt"), ContentAvailability: models.TurnChangeAvailabilityReady},
	}
	payload, err := service.readCommon(context.Background(), source, models.TurnChangeContentCanonicalPatch)
	if err != nil || payload == nil || !strings.Contains(string(payload.Content), "+new") {
		t.Fatalf("common-source read = %q, %v", payload.Content, err)
	}
	// A removed worktree leaves the common directory usable.
	if err := os.RemoveAll(repoDir); err != nil {
		t.Fatal(err)
	}
	if _, err := service.readCommon(context.Background(), source, models.TurnChangeContentCanonicalPatch); err == nil {
		t.Fatal("common-source read unexpectedly survived common directory removal")
	}
}

func TestTurnGitHistoryCommonSourceSurvivesLinkedWorktreeRemoval(t *testing.T) {
	archive, repoDir, startTree, endTree := buildTurnArchiveFixture(t)
	worktree := filepath.Join(t.TempDir(), "linked-worktree")
	runGitTest(t, repoDir, "worktree", "add", "-q", "--detach", worktree, "HEAD")
	store := NewGitHistoryStore(t.TempDir(), nil)
	service := NewContentService(&fakeContentRepository{}, nil)
	service.SetHistoryStore(store)
	source := &models.TurnFileContentSource{
		ChangeSet: &models.TurnChangeSet{ID: archive.ChangeSetID, Availability: models.TurnChangeAvailabilityReady},
		Repository: &models.TurnRepositoryChangeSet{
			ContentSource: models.TurnChangeSourceGitCommon, SourceGitDir: filepath.Join(repoDir, ".git"),
			HashAlgorithm: "sha1", StartCommitOID: archive.StartCommitOID, StartTreeOID: startTree,
			EndCommitOID: archive.EndCommitOID, EndTreeOID: endTree,
		},
		File: &models.TurnFileChange{ID: uuid.NewString(), PathBytes: []byte("src/app.txt"), ContentAvailability: models.TurnChangeAvailabilityReady},
	}
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatal(err)
	}
	payload, err := service.readCommon(context.Background(), source, models.TurnChangeContentCanonicalPatch)
	if err != nil || payload == nil || !strings.Contains(string(payload.Content), "+new") {
		t.Fatalf("read after linked worktree removal = %q, %v", payload.Content, err)
	}
}
