package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kandev/kandev/internal/task/models"
)

// MaterializeRepositoryRequest is the credential-free repository checkout
// request accepted by agentctl. It intentionally has no credential field;
// lifecycle wiring supplies Git credentials separately when that is needed.
type MaterializeRepositoryRequest struct {
	RepositoryURL           string                          `json:"repository_url"`
	Destination             string                          `json:"destination"`
	BaseBranch              string                          `json:"base_branch"`
	CheckoutBranch          string                          `json:"checkout_branch,omitempty"`
	RemoteContribution      *models.RemoteContribution      `json:"remote_contribution,omitempty"`
	ContributionDestination *models.ContributionDestination `json:"contribution_destination,omitempty"`
}

// MaterializeRepositoryResponse reports the adopted workspace subdirectory.
type MaterializeRepositoryResponse struct {
	Destination         string `json:"destination"`
	Reused              bool   `json:"reused,omitempty"`
	GitMetadataAttested bool   `json:"git_metadata_attested,omitempty"`
	Error               string `json:"error,omitempty"`
}

// GitMetadataAttestationResponse reports agentctl's final validation of its
// server-owned checkout allowlist. Its executor paths stay on the authenticated
// lifecycle control channel and are never sent to a user-facing error.
type GitMetadataAttestationResponse struct {
	Attested  bool                     `json:"attested"`
	Checkouts []GitMetadataAttestation `json:"checkouts,omitempty"`
	Error     string                   `json:"error,omitempty"`
}

// GitMetadataAttestationRequest narrows a Git metadata proof to checkout
// roots already authorized for ordinary workspace access. This keeps ACP
// directory grants independent from the subset that must be Git checkouts.
type GitMetadataAttestationRequest struct {
	CheckoutRoots []string `json:"checkout_roots,omitempty"`
}

// GitMetadataAttestation is an agentctl-approved executor checkout and its
// regular Git directory. Lifecycle uses only these returned pairs when it
// renders a clone policy.
type GitMetadataAttestation struct {
	CheckoutPath string `json:"checkout_path"`
	GitDir       string `json:"git_dir"`
}

// AttestWorkspaceGitMetadata asks agentctl to validate the regular .git
// directory in its current workspace before lifecycle configures a mutable
// agent. This is the executor-side attestation boundary for clone launches.
func (c *Client) AttestWorkspaceGitMetadata(ctx context.Context, checkoutRoots []string) ([]GitMetadataAttestation, error) {
	var body *bytes.Reader
	if checkoutRoots != nil {
		payload, err := json.Marshal(GitMetadataAttestationRequest{CheckoutRoots: checkoutRoots})
		if err != nil {
			return nil, fmt.Errorf("marshal workspace Git metadata attestation request: %w", err)
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/workspace/attest-git-metadata", body)
	if err != nil {
		return nil, fmt.Errorf("create workspace Git metadata attestation request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send workspace Git metadata attestation request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var response GitMetadataAttestationResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode workspace Git metadata attestation response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || !response.Attested || len(response.Checkouts) == 0 {
		return nil, fmt.Errorf("workspace Git metadata attestation failed")
	}
	return response.Checkouts, nil
}

// RemoveMaterializedRepositoryRequest identifies an owned checkout for
// rollback after a later item in a remote materialization batch fails.
type RemoveMaterializedRepositoryRequest struct {
	RepositoryURL string `json:"repository_url"`
	Destination   string `json:"destination"`
}

type removeMaterializedRepositoryResponse struct {
	Removed bool   `json:"removed"`
	Error   string `json:"error,omitempty"`
}

// WorkspaceMaterializationError preserves an actionable remote status without
// retaining or formatting the repository locator supplied by the caller.
type WorkspaceMaterializationError struct {
	StatusCode int
	Message    string
}

func (e *WorkspaceMaterializationError) Error() string {
	return fmt.Sprintf("workspace repository materialization failed (%d): %s", e.StatusCode, e.Message)
}

// MaterializeRepository asks the live agentctl instance to atomically clone
// and check out a repository under its current workspace root.
func (c *Client) MaterializeRepository(ctx context.Context, materialization MaterializeRepositoryRequest) (*MaterializeRepositoryResponse, error) {
	body, err := json.Marshal(materialization)
	if err != nil {
		return nil, fmt.Errorf("marshal workspace materialization request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/workspace/materialize-repository", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create workspace materialization request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send workspace materialization request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var response MaterializeRepositoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode workspace materialization response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || response.Error != "" {
		message := response.Error
		if message == "" {
			message = "remote agentctl rejected the request"
		}
		return &response, &WorkspaceMaterializationError{StatusCode: resp.StatusCode, Message: message}
	}
	return &response, nil
}

// RemoveMaterializedRepository removes only a checkout whose destination and
// origin match the supplied credential-free request. Nonexistent destinations
// are treated as a successful idempotent rollback by agentctl.
func (c *Client) RemoveMaterializedRepository(ctx context.Context, removal RemoveMaterializedRepositoryRequest) error {
	body, err := json.Marshal(removal)
	if err != nil {
		return fmt.Errorf("marshal workspace cleanup request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/workspace/materialize-repository/remove", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create workspace cleanup request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send workspace cleanup request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var response removeMaterializedRepositoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("decode workspace cleanup response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || response.Error != "" {
		message := response.Error
		if message == "" {
			message = "remote agentctl rejected the cleanup request"
		}
		return &WorkspaceMaterializationError{StatusCode: resp.StatusCode, Message: message}
	}
	return nil
}
