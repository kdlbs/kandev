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

// TestApplyIncoming_PreservesReportsToFromFilesystem covers the
// filesystem-config-sync path (ApplyIncoming -> ScanFilesystem ->
// ApplyImport), not just the bundle-upload import endpoint. reports_to
// written to an on-disk agent YAML file must survive the scan and resolve to
// the manager's ID, the same as it does for a directly-uploaded bundle.
func TestApplyIncoming_PreservesReportsToFromFilesystem(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	env.writeFSAgent(t, "grace", "name: grace\nrole: manager\n")
	env.writeFSAgent(t, "bob", "name: bob\nrole: worker\nreports_to: grace\n")

	if _, err := env.svc.ApplyIncoming(ctx, testWorkspaceID); err != nil {
		t.Fatalf("ApplyIncoming: %v", err)
	}

	grace := agentByName(t, env, testWorkspaceID, "grace")
	bob := agentByName(t, env, testWorkspaceID, "bob")
	assertEqual(t, "bob reports_to grace after fs sync", bob.ReportsTo, grace.ID)
}

// TestApplyIncoming_DoesNotKeepReportsToForPrunedManager verifies that the
// incoming sync does not resolve hierarchy names against DB-only agents.
// A manager that is absent from the filesystem must not leave a dangling ID
// on an agent that remains in the imported bundle.
func TestApplyIncoming_DoesNotKeepReportsToForPrunedManager(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	manager := seedAgent(t, env, testWorkspaceID, "grace")
	seedAgent(t, env, testWorkspaceID, "bob")
	env.writeFSAgent(t, "bob", "name: bob\nrole: worker\nreports_to: grace\n")

	result, err := env.svc.ApplyIncoming(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("ApplyIncoming: %v", err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings: got %d, want 1: %v", len(result.Warnings), result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "bob") || !strings.Contains(result.Warnings[0], "grace") {
		t.Errorf("warning %q does not name both the agent and the pruned manager", result.Warnings[0])
	}
	assertEqual(t, "bob reports_to after manager prune", agentByName(t, env, testWorkspaceID, "bob").ReportsTo, "")
	if _, err := env.repo.GetAgentInstance(ctx, manager.ID); err == nil {
		t.Fatalf("pruned manager %q is still visible in the repository", manager.ID)
	}
}

// TestApplyImport_ReportsToTwoAgentCycleWarns asserts a 2-cycle formed
// entirely within one bundle (bob reports_to carol, carol reports_to bob) is
// detected, both edges are cleared, and each is reported with its own
// warning rather than silently persisted.
func TestApplyImport_ReportsToTwoAgentCycleWarns(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	bundle := &ConfigBundle{Agents: []AgentConfig{
		hierarchyAgent("bob", "carol"),
		hierarchyAgent("carol", "bob"),
	}}

	result, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle)
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("warnings: got %d, want 2: %v", len(result.Warnings), result.Warnings)
	}
	for _, w := range result.Warnings {
		if !strings.Contains(w, "bob") || !strings.Contains(w, "carol") {
			t.Errorf("warning %q does not name both agents in the cycle", w)
		}
	}
	assertEqual(t, "bob reports_to cleared", agentByName(t, env, testWorkspaceID, "bob").ReportsTo, "")
	assertEqual(t, "carol reports_to cleared", agentByName(t, env, testWorkspaceID, "carol").ReportsTo, "")
}

// TestApplyImport_ReportsToThreeAgentChainCycleWarns asserts a longer chain
// cycle (a -> b -> c -> a), not just a direct 2-cycle, is detected. Every
// edge in the cycle is cleared with its own warning.
func TestApplyImport_ReportsToThreeAgentChainCycleWarns(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	bundle := &ConfigBundle{Agents: []AgentConfig{
		hierarchyAgent("a", "b"),
		hierarchyAgent("b", "c"),
		hierarchyAgent("c", "a"),
	}}

	result, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle)
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	if len(result.Warnings) != 3 {
		t.Fatalf("warnings: got %d, want 3: %v", len(result.Warnings), result.Warnings)
	}
	assertEqual(t, "a reports_to cleared", agentByName(t, env, testWorkspaceID, "a").ReportsTo, "")
	assertEqual(t, "b reports_to cleared", agentByName(t, env, testWorkspaceID, "b").ReportsTo, "")
	assertEqual(t, "c reports_to cleared", agentByName(t, env, testWorkspaceID, "c").ReportsTo, "")
}

// TestApplyImport_ReportsToCycleThroughPersistedNonBundleAgentWarns asserts
// a cycle is still detected when only one side of it is in the incoming
// bundle: frank already exists in the workspace, eve already persistently
// reports_to frank (outside this bundle), and this bundle proposes frank
// reports_to eve. Resolving only against the bundle's own contents would
// miss this; the walk must also consult persisted state.
func TestApplyImport_ReportsToCycleThroughPersistedNonBundleAgentWarns(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	seed := &ConfigBundle{Agents: []AgentConfig{hierarchyAgent("frank", "")}}
	if _, err := env.svc.ApplyImport(ctx, testWorkspaceID, seed); err != nil {
		t.Fatalf("seed frank: %v", err)
	}
	frank := agentByName(t, env, testWorkspaceID, "frank")

	eve := seedAgent(t, env, testWorkspaceID, "eve")
	eve.ReportsTo = frank.ID
	if err := env.repo.UpdateAgentInstance(ctx, eve); err != nil {
		t.Fatalf("seed eve reports_to frank: %v", err)
	}

	bundle := &ConfigBundle{Agents: []AgentConfig{hierarchyAgent("frank", "eve")}}
	result, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle)
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings: got %d, want 1: %v", len(result.Warnings), result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "frank") || !strings.Contains(result.Warnings[0], "eve") {
		t.Errorf("warning %q does not name both agents in the cycle", result.Warnings[0])
	}
	assertEqual(t, "frank reports_to cleared", agentByName(t, env, testWorkspaceID, "frank").ReportsTo, "")
	assertEqual(t, "eve reports_to unchanged", agentByName(t, env, testWorkspaceID, "eve").ReportsTo, frank.ID)
}

