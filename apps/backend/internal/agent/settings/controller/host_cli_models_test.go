package controller

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostcli"
	"github.com/kandev/kandev/internal/agent/hostutility"
)

// hostCLICapabilities serves one fixed bridge catalogue to the model endpoints.
type hostCLICapabilities struct {
	caps hostutility.AgentCapabilities
}

func (f *hostCLICapabilities) Get(string) (hostutility.AgentCapabilities, bool) {
	return f.caps, true
}

func (f *hostCLICapabilities) Refresh(context.Context, string) (hostutility.AgentCapabilities, error) {
	return f.caps, nil
}

func (f *hostCLICapabilities) ResolveModelConfig(
	context.Context,
	string,
	hostutility.ModelConfigResolutionRequest,
) (hostutility.ModelConfigResolution, error) {
	return hostutility.ModelConfigResolution{}, nil
}

// stubCLIRunner answers app-server JSON-RPC with a fixed model/list result and
// counts how many listings actually ran.
type stubCLIRunner struct {
	mu        sync.Mutex
	starts    int
	listJSON  string
	startErr  error
	processes []*stubCLIProcess
}

type stubCLIProcess struct {
	in     *io.PipeWriter
	inRead *io.PipeReader
	out    *io.PipeReader
	outW   *io.PipeWriter
	done   chan struct{}
}

func (p *stubCLIProcess) Stdin() io.Writer  { return p.in }
func (p *stubCLIProcess) Stdout() io.Reader { return p.out }
func (p *stubCLIProcess) Wait() error       { <-p.done; return nil }
func (p *stubCLIProcess) Kill() error {
	select {
	case <-p.done:
	default:
		close(p.done)
		_ = p.in.Close()
		_ = p.outW.Close()
	}
	return nil
}

func (r *stubCLIRunner) Output(context.Context, []string) (string, error) { return "", nil }

func (r *stubCLIRunner) Start(_ context.Context, _ []string) (hostcli.Process, error) {
	r.mu.Lock()
	r.starts++
	startErr := r.startErr
	listJSON := r.listJSON
	r.mu.Unlock()
	if startErr != nil {
		return nil, startErr
	}
	inRead, in := io.Pipe()
	out, outW := io.Pipe()
	proc := &stubCLIProcess{in: in, inRead: inRead, out: out, outW: outW, done: make(chan struct{})}
	r.mu.Lock()
	r.processes = append(r.processes, proc)
	r.mu.Unlock()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := inRead.Read(buf)
			if n > 0 && strings.Contains(string(buf[:n]), "model/list") {
				_, _ = io.WriteString(outW, listJSON+"\n")
			}
			if err != nil {
				return
			}
		}
	}()
	return proc, nil
}

func (r *stubCLIRunner) startCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts
}

const codexListJSON = `{"id":2,"result":{"data":[` +
	`{"id":"gpt-6-astra","model":"gpt-6-astra","displayName":"GPT-6-Astra","description":"Frontier","isDefault":true},` +
	`{"id":"gpt-6-sol","model":"gpt-6-sol","displayName":"GPT-6-Sol","description":"Workhorse"}]}}`

func bridgeCapabilities(ids ...string) hostutility.AgentCapabilities {
	caps := hostutility.AgentCapabilities{Status: hostutility.StatusOK, CurrentModelID: ids[0]}
	for _, id := range ids {
		caps.Models = append(caps.Models, hostutility.Model{ID: id, Name: id})
	}
	return caps
}

func newHostCLIController(t *testing.T, runner hostcli.Runner, caps hostutility.AgentCapabilities) *Controller {
	t.Helper()
	ctrl := newTestController(map[string]agents.Agent{
		"codex-acp":  agents.NewCodexACP(),
		"claude-acp": agents.NewClaudeACP(),
		"test-agent": &testAgent{id: "test-agent", name: "test-agent", enabled: true},
	})
	ctrl.hostUtility = &hostCLICapabilities{caps: caps}
	ctrl.SetHostCLIRunner(runner)
	return ctrl
}

// @covers AC-AGENTS-HOST-CLI-002.1
func TestFetchDynamicModelsMergesHostCLIModels(t *testing.T) {
	runner := &stubCLIRunner{listJSON: codexListJSON}
	ctrl := newHostCLIController(t, runner, bridgeCapabilities("gpt-6-sol", "legacy-bridge-model"))

	resp, err := ctrl.FetchDynamicModels(context.Background(), "codex-acp", true)
	if err != nil {
		t.Fatalf("FetchDynamicModels: %v", err)
	}
	ids := make([]string, 0, len(resp.Models))
	sources := make(map[string]string, len(resp.Models))
	for _, model := range resp.Models {
		ids = append(ids, model.ID)
		sources[model.ID] = model.Source
	}
	if strings.Join(ids, ",") != "gpt-6-astra,gpt-6-sol,legacy-bridge-model" {
		t.Fatalf("models = %v, want CLI entries first then bridge-only entries", ids)
	}
	if sources["gpt-6-astra"] != modelEntrySourceCLI || sources["legacy-bridge-model"] != modelEntrySourceACP {
		t.Fatalf("sources = %v", sources)
	}
	if resp.Discovery == nil {
		t.Fatal("discovery block missing")
	}
	if resp.Discovery.Source != modelDiscoverySourceCLI || resp.Discovery.Status != hostCLIModelStatusOK {
		t.Fatalf("discovery = %+v", resp.Discovery)
	}
	if !resp.Discovery.AllowsCustomModel || resp.Discovery.Executable != "codex" {
		t.Fatalf("discovery = %+v", resp.Discovery)
	}
	if resp.Discovery.CheckedAt == nil {
		t.Fatal("discovery.checked_at missing after a successful listing")
	}
}

