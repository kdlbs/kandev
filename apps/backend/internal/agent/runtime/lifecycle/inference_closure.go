package lifecycle

import (
	"context"
	"encoding/json"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

const closureStandaloneExecutor = "standalone"

type inferenceClosureSettler interface {
	CaptureInferenceClosureClaims(context.Context, string, string) (json.RawMessage, error)
	SettleInferenceClosure(context.Context, *agentctl.Client, *utility.InferenceClosureResponse, json.RawMessage) error
	VerifyInferenceClosureClaims(context.Context, string, string) error
}

func (m *Manager) closureClient(sid, executionID string) (*agentctl.Client, func(), error) {
	execution, ok := m.executionStore.Get(executionID)
	if !ok || execution.SessionID != sid {
		return nil, nil, utility.ErrInferenceClosureIncomplete
	}
	if execution.ExecutorType != closureStandaloneExecutor {
		return nil, nil, agentctl.ErrInferenceOwnershipUnsupported
	}
	for _, other := range m.ListExecutions() {
		if other.SessionID == sid && other.ID != executionID {
			return nil, nil, utility.ErrInferenceClosureIncomplete
		}
	}
	client, release := execution.AcquireAgentCtlClient()
	if client == nil {
		release()
		return nil, nil, utility.ErrInferenceClosureIncomplete
	}
	return client, release, nil
}

// ProbeInferenceClosure confirms support before any recovery fence is written.
func (m *Manager) ProbeInferenceClosure(ctx context.Context, sid, executionID string) error {
	if _, ok := m.inferenceConsumer.(inferenceClosureSettler); !ok {
		return agentctl.ErrInferenceOwnershipUnsupported
	}
	client, release, err := m.closureClient(sid, executionID)
	if err != nil {
		return err
	}
	defer release()
	_, err = client.InferenceClosure(ctx, utility.InferenceClosureRequest{OperationID: "probe", Action: "probe"})
	return err
}

func (m *Manager) InferenceClosureStep(ctx context.Context, sid, executionID string, req utility.InferenceClosureRequest) (*utility.InferenceClosureResponse, error) {
	client, release, err := m.closureClient(sid, executionID)
	if err != nil {
		return nil, err
	}
	defer release()
	if req.Action == "prepare" && req.ExpectedOwnerID == "" {
		snapshot, readErr := client.InferenceAttempts(ctx)
		if readErr != nil {
			return nil, readErr
		}
		req.ExpectedOwnerID = snapshot.OwnerID
	}
	result, err := client.InferenceClosure(ctx, req)
	if err != nil {
		return nil, err
	}
	c := result.Snapshot.Closure
	if c.SessionID != sid || (c.SourceInstanceID != executionID && c.CandidateInstanceID != executionID) {
		return nil, utility.ErrInferenceClosureIncomplete
	}
	return result, nil
}

func (m *Manager) CaptureInferenceClosureClaims(ctx context.Context, sid, executionID string) (json.RawMessage, error) {
	settler, ok := m.inferenceConsumer.(inferenceClosureSettler)
	if !ok {
		return nil, utility.ErrInferenceClosureIncomplete
	}
	return settler.CaptureInferenceClosureClaims(ctx, sid, executionID)
}

func (m *Manager) SettleInferenceClosure(ctx context.Context, sid, executionID string, observation *utility.InferenceClosureResponse, claims json.RawMessage) error {
	settler, ok := m.inferenceConsumer.(inferenceClosureSettler)
	if !ok {
		return utility.ErrInferenceClosureIncomplete
	}
	client, release, err := m.closureClient(sid, executionID)
	if err != nil {
		return err
	}
	defer release()
	return settler.SettleInferenceClosure(ctx, client, observation, claims)
}

func (m *Manager) VerifyInferenceClosureClaims(ctx context.Context, sid, executionID string) error {
	settler, ok := m.inferenceConsumer.(inferenceClosureSettler)
	if !ok {
		return utility.ErrInferenceClosureIncomplete
	}
	return settler.VerifyInferenceClosureClaims(ctx, sid, executionID)
}
