package acp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type modeLockAttemptContext struct {
	context.Context
	errCalls chan struct{}
}

func (c *modeLockAttemptContext) Err() error {
	select {
	case c.errCalls <- struct{}{}:
	default:
	}
	return c.Context.Err()
}

func TestModeAndOtherConfigSnapshotsShareOrdering(t *testing.T) {
	for _, modeFirst := range []bool{true, false} {
		name := "effort first"
		if modeFirst {
			name = "mode first"
		}
		t.Run(name, func(t *testing.T) {
			requests := make(chan acpsdk.SetSessionConfigOptionRequest, 2)
			releaseMode := make(chan struct{}, 1)
			releaseEffort := make(chan struct{}, 1)
			unblock := func(channel chan struct{}) {
				select {
				case channel <- struct{}{}:
				default:
				}
			}
			defer func() {
				unblock(releaseMode)
				unblock(releaseEffort)
			}()
			var modeCalls, effortCalls atomic.Int32
			adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
				t.Fatal("advertised mode option unexpectedly used session/set_mode")
				return acpsdk.SetSessionModeResponse{}, nil
			})
			agent.configRequests = requests
			agent.configHandler = func(ctx context.Context, request acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
				if request.ValueId == nil {
					return acpsdk.SetSessionConfigOptionResponse{}, fmt.Errorf("missing config value")
				}
				modeRequest := string(request.ValueId.ConfigId) == "claude_permission_mode"
				switch {
				case modeRequest:
					modeCalls.Add(1)
				default:
					effortCalls.Add(1)
				}
				release := releaseEffort
				mode := "bypassPermissions"
				effort := "high"
				if modeRequest {
					release = releaseMode
					if modeFirst {
						effort = "low"
					}
				}
				select {
				case <-release:
				case <-ctx.Done():
					return acpsdk.SetSessionConfigOptionResponse{}, ctx.Err()
				}
				return acpsdk.SetSessionConfigOptionResponse{ConfigOptions: sessionConfigSnapshot(mode, "model-a", effort)}, nil
			}
			adapter.availableConfigOptions = convertACPConfigOptions(sessionConfigSnapshot("default", "model-a", "low"))

			modeDone := make(chan error, 1)
			effortDone := make(chan error, 1)
			startMode := func() {
				go func() {
					_, err := adapter.SetMode(context.Background(), "bypassPermissions")
					modeDone <- err
				}()
			}
			startEffort := func() {
				go func() {
					effortDone <- adapter.SetConfigOption(context.Background(), "reasoning_effort", "high")
				}()
			}
			if modeFirst {
				startMode()
			} else {
				startEffort()
			}
			first := <-requests
			if modeFirst {
				if first.ValueId == nil || string(first.ValueId.ConfigId) != "claude_permission_mode" {
					t.Fatalf("first config request = %+v, want permission mode", first)
				}
				startEffort()
			} else {
				if first.ValueId == nil || string(first.ValueId.ConfigId) != "reasoning_effort" {
					t.Fatalf("first config request = %+v, want reasoning effort", first)
				}
				startMode()
			}
			select {
			case overlapped := <-requests:
				t.Fatalf("full config snapshots from mode and effort requests overlapped: first=%+v second=%+v", first.ValueId, overlapped.ValueId)
			case <-time.After(50 * time.Millisecond):
			}

			if modeFirst {
				unblock(releaseMode)
			} else {
				unblock(releaseEffort)
			}
			second := <-requests
			if second.ValueId == nil || string(second.ValueId.ConfigId) == string(first.ValueId.ConfigId) {
				t.Fatalf("second config request = %+v, want the other config option", second)
			}
			if modeFirst {
				unblock(releaseEffort)
			} else {
				unblock(releaseMode)
			}
			if err := <-modeDone; err != nil {
				t.Fatalf("SetMode: %v", err)
			}
			if err := <-effortDone; err != nil {
				t.Fatalf("SetConfigOption: %v", err)
			}
			if modeCalls.Load() != 1 || effortCalls.Load() != 1 {
				t.Fatalf("RPC counts mode=%d effort=%d, want one each", modeCalls.Load(), effortCalls.Load())
			}
			adapter.mu.RLock()
			cached := cloneConfigOptions(adapter.availableConfigOptions)
			adapter.mu.RUnlock()
			if got := currentModeFromConfig(cached); got != "bypassPermissions" {
				t.Fatalf("cached mode = %q, want latest bypassPermissions", got)
			}
			if got := configOptionCurrentValue(cached, "reasoning_effort"); got != "high" {
				t.Fatalf("cached effort = %q, want high", got)
			}
			if got := currentModelFromConfig(cached); got != "model-a" {
				t.Fatalf("cached model = %q, want model-a", got)
			}
		})
	}
}

