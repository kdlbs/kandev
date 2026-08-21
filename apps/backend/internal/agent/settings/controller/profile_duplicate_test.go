package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

// sourceProfile returns a fully-populated kanban-flavour profile so every
// copy assertion has a concrete value to compare against.
func sourceProfile() *models.AgentProfile {
	failureThreshold := 4
	lastRun := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	return &models.AgentProfile{
		ID:               "source-1",
		AgentID:          "agent-1",
		Name:             "Default",
		AgentDisplayName: "Claude Code",
		Model:            "claude-sonnet",
		FallbackModel:    "claude-haiku",
		AutoFallback:     true,
		Mode:             "plan",
		ConfigOptions:    map[string]string{"effort": "high"},
		AllowIndexing:    true,
		AutoApprove:      true,
		CLIPassthrough:   true,
		CLIFlags: []models.CLIFlag{
			{Description: "Allow all tools", Flag: "--allow-all-tools", Enabled: true},
			{Description: "Custom", Flag: "--my-flag value", Enabled: false},
		},
		EnvVars: []models.ProfileEnvVar{
			{Key: "FOO", Value: "bar"},
			{Key: "TOKEN", SecretID: "sec-1"},
		},
		CommandPrefix: "greywall --",
		UserModified:  true,
		Enabled:       true,
		// Office enrichment configuration — must be copied. WorkspaceID stays
		// empty (kanban): the settings duplicate endpoint rejects office-scoped
		// sources, so the copy assertions cover the enrichment fields on a
		// kanban-scoped profile.
		WorkspaceID:           "",
		Role:                  "worker",
		Icon:                  "🤖",
		ReportsTo:             "profile-other",
		SkillIDs:              `["s1","s2"]`,
		DesiredSkills:         `["ds1"]`,
		MaxConcurrentSessions: 2,
		CooldownSec:           60,
		SkipIdleRuns:          true,
		FailureThreshold:      &failureThreshold,
		ExecutorPreference:    "exec-local",
		BudgetMonthlyCents:    12500,
		Settings:              `{"office_theme":"dark"}`,
		Permissions:           `{"spawn_subagents":true}`,
		// Deprecated legacy columns — still copied for fidelity so an old row's
		// values survive duplication.
		DangerouslySkipPermissions: true,
		CustomPrompt:               "legacy prompt",
		// Runtime state — must NOT be copied.
		Status:              "working",
		PauseReason:         "auto-paused",
		LastRunFinishedAt:   &lastRun,
		ConsecutiveFailures: 3,
	}
}

// duplicateSetup wires a controller to a fake store pre-seeded with the source profile.
func duplicateSetup(source *models.AgentProfile) (*Controller, *fakeStore) {
	ctrl := newTestController(nil)
	st := newFakeStore()
	st.agents[source.AgentID] = &models.Agent{ID: source.AgentID, Name: "test-agent"}
	st.profiles[source.AgentID] = []*models.AgentProfile{source}
	ctrl.repo = st
	return ctrl, st
}

