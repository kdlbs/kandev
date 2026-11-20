package utility

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/launchwrapper"
)

func TestGenericWrapperSecondaryDiscovery(t *testing.T) {
	cfg, receipt := genericWrapperFixture(t)
	child := filepath.Join(cfg.WorkDir, "mock-agent")
	writeProfileProbeFixture(t, child, "#!/bin/sh\nprintf 'provider/model\\n'\n")
	models, err := probeOpenCodeModels(context.Background(), child, cfg.WorkDir, true, cfg)
	require.NoError(t, err)
	require.Len(t, models, 1)
	got, err := os.ReadFile(receipt)
	require.NoError(t, err)
	require.Equal(t, cfg.WorkDir+"\n"+child+"\nmodels\n--refresh\n\nspace value\n$(false)\n", string(got))
}

func TestGenericWrapperNativeRefusal(t *testing.T) {
	cfg, receipt := genericWrapperFixture(t)
	executor := NewCodexAppServerInferenceExecutor(zap.NewNop())
	resp, err := executor.Probe(context.Background(), &ProbeRequest{InferenceConfig: cfg})
	require.NoError(t, err)
	require.False(t, resp.Success)
	require.Equal(t, ProbeFailureUnsupportedContext, resp.FailureCode)
	_, _, err = resolveCodexAppServerCommand(&InferenceConfigDTO{Command: []string{"codex", "app-server"}, LaunchWrapper: cfg.LaunchWrapper})
	require.Error(t, err)
	_, err = os.Stat(receipt)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestGenericWrapperFailureDoesNotLeak(t *testing.T) {
	for _, prompt := range []bool{false, true} {
		t.Run(map[bool]string{false: "probe", true: "prompt"}[prompt], func(t *testing.T) {
			cfg, _ := genericWrapperFixture(t)
			secret := "wrapper-private-credential"
			writeProfileProbeFixture(t, cfg.LaunchWrapper.Argv[0], "#!/bin/sh\nprintf '"+secret+"\\n' >&2\nexit 1\n")
			core, logs := observer.New(zap.DebugLevel)
			executor := NewACPInferenceExecutor(zap.New(core))
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
			defer cancel()
			var failure string
			if prompt {
				resp, err := executor.Execute(ctx, &PromptRequest{AgentID: "mock-agent", InferenceConfig: cfg})
				require.NoError(t, err)
				require.False(t, resp.Success)
				failure = resp.Error
			} else {
				resp, err := executor.Probe(ctx, &ProbeRequest{AgentID: "mock-agent", InferenceConfig: cfg})
				require.NoError(t, err)
				require.False(t, resp.Success)
				failure = resp.Error
			}
			require.NotContains(t, failure, secret)
			require.NotContains(t, failure, cfg.WorkDir)
			for _, entry := range logs.All() {
				require.NotContains(t, entry.Message, secret)
				require.NotContains(t, entry.ContextMap(), secret)
				for _, value := range entry.ContextMap() {
					require.NotContains(t, fmt.Sprint(value), secret)
				}
			}
		})
	}
}

func TestGenericWrapperRefusalDoesNotRunChild(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		cfg, receipt := genericWrapperFixture(t)
		if conflict {
			cfg.CommandPrefix = []string{"mock-agent"}
		} else {
			cfg.LaunchWrapper = launchwrapper.Profile([]string{filepath.Join(cfg.WorkDir, "missing wrapper")})
		}
		resp, err := NewACPInferenceExecutor(zap.NewNop()).Probe(context.Background(), &ProbeRequest{InferenceConfig: cfg})
		require.NoError(t, err)
		require.False(t, resp.Success)
		require.Equal(t, ProbeFailureUnsupportedContext, resp.FailureCode)
		_, err = os.Stat(receipt)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
}

func TestGenericWrapperEnvironmentStripsControlCredentials(t *testing.T) {
	cfg, _ := genericWrapperFixture(t)
	t.Setenv("AGENTCTL_BOOTSTRAP_NONCE", "host-control-secret")
	cfg.Env["AGENTCTL_BOOTSTRAP_NONCE"] = "profile-control-secret"
	cfg.Env["STRIPPED"] = "profile-value"
	cfg.StripEnv = []string{"STRIPPED"}
	env := sanitizeEnvForAgent(cfg)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		require.NotEqual(t, "AGENTCTL_BOOTSTRAP_NONCE", key)
		require.NotEqual(t, "STRIPPED", key)
	}
}