// @covers AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.9
func TestQueuedSetModeRechecksAlreadySatisfiedLegacyMode(t *testing.T) {
	requests := make(chan acpsdk.SetSessionModeRequest, 2)
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseFirstOnce sync.Once
	releaseFirstCall := func() { releaseFirstOnce.Do(func() { close(releaseFirst) }) }
	defer releaseFirstCall()
	var adapter *Adapter
	var agent *setModeTestAgent
	var processed chan string
	var requestCount atomic.Int32
	adapter, agent, processed = newSetModeTestAdapter(t, func(ctx context.Context, _ acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		if requestCount.Add(1) == 1 {
			close(firstEntered)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return acpsdk.SetSessionModeResponse{}, ctx.Err()
			}
			if err := reportModeFromAgent(agent, "ask"); err != nil {
				return acpsdk.SetSessionModeResponse{}, err
			}
			select {
			case <-processed:
			case <-ctx.Done():
				return acpsdk.SetSessionModeResponse{}, ctx.Err()
			}
		}
		return acpsdk.SetSessionModeResponse{}, nil
	})
	agent.legacyRequests = requests
	adapter.availableModes = legacyModeInfos()

	type outcome struct {
		result streams.ModeResult
		err    error
	}
	firstDone := make(chan outcome, 1)
	go func() {
		result, err := adapter.SetMode(context.Background(), "ask")
		firstDone <- outcome{result: result, err: err}
	}()
	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("first mode request did not enter the agent")
	}

	secondCtx := &modeLockAttemptContext{Context: context.Background(), errCalls: make(chan struct{}, 8)}
	secondDone := make(chan outcome, 1)
	go func() {
		result, err := adapter.SetMode(secondCtx, "ask")
		secondDone <- outcome{result: result, err: err}
	}()
	// lockModeChange checks the request context before each try-lock. Its second
	// check can only happen after the first try-lock failed and the ticker woke
	// the waiter, which establishes that this successor reached the held gate.
	for range 2 {
		select {
		case <-secondCtx.errCalls:
		case <-time.After(2 * time.Second):
			t.Fatal("successor did not retry the mode gate while the first request was held")
		}
	}
	if got := requestCount.Load(); got != 1 {
		t.Fatalf("provider mode requests before releasing first call = %d, want 1", got)
	}
	if got := len(requests); got != 1 {
		t.Fatalf("captured provider mode requests before release = %d, want 1", got)
	}
	select {
	case got := <-secondDone:
		t.Fatalf("successor returned before the first request released the mode gate: %+v, %v", got.result, got.err)
	default:
	}
	releaseFirstCall()

	for name, done := range map[string]<-chan outcome{"first": firstDone, "second": secondDone} {
		select {
		case got := <-done:
			if got.err != nil || !got.result.Applied() || got.result.Effective != "ask" {
				t.Fatalf("%s SetMode = %+v, %v; want confirmed ask", name, got.result, got.err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s SetMode did not finish", name)
		}
	}
	if got := requestCount.Load(); got != 1 {
		t.Fatalf("provider mode requests = %d, want only the in-flight request", got)
	}
	if request := <-requests; string(request.ModeId) != "ask" {
		t.Fatalf("provider request = %+v, want ask", request)
	}
}

func TestModeAndModelConfigSnapshotsShareOrdering(t *testing.T) {
	for _, modeFirst := range []bool{true, false} {
		name := "model first"
		if modeFirst {
			name = "mode first"
		}
		t.Run(name, func(t *testing.T) {
			requests := make(chan acpsdk.SetSessionConfigOptionRequest, 2)
			releaseMode := make(chan struct{}, 1)
			releaseModel := make(chan struct{}, 1)
			unblock := func(channel chan struct{}) {
				select {
				case channel <- struct{}{}:
				default:
				}
			}
			defer func() {
				unblock(releaseMode)
				unblock(releaseModel)
			}()
			var modeCalls, modelCalls atomic.Int32
			adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
				t.Fatal("advertised mode option unexpectedly used session/set_mode")
				return acpsdk.SetSessionModeResponse{}, nil
			})
			agent.configRequests = requests
			agent.configHandler = func(ctx context.Context, request acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
				if request.ValueId == nil {
					return acpsdk.SetSessionConfigOptionResponse{}, fmt.Errorf("missing config value")
				}
				modeRequest := string(request.ValueId.ConfigId) == "claude_permission_mode"
				switch {
				case modeRequest:
					modeCalls.Add(1)
				case string(request.ValueId.ConfigId) == "model":
					modelCalls.Add(1)
				default:
					return acpsdk.SetSessionConfigOptionResponse{}, fmt.Errorf("unexpected config ID %q", request.ValueId.ConfigId)
				}

				release := releaseModel
				mode, model := "default", "model-b"
				if modeRequest {
					release = releaseMode
					mode = "bypassPermissions"
					if modeFirst {
						model = "model-a"
					}
				} else if modeFirst {
					mode = "bypassPermissions"
				}
				select {
				case <-release:
				case <-ctx.Done():
					return acpsdk.SetSessionConfigOptionResponse{}, ctx.Err()
				}
				return acpsdk.SetSessionConfigOptionResponse{ConfigOptions: sessionConfigSnapshot(mode, model, "low")}, nil
			}
			adapter.availableModels = []modelInfo{{ModelId: "model-a"}, {ModelId: "model-b"}}
			adapter.availableConfigOptions = convertACPConfigOptions(sessionConfigSnapshot("default", "model-a", "low"))

			modeDone := make(chan error, 1)
			modelDone := make(chan error, 1)
			startMode := func() {
				go func() {
					_, err := adapter.SetMode(context.Background(), "bypassPermissions")
					modeDone <- err
				}()
			}
			startModel := func() {
				go func() { modelDone <- adapter.SetModel(context.Background(), "model-b") }()
			}
			if modeFirst {
				startMode()
			} else {
				startModel()
			}
			first := <-requests
			if modeFirst {
				if first.ValueId == nil || string(first.ValueId.ConfigId) != "claude_permission_mode" {
					t.Fatalf("first config request = %+v, want permission mode", first)
				}
				startModel()
			} else {
				if first.ValueId == nil || string(first.ValueId.ConfigId) != "model" {
					t.Fatalf("first config request = %+v, want model", first)
				}
				startMode()
			}
			select {
			case overlapped := <-requests:
				t.Fatalf("full config snapshots from mode and model requests overlapped: first=%+v second=%+v", first.ValueId, overlapped.ValueId)
			case <-time.After(50 * time.Millisecond):
			}

			if modeFirst {
				unblock(releaseMode)
			} else {
				unblock(releaseModel)
			}
			second := <-requests
			if second.ValueId == nil || string(second.ValueId.ConfigId) == string(first.ValueId.ConfigId) {
				t.Fatalf("second config request = %+v, want the other config option", second)
			}
			if modeFirst {
				unblock(releaseModel)
			} else {
				unblock(releaseMode)
			}
			if err := <-modeDone; err != nil {
				t.Fatalf("SetMode: %v", err)
			}
			if err := <-modelDone; err != nil {
				t.Fatalf("SetModel: %v", err)
			}
			if modeCalls.Load() != 1 || modelCalls.Load() != 1 {
				t.Fatalf("RPC counts mode=%d model=%d, want one each", modeCalls.Load(), modelCalls.Load())
			}
			adapter.mu.RLock()
			cached := cloneConfigOptions(adapter.availableConfigOptions)
			adapter.mu.RUnlock()
			if got := currentModeFromConfig(cached); got != "bypassPermissions" {
				t.Fatalf("cached mode = %q, want latest bypassPermissions", got)
			}
			if got := currentModelFromConfig(cached); got != "model-b" {
				t.Fatalf("cached model = %q, want latest model-b", got)
			}
			if got := configOptionCurrentValue(cached, "reasoning_effort"); got != "low" {
				t.Fatalf("cached effort = %q, want low", got)
			}
		})
	}
}

