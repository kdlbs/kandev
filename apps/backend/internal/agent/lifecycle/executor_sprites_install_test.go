package lifecycle

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestExtractAgentID(t *testing.T) {
	tests := []struct {
		name     string
		methodID string
		want     string
	}{
		{name: "full method ID", methodID: "agent:claude-acp:env:ANTHROPIC_API_KEY", want: "claude-acp"},
		{name: "files method", methodID: "agent:claude-acp:files:0", want: "claude-acp"},
		{name: "codex env", methodID: "agent:codex-acp:env:OPENAI_API_KEY", want: "codex-acp"},
		{name: "two parts only", methodID: "agent:gemini", want: "gemini"},
		{name: "non-agent prefix", methodID: "system:some-key:value", want: ""},
		{name: "empty string", methodID: "", want: ""},
		{name: "single segment", methodID: "agent", want: ""},
		{name: "no colon", methodID: "agentclaude", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAgentID(tt.methodID)
			require.Equal(t, tt.want, got)
		})
	}
}

// stubAgent wraps MockAgent to allow custom ID and install script.
type stubAgent struct {
	*agents.MockAgent
	id            string
	installScript string
}

func (a *stubAgent) ID() string            { return a.id }
func (a *stubAgent) InstallScript() string { return a.installScript }

type mockAgentLister struct {
	agents []agents.Agent
}

func (m *mockAgentLister) ListEnabled() []agents.Agent { return m.agents }

func newSpritesExecutorWithAgents(ags []agents.Agent) *SpritesExecutor {
	return &SpritesExecutor{
		agentList: &mockAgentLister{agents: ags},
		logger:    newTestLogger(),
		tokens:    make(map[string]string),
		proxies:   make(map[string]*SpritesProxySession),
		mu:        sync.RWMutex{},
	}
}

func TestCollectAgentInstallScripts(t *testing.T) {
	claude := &stubAgent{MockAgent: agents.NewMockAgent(), id: "claude-acp", installScript: "npm i -g claude"}
	codex := &stubAgent{MockAgent: agents.NewMockAgent(), id: "codex-acp", installScript: "pip install codex"}
	gemini := &stubAgent{MockAgent: agents.NewMockAgent(), id: "gemini", installScript: "gem install gemini"}

	t.Run("no agents and no metadata returns empty", func(t *testing.T) {
		r := newSpritesExecutorWithAgents([]agents.Agent{claude, codex})
		got := r.collectAgentInstallScripts(&ExecutorCreateRequest{
			Metadata: map[string]interface{}{},
		})
		require.Empty(t, got)
	})

	t.Run("current task agent included", func(t *testing.T) {
		r := newSpritesExecutorWithAgents([]agents.Agent{claude, codex})
		got := r.collectAgentInstallScripts(&ExecutorCreateRequest{
			AgentConfig: claude,
			Metadata:    map[string]interface{}{},
		})
		require.Equal(t, []string{"npm i -g claude"}, got)
	})

	t.Run("agents from remote_credentials metadata", func(t *testing.T) {
		credsJSON, _ := json.Marshal([]string{"agent:codex-acp:env:OPENAI_API_KEY"})
		r := newSpritesExecutorWithAgents([]agents.Agent{claude, codex})
		got := r.collectAgentInstallScripts(&ExecutorCreateRequest{
			Metadata: map[string]interface{}{"remote_credentials": string(credsJSON)},
		})
		require.Equal(t, []string{"pip install codex"}, got)
	})

	t.Run("agents from remote_auth_secrets metadata", func(t *testing.T) {
		secretsJSON, _ := json.Marshal(map[string]string{"agent:gemini:env:API_KEY": "secret-1"})
		r := newSpritesExecutorWithAgents([]agents.Agent{claude, codex, gemini})
		got := r.collectAgentInstallScripts(&ExecutorCreateRequest{
			Metadata: map[string]interface{}{"remote_auth_secrets": string(secretsJSON)},
		})
		require.Equal(t, []string{"gem install gemini"}, got)
	})

	t.Run("deduplicates agents from multiple sources", func(t *testing.T) {
		credsJSON, _ := json.Marshal([]string{"agent:claude-acp:files:0"})
		secretsJSON, _ := json.Marshal(map[string]string{"agent:claude-acp:env:KEY": "s1"})
		r := newSpritesExecutorWithAgents([]agents.Agent{claude})
		got := r.collectAgentInstallScripts(&ExecutorCreateRequest{
			AgentConfig: claude,
			Metadata: map[string]interface{}{
				"remote_credentials":  string(credsJSON),
				"remote_auth_secrets": string(secretsJSON),
			},
		})
		require.Equal(t, []string{"npm i -g claude"}, got)
	})

	t.Run("nil agentList returns empty", func(t *testing.T) {
		r := &SpritesExecutor{
			logger:  newTestLogger(),
			tokens:  make(map[string]string),
			proxies: make(map[string]*SpritesProxySession),
		}
		got := r.collectAgentInstallScripts(&ExecutorCreateRequest{
			AgentConfig: claude,
			Metadata:    map[string]interface{}{},
		})
		require.Empty(t, got)
	})
}
