package utility

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/processcontainment"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func utilitySandboxFixture(t *testing.T, cfg *InferenceConfigDTO) string {
	t.Helper()
	content := []byte("#!/bin/sh\n[ -z \"$UNAPPROVED_SECRET\" ] || exit 61\n[ \"$APPROVED_SECRET\" = fixture ] || exit 62\nprintf 'started' >> \"$SANDBOX_RECEIPT\"\nexec \"$@\"\n")
	digest := sha256.Sum256(content)
	files, err := sandbox.MaterializeRuntime(context.Background(), t.TempDir(), content, hex.EncodeToString(digest[:]), nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, files.Close()) })
	group, groupErr := processcontainment.Create()
	if groupErr != nil && os.Getenv("KANDEV_TEST_SANDBOX_CGROUP") != "1" {
		t.Skip("requires delegated Linux cgroup v2")
	}
	require.NoError(t, groupErr)
	t.Cleanup(func() { require.NoError(t, group.Kill(context.Background())); require.NoError(t, group.Remove()) })
	receipt := filepath.Join(cfg.WorkDir, "sandbox-receipt")
	cfg.Sandbox = &sandbox.Launch{Containment: group, Version: 1, AttemptID: "utility-attempt", Generation: 1, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests, EnvironmentKeys: []string{"APPROVED_SECRET", "SANDBOX_RECEIPT", "WRAPPER_RECEIPT"}}
	cfg.SandboxStarting = func() error { return nil }
	cfg.SandboxStartFailed = func() {}
	cfg.SandboxStarted = func(_ context.Context, owner *sandbox.ProcessOwner) error { _, err := owner.LiveSnapshot(); return err }
	cfg.Env["APPROVED_SECRET"] = "fixture"
	cfg.Env["UNAPPROVED_SECRET"] = "must-not-reach-supervisor"
	cfg.Env["SANDBOX_RECEIPT"] = receipt
	return receipt
}

