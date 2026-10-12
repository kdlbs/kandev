package lifecycle

import (
	"context"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

func retireInterruptedDeliverySubmission(ctx context.Context, client *agentctl.Client, execution *AgentExecution) error {
	if client == nil || execution == nil || execution.RequiredNativeConversationID == "" || execution.InterruptedSubmissionID == "" || execution.InterruptedHarnessGeneration+1 != execution.DeliveryHarnessGeneration {
		return ErrDeliveryRecoveryBlocked
	}
	return client.RetireInterruptedDeliverySubmission(ctx, execution.InterruptedSubmissionID, execution.InterruptedStreamID, execution.InterruptedHarnessGeneration)
}
