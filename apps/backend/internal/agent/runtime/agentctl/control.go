// Package agentctl provides a client for communicating with agentctl.
// This file contains the ControlClient for the agentctl control server API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/subproc"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// ControlClient is a client for the agentctl control server API.
// It manages creation and deletion of agent instances.
// Used by both Docker and standalone runtimes.
type ControlClient struct {
	mu         sync.Mutex
	baseURL    string
	httpClient *http.Client
	logger     *logger.Logger
	authToken  string
}

// McpServerConfig holds configuration for an MCP server.
type McpServerConfig struct {
	Name    string            `json:"name"`
	URL     string            `json:"url,omitempty"`
	Type    string            `json:"type,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// CreateInstanceRequest contains the parameters for creating a new agent instance.
type CreateInstanceRequest struct {
	ID                     string              `json:"id,omitempty"`
	WorkspacePath          string              `json:"workspace_path"`
	AgentCommand           string              `json:"agent_command,omitempty"`
	Protocol               string              `json:"protocol,omitempty"`       // Protocol adapter to use (currently "acp")
	AgentType              string              `json:"agent_type,omitempty"`     // Agent type ID for debug file naming (e.g., "codex", "auggie")
	WorkspaceFlag          string              `json:"workspace_flag,omitempty"` // CLI flag for workspace path (e.g., "--workspace-root")
	Env                    map[string]string   `json:"env,omitempty"`
	AutoStart              bool                `json:"auto_start,omitempty"`
	AutoApprovePermissions *bool               `json:"auto_approve_permissions,omitempty"`
	McpServers             []McpServerConfig   `json:"mcp_servers,omitempty"`
	SessionID              string              `json:"session_id,omitempty"`           // Task session ID for MCP tool calls
	TaskID                 string              `json:"task_id,omitempty"`              // Task ID for MCP plan tool calls (server-side injection)
	DisableAskQuestion     bool                `json:"disable_ask_question,omitempty"` // Disable ask_user_question MCP tool (TUI agents)
	AssumeMcpSse           bool                `json:"assume_mcp_sse,omitempty"`       // Assume agent supports SSE MCP servers
	AssumeMcpHttp          bool                `json:"assume_mcp_http,omitempty"`      // Assume agent supports HTTP MCP servers
	McpMode                string              `json:"mcp_mode,omitempty"`             // MCP tool mode: "task" (default), "task-title-pending", "config", "office", or "automation"
	McpProviders           []string            `json:"mcp_providers,omitempty"`        // Supported review-automation providers
	McpProfile             *mcpprofile.Context `json:"mcp_profile,omitempty"`          // Backend-owned typed MCP tool profile
	// RequiresProcessKill tells agentctl to skip the graceful stdin-close wait
	// and reap the agent process group immediately. Required for agents whose
	// runtime keeps child processes (e.g. MCP servers) alive when stdin closes
	// — notably opencode acp.
	RequiresProcessKill bool `json:"requires_process_kill,omitempty"`

	// StripEnv lists environment variables to strip from the agent's child
	// process environment entirely (not just set to empty). Propagated from
	// RuntimeConfig.StripEnv by the lifecycle executors.
	StripEnv []string `json:"strip_env,omitempty"`

	// NamespacesMCPToolsByServer tells the per-instance MCP server to adapt
	// built-in tool names for an agent that appends the server name itself.
	NamespacesMCPToolsByServer bool `json:"namespaces_mcp_tools_by_server,omitempty"`

	// BaseBranches maps RepositoryName → base branch ref for the task's
	// per-repo diff stats. The empty key "" applies to the root /
	// single-repo tracker. Empty map disables the override and falls back
	// to the hardcoded origin/main → master priority list inside agentctl.
	BaseBranches map[string]string `json:"base_branches,omitempty"`
	// RemoteContributions maps an agentctl workspace repository subpath to the
	// server-authored contribution binding for that checkout. The empty key is
	// the workspace root.
	RemoteContributions      map[string]models.RemoteContribution      `json:"remote_contributions,omitempty"`
	ContributionDestinations map[string]models.ContributionDestination `json:"contribution_destinations,omitempty"`
	ComparisonTargets        map[string]models.ComparisonTarget        `json:"comparison_targets,omitempty"`
	// WorkspaceSourceRoots are canonical host roots explicitly attached to the
	// workspace. Agentctl permits file operations through links only beneath
	// these roots.
	WorkspaceSourceRoots []string `json:"workspace_source_roots,omitempty"`
}

// CreateInstanceResponse contains the result of creating a new agent instance.
type CreateInstanceResponse struct {
	ID   string `json:"id"`
	Port int    `json:"port"`
}

// InstanceInfo contains information about an agent instance.
type InstanceInfo struct {
	ID            string            `json:"id"`
	Port          int               `json:"port"`
	Status        string            `json:"status"`
	WorkspacePath string            `json:"workspace_path"`
	AgentCommand  string            `json:"agent_command"`
	Env           map[string]string `json:"env,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	// SessionID is the task session ID this instance was created for, if any.
	SessionID string `json:"session_id,omitempty"`
	// TaskID is the task ID this instance was created for, if any.
	TaskID string `json:"task_id,omitempty"`
	// WorkspaceSourceRoots is the instance's live, current source-root
	// allowlist, read back rather than pushed (AC-EXECUTORS-SURVIVAL-002.14's
	// "workspace source roots" reconstruction row).
	WorkspaceSourceRoots []string `json:"workspace_source_roots,omitempty"`
	// ProviderSessionID is the live agent CLI's own session identity, read
	// back from the adopted instance (AC-EXECUTORS-SURVIVAL-002.14's
	// "provider session identity" reconstruction row).
	ProviderSessionID string `json:"provider_session_id,omitempty"`
}