// TestApplyImport_ReportsToDeepChainNoCycleResolves guards against
// over-rejection: a valid deep chain with no cycle (a -> b -> c) must still
// resolve every edge with zero warnings.
func TestApplyImport_ReportsToDeepChainNoCycleResolves(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	bundle := &ConfigBundle{Agents: []AgentConfig{
		hierarchyAgent("a", "b"),
		hierarchyAgent("b", "c"),
		hierarchyAgent("c", ""),
	}}

	result, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle)
	if err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings: got %d, want 0: %v", len(result.Warnings), result.Warnings)
	}
	a := agentByName(t, env, testWorkspaceID, "a")
	b := agentByName(t, env, testWorkspaceID, "b")
	c := agentByName(t, env, testWorkspaceID, "c")
	assertEqual(t, "a reports_to b", a.ReportsTo, b.ID)
	assertEqual(t, "b reports_to c", b.ReportsTo, c.ID)
	assertEqual(t, "c reports_to", c.ReportsTo, "")
}

// TestApplyImport_PreExistingPersistedCycleDoesNotPreventResolution asserts
// that a cyclic reports_to pair already sitting in the database (from before this
// guard existed) cannot hang import resolution for an unrelated agent. The
// walk must be bounded by a visited set, not rely on the graph being
// acyclic.
func TestApplyImport_PreExistingPersistedCycleDoesNotPreventResolution(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	x := seedAgent(t, env, testWorkspaceID, "x")
	y := seedAgent(t, env, testWorkspaceID, "y")
	x.ReportsTo = y.ID
	if err := env.repo.UpdateAgentInstance(ctx, x); err != nil {
		t.Fatalf("seed x reports_to y: %v", err)
	}
	y.ReportsTo = x.ID
	if err := env.repo.UpdateAgentInstance(ctx, y); err != nil {
		t.Fatalf("seed y reports_to x: %v", err)
	}

	bundle := &ConfigBundle{Agents: []AgentConfig{hierarchyAgent("zack", "x")}}
	if _, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}

	zack := agentByName(t, env, testWorkspaceID, "zack")
	assertEqual(t, "zack reports_to x", zack.ReportsTo, x.ID)
}

// TestApplyIncoming_CycleThroughPrunedExternalManagerDoesNotFalseFlag
// asserts the filesystem-sync path (allowExternalManagers=false) does not
// walk into a pruned, DB-only agent's persisted reports_to when checking a
// bundle member for a cycle. grace is DB-only (absent from the filesystem)
// and persistently reports_to bob; the filesystem snapshot proposes bob
// reports_to carol and carol reports_to grace. bob must resolve cleanly, while
// carol must receive the expected dangling-reference warning.
func TestApplyIncoming_CycleThroughPrunedExternalManagerDoesNotFalseFlag(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	grace := seedAgent(t, env, testWorkspaceID, "grace")
	bob := seedAgent(t, env, testWorkspaceID, "bob")
	grace.ReportsTo = bob.ID
	if err := env.repo.UpdateAgentInstance(ctx, grace); err != nil {
		t.Fatalf("seed grace reports_to bob: %v", err)
	}

	env.writeFSAgent(t, "bob", "name: bob\nrole: worker\nreports_to: carol\n")
	env.writeFSAgent(t, "carol", "name: carol\nrole: manager\nreports_to: grace\n")

	result, err := env.svc.ApplyIncoming(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("ApplyIncoming: %v", err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings: got %d, want 1: %v", len(result.Warnings), result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "carol") || !strings.Contains(result.Warnings[0], "grace") {
		t.Errorf("warning %q does not name the pruned manager and report", result.Warnings[0])
	}
	carol := agentByName(t, env, testWorkspaceID, "carol")
	assertEqual(t, "bob reports_to carol after fs sync", agentByName(t, env, testWorkspaceID, "bob").ReportsTo, carol.ID)
	assertEqual(t, "carol reports_to cleared", carol.ReportsTo, "")
	if _, err := env.repo.GetAgentInstance(ctx, grace.ID); err == nil {
		t.Fatalf("pruned manager %q is still visible in the repository", grace.ID)
	}
}

// TestApplyImport_DuplicateAgentNamesFailBeforeWrites rejects an ambiguous
// bundle before it can create duplicate rows or build an incorrect graph.
func TestApplyImport_DuplicateAgentNamesFailBeforeWrites(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	bundle := &ConfigBundle{Agents: []AgentConfig{
		hierarchyAgent("bob", "carol"),
		hierarchyAgent("bob", "dave"),
	}}

	if _, err := env.svc.ApplyImport(ctx, testWorkspaceID, bundle); err == nil {
		t.Fatal("ApplyImport succeeded for a bundle with duplicate agent names")
	}
	agents, err := env.repo.ListAgentInstances(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("duplicate bundle partially wrote %d agent rows", len(agents))
	}
}