// @covers AC-AGENTS-HOST-CLI-002.5 AC-AGENTS-HOST-CLI-004.3
func TestFetchDynamicModelsFallsBackWhenCLIFails(t *testing.T) {
	cases := map[string]struct {
		startErr   error
		listJSON   string
		wantStatus string
	}{
		"not installed": {startErr: hostcli.ErrNotInstalled, wantStatus: hostCLIModelStatusNotInstalled},
		"not logged in": {
			listJSON:   `{"id":2,"error":{"code":-32000,"message":"Not logged in. Run codex login."}}`,
			wantStatus: hostCLIModelStatusNotLoggedIn,
		},
		"cli error": {
			listJSON:   `{"id":2,"error":{"code":-32601,"message":"boom"}}`,
			wantStatus: hostCLIModelStatusFailed,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			runner := &stubCLIRunner{listJSON: tc.listJSON, startErr: tc.startErr}
			ctrl := newHostCLIController(t, runner, bridgeCapabilities("bridge-a", "bridge-b"))

			resp, err := ctrl.FetchDynamicModels(context.Background(), "codex-acp", true)
			if err != nil {
				t.Fatalf("FetchDynamicModels: %v", err)
			}
			if len(resp.Models) != 2 || resp.Models[0].ID != "bridge-a" {
				t.Fatalf("models = %+v, want the bridge list preserved", resp.Models)
			}
			if resp.Discovery == nil || resp.Discovery.Status != tc.wantStatus {
				t.Fatalf("discovery = %+v, want status %q", resp.Discovery, tc.wantStatus)
			}
			if resp.Discovery.Source != modelDiscoverySourceProbe || resp.Discovery.Error == "" {
				t.Fatalf("discovery = %+v", resp.Discovery)
			}
			if !resp.Discovery.AllowsCustomModel {
				t.Fatal("a failed listing must still allow a typed model identifier")
			}
		})
	}
}

// @covers AC-AGENTS-HOST-CLI-002.2
func TestFetchDynamicModelsSkipsAgentsWithoutModelSource(t *testing.T) {
	runner := &stubCLIRunner{listJSON: codexListJSON}
	ctrl := newHostCLIController(t, runner, bridgeCapabilities("opus[1m]", "sonnet"))

	resp, err := ctrl.FetchDynamicModels(context.Background(), "claude-acp", true)
	if err != nil {
		t.Fatalf("FetchDynamicModels: %v", err)
	}
	if len(resp.Models) != 2 || resp.Models[0].ID != "opus[1m]" {
		t.Fatalf("models = %+v", resp.Models)
	}
	if resp.Discovery == nil || resp.Discovery.Status != hostCLIModelStatusSkipped {
		t.Fatalf("discovery = %+v", resp.Discovery)
	}
	if resp.Discovery.Source != modelDiscoverySourceProbe || !resp.Discovery.AllowsCustomModel {
		t.Fatalf("discovery = %+v", resp.Discovery)
	}
	if runner.startCount() != 0 {
		t.Fatalf("started %d processes for a CLI with no model listing", runner.startCount())
	}
}

// @covers AC-AGENTS-HOST-CLI-002.6
func TestFetchDynamicModelsLeavesOtherAgentsUnchanged(t *testing.T) {
	runner := &stubCLIRunner{listJSON: codexListJSON}
	ctrl := newHostCLIController(t, runner, bridgeCapabilities("m1"))

	resp, err := ctrl.FetchDynamicModels(context.Background(), "test-agent", true)
	if err != nil {
		t.Fatalf("FetchDynamicModels: %v", err)
	}
	if resp.Discovery != nil {
		t.Fatalf("discovery = %+v, want nil for an agent without a host CLI", resp.Discovery)
	}
	if len(resp.Models) != 1 || resp.Models[0].Source != "" {
		t.Fatalf("models = %+v, want the untouched bridge list", resp.Models)
	}
}

