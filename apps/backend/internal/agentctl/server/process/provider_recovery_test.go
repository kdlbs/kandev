package process

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestProviderReplacementRecoveryDoesNotForgetClosedAdmission(t *testing.T) {
	for _, phase := range []journal.ProviderReplacementPhase{journal.ProviderReplacementPrepared, journal.ProviderReplacementRetiring} {
		t.Run(string(phase), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "journal.bbolt")
			storage, err := journal.Open(journal.Config{Path: path})
			require.NoError(t, err)
			receipt, err := storage.CreateProviderReplacement(context.Background(), journal.ProviderReplacementReceipt{OperationID: "retained", SessionID: "session", ExecutionID: "execution", HelperID: "previous-helper", SourceProcessGeneration: 1, SourceHarnessGeneration: 1, SourceStreamID: "source-stream", SourceTaskID: "task", RequestHash: strings.Repeat("a", 64), Phase: journal.ProviderReplacementPrepared, ExpiresAt: time.Now().Add(-time.Minute)})
			require.NoError(t, err)
			if phase == journal.ProviderReplacementRetiring {
				receipt.Phase = phase
				_, err = storage.UpdateProviderReplacement(context.Background(), receipt.Version, receipt)
				require.NoError(t, err)
			}
			require.NoError(t, storage.Close())
			cfg := persistentRestartConfig(t.TempDir())
			cfg.InstanceID = "execution"
			cfg.SessionID = "session"
			cfg.TaskID = "task"
			cfg.DeliveryHarnessGeneration = 1
			cfg.DeliveryStreamID = "source-stream"
			cfg.DurableJournalPath = path
			mgr := newRestartManager(t, cfg)
			t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })
			require.Equal(t, phase == journal.ProviderReplacementRetiring, mgr.ProviderMCPProtocolRequired())
			_, err = mgr.BeginProviderOperation("")
			require.Error(t, err, "helper recovery must not reopen an uncertain source or forget a retired scope")
			_, release, err := mgr.BeginOwnedOperation(context.Background())
			require.NoError(t, err)
			release()
		})
	}
}

func TestProviderReplacementRecoveryRequiresFreshAuthenticatedOwner(t *testing.T) {
	for _, scenario := range []string{"fresh", "same-generation", "same-stream", "foreign-session", "foreign-task", "missing-profile"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "journal.bbolt")
			storage, err := journal.Open(journal.Config{Path: path})
			require.NoError(t, err)
			receipt, err := storage.CreateProviderReplacement(context.Background(), journal.ProviderReplacementReceipt{OperationID: "previous", SessionID: "session", ExecutionID: "execution", HelperID: "previous-helper", SourceProcessGeneration: 1, SourceHarnessGeneration: 1, SourceStreamID: "source-stream", SourceTaskID: "task", RequestHash: strings.Repeat("a", 64), Phase: journal.ProviderReplacementPrepared, ExpiresAt: time.Now().Add(time.Minute)})
			require.NoError(t, err)
			receipt.Phase = journal.ProviderReplacementRetiring
			_, err = storage.UpdateProviderReplacement(context.Background(), receipt.Version, receipt)
			require.NoError(t, err)
			require.NoError(t, storage.Close())
			cfg := persistentRestartConfig(root)
			cfg.TaskID = "task"
			cfg.SessionID = "session"
			cfg.InstanceID = "new-execution"
			cfg.DurableJournalPath = path
			cfg.DeliveryHarnessGeneration = 2
			cfg.DeliveryStreamID = "new-owner"
			profile := p.New(p.SurfaceKanbanTask, nil, nil)
			profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
			cfg.McpProfile = &profile
			switch scenario {
			case "same-generation":
				cfg.DeliveryHarnessGeneration = 1
			case "same-stream":
				cfg.DeliveryStreamID = "source-stream"
			case "foreign-session":
				cfg.SessionID = "other-session"
			case "foreign-task":
				cfg.TaskID = "other-task"
			case "missing-profile":
				cfg.McpProfile = nil
			}
			mgr := newRestartManager(t, cfg)
			t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })
			release, err := mgr.BeginProviderOperation("")
			if scenario == "fresh" {
				require.NoError(t, err)
				release()
			} else {
				require.Error(t, err)
			}
			require.True(t, mgr.ProviderMCPProtocolRequired(), "even a fresh owner must keep older backend readers out")
		})
	}
}
