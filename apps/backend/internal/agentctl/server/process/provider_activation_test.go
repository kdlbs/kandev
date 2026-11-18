package process

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestProviderReplacementActivationPreservesOwnerAndAdvancesOnlyProvider(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cfg := persistentRestartConfig(root)
	initial := p.New(p.SurfaceKanbanTask, nil, nil)
	initial.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: initial.Surface, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
	cfg.InstanceID = "execution"
	cfg.TaskID = "task"
	cfg.SessionID = "session"
	cfg.McpProfile = &initial
	cfg.Port = 12345
	cfg.DurableJournalPath = filepath.Join(root, "journal.bbolt")
	cfg.DeliveryStreamID = "source-stream"
	cfg.DeliveryIncarnationID = "incarnation"
	cfg.DeliveryHarnessGeneration = 1
	mgr := newRestartManager(t, cfg)
	startRestartManager(t, mgr)
	control, ok := any(mgr).(interface {
		ActivateProviderReplacement(context.Context, types.ProviderReplacementRequest) (uint64, error)
	})
	require.True(t, ok, "activation must bind the provider configuration and successor journal owner")
	source, err := mgr.ProviderReplacementSource()
	require.NoError(t, err)
	storage, err := mgr.DeliveryJournal()
	require.NoError(t, err)
	event, err := storage.Append(ctx, journal.Event{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "source-stream", Type: "status", Payload: []byte(`{}`)})
	require.NoError(t, err)
	require.NoError(t, storage.Acknowledge(ctx, "source-stream", event.Sequence))
	require.NoError(t, mgr.ReserveProviderReplacement("operation", time.Now().Add(time.Minute)))
	require.NoError(t, mgr.RetireProviderReplacement(ctx, source))
	target := initial
	target.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: initial.Surface, Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "task"}}
	digest, err := p.BuiltinBindingDigest(target)
	require.NoError(t, err)
	scope := p.ProviderMCPScope{BindingID: "target-binding", ExecutionID: "execution", HelperID: source.HelperID, ProcessGeneration: source.ProcessGeneration + 1, HarnessGeneration: source.HarnessGeneration, ProfileDigest: digest}
	req := types.ProviderReplacementRequest{OperationID: "operation", Source: source, Scope: scope, Profile: target, TargetStreamID: source.StreamID, ScopeCredential: strings.Repeat("s", 64)}
	readerStarted, readerStop, readerDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(readerDone)
		close(readerStarted)
		for {
			select {
			case <-readerStop:
				return
			default:
				_ = mgr.DeliveryStreamID()
				_ = mgr.DeliveryHarnessGeneration()
				_ = mgr.DeliveryIncarnationID()
				runtime.Gosched()
			}
		}
	}()
	<-readerStarted
	generation, err := control.ActivateProviderReplacement(ctx, req)
	close(readerStop)
	<-readerDone
	require.NoError(t, err)
	require.Equal(t, scope.ProcessGeneration, generation)
	require.Equal(t, source.StreamID, mgr.DeliveryStreamID())
	require.Equal(t, source.HarnessGeneration, mgr.DeliveryHarnessGeneration())
	require.Equal(t, "incarnation", mgr.DeliveryIncarnationID())
	current, err := mgr.ProviderReplacementSource()
	require.NoError(t, err)
	require.Equal(t, "target-binding", current.BindingID)
	require.Equal(t, "execution", current.ExecutionID)
	expected, err := config.ProviderBuiltinMCPServers(target, *cfg.ProviderMCP, cfg.MCPHost, cfg.Port)
	require.NoError(t, err)
	require.Equal(t, expected, cfg.McpServers)
	_, err = control.ActivateProviderReplacement(ctx, req)
	require.Error(t, err, "replay must reconcile receipt, never create another process")
	require.Equal(t, generation, mgr.ProcessGeneration())
	mgr.ReleaseProviderReplacement("operation")
	require.NoError(t, mgr.Stop(context.Background()))
	require.Error(t, mgr.Start(context.Background()), "an ordinary start must not reuse a scoped credential for a different process generation")
}

func TestProviderReplacementFailedSpawnConsumesProcessGeneration(t *testing.T) {
	mgr := newRestartManager(t, persistentRestartConfig(t.TempDir()))
	startRestartManager(t, mgr)
	source, err := mgr.stopProviderForReplacement(context.Background(), mgr.ProcessGeneration())
	require.NoError(t, err)
	source, err = mgr.configureProviderForReplacement(source, []string{filepath.Join(t.TempDir(), "missing-provider")}, nil)
	require.NoError(t, err)
	_, err = mgr.startProviderForReplacement(context.Background(), source)
	require.Error(t, err)
	require.Greater(t, mgr.ProcessGeneration(), source.processGeneration, "a failed spawn must not let a later retry reuse its scoped generation")
}
