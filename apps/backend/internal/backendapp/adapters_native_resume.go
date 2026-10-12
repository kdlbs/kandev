package backendapp

import (
	"context"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

func (a *lifecycleAdapter) AcknowledgeNativeResumeDelivery(ctx context.Context, sessionID string) error {
	return a.mgr.AcknowledgeNativeResumeDelivery(ctx, sessionID)
}

func (a *lifecycleAdapter) ReconcileAlreadyResumedDelivery(ctx context.Context, identity runtimeapi.AgentDeliveryRecoveryIdentity, nativeID string) (bool, error) {
	return a.mgr.ReconcileAlreadyResumedDelivery(ctx, identity, nativeID)
}

func (a *lifecycleAdapter) GetNativeResumeDeliveryEvidence(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
	return a.mgr.GetNativeResumeDeliveryEvidence(ctx, sessionID)
}

func (a *lifecycleAdapter) ReconcileLoadedNativeDelivery(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
	return a.mgr.ReconcileLoadedNativeDelivery(ctx, sessionID)
}
