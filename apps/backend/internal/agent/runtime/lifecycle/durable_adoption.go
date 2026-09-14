package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

var (
	// ErrDurableAdoptionBlocked identifies recovery evidence that is incomplete
	// or contradictory. The caller must keep the existing recovery guard and
	// use its normal bounded stop path.
	ErrDurableAdoptionBlocked = errors.New("durable adoption blocked")
	// ErrDurableAdoptionIdentityMismatch identifies a descriptor that does not
	// belong to the SQL harness generation being recovered.
	ErrDurableAdoptionIdentityMismatch = errors.New("durable adoption owner identity mismatch")
	// ErrDurableAdoptionEvidenceUnavailable identifies a failed required read.
	ErrDurableAdoptionEvidenceUnavailable = errors.New("durable adoption evidence unavailable")
)

const initialPromptSubmissionPrefix = "prompt:initial:"

func initialPromptSubmissionID(sessionID string, generation uint64) string {
	if sessionID == "" {
		return ""
	}
	if generation == 0 {
		generation = 1
	}
	return fmt.Sprintf("%s%s:%d", initialPromptSubmissionPrefix, sessionID, generation)
}

func initialPromptDeliverySubmissionID(execution *AgentExecution) string {
	if execution == nil || execution.SessionID == "" {
		return ""
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return ""
	}
	capability, advertised := client.DurableDeliveryCapability()
	if !advertised || !capability.Durable || capability.Version != journal.CurrentVersion || capability.Unresolved {
		return ""
	}
	return initialPromptSubmissionID(execution.SessionID, execution.DeliveryHarnessGeneration)
}

type agentDeliverySubmissionReader interface {
	GetAgentDeliverySubmission(context.Context, string) (*models.AgentDeliverySubmission, error)
}

type agentDeliverySubmissionLister interface {
	ListAgentDeliverySubmissions(context.Context, string) ([]*models.AgentDeliverySubmission, error)
}

func (m *Manager) restoreRecoveredDelivery(
	ctx context.Context,
	execution *AgentExecution,
	ri *ExecutorInstance,
) error {
	if execution == nil || ri == nil {
		return fmt.Errorf("%w: recovered execution is incomplete", ErrDurableAdoptionBlocked)
	}
	if ri.DeliveryRecoveryError != nil {
		return fmt.Errorf("%w: %w: %v", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable, ri.DeliveryRecoveryError)
	}
	if ri.DeliveryStatus == nil {
		if !ri.DeliveryLegacyEvidence {
			return nil
		}
		return m.restoreLegacyDelivery(ctx, execution)
	}

	status := ri.DeliveryStatus
	if err := validateRecoveredDeliveryStatus(execution, status); err != nil {
		return err
	}
	delivery := m.streamManager.deliveryRepository()
	generation, err := loadRecoveredHarnessGeneration(ctx, delivery, execution.SessionID, status.IncarnationID)
	if err != nil {
		return err
	}
	if err := validateRecoveredHarnessGeneration(execution, ri, status, generation); err != nil {
		return err
	}
	if err := validateRecoveredStream(status); err != nil {
		return err
	}
	cursor, err := adoptionReplayCursor(ctx, delivery, status.StreamID, status.Stream)
	if err != nil {
		return fmt.Errorf("%w: %w: %v", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable, err)
	}
	if err := validateRecoveredCursor(cursor, status.Stream); err != nil {
		return err
	}

	applyRecoveredDeliveryIdentity(execution, status, generation, cursor)

	return m.restoreRecoveredSubmission(ctx, execution, status, delivery)
}

