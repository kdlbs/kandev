package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// TestDuplicateAgentProfile_RoundTrip verifies the atomic copy path: the copy
// gets a fresh ID, keeps every configuration field, preserves the source's
// disabled state (the store must NOT force a duplicate enabled like
// CreateAgentProfile does), and copies the MCP config row in the same write.
func TestDuplicateAgentProfile_RoundTrip(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgent(ctx, &models.Agent{Name: "test-agent", SupportsMCP: true}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "test-agent")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}

	source := &models.AgentProfile{
		AgentID:          agent.ID,
		Name:             "Default",
		AgentDisplayName: "Test Agent",
		Model:            "model-1",
		Mode:             "plan",
		AutoApprove:      true,
		CLIPassthrough:   true,
		CLIFlags:         []models.CLIFlag{{Description: "Tools", Flag: "--allow-all-tools", Enabled: true}},
		EnvVars:          []models.ProfileEnvVar{{Key: "FOO", Value: "bar"}},
		CommandPrefix:    "greywall --",
		UserModified:     true,
	}
	if err := repo.CreateAgentProfile(ctx, source); err != nil {
		t.Fatalf("create source: %v", err)
	}
	// CreateAgentProfile forces enabled=true; simulate a user-disabled source.
	// The returned timestamp is the DB revision the duplicate input must carry.
	disabledAt, err := repo.UpdateAgentProfileEnabled(ctx, source.ID, false)
	if err != nil {
		t.Fatalf("disable source: %v", err)
	}
	source.UpdatedAt = disabledAt
	sourceMcp := &models.AgentProfileMcpConfig{
		ProfileID: source.ID,
		Enabled:   true,
		Servers:   map[string]interface{}{"github": map[string]interface{}{"url": "https://api.github.com"}},
		Meta:      map[string]interface{}{"k": "v"},
	}
	if err := repo.UpsertAgentProfileMcpConfig(ctx, sourceMcp); err != nil {
		t.Fatalf("seed source mcp: %v", err)
	}

	clone := &models.AgentProfile{
		AgentID:          source.AgentID,
		Name:             "Default Copy",
		AgentDisplayName: source.AgentDisplayName,
		Model:            source.Model,
		Mode:             source.Mode,
		AutoApprove:      source.AutoApprove,
		CLIPassthrough:   source.CLIPassthrough,
		CLIFlags:         source.CLIFlags,
		EnvVars:          source.EnvVars,
		CommandPrefix:    source.CommandPrefix,
		Enabled:          false,
		UserModified:     true,
	}
	copiedMcp := &models.AgentProfileMcpConfig{
		Enabled: sourceMcp.Enabled,
		Servers: sourceMcp.Servers,
		Meta:    sourceMcp.Meta,
	}
	if err := repo.DuplicateAgentProfile(ctx, DuplicateAgentProfileInput{
		Source:    source,
		SourceMcp: sourceMcp,
		Profile:   clone,
		McpConfig: copiedMcp,
	}); err != nil {
		t.Fatalf("DuplicateAgentProfile: %v", err)
	}

	if clone.ID == source.ID || clone.ID == "" {
		t.Fatalf("copy ID = %q, want a fresh ID", clone.ID)
	}
	got, err := repo.GetAgentProfile(ctx, clone.ID)
	if err != nil {
		t.Fatalf("get copy: %v", err)
	}
	// The duplicate must NOT be forced enabled like CreateAgentProfile: a
	// disabled source produces a disabled copy in the same write.
	if got.Enabled {
		t.Error("duplicate enabled = true, want false (source disabled)")
	}
	if got.Name != "Default Copy" || got.Model != "model-1" || got.Mode != "plan" ||
		!got.AutoApprove || !got.CLIPassthrough || got.CommandPrefix != "greywall --" ||
		len(got.CLIFlags) != 1 || got.CLIFlags[0].Flag != "--allow-all-tools" ||
		len(got.EnvVars) != 1 || got.EnvVars[0].Key != "FOO" {
		t.Errorf("copy configuration not preserved: %+v", got)
	}
	if !got.CreatedAt.Equal(got.UpdatedAt) {
		t.Errorf("copy timestamps differ: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}

	gotMcp, err := repo.GetAgentProfileMcpConfig(ctx, clone.ID)
	if err != nil {
		t.Fatalf("get copy mcp config: %v", err)
	}
	if gotMcp == nil || !gotMcp.Enabled {
		t.Fatal("copy mcp config missing or disabled")
	}
	servers, ok := gotMcp.Servers["github"].(map[string]interface{})
	if !ok || servers["url"] != "https://api.github.com" {
		t.Errorf("copy mcp servers = %+v, want github entry preserved", gotMcp.Servers)
	}
	if gotMcp.Meta["k"] != "v" {
		t.Errorf("copy mcp meta = %+v, want k=v", gotMcp.Meta)
	}

	// The source row must be untouched.
	srcAgain, err := repo.GetAgentProfile(ctx, source.ID)
	if err != nil || srcAgain == nil || srcAgain.Enabled {
		t.Errorf("source row altered by duplicate: err=%v enabled=%v", err, srcAgain.Enabled)
	}
}

// TestDuplicateAgentProfile_NoMcpConfig verifies the copy is created without
// an MCP row when none is passed.
func TestDuplicateAgentProfile_NoMcpConfig(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgent(ctx, &models.Agent{Name: "test-agent"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "test-agent")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	source := &models.AgentProfile{AgentID: agent.ID, Name: "Default"}
	if err := repo.CreateAgentProfile(ctx, source); err != nil {
		t.Fatalf("create source: %v", err)
	}

	clone := &models.AgentProfile{AgentID: agent.ID, Name: "Default Copy"}
	if err := repo.DuplicateAgentProfile(ctx, DuplicateAgentProfileInput{
		Source:  source,
		Profile: clone,
	}); err != nil {
		t.Fatalf("DuplicateAgentProfile: %v", err)
	}
	if _, err := repo.GetAgentProfileMcpConfig(ctx, clone.ID); err == nil {
		t.Fatal("expected no MCP row for a copy without config")
	}
}

// TestDuplicateAgentProfile_DetectsConcurrentChange verifies the transaction
// aborts with ErrProfileChanged when the source row moved on since the caller
// built the copy (here: a stale revision), and creates nothing.
func TestDuplicateAgentProfile_DetectsConcurrentChange(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgent(ctx, &models.Agent{Name: "test-agent"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "test-agent")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	source := &models.AgentProfile{AgentID: agent.ID, Name: "Default"}
	if err := repo.CreateAgentProfile(ctx, source); err != nil {
		t.Fatalf("create source: %v", err)
	}
	// A concurrent writer bumps the source after the caller's snapshot.
	if _, err := repo.UpdateAgentProfileEnabled(ctx, source.ID, false); err != nil {
		t.Fatalf("bump source: %v", err)
	}

	clone := &models.AgentProfile{AgentID: agent.ID, Name: "Default Copy"}
	err = repo.DuplicateAgentProfile(ctx, DuplicateAgentProfileInput{
		Source:  source, // stale: UpdatedAt predates the bump above
		Profile: clone,
	})
	if !errors.Is(err, ErrProfileChanged) {
		t.Fatalf("err = %v, want ErrProfileChanged", err)
	}
	if _, err := repo.GetAgentProfile(ctx, clone.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("copy row created despite stale source: err=%v", err)
	}
}

// TestDuplicateAgentProfile_DetectsMcpRowCreatedMidFlight verifies the
// transaction aborts when the caller's snapshot had NO MCP row but one was
// created before the duplicate ran — otherwise the copy would silently drop
// the newly configured servers.
func TestDuplicateAgentProfile_DetectsMcpRowCreatedMidFlight(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgent(ctx, &models.Agent{Name: "test-agent"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "test-agent")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	source := &models.AgentProfile{AgentID: agent.ID, Name: "Default"}
	if err := repo.CreateAgentProfile(ctx, source); err != nil {
		t.Fatalf("create source: %v", err)
	}
	// The caller read "no MCP row", then a concurrent writer created one.
	mcp := &models.AgentProfileMcpConfig{
		ProfileID: source.ID,
		Enabled:   true,
		Servers:   map[string]interface{}{"github": "x"},
	}
	if err := repo.UpsertAgentProfileMcpConfig(ctx, mcp); err != nil {
		t.Fatalf("create source mcp: %v", err)
	}

	clone := &models.AgentProfile{AgentID: agent.ID, Name: "Default Copy"}
	err = repo.DuplicateAgentProfile(ctx, DuplicateAgentProfileInput{
		Source:  source, // SourceMcp nil: snapshot predates the MCP row
		Profile: clone,
	})
	if !errors.Is(err, ErrProfileChanged) {
		t.Fatalf("err = %v, want ErrProfileChanged (MCP row created mid-flight)", err)
	}
	if _, err := repo.GetAgentProfile(ctx, clone.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("copy row created despite new MCP row: err=%v", err)
	}
}

// TestDuplicateAgentProfile_RollsBackMcpFailure verifies the transaction
// rolls back the inserted profile row when the MCP upsert fails AFTER the
// insert. A regression that commits the profile before the MCP write would
// leave a partial, selectable copy on every failed duplicate.
func TestDuplicateAgentProfile_RollsBackMcpFailure(t *testing.T) {
	repo := newFreshRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgent(ctx, &models.Agent{Name: "test-agent"}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	agent, err := repo.GetAgentByName(ctx, "test-agent")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	source := &models.AgentProfile{AgentID: agent.ID, Name: "Default"}
	if err := repo.CreateAgentProfile(ctx, source); err != nil {
		t.Fatalf("create source: %v", err)
	}
	// The source has a valid MCP row so the snapshot verification passes and
	// the failure below happens during the insert-time serialization.
	sourceMcp := &models.AgentProfileMcpConfig{
		ProfileID: source.ID,
		Enabled:   true,
		Servers:   map[string]interface{}{"ok": "v"},
	}
	if err := repo.UpsertAgentProfileMcpConfig(ctx, sourceMcp); err != nil {
		t.Fatalf("seed source mcp: %v", err)
	}

	clone := &models.AgentProfile{AgentID: agent.ID, Name: "Default Copy"}
	// A channel is not JSON-serializable: the MCP upsert fails during
	// serialization, which happens AFTER the profile row was inserted inside
	// the transaction.
	badMcp := &models.AgentProfileMcpConfig{
		Enabled: true,
		Servers: map[string]interface{}{"bad": make(chan int)},
	}
	err = repo.DuplicateAgentProfile(ctx, DuplicateAgentProfileInput{
		Source:    source,
		SourceMcp: sourceMcp,
		Profile:   clone,
		McpConfig: badMcp,
	})
	if err == nil {
		t.Fatal("expected MCP serialization error, got nil")
	}
	if _, err := repo.GetAgentProfile(ctx, clone.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("copy row survived a failed duplicate (rollback missing): err=%v", err)
	}
}
