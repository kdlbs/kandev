package backendapp

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"golang.org/x/sync/errgroup"
)

type executionEvidenceSource interface {
	Observe(context.Context, string, string) (*contract.Observation, error)
}

type executionObserver struct {
	tasks   *taskrepo.Repository
	source  executionEvidenceSource
	publish func(context.Context, *models.ExecutionSnapshot) error
	token   string
	wakeups chan struct{}
}

func newExecutionObserver(ctx context.Context, tasks *taskrepo.Repository, source executionEvidenceSource, publish func(context.Context, *models.ExecutionSnapshot) error) (*executionObserver, error) {
	token := uuid.NewString()
	if _, err := tasks.ClaimExecutionObserver(ctx, token); err != nil {
		return nil, err
	}
	return &executionObserver{tasks: tasks, source: source, publish: publish, token: token, wakeups: make(chan struct{}, 1)}, nil
}

func (o *executionObserver) refresh(ctx context.Context) error {
	if o.source == nil {
		return nil
	}
	const pageSize = 100
	after := ""
	var failures []error
	for ctx.Err() == nil {
		candidates, err := o.tasks.ListExecutionObservationCandidates(ctx, after, pageSize)
		if err != nil {
			return err
		}
		var group errgroup.Group
		group.SetLimit(4)
		for _, candidate := range candidates {
			group.Go(func() error { return o.observe(ctx, candidate) })
		}
		if err = group.Wait(); err != nil {
			failures = append(failures, err)
		}
		if len(candidates) < pageSize {
			return errors.Join(failures...)
		}
		after = candidates[len(candidates)-1].Snapshot.AttemptID
	}
	return ctx.Err()
}

func (o *executionObserver) observe(ctx context.Context, candidate taskrepo.ExecutionObservationCandidate) error {
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	value, err := o.source.Observe(readCtx, candidate.OrgID, candidate.Snapshot.AttemptID)
	readErr := readCtx.Err()
	cancel()
	ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err != nil || value == nil || readErr != nil {
		return o.observationFailed(ctx, candidate.Snapshot)
	}
	if _, err = o.tasks.RecordExecutionObservation(ctx, o.token, candidate.Snapshot, *value); err != nil {
		return err
	}
	if o.publish != nil {
		return o.publish(ctx, candidate.Snapshot)
	}
	return nil
}

// observationFailed removes a live claim without exposing provider diagnostics.
func (o *executionObserver) observationFailed(ctx context.Context, snapshot *models.ExecutionSnapshot) error {
	session, err := o.tasks.GetTaskSession(ctx, snapshot.SessionID)
	if err != nil {
		return err
	}
	previous := session.ExecutionObservation
	if previous == nil || previous.AttemptID != snapshot.AttemptID {
		return errExecutionSnapshotUnavailable
	}
	value := contract.Observation{AttemptID: snapshot.AttemptID, Generation: snapshot.Sandbox.Generation,
		AttemptVersion: previous.AttemptVersion, Status: contract.StatusUnknown, ProviderHealth: contract.HealthUnknown, Reason: "evidence_unavailable", CheckedAt: time.Now().UTC()}
	if _, err = o.tasks.RecordExecutionObservation(ctx, o.token, snapshot, value); err != nil {
		return err
	}
	if o.publish != nil {
		err = o.publish(ctx, snapshot)
	}
	return errors.Join(errExecutionSnapshotUnavailable, err)
}
