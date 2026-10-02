package acp

import (
	"context"
	"errors"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// @covers AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.9
func TestSetModeAlreadySatisfiedLegacyMode(t *testing.T) {
	for _, mode := range []string{"default", "ask"} {
		t.Run(mode, func(t *testing.T) {
			requests := make(chan acpsdk.SetSessionModeRequest, 1)
			adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
				return acpsdk.SetSessionModeResponse{}, nil
			})
			agent.legacyRequests = requests
			adapter.availableModes = []streams.SessionModeInfo{{ID: "default"}, {ID: "ask"}}
			if mode != "default" {
				adapter.noteCurrentMode("session-1", mode)
			}

			for range 2 {
				result, err := adapter.SetMode(context.Background(), mode)
				if err != nil || !result.Applied() || result.Effective != mode {
					t.Fatalf("SetMode(%q) = %+v, %v; want confirmed already-satisfied mode", mode, result, err)
				}
			}
			select {
			case request := <-requests:
				t.Fatalf("already-satisfied mode sent redundant request: %+v", request)
			default:
			}

			events := drainEvents(adapter)
			if len(events) != 2 {
				t.Fatalf("mode events = %+v, want one completion event for each selection", events)
			}
			for _, event := range events {
				if event.Type != streams.EventTypeSessionMode || event.SessionID != "session-1" || event.CurrentModeID != mode || event.RequestedModeID != "" {
					t.Fatalf("already-satisfied mode event = %+v, want normal confirmed event", event)
				}
				if event.SessionSettingsGeneration == 0 {
					t.Fatalf("already-satisfied mode event = %+v, want settings generation", event)
				}
			}
		})
	}
}

// @covers AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.10
func TestSetModeAlreadySatisfiedLegacyModeGuards(t *testing.T) {
	tests := []struct {
		name          string
		prepare       func(*testing.T, *Adapter, *setModeTestAgent) context.Context
		wantRequest   bool
		wantError     bool
		requestedMode string
	}{
		{
			name: "unknown current mode",
			prepare: func(_ *testing.T, adapter *Adapter, _ *setModeTestAgent) context.Context {
				adapter.mu.Lock()
				adapter.currentModeID = ""
				adapter.modeSessionID = ""
				adapter.mu.Unlock()
				return context.Background()
			},
			wantRequest: true, requestedMode: "default",
		},
		{
			name: "observation belongs to replaced session",
			prepare: func(_ *testing.T, adapter *Adapter, _ *setModeTestAgent) context.Context {
				adapter.mu.Lock()
				adapter.modeSessionID = "old-session"
				adapter.mu.Unlock()
				return context.Background()
			},
			wantRequest: true, requestedMode: "default",
		},
		{
			name: "reported mode differs",
			prepare: func(_ *testing.T, _ *Adapter, _ *setModeTestAgent) context.Context {
				return context.Background()
			},
			wantRequest: true, requestedMode: "ask",
		},
		{
			name: "prior operation remains uncertain",
			prepare: func(_ *testing.T, adapter *Adapter, _ *setModeTestAgent) context.Context {
				adapter.mu.Lock()
				adapter.modeOutcomeUncertain = true
				adapter.mu.Unlock()
				return context.Background()
			},
			wantRequest: true, requestedMode: "default",
		},
		{
			name: "caller canceled",
			prepare: func(_ *testing.T, _ *Adapter, _ *setModeTestAgent) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantError: true, requestedMode: "default",
		},
		{
			name: "adapter closed",
			prepare: func(t *testing.T, adapter *Adapter, _ *setModeTestAgent) context.Context {
				if err := adapter.Close(); err != nil {
					t.Fatalf("Close: %v", err)
				}
				return context.Background()
			},
			wantError: true, requestedMode: "default",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan acpsdk.SetSessionModeRequest, 1)
			adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
				return acpsdk.SetSessionModeResponse{}, nil
			})
			agent.legacyRequests = requests
			adapter.availableModes = []streams.SessionModeInfo{{ID: "default"}, {ID: "ask"}}
			ctx := tc.prepare(t, adapter, agent)
			result, err := adapter.SetMode(ctx, tc.requestedMode)
			if tc.wantError && err == nil {
				t.Fatal("SetMode succeeded after the adapter closed or caller canceled")
			}
			if !tc.wantError && err != nil {
				t.Fatalf("SetMode: %v", err)
			}
			if result.Applied() {
				t.Fatalf("SetMode = %+v; guard failure incorrectly used cached mode", result)
			}
			select {
			case request := <-requests:
				if !tc.wantRequest {
					t.Fatalf("guard failure sent unexpected provider request: %+v", request)
				}
				if string(request.ModeId) != tc.requestedMode {
					t.Fatalf("provider request = %+v, want %q", request, tc.requestedMode)
				}
			default:
				if tc.wantRequest {
					t.Fatal("guard failure skipped the required provider request")
				}
			}
		})
	}
}

