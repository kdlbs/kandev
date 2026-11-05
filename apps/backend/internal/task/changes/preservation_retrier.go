package changes

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// ArchiveClientProvider resolves the live executor client for a session. It
// returns no client when the original execution is no longer available.
type ArchiveClientProvider func(ctx context.Context, sessionID string) (ArchiveClient, func(), error)

// PreservationChanged reports a recovered turn so the task service can publish
// its updated summary.
type PreservationChanged func(ctx context.Context, taskID, sessionID, changeSetID string)

type PreservationRetryRepository interface {
	ListTurnPreservationJobs(ctx context.Context, limit int) ([]*models.TurnRepositoryChangeSet, error)
	ListTurnRepositoryChanges(ctx context.Context, changeSetID string) ([]*models.TurnRepositoryChangeSet, error)
	SetTurnRepositoryPreservationDeadline(ctx context.Context, repositoryChangeID string, deadline time.Time) error
	UpdateTurnChangeSetContentStatus(ctx context.Context, changeSetID string, contentComplete bool, reason models.TurnChangeReason) error
}

// PreservationRetrier is the durable bounded owner for pending terminal
// preservation. It only retries the original accepted pair through a live
// execution and settles explicitly when the recovery window expires.
type PreservationRetrier struct {
	repository PreservationRetryRepository
	content    *ContentService
	provider   ArchiveClientProvider
	changed    PreservationChanged
	now        func() time.Time
	window     time.Duration
}

type PreservationRetryResult struct {
	Retried int
	Ready   int
	Settled int
	Waiting int
}

func NewPreservationRetrier(
	repository PreservationRetryRepository,
	content *ContentService,
	provider ArchiveClientProvider,
	changed PreservationChanged,
	now func() time.Time,
) *PreservationRetrier {
	if now == nil {
		now = time.Now
	}
	return &PreservationRetrier{
		repository: repository, content: content, provider: provider, changed: changed,
		now: now, window: PreservationRecoveryWindow,
	}
}

// RetryPending processes one bounded page. A missing live execution leaves a
// row pending until its deadline; expired deadlines settle explicitly.
func (r *PreservationRetrier) RetryPending(ctx context.Context) (PreservationRetryResult, error) {
	result := PreservationRetryResult{}
	if r == nil || r.repository == nil || r.content == nil || !r.content.HistoryEnabled() {
		return result, nil
	}
	jobs, err := r.repository.ListTurnPreservationJobs(ctx, 100)
	if err != nil {
		return result, err
	}
	var failures []error
	for _, job := range jobs {
		if job == nil {
			continue
		}
		outcome, jobErr := r.retryJob(ctx, job)
		if jobErr != nil {
			failures = append(failures, jobErr)
		}
		result.Retried += outcome.retried
		result.Ready += outcome.ready
		result.Settled += outcome.settled
		result.Waiting += outcome.waiting
	}
	return result, errors.Join(failures...)
}

type preservationJobOutcome struct {
	retried, ready, settled, waiting int
}

func (r *PreservationRetrier) retryJob(ctx context.Context, job *models.TurnRepositoryChangeSet) (preservationJobOutcome, error) {
	now := r.now().UTC()
	if job.PreservationDeadline == nil {
		deadline := now.Add(r.window)
		if err := r.repository.SetTurnRepositoryPreservationDeadline(ctx, job.ID, deadline); err != nil {
			return preservationJobOutcome{}, err
		}
		job.PreservationDeadline = &deadline
	}
	if !job.PreservationDeadline.After(now) {
		if err := r.content.SettlePreservationExpired(ctx, *job); err != nil {
			return preservationJobOutcome{}, err
		}
		return preservationJobOutcome{settled: 1}, nil
	}
	if r.provider == nil {
		return preservationJobOutcome{waiting: 1}, nil
	}
	client, release, providerErr := r.provider(ctx, job.SessionID)
	if providerErr != nil || client == nil {
		if release != nil {
			release()
		}
		return preservationJobOutcome{waiting: 1}, nil
	}
	receipt, attemptErr := r.content.PreserveOnce(ctx, client, *job)
	release()
	if attemptErr != nil {
		return preservationJobOutcome{retried: 1}, attemptErr
	}
	if receipt.Complete {
		if err := r.refreshChangeSet(ctx, job); err != nil {
			return preservationJobOutcome{retried: 1, ready: 1}, err
		}
		return preservationJobOutcome{retried: 1, ready: 1}, nil
	}
	if receipt.Retryable {
		return preservationJobOutcome{retried: 1, waiting: 1}, nil
	}
	if err := r.refreshChangeSet(ctx, job); err != nil {
		return preservationJobOutcome{retried: 1, settled: 1}, err
	}
	return preservationJobOutcome{retried: 1, settled: 1}, nil
}

func (r *PreservationRetrier) refreshChangeSet(ctx context.Context, job *models.TurnRepositoryChangeSet) error {
	rows, err := r.repository.ListTurnRepositoryChanges(ctx, job.TurnChangeSetID)
	if err != nil {
		return err
	}
	allReady := len(rows) > 0
	for _, row := range rows {
		if row == nil || !row.ContentComplete {
			allReady = false
			break
		}
	}
	reason := models.TurnChangeReasonContentUnavailable
	if allReady {
		reason = ""
	}
	if err := r.repository.UpdateTurnChangeSetContentStatus(ctx, job.TurnChangeSetID, allReady, reason); err != nil {
		return err
	}
	if allReady && r.changed != nil {
		r.changed(ctx, job.TaskID, job.SessionID, job.TurnChangeSetID)
	}
	return nil
}
