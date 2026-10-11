package backendapp

import (
	"context"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
)

func (a *lifecycleAdapter) AcknowledgeNativeResumeDelivery(ctx context.Context, sessionID string) error {
	return a.mgr.AcknowledgeNativeResumeDelivery(ctx, sessionID)
}

func (a *lifecycleAdapter) ReconcileAlreadyResumedDelivery(ctx context.Context, identity runtimeapi.AgentDeliveryRecoveryIdentity, nativeID string) (bool, error) {
	return a.mgr.ReconcileAlreadyResumedDelivery(ctx, identity, nativeID)
}