func TestConfigChangeWaitersRespectCancellationAndDoNotMutateSession(t *testing.T) {
	tests := []struct {
		name      string
		operation func(context.Context, *Adapter) error
		modeLock  bool
	}{
		{
			name: "mode change",
			operation: func(ctx context.Context, adapter *Adapter) error {
				_, err := adapter.SetMode(ctx, "bypassPermissions")
				return err
			},
			modeLock: true,
		},
		{
			name: "new session",
			operation: func(ctx context.Context, adapter *Adapter) error {
				_, err := adapter.NewSession(ctx, nil)
				return err
			},
		},
		{
			name: "load session",
			operation: func(ctx context.Context, adapter *Adapter) error {
				return adapter.LoadSession(ctx, "session-2", nil)
			},
		},
		{
			name: "reset session",
			operation: func(ctx context.Context, adapter *Adapter) error {
				_, err := adapter.ResetSession(ctx, nil)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := make(chan acpsdk.SetSessionConfigOptionRequest, 2)
			firstEntered := make(chan struct{})
			releaseFirst := make(chan struct{})
			var releaseOnce sync.Once
			unblockFirst := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
			defer unblockFirst()

			adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
				return acpsdk.SetSessionModeResponse{}, errors.New("unexpected legacy mode request")
			})
			agent.configRequests = requests
			agent.newCalls = make(chan struct{}, 1)
			agent.loadStarted = make(chan struct{})
			agent.configHandler = func(ctx context.Context, request acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
				if request.ValueId == nil || string(request.ValueId.ConfigId) != "model" {
					return acpsdk.SetSessionConfigOptionResponse{}, fmt.Errorf("first request = %+v, want model config", request.ValueId)
				}
				close(firstEntered)
				select {
				case <-releaseFirst:
					return acpsdk.SetSessionConfigOptionResponse{ConfigOptions: sessionConfigSnapshot("default", "model-b", "low")}, nil
				case <-ctx.Done():
					return acpsdk.SetSessionConfigOptionResponse{}, ctx.Err()
				}
			}
			adapter.availableModels = []modelInfo{{ModelId: "model-a"}, {ModelId: "model-b"}}
			adapter.availableConfigOptions = convertACPConfigOptions(sessionConfigSnapshot("default", "model-a", "low"))

			holderDone := make(chan error, 1)
			go func() { holderDone <- adapter.SetModel(context.Background(), "model-b") }()
			<-firstEntered
			<-requests

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			waiterStarted := make(chan struct{})
			waiterDone := make(chan error, 1)
			go func() {
				close(waiterStarted)
				waiterDone <- tt.operation(ctx, adapter)
			}()
			<-waiterStarted
			ownershipMu := &adapter.sessionTransitionMu
			if tt.modeLock {
				ownershipMu = &adapter.modeChangeMu
			}
			ownershipDeadline := time.NewTimer(time.Second)
			defer ownershipDeadline.Stop()
			ownershipPoll := time.NewTicker(time.Millisecond)
			defer ownershipPoll.Stop()
			for ownershipMu.TryLock() {
				ownershipMu.Unlock()
				select {
				case <-ownershipDeadline.C:
					t.Fatal("operation did not acquire its mode or session transition ownership")
				case <-ownershipPoll.C:
				}
			}
			cancel()

			var waitErr error
			timedOut := false
			select {
			case waitErr = <-waiterDone:
			case <-time.After(time.Second):
				timedOut = true
			}
			if timedOut {
				unblockFirst()
				select {
				case <-waiterDone:
				case <-time.After(time.Second):
					t.Fatal("canceled waiter did not return after the in-flight config RPC was released")
				}
				if err := <-holderDone; err != nil {
					t.Fatalf("SetModel holder: %v", err)
				}
				t.Fatal("canceled waiter did not return before the in-flight config RPC was released")
			}
			if !errors.Is(waitErr, context.Canceled) {
				t.Fatalf("canceled operation error = %v, want context.Canceled", waitErr)
			}

			adapter.mu.RLock()
			mode := currentModeFromConfig(adapter.availableConfigOptions)
			model := currentModelFromConfig(adapter.availableConfigOptions)
			sessionID := adapter.sessionID
			adapter.mu.RUnlock()
			if sessionID != "session-1" || mode != "default" || model != "model-a" {
				t.Fatalf("canceled waiter changed session cache: session=%q mode=%q model=%q", sessionID, mode, model)
			}
			select {
			case <-requests:
				t.Fatal("canceled waiter sent another config request")
			default:
			}
			select {
			case <-agent.newCalls:
				t.Fatal("canceled waiter sent session/new")
			default:
			}
			select {
			case <-agent.loadStarted:
				t.Fatal("canceled waiter sent session/load")
			default:
			}

			if !ownershipMu.TryLock() {
				t.Fatal("canceled operation retained mode or session transition ownership")
			}
			ownershipMu.Unlock()

			unblockFirst()
			if err := <-holderDone; err != nil {
				t.Fatalf("SetModel holder: %v", err)
			}
			cancel()
		})
	}
}

