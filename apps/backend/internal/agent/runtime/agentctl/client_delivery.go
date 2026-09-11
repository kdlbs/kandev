package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

// DeliveryStatus is the agentctl delivery capability and, when requested,
// the current stream watermark.
type DeliveryStatus struct {
	journal.StorageCapability
	Stream *journal.Stream `json:"stream,omitempty"`
}

// DurableDeliveryInfo is the initialize-time transport capability returned by
// agentctl. A missing value means the peer predates the durable-delivery
// response field and must be treated as a legacy peer.
type DurableDeliveryInfo struct {
	Version    uint32 `json:"version"`
	Durable    bool   `json:"durable"`
	Unresolved bool   `json:"unresolved,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

func (c *Client) setDurableDelivery(info *DurableDeliveryInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if info == nil {
		c.durableDelivery = nil
		return
	}
	copy := *info
	c.durableDelivery = &copy
}

// DurableDeliveryCapability returns the initialize-time capability and whether
// the peer sent the field. The presence bit preserves compatibility with older
// agentctl peers that have no durable-delivery protocol at all.
func (c *Client) DurableDeliveryCapability() (journal.StorageCapability, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.durableDelivery == nil {
		return journal.StorageCapability{}, false
	}
	return journal.StorageCapability{
		Version:    c.durableDelivery.Version,
		Durable:    c.durableDelivery.Durable,
		Unresolved: c.durableDelivery.Unresolved,
		Reason:     c.durableDelivery.Reason,
	}, true
}

func (c *Client) setLastDeliverySubmissionID(id string) {
	c.mu.Lock()
	c.lastDeliverySubmissionID = id
	c.mu.Unlock()
}

// LastDeliverySubmissionID returns the most recent accepted prompt identity.
// The value is advisory and must be reconciled through GetDeliverySubmission
// before any retry decision.
func (c *Client) LastDeliverySubmissionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastDeliverySubmissionID
}

// GetDeliveryStatus returns the retained-storage capability advertised by the
// agentctl instance. A legacy instance is a successful response with Durable
// false, not a storage error.
func (c *Client) GetDeliveryStatus(ctx context.Context, streamID string) (*DeliveryStatus, error) {
	path := "/api/v1/agent/delivery"
	if streamID != "" {
		path += "?" + url.Values{"stream_id": []string{streamID}}.Encode()
	}
	var result DeliveryStatus
	if err := c.doDeliveryRequest(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SubmitDelivery records and accepts one immutable submission. The caller
// must reconcile an accepted or dispatching record before retrying it.
func (c *Client) SubmitDelivery(ctx context.Context, submission journal.Submission) (*journal.Submission, error) {
	var result journal.Submission
	if err := c.doDeliveryRequest(ctx, http.MethodPost, "/api/v1/agent/submissions", submission, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetDeliverySubmission returns the durable state of one submission.
func (c *Client) GetDeliverySubmission(ctx context.Context, id string) (*journal.Submission, error) {
	var result journal.Submission
	path := "/api/v1/agent/submissions/" + url.PathEscape(id)
	if err := c.doDeliveryRequest(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ReplayDelivery reads events after a committed cursor. Cursor expiration is
// returned as an error so the caller can perform the explicit recovery path.
func (c *Client) ReplayDelivery(ctx context.Context, streamID string, after uint64, limit int) ([]journal.Event, journal.Stream, error) {
	query := url.Values{
		"stream_id": []string{streamID},
		"after":     []string{strconv.FormatUint(after, 10)},
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	var result struct {
		Events []journal.Event `json:"events"`
		Stream journal.Stream  `json:"stream"`
	}
	if err := c.doDeliveryRequest(ctx, http.MethodGet, "/api/v1/agent/delivery/stream?"+query.Encode(), nil, &result); err != nil {
		return nil, journal.Stream{}, err
	}
	return result.Events, result.Stream, nil
}

// AcknowledgeDelivery commits the highest event safely received by the
// backend. It does not imply that the canonical projection has completed.
func (c *Client) AcknowledgeDelivery(ctx context.Context, streamID string, sequence uint64) error {
	return c.doDeliveryRequest(ctx, http.MethodPost, "/api/v1/agent/delivery/stream/ack", struct {
		StreamID string `json:"stream_id"`
		Sequence uint64 `json:"sequence"`
	}{StreamID: streamID, Sequence: sequence}, nil)
}

func (c *Client) doDeliveryRequest(ctx context.Context, method, path string, input, output any) error {
	var body *bytes.Reader
	if input == nil {
		body = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("marshal durable delivery request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := readResponseBody(resp)
	if err != nil {
		return fmt.Errorf("read durable delivery response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("durable delivery request failed with status %d: %s", resp.StatusCode, truncateBody(responseBody))
	}
	if output == nil || len(responseBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return fmt.Errorf("parse durable delivery response: %w", err)
	}
	return nil
}
