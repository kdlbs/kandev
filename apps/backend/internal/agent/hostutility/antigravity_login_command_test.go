package hostutility

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/server/api"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// antigravityLoginFixtureStartMarker proves the configure request passed the
// production handler and reached start. The fixture returns it as the start
// error so no child process or agent stream is created.
const antigravityLoginFixtureStartMarker = "antigravity-login-fixture-start-reached"

// antigravityLoginCommandAgent supplies deterministic local runtime commands
// while retaining the real Antigravity inference and runtime metadata.
type antigravityLoginCommandAgent struct {
	*agents.AntigravityACP
	command agents.Command
	env     map[string]string
}

func (a *antigravityLoginCommandAgent) ResolveLocalRuntime(
	context.Context,
) (agents.Command, map[string]string, error) {
	return a.command, a.env, nil
}

type antigravityLoginConfigureFixture struct {
	config       *config.InstanceConfig
	startReached chan struct{}
	handler      http.Handler
}

func newAntigravityLoginConfigureFixture(
	t *testing.T,
	log *logger.Logger,
) *antigravityLoginConfigureFixture {
	t.Helper()
	cfg := &config.InstanceConfig{
		Port:     0,
		WorkDir:  t.TempDir(),
		AgentEnv: []string{"KANDEV_HOST_UTILITY_FIXTURE=1"},
	}
	procMgr := process.NewManager(cfg, log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := procMgr.StopForTeardown(ctx); err != nil {
			t.Errorf("stop fixture process manager: %v", err)
		}
	})
	router := api.NewServer(cfg, procMgr, nil, nil, log).Router()
	startReached := make(chan struct{}, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/start" {
			select {
			case startReached <- struct{}{}:
			default:
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"success":false,"error":"`+antigravityLoginFixtureStartMarker+`"}`)
			return
		}
		router.ServeHTTP(w, r)
	})
	return &antigravityLoginConfigureFixture{
		config:       cfg,
		startReached: startReached,
		handler:      handler,
	}
}

func newAntigravityLoginFixtureClient(
	t *testing.T,
	server *httptest.Server,
	log *logger.Logger,
) *agentctlclient.Client {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse fixture server URL: %v", err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parse fixture server port: %v", err)
	}
	return agentctlclient.NewClient(parsed.Hostname(), port, log)
}

func TestAntigravityLoginConfigurePreservesCommand(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create test logger: %v", err)
	}
	harnessPath := "/fixtures/Antigravity Harness/localharness_external"
	cases := []struct {
		name string
		argv []string
	}{
		{
			name: "linux standalone server with uid flag",
			argv: []string{"/fixtures/antigravity/agy_acp_server.par", "--uid="},
		},
		{
			name: "executable only server",
			argv: []string{"/fixtures/antigravity/agy_acp_server.par"},
		},
		{
			name: "executable path with spaces",
			argv: []string{"/fixtures/Antigravity ACP/agy_acp_server.par", "--uid="},
		},
		{
			name: "windows shaped command",
			argv: []string{`C:\Program Files\Antigravity\agy_acp_server.exe`, "--uid="},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newAntigravityLoginConfigureFixture(t, log)
			server := httptest.NewServer(fixture.handler)
			t.Cleanup(server.Close)
			client := newAntigravityLoginFixtureClient(t, server, log)
			t.Cleanup(client.Close)
			ia := &antigravityLoginCommandAgent{
				AntigravityACP: agents.NewAntigravityACP(),
				command:        agents.NewCommand(tc.argv...),
				env:            map[string]string{acpcompat.AntigravityHarnessPathEnv: harnessPath},
			}
			manager := NewManager(nil, "", 0, nil, log)

			err := manager.configureAntigravityLoginInstance(
				context.Background(), &instance{client: client}, ia, &antigravityLoginAttempt{},
			)
			if err == nil || !strings.Contains(err.Error(), antigravityLoginFixtureStartMarker) {
				t.Fatalf("configure error = %v, want controlled start marker %q",
					err, antigravityLoginFixtureStartMarker)
			}
			if !strings.Contains(err.Error(), "start antigravity login instance:") {
				t.Fatalf("configure error = %v, want a start failure after a successful configure", err)
			}
			select {
			case <-fixture.startReached:
			case <-time.After(5 * time.Second):
				t.Fatal("configure did not reach start")
			}
			if got := fixture.config.AgentCommand; got != tc.argv[0] {
				t.Fatalf("received AgentCommand = %q, want %q", got, tc.argv[0])
			}
			if got := fixture.config.AgentArgs; !slices.Equal(got, tc.argv) {
				t.Fatalf("received AgentArgs = %#v, want %#v", got, tc.argv)
			}
			wantEnv := acpcompat.AntigravityHarnessPathEnv + "=" + harnessPath
			if !slices.Contains(fixture.config.AgentEnv, wantEnv) {
				t.Fatalf("received AgentEnv = %#v, want %q", fixture.config.AgentEnv, wantEnv)
			}
			if !slices.Contains(fixture.config.AgentEnv, "KANDEV_HOST_UTILITY_FIXTURE=1") {
				t.Fatalf("received AgentEnv = %#v, want the inherited fixture variable", fixture.config.AgentEnv)
			}
		})
	}
}
