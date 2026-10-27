package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

const expectedTokenFingerprintHeader = "X-Expected-Token-Fingerprint"

// CreateConflictError reports a 409 from instance creation: the ID is already
// held (instance_exists) or still being torn down (instance_cleaning_up).
type CreateConflictError struct{ Code string }

func (e *CreateConflictError) Error() string {
	return "create instance refused: " + e.Code
}

func isCreateConflictCode(code string) bool {
	return code == "instance_exists" || code == "instance_cleaning_up"
}

// StatusError reports a non-2xx response from an identity or revoke request.
type StatusError struct {
	Op     string
	Status int
}

func (e *StatusError) Error() string { return fmt.Sprintf("%s: status %d", e.Op, e.Status) }

// StatusOf returns the HTTP status carried by err, or 0 when err has none.
func StatusOf(err error) int {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Status
	}
	return 0
}

// ErrIdentityMalformed marks a 200 identity response whose body cannot be decoded.
var ErrIdentityMalformed = errors.New("instance identity malformed")

type expectedFingerprintKey struct{}

// WithExpectedTokenFingerprint makes any DeleteInstance issued with the
// returned context carry the delete precondition for fingerprint.
func WithExpectedTokenFingerprint(ctx context.Context, fingerprint string) context.Context {
	return context.WithValue(ctx, expectedFingerprintKey{}, fingerprint)
}

func ExpectedTokenFingerprintFrom(ctx context.Context) string {
	fp, _ := ctx.Value(expectedFingerprintKey{}).(string)
	return fp
}

// InstanceIdentity is the per-instance /identity payload.
type InstanceIdentity struct {
	Capabilities              []string `json:"capabilities"`
	MCPTokenFingerprint       string   `json:"mcp_token_fingerprint"`
	BrokerUpstreamFingerprint string   `json:"broker_upstream_fingerprint"`
}

// HasCapability reports whether the instance advertises capability.
func (i *InstanceIdentity) HasCapability(capability string) bool {
	for _, c := range i.Capabilities {
		if c == capability {
			return true
		}
	}
	return false
}

// GetInstanceIdentity reads the instance's capability and fingerprint report.
func (c *Client) GetInstanceIdentity(ctx context.Context) (*InstanceIdentity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/identity", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("instance identity: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{Op: "instance identity", Status: resp.StatusCode}
	}
	var id InstanceIdentity
	if err := json.NewDecoder(resp.Body).Decode(&id); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIdentityMalformed, err)
	}
	return &id, nil
}

// DeleteInstanceMatching deletes the instance only while it still holds the
// token with the given fingerprint. An empty fingerprint deletes
// unconditionally.
func (c *ControlClient) DeleteInstanceMatching(ctx context.Context, instanceID, tokenFingerprint string) error {
	return c.DeleteInstance(WithExpectedTokenFingerprint(ctx, tokenFingerprint), instanceID)
}

// RevokeMCPToken clears the instance's in-memory MCP token. A missing instance
// is reported as a 404 StatusError.
func (c *ControlClient) RevokeMCPToken(ctx context.Context, instanceID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/instances/"+instanceID+"/mcp-token/revoke", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("revoke mcp token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &StatusError{Op: "revoke mcp token", Status: resp.StatusCode}
	}
	return nil
}
