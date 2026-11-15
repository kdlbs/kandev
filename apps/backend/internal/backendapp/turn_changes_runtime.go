package backendapp

import (
	"context"
	"errors"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/system/toolretention"
	"github.com/kandev/kandev/internal/task/changes"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type runtimeTurnChangeCaptureHandler struct {
	coordinator *changes.Coordinator
	publisher   interface {
		PublishTurnChangeSummary(context.Context, string, string, string) error
	}
}

func (h *runtimeTurnChangeCaptureHandler) AdmitTurnChanges(
	ctx context.Context,
	admission agentruntime.TurnChangeAdmission,
	client agentruntime.TurnChangeCheckpointClient,
) error {
	admitErr := h.coordinator.Admit(ctx, changes.Admission{
		TaskID: admission.TaskID, SessionID: admission.SessionID,
		TaskEnvironmentID: admission.TaskEnvironmentID, TurnID: admission.TurnID,
		ExecutionID: admission.ExecutionID, StartupAttemptID: admission.StartupAttemptID,
		PromptGeneration: admission.PromptGeneration, ExecutionProfileID: admission.ExecutionProfileID,
		RouteGeneration: admission.RouteGeneration, Checkouts: runtimeTurnChangeCheckouts(admission.Checkouts),
	}, client)
	if h.publisher == nil {
		return admitErr
	}
	publishErr := h.publisher.PublishTurnChangeSummary(ctx, admission.TaskID, admission.SessionID, changes.ChangeSetIDForTurn(admission.TurnID))
	return errors.Join(admitErr, publishErr)
}

func (h *runtimeTurnChangeCaptureHandler) FinishTurnChanges(
	ctx context.Context,
	terminal agentruntime.TurnChangeTerminal,
	client agentruntime.TurnChangeCheckpointClient,
) error {
	finishErr := h.coordinator.CaptureTerminalEndpoints(ctx, coordinatorTurnChangeTerminal(terminal), client)
	if finishErr != nil {
		// A terminal whose executor deadline expired can still be settled from
		// the persisted claim and any accepted endpoint. Retry once so a bounded
		// capture failure becomes a durable unavailable result before the
		// lifecycle releases its generation fence.
		finishErr = h.coordinator.CaptureTerminalEndpoints(ctx, coordinatorTurnChangeTerminal(terminal), client)
	}
	return finishErr
}

func (h *runtimeTurnChangeCaptureHandler) ProcessTurnChanges(
	ctx context.Context,
	terminal agentruntime.TurnChangeTerminal,
	client agentruntime.TurnChangeCheckpointClient,
) error {
	coordinatorTerminal := coordinatorTurnChangeTerminal(terminal)
	if err := h.coordinator.ProcessTerminalComparison(ctx, coordinatorTerminal, client); err != nil {
		return err
	}
	if err := h.publishTurnChangeSummary(ctx, terminal); err != nil {
		return err
	}
	if err := h.coordinator.ProcessTerminalContent(ctx, coordinatorTerminal, client); err != nil {
		return err
	}
	return h.publishTurnChangeSummary(ctx, terminal)
}

func (h *runtimeTurnChangeCaptureHandler) publishTurnChangeSummary(ctx context.Context, terminal agentruntime.TurnChangeTerminal) error {
	if h.publisher == nil {
		return nil
	}
	err := h.publisher.PublishTurnChangeSummary(ctx, terminal.TaskID, terminal.SessionID, changes.ChangeSetIDForTurn(terminal.TurnID))
	// Uncaptured or removed turns have no summary to publish.
	if errors.Is(err, repoerrors.ErrTurnChangeSetNotFound) {
		return nil
	}
	return err
}

func coordinatorTurnChangeTerminal(terminal agentruntime.TurnChangeTerminal) changes.Terminal {
	return changes.Terminal{
		Admission: changes.Admission{
			TaskID: terminal.TaskID, SessionID: terminal.SessionID,
			TaskEnvironmentID: terminal.TaskEnvironmentID, TurnID: terminal.TurnID,
			ExecutionID: terminal.ExecutionID, StartupAttemptID: terminal.StartupAttemptID,
			PromptGeneration: terminal.PromptGeneration, ExecutionProfileID: terminal.ExecutionProfileID,
			RouteGeneration: terminal.RouteGeneration, Checkouts: runtimeTurnChangeCheckouts(terminal.Checkouts),
		},
		At: terminal.At, Outcome: terminal.Outcome,
		FinalAssistantMessageID: terminal.FinalAssistantMessageID,
	}
}

func runtimeTurnChangeCheckouts(checkouts []agentruntime.TurnChangeCheckout) []changes.Checkout {
	result := make([]changes.Checkout, 0, len(checkouts))
	for _, checkout := range checkouts {
		result = append(result, changes.Checkout{
			ID: checkout.ID, EnvironmentRepoID: checkout.EnvironmentRepoID,
			TaskRepositoryID: checkout.TaskRepositoryID, RepositoryID: checkout.RepositoryID,
			WorktreeID: checkout.WorktreeID, DisplayName: checkout.DisplayName,
			RepositorySubpath:      checkout.RepositorySubpath,
			RepositorySubpathKnown: checkout.RepositorySubpathKnown,
		})
	}
	return result
}

var _ agentruntime.TurnChangeCaptureHandler = (*runtimeTurnChangeCaptureHandler)(nil)
var _ agentruntime.TurnChangeSummaryProcessor = (*runtimeTurnChangeCaptureHandler)(nil)

// historyCompactorAdapter maps the task-owned history operations onto the
// shared system-page compaction contract without importing each other's types.
type historyCompactorAdapter struct {
	service *changes.HistoryService
}

func (a historyCompactorAdapter) Analyze(ctx context.Context, cutoff time.Time) (toolretention.HistoryStats, error) {
	if a.service == nil {
		return toolretention.HistoryStats{}, nil
	}
	inventory, err := a.service.Analyze(ctx, cutoff)
	if err != nil {
		return toolretention.HistoryStats{}, err
	}
	return toolretention.HistoryStats{
		Tasks: inventory.Tasks, Snapshots: inventory.Snapshots,
		LegacySQLBytes: inventory.LegacySQLBytes, ArchiveBytes: inventory.ArchiveBytes,
		Complete: inventory.Complete,
	}, nil
}

func (a historyCompactorAdapter) HasWork(ctx context.Context, cutoff time.Time) (bool, error) {
	if a.service == nil {
		return false, nil
	}
	return a.service.HasWork(ctx, cutoff)
}

func (a historyCompactorAdapter) CompactOnce(ctx context.Context, cutoff time.Time, limit int) (toolretention.HistoryStats, error) {
	if a.service == nil {
		return toolretention.HistoryStats{}, nil
	}
	result, err := a.service.CompactOnce(ctx, cutoff, "", limit)
	if err != nil {
		return toolretention.HistoryStats{}, err
	}
	return toolretention.HistoryStats{
		ExpiredChangeSets: result.ExpiredChangeSets, DeletedContents: result.DeletedContents,
		FreedPayloadBytes: result.FreedPayloadBytes, DeletedArchiveRefs: result.DeletedArchiveRefs,
		ReclaimedBytes: result.ReclaimedBytes, ArchiveFailures: result.ArchiveFailures, Remaining: result.RemainingCandidates,
	}, nil
}