// @covers AC-AGENTS-HOST-CLI-002.4
func TestHostCLIModelsCacheAndRefresh(t *testing.T) {
	runner := &stubCLIRunner{listJSON: codexListJSON}
	ctrl := newHostCLIController(t, runner, bridgeCapabilities("bridge-a"))

	// Before any discovery the cache-only path reports pending and spawns
	// nothing, so listing agents never blocks on a vendor process.
	resp, err := ctrl.FetchDynamicModels(context.Background(), "codex-acp", false)
	if err != nil {
		t.Fatalf("FetchDynamicModels: %v", err)
	}
	if resp.Discovery == nil || resp.Discovery.Status != hostCLIModelStatusPending {
		t.Fatalf("discovery = %+v, want pending", resp.Discovery)
	}
	if runner.startCount() != 0 {
		t.Fatalf("started %d processes without a refresh", runner.startCount())
	}

	if _, err := ctrl.FetchDynamicModels(context.Background(), "codex-acp", true); err != nil {
		t.Fatalf("FetchDynamicModels: %v", err)
	}
	if runner.startCount() != 1 {
		t.Fatalf("start count = %d after one refresh", runner.startCount())
	}

	// A later read serves the discovered catalogue from the cache.
	cached, err := ctrl.FetchDynamicModels(context.Background(), "codex-acp", false)
	if err != nil {
		t.Fatalf("FetchDynamicModels: %v", err)
	}
	if runner.startCount() != 1 {
		t.Fatalf("start count = %d, want the cached catalogue reused", runner.startCount())
	}
	if len(cached.Models) != 3 || cached.Models[0].ID != "gpt-6-astra" {
		t.Fatalf("cached models = %+v", cached.Models)
	}

	// Invalidation (a successful CLI update) forces the next refresh to re-read.
	ctrl.InvalidateHostCLIModels("codex-acp")
	if _, err := ctrl.FetchDynamicModels(context.Background(), "codex-acp", true); err != nil {
		t.Fatalf("FetchDynamicModels: %v", err)
	}
	if runner.startCount() != 2 {
		t.Fatalf("start count = %d after invalidation", runner.startCount())
	}
}

func TestWarmHostCLIModelsOnlyVisitsModelSourceAgents(t *testing.T) {
	runner := &stubCLIRunner{listJSON: codexListJSON}
	ctrl := newHostCLIController(t, runner, bridgeCapabilities("bridge-a"))

	ctrl.WarmHostCLIModels(context.Background())

	if runner.startCount() != 1 {
		t.Fatalf("start count = %d, want one listing for codex only", runner.startCount())
	}
	resp, err := ctrl.FetchDynamicModels(context.Background(), "codex-acp", false)
	if err != nil {
		t.Fatalf("FetchDynamicModels: %v", err)
	}
	if resp.Discovery == nil || resp.Discovery.Status != hostCLIModelStatusOK {
		t.Fatalf("discovery = %+v, want the warmed catalogue", resp.Discovery)
	}
}

func TestClassifyHostCLIModelError(t *testing.T) {
	cases := map[error]string{
		hostcli.ErrNotInstalled: hostCLIModelStatusNotInstalled,
		hostcli.ErrNotLoggedIn:  hostCLIModelStatusNotLoggedIn,
		hostcli.ErrTimeout:      hostCLIModelStatusTimeout,
		errors.New("other"):     hostCLIModelStatusFailed,
	}
	for err, want := range cases {
		status, message := classifyHostCLIModelError(err)
		if status != want || message == "" {
			t.Fatalf("classifyHostCLIModelError(%v) = %q, %q", err, status, message)
		}
	}
}

// @covers AC-AGENTS-HOST-CLI-003.3
func TestHostCLIInstallSucceededRediscoversModels(t *testing.T) {
	runner := &stubCLIRunner{listJSON: codexListJSON}
	ctrl := newHostCLIController(t, runner, bridgeCapabilities("bridge-a"))

	ctrl.WarmHostCLIModels(context.Background())
	if runner.startCount() != 1 {
		t.Fatalf("start count = %d after warmup", runner.startCount())
	}

	// A second warmup inside the freshness window must not re-read the CLI.
	ctrl.WarmHostCLIModels(context.Background())
	if runner.startCount() != 1 {
		t.Fatalf("start count = %d, want the fresh catalogue reused", runner.startCount())
	}

	// The existing install flow replaced the binary on disk, so the cached
	// catalogue is dropped and rediscovered without an explicit refresh.
	ctrl.hostCLIInstallSucceeded("codex-acp")
	waitForStartCount(t, runner, 2)

	resp, err := ctrl.FetchDynamicModels(context.Background(), "codex-acp", false)
	if err != nil {
		t.Fatalf("FetchDynamicModels: %v", err)
	}
	if resp.Discovery == nil || resp.Discovery.Status != hostCLIModelStatusOK {
		t.Fatalf("discovery = %+v, want the rediscovered catalogue", resp.Discovery)
	}
}

func TestHostCLIInstallSucceededIgnoresAgentsWithoutCLI(t *testing.T) {
	runner := &stubCLIRunner{listJSON: codexListJSON}
	ctrl := newHostCLIController(t, runner, bridgeCapabilities("bridge-a"))

	ctrl.hostCLIInstallSucceeded("test-agent")

	if runner.startCount() != 0 {
		t.Fatalf("start count = %d for an agent without a host CLI", runner.startCount())
	}
}

func waitForStartCount(t *testing.T, runner *stubCLIRunner, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runner.startCount() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("start count = %d, want %d", runner.startCount(), want)
}
