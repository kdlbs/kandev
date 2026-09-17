package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantCapabilitiesStoredIntegrationHealth(t *testing.T) {
	_, database := newMatcherTestRepos(t)
	jiraStore, err := jira.NewStore(database, database)
	require.NoError(t, err)
	linearStore, err := linear.NewStore(database, database)
	require.NoError(t, err)
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, jiraStore.UpsertConfigForWorkspace(ctx, "ws-1", &jira.JiraConfig{SiteURL: "https://example.invalid", Email: "SYNTHETIC_EMAIL_CANARY", AuthMethod: jira.AuthMethodSessionCookie}))
	require.NoError(t, jiraStore.UpdateAuthHealthForWorkspace(ctx, "ws-1", false, "SYNTHETIC_ERROR_CANARY", now))
	require.NoError(t, linearStore.UpsertConfigForWorkspace(ctx, "ws-1", &linear.LinearConfig{DefaultTeamKey: "SYNTHETIC_TEAM_CANARY"}))
	require.NoError(t, linearStore.UpdateAuthHealthForWorkspace(ctx, "ws-1", true, "", "SYNTHETIC_ORGANIZATION_CANARY", now))
	services := &Services{Jira: jira.NewService(jiraStore, nil, func(*jira.JiraConfig, string) jira.Client {
		t.Fatal("directory must not construct a provider client")
		return nil
	}, nil), Linear: linear.NewService(linearStore, nil, func(*linear.LinearConfig, string) linear.Client {
		t.Fatal("directory must not construct a provider client")
		return nil
	}, nil)}
	reader := &assistantCapabilityReader{integrations: assistantIntegrationSources(services)}
	q := shared.CapabilityQuery{WorkspaceID: "ws-1"}
	entries, err := reader.integrationCapabilities(ctx, q)
	require.NoError(t, err)
	require.Len(t, entries, 6)
	health := map[string]string{}
	for _, entry := range entries {
		health[entry.Name] = entry.Health
		require.False(t, entry.Attached)
	}
	require.Equal(t, "disconnected", health["jira"])
	require.Equal(t, "ready", health["linear"])
	require.Equal(t, "missing", health["sentry"])
	raw, err := json.Marshal(entries)
	require.NoError(t, err)
	for _, canary := range []string{"SYNTHETIC_EMAIL_CANARY", "SYNTHETIC_ERROR_CANARY", "SYNTHETIC_TEAM_CANARY", "SYNTHETIC_ORGANIZATION_CANARY"} {
		require.NotContains(t, string(raw), canary)
	}
	reader.integrations = []capabilityIntegration{{"example", func(context.Context, string) ([]storedIntegrationHealth, error) {
		return nil, fmt.Errorf("SYNTHETIC_CONNECTION_SECRET")
	}}}
	entries, err = reader.integrationCapabilities(ctx, q)
	require.NoError(t, err)
	raw, err = json.Marshal(entries)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "SYNTHETIC_CONNECTION_SECRET")
	require.Equal(t, "unavailable", entries[0].Health)
}

func TestAssistantCapabilitiesSchemaSizeAndSecretProjection(t *testing.T) {
	properties := map[string]any{}
	for i := 0; i < 100; i++ {
		properties[fmt.Sprintf("field-%03d", i)] = map[string]string{"type": "string", "default": "SYNTHETIC_SECRET"}
	}
	raw, err := json.Marshal(map[string]any{"type": "object", "properties": properties})
	require.NoError(t, err)
	projected, partial := capabilitySchema(raw)
	require.True(t, partial)
	require.LessOrEqual(t, len(projected), 8192)
	require.NotContains(t, string(projected), "SYNTHETIC_SECRET")
	q := shared.CapabilityQuery{Limit: 100}
	rows := make([]shared.Capability, 101)
	for i := range rows {
		rows[i] = catalogCapability("native", fmt.Sprint(i), "Example", "ws-1", "1")
		rows[i].InputSchema = projected
	}
	page, err := capabilityPage(rows, q)
	require.NoError(t, err)
	data, err := json.Marshal(page)
	require.NoError(t, err)
	require.Len(t, page.Entries, 100)
	require.Less(t, len(data), 2*1024*1024)
	t.Logf("100-entry synthetic directory: %d bytes", len(data))
}
