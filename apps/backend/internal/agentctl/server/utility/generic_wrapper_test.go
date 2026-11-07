package utility

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func genericWrapperFixture(t *testing.T) (*InferenceConfigDTO, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX process forwarding")
	}
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "independent launcher")
	child := filepath.Join(dir, "mock-agent")
	receipt := filepath.Join(dir, "wrapper-receipt")
	writeProfileProbeFixture(t, wrapper, `#!/bin/sh
printf '%s\n' "$@" > "$WRAPPER_RECEIPT"
shift
exec "$@"
`)
	writeProfileProbeFixture(t, child, `#!/bin/sh
while IFS= read -r line; do
 id=$(printf '%s' "$line" | sed -n 's/.*"id":\([^,}]*\).*/\1/p')
 case "$line" in
 *'"initialize"'*) printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":1,"agentCapabilities":{}}}\n' "$id" ;;
 *'"session/new"'*) printf '{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"wrapper-test","configOptions":[]}}\n' "$id" ;;
 *'"session/prompt"'*) printf '{"jsonrpc":"2.0","id":%s,"result":{"stopReason":"end_turn"}}\n' "$id" ;;
 esac
done
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	raw, err := json.Marshal(map[string]any{
		"command": []string{"mock-agent"}, "work_dir": dir,
		"env":            map[string]string{"WRAPPER_RECEIPT": receipt},
		"cli_flags":      []string{"", "space value", "$(false)"},
		"launch_wrapper": map[string]any{"version": 1, "source": "profile", "argv": []string{wrapper, "{{kandev.work_dir}}"}},
	})
	require.NoError(t, err)
	var cfg InferenceConfigDTO
	require.NoError(t, json.Unmarshal(raw, &cfg))
	return &cfg, receipt
}

// @covers AC-AGENTS-WRAPPERS-003.1 AC-AGENTS-WRAPPERS-003.2 AC-AGENTS-WRAPPERS-003.5
func TestGenericWrapperProbeAndPrompt(t *testing.T) {
	for _, prompt := range []bool{false, true} {
		t.Run(map[bool]string{false: "probe", true: "prompt"}[prompt], func(t *testing.T) {
			cfg, receipt := genericWrapperFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			executor := NewACPInferenceExecutor(zap.NewNop())
			if prompt {
				resp, err := executor.Execute(ctx, &PromptRequest{AgentID: "mock-agent", Prompt: "fixture only", InferenceConfig: cfg})
				require.NoError(t, err)
				require.True(t, resp.Success, resp.Error)
			} else {
				resp, err := executor.Probe(ctx, &ProbeRequest{AgentID: "mock-agent", ProfileContext: true, InferenceConfig: cfg})
				require.NoError(t, err)
				require.True(t, resp.Success, resp.Error)
			}
			got, err := os.ReadFile(receipt)
			require.NoError(t, err, "successful execution must include a wrapper receipt")
			require.Equal(t, cfg.WorkDir+"\n"+filepath.Join(cfg.WorkDir, "mock-agent")+"\n\nspace value\n$(false)\n", string(got))
		})
	}
}
