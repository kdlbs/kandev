package changes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	turnChangeStartPersistenceReserve = 6 * time.Second
	turnChangeStartCaptureConcurrency = 2
)

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
	repositories, captureFailures := c.captureStartRepositories(ctx, admission, client, changeSet, scopeSet)

	accepted, err := c.repository.AcceptTurnChangeSetStart(ctx, changeSet.ID, changeSet.Revision, repositories)
	if err != nil {
		return errors.Join(fmt.Errorf("persist turn change start endpoints: %w", err), errors.Join(captureFailures...))
	}
	if !accepted {
		return nil
	}
	overlapErr := c.reconcileWriterOverlaps(ctx, changeSet)
	return errors.Join(errors.Join(captureFailures...), overlapErr)
}

func (c *Coordinator) captureStartRepositories(
	ctx context.Context,
	admission Admission,
	client CheckpointClient,
	changeSet *models.TurnChangeSet,
	scopeSet map[string]struct{},
) ([]models.TurnRepositoryChangeSet, []error) {
	captureCtx, cancelCapture := c.startCaptureContext(ctx)
	defer cancelCapture()
	repositories := make([]models.TurnRepositoryChangeSet, len(admission.Checkouts))
	captureFailures := make([]error, len(admission.Checkouts))
	var captureIndexes []int
	for index, checkout := range admission.Checkouts {
		row := c.repositoryStartRow(changeSet.ID, checkout)
		scope, ok := checkoutScope(checkout, len(admission.Checkouts), scopeSet)
		if !ok {
			row.Availability = models.TurnChangeAvailabilityUnavailable
			row.Reason = models.TurnChangeReasonCheckoutUnavailable
			captureFailures[index] = fmt.Errorf("checkout %q is absent from registered repository scopes", checkout.ID)
			repositories[index] = row
			continue
		}
		row.RepositorySubpath = scope
		repositories[index] = row
		if captureCtx.Err() != nil {
			repositories[index], captureFailures[index] = unavailableStartCapture(row, checkout.ID, captureCtx.Err())
			continue
		}
		captureIndexes = append(captureIndexes, index)
	}

	jobs := make(chan int)
	workerCount := min(turnChangeStartCaptureConcurrency, len(captureIndexes))
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				checkout := admission.Checkouts[index]
				release, acquired := c.startCaptureLimiter.acquire(captureCtx, admission.ExecutionID)
				if !acquired {
					repositories[index], captureFailures[index] = unavailableStartCapture(repositories[index], checkout.ID, captureCtx.Err())
					continue
				}
				row, captureErr := c.captureStartCheckout(captureCtx, client, changeSet, checkout, repositories[index])
				release()
				repositories[index] = row
				captureFailures[index] = captureErr
			}
		}()
	}
	for _, index := range captureIndexes {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	return repositories, captureFailures
}

func (c *Coordinator) startCaptureContext(admissionCtx context.Context) (context.Context, context.CancelFunc) {
	deadline, ok := admissionCtx.Deadline()
	if !ok {
		return admissionCtx, func() {}
	}
	reserve := c.startPersistenceReserve
	if reserve <= 0 {
		reserve = turnChangeStartPersistenceReserve
	}
	return context.WithDeadline(admissionCtx, deadline.Add(-reserve))
}

func (c *Coordinator) captureStartCheckout(
	ctx context.Context,
	client CheckpointClient,
	changeSet *models.TurnChangeSet,
	checkout Checkout,
	row models.TurnRepositoryChangeSet,
) (models.TurnRepositoryChangeSet, error) {
	if err := ctx.Err(); err != nil {
		return unavailableStartCapture(row, checkout.ID, err)
	}
	captured, captureErr := client.CaptureTurnCheckpoint(ctx, turnchanges.CheckpointRequest{
		ChangeSetID: changeSet.ID, CheckoutID: checkout.ID, Repo: row.RepositorySubpath, Boundary: turnchanges.CheckpointStart,
	})
	if captureErr != nil || !validCheckpoint(captured, changeSet.ID, checkout.ID, turnchanges.CheckpointStart) {
		if captureErr == nil {
			captureErr = errors.New("checkpoint response did not match the admitted start boundary")
		}
		return unavailableStartCapture(row, checkout.ID, captureErr)
	}
	row.StartCommitOID = captured.CommitOID
	row.StartTreeOID = captured.TreeOID
	row.HashAlgorithm = captured.HashAlgorithm
	row.StartReachabilityRef = captured.ReachabilityRef
	row.SourceGitDir = captured.CommonGitDir
	row.StartProvenanceJSON = encodeTurnChangeProvenance(captured.Provenance)
	row.CleanupPending = captured.ReachabilityRef != ""
	capturedAt := captured.CapturedAt.UTC()
	row.StartCapturedAt = &capturedAt
	row.Availability = models.TurnChangeAvailabilityPending
	return row, nil
}

func unavailableStartCapture(row models.TurnRepositoryChangeSet, checkoutID string, err error) (models.TurnRepositoryChangeSet, error) {
	row.Availability = models.TurnChangeAvailabilityUnavailable
	row.Reason = checkpointReason(err, turnchanges.ReasonCaptureFailed)
	return row, fmt.Errorf("capture start for checkout %q: %w", checkoutID, err)
}