// ControlClientOption configures optional ControlClient settings.
type ControlClientOption func(*ControlClient)

// WithControlAuthToken sets the Bearer token for authenticating control requests.
func WithControlAuthToken(token string) ControlClientOption {
	return func(c *ControlClient) {
		c.authToken = token
	}
}

// NewControlClient creates a new ControlClient for the agentctl control server.
func NewControlClient(host string, port int, log *logger.Logger, opts ...ControlClientOption) *ControlClient {
	c := &ControlClient{
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: log.WithFields(zap.String("component", "agentctl-control")),
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.authToken != "" {
		c.httpClient.Transport = &authTransport{token: c.authToken}
	}
	return c
}

// AuthToken returns the current auth token. Used to propagate the token
// from the ControlClient to per-instance Clients after a handshake.
func (c *ControlClient) AuthToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.authToken
}

// SetAuthToken sets the Bearer token for future requests.
// Used after a successful Handshake to authenticate subsequent calls.
func (c *ControlClient) SetAuthToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authToken = token
	c.httpClient.Transport = &authTransport{token: token}
}

// Handshake performs the bootstrap handshake with agentctl.
// It sends the one-time nonce and receives the self-generated auth token.
// On success, the token is automatically set for future requests.
func (c *ControlClient) Handshake(ctx context.Context, nonce string) (string, error) {
	body, err := json.Marshal(map[string]string{"nonce": nonce})
	if err != nil {
		return "", fmt.Errorf("failed to marshal handshake request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/auth/handshake", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("handshake failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if decErr := json.NewDecoder(resp.Body).Decode(&errResp); decErr == nil && errResp.Error != "" {
			return "", fmt.Errorf("handshake rejected: %s (status %d)", errResp.Error, resp.StatusCode)
		}
		return "", fmt.Errorf("handshake failed: status %d", resp.StatusCode)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode handshake response: %w", err)
	}
	if result.Token == "" {
		return "", fmt.Errorf("handshake returned empty token")
	}

	// Automatically set the token for future requests
	c.SetAuthToken(result.Token)
	c.logger.Info("bootstrap handshake completed")

	return result.Token, nil
}

// Health checks if the agentctl control server is healthy.
func (c *ControlClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: %d", resp.StatusCode)
	}
	return nil
}

// SubprocessAdmission returns the agentctl process's current Git admission
// state. It is used by the backend debug export to correlate host and agentctl
// pressure without exposing control-server internals directly.
func (c *ControlClient) SubprocessAdmission(ctx context.Context) (subproc.Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/debug/subprocess-admission", nil)
	if err != nil {
		return subproc.Snapshot{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return subproc.Snapshot{}, fmt.Errorf("failed to get subprocess admission: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return subproc.Snapshot{}, fmt.Errorf("failed to get subprocess admission: status %d", resp.StatusCode)
	}

	var snapshot subproc.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return subproc.Snapshot{}, fmt.Errorf("failed to decode subprocess admission: %w", err)
	}
	return snapshot, nil
}

// CreateInstance creates a new agent instance.
func (c *ControlClient) CreateInstance(ctx context.Context, req *CreateInstanceRequest) (*CreateInstanceResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/instances", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("failed to decode error response: %w", err)
		}
		return nil, fmt.Errorf("failed to create instance: %s (status %d)", errResp.Error, resp.StatusCode)
	}

	var result CreateInstanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Info("created agent instance",
		zap.String("instance_id", result.ID),
		zap.Int("port", result.Port))

	return &result, nil
}