func TestSandboxUtilityProcessesUseEnvelope(t *testing.T) {
	for _, kind := range []string{"probe", "prompt", "secondary", "no-profile-wrapper"} {
		t.Run(kind, func(t *testing.T) {
			cfg, wrapperReceipt := genericWrapperFixture(t)
			receipt := utilitySandboxFixture(t, cfg)
			if kind == "no-profile-wrapper" {
				cfg.LaunchWrapper = launchwrapper.Spec{}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			executor := NewACPInferenceExecutor(zap.NewNop())
			switch kind {
			case "probe", "no-profile-wrapper":
				response, err := executor.Probe(ctx, &ProbeRequest{AgentID: "mock-agent", InferenceConfig: cfg})
				require.NoError(t, err)
				require.True(t, response.Success, response.Error)
			case "prompt":
				response, err := executor.Execute(ctx, &PromptRequest{AgentID: "mock-agent", InferenceConfig: cfg})
				require.NoError(t, err)
				require.True(t, response.Success, response.Error)
			case "secondary":
				child := filepath.Join(cfg.WorkDir, "mock-agent")
				writeProfileProbeFixture(t, child, "#!/bin/sh\nprintf 'provider/model\\n'\n")
				models, err := probeOpenCodeModels(ctx, child, cfg.WorkDir, true, cfg)
				require.Error(t, err, "secondary discovery requires its own launch identity")
				require.Empty(t, models)
				_, err = os.Stat(receipt)
				require.ErrorIs(t, err, os.ErrNotExist)
				return
			}
			got, err := os.ReadFile(receipt)
			require.NoError(t, err)
			require.Equal(t, "started", string(got))
			if kind != "no-profile-wrapper" {
				_, err = os.Stat(wrapperReceipt)
				require.NoError(t, err, "profile wrapper must remain inside the sandbox")
			}
		})
	}
}

func TestSandboxUtilityRefusesChangedArtifact(t *testing.T) {
	cfg, receipt := genericWrapperFixture(t)
	utilitySandboxFixture(t, cfg)
	require.NoError(t, os.Chmod(cfg.Sandbox.Prefix[0], 0700))
	require.NoError(t, os.WriteFile(cfg.Sandbox.Prefix[0], []byte("#!/bin/sh\nexec \"$@\"\n"), 0500))
	response, err := NewACPInferenceExecutor(zap.NewNop()).Probe(context.Background(), &ProbeRequest{InferenceConfig: cfg})
	require.NoError(t, err)
	require.False(t, response.Success)
	_, err = os.Stat(receipt)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestSandboxUtilityRequiresAuthenticatedControl(t *testing.T) {
	cfg, _ := genericWrapperFixture(t)
	utilitySandboxFixture(t, cfg)
	cfg.LaunchWrapper.Argv = nil
	cfg.LaunchWrapper.Version = 0
	cfg.LaunchWrapper.Source = ""
	for _, authenticated := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		h := &Handler{authenticatedControl: func(*gin.Context) bool { return authenticated }}
		require.Equal(t, authenticated, h.admitWrapper(c, cfg))
		if !authenticated {
			require.Equal(t, http.StatusForbidden, recorder.Code)
		}
	}
}

func TestSandboxUtilityNativeProtocolRefusesEnvelope(t *testing.T) {
	cfg, _ := genericWrapperFixture(t)
	utilitySandboxFixture(t, cfg)
	cfg.LaunchWrapper.Argv = nil
	cfg.LaunchWrapper.Version = 0
	cfg.LaunchWrapper.Source = ""
	cfg.CLIFlags = nil
	cfg.Command = []string{"codex", "app-server"}
	_, _, err := resolveCodexAppServerCommand(cfg)
	require.Error(t, err)
}

func TestSandboxUtilityFailureDoesNotExposeRuntimeDetails(t *testing.T) {
	for _, prompt := range []bool{false, true} {
		t.Run(map[bool]string{false: "probe", true: "prompt"}[prompt], func(t *testing.T) {
			cfg, _ := genericWrapperFixture(t)
			utilitySandboxFixture(t, cfg)
			cfg.LaunchWrapper.Argv = nil
			cfg.LaunchWrapper.Version = 0
			cfg.LaunchWrapper.Source = ""
			cfg.Command = []string{filepath.Join(cfg.WorkDir, "mock-agent")}
			cfg.OperatorDefined = true
			content := []byte("#!/bin/sh\nprintf 'sandbox-private-fixture-value\\n' >&2\nsleep 0.1\nexit 1\n")
			require.NoError(t, os.Chmod(cfg.Sandbox.Prefix[0], 0700))
			require.NoError(t, os.WriteFile(cfg.Sandbox.Prefix[0], content, 0500))
			require.NoError(t, os.Chmod(cfg.Sandbox.Prefix[0], 0500))
			digest := sha256.Sum256(content)
			cfg.Sandbox.Digests["runtime"] = hex.EncodeToString(digest[:])
			core, logs := observer.New(zap.DebugLevel)
			executor := NewACPInferenceExecutor(zap.New(core))
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if prompt {
				response, err := executor.Execute(ctx, &PromptRequest{InferenceConfig: cfg})
				require.NoError(t, err)
				require.False(t, response.Success)
				require.NotContains(t, response.Error, "sandbox-private-fixture-value")
			} else {
				response, err := executor.Probe(ctx, &ProbeRequest{InferenceConfig: cfg})
				require.NoError(t, err)
				require.False(t, response.Success)
				require.NotContains(t, response.Error, "sandbox-private-fixture-value")
			}
			for _, entry := range logs.All() {
				require.NotContains(t, entry.Message, "sandbox-private-fixture-value")
				require.NotContains(t, fmt.Sprint(entry.ContextMap()), "sandbox-private-fixture-value")
			}
		})
	}
}
