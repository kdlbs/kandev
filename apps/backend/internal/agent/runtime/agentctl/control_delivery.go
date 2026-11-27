package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

func (c *ControlClient) ReadRetainedDelivery(ctx context.Context, input journal.RetainedRecoveryRequest) (*journal.RetainedRecoveryResponse, error) {
	var result journal.RetainedRecoveryResponse
	if err := c.postRetainedDelivery(ctx, "/api/v1/delivery/retained", input, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ReadRetainedReconstructionEvidence returns only the bounded candidate
// descriptor. The owner process proof remains absent when it cannot be
// verified.
func (c *ControlClient) ReadRetainedReconstructionEvidence(
	ctx context.Context,
	input journal.RetainedReconstructionEvidenceRequest,
) (*journal.RetainedReconstructionEvidence, error) {
	var result journal.RetainedReconstructionEvidence
	if err := c.postRetainedDelivery(ctx, "/api/v1/delivery/retained/evidence", input, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ReadRetainedReconstructionSubmission fetches exactly one previously
// inspected candidate payload through the authenticated runtime boundary.
func (c *ControlClient) ReadRetainedReconstructionSubmission(
	ctx context.Context,
	input journal.RetainedReconstructionSubmissionRequest,
) (*journal.Submission, error) {
	var result journal.Submission
	if err := c.postRetainedDelivery(ctx, "/api/v1/delivery/retained/submission", input, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *ControlClient) postRetainedDelivery(ctx context.Context, path string, input, output any) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		body, readErr := readResponseBody(response)
		if readErr != nil {
			return readErr
		}
		return &DeliveryHTTPError{StatusCode: response.StatusCode, Body: truncateBody(body)}
	}
	if err = json.NewDecoder(response.Body).Decode(output); err != nil {
		return err
	}
	return nil
}
