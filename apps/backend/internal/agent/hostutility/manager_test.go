package hostutility

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestStopCancelsInFlightBootstrap(t *testing.T) {
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)

	started := make(chan struct{})
	canceled := make(chan struct{})
	require.NoError(t, reg.Register(&blockingInferenceAgent{
		started:  started,
		canceled: canceled,
	}))

	mgr := NewManager(reg, "127.0.0.1", 1, nil, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- mgr.Start(ctx)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("host utility bootstrap did not start installation check")
	}

	mgr.Stop(context.Background())

	select {
	case <-canceled:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("host utility Stop did not cancel in-flight bootstrap")
	}

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("host utility Start did not return after Stop")
	}
}

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return log
}

type blockingInferenceAgent struct {
	once     sync.Once
	started  chan struct{}
	canceled chan struct{}
}

func (a *blockingInferenceAgent) ID() string          { return "blocking-acp" }
func (a *blockingInferenceAgent) Name() string        { return "Blocking ACP" }
func (a *blockingInferenceAgent) DisplayName() string { return "Blocking ACP" }
func (a *blockingInferenceAgent) Description() string { return "blocking test agent" }
func (a *blockingInferenceAgent) Enabled() bool       { return true }
func (a *blockingInferenceAgent) DisplayOrder() int   { return 1 }
func (a *blockingInferenceAgent) Logo(agents.LogoVariant) []byte {
	return nil
}
func (a *blockingInferenceAgent) IsInstalled(ctx context.Context) (*agents.DiscoveryResult, error) {
	a.once.Do(func() {
		close(a.started)
	})
	<-ctx.Done()
	close(a.canceled)
	return nil, ctx.Err()
}
func (a *blockingInferenceAgent) BuildCommand(agents.CommandOptions) agents.Command {
	return agents.NewCommand("blocking-acp")
}
func (a *blockingInferenceAgent) PermissionSettings() map[string]agents.PermissionSetting {
	return nil
}
func (a *blockingInferenceAgent) Runtime() *agents.RuntimeConfig {
	return &agents.RuntimeConfig{Protocol: agent.ProtocolACP}
}
func (a *blockingInferenceAgent) BillingType() usage.BillingType {
	return usage.BillingTypeSubscription
}
func (a *blockingInferenceAgent) RemoteAuth() *agents.RemoteAuth { return nil }
func (a *blockingInferenceAgent) InstallScript() string          { return "" }
func (a *blockingInferenceAgent) InferenceConfig() *agents.InferenceConfig {
	return &agents.InferenceConfig{
		Supported: true,
		Command:   agents.NewCommand("blocking-acp"),
	}
}
