package client

import (
	"context"
	"io"
	"net/http"

	"github.com/kandev/kandev/internal/common/logger"
)

type runtimeBindingGuard struct {
	owner   *RuntimeOwner
	binding *runtimeBinding
}

func (lease *RuntimeLease) bindingGuard() *runtimeBindingGuard {
	if lease == nil || lease.owner == nil || lease.binding == nil {
		return nil
	}
	return &runtimeBindingGuard{owner: lease.owner, binding: lease.binding}
}

func (guard *runtimeBindingGuard) current() bool {
	if guard == nil || guard.owner == nil || guard.binding == nil || guard.binding.ctx.Err() != nil {
		return false
	}
	guard.owner.mu.RLock()
	defer guard.owner.mu.RUnlock()
	return !guard.owner.stopping && guard.owner.active == guard.binding &&
		guard.owner.snapshot.Status == AvailabilityStatusAvailable
}

func (guard *runtimeBindingGuard) checkCurrent() error {
	if !guard.current() {
		return ErrRuntimeLeaseRetired
	}
	return nil
}

func (guard *runtimeBindingGuard) bindContext(ctx context.Context) (context.Context, func(), error) {
	if guard == nil {
		if ctx == nil {
			ctx = context.Background()
		}
		return ctx, func() {}, nil
	}
	if err := guard.checkCurrent(); err != nil {
		return nil, func() {}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	boundCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(guard.binding.ctx, cancel)
	if err := guard.checkCurrent(); err != nil {
		stop()
		cancel()
		return nil, func() {}, err
	}
	return boundCtx, func() {
		stop()
		cancel()
	}, nil
}

type runtimeBindingTransport struct {
	guard *runtimeBindingGuard
	base  http.RoundTripper
}

func (transport *runtimeBindingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := transport.guard.checkCurrent(); err != nil {
		return nil, err
	}
	ctx, cancel, err := transport.guard.bindContext(request.Context())
	if err != nil {
		return nil, err
	}
	base := transport.base
	if base == nil {
		base = http.DefaultTransport
	}
	response, err := base.RoundTrip(request.Clone(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	if err := transport.guard.checkCurrent(); err != nil {
		_ = response.Body.Close()
		cancel()
		return nil, err
	}
	if response.Body == nil {
		cancel()
		return response, nil
	}
	response.Body = &runtimeBoundResponseBody{
		ReadCloser: response.Body,
		guard:      transport.guard,
		cancel:     cancel,
	}
	return response, nil
}

type runtimeBoundResponseBody struct {
	io.ReadCloser
	guard  *runtimeBindingGuard
	cancel func()
}

func (body *runtimeBoundResponseBody) Read(p []byte) (int, error) {
	if err := body.guard.checkCurrent(); err != nil {
		_ = body.Close()
		return 0, err
	}
	return body.ReadCloser.Read(p)
}

func (body *runtimeBoundResponseBody) Close() error {
	err := body.ReadCloser.Close()
	body.cancel()
	return err
}

// WithRuntimeLease binds every HTTP request and websocket opened by the client
// to the runtime generation represented by lease. It does not retain the
// caller's lease context; the connection remains valid until that generation
// is retired.
func WithRuntimeLease(lease *RuntimeLease) ClientOption {
	return func(c *Client) {
		c.runtimeGuard = lease.bindingGuard()
	}
}

func WithControlRuntimeLease(lease *RuntimeLease) ControlClientOption {
	return func(c *ControlClient) {
		c.runtimeGuard = lease.bindingGuard()
	}
}

func newRuntimeBoundControlClient(lease *RuntimeLease, log *logger.Logger) *ControlClient {
	if lease == nil || !lease.IsCurrent() {
		return nil
	}
	return NewControlClient(lease.binding.host, lease.binding.port, log,
		WithControlAuthToken(lease.binding.credential), WithControlRuntimeLease(lease))
}