// DeleteInstance stops and removes an agent instance.
func (c *ControlClient) DeleteInstance(ctx context.Context, instanceID string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+"/api/v1/instances/"+instanceID, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete instance: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// A retry may observe 404 after agentctl completed the first delete but its
	// response was lost. The desired postcondition is already satisfied.
	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusNoContent &&
		resp.StatusCode != http.StatusNotFound {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return fmt.Errorf("failed to decode error response: %w", err)
		}
		return fmt.Errorf("failed to delete instance: %s (status %d)", errResp.Error, resp.StatusCode)
	}

	c.logger.Info("deleted agent instance", zap.String("instance_id", instanceID))
	return nil
}

// GetInstance gets information about a specific instance.
func (c *ControlClient) GetInstance(ctx context.Context, instanceID string) (*InstanceInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/instances/"+instanceID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("instance %q not found", instanceID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get instance: status %d", resp.StatusCode)
	}

	var info InstanceInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &info, nil
}

// IdentityInfo is the response from GET /identity: installation identity and
// the capability set this control server advertises.
type IdentityInfo struct {
	ServerIdentity string   `json:"server_identity"`
	Capabilities   []string `json:"capabilities"`
	// UnownedPeriodMS is the reporting server's own resolved unowned period
	// in milliseconds (AC-EXECUTORS-CONTROL-OWNERSHIP-003.2/.7) -- the value
	// that server itself enforces, which an adopting backend's local config
	// can disagree with across a restart. Use this, not local config, to
	// compute an adopted server's ownership-renewal cadence.
	UnownedPeriodMS int64 `json:"unowned_period_ms"`
}

// ServerDetails carries the control-server values an adopting backend
// records but that are withheld from the unauthenticated identity endpoint.
type ServerDetails struct {
	HomeDir           string `json:"home_dir"`
	DiagnosticLogPath string `json:"diagnostic_log_path"`
}

// GetIdentity fetches the control server's identity and capability set. It
// deliberately does not require a valid auth token to succeed server-side
// (see agentctl's identity handler): identity retrieval decides adoption
// compatibility, so it cannot itself be gated behind the answer.
func (c *ControlClient) GetIdentity(ctx context.Context) (*IdentityInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/identity", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get identity: status %d", resp.StatusCode)
	}

	var info IdentityInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &info, nil
}

// ErrOwnershipCredentialSuperseded indicates the control server rejected an
// ownership-claim request because the presented credential is no longer
// current (HTTP 401) -- distinct from a transient failure (network error,
// 5xx). This is not the same backend's credential being stale from a
// renewal it initiated: a rotation this backend performs always leaves it
// holding the replacement, so a rejection here means some other party
// rotated the credential out from under it, most likely a second backend
// that adopted the server. Callers must not retry on this error
// (AC-EXECUTORS-CONTROL-OWNERSHIP-002.3).
var ErrOwnershipCredentialSuperseded = errors.New("ownership claim rejected: credential superseded")

// ClaimOwnership establishes or renews this backend's ownership of the
// control server. It carries no instance identity: a server with zero
// instances is still owned. Refused (non-nil error) once the server's
// unowned-shutdown one-way door has fired.
func (c *ControlClient) ClaimOwnership(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/ownership/claim", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to claim ownership: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrOwnershipCredentialSuperseded
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to claim ownership: status %d", resp.StatusCode)
	}
	return nil
}

// CredentialRotationResult is the response from the rotate half of the
// two-phase credential rotation: a rotation identifier from the control
// server's strictly increasing sequence, and the replacement credential to
// authenticate with from now on.
type CredentialRotationResult struct {
	RotationID int64  `json:"rotation_id"`
	Credential string `json:"credential"`
}

