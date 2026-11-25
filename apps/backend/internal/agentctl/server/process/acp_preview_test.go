package process

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/opencode"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestACPPreviewLaunchNegotiation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		enabled  bool
		selected uint16
		launches string
	}{
		{name: "disabled", selected: 1, launches: "1 "},
		{name: "v2", enabled: true, selected: 2, launches: "2 "},
		{name: "v1 selection", enabled: true, selected: 1, launches: "2 exit 1 "},
		{name: "legacy envelope", enabled: true, selected: 1, launches: "2 exit 1 "},
		{name: "explicit version error", enabled: true, selected: 1, launches: "2 exit 1 "},
		{name: "ambiguous response", enabled: true, launches: "2 "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runACPPreviewNegotiation(t, tc.name, tc.enabled, tc.selected, tc.launches)
		})
	}
}

func runACPPreviewNegotiation(t *testing.T, scenario string, enabled bool, selected uint16, transitions string) {
	t.Helper()
	workspace := t.TempDir()
	path := filepath.Join(workspace, "launches.txt")
	executable, err := os.Executable()
	require.NoError(t, err)
	manager := NewManager(&config.InstanceConfig{
		WorkDir: workspace, Protocol: agent.ProtocolACP, AgentType: "mock-agent",
		AgentEnv: append(os.Environ(), "KANDEV_ACP_PREVIEW_PEER="+scenario, "KANDEV_ACP_PREVIEW_TRANSITIONS="+path),
	}, newTestLogger(t))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, manager.StopForTeardown(ctx))
	})
	args := []string{executable, "-test.run=^TestACPPreviewProcessPeer$"}
	require.NoError(t, manager.ConfigureRuntime("", args, true, nil, "", nil, false, false, enabled,
		nil, nil, opencode.Runtime{}, launchwrapper.Spec{}))
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	generation, err := manager.StartWithGeneration(ctx)
	require.NoError(t, err)
	adpt := manager.GetAdapter()
	replacement, initialized, err := manager.InitializeAdapter(ctx, adpt, generation)
	if selected == 0 {
		require.ErrorContains(t, err, "initialization failed")
	} else {
		require.NoError(t, err)
		require.Equal(t, selected, manager.ACPProtocolVersion())
		id, err := replacement.NewSession(ctx, nil)
		require.NoError(t, err)
		require.Equal(t, "fixture-session", id)
	}
	if strings.Contains(transitions, "exit") {
		require.Greater(t, initialized, generation)
		require.NotSame(t, adpt, replacement)
	} else {
		require.Equal(t, generation, initialized)
		require.Same(t, adpt, replacement)
	}
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(body), transitions), "process transitions = %q, want %q", body, transitions)
}

func TestACPPreviewProcessPeer(t *testing.T) {
	scenario := os.Getenv("KANDEV_ACP_PREVIEW_PEER")
	if scenario == "" {
		return
	}
	path := os.Getenv("KANDEV_ACP_PREVIEW_TRANSITIONS")
	log, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	input := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for input.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params map[string]any  `json:"params"`
		}
		require.NoError(t, json.Unmarshal(input.Bytes(), &request))
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		switch request.Method {
		case "initialize":
			version := request.Params["protocolVersion"].(float64)
			_, err := log.WriteString(map[float64]string{1: "1 ", 2: "2 "}[version])
			require.NoError(t, err)
			if version == 1 {
				require.NotNil(t, request.Params["clientCapabilities"])
			}
			response = previewInitializeReply(response, scenario, int(version))
		case "session/new":
			response["result"] = map[string]any{"sessionId": "fixture-session"}
		default:
			t.Fatalf("unexpected request: %s", request.Method)
		}
		require.NoError(t, encoder.Encode(response))
	}
	require.NoError(t, input.Err())
	_, err = log.WriteString("exit ")
	require.NoError(t, err)
	require.NoError(t, log.Close())
	os.Exit(0)
}

func previewInitializeReply(response map[string]any, scenario string, version int) map[string]any {
	info := map[string]any{"name": "fixture", "version": "1"}
	if version == 1 || scenario == "v1 selection" {
		response["result"] = map[string]any{"protocolVersion": 1, "agentInfo": info, "agentCapabilities": map[string]any{}}
		return response
	}
	switch scenario {
	case "v2":
		response["result"] = map[string]any{"protocolVersion": 2, "info": info}
	case "legacy envelope":
		response["result"] = map[string]any{"protocolVersion": 2, "agentInfo": info}
	case "ambiguous response":
		response["result"] = map[string]any{"protocolVersion": 2}
	case "explicit version error":
		response["error"] = map[string]any{"code": -32022, "message": "unsupported version",
			"data": map[string]any{"supported": []int{1}, "requested": 2}}
	}
	return response
}
