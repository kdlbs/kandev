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

// DeliveryStatus is the authenticated recovery descriptor returned by an
// agentctl owner. It contains bounded identity and retained-work evidence but
// never includes prompt payloads.
type DeliveryStatus = journal.RecoveryDescriptor

// DeliveryHTTPError preserves an HTTP status so recovery can distinguish an
// explicitly unsupported old route from authentication, storage, and
// transport failures.
type DeliveryHTTPError struct {
	StatusCode int
	Body       string
}

func (e *DeliveryHTTPError) Error() string {
	if e == nil {
		return "durable delivery request failed"
	}
	return fmt.Sprintf("durable delivery request failed with status %d: %s", e.StatusCode, e.Body)
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

func (c *Client) setDeliveryStatus(status *DeliveryStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if status == nil {
		c.deliveryStatus = nil
		return
	}
	copy := *status
	if status.Stream != nil {
		stream := *status.Stream
		copy.Stream = &stream
	}
	copy.Submissions = append([]journal.SubmissionSummary(nil), status.Submissions...)
	c.deliveryStatus = &copy
}

// DurableDeliveryCapability returns the initialize-time capability and whether
// the peer sent the field. The presence bit preserves compatibility with older
// agentctl peers that have no durable-delivery protocol at all.
func (c *Client) DurableDeliveryCapability() (journal.StorageCapability, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.durableDelivery == nil && c.deliveryStatus == nil {
		return journal.StorageCapability{}, false
	}
	if c.durableDelivery == nil {
		return c.deliveryStatus.StorageCapability, true
	}
	return journal.StorageCapability{
		Version:    c.durableDelivery.Version,
		Durable:    c.durableDelivery.Durable,
		Unresolved: c.durableDelivery.Unresolved,
		Reason:     c.durableDelivery.Reason,
	}, true
}

// DeliveryRecoveryDescriptor returns the last authenticated recovery
// descriptor discovered for this client.
func (c *Client) DeliveryRecoveryDescriptor() (*DeliveryStatus, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.deliveryStatus == nil {
		return nil, false
	}
	copy := *c.deliveryStatus
	if c.deliveryStatus.Stream != nil {
		stream := *c.deliveryStatus.Stream
		copy.Stream = &stream
	}
	copy.Submissions = append([]journal.SubmissionSummary(nil), c.deliveryStatus.Submissions...)
	return &copy, true
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
	c.setDeliveryStatus(&result)
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

// ListDeliverySubmissions returns the submissions retained for one session.
// It is used only during an explicitly authorized harness-generation change.
func (c *Client) ListDeliverySubmissions(ctx context.Context, sessionID string) ([]journal.Submission, error) {
	var result []journal.Submission
	path := "/api/v1/agent/submissions?" + url.Values{"session_id": []string{sessionID}}.Encode()
	if err := c.doDeliveryRequest(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// RetireDeliverySubmission seals an uncertain submission after an explicit
// recovery has committed a newer harness generation.
func (c *Client) RetireDeliverySubmission(ctx context.Context, id string) error {
	path := "/api/v1/agent/submissions/" + url.PathEscape(id) + "/retire"
	return c.doDeliveryRequest(ctx, http.MethodPost, path, nil, nil)
}

// CancelDeliverySubmission settles a prompt after an explicit user cancel.
// The agentctl peer keeps the terminal record so reconnects cannot resend it.
func (c *Client) CancelDeliverySubmission(ctx context.Context, id string) error {
	path := "/api/v1/agent/submissions/" + url.PathEscape(id) + "/cancel"
	return c.doDeliveryRequest(ctx, http.MethodPost, path, nil, nil)
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
		return &DeliveryHTTPError{StatusCode: resp.StatusCode, Body: truncateBody(responseBody)}
	}
	if output == nil || len(responseBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return fmt.Errorf("parse durable delivery response: %w", err)
	}
	return nil
}
