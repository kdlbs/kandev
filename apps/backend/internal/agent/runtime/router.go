package runtime

import (
	"context"
	"errors"
	"fmt"
)

// ExecutionRuntimeResolver returns the durable runtime owner for a persisted
// execution ID. ErrNotFound selects the default runtime; every other error
// fails closed so a managed execution cannot fall through to local state.
type ExecutionRuntimeResolver interface {
	RuntimeForExecution(context.Context, string) (string, error)
}

type ExecutionRuntimeResolverFunc func(context.Context, string) (string, error)

func (f ExecutionRuntimeResolverFunc) RuntimeForExecution(ctx context.Context, executionID string) (string, error) {
	return f(ctx, executionID)
}

// Router dispatches new executions by their selected runtime and existing
// executions by durable identity. The default runtime remains the legacy
// lifecycle facade, preserving behavior for every existing executor.
type Router struct {
	defaultRuntime Runtime
	runtimes       map[string]Runtime
	resolver       ExecutionRuntimeResolver
}

func NewRouter(defaultRuntime Runtime, resolver ExecutionRuntimeResolver, runtimes map[string]Runtime) (*Router, error) {
	if defaultRuntime == nil || resolver == nil {
		return nil, errors.New("runtime router requires a default runtime and execution resolver")
	}
	copy := make(map[string]Runtime, len(runtimes))
	for name, implementation := range runtimes {
		if name == "" || implementation == nil {
			return nil, errors.New("runtime router contains an invalid runtime")
		}
		copy[name] = implementation
	}
	return &Router{defaultRuntime: defaultRuntime, resolver: resolver, runtimes: copy}, nil
}

func (r *Router) Launch(ctx context.Context, spec LaunchSpec) (ExecutionRef, error) {
	implementation, err := r.runtimeForName(spec.RuntimeName)
	if err != nil {
		return ExecutionRef{}, err
	}
	return implementation.Launch(ctx, spec)
}

func (r *Router) Start(ctx context.Context, spec LaunchSpec) (ExecutionRef, error) {
	implementation, err := r.runtimeForName(spec.RuntimeName)
	if err != nil {
		return ExecutionRef{}, err
	}
	return implementation.Start(ctx, spec)
}

func (r *Router) StartExecution(ctx context.Context, executionID string) error {
	implementation, err := r.runtimeForExecution(ctx, executionID)
	if err != nil {
		return err
	}
	return implementation.StartExecution(ctx, executionID)
}

func (r *Router) Resume(ctx context.Context, executionID, prompt string) error {
	implementation, err := r.runtimeForExecution(ctx, executionID)
	if err != nil {
		return err
	}
	return implementation.Resume(ctx, executionID, prompt)
}

// ResumeWithTurnID preserves durable prompt identity where the selected
// runtime supports it, while keeping legacy runtimes source-compatible.
func (r *Router) ResumeWithTurnID(ctx context.Context, executionID, turnID, prompt string) error {
	implementation, err := r.runtimeForExecution(ctx, executionID)
	if err != nil {
		return err
	}
	if target, ok := implementation.(interface {
		ResumeWithTurnID(context.Context, string, string, string) error
	}); ok {
		return target.ResumeWithTurnID(ctx, executionID, turnID, prompt)
	}
	return implementation.Resume(ctx, executionID, prompt)
}

func (r *Router) Stop(ctx context.Context, executionID, reason string) error {
	implementation, err := r.runtimeForExecution(ctx, executionID)
	if err != nil {
		return err
	}
	return implementation.Stop(ctx, executionID, reason)
}

// CancelActive forwards chat cancellation to runtimes that distinguish an
// interrupted turn from execution termination.
func (r *Router) CancelActive(ctx context.Context, executionID string) error {
	implementation, err := r.runtimeForExecution(ctx, executionID)
	if err != nil {
		return err
	}
	canceller, ok := implementation.(interface {
		CancelActive(context.Context, string) error
	})
	if !ok {
		return ErrUnsupported
	}
	return canceller.CancelActive(ctx, executionID)
}

func (r *Router) GetExecution(ctx context.Context, executionID string) (*Execution, error) {
	implementation, err := r.runtimeForExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	return implementation.GetExecution(ctx, executionID)
}

func (r *Router) SubscribeEvents(ctx context.Context, executionID string) (<-chan Event, error) {
	implementation, err := r.runtimeForExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	return implementation.SubscribeEvents(ctx, executionID)
}

func (r *Router) SetMcpMode(ctx context.Context, executionID, mode string) error {
	implementation, err := r.runtimeForExecution(ctx, executionID)
	if err != nil {
		return err
	}
	return implementation.SetMcpMode(ctx, executionID, mode)
}

func (r *Router) runtimeForExecution(ctx context.Context, executionID string) (Runtime, error) {
	if executionID == "" {
		return nil, errors.New("runtime router requires an execution ID")
	}
	name, err := r.resolver.RuntimeForExecution(ctx, executionID)
	if errors.Is(err, ErrNotFound) {
		return r.defaultRuntime, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve execution runtime: %w", err)
	}
	return r.runtimeForName(name)
}

func (r *Router) runtimeForName(name string) (Runtime, error) {
	if name == "" {
		return r.defaultRuntime, nil
	}
	implementation, ok := r.runtimes[name]
	if !ok {
		return nil, fmt.Errorf("runtime router has no implementation for %q", name)
	}
	return implementation, nil
}
