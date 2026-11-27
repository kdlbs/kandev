package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/orchestrator"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (w *orchestratorWrapper) PromptTaskWithDeliverySubmissionID(
	ctx context.Context,
	taskID, taskSessionID, prompt, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	submissionID string,
	accepted ...orchestrator.DirectPromptStartOptions,
) (*orchestrator.PromptResult, error) {
	return w.svc.PromptTaskWithDeliverySubmissionID(
		ctx, taskID, taskSessionID, prompt, model, planMode, attachments, dispatchOnly, submissionID, accepted...,
	)
}