// @covers AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.9, AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.10
func TestSetModeAlreadySatisfiedLegacyModeSessionTransitions(t *testing.T) {
	transitions := []struct {
		name string
		run  func(*testing.T, *Adapter, *setModeTestAgent) error
	}{
		{
			name: "new session",
			run: func(t *testing.T, adapter *Adapter, agent *setModeTestAgent) error {
				agent.newResponse = &acpsdk.NewSessionResponse{SessionId: "new-session", Modes: legacyModeState("ask")}
				_, err := adapter.NewSession(context.Background(), nil)
				return err
			},
		},
		{
			name: "loaded session",
			run: func(t *testing.T, adapter *Adapter, agent *setModeTestAgent) error {
				adapter.capabilities.LoadSession = true
				agent.loadResponse = &acpsdk.LoadSessionResponse{Modes: legacyModeState("ask")}
				return adapter.LoadSession(context.Background(), "loaded-session", nil)
			},
		},
		{
			name: "context reset",
			run: func(t *testing.T, adapter *Adapter, agent *setModeTestAgent) error {
				agent.newResponse = &acpsdk.NewSessionResponse{SessionId: "reset-session", Modes: legacyModeState("ask")}
				_, err := adapter.ResetSession(context.Background(), nil)
				return err
			},
		},
	}
	for _, transition := range transitions {
		t.Run(transition.name, func(t *testing.T) {
			requests := make(chan acpsdk.SetSessionModeRequest, 1)
			adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
				return acpsdk.SetSessionModeResponse{}, nil
			})
			agent.legacyRequests = requests
			adapter.availableModes = legacyModeInfos()
			adapter.mu.Lock()
			adapter.modeOutcomeUncertain = true
			adapter.mu.Unlock()
			if err := transition.run(t, adapter, agent); err != nil {
				t.Fatalf("session transition: %v", err)
			}

			result, err := adapter.SetMode(context.Background(), "ask")
			if err != nil || !result.Applied() || result.Effective != "ask" {
				t.Fatalf("SetMode after transition = %+v, %v; want the replacement session's report", result, err)
			}
			select {
			case request := <-requests:
				t.Fatalf("fresh replacement-session report sent redundant request: %+v", request)
			default:
			}
		})
	}

	t.Run("missing replacement report cannot borrow prior mode", func(t *testing.T) {
		requests := make(chan acpsdk.SetSessionModeRequest, 1)
		adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
			return acpsdk.SetSessionModeResponse{}, nil
		})
		agent.legacyRequests = requests
		adapter.capabilities.LoadSession = true
		adapter.availableModes = legacyModeInfos()
		agent.loadResponse = &acpsdk.LoadSessionResponse{Modes: legacyModeState("")}
		if err := adapter.LoadSession(context.Background(), "loaded-without-mode", nil); err != nil {
			t.Fatalf("LoadSession: %v", err)
		}
		result, err := adapter.SetMode(context.Background(), "default")
		if err != nil || result.Applied() {
			t.Fatalf("SetMode without replacement report = %+v, %v; want unconfirmed", result, err)
		}
		select {
		case request := <-requests:
			if string(request.ModeId) != "default" {
				t.Fatalf("SetMode request = %+v, want default", request)
			}
		default:
			t.Fatal("missing replacement report reused the prior session's matching mode")
		}
	})
}

func legacyModeInfos() []streams.SessionModeInfo {
	return []streams.SessionModeInfo{{ID: "default", Name: "Default"}, {ID: "ask", Name: "Ask"}}
}

func legacyModeState(current string) *acpsdk.SessionModeState {
	return &acpsdk.SessionModeState{
		CurrentModeId: acpsdk.SessionModeId(current),
		AvailableModes: []acpsdk.SessionMode{
			{Id: "default", Name: "Default"},
			{Id: "ask", Name: "Ask"},
		},
	}
}