func configOptionCurrentValue(options []streams.ConfigOption, id string) string {
	for _, option := range options {
		if option.ID == id {
			return option.CurrentValue
		}
	}
	return ""
}

func TestUnrelatedConfigResponseDoesNotClearModeTimeoutUncertainty(t *testing.T) {
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		t.Fatal("SetConfigOption unexpectedly changed permission mode")
		return acpsdk.SetSessionModeResponse{}, nil
	})
	adapter.availableConfigOptions = convertACPConfigOptions(sessionConfigSnapshot("default", "model-a", "low"))
	adapter.mu.Lock()
	adapter.modeOutcomeUncertain = true
	adapter.mu.Unlock()
	agent.configHandler = func(context.Context, acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
		return acpsdk.SetSessionConfigOptionResponse{ConfigOptions: sessionConfigSnapshot("default", "model-a", "high")}, nil
	}
	if err := adapter.SetConfigOption(context.Background(), "reasoning_effort", "high"); err != nil {
		t.Fatalf("SetConfigOption: %v", err)
	}
	adapter.mu.RLock()
	uncertain := adapter.modeOutcomeUncertain
	adapter.mu.RUnlock()
	if !uncertain {
		t.Fatal("unrelated config response cleared unresolved permission-mode uncertainty")
	}
}

