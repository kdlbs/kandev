package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// InferenceInventory applies the start gate and enumerates under one server lock.
func (c *ControlClient) InferenceInventory(ctx context.Context, required bool) ([]*InstanceInfo, error) {
	body, err := json.Marshal(struct {
		Required bool `json:"required"`
	}{required})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/inference/inventory", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound && !required {
		return c.ListInstances(ctx)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("inference inventory unavailable (status %d)", resp.StatusCode)
	}
	var result struct {
		Version   int             `json:"version"`
		Required  *bool           `json:"required"`
		Instances []*InstanceInfo `json:"instances"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Version != 1 || result.Required == nil || *result.Required != required || result.Instances == nil {
		return nil, fmt.Errorf("inference inventory scope unavailable")
	}
	return result.Instances, nil
}

func (c *Client) AdmitInferenceCapacityAttempt(ctx context.Context, id string) error {
	_, err := c.postInferenceTokenObservation(ctx, id, "/admit")
	return err
}