func validateRecoveredDeliveryStatus(execution *AgentExecution, status *agentctl.DeliveryStatus) error {
	if status == nil || !status.Durable || status.Version != journal.CurrentVersion {
		return fmt.Errorf("%w: peer did not advertise compatible retained storage", ErrDurableAdoptionBlocked)
	}
	if status.SessionID == "" || status.IncarnationID == "" || status.HarnessGeneration == 0 || status.StreamID == "" {
		return fmt.Errorf("%w: %w: delivery descriptor is missing owner identity", ErrDurableAdoptionBlocked, ErrDurableAdoptionIdentityMismatch)
	}
	if status.SessionID != execution.SessionID {
		return fmt.Errorf("%w: %w: descriptor session %q, execution session %q", ErrDurableAdoptionBlocked, ErrDurableAdoptionIdentityMismatch, status.SessionID, execution.SessionID)
	}
	return nil
}

func loadRecoveredHarnessGeneration(
	ctx context.Context,
	delivery AgentDeliveryRepository,
	sessionID, incarnationID string,
) (*models.HarnessSessionGeneration, error) {
	if delivery == nil {
		return nil, fmt.Errorf("%w: %w: backend delivery repository is not configured", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable)
	}
	reader, ok := delivery.(harnessGenerationReader)
	if !ok {
		return nil, fmt.Errorf("%w: %w: backend harness-generation reader is not configured", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable)
	}
	generation, err := reader.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnationID)
	if err == nil {
		return generation, nil
	}
	if errors.Is(err, models.ErrTaskSessionNotFound) {
		return nil, fmt.Errorf("%w: %w: SQL owner generation is absent", ErrDurableAdoptionBlocked, ErrDurableAdoptionIdentityMismatch)
	}
	return nil, fmt.Errorf("%w: %w: load SQL owner generation: %v", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable, err)
}

func validateRecoveredHarnessGeneration(
	execution *AgentExecution,
	ri *ExecutorInstance,
	status *agentctl.DeliveryStatus,
	generation *models.HarnessSessionGeneration,
) error {
	if generation == nil || generation.SessionID != execution.SessionID ||
		generation.IncarnationID != status.IncarnationID ||
		generation.Generation <= 0 || uint64(generation.Generation) != status.HarnessGeneration {
		return fmt.Errorf("%w: %w: descriptor generation does not match SQL", ErrDurableAdoptionBlocked, ErrDurableAdoptionIdentityMismatch)
	}
	if ri.ProviderSessionID != "" && generation.NativeSessionID != "" && ri.ProviderSessionID != generation.NativeSessionID {
		return fmt.Errorf("%w: %w: adopted native session does not match SQL", ErrDurableAdoptionBlocked, ErrDurableAdoptionIdentityMismatch)
	}
	return nil
}

func validateRecoveredStream(status *agentctl.DeliveryStatus) error {
	stream := status.Stream
	if stream == nil {
		return nil
	}
	if stream.StreamID != status.StreamID || stream.SessionID != status.SessionID ||
		stream.IncarnationID != status.IncarnationID || stream.HarnessGeneration != status.HarnessGeneration {
		return fmt.Errorf("%w: %w: retained stream owner does not match descriptor", ErrDurableAdoptionBlocked, ErrDurableAdoptionIdentityMismatch)
	}
	return nil
}

func validateRecoveredCursor(cursor uint64, stream *journal.Stream) error {
	if stream != nil && stream.FirstRetained > 0 && cursor != ^uint64(0) && cursor+1 < stream.FirstRetained {
		return fmt.Errorf("%w: retained stream no longer covers the SQL projected cursor", ErrDurableAdoptionBlocked)
	}
	return nil
}

func applyRecoveredDeliveryIdentity(
	execution *AgentExecution,
	status *agentctl.DeliveryStatus,
	generation *models.HarnessSessionGeneration,
	cursor uint64,
) {
	// These fields are committed only after the descriptor, SQL owner, stream,
	// and cursor all agree. No ACP operation is needed to reconstruct them.
	execution.DeliveryStreamID = status.StreamID
	execution.DeliveryIncarnationID = status.IncarnationID
	execution.DeliveryHarnessGeneration = status.HarnessGeneration
	execution.DeliveryMode = DurableDeliveryV1
	execution.DeliveryDescriptor = cloneDeliveryStatus(status)
	execution.DeliveryReplayCursor = cursor
	if generation.OriginalWorkspace != "" {
		execution.OriginalWorkspacePath = generation.OriginalWorkspace
	}
}

