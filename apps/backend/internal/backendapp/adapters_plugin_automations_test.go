package backendapp

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/common/logger"
	_ "github.com/mattn/go-sqlite3"
)

func newPluginAutomationSource(t *testing.T) (*automation.Service, pluginsAutomationSourceAdapter) {
	t.Helper()
	database, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	store, err := automation.NewStore(database, database)
	require.NoError(t, err)
	svc := automation.NewService(store, nil, logger.Default())
	return svc, pluginsAutomationSourceAdapter{svc: svc}
}

func TestPluginsAutomationSourceAdapter_RedactsSecretsAndHidesForeignAutomation(t *testing.T) {
	svc, adapter := newPluginAutomationSource(t)
	ctx := context.Background()
	item := &automation.Automation{
		WorkspaceID: "ws-owned", Name: "Coordinator cycle", Description: "Owned binding",
		AgentProfileID: "agent-1", ExecutorProfileID: "executor-1", Prompt: "WAKE:CYCLE",
		WebhookSecret: "must-not-leak", RepositoryIDs: []string{"repository-private"}, Enabled: true,
		MaxConcurrentRuns: 3,
	}
	require.NoError(t, svc.Store().CreateAutomation(ctx, item))

	items, err := adapter.ListPluginAutomations(ctx, "ws-owned")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "WAKE:CYCLE", items[0].Prompt)
	require.Equal(t, int32(3), items[0].MaxConcurrentRuns)

	got, err := adapter.GetPluginAutomation(ctx, "ws-owned", item.ID)
	require.NoError(t, err)
	require.Equal(t, item.ID, got.ID)
	require.Equal(t, "ws-owned", got.WorkspaceID)

	foreign, err := adapter.GetPluginAutomation(ctx, "ws-other", item.ID)
	require.Nil(t, foreign)
	require.Equal(t, codes.NotFound, status.Code(err))
}
