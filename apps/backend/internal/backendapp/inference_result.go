package backendapp

import (
	"errors"
	"strings"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

func (b *sharedInferenceConsumer) finishInferencePrompt(client *agentruntime.InferenceClient, binding sessioncapacity.InferenceBinding, response *utility.PromptResponse, err error) (*utility.PromptResponse, error) {
	proof := b.settleInferenceResponse(client, binding, response, err)
	if inferenceFailureNeedsSettlement(response, err) && proof == nil {
		return response, unconfirmedInferenceFailure(response, err)
	}
	if inferenceFailureNeedsSettlement(response, err) && proof != nil {
		if err == nil {
			err = errors.New("inference returned no successful result")
			if response != nil && response.Error != "" {
				err = errors.New(response.Error)
			}
		}
		return response, &agentruntime.InferenceAttemptError{AttemptID: binding.AttemptID, Attempt: proof, Cause: err}
	}
	return response, err
}

func inferenceFailureNeedsSettlement(response *utility.PromptResponse, err error) bool {
	return err != nil || response == nil || (!response.Success && strings.TrimSpace(response.Response) == "")
}

func unconfirmedInferenceFailure(response *utility.PromptResponse, cause error) error {
	if cause == nil {
		message := "inference returned no successful result"
		if response != nil && response.Error != "" {
			message = response.Error
		}
		cause = errors.New(message)
	}
	return &agentruntime.InferenceCapacityError{Reason: "child_settlement_unknown", Cause: cause}
}
