package lifecycle

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/nomad"
	commonconfig "github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

// captureNomadTransport records the private environment file piped to the worker.
type captureNomadTransport struct{ envFile string }

func (c *captureNomadTransport) Run(_ context.Context, command string) (string, string, error) {
	if strings.HasPrefix(command, "shasum") {
		return strings.Repeat("0", 64) + "  artifact", "", nil
	}
	return "", "", nil
}

func (c *captureNomadTransport) RunStdin(_ context.Context, _ string, stdin io.Reader) (string, string, error) {
	raw, err := io.ReadAll(stdin)
	c.envFile = string(raw)
	return "", "", err
}

func (*captureNomadTransport) Forward(int, *logger.Logger) (nomadPortForward, error) {
	return nil, nil
}
func (*captureNomadTransport) Close() error { return nil }

func (*captureNomadTransport) Upload(context.Context, string, []byte, os.FileMode) error { return nil }

func TestStageNomadLaunchFilesDisablesManagedIdleReaping(t *testing.T) {
	fixture := newNomadCreateFixture(t, false)
	fixture.request.AgentctlStartupConfig = commonconfig.AgentctlStartupConfig{Configured: true, IdleTimeout: time.Hour, IdleReaperInterval: time.Minute, NotificationQueueCapacity: 4096}
	req := fixture.request
	id, err := resolveNomadLaunchIdentity(req)
	if err != nil {
		t.Fatal(err)
	}
	profile := id.profile
	state := newNomadIntentState(req, id)
	transport := &captureNomadTransport{}
	// The artifact digest check fails against the fake; only the staged env matters.
	_, _ = stageNomadLaunchFilesOver(context.Background(), transport, req, profile, state, []byte("artifact"), "nonce")

	var raw string
	for _, line := range strings.Split(transport.envFile, "\n") {
		if strings.HasPrefix(line, commonconfig.InternalAgentctlStartupConfigEnv+"=") {
			raw = strings.Trim(strings.TrimPrefix(line, commonconfig.InternalAgentctlStartupConfigEnv+"="), "'")
		}
	}
	if raw == "" {
		t.Fatalf("startup config not staged in env file: %q", transport.envFile)
	}
	staged, err := commonconfig.DecodeAgentctlStartupConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if staged.IdleTimeout != 0 {
		t.Fatalf("staged idle timeout = %v, want 0", staged.IdleTimeout)
	}
	if req.AgentctlStartupConfig.IdleTimeout != time.Hour {
		t.Fatal("request startup config was mutated")
	}
}

func TestNomadProfileCAFileSurvivesCanonicalMetadata(t *testing.T) {
	for _, mode := range []string{"pinned", "pool"} {
		parsed := nomad.ProfileConfig{CAFile: "/etc/ssl/nomad-ca.pem", Mode: nomad.ModeLegacyPinned}
		if mode == "pool" {
			parsed.Mode = nomad.ModePool
		}
		canonical := nomadCanonicalProfileMetadata(parsed)
		if canonical[nomadMetadataKeyCAFile] != parsed.CAFile {
			t.Fatalf("%s: canonical metadata dropped ca_file: %v", mode, canonical)
		}
		if !isTrustedExecutorConfigKey(nomadMetadataKeyCAFile) {
			t.Fatal("ca_file must be a trusted profile key")
		}
		metadata := map[string]interface{}{}
		for k, v := range canonical {
			metadata[k] = v
		}
		if got := getMetadataString(metadata, nomadMetadataKeyCAFile); got != parsed.CAFile {
			t.Fatalf("%s: metadata lookup = %q", mode, got)
		}
	}
}