// TestDuplicateProfile_CopiesFullConfiguration verifies every configuration field (office enrichment and deprecated legacy columns included) survives duplication while runtime state is reset.
func TestDuplicateProfile_CopiesFullConfiguration(t *testing.T) {
	source := sourceProfile()
	ctrl, st := duplicateSetup(source)

	result, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if err != nil {
		t.Fatalf("DuplicateProfile: %v", err)
	}

	if result.ID == source.ID || result.ID == "" {
		t.Fatalf("copy ID = %q, want a fresh non-empty ID", result.ID)
	}
	if result.Name != "Default Copy" {
		t.Errorf("copy name = %q, want %q", result.Name, "Default Copy")
	}
	if result.AgentID != source.AgentID {
		t.Errorf("copy agent_id = %q, want %q", result.AgentID, source.AgentID)
	}
	if result.AgentDisplayName != source.AgentDisplayName {
		t.Errorf("copy display name = %q, want %q", result.AgentDisplayName, source.AgentDisplayName)
	}
	if result.Model != source.Model || result.FallbackModel != source.FallbackModel || !result.AutoFallback {
		t.Errorf("copy model fields = (%q, %q, %v), want (%q, %q, true)",
			result.Model, result.FallbackModel, result.AutoFallback, source.Model, source.FallbackModel)
	}
	if result.Mode != source.Mode {
		t.Errorf("copy mode = %q, want %q", result.Mode, source.Mode)
	}
	if len(result.ConfigOptions) != 1 || result.ConfigOptions["effort"] != "high" {
		t.Errorf("copy config options = %+v, want effort=high", result.ConfigOptions)
	}
	if !result.AllowIndexing || !result.AutoApprove || !result.CLIPassthrough {
		t.Errorf("copy boolean config not preserved: allow_indexing=%v auto_approve=%v cli_passthrough=%v",
			result.AllowIndexing, result.AutoApprove, result.CLIPassthrough)
	}
	if !result.UserModified {
		t.Error("copy user_modified = false, want true")
	}
	if !result.Enabled {
		t.Error("copy enabled = false, want true (source enabled)")
	}
	if result.CommandPrefix != "greywall --" {
		t.Errorf("copy command prefix = %q, want %q", result.CommandPrefix, "greywall --")
	}
	if len(result.CLIFlags) != 2 ||
		result.CLIFlags[0].Flag != "--allow-all-tools" || !result.CLIFlags[0].Enabled ||
		result.CLIFlags[1].Flag != "--my-flag value" || result.CLIFlags[1].Enabled {
		t.Errorf("copy cli flags = %+v, want both source entries with their enabled states", result.CLIFlags)
	}
	if len(result.EnvVars) != 2 ||
		result.EnvVars[0].Key != "FOO" || result.EnvVars[0].Value != "bar" || result.EnvVars[0].SecretID != "" ||
		result.EnvVars[1].Key != "TOKEN" || result.EnvVars[1].SecretID != "sec-1" {
		t.Errorf("copy env vars = %+v, want both source entries incl. secret ref", result.EnvVars)
	}

	if len(st.created) != 1 {
		t.Fatalf("stored copies = %d, want 1", len(st.created))
	}
	stored := st.created[0]
	if stored.ID != result.ID {
		t.Errorf("stored copy ID = %q, response ID = %q", stored.ID, result.ID)
	}
	// Office enrichment configuration copied; runtime state reset.
	if stored.WorkspaceID != "" || stored.Role != "worker" || stored.Icon != "🤖" ||
		stored.ReportsTo != "profile-other" || stored.SkillIDs != `["s1","s2"]` ||
		stored.DesiredSkills != `["ds1"]` || stored.MaxConcurrentSessions != 2 ||
		stored.CooldownSec != 60 || !stored.SkipIdleRuns ||
		stored.FailureThreshold == nil || *stored.FailureThreshold != 4 ||
		stored.ExecutorPreference != "exec-local" || stored.BudgetMonthlyCents != 12500 ||
		stored.Settings != `{"office_theme":"dark"}` || stored.Permissions != `{"spawn_subagents":true}` {
		t.Errorf("stored office enrichment not copied: %+v", stored)
	}
	if !stored.DangerouslySkipPermissions || stored.CustomPrompt != "legacy prompt" {
		t.Errorf("deprecated legacy columns not copied: dangerously_skip_permissions=%v custom_prompt=%q",
			stored.DangerouslySkipPermissions, stored.CustomPrompt)
	}
	if stored.Status != "" || stored.PauseReason != "" || stored.LastRunFinishedAt != nil || stored.ConsecutiveFailures != 0 {
		t.Errorf("runtime state must not be copied: status=%q pause=%q last_run=%v failures=%d",
			stored.Status, stored.PauseReason, stored.LastRunFinishedAt, stored.ConsecutiveFailures)
	}
	if stored.MigratedFrom != "" {
		t.Errorf("legacy field must not be copied: migrated_from=%q", stored.MigratedFrom)
	}
}

