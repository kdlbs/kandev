package cursorcloud

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	provider "github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	maxSubmissionCandidatePages = 10
	maxSubmissionCandidates     = 25
)

var (
	ErrSubmissionResolutionUnavailable = errors.New("cursor cloud submission cannot be resolved")
	ErrSubmissionCandidateUnavailable  = errors.New("cursor cloud run is not a valid candidate")
	ErrRetryAcknowledgmentRequired     = errors.New("duplicate-work acknowledgment is required")
)

type SubmissionCandidate struct {
	RunID     string    `json:"runId"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type runLister interface {
	ListRuns(context.Context, string, int, string) (provider.RunPage, error)
}

func (r *Runtime) ListSubmissionCandidates(ctx context.Context, executionID string) (*models.ManagedAgentOperation, []SubmissionCandidate, error) {
	binding, operation, client, err := r.loadUnknownFollowup(ctx, executionID)
	if err != nil {
		return nil, nil, err
	}
	lister, ok := client.(runLister)
	if !ok {
		return nil, nil, errors.New("cursor cloud client does not support run resolution")
	}
	if operation.DispatchStartedAt == nil {
		return operation, []SubmissionCandidate{}, nil
	}
	candidates, err := collectSubmissionCandidates(ctx, lister, binding, operation)
	if err != nil {
		return nil, nil, err
	}
	return operation, sortedSubmissionCandidates(candidates), nil
}

func collectSubmissionCandidates(
	ctx context.Context,
	lister runLister,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
) (map[string]SubmissionCandidate, error) {
	cursor := ""
	seenCursors := map[string]struct{}{}
	candidates := make(map[string]SubmissionCandidate)
	for pageIndex := 0; pageIndex < maxSubmissionCandidatePages; pageIndex++ {
		page, err := lister.ListRuns(ctx, binding.RemoteAgentID, 100, cursor)
		if err != nil {
			return nil, fmt.Errorf("list Cursor Cloud runs for submission resolution: %w", err)
		}
		addEligibleSubmissionCandidates(candidates, page.Items, binding.RemoteAgentID, operation)
		if page.NextCursor == "" || len(candidates) >= maxSubmissionCandidates {
			break
		}
		if _, duplicate := seenCursors[page.NextCursor]; duplicate {
			return nil, errors.New("cursor cloud run listing repeated a pagination cursor")
		}
		seenCursors[page.NextCursor] = struct{}{}
		cursor = page.NextCursor
	}
	return candidates, nil
}

func addEligibleSubmissionCandidates(
	candidates map[string]SubmissionCandidate,
	runs []provider.Run,
	remoteAgentID string,
	operation *models.ManagedAgentOperation,
) {
	for _, run := range runs {
		createdAt, eligible := eligibleSubmissionRun(run, remoteAgentID, operation)
		if !eligible {
			continue
		}
		candidates[run.ID] = SubmissionCandidate{RunID: run.ID, Status: boundedRunStatus(run.Status), CreatedAt: createdAt}
	}
}

func eligibleSubmissionRun(
	run provider.Run,
	remoteAgentID string,
	operation *models.ManagedAgentOperation,
) (time.Time, bool) {
	if run.ID == "" || run.AgentID != remoteAgentID || run.ID == operation.PreSubmitRunID {
		return time.Time{}, false
	}
	createdAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(run.CreatedAt))
	if err != nil || createdAt.Before(*operation.DispatchStartedAt) {
		return time.Time{}, false
	}
	return createdAt, true
}

func sortedSubmissionCandidates(candidates map[string]SubmissionCandidate) []SubmissionCandidate {
	result := make([]SubmissionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if len(result) > maxSubmissionCandidates {
		result = result[:maxSubmissionCandidates]
	}
	return result
}

func boundedRunStatus(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "CREATING", "RUNNING", "FINISHED", "FAILED", "ERROR", "CANCELLED", "EXPIRED":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "unknown"
	}
}

func (r *Runtime) BindSubmissionCandidate(ctx context.Context, executionID, runID string) (*models.ManagedAgentOperation, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, ErrSubmissionCandidateUnavailable
	}
	binding, operation, client, err := r.loadUnknownFollowup(ctx, executionID)
	if err != nil {
		return nil, err
	}
	if err := r.validateSubmissionCandidate(ctx, executionID, runID); err != nil {
		return nil, err
	}
	run, err := client.GetRun(ctx, binding.RemoteAgentID, runID)
	if err != nil {
		return nil, fmt.Errorf("verify Cursor Cloud submission candidate: %w", err)
	}
	if run.ID != runID || run.AgentID != binding.RemoteAgentID {
		return nil, ErrSubmissionCandidateUnavailable
	}
	accepted, err := r.acceptSubmissionCandidate(ctx, binding, operation, run.ID)
	if err != nil {
		return nil, err
	}
	return r.settleAcceptedCandidate(ctx, binding, accepted, client, run)
}

func (r *Runtime) validateSubmissionCandidate(ctx context.Context, executionID, runID string) error {
	_, candidates, err := r.ListSubmissionCandidates(ctx, executionID)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		if candidate.RunID == runID {
			return nil
		}
	}
	return ErrSubmissionCandidateUnavailable
}

func (r *Runtime) acceptSubmissionCandidate(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	runID string,
) (*models.ManagedAgentOperation, error) {
	accepted, err := r.repository.CompareAndSwapManagedAgentOperation(ctx, models.ManagedAgentOperationUpdate{
		OperationID: operation.ID, ExpectedRevision: operation.Revision,
		ExpectedBindingRevision: binding.Revision, State: models.ManagedAgentSubmissionAccepted,
		RemoteRunID: runID,
	})
	if err != nil {
		latest, readErr := r.repository.GetManagedAgentLatestOperation(ctx, binding.ID)
		if readErr != nil || latest.State != models.ManagedAgentSubmissionAccepted || latest.RemoteRunID != runID {
			return nil, fmt.Errorf("bind verified Cursor Cloud run: %w", err)
		}
		accepted = latest
	}
	recordDispatch(accepted.Kind, "accepted")
	return accepted, nil
}

func (r *Runtime) settleAcceptedCandidate(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	accepted *models.ManagedAgentOperation,
	client Provider,
	run provider.Run,
) (*models.ManagedAgentOperation, error) {
	if _, terminal := terminalSubmissionState(run.Status); terminal {
		currentBinding, err := r.repository.GetManagedAgentBindingByExecution(ctx, binding.ExecutionID)
		if err != nil {
			return nil, err
		}
		result := resultSnapshotForRun(currentBinding, run)
		if agent, agentErr := client.GetAgent(ctx, currentBinding.RemoteAgentID); agentErr == nil {
			result.AgentURL = safeCursorAgentURL(agent.URL)
		}
		settled, err := r.settleObservedRun(ctx, currentBinding, accepted, run, result)
		if err != nil {
			return nil, err
		}
		if settled && r.publishStream != nil {
			r.publishStream(ctx, terminalStreamPayload(currentBinding, accepted, run.Status, result))
		}
		return r.repository.GetManagedAgentLatestOperation(ctx, currentBinding.ID)
	}
	return accepted, nil
}

func (r *Runtime) RetryUnknownSubmission(ctx context.Context, executionID, resolutionID string, acknowledgeDuplicateWork bool) (*models.ManagedAgentOperation, error) {
	if !r.enabled() {
		return nil, errors.New("cursor cloud is disabled for new dispatches")
	}
	if !acknowledgeDuplicateWork {
		return nil, ErrRetryAcknowledgmentRequired
	}
	resolutionID = strings.TrimSpace(resolutionID)
	if resolutionID == "" || len(resolutionID) > 128 {
		return nil, errors.New("cursor cloud retry identity is invalid")
	}
	binding, original, _, err := r.loadUnknownFollowup(ctx, executionID)
	if err != nil {
		return nil, err
	}
	turnID := "cursor-cloud-retry:" + original.ID + ":" + resolutionID
	snapshot := original.RequestSnapshot
	snapshot.TurnID = turnID
	operation := &models.ManagedAgentOperation{
		ID: r.newID(), BindingID: binding.ID, PromptTurnID: turnID, Kind: models.ManagedAgentOperationFollowup,
		RequestDigest: digestRequest(snapshot.Prompt, turnID, binding.Launch), RequestSnapshot: snapshot,
		RetryAcknowledgesOperationID: original.ID, DuplicationRiskAcknowledged: true,
	}
	owner := r.newID()
	reservedBinding, reserved, _, err := r.repository.ReserveManagedAgentOperation(
		ctx, operation, binding.Revision, owner, r.now().Add(leaseDuration),
	)
	if err != nil {
		return nil, fmt.Errorf("reserve explicitly acknowledged Cursor Cloud retry: %w", err)
	}
	dispatchOwner := reservedBinding.DispatchOwner
	if reserved.State == models.ManagedAgentSubmissionReserved &&
		(reservedBinding.DispatchLeaseUntil == nil || !reservedBinding.DispatchLeaseUntil.After(r.now())) {
		reservedBinding, dispatchOwner, err = r.ensureLease(ctx, reservedBinding, reserved)
		if err != nil {
			return nil, err
		}
	}
	err = r.dispatchFollowup(ctx, reservedBinding, reserved, dispatchOwner)
	latest, readErr := r.repository.GetManagedAgentLatestOperation(ctx, binding.ID)
	if readErr != nil {
		return nil, errors.Join(err, readErr)
	}
	if err != nil && !errors.Is(err, provider.ErrOutcomeUnknown) {
		return latest, err
	}
	return latest, err
}

func (r *Runtime) loadUnknownFollowup(ctx context.Context, executionID string) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, Provider, error) {
	binding, err := r.repository.GetManagedAgentBindingByExecution(ctx, executionID)
	if err != nil {
		return nil, nil, nil, err
	}
	operation, err := r.repository.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	if operation.Kind != models.ManagedAgentOperationFollowup ||
		operation.State != models.ManagedAgentSubmissionUnknown && operation.State != models.ManagedAgentSubmissionSubmitting {
		return nil, nil, nil, ErrSubmissionResolutionUnavailable
	}
	client, err := r.clientFactory(ctx, binding)
	if err != nil {
		return nil, nil, nil, err
	}
	return binding, operation, client, nil
}