// @covers AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.10
func TestCorrelatedModeConfigSnapshotClearsLegacyModeUncertainty(t *testing.T) {
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		t.Fatal("advertised mode option unexpectedly used session/set_mode")
		return acpsdk.SetSessionModeResponse{}, nil
	})
	adapter.availableConfigOptions = convertACPConfigOptions([]acpsdk.SessionConfigOption{groupedModeConfigOption("default")})
	adapter.mu.Lock()
	adapter.modeOutcomeUncertain = true
	adapter.mu.Unlock()
	agent.configHandler = func(_ context.Context, request acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
		return acpsdk.SetSessionConfigOptionResponse{ConfigOptions: []acpsdk.SessionConfigOption{
			groupedModeConfigOption(string(request.ValueId.Value)),
		}}, nil
	}

	result, err := adapter.SetMode(context.Background(), "default")
	if err != nil || !result.Applied() || result.Effective != "default" {
		t.Fatalf("SetMode = %+v, %v; want correlated mode-config confirmation", result, err)
	}
	adapter.mu.RLock()
	uncertain := adapter.modeOutcomeUncertain
	adapter.mu.RUnlock()
	if uncertain {
		t.Fatal("correlated mode-config snapshot left the prior uncertainty unresolved")
	}
}

func TestSetModeAndModeConfigOptionAreSerialized(t *testing.T) {
	requests := make(chan acpsdk.SetSessionConfigOptionRequest, 2)
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls atomic.Int32
	adapter, agent, _ := newSetModeTestAdapter(t, func(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
		t.Fatal("advertised mode option unexpectedly used session/set_mode")
		return acpsdk.SetSessionModeResponse{}, nil
	})
	agent.configRequests = requests
	agent.configHandler = func(ctx context.Context, request acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
		call := calls.Add(1)
		if call == 1 {
			close(firstEntered)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return acpsdk.SetSessionConfigOptionResponse{}, ctx.Err()
			}
		}
		return acpsdk.SetSessionConfigOptionResponse{ConfigOptions: []acpsdk.SessionConfigOption{
			groupedModeConfigOption(string(request.ValueId.Value)),
		}}, nil
	}
	adapter.availableConfigOptions = convertACPConfigOptions([]acpsdk.SessionConfigOption{groupedModeConfigOption("default")})

	firstResult := make(chan error, 1)
	go func() {
		_, err := adapter.SetMode(context.Background(), "plan")
		firstResult <- err
	}()
	<-firstEntered
	firstRequest := <-requests
	if firstRequest.ValueId == nil || string(firstRequest.ValueId.Value) != "plan" {
		t.Fatalf("first mode request = %+v, want plan", firstRequest)
	}
	secondStarted := make(chan struct{})
	secondResult := make(chan error, 1)
	go func() {
		close(secondStarted)
		secondResult <- adapter.SetConfigOption(context.Background(), "claude_permission_mode", "acceptEdits")
	}()
	<-secondStarted
	select {
	case request := <-requests:
		t.Fatalf("second mode request %q overlapped the first request", request.ValueId.Value)
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstResult; err != nil {
		t.Fatalf("first SetMode: %v", err)
	}
	if err := <-secondResult; err != nil {
		t.Fatalf("second SetConfigOption: %v", err)
	}
	select {
	case request := <-requests:
		if string(request.ValueId.Value) != "acceptEdits" {
			t.Fatalf("second request value = %q, want acceptEdits", request.ValueId.Value)
		}
	default:
		t.Fatal("serialized config-option mode request was not sent")
	}
}
