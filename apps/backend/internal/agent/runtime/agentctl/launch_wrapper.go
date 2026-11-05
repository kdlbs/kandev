package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/kandev/kandev/internal/common/launchwrapper"
)

// ErrLaunchWrapperUnsupported prevents old agentctl builds from ignoring a wrapper.
var ErrLaunchWrapperUnsupported = errors.New("agentctl does not support this launch wrapper. Update the execution runtime before retrying")

func (c *Client) requireLaunchWrapper(ctx context.Context, spec launchwrapper.Spec) error {
	if spec.IsZero() {
		return nil
	}
	if err := spec.Validate(); err != nil {
		return err
	}
	return c.checkLaunchWrapperCapability(ctx)
}

func (c *Client) checkLaunchWrapperCapability(ctx context.Context) error {
	return c.checkLaunchCapability(ctx, launchwrapper.Capability, ErrLaunchWrapperUnsupported)
}

func (c *Client) checkLaunchCapability(ctx context.Context, capability string, unsupported error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/identity", nil)
	if err != nil {
		return launchwrapper.ErrContext
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return launchwrapper.ErrContext
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return unsupported
	}
	if resp.StatusCode != http.StatusOK {
		return launchwrapper.ErrContext
	}
	body, err := readResponseBody(resp)
	if err != nil {
		return launchwrapper.ErrContext
	}
	var identity struct {
		Capabilities []string `json:"capabilities"`
	}
	if json.Unmarshal(body, &identity) != nil {
		return launchwrapper.ErrContext
	}
	if !slices.Contains(identity.Capabilities, capability) {
		return unsupported
	}
	return nil
}

// ValidateLaunchWrapper checks peer compatibility before a live process is stopped.
func (c *Client) ValidateLaunchWrapper(ctx context.Context, spec launchwrapper.Spec) error {
	return c.requireLaunchWrapper(ctx, spec)
}

// GetLaunchWrapper reads the protected template without accepting child commands from the peer.
// A false supported value identifies a legacy peer with no wrapper snapshot contract.
func (c *Client) GetLaunchWrapper(ctx context.Context) (launchwrapper.Spec, bool, error) {
	if err := c.checkLaunchWrapperCapability(ctx); err != nil {
		if errors.Is(err, ErrLaunchWrapperUnsupported) {
			return launchwrapper.Spec{}, false, nil
		}
		return launchwrapper.Spec{}, false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/agent/launch-wrapper", nil)
	if err != nil {
		return launchwrapper.Spec{}, true, launchwrapper.ErrContext
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return launchwrapper.Spec{}, true, launchwrapper.ErrContext
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return launchwrapper.Spec{}, true, launchwrapper.ErrContext
	}
	data, err := readResponseBody(resp)
	if err != nil {
		return launchwrapper.Spec{}, true, launchwrapper.ErrContext
	}
	var state struct {
		Version int                `json:"version"`
		Wrapper launchwrapper.Spec `json:"launch_wrapper,omitzero"`
	}
	if json.Unmarshal(data, &state) != nil || state.Version != 1 {
		return launchwrapper.Spec{}, true, launchwrapper.ErrInvalid
	}
	return state.Wrapper, true, nil
}
