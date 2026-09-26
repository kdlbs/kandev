package runtime

import (
	"context"
	"errors"
	"testing"
)

func TestRouterUsesDurableRuntimeOwner(t *testing.T) {
	local := &routerTestRuntime{}
	cloud := &routerTestRuntime{}
	router, err := NewRouter(local, ExecutionRuntimeResolverFunc(func(context.Context, string) (string, error) {
		return "cursor_cloud", nil
	}), map[string]Runtime{"cursor_cloud": cloud})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if _, err := router.Launch(context.Background(), LaunchSpec{RuntimeName: "cursor_cloud"}); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if err := router.StartExecution(context.Background(), "managed-execution"); err != nil {
		t.Fatalf("StartExecution: %v", err)
	}
	if cloud.launchCalls != 1 || cloud.startCalls != 1 || local.launchCalls != 0 || local.startCalls != 0 {
		t.Fatalf("routed calls: cloud=%#v local=%#v", cloud, local)
	}
}

func TestRouterFailsClosedWhenRuntimeResolutionFails(t *testing.T) {
	local := &routerTestRuntime{}
	router, err := NewRouter(local, ExecutionRuntimeResolverFunc(func(context.Context, string) (string, error) {
		return "", errors.New("database unavailable")
	}), nil)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if err := router.StartExecution(context.Background(), "managed-execution"); err == nil {
		t.Fatal("StartExecution succeeded after runtime resolution failed")
	}
	if local.startCalls != 0 {
		t.Fatalf("local fallback called %d times after resolver failure", local.startCalls)
	}
}

type routerTestRuntime struct {
	launchCalls int
	startCalls  int
}

func (r *routerTestRuntime) Launch(context.Context, LaunchSpec) (ExecutionRef, error) {
	r.launchCalls++
	return ExecutionRef{}, nil
}
func (r *routerTestRuntime) Start(ctx context.Context, spec LaunchSpec) (ExecutionRef, error) {
	ref, err := r.Launch(ctx, spec)
	if err == nil {
		err = r.StartExecution(ctx, ref.ID)
	}
	return ref, err
}
func (r *routerTestRuntime) StartExecution(context.Context, string) error {
	r.startCalls++
	return nil
}
func (*routerTestRuntime) Resume(context.Context, string, string) error { return nil }
func (*routerTestRuntime) Stop(context.Context, string, string) error   { return nil }
func (*routerTestRuntime) GetExecution(context.Context, string) (*Execution, error) {
	return nil, nil
}
func (*routerTestRuntime) SubscribeEvents(context.Context, string) (<-chan Event, error) {
	return nil, nil
}
func (*routerTestRuntime) SetMcpMode(context.Context, string, string) error { return nil }
