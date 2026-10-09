package backendapp

import "context"

func (a *lifecycleAdapter) AcknowledgeNativeResumeDelivery(ctx context.Context, sessionID string) error {
	return a.mgr.AcknowledgeNativeResumeDelivery(ctx, sessionID)
}