// TestDuplicateProfile_CopiesDisabledState verifies a disabled source produces a disabled copy.
func TestDuplicateProfile_CopiesDisabledState(t *testing.T) {
	source := sourceProfile()
	source.Enabled = false
	ctrl, st := duplicateSetup(source)

	result, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if err != nil {
		t.Fatalf("DuplicateProfile: %v", err)
	}
	if result.Enabled {
		t.Error("copy enabled = true, want false (source disabled)")
	}
	if len(st.created) != 1 || st.created[0].Enabled {
		t.Errorf("stored copy enabled = %v, want false", st.created[0].Enabled)
	}
	// The disabled state must be part of the single atomic duplicate write:
	// no follow-up enabled flip is allowed (a flip would create a window where
	// the copy is selectable and would need a second write).
	if len(st.updated) != 0 {
		t.Errorf("duplicate issued %d follow-up updates, want 0 (atomic write)", len(st.updated))
	}
	// The response timestamps must be the ones actually stored (no stale
	// insert timestamp after a separate write).
	stored := st.created[0]
	if !result.CreatedAt.Equal(stored.CreatedAt) || !result.UpdatedAt.Equal(stored.UpdatedAt) {
		t.Errorf("response timestamps (%v/%v) differ from stored (%v/%v)",
			result.CreatedAt, result.UpdatedAt, stored.CreatedAt, stored.UpdatedAt)
	}
}

// TestDuplicateProfile_StoreFailureLeavesNoCopy verifies a failed transaction leaves no copy behind.
func TestDuplicateProfile_StoreFailureLeavesNoCopy(t *testing.T) {
	source := sourceProfile()
	ctrl, st := duplicateSetup(source)
	wantErr := errors.New("boom")
	st.duplicateProfErr = wantErr

	_, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if len(st.created) != 0 || len(st.mcpConfigs) != 0 {
		t.Errorf("partial copy persisted despite store error: created=%d mcp=%d",
			len(st.created), len(st.mcpConfigs))
	}
}

// TestDuplicateProfile_CopiesMcpConfig verifies the MCP config (enabled, servers, meta) is copied under the new profile ID.
func TestDuplicateProfile_CopiesMcpConfig(t *testing.T) {
	source := sourceProfile()
	ctrl, st := duplicateSetup(source)
	st.mcpConfigs[source.ID] = &models.AgentProfileMcpConfig{
		ProfileID: source.ID,
		Enabled:   true,
		Servers: map[string]interface{}{
			"github": map[string]interface{}{"url": "https://api.github.com", "enabled": true},
		},
		Meta: map[string]interface{}{"source": "user"},
	}

	result, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if err != nil {
		t.Fatalf("DuplicateProfile: %v", err)
	}

	copied, ok := st.mcpConfigs[result.ID]
	if !ok {
		t.Fatalf("no mcp config row for copy %q", result.ID)
	}
	if !copied.Enabled {
		t.Error("copied mcp config enabled = false, want true")
	}
	servers, ok := copied.Servers["github"].(map[string]interface{})
	if !ok || servers["url"] != "https://api.github.com" || servers["enabled"] != true {
		t.Errorf("copied mcp servers = %+v, want github entry preserved", copied.Servers)
	}
	if copied.Meta["source"] != "user" {
		t.Errorf("copied mcp meta = %+v, want source=user", copied.Meta)
	}
	// The source row must be untouched.
	if _, ok := st.mcpConfigs[source.ID]; !ok {
		t.Error("source mcp config row disappeared")
	}
}

// TestDuplicateProfile_NoMcpConfigLeavesCopyWithoutRow verifies a source without an MCP row leaves the copy without one.
func TestDuplicateProfile_NoMcpConfigLeavesCopyWithoutRow(t *testing.T) {
	source := sourceProfile()
	ctrl, st := duplicateSetup(source)

	if _, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID}); err != nil {
		t.Fatalf("DuplicateProfile: %v", err)
	}
	if len(st.mcpConfigs) != 0 {
		t.Errorf("copy created an mcp config row when the source had none: %+v", st.mcpConfigs)
	}
}

// TestDuplicateProfile_NotFound verifies an unknown source ID maps to ErrAgentProfileNotFound.
func TestDuplicateProfile_NotFound(t *testing.T) {
	ctrl, _ := duplicateSetup(sourceProfile())

	_, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: "missing"})
	if !errors.Is(err, ErrAgentProfileNotFound) {
		t.Fatalf("err = %v, want ErrAgentProfileNotFound", err)
	}
}

