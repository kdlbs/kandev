package lifecycle

import (
	"context"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"

	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// ProviderTurnRequest contains execution-owned identity, never prompt payloads.
type ProviderTurnRequest struct {
	Population      func() []ProviderTurnRequest
	RecordUnmetered func()
	SessionID       string
	ExecutionID     string
	ProfileID       string
	AgentID         string
	ExecutorType    string
	Model           string
	Environment     map[string]string
	Generation      uint64
	SubmissionID    string
}

type ProviderTurnSettlement interface {
	FinishProviderTurn(context.Context, string, uint64, bool) error
	CompleteProviderExecution(context.Context, string) error
}

type ProviderTurnAdmission interface {
	ProviderTurnSettlement
	AdmitProviderTurn(context.Context, ProviderTurnRequest) (bool, error)
}

// SetProviderTurnAdmission is wired only for the enabled native usage feature.
func (m *Manager) SetProviderTurnAdmission(admission ProviderTurnAdmission) {
	m.sessionManager.providerTurnAdmission = admission
}

// SetProviderTurnSettlement preserves verified settlement across feature changes.
// It is installed before native runtime recovery and never admits new claims.
func (m *Manager) SetProviderTurnSettlement(settler ProviderTurnSettlement) {
	m.sessionManager.providerTurnSettler = settler
}

func (sm *SessionManager) providerTurnSettlement() ProviderTurnSettlement {
	if sm.providerTurnSettler != nil {
		return sm.providerTurnSettler
	}
	return sm.providerTurnAdmission
}

func (sm *SessionManager) admitProviderTurn(ctx context.Context, execution *AgentExecution, generation uint64, submission string) (bool, error) {
	if sm.providerTurnAdmission == nil {
		return false, nil
	}
	req := providerTurnRequest(execution, generation, submission)
	req.RecordUnmetered = func() { execution.providerTurnUnmeteredGeneration.Store(generation) }
	req.Population = func() []ProviderTurnRequest { return sm.providerTurnPopulation(execution.ID, generation) }
	return sm.providerTurnAdmission.AdmitProviderTurn(ctx, req)
}

func providerTurnRequest(execution *AgentExecution, generation uint64, submission string) ProviderTurnRequest {
	model := ""
	if state := execution.GetModelState(); state != nil {
		model = state.CurrentModelID
	}
	return ProviderTurnRequest{
		SessionID:   execution.SessionID,
		ExecutionID: execution.ID, ProfileID: execution.AgentProfileID, AgentID: execution.AgentID,
		ExecutorType: execution.ExecutorType, Model: model, Environment: execution.RuntimeEnvironment(),
		Generation: generation, SubmissionID: submission,
	}
}

// The store snapshot is read only at admission, including pre-policy turns.
func (sm *SessionManager) providerTurnPopulation(current string, generation uint64) []ProviderTurnRequest {
	if sm.executionStore == nil {
		return nil
	}
	var result []ProviderTurnRequest
	for _, execution := range sm.executionStore.List() {
		snapshot, ok := sm.executionStore.promptLifecycleSnapshot(execution.ID)
		if !ok || snapshot.execution != execution || execution.ID == current && snapshot.generation == generation {
			continue
		}
		active := snapshot.generation != 0 && snapshot.completedGeneration != snapshot.generation &&
			(snapshot.dispatchedGeneration == snapshot.generation || execution.providerTurnUnmeteredGeneration.Load() == snapshot.generation)
		unknownActive := snapshot.generation == 0 && sm.providerExecutionRunning(execution.ID)
		if !active && !unknownActive {
			continue
		}
		result = append(result, providerTurnRequest(execution, snapshot.generation, execution.deliverySubmissionIDSnapshot()))
	}
	return result
}

func (sm *SessionManager) providerExecutionRunning(id string) bool {
	running := false
	_ = sm.executionStore.WithLock(id, func(execution *AgentExecution) { running = execution.Status == v1.AgentStatusRunning })
	return running
}

func (sm *SessionManager) finishProviderTurn(execution string, generation uint64, refund bool) {
	settler := sm.providerTurnSettlement()
	if settler == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := settler.FinishProviderTurn(ctx, execution, generation, refund); err != nil {
		sm.logger.Error("provider turn claim settlement failed", zap.Error(err))
	}
}

func (m *Manager) completeProviderExecution(ctx context.Context, execution string) error {
	settler := m.sessionManager.providerTurnSettlement()
	if settler == nil {
		return nil
	}
	settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return settler.CompleteProviderExecution(settlementCtx, execution)
}

// Local delivery failures can finish the UI prompt without confirming a remote turn.
func (m *Manager) settleConfirmedProviderCompletion(execution *AgentExecution, event *agentctl.AgentEvent, confirmed, claimed bool) {
	if !confirmed || event.PromptGeneration == 0 {
		return
	}
	if !claimed {
		snapshot, exists := m.executionStore.promptLifecycleSnapshot(execution.ID)
		if !exists || snapshot.execution != execution || snapshot.generation != event.PromptGeneration ||
			snapshot.completedGeneration != event.PromptGeneration {
			return
		}
	}
	m.sessionManager.finishProviderTurn(execution.ID, event.PromptGeneration, false)
}