func (m *Manager) restoreLegacyDelivery(ctx context.Context, execution *AgentExecution) error {
	delivery := m.streamManager.deliveryRepository()
	if delivery == nil {
		return fmt.Errorf("%w: %w: backend delivery repository is not configured", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable)
	}
	lister, ok := delivery.(agentDeliverySubmissionLister)
	if !ok {
		return fmt.Errorf("%w: %w: SQL submission listing is not configured", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable)
	}
	submissions, err := lister.ListAgentDeliverySubmissions(ctx, execution.SessionID)
	if err != nil {
		return fmt.Errorf("%w: %w: list SQL submissions: %v", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable, err)
	}
	for _, submission := range submissions {
		if submission != nil && deliverySubmissionNeedsRecovery(submission.State) {
			return fmt.Errorf("%w: SQL contains unresolved v1 submission %q", ErrDurableAdoptionBlocked, submission.ID)
		}
	}
	execution.DeliveryMode = DurableDeliveryLegacy
	return nil
}

func (m *Manager) restoreRecoveredSubmission(
	ctx context.Context,
	execution *AgentExecution,
	status *agentctl.DeliveryStatus,
	delivery AgentDeliveryRepository,
) error {
	if status.SubmissionsTruncated {
		return fmt.Errorf("%w: retained submission evidence is truncated", ErrDurableAdoptionBlocked)
	}
	activePeer, err := activePeerSubmissions(execution, status)
	if err != nil {
		return err
	}
	activeBackend, err := activeBackendSubmissions(ctx, delivery, execution.SessionID)
	if err != nil {
		return err
	}
	if len(activePeer) == 1 && len(activeBackend) == 0 && isInitialPromptSubmission(execution, activePeer[0]) {
		if _, ok := delivery.(agentDeliverySubmissionLister); ok {
			// The lifecycle bootstrap prompt is dispatched before the orchestrator
			// has an interactive submission row. Its deterministic identity still
			// gives Stop and replay one exact peer-owned target; arbitrary peer
			// submissions without SQL evidence remain blocked below.
			execution.setDeliverySubmissionID(activePeer[0].ID)
			return nil
		}
	}
	if err := compareActiveSubmissions(activePeer, activeBackend); err != nil {
		return err
	}
	if len(activePeer) == 0 {
		return nil
	}
	return m.restoreActiveSubmission(ctx, execution, status, delivery, activePeer[0])
}

func isInitialPromptSubmission(execution *AgentExecution, submission journal.SubmissionSummary) bool {
	if execution == nil {
		return false
	}
	return submission.ID == initialPromptSubmissionID(execution.SessionID, execution.DeliveryHarnessGeneration) &&
		submission.HarnessGeneration == execution.DeliveryHarnessGeneration
}

func activePeerSubmissions(execution *AgentExecution, status *agentctl.DeliveryStatus) ([]journal.SubmissionSummary, error) {
	var activePeer []journal.SubmissionSummary
	for _, submission := range status.Submissions {
		if submission.SessionID != execution.SessionID ||
			submission.IncarnationID != status.IncarnationID {
			return nil, fmt.Errorf("%w: %w: submission %q has a foreign owner", ErrDurableAdoptionBlocked, ErrDurableAdoptionIdentityMismatch, submission.ID)
		}
		if deliverySubmissionNeedsRecovery(models.DeliverySubmissionState(submission.State)) {
			activePeer = append(activePeer, submission)
		}
	}
	if len(activePeer) > 1 {
		return nil, fmt.Errorf("%w: multiple active peer submissions are possible", ErrDurableAdoptionBlocked)
	}
	return activePeer, nil
}