// TestDuplicateProfile_RejectsOfficeScopedSource verifies the settings
// duplicate endpoint refuses office-scoped profiles: they are owned by the
// workspace-scoped office surface and hidden from this instance-level API, so
// duplication must fail closed (as 404, not 403, to keep their existence
// hidden) without creating a copy.
func TestDuplicateProfile_RejectsOfficeScopedSource(t *testing.T) {
	source := sourceProfile()
	source.WorkspaceID = "ws-1"
	ctrl, st := duplicateSetup(source)

	_, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if !errors.Is(err, ErrAgentProfileNotFound) {
		t.Fatalf("err = %v, want ErrAgentProfileNotFound for an office-scoped source", err)
	}
	if len(st.created) != 0 {
		t.Errorf("office-scoped source created %d copies, want 0", len(st.created))
	}
}

func TestDuplicateProfile_RejectsDynamicSource(t *testing.T) {
	source := sourceProfile()
	source.AgentID = agents.DynamicAgentID
	ctrl, st := duplicateSetup(source)

	_, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if !errors.Is(err, ErrDynamicProfileDuplicationUnsupported) {
		t.Fatalf("err = %v, want dynamic duplication unsupported", err)
	}
	if len(st.created) != 0 {
		t.Errorf("dynamic source created %d copies, want 0", len(st.created))
	}
}

// TestDuplicateProfile_NameSuffixFromEmptySourceName pins the copy name when the source name is empty.
func TestDuplicateProfile_NameSuffixFromEmptySourceName(t *testing.T) {
	source := sourceProfile()
	source.Name = ""
	ctrl, st := duplicateSetup(source)

	result, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if err != nil {
		t.Fatalf("DuplicateProfile: %v", err)
	}
	if result.Name != " Copy" {
		t.Errorf("copy name = %q, want %q", result.Name, " Copy")
	}
	_ = st
}

// TestDuplicateProfile_RetriesOnConcurrentChange verifies the controller
// re-reads the source and retries when the repository aborts the first
// attempt with ErrProfileChanged (a concurrent writer), ending with exactly
// one copy — and that the retried copy reflects the NEW source state (proving
// the fresh GetAgentProfile happened).
func TestDuplicateProfile_RetriesOnConcurrentChange(t *testing.T) {
	source := sourceProfile()
	ctrl, st := duplicateSetup(source)
	st.duplicateChangedOnce = true

	result, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if err != nil {
		t.Fatalf("DuplicateProfile after retry: %v", err)
	}
	if result.ID == source.ID || result.ID == "" {
		t.Fatalf("copy ID = %q, want a fresh ID", result.ID)
	}
	if result.Name != "Changed Name Copy" {
		t.Errorf("copy name = %q, want %q (copy must be built from the re-read source)", result.Name, "Changed Name Copy")
	}
	if len(st.created) != 1 {
		t.Fatalf("stored copies = %d, want exactly 1 after retry", len(st.created))
	}
}

// TestDuplicateProfile_ExhaustsRetriesOnPersistentChange verifies the
// controller gives up (and creates nothing) when every attempt aborts with
// ErrProfileChanged.
func TestDuplicateProfile_ExhaustsRetriesOnPersistentChange(t *testing.T) {
	source := sourceProfile()
	ctrl, st := duplicateSetup(source)
	st.duplicateProfErr = store.ErrProfileChanged

	_, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if !errors.Is(err, store.ErrProfileChanged) {
		t.Fatalf("err = %v, want ErrProfileChanged", err)
	}
	if len(st.created) != 0 {
		t.Errorf("copy created despite persistent source changes: %d rows", len(st.created))
	}
}

// TestDuplicateProfile_SourceDeletedMidFlightMapsToNotFound verifies a source
// deleted during the duplicate retries surfaces as ErrAgentProfileNotFound
// (HTTP 404) instead of the raw store error (HTTP 500).
func TestDuplicateProfile_SourceDeletedMidFlightMapsToNotFound(t *testing.T) {
	source := sourceProfile()
	ctrl, st := duplicateSetup(source)
	st.duplicateProfErr = store.ErrSourceProfileNotFound

	_, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: source.ID})
	if !errors.Is(err, ErrAgentProfileNotFound) {
		t.Fatalf("err = %v, want ErrAgentProfileNotFound", err)
	}
	if len(st.created) != 0 {
		t.Errorf("copy created despite deleted source: %d rows", len(st.created))
	}
}