// ProveOwnership asks the control server to demonstrate it already holds
// the credential this backend has stored for it, over a challenge generated
// for this attempt. It carries no auth token, because it is what
// establishes that the server is worth sending one to: the answer is a
// keyed digest the server can only produce from the credential, and it
// never carries the credential itself.
func (c *ControlClient) ProveOwnership(ctx context.Context, challenge string) ([]string, error) {
	payload, err := json.Marshal(map[string]string{"challenge": challenge})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/ownership/prove", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request ownership proof: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to request ownership proof: status %d", resp.StatusCode)
	}

	var decoded struct {
		Proofs []string `json:"proofs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return decoded.Proofs, nil
}

// GetServerDetails reads the control-server values withheld from the
// unauthenticated identity endpoint. Requires a valid credential.
func (c *ControlClient) GetServerDetails(ctx context.Context) (*ServerDetails, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/ownership/details", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get server details: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get server details: status %d", resp.StatusCode)
	}

	var details ServerDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &details, nil
}

// RotateCredential presents this client's current auth token (via the
// normal Authorization header) and asks the control server to replace it.
// The response's replacement credential authenticates every operation
// immediately; the presented (now superseded) credential remains valid for
// a further rotation attempt or the ownership-shutdown operation only,
// until ConfirmCredentialRotation is called. Idempotent under retry: a
// retry presenting the same (superseded) credential while unconfirmed
// returns the same rotation again rather than allocating a new one.
func (c *ControlClient) RotateCredential(ctx context.Context) (*CredentialRotationResult, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/ownership/rotate", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to rotate credential: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to rotate credential: status %d", resp.StatusCode)
	}

	var result CredentialRotationResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// ConfirmCredentialRotation names rotationID as durably stored (both the
// replacement credential and the control-server record's reference to it).
// Call this only after both writes have durably completed; sending it
// earlier and then failing can strand the server unadoptable until its
// unowned period elapses (AC-EXECUTORS-CONTROL-OWNERSHIP-002.7).
func (c *ControlClient) ConfirmCredentialRotation(ctx context.Context, rotationID int64) error {
	body, err := json.Marshal(struct {
		RotationID int64 `json:"rotation_id"`
	}{RotationID: rotationID})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/ownership/confirm", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to confirm credential rotation: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to confirm credential rotation: status %d", resp.StatusCode)
	}
	return nil
}

// ShutdownControlServer invokes the ownership-shutdown operation: the
// control server stops every instance it supervises, together with their
// agent subprocesses, and exits. It requires no prior adoption or rotation
// and is authenticated by any credential in the acceptable set, including a
// superseded one that has not yet been confirmed away.
func (c *ControlClient) ShutdownControlServer(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/ownership/shutdown", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to shut down control server: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to shut down control server: status %d", resp.StatusCode)
	}
	return nil
}

// ListInstances lists all running agent instances.
func (c *ControlClient) ListInstances(ctx context.Context) ([]*InstanceInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/instances", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list instances: status %d", resp.StatusCode)
	}

	var result struct {
		Instances []*InstanceInfo `json:"instances"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result.Instances, nil
}

// TurnOutcome is a retained terminal turn outcome for one instance
// (AC-EXECUTORS-SURVIVAL-004.1): the turn identifier the control server
// assigned, and the terminal event itself.
type TurnOutcome struct {
	TurnID int64              `json:"turn_id"`
	Event  streams.AgentEvent `json:"event"`
}

// GetTurnOutcome retrieves the named instance's retained last terminal turn
// outcome, if any. Returns (nil, nil) when the instance exists but nothing
// is retained -- the AC-EXECUTORS-SURVIVAL-004.5 case that publishes the
// session as running. The read is repeatable: it never discards what it
// returns (AC-EXECUTORS-SURVIVAL-004.6); only AckTurnOutcome does.
func (c *ControlClient) GetTurnOutcome(ctx context.Context, instanceID string) (*TurnOutcome, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/instances/"+instanceID+"/turn-outcome", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get turn outcome: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("instance %q not found", instanceID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get turn outcome: status %d", resp.StatusCode)
	}

	var body struct {
		Retained bool               `json:"retained"`
		TurnID   int64              `json:"turn_id"`
		Event    streams.AgentEvent `json:"event"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	if !body.Retained {
		return nil, nil
	}
	return &TurnOutcome{TurnID: body.TurnID, Event: body.Event}, nil
}

// AckTurnOutcome acknowledges the named instance's retained outcome by turn
// identifier, discarding it. Naming an identifier the control server no
// longer holds, or never held, is accepted and changes nothing
// (AC-EXECUTORS-SURVIVAL-004.6), so a retried acknowledgement is always safe.
func (c *ControlClient) AckTurnOutcome(ctx context.Context, instanceID string, turnID int64) error {
	body, err := json.Marshal(struct {
		TurnID int64 `json:"turn_id"`
	}{TurnID: turnID})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/instances/"+instanceID+"/turn-outcome/ack", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to ack turn outcome: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to ack turn outcome: status %d", resp.StatusCode)
	}
	return nil
}