func activeBackendSubmissions(
	ctx context.Context,
	delivery AgentDeliveryRepository,
	sessionID string,
) ([]*models.AgentDeliverySubmission, error) {
	lister, ok := delivery.(agentDeliverySubmissionLister)
	if !ok {
		return nil, nil
	}
	submissions, err := lister.ListAgentDeliverySubmissions(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w: list SQL submissions: %v", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable, err)
	}
	active := make([]*models.AgentDeliverySubmission, 0, len(submissions))
	for _, submission := range submissions {
		if submission != nil && deliverySubmissionNeedsRecovery(submission.State) {
			active = append(active, submission)
		}
	}
	return active, nil
}

func compareActiveSubmissions(
	activePeer []journal.SubmissionSummary,
	activeBackend []*models.AgentDeliverySubmission,
) error {
	if len(activeBackend) > 1 || len(activeBackend) != len(activePeer) {
		return fmt.Errorf("%w: peer and SQL active submission evidence disagree", ErrDurableAdoptionBlocked)
	}
	if len(activeBackend) == 1 && activeBackend[0].ID != activePeer[0].ID {
		return fmt.Errorf("%w: peer and SQL identify different active submissions", ErrDurableAdoptionBlocked)
	}
	return nil
}

func (m *Manager) restoreActiveSubmission(
	ctx context.Context,
	execution *AgentExecution,
	status *agentctl.DeliveryStatus,
	delivery AgentDeliveryRepository,
	peer journal.SubmissionSummary,
) error {
	reader, ok := delivery.(agentDeliverySubmissionReader)
	if !ok {
		return fmt.Errorf("%w: %w: SQL submission reader is not configured", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable)
	}
	submission, err := reader.GetAgentDeliverySubmission(ctx, peer.ID)
	if err != nil {
		return fmt.Errorf("%w: %w: load active submission %q: %v", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable, peer.ID, err)
	}
	if submission == nil || submission.ID != peer.ID ||
		submission.SessionID != execution.SessionID ||
		submission.IncarnationID != status.IncarnationID ||
		submission.HarnessGeneration != int64(status.HarnessGeneration) ||
		!deliverySubmissionNeedsRecovery(submission.State) {
		return fmt.Errorf("%w: %w: active submission %q does not match SQL", ErrDurableAdoptionBlocked, ErrDurableAdoptionIdentityMismatch, peer.ID)
	}
	execution.setDeliverySubmissionID(submission.ID)
	return nil
}

func adoptionReplayCursor(
	ctx context.Context,
	delivery AgentDeliveryRepository,
	streamID string,
	stream *journal.Stream,
) (uint64, error) {
	cursor, err := delivery.GetAgentDeliveryCursor(ctx, streamID)
	if errors.Is(err, sql.ErrNoRows) {
		if stream == nil || stream.FirstRetained <= 1 {
			return 0, nil
		}
		return 0, fmt.Errorf("projected cursor is absent after retained sequence %d", stream.FirstRetained)
	}
	if err != nil {
		return 0, err
	}
	if cursor == nil || cursor.StreamID != "" && cursor.StreamID != streamID {
		return 0, errors.New("projected cursor is unavailable")
	}
	if cursor.ProjectedSequence < 0 {
		return 0, errors.New("projected cursor is negative")
	}
	value := uint64(cursor.ProjectedSequence)
	if stream == nil && value != 0 {
		return 0, errors.New("projected cursor exists without a retained stream")
	}
	if stream != nil && value > stream.HighWater {
		return 0, errors.New("projected cursor is ahead of retained stream")
	}
	return value, nil
}

func deliverySubmissionNeedsRecovery(state models.DeliverySubmissionState) bool {
	switch state {
	case models.DeliverySubmissionPrepared,
		models.DeliverySubmissionAccepted,
		models.DeliverySubmissionDispatching,
		models.DeliverySubmissionInterruptedUnknown:
		return true
	default:
		return false
	}
}

func cloneDeliveryStatus(status *agentctl.DeliveryStatus) *agentctl.DeliveryStatus {
	if status == nil {
		return nil
	}
	copy := *status
	if status.Stream != nil {
		stream := *status.Stream
		copy.Stream = &stream
	}
	copy.Submissions = append([]journal.SubmissionSummary(nil), status.Submissions...)
	return &copy
}
