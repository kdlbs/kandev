package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

type nativeResumeRecoveryVerifier interface {
	HasResolvedNativeResumeRecovery(context.Context, models.AgentDeliveryRecovery) (bool, error)
}

type historicalNativeResumeRecoveryVerifier interface {
	HasResolvedHistoricalNativeResumeRecovery(context.Context, models.AgentDeliveryRecovery) (bool, error)
}

type historicalTerminalRecoveryVerifier interface {
	HasResolvedHistoricalTerminalRecovery(context.Context, models.AgentDeliveryRecovery, int64, string) (bool, error)
}
