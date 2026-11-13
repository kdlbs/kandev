package hostutility

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestManagerProfileProbeAndModelResolutionUseScopedLaunchContext(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var requests []agentctlutil.ProbeRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity" {
			_, _ = w.Write([]byte(`{"capabilities":["launch-wrapper.v1"]}`))
			return
		}
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/api/v1/inference/probe" {
			http.NotFound(w, r)
			return
		}
		var req agentctlutil.ProbeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode probe request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"models":[{"id":"codex","name":"Codex"}],"config_options":[{"type":"select","id":"reasoning","name":"Reasoning","current_value":"medium"}]}`))
	}))
	defer server.Close()
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	reg := registry.NewRegistry(log)
	if err := reg.Register(agents.NewClaudeACP()); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(reg, host, port, nil, log)
	manager.instances["claude-acp"] = &instance{
		agentType: "claude-acp", instanceID: "test-instance", workDir: t.TempDir(),
		client: agentctlclient.NewClient(host, port, log),
	}
	profileContext := ProfileProbeContext{
		Scope:         "user-1:profile-1",
		Env:           map[string]string{"CODEX_PATH": "/profile/codex", "TOKEN": "secret-value"},
		CLIFlags:      []string{"--profile-flag", "ordered-value"},
		CommandPrefix: []string{"npx", "--"},
	}
	first, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{
		Context: profileContext, Refresh: true,
	})
	if err != nil {
		t.Fatalf("ProbeProfileCapabilities: %v", err)
	}
	if first.Capabilities.Status != StatusOK || first.ContextRevision == "" {
		t.Fatalf("profile probe = %#v", first)
	}
	if _, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{Context: profileContext}); err != nil {
		t.Fatalf("cached ProbeProfileCapabilities: %v", err)
	}
	resolution, err := manager.ResolveModelConfig(context.Background(), "claude-acp", ModelConfigResolutionRequest{
		Model: "codex", ProfileContext: &profileContext,
	})
	if err != nil {
		t.Fatalf("ResolveModelConfig: %v", err)
	}
	if resolution.Status != StatusOK || resolution.ContextRevision != first.ContextRevision || len(resolution.ConfigOptions) != 1 {
		t.Fatalf("profile model resolution = %#v", resolution)
	}
	manager.PublishCapabilities("claude-acp", AgentCapabilities{AgentType: "claude-acp", Status: StatusOK})
	refreshed, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{Context: profileContext})
	if err != nil {
		t.Fatalf("ProbeProfileCapabilities after runtime publication: %v", err)
	}
	if refreshed.ContextRevision == first.ContextRevision {
		t.Fatal("runtime publication did not change the profile context generation")
	}
	rotatedContext := profileContext
	rotatedContext.Env = map[string]string{"CODEX_PATH": "/profile/codex", "TOKEN": "rotated-secret"}
	if _, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{Context: rotatedContext}); err != nil {
		t.Fatalf("ProbeProfileCapabilities after secret rotation: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 4 {
		t.Fatalf("provider requests = %d, want baseline, model options, runtime refresh, and secret rotation probes", len(requests))
	}
	for i, request := range requests {
		wantToken := "secret-value"
		if i == len(requests)-1 {
			wantToken = "rotated-secret"
		}
		if !request.ProfileContext || request.InferenceConfig.Env["CODEX_PATH"] != "/profile/codex" ||
			request.InferenceConfig.Env["TOKEN"] != wantToken ||
			!slices.Equal(request.InferenceConfig.CLIFlags, profileContext.CLIFlags) ||
			!slices.Equal(request.InferenceConfig.LaunchWrapper.Argv, profileContext.CommandPrefix) || request.InferenceConfig.LaunchWrapper.Version != 1 || len(request.InferenceConfig.CommandPrefix) != 0 {
			t.Fatalf("request %d omitted profile launch context: %#v", i, request)
		}
	}
	if requests[1].Model != "codex" {
		t.Fatalf("model probe selected %q, want codex", requests[1].Model)
	}
}

func TestProfileManagedRuntimeCacheRecoveryPreservesScopedContext(t *testing.T) {
	for _, protocol := range []struct {
		name  string
		agent agents.Agent
	}{
		{name: "managed ACP", agent: agents.NewClaudeACP()},
		{name: "managed native Codex app-server", agent: agents.NewCodexAppServer(true)},
	} {
		for _, test := range []struct {
			name       string
			modelProbe bool
			customWrap bool
		}{
			{name: "baseline capability probe"},
			{name: "model options probe", modelProbe: true},
			{name: "custom launch wrapper is ineligible", customWrap: true},
		} {
			t.Run(protocol.name+"/"+test.name, func(t *testing.T) {
				const version = "2.1.3"
				agent := protocol.agent
				managedAgent, ok := agent.(agents.ManagedNPMRuntimeAgent)
				if !ok {
					t.Fatal("test agent does not expose managed npm runtime metadata")
				}
				managedRuntime := managedAgent.ManagedNPMRuntime()
				packageSpec := managedRuntime.PackageSpec(version)
				profileContext := ProfileProbeContext{
					Scope: "user-1:profile-1",
					Env:   map[string]string{"PROFILE_SENTINEL": "profile-only-secret"},
				}
				if test.customWrap {
					profileContext.CommandPrefix = []string{"custom-wrapper", "--"}
				}
				var requests []agentctlutil.ProbeRequest
				manager, _ := newProfileProbeTestManagerWithCapabilities(
					t,
					agent,
					managedRuntimeSelectionReader{
						selection: managedruntime.Selection{Package: managedRuntime.Package, Version: version},
						found:     true,
					},
					[]string{"launch-wrapper.v1", agentctltypes.ManagedRuntimePrivatePreparationCapability},
					func(w http.ResponseWriter, request agentctlutil.ProbeRequest) {
						requests = append(requests, request)
						if len(requests) == 1 {
							data, _ := json.Marshal(agentctlutil.ProbeResponse{
								Success: false, Error: "verified npm execution root is incomplete",
								FailureCode: agentctlutil.ProbeFailureManagedRuntimeCacheIncomplete,
								CacheEvidence: &agentctltypes.ManagedRuntimeCacheEvidence{
									Condition:   agentctltypes.ManagedRuntimeCacheRootManifestMissing,
									PackageSpec: packageSpec, ProbeAttemptID: "opaque-profile-attempt-1",
								},
							})
							_, _ = w.Write(data)
							return
						}
						response := agentctlutil.ProbeResponse{
							Success: true,
							Models:  []agentctlutil.ProbeModel{{ID: "claude-sonnet", Name: "Claude Sonnet"}},
						}
						if request.Model != "" {
							response.ConfigOptions = []agentctlutil.ProbeConfigOption{{
								ID: "reasoning_effort", CurrentValue: "high",
							}}
						}
						data, _ := json.Marshal(response)
						_, _ = w.Write(data)
					},
				)

				switch {
				case test.modelProbe:
					resolution, err := manager.ResolveModelConfig(context.Background(), agent.ID(), ModelConfigResolutionRequest{
						Model: "claude-sonnet", ProfileContext: &profileContext, Refresh: true,
					})
					if err != nil {
						t.Fatalf("ResolveModelConfig: %v", err)
					}
					if resolution.Status != StatusOK || len(resolution.ConfigOptions) != 1 {
						t.Fatalf("model resolution = %#v, want recovered config options", resolution)
					}
				default:
					result, err := manager.ProbeProfileCapabilities(context.Background(), agent.ID(), ProfileCapabilityRequest{
						Context: profileContext, Refresh: true,
					})
					if err != nil {
						t.Fatalf("ProbeProfileCapabilities: %v", err)
					}
					expectedStatus := StatusOK
					if test.customWrap {
						expectedStatus = StatusFailed
					}
					if result.Capabilities.Status != expectedStatus {
						t.Fatalf("profile status = %q, want %q", result.Capabilities.Status, expectedStatus)
					}
					if !test.customWrap && len(result.Capabilities.Models) != 1 {
						t.Fatalf("profile capabilities = %#v, want recovered models", result.Capabilities)
					}
				}
				wantRequests := 2
				if test.customWrap {
					wantRequests = 1
				}
				if len(requests) != wantRequests {
					t.Fatalf("probe requests = %d, want %d", len(requests), wantRequests)
				}
				wantCommand := managedRuntime.ACPCommandWithNpmPreference(version, false).Args()
				for index, request := range requests {
					if !request.ProfileContext || !equalStrings(request.InferenceConfig.Command, wantCommand) ||
						request.InferenceConfig.Env["PROFILE_SENTINEL"] != "profile-only-secret" {
						t.Fatalf("probe request %d lost scoped launch context: %#v", index, request)
					}
				}
				if test.customWrap {
					if requests[0].Preparation != nil || len(requests[0].InferenceConfig.LaunchWrapper.Argv) == 0 {
						t.Fatalf("custom wrapper request unexpectedly prepared privately: %#v", requests[0])
					}
					if _, found := manager.Get(agent.ID()); found {
						t.Fatal("ineligible wrapper failure published into shared capability cache")
					}
					return
				}
				if requests[1].Preparation == nil ||
					requests[1].Preparation.Condition != agentctltypes.ManagedRuntimeCacheRootManifestMissing ||
					requests[1].Preparation.PackageSpec != packageSpec ||
					requests[1].Preparation.ProbeAttemptID != "opaque-profile-attempt-1" {
					t.Fatalf("private preparation directive = %#v", requests[1].Preparation)
				}
				retryWithoutDirective := requests[1]
				retryWithoutDirective.Preparation = nil
				if !reflect.DeepEqual(retryWithoutDirective, requests[0]) {
					t.Fatalf("retry changed profile request beyond the preparation directive:\ninitial=%#v\nretry=%#v", requests[0], retryWithoutDirective)
				}
				if _, found := manager.Get(agent.ID()); found {
					t.Fatal("profile recovery published into the shared agent capability cache")
				}
			})
		}
	}
}

func TestProfileManagedRuntimeCacheRecoveryCancelsSupersededFlight(t *testing.T) {
	const version = "2.1.3"
	agent := agents.NewClaudeACP()
	packageSpec := agent.ManagedNPMRuntime().PackageSpec(version)
	preparationStarted := make(chan struct{})
	recoveryCancelled := make(chan struct{})
	forceRecoveryRelease := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(forceRecoveryRelease) })
	var probeCalls atomic.Int32
	manager, server := newProfileProbeTestManagerWithRequestHandler(
		t,
		agent,
		managedRuntimeSelectionReader{
			selection: managedruntime.Selection{Package: agent.ManagedNPMRuntime().Package, Version: version},
			found:     true,
		},
		[]string{"managed_runtime_private_preparation_v1"},
		func(w http.ResponseWriter, r *http.Request, req agentctlutil.ProbeRequest) {
			call := probeCalls.Add(1)
			if call == 1 {
				data, _ := json.Marshal(agentctlutil.ProbeResponse{
					Success: false, Error: "verified npm execution root is incomplete",
					FailureCode: agentctlutil.ProbeFailureManagedRuntimeCacheIncomplete,
					CacheEvidence: &agentctltypes.ManagedRuntimeCacheEvidence{
						Condition:   agentctltypes.ManagedRuntimeCacheRootManifestMissing,
						PackageSpec: packageSpec, ProbeAttemptID: "opaque-superseded-attempt",
					},
				})
				_, _ = w.Write(data)
				return
			}
			if req.Preparation != nil {
				close(preparationStarted)
				select {
				case <-r.Context().Done():
					close(recoveryCancelled)
					return
				case <-forceRecoveryRelease:
				}
			}
			data, _ := json.Marshal(agentctlutil.ProbeResponse{Success: true, Models: []agentctlutil.ProbeModel{{ID: "claude-sonnet"}}})
			_, _ = w.Write(data)
		},
	)
	defer server.Close()
	_ = manager
	oldContext := ProfileProbeContext{Scope: "user-1:profile-1", Env: map[string]string{"PROFILE": "old"}}
	oldResult := make(chan error, 1)
	go func() {
		_, err := manager.ProbeProfileCapabilities(context.Background(), agent.ID(), ProfileCapabilityRequest{
			Context: oldContext, Refresh: true,
		})
		oldResult <- err
	}()
	select {
	case <-preparationStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("old profile did not enter private preparation")
	}
	newResult := make(chan error, 1)
	go func() {
		result, err := manager.ProbeProfileCapabilities(context.Background(), agent.ID(), ProfileCapabilityRequest{
			Context: ProfileProbeContext{Scope: oldContext.Scope, Env: map[string]string{"PROFILE": "new"}}, Refresh: true,
		})
		if err == nil && result.Capabilities.Status != StatusOK {
			err = errors.New("new profile probe did not publish successful capabilities")
		}
		newResult <- err
	}()
	select {
	case <-recoveryCancelled:
	case <-time.After(time.Second):
		releaseOnce.Do(func() { close(forceRecoveryRelease) })
		<-oldResult
		<-newResult
		t.Fatal("superseded private preparation continued after the scoped profile changed")
	}
	select {
	case err := <-oldResult:
		if !errors.Is(err, errProfileContextChanged) {
			t.Fatalf("old profile result = %v, want superseded-context result", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("superseded profile flight did not finish")
	}
	select {
	case err := <-newResult:
		if err != nil {
			t.Fatalf("current profile result: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("current profile flight did not finish after superseded preparation canceled")
	}
}

func TestProfileManagedRuntimeProbeWaiterCancellationDoesNotCancelFlight(t *testing.T) {
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseProbe) })
	var requests atomic.Int32
	manager, server := newProfileProbeTestManager(t, agents.NewClaudeACP(), nil, func(w http.ResponseWriter, _ agentctlutil.ProbeRequest) {
		if requests.Add(1) == 1 {
			close(probeStarted)
			<-releaseProbe
		}
		data, _ := json.Marshal(agentctlutil.ProbeResponse{Success: true, Models: []agentctlutil.ProbeModel{{ID: "claude-sonnet"}}})
		_, _ = w.Write(data)
	})
	defer server.Close()
	profileContext := ProfileProbeContext{Scope: "user-1:profile-1", Env: map[string]string{"PROFILE": "same"}}
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, err := manager.ProbeProfileCapabilities(firstCtx, "claude-acp", ProfileCapabilityRequest{Context: profileContext})
		firstResult <- err
	}()
	select {
	case <-probeStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("profile probe did not start")
	}
	secondResult := make(chan error, 1)
	go func() {
		_, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{Context: profileContext})
		secondResult <- err
	}()
	cancelFirst()
	releaseOnce.Do(func() { close(releaseProbe) })
	for _, result := range []<-chan error{firstResult, secondResult} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatalf("single-flight waiter result: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("single-flight waiter did not finish")
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("provider probes = %d, want one shared flight", got)
	}
}

func TestProfileRefreshGenerationIsScopedToProfileContext(t *testing.T) {
	var requests atomic.Int32
	manager, server := newProfileProbeTestManager(t, agents.NewClaudeACP(), nil, func(w http.ResponseWriter, req agentctlutil.ProbeRequest) {
		requests.Add(1)
		version := req.InferenceConfig.Env["PROFILE"]
		data, _ := json.Marshal(agentctlutil.ProbeResponse{
			Success: true,
			Models:  []agentctlutil.ProbeModel{{ID: "codex", Name: "Codex"}},
			RuntimeInfo: &agents.RuntimeInfo{
				Scope: "host", Components: []agents.RuntimeComponent{{
					Role: agents.RuntimeComponentBridge, Name: "Claude", ObservedVersion: version,
				}},
			},
		})
		_, _ = w.Write(data)
	})
	defer server.Close()

	profileA := ProfileProbeContext{Scope: "user-1:profile-a", Env: map[string]string{"PROFILE": "a"}}
	profileB := ProfileProbeContext{Scope: "user-1:profile-b", Env: map[string]string{"PROFILE": "b"}}
	firstA, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{Context: profileA, Refresh: true})
	if err != nil {
		t.Fatalf("refresh profile A: %v", err)
	}
	if _, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{Context: profileB, Refresh: true}); err != nil {
		t.Fatalf("refresh profile B: %v", err)
	}
	resolution, err := manager.ResolveModelConfig(context.Background(), "claude-acp", ModelConfigResolutionRequest{
		Model: "codex", ProfileContext: &profileA,
	})
	if err != nil {
		t.Fatalf("resolve model options for profile A: %v", err)
	}
	if resolution.ContextRevision != firstA.ContextRevision {
		t.Fatalf("profile B refresh changed profile A revision: got %q, want %q", resolution.ContextRevision, firstA.ContextRevision)
	}
	cachedA, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{Context: profileA})
	if err != nil {
		t.Fatalf("read cached profile A: %v", err)
	}
	if cachedA.Capabilities.RuntimeInfo == nil || cachedA.Capabilities.RuntimeInfo.Components[0].ObservedVersion != "a" {
		t.Fatalf("profile B observation leaked into profile A: %#v", cachedA.Capabilities.RuntimeInfo)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("probe count = %d, want 3 (A baseline, B baseline, A model options)", got)
	}
}

func TestProfileRuntimeObservationRejectedAfterActivation(t *testing.T) {
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseProbe) }) }
	t.Cleanup(release)
	var requests atomic.Int32
	manager, server := newProfileProbeTestManager(t, agents.NewClaudeACP(), nil, func(w http.ResponseWriter, _ agentctlutil.ProbeRequest) {
		request := requests.Add(1)
		if request == 1 {
			close(probeStarted)
			<-releaseProbe
		}
		version := "1.2.4"
		if request == 1 {
			version = "1.2.3"
		}
		data, _ := json.Marshal(agentctlutil.ProbeResponse{
			Success: true,
			Models:  []agentctlutil.ProbeModel{{ID: "codex", Name: "Codex"}},
			RuntimeInfo: &agents.RuntimeInfo{
				Scope: "host", Components: []agents.RuntimeComponent{{
					Role: agents.RuntimeComponentBridge, Name: "Claude", ObservedVersion: version,
				}},
			},
		})
		_, _ = w.Write(data)
	})
	defer server.Close()

	type probeResult struct {
		result ProfileCapabilityResult
		err    error
	}
	resultCh := make(chan probeResult, 1)
	go func() {
		result, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{
			Context: ProfileProbeContext{Scope: "user-1:profile-a"}, Refresh: true,
		})
		resultCh <- probeResult{result: result, err: err}
	}()
	select {
	case <-probeStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("profile probe did not reach the provider")
	}

	manager.PublishCapabilities("claude-acp", AgentCapabilities{AgentType: "claude-acp", Status: StatusOK})
	release()
	select {
	case result := <-resultCh:
		if result.err == nil {
			t.Fatalf("obsolete profile probe returned success: %#v", result.result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("profile probe did not finish after release")
	}
	current, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{
		Context: ProfileProbeContext{Scope: "user-1:profile-a"},
	})
	if err != nil {
		t.Fatalf("probe current runtime after activation: %v", err)
	}
	if current.Capabilities.RuntimeInfo == nil || current.Capabilities.RuntimeInfo.Components[0].ObservedVersion != "1.2.4" {
		t.Fatalf("current profile reused pre-activation observation: %#v", current.Capabilities.RuntimeInfo)
	}
}

func TestProfileProbeRejectsCommandResolvedAcrossRuntimeActivation(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	selectionReader := &blockingProfileProbeSelectionReader{
		started: make(chan struct{}), release: make(chan struct{}),
	}
	t.Cleanup(func() { selectionReader.releaseOnce.Do(func() { close(selectionReader.release) }) })
	var requests atomic.Int32
	manager, server := newProfileProbeTestManager(t, agents.NewOpenCodeACP(), selectionReader, func(w http.ResponseWriter, req agentctlutil.ProbeRequest) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"success":true,"models":[{"id":"opencode","name":"OpenCode"}]}`))
	})
	defer server.Close()

	type probeResult struct {
		result ProfileCapabilityResult
		err    error
	}
	resultCh := make(chan probeResult, 1)
	go func() {
		result, err := manager.ProbeProfileCapabilities(context.Background(), "opencode-acp", ProfileCapabilityRequest{
			Context: ProfileProbeContext{Scope: "user-1:profile-opencode"}, Refresh: true,
		})
		resultCh <- probeResult{result: result, err: err}
	}()
	select {
	case <-selectionReader.started:
	case <-time.After(5 * time.Second):
		t.Fatal("managed command resolution did not start")
	}

	manager.PublishCapabilities("opencode-acp", AgentCapabilities{AgentType: "opencode-acp", Status: StatusOK})
	selectionReader.releaseOnce.Do(func() { close(selectionReader.release) })
	select {
	case result := <-resultCh:
		if result.err == nil {
			t.Fatalf("probe returned success using a command snapshot that crossed runtime activation: %#v", result.result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("profile probe did not finish after command resolution was released")
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("provider received %d probes for an obsolete command snapshot", got)
	}
}

type blockingProfileProbeSelectionReader struct {
	started     chan struct{}
	release     chan struct{}
	mu          sync.Mutex
	calls       int
	releaseOnce sync.Once
}

func (r *blockingProfileProbeSelectionReader) Get(context.Context, string, string) (managedruntime.Selection, bool, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	if call == 1 {
		close(r.started)
		<-r.release
		return managedruntime.Selection{Package: "opencode-ai", Version: "1.18.5"}, true, nil
	}
	return managedruntime.Selection{Package: "opencode-ai", Version: "1.19.0"}, true, nil
}

func newProfileProbeTestManager(
	t *testing.T,
	agent agents.Agent,
	selectionReader managedruntime.SelectionReader,
	handleProbe func(http.ResponseWriter, agentctlutil.ProbeRequest),
) (*Manager, *httptest.Server) {
	return newProfileProbeTestManagerWithCapabilities(
		t, agent, selectionReader, []string{"launch-wrapper.v1"}, handleProbe,
	)
}

func newProfileProbeTestManagerWithCapabilities(
	t *testing.T,
	agent agents.Agent,
	selectionReader managedruntime.SelectionReader,
	capabilities []string,
	handleProbe func(http.ResponseWriter, agentctlutil.ProbeRequest),
) (*Manager, *httptest.Server) {
	return newProfileProbeTestManagerWithRequestHandler(
		t, agent, selectionReader, capabilities,
		func(w http.ResponseWriter, _ *http.Request, request agentctlutil.ProbeRequest) {
			handleProbe(w, request)
		},
	)
}

func newProfileProbeTestManagerWithRequestHandler(
	t *testing.T,
	agent agents.Agent,
	selectionReader managedruntime.SelectionReader,
	capabilities []string,
	handleProbe func(http.ResponseWriter, *http.Request, agentctlutil.ProbeRequest),
) (*Manager, *httptest.Server) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity" {
			data, _ := json.Marshal(map[string]any{"capabilities": capabilities})
			_, _ = w.Write(data)
			return
		}
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/api/v1/inference/probe" {
			http.NotFound(w, r)
			return
		}
		var req agentctlutil.ProbeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode probe request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		handleProbe(w, r, req)
	}))
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	reg := registry.NewRegistry(log)
	if err := reg.Register(agent); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(reg, host, port, nil, log)
	manager.managedRuntimeSelections = selectionReader
	manager.instances[agent.ID()] = &instance{
		agentType: agent.ID(), instanceID: "test-instance", workDir: t.TempDir(),
		client: agentctlclient.NewClient(host, port, log),
	}
	return manager, server
}
