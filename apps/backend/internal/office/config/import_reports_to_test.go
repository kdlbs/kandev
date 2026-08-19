package config

import (
	"context"
	"strings"
	"testing"
)

// TestApplyImport_ReportsToResolvesRegardlessOfBundleOrder imports a manager
// that appears after its report in the bundle slice and asserts resolution
// still succeeds. The resolver re-lists agents after every create/update in
// the bundle has run, rather than relying on encounter order.
func TestApplyImport_ReportsToResolvesRegardlessOfBundleOrder(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	bundle := &ConfigBundle{Agents: []AgentConfig{
		hierarchyAgent("bob", "carol"),
		hierarchyAgent("carol", ""),
	}}

	if _, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}

	carol := agentByName(t, env, testWorkspaceID, "carol")
	bob := agentByName(t, env, testWorkspaceID, "bob")
	assertEqual(t, "bob reports_to", bob.ReportsTo, carol.ID)
}

// TestApplyImport_ReportsToDanglingReferenceWarns asserts a reports_to naming
// nobody leaves the field empty, does not fail the import, and records
// exactly one warning naming both the agent and the missing manager.
func TestApplyImport_ReportsToDanglingReferenceWarns(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	bundle := &ConfigBundle{Agents: []AgentConfig{hierarchyAgent("bob", "nobody")}}

	result, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle)
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	assertEqual(t, "created count", result.CreatedCount, 1)
	assertEqual(t, "updated count", result.UpdatedCount, 0)
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings: got %d, want 1: %v", len(result.Warnings), result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "bob") || !strings.Contains(result.Warnings[0], "nobody") {
		t.Errorf("warning %q does not name both the agent and the missing manager", result.Warnings[0])
	}
	assertEqual(t, "bob reports_to", agentByName(t, env, testWorkspaceID, "bob").ReportsTo, "")
}

// TestApplyImport_ReportsToSelfReferenceWarns asserts an agent naming itself
// as its own manager is skipped with a warning rather than persisted or
// treated as an error.
func TestApplyImport_ReportsToSelfReferenceWarns(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	bundle := &ConfigBundle{Agents: []AgentConfig{hierarchyAgent("bob", "bob")}}

	result, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle)
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings: got %d, want 1: %v", len(result.Warnings), result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "bob") {
		t.Errorf("warning %q does not name the self-referencing agent", result.Warnings[0])
	}
	assertEqual(t, "bob reports_to", agentByName(t, env, testWorkspaceID, "bob").ReportsTo, "")
}

// TestApplyImport_ReportsToIsIdempotent imports the same hierarchy bundle
// twice and asserts the second pass leaves reports_to unchanged and does not
// duplicate rows.
func TestApplyImport_ReportsToIsIdempotent(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	bundle := &ConfigBundle{Agents: []AgentConfig{
		hierarchyAgent("bob", "carol"),
		hierarchyAgent("carol", ""),
	}}

	if _, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle); err != nil {
		t.Fatalf("first import: %v", err)
	}
	firstBobID := agentByName(t, env, testWorkspaceID, "bob").ID
	firstCarolID := agentByName(t, env, testWorkspaceID, "carol").ID

	if _, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle); err != nil {
		t.Fatalf("second import: %v", err)
	}

	agents, err := env.repo.ListAgentInstances(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	assertEqual(t, "agent row count", len(agents), 2)

	bob := agentByName(t, env, testWorkspaceID, "bob")
	carol := agentByName(t, env, testWorkspaceID, "carol")
	assertEqual(t, "bob id unchanged", bob.ID, firstBobID)
	assertEqual(t, "carol id unchanged", carol.ID, firstCarolID)
	assertEqual(t, "bob reports_to stable", bob.ReportsTo, carol.ID)
}

// TestApplyImport_ReportsToClearsOnOmission asserts that re-importing an
// agent with a bundle omitting reports_to clears a previously-set value,
// matching every other field's overwrite-on-update semantics.
func TestApplyImport_ReportsToClearsOnOmission(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seeded := &ConfigBundle{Agents: []AgentConfig{
		hierarchyAgent("bob", "carol"),
		hierarchyAgent("carol", ""),
	}}
	if _, err := env.svc.ApplyImport(ctx, testWorkspaceID, seeded); err != nil {
		t.Fatalf("seed import: %v", err)
	}
	if agentByName(t, env, testWorkspaceID, "bob").ReportsTo == "" {
		t.Fatal("setup: bob should already report to carol")
	}

	cleared := &ConfigBundle{Agents: []AgentConfig{
		hierarchyAgent("bob", ""),
		hierarchyAgent("carol", ""),
	}}
	if _, err := env.svc.ApplyImport(ctx, testWorkspaceID, cleared); err != nil {
		t.Fatalf("clearing import: %v", err)
	}
	assertEqual(t, "bob reports_to cleared", agentByName(t, env, testWorkspaceID, "bob").ReportsTo, "")
}

// TestApplyImport_ReportsToRoundTripsAcrossWorkspaces exports a workspace
// with a hierarchy and imports the resulting bundle into a separate
// workspace (its own database, mirroring a real export/import across
// installs), asserting the hierarchy survives the round trip.
func TestApplyImport_ReportsToRoundTripsAcrossWorkspaces(t *testing.T) {
	source := newTestEnv(t)
	ctx := context.Background()
	seeded := &ConfigBundle{Agents: []AgentConfig{
		hierarchyAgent("bob", "carol"),
		hierarchyAgent("carol", ""),
	}}
	if _, err := source.svc.ApplyImport(ctx, testWorkspaceID, seeded); err != nil {
		t.Fatalf("seed source workspace: %v", err)
	}

	bundle, err := source.svc.ExportBundle(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}

	target := newTestEnv(t)
	if _, err := target.svc.ApplyImport(ctx, testWorkspaceID, bundle); err != nil {
		t.Fatalf("ApplyImport into target workspace: %v", err)
	}

	bob := agentByName(t, target, testWorkspaceID, "bob")
	carol := agentByName(t, target, testWorkspaceID, "carol")
	assertEqual(t, "bob reports_to carol in target workspace", bob.ReportsTo, carol.ID)
}
