package changes

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
)

func TestPreservationRetrierRecoversTemporaryFailureAcrossPasses(t *testing.T) {
	archive, _, _, _ := buildTurnArchiveFixture(t)
	store := NewGitHistoryStore(t.TempDir(), nil)
	contentRepo := &fakeContentRepository{}
	content := NewContentService(contentRepo, nil)
	content.SetHistoryStore(store)
	job := &models.TurnRepositoryChangeSet{
		ID: uuid.NewString(), TurnChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(),
		RepositoryID: uuid.NewString(), HashAlgorithm: "sha1",
		StartCommitOID: archive.StartCommitOID, StartTreeOID: archive.StartTreeOID,
		EndCommitOID: archive.EndCommitOID, EndTreeOID: archive.EndTreeOID,
		PreservationState: models.TurnChangePreservationPending, TaskID: "task-1", SessionID: "session-1",
	}
	archive.ChangeSetID, archive.CheckoutID = job.TurnChangeSetID, job.CheckoutID
	client := &flakyArchiveClient{archive: archive, failures: 1}
	repo := &fakeRetryRepository{job: job}
	changed := 0
	retrier := NewPreservationRetrier(repo, content, func(context.Context, string) (ArchiveClient, func(), error) {
		return client, func() {}, nil
	}, func(context.Context, string, string, string) { changed++ }, nil)

	first, err := retrier.RetryPending(context.Background())
	if err != nil {
		t.Fatalf("first retry pass: %v", err)
	}
	if first.Retried != 1 || first.Waiting != 1 || job.PreservationDeadline == nil {
		t.Fatalf("first retry = %+v deadline=%v", first, job.PreservationDeadline)
	}
	repo.ready = true
	second, err := retrier.RetryPending(context.Background())
	if err != nil {
		t.Fatalf("second retry pass: %v", err)
	}
	if second.Ready != 1 || changed != 1 || !contentRepo.contentComplete {
		t.Fatalf("second retry = %+v changed=%d complete=%t", second, changed, contentRepo.contentComplete)
	}
	if len(repo.statusUpdates) != 1 || !repo.statusUpdates[0] {
		t.Fatalf("turn content status updates = %v", repo.statusUpdates)
	}
}

func TestPreservationRetrierSettlesExpiredWindowWithoutCallingExecutor(t *testing.T) {
	store := NewGitHistoryStore(t.TempDir(), nil)
	contentRepo := &fakeContentRepository{}
	content := NewContentService(contentRepo, nil)
	content.SetHistoryStore(store)
	deadline := time.Now().Add(-time.Minute)
	job := &models.TurnRepositoryChangeSet{
		ID: uuid.NewString(), TurnChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(),
		PreservationState: models.TurnChangePreservationPending, PreservationDeadline: &deadline,
		TaskID: "task-expired", SessionID: "session-expired",
	}
	repo := &fakeRetryRepository{job: job}
	calls := 0
	retrier := NewPreservationRetrier(repo, content, func(context.Context, string) (ArchiveClient, func(), error) {
		calls++
		return nil, func() {}, nil
	}, nil, nil)
	result, err := retrier.RetryPending(context.Background())
	if err != nil {
		t.Fatalf("expired retry pass: %v", err)
	}
	if result.Settled != 1 || calls != 0 {
		t.Fatalf("expired retry = %+v executor calls=%d", result, calls)
	}
	if contentRepo.preservationState != models.TurnChangePreservationUnavailable {
		t.Fatalf("expired preservation state = %q", contentRepo.preservationState)
	}
}

func TestPreservationRetrierLeavesPendingWorkWhenExecutorIsGone(t *testing.T) {
	store := NewGitHistoryStore(t.TempDir(), nil)
	content := NewContentService(&fakeContentRepository{}, nil)
	content.SetHistoryStore(store)
	job := &models.TurnRepositoryChangeSet{
		ID: uuid.NewString(), TurnChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(),
		PreservationState: models.TurnChangePreservationPending, TaskID: "task-gone", SessionID: "session-gone",
	}
	retrier := NewPreservationRetrier(&fakeRetryRepository{job: job}, content,
		func(context.Context, string) (ArchiveClient, func(), error) {
			return nil, func() {}, errNoPreservationExecutor
		}, nil, nil)
	result, err := retrier.RetryPending(context.Background())
	if err != nil {
		t.Fatalf("missing executor retry: %v", err)
	}
	if result.Waiting != 1 || job.PreservationDeadline == nil {
		t.Fatalf("missing executor result = %+v deadline=%v", result, job.PreservationDeadline)
	}
}

type fakeRetryRepository struct {
	job           *models.TurnRepositoryChangeSet
	ready         bool
	statusUpdates []bool
	deadlineSet   []time.Time
}

func (f *fakeRetryRepository) ListTurnPreservationJobs(context.Context, int) ([]*models.TurnRepositoryChangeSet, error) {
	if f.job == nil {
		return nil, nil
	}
	copy := *f.job
	return []*models.TurnRepositoryChangeSet{&copy}, nil
}

func (f *fakeRetryRepository) ListTurnRepositoryChanges(context.Context, string) ([]*models.TurnRepositoryChangeSet, error) {
	row := *f.job
	row.ContentComplete = f.ready
	return []*models.TurnRepositoryChangeSet{&row}, nil
}

func (f *fakeRetryRepository) SetTurnRepositoryPreservationDeadline(_ context.Context, _ string, deadline time.Time) error {
	f.job.PreservationDeadline = &deadline
	f.deadlineSet = append(f.deadlineSet, deadline)
	return nil
}

func (f *fakeRetryRepository) UpdateTurnChangeSetContentStatus(_ context.Context, _ string, complete bool, _ models.TurnChangeReason) error {
	f.statusUpdates = append(f.statusUpdates, complete)
	return nil
}

var errNoPreservationExecutor = &preservationClientError{}

type preservationClientError struct{}

func (*preservationClientError) Error() string { return "executor unavailable" }
