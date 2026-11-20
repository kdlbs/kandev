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

func (m *Manager) abandonOrphanSuccessorStreams(
	ctx context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
	repository AgentDeliveryRepository,
) error {
	reader, ok := repository.(harnessGenerationReader)
	if !ok || execution == nil || client == nil {
		return nil
	}
	current, err := readNativeResumeCommittedGeneration(ctx, reader, execution)
	if err != nil {
		return err
	}
	if current == nil {
		return nil
	}
	streams, err := client.ListSuccessorDeliveryStreams(ctx)
	if err != nil {
		return fmt.Errorf("list successor delivery streams before native Resume: %w", err)
	}
	for _, stream := range streams {
		if err := m.abandonOrphanSuccessorStream(ctx, reader, repository, execution, client, current, stream); err != nil {
			return err
		}
	}
	return nil
}

func readNativeResumeCommittedGeneration(
	ctx context.Context,
	reader harnessGenerationReader,
	execution *AgentExecution,
) (*models.HarnessSessionGeneration, error) {
	current, err := reader.GetCurrentHarnessSessionGeneration(ctx, execution.SessionID, execution.DeliveryIncarnationID)
	if errors.Is(err, models.ErrTaskSessionNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read committed generation before native Resume: %w", err)
	}
	if current == nil || current.SessionID != execution.SessionID ||
		current.IncarnationID != execution.DeliveryIncarnationID ||
		uint64(current.Generation) != execution.DeliveryHarnessGeneration ||
		current.NativeSessionID != execution.ACPSessionID {
		return nil, nil
	}
	return current, nil
}

func (m *Manager) abandonOrphanSuccessorStream(
	ctx context.Context,
	reader harnessGenerationReader,
	repository AgentDeliveryRepository,
	execution *AgentExecution,
	client *agentctl.Client,
	current *models.HarnessSessionGeneration,
	stream journal.Stream,
) error {
	if !isUncommittedSuccessorStream(stream, execution, current) {
		return nil
	}
	if err := nativeResumeOrphanStreamHasNoSQLAuthority(ctx, reader, repository, execution, stream); err != nil {
		if errors.Is(err, errNativeResumeOrphanStreamHasAuthority) {
			return nil
		}
		return err
	}
	if err := client.AbandonSuccessorDeliveryStream(ctx, stream); err != nil {
		return fmt.Errorf("abandon orphan successor stream %q before native Resume: %w", stream.StreamID, err)
	}
	return nil
}

func isUncommittedSuccessorStream(stream journal.Stream, execution *AgentExecution, current *models.HarnessSessionGeneration) bool {
	return stream.StreamID != "" && stream.SessionID == execution.SessionID &&
		stream.IncarnationID == execution.DeliveryIncarnationID &&
		stream.HarnessGeneration > uint64(current.Generation) && !stream.Sealed &&
		stream.HighWater > stream.Acknowledged
}

var errNativeResumeOrphanStreamHasAuthority = errors.New("successor stream has SQL authority")

func nativeResumeOrphanStreamHasNoSQLAuthority(
	ctx context.Context,
	reader harnessGenerationReader,
	repository AgentDeliveryRepository,
	execution *AgentExecution,
	stream journal.Stream,
) error {
	current, err := reader.GetCurrentHarnessSessionGeneration(ctx, execution.SessionID, execution.DeliveryIncarnationID)
	if err != nil {
		return fmt.Errorf("recheck committed generation for successor stream %q: %w", stream.StreamID, err)
	}
	if current == nil || current.SessionID != execution.SessionID ||
		current.IncarnationID != execution.DeliveryIncarnationID ||
		uint64(current.Generation) != execution.DeliveryHarnessGeneration ||
		uint64(current.Generation) >= stream.HarnessGeneration || current.NativeSessionID != execution.ACPSessionID {
		return errNativeResumeOrphanStreamHasAuthority
	}
	if _, err := repository.GetAgentDeliveryCursor(ctx, stream.StreamID); err == nil {
		return errNativeResumeOrphanStreamHasAuthority
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read SQL cursor for successor stream %q: %w", stream.StreamID, err)
	}
	return nil
}

func (m *Manager) abandonFailedSuccessorBootstrapStream(ctx context.Context, execution *AgentExecution) error {
	if execution == nil || !execution.replacesPredecessorGeneration() ||
		execution.DeliveryHarnessGeneration < 2 || m.streamManager == nil {
		return nil
	}
	repository := m.streamManager.deliveryRepository()
	reader, ok := repository.(harnessGenerationReader)
	if !ok {
		return nil
	}
	current, err := reader.GetCurrentHarnessSessionGeneration(ctx, execution.SessionID, execution.DeliveryIncarnationID)
	if err != nil {
		if errors.Is(err, models.ErrTaskSessionNotFound) {
			return nil
		}
		return fmt.Errorf("read committed generation before failed bootstrap cleanup: %w", err)
	}
	if current == nil || current.SessionID != execution.SessionID ||
		current.IncarnationID != execution.DeliveryIncarnationID ||
		uint64(current.Generation)+1 != execution.DeliveryHarnessGeneration {
		return nil
	}
	if _, err := repository.GetAgentDeliveryCursor(ctx, execution.DeliveryStreamID); err == nil {
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read SQL cursor before failed bootstrap cleanup: %w", err)
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return nil
	}
	return client.AbandonFailedBootstrapDeliveryStream(ctx, execution.DeliveryStreamID, execution.DeliveryHarnessGeneration)
}
