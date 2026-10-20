package orchestrator

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (s *Service) configureInitialDeliveryAdmission(session *models.TaskSession, options startCreatedSessionOptions, launchOptions *executor.LaunchOptions) {
	launchOptions.DeliverySubmissionID = options.deliverySubmissionID
	deliveryOptions := promptTaskOptions{
		deliveryProtocol: options.deliveryProtocol, deliverySubmissionID: options.deliverySubmissionID,
		deliveryPayloadHash: options.deliveryPayloadHash, deliveryClaimUpdater: options.deliveryClaimUpdater,
	}
	// Executor binds these options before starting the goroutine that admits the prompt.
	launchOptions.BeforeAgentStart = func(admissionCtx context.Context, executionID, _ string) error {
		var bindErr error
		deliveryOptions, bindErr = s.bindInitialAgentDeliverySubmission(admissionCtx, session, executionID, deliveryOptions)
		return bindErr
	}
	validateAdmission := launchOptions.BeforeInitialPromptDispatch
	launchOptions.BeforeInitialPromptDispatch = func(admissionCtx context.Context, executionID, finalPrompt string, finalAttachments []v1.MessageAttachment) error {
		if validateAdmission != nil {
			if err := validateAdmission(admissionCtx, executionID, finalPrompt, finalAttachments); err != nil {
				if options.onInitialPromptAdmissionFailed != nil {
					options.onInitialPromptAdmissionFailed(err)
				}
				return err
			}
		}
		_, admissionErr := s.prepareAgentDeliverySubmission(admissionCtx, session, executionID, finalPrompt, finalAttachments, deliveryOptions)
		if admissionErr != nil {
			if options.onInitialPromptAdmissionFailed != nil {
				options.onInitialPromptAdmissionFailed(admissionErr)
			}
			return &agentruntime.PromptAdmissionPersistenceError{Err: admissionErr}
		}
		return nil
	}
}

func (s *Service) dispatchPreparedPrompt(
	ctx context.Context, taskID string, session *models.TaskSession, executionID, prompt string,
	attachments []v1.MessageAttachment, dispatchOnly bool, options promptTaskOptions, submissionID string,
	validateAdmission func() error, onDispatched func(),
) (*executor.PromptResult, *agentDeliverySubmissionRuntime, error) {
	var delivery *agentDeliverySubmissionRuntime
	beforeAdmission := func(admissionCtx context.Context, finalPrompt string, finalAttachments []v1.MessageAttachment) error {
		if validateAdmission != nil {
			if err := validateAdmission(); err != nil {
				return err
			}
		}
		var admissionErr error
		delivery, admissionErr = s.prepareAgentDeliverySubmission(
			admissionCtx, session, executionID, finalPrompt, finalAttachments, options,
		)
		if admissionErr != nil {
			return markPromptAdmissionRejected(admissionErr)
		}
		if delivery != nil {
			delivery.dispatchOnly = dispatchOnly
		}
		return nil
	}
	if options.deliveryProtocol == messagequeue.DeliveryProtocolLegacy && validateAdmission == nil && options.deliveryClaimUpdater == nil {
		beforeAdmission = nil
	}
	result, execErr := s.executor.PromptWithAdmissionCallbackAndSubmissionID(
		ctx, taskID, session.ID, prompt, attachments, dispatchOnly,
		beforeAdmission, onDispatched, submissionID, session,
	)
	return result, delivery, execErr
}
