package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

func (c *ControlClient) ReadRetainedDelivery(ctx context.Context, input journal.RetainedRecoveryRequest) (*journal.RetainedRecoveryResponse, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/delivery/retained", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		body, readErr := readResponseBody(response)
		if readErr != nil {
			return nil, readErr
		}
		return nil, &DeliveryHTTPError{StatusCode: response.StatusCode, Body: truncateBody(body)}
	}
	var result journal.RetainedRecoveryResponse
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
