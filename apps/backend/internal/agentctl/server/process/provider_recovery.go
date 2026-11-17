package process

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/journal"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

// A new manager cannot infer that an old provider scope is safe to reopen.
// Fresh launch configuration must name a newer SQL-allocated journal owner.
func (m *Manager) restoreProviderReplacementAdmission(ctx context.Context) error {
	state, err := m.deliveryJournal.ProviderReplacementRecovery(ctx)
	if err != nil {
		return err
	}
	if state.Required {
		m.requireProviderMCPProtocol()
	}
	receipt := state.Latest
	if state.Active.OperationID != "" {
		receipt = state.Active
	}
	if receipt.OperationID == "" {
		return nil
	}
	if m.freshProviderRecoveryOwner(receipt, state.Latest) {
		return nil
	}
	m.providerAdmission.mu.Lock()
	defer m.providerAdmission.mu.Unlock()
	m.providerAdmission.operationID = receipt.OperationID
	return nil
}

func (m *Manager) freshProviderRecoveryOwner(receipt, latest journal.ProviderReplacementReceipt) bool {
	high := max(receipt.SourceHarnessGeneration, receipt.TargetScope.HarnessGeneration, latest.SourceHarnessGeneration, latest.TargetScope.HarnessGeneration)
	if m.cfg.DeliveryHarnessGeneration <= high || m.cfg.DeliveryStreamID == "" || m.cfg.DeliveryStreamID == receipt.SourceStreamID || m.cfg.DeliveryStreamID == receipt.TargetStreamID {
		return false
	}
	if m.cfg.SessionID != receipt.SessionID || m.cfg.TaskID != receipt.SourceTaskID || m.cfg.ProviderMCP != nil || m.cfg.McpProfile == nil {
		return false
	}
	_, err := p.MarshalBuiltinBinding(*m.cfg.McpProfile)
	return err == nil && m.cfg.McpProfile.BuiltinPackages.TaskID == receipt.SourceTaskID
}
