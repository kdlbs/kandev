package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/sandbox"
)

var ErrSandboxLaunchUnsupported = errors.New("agentctl does not support sandbox launch. Update the execution runtime before retrying")

func (c *Client) requireSandbox(ctx context.Context, launch *sandbox.Launch) error {
	if launch == nil {
		return nil
	}
	if err := launch.Validate(); err != nil {
		return err
	}
	return c.checkLaunchCapability(ctx, sandbox.LaunchCapability, ErrSandboxLaunchUnsupported)
}

// ConfigureAgentWithSandbox sends a captured sandbox and complete environment atomically.
func (c *Client) ConfigureAgentWithSandbox(ctx context.Context, command string, args []string, env map[string]string, continueCommand string, continueArgs []string, launch *sandbox.Launch, wrapper launchwrapper.Spec) error {
	return c.configureAgent(ctx, command, args, env, continueCommand, continueArgs, true, launch, wrapper)
}

// SandboxOwner returns retained spawn identity from the authenticated execution host.
func (c *Client) SandboxOwner(ctx context.Context, attemptID string, generation uint64, digest string) (*sandbox.ProcessOwner, error) {
	if err := c.checkLaunchCapability(ctx, sandbox.LaunchCapability, ErrSandboxLaunchUnsupported); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/agent/sandbox/owner", nil)
	if err != nil {
		return nil, ErrSandboxLaunchUnsupported
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, ErrSandboxLaunchUnsupported
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, ErrSandboxLaunchUnsupported
	}
	body, err := readResponseBody(response)
	if err != nil {
		return nil, ErrSandboxLaunchUnsupported
	}
	var owner sandbox.ProcessOwner
	if json.Unmarshal(body, &owner) != nil || owner.AttemptID != attemptID || owner.Generation != generation || owner.PolicyDigest != digest || owner.Supervisor.Validate() != nil || owner.Supervisor.SessionID != owner.Supervisor.PID || owner.Supervisor.GroupID != owner.Supervisor.PID {
		return nil, ErrSandboxLaunchUnsupported
	}
	return &owner, nil
}

// SetSandboxOwner binds a local PTY's captured process to the configured instance.
func (c *Client) SetSandboxOwner(ctx context.Context, owner *sandbox.ProcessOwner) error {
	if owner == nil {
		return ErrSandboxLaunchUnsupported
	}
	if err := c.checkLaunchCapability(ctx, sandbox.LaunchCapability, ErrSandboxLaunchUnsupported); err != nil {
		return err
	}
	body, err := json.Marshal(owner)
	if err != nil {
		return ErrSandboxLaunchUnsupported
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/api/v1/agent/sandbox/owner", bytes.NewReader(body))
	if err != nil {
		return ErrSandboxLaunchUnsupported
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return ErrSandboxLaunchUnsupported
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNoContent {
		return ErrSandboxLaunchUnsupported
	}
	return nil
}

// ValidateSandboxLaunch checks the envelope and peer capability before dispatch intent.
func (c *Client) ValidateSandboxLaunch(ctx context.Context, launch *sandbox.Launch) error {
	return c.requireSandbox(ctx, launch)
}
