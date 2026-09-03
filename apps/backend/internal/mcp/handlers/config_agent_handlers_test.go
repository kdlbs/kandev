package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/registry"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	settingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// newAgentSettingsHandlers wires the MCP handlers to a real agent-settings
// controller over an in-memory sqlite store, so the delete tool below is
// classified from the error shapes production actually produces.
func newAgentSettingsHandlers(t *testing.T) (*Handlers, settingsstore.Repository, *agentsettingscontroller.Controller) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)

	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo, cleanup, err := settingsstore.Provide(db, db, log)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })

	agentRegistry := registry.NewRegistry(log)
	agentRegistry.LoadDefaults()
	ctrl := agentsettingscontroller.NewController(repo, nil, agentRegistry, nil, log)
	return &Handlers{logger: log, agentSettingsCtrl: ctrl}, repo, ctrl
}

func decodeAgentProfileResponse(t *testing.T, resp *ws.Message) settingsdto.AgentProfileDTO {
	t.Helper()
	require.Equalf(t, ws.MessageTypeResponse, resp.Type, "unexpected response: %s", string(resp.Payload))
	var profile settingsdto.AgentProfileDTO
	require.NoError(t, json.Unmarshal(resp.Payload, &profile))
	return profile
}

func TestHandleCreateAgentProfile_PersistsAutoApprove(t *testing.T) {
	h, repo, _ := newAgentSettingsHandlers(t)
	ctx := context.Background()
	agent := &settingsmodels.Agent{Name: "codex-acp"}
	require.NoError(t, repo.CreateAgent(ctx, agent))

	msg := makeWSMessage(t, ws.ActionMCPCreateAgentProfile, map[string]interface{}{
		"agent_id":     agent.ID,
		"name":         "Auto approve",
		"model":        "gpt-5",
		"auto_approve": true,
	})
	resp, err := h.handleCreateAgentProfile(ctx, msg)
	require.NoError(t, err)
	created := decodeAgentProfileResponse(t, resp)
	require.True(t, created.AutoApprove)

	stored, err := repo.GetAgentProfile(ctx, created.ID)
	require.NoError(t, err)
	require.True(t, stored.AutoApprove)
}

func TestHandleUpdateAgentProfile_AppliesExplicitAutoApprove(t *testing.T) {
	tests := []struct {
		name    string
		initial bool
		update  bool
	}{
		{name: "enable", initial: false, update: true},
		{name: "disable", initial: true, update: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, repo, _ := newAgentSettingsHandlers(t)
			ctx := context.Background()
			profile := &settingsmodels.AgentProfile{
				AgentID: "agent-1", Name: "Profile", Model: "gpt-5", AutoApprove: tt.initial,
			}
			require.NoError(t, repo.CreateAgentProfile(ctx, profile))

			msg := makeWSMessage(t, ws.ActionMCPUpdateAgentProfile, map[string]interface{}{
				"profile_id": profile.ID, "auto_approve": tt.update,
			})
			resp, err := h.handleUpdateAgentProfile(ctx, msg)
			require.NoError(t, err)
			updated := decodeAgentProfileResponse(t, resp)
			require.Equal(t, tt.update, updated.AutoApprove)

			stored, err := repo.GetAgentProfile(ctx, profile.ID)
			require.NoError(t, err)
			require.Equal(t, tt.update, stored.AutoApprove)
		})
	}
}

func TestHandleUpdateAgentProfile_OmittedAutoApprovePreservesValue(t *testing.T) {
	h, repo, _ := newAgentSettingsHandlers(t)
	ctx := context.Background()
	profile := &settingsmodels.AgentProfile{
		AgentID: "agent-1", Name: "Profile", Model: "gpt-5", AutoApprove: true,
	}
	require.NoError(t, repo.CreateAgentProfile(ctx, profile))

	msg := makeWSMessage(t, ws.ActionMCPUpdateAgentProfile, map[string]interface{}{
		"profile_id": profile.ID, "name": "Renamed",
	})
	resp, err := h.handleUpdateAgentProfile(ctx, msg)
	require.NoError(t, err)
	updated := decodeAgentProfileResponse(t, resp)
	require.True(t, updated.AutoApprove)

	stored, err := repo.GetAgentProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.True(t, stored.AutoApprove)
}

// stubUtilityDeps reports a fixed set of utility agents bound to a profile so
// the in-use branch of the delete tool stays reachable.
type stubUtilityDeps struct {
	refs []agentsettingscontroller.UtilityAgentReference
}

func (s *stubUtilityDeps) ListUtilityAgentsByAgentProfile(
	context.Context, string,
) ([]agentsettingscontroller.UtilityAgentReference, error) {
	return s.refs, nil
}

func (s *stubUtilityDeps) ClearUtilityAgentProfileBindings(context.Context, string) error {
	return nil
}

// TestHandleDeleteAgentProfile_UnknownProfileIsNotFound pins the MCP surface of
// the missing-profile classification. Without a not-found branch every failure
// but the in-use one collapsed into INTERNAL_ERROR, so an agent deleting a
// profile that was already gone was told the server had broken.
func TestHandleDeleteAgentProfile_UnknownProfileIsNotFound(t *testing.T) {
	h, _, _ := newAgentSettingsHandlers(t)

	msg := makeWSMessage(t, ws.ActionMCPDeleteAgentProfile, map[string]interface{}{
		"profile_id": "never-existed",
	})
	resp, err := h.handleDeleteAgentProfile(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)
}

// TestHandleDeleteAgentProfile_SoftDeletedProfileIsNotFound is the same
// classification reached the way a user reaches it: the row exists but is
// already soft-deleted, which the store hides behind sql.ErrNoRows.
func TestHandleDeleteAgentProfile_SoftDeletedProfileIsNotFound(t *testing.T) {
	h, repo, _ := newAgentSettingsHandlers(t)
	ctx := context.Background()

	profile := &settingsmodels.AgentProfile{AgentID: "agent-1", Name: "Doomed", Model: "model-a"}
	require.NoError(t, repo.CreateAgentProfile(ctx, profile))
	require.NoError(t, repo.DeleteAgentProfile(ctx, profile.ID))

	msg := makeWSMessage(t, ws.ActionMCPDeleteAgentProfile, map[string]interface{}{
		"profile_id": profile.ID,
	})
	resp, err := h.handleDeleteAgentProfile(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)
}

// TestHandleDeleteAgentProfile_InUseStaysAValidationError guards the branch the
// not-found mapping was added next to: a profile still bound to a utility agent
// must keep reporting VALIDATION_ERROR rather than being swallowed by it.
func TestHandleDeleteAgentProfile_InUseStaysAValidationError(t *testing.T) {
	h, repo, ctrl := newAgentSettingsHandlers(t)
	ctx := context.Background()

	profile := &settingsmodels.AgentProfile{AgentID: "agent-1", Name: "Bound", Model: "model-a"}
	require.NoError(t, repo.CreateAgentProfile(ctx, profile))
	ctrl.SetUtilityDependencyChecker(&stubUtilityDeps{
		refs: []agentsettingscontroller.UtilityAgentReference{{ID: "utility-1", Name: "Title"}},
	})

	msg := makeWSMessage(t, ws.ActionMCPDeleteAgentProfile, map[string]interface{}{
		"profile_id": profile.ID,
	})
	resp, err := h.handleDeleteAgentProfile(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}
