package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	nativeResumeProjectionBudget = 10 * time.Second
	nativeResumeProjectionPoll   = 200 * time.Millisecond
)

var (
	errNativeResumeProjectionLag  = fmt.Errorf("%w: native resume projection is incomplete", ErrDurableAdoptionBlocked)
	errNativeResumeStreamProgress = fmt.Errorf("%w: native resume stream advanced", ErrDeliveryRecordEvidenceStale)
)

type nativeResumeProofOwner struct {
	execution                                    *AgentExecution
	client                                       *agentctl.Client
	sessionID, nativeID, incarnationID, streamID string
	generation                                   uint64
}

func captureNativeResumeProofOwner(execution *AgentExecution, client *agentctl.Client) nativeResumeProofOwner {
	return nativeResumeProofOwner{
		execution: execution, client: client, sessionID: execution.SessionID,
		nativeID: execution.ACPSessionID, incarnationID: execution.DeliveryIncarnationID,
		streamID: execution.DeliveryStreamID, generation: execution.DeliveryHarnessGeneration,
	}
}

func (m *Manager) ownsNativeResumeProof(owner nativeResumeProofOwner) bool {
	current, exists := m.GetExecutionBySessionID(owner.sessionID)
	if !exists || current != owner.execution || !m.runtimeExecutionCurrent(current) {
		return false
	}
	// The caller holds the client lease throughout this bounded proof.
	return current.currentAgentCtlClient() == owner.client && current.ACPSessionID == owner.nativeID &&
		current.DeliveryIncarnationID == owner.incarnationID &&
		current.DeliveryStreamID == owner.streamID && current.DeliveryHarnessGeneration == owner.generation
}

func (m *Manager) nativeResumeOwnedProof(ctx context.Context, owner nativeResumeProofOwner) (*models.NativeResumeDeliveryEvidence, error) {
	client, release := owner.execution.AcquireAgentCtlClient()
	defer release()
	if client != owner.client || !m.ownsNativeResumeProof(owner) {
		return nil, ErrDurableAdoptionIdentityMismatch
	}
	proof, err := m.nativeResumeDeliveryProof(ctx, owner.execution, client)
	if !m.ownsNativeResumeProof(owner) {
		return nil, ErrDurableAdoptionIdentityMismatch
	}
	return proof, err
}

func (m *Manager) waitForNativeResumeProjection(ctx context.Context, owner nativeResumeProofOwner) (*models.NativeResumeDeliveryEvidence, error) {
	waiting := false
	for {
		if ctx.Err() != nil {
			return nil, errors.Join(errNativeResumeProjectionLag, ctx.Err())
		}
		proof, err := m.nativeResumeOwnedProof(ctx, owner)
		if ctx.Err() != nil {
			return nil, errors.Join(errNativeResumeProjectionLag, ctx.Err())
		}
		lag := errors.Is(err, errNativeResumeProjectionLag)
		if !lag && (!waiting || !errors.Is(err, errNativeResumeStreamProgress)) {
			return proof, err
		}
		if !waiting {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, nativeResumeProjectionBudget)
			defer cancel()
			waiting = true
		}
		timer := time.NewTimer(nativeResumeProjectionPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, errors.Join(errNativeResumeProjectionLag, ctx.Err())
		case <-timer.C:
		}
	}
}

func nativeResumeStreamAdvanced(before, after *agentctl.DeliveryStatus) bool {
	if before == nil || after == nil || before.Stream == nil || after.Stream == nil ||
		!sameDeliveryRecordDescriptor(*before, *after) || before.StreamID != after.StreamID ||
		before.Unresolved || after.Unresolved || validateRecoveredStream(after) != nil {
		return false
	}
	previous, current := *before.Stream, *after.Stream
	if current.HighWater < previous.HighWater || current.Acknowledged < previous.Acknowledged ||
		current.FirstRetained < previous.FirstRetained || current.Acknowledged > current.HighWater {
		return false
	}
	previous.HighWater, previous.Acknowledged = current.HighWater, current.Acknowledged
	previous.FirstRetained, previous.Bytes = current.FirstRetained, current.Bytes
	return previous == current
}
