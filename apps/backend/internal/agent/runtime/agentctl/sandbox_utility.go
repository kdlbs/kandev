package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/kandev/kandev/internal/common/sandbox"
)

// WithSandboxOwner binds an admitted utility launch before the execution host
// starts ACP communication. Dispatch must honor context cancellation.
func (c *Client) WithSandboxOwner(parent context.Context, launch *sandbox.Launch, bind func(context.Context, *sandbox.ProcessOwner) error, dispatch func(context.Context) error) error {
	if launch == nil {
		return dispatch(parent)
	}
	if bind == nil {
		return ErrSandboxLaunchUnsupported
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan error, 1)
	go func() { done <- dispatch(ctx) }()
	received := false
	defer func() {
		cancel()
		if !received {
			<-done
		}
	}()
	handshake, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	acknowledged := false
	for !acknowledged {
		select {
		case err := <-done:
			received = true
			return errors.Join(ErrSandboxLaunchUnsupported, err)
		case <-handshake.Done():
			return handshake.Err()
		case <-ticker.C:
			owner, err := c.utilitySandboxOwner(handshake, launch)
			if err != nil {
				return err
			}
			if owner == nil || bind(handshake, owner) != nil {
				continue
			}
			if err = c.acknowledgeUtilitySandbox(handshake, owner); err != nil {
				return err
			}
			acknowledged = true
		}
	}
	select {
	case err := <-done:
		received = true
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) utilitySandboxOwner(ctx context.Context, launch *sandbox.Launch) (*sandbox.ProcessOwner, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.utilitySandboxOwnerURL(launch.AttemptID), nil)
	if err != nil {
		return nil, ErrSandboxLaunchUnsupported
	}
	response, err := c.ProxyTransport().RoundTrip(request)
	if err != nil {
		return nil, ErrSandboxLaunchUnsupported
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, ErrSandboxLaunchUnsupported
	}
	var owner sandbox.ProcessOwner
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(raw) > 4096 || json.Unmarshal(raw, &owner) != nil {
		return nil, ErrSandboxLaunchUnsupported
	}
	if owner.AttemptID != launch.AttemptID || owner.Generation != launch.Generation || owner.PolicyDigest != launch.PolicyDigest || owner.Supervisor.Validate() != nil || owner.Supervisor.SessionID != owner.Supervisor.PID || owner.Supervisor.GroupID != owner.Supervisor.PID {
		return nil, ErrSandboxLaunchUnsupported
	}
	return &owner, nil
}

func (c *Client) acknowledgeUtilitySandbox(ctx context.Context, owner *sandbox.ProcessOwner) error {
	body, err := json.Marshal(owner)
	if err != nil {
		return ErrSandboxLaunchUnsupported
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, c.utilitySandboxOwnerURL(owner.AttemptID), bytes.NewReader(body))
	if err != nil {
		return ErrSandboxLaunchUnsupported
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.ProxyTransport().RoundTrip(request)
	if err != nil {
		return ErrSandboxLaunchUnsupported
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNoContent {
		return ErrSandboxLaunchUnsupported
	}
	return nil
}

func (c *Client) utilitySandboxOwnerURL(attemptID string) string {
	return c.baseURL + "/api/v1/inference/sandbox/" + url.PathEscape(attemptID) + "/owner"
}