func TestLateTimedOutModeReportWhileIdleDoesNotRestoreShortcutCertainty(t *testing.T) {
	requests := make(chan acpsdk.SetSessionModeRequest, 2)
	adapter, agent, processed := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		return acpsdk.SetSessionModeResponse{}, nil
	})
	agent.legacyRequests = requests

	first, err := adapter.SetMode(context.Background(), "plan")
	if err != nil || first.Confirmed {
		t.Fatalf("first SetMode = %+v, %v; want an unconfirmed result", first, err)
	}
	if request := <-requests; string(request.ModeId) != "plan" {
		t.Fatalf("first request = %+v, want plan", request)
	}

	// This uncorrelated report can be delayed from the timed-out mode operation.
	if err := reportModeFromAgent(agent, "default"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-processed:
	case <-time.After(2 * time.Second):
		t.Fatal("late idle report was not processed")
	}
	adapter.mu.RLock()
	uncertain := adapter.modeOutcomeUncertain
	adapter.mu.RUnlock()
	if !uncertain {
		t.Fatal("uncorrelated idle report cleared the timed-out operation's uncertainty")
	}

	second, err := adapter.SetMode(context.Background(), "default")
	if err != nil || second.Confirmed || second.Effective != "" {
		t.Fatalf("second SetMode = %+v, %v; want the uncertain operation to remain unconfirmed", second, err)
	}
	select {
	case request := <-requests:
		if string(request.ModeId) != "default" {
			t.Fatalf("second request = %+v, want default; idle report incorrectly cleared uncertainty", request)
		}
	default:
		t.Fatal("matching cached mode bypassed the provider RPC after an idle late report")
	}
}

func TestSetModeCanceledBeforeLegacyRPCDoesNotMakeOutcomeUncertain(t *testing.T) {
	requests := make(chan acpsdk.SetSessionModeRequest, 1)
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		return acpsdk.SetSessionModeResponse{}, nil
	})
	agent.legacyRequests = requests
	adapter.availableModes = []streams.SessionModeInfo{{ID: "default"}, {ID: "ask"}}

	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelOnErrContext{Context: baseCtx, cancel: cancel, cancelAt: 9} // Final legacy pre-RPC check.
	result, err := adapter.SetMode(ctx, "ask")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SetMode before legacy RPC error = %v, want context canceled", err)
	}
	if result.Applied() {
		t.Fatalf("canceled SetMode result = %+v, want unapplied", result)
	}
	select {
	case request := <-requests:
		t.Fatalf("canceled SetMode sent legacy RPC: %+v", request)
	default:
	}
	adapter.mu.RLock()
	uncertain := adapter.modeOutcomeUncertain
	adapter.mu.RUnlock()
	if uncertain {
		t.Fatal("cancellation before the legacy RPC made the mode outcome uncertain")
	}

	adapter.noteCurrentMode("session-1", "ask")
	result, err = adapter.SetMode(context.Background(), "ask")
	if err != nil || !result.Applied() || result.Effective != "ask" {
		t.Fatalf("SetMode after matching report = %+v, %v; want confirmed already-satisfied mode", result, err)
	}
	select {
	case request := <-requests:
		t.Fatalf("matching report should avoid a redundant legacy RPC: %+v", request)
	default:
	}
}

func TestSetModeCanceledBeforeModeConfigRPCDoesNotMakeOutcomeUncertain(t *testing.T) {
	requests := make(chan acpsdk.SetSessionConfigOptionRequest, 1)
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		return acpsdk.SetSessionModeResponse{}, nil
	})
	agent.configRequests = requests
	adapter.availableConfigOptions = convertACPConfigOptions([]acpsdk.SessionConfigOption{groupedModeConfigOption("default")})

	baseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelOnErrContext{Context: baseCtx, cancel: cancel, cancelAt: 7} // Mode-config pre-RPC check.
	result, err := adapter.SetMode(ctx, "bypassPermissions")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SetMode before mode config RPC error = %v, want context canceled", err)
	}
	if result.Applied() {
		t.Fatalf("canceled SetMode result = %+v, want unapplied", result)
	}
	select {
	case request := <-requests:
		t.Fatalf("canceled SetMode sent mode config RPC: %+v", request)
	default:
	}
	adapter.mu.RLock()
	uncertain := adapter.modeOutcomeUncertain
	adapter.mu.RUnlock()
	if uncertain {
		t.Fatal("cancellation before the mode config RPC made the mode outcome uncertain")
	}

	adapter.mu.Lock()
	adapter.availableConfigOptions = nil
	adapter.mu.Unlock()
	adapter.noteCurrentMode("session-1", "bypassPermissions")
	result, err = adapter.SetMode(context.Background(), "bypassPermissions")
	if err != nil || !result.Applied() || result.Effective != "bypassPermissions" {
		t.Fatalf("SetMode after matching report = %+v, %v; want confirmed already-satisfied mode", result, err)
	}
	select {
	case request := <-requests:
		t.Fatalf("matching report should avoid a redundant mode config RPC: %+v", request)
	default:
	}
}

type cancelOnErrContext struct {
	context.Context
	cancel   context.CancelFunc
	cancelAt int
	errCalls int
}

func (c *cancelOnErrContext) Err() error {
	c.errCalls++
	if c.errCalls == c.cancelAt {
		c.cancel()
	}
	return c.Context.Err()
}
