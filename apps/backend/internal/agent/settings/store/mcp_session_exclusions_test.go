package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

// @covers AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.2, AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.5, AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.6
func TestMCPSessionExclusionResolution(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	ctx := context.Background()
	def := testMCPDefinition()
	def.SecretBindings = []mcpconfig.MCPSecretBinding{{InputName: "Authorization", SecretID: "secret"}}
	if err := repo.CreateMCPServerDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	for _, r := range []string{"r1", "r2"} {
		if err := repo.ReplaceMCPSelections(ctx, mcpconfig.SelectionScopeRepository, def.WorkspaceID, r, []string{def.ID}); err != nil {
			t.Fatal(err)
		}
	}
	rc := mcpconfig.ResolutionContext{WorkspaceID: def.WorkspaceID, SessionID: "s", RepositoryIDs: []string{"r1", "r2"}}
	origin := mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeTaskSession, WorkspaceID: def.WorkspaceID, OwnerID: "s"}
	resolver := mcpconfig.NewResolver(repo, repo)
	secrets := 0
	resolver.SetSecretResolver(func(context.Context, string, string) (string, error) { secrets++; return "secret-value", nil })
	before, err := resolver.Resolve(ctx, rc, mcpconfig.Policy{AllowHTTP: true})
	if err != nil || len(before.Servers) != 1 || secrets != 1 {
		t.Fatalf("positive control: %#v %v secrets=%d", before, err, secrets)
	}
	if err := repo.SaveAcceptedMCPSetup(ctx, "s", 0, "accepted", before.Setup); err != nil {
		t.Fatal(err)
	}
	ids := []string{def.ID}
	revision := int64(0)
	_, err = repo.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{ExpectedRevision: &revision, ExcludedDefinitionIDs: &ids, ExclusionContext: &rc})
	if err != nil {
		t.Fatal(err)
	}
	secrets = 0
	after, err := resolver.Resolve(ctx, rc, mcpconfig.Policy{AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Servers) != 0 || secrets != 0 {
		t.Fatalf("excluded definition prepared/delivered: servers=%d secrets=%d", len(after.Servers), secrets)
	}
	if after.Setup == nil || len(after.Setup.Servers) != 0 || len(after.Decisions) != 1 || after.Decisions[0].ReasonCode != "session_excluded" || len(after.Decisions[0].Origins) != 2 {
		t.Fatalf("lost exclusion/origin evidence: %#v", after)
	}
	service := mcpconfig.NewSelectionService(repo, repo)
	service.SetSessionMCPStateRepository(repo)
	service.SetSessionResolutionContext(func(context.Context, string, string) (mcpconfig.ResolutionContext, error) { return rc, nil })
	snapshot, err := service.ReadSnapshot(ctx, origin)
	if err != nil {
		t.Fatal(err)
	}
	if c := snapshot.State.Comparison; c.Status != "known" || len(c.Changes) != 1 || c.Changes[0].Action != "remove" {
		t.Fatalf("comparison did not use post-exclusion setup: %#v", c)
	}
	rc.SessionID = "sibling"
	sibling, err := resolver.Resolve(ctx, rc, mcpconfig.Policy{AllowHTTP: true})
	if err != nil || len(sibling.Servers) != 1 {
		t.Fatalf("changed sibling: %#v %v", sibling, err)
	}
}

func seedSessionExclusion(t *testing.T, repo *sqliteRepository) (mcpconfig.SelectionOrigin, mcpconfig.ResolutionContext, *mcpconfig.MCPServerDefinition) {
	t.Helper()
	ctx := context.Background()
	def := testMCPDefinition()
	if err := repo.CreateMCPServerDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceMCPSelections(ctx, mcpconfig.SelectionScopeRepository, def.WorkspaceID, "r", []string{def.ID}); err != nil {
		t.Fatal(err)
	}
	rc := mcpconfig.ResolutionContext{WorkspaceID: def.WorkspaceID, SessionID: "s", RepositoryIDs: []string{"r"}}
	origin := mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeTaskSession, WorkspaceID: def.WorkspaceID, OwnerID: "s"}
	ids := []string{def.ID}
	revision := int64(0)
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{ExpectedRevision: &revision, ExcludedDefinitionIDs: &ids, ExclusionContext: &rc}); err != nil {
		t.Fatal(err)
	}
	return origin, rc, def
}

// @covers AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.3, AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.4, AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.5
func TestMCPSessionExclusionDormantReplay(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	origin, rc, def := seedSessionExclusion(t, repo)
	ctx := context.Background()
	if err := repo.ReplaceMCPSelections(ctx, mcpconfig.SelectionScopeRepository, def.WorkspaceID, "r", nil); err != nil {
		t.Fatal(err)
	}
	def.Enabled = false
	if err := repo.UpdateMCPServerDefinition(ctx, def, def.Revision); err != nil {
		t.Fatal(err)
	}
	reopened, err := newSQLiteRepositoryWithDB(repo.db, repo.ro, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reopened.ReadMCPSelectionSnapshot(ctx, origin)
	if err != nil || len(snapshot.ExcludedDefinitionIDs) != 1 {
		t.Fatalf("replay lost dormant exclusion: %#v %v", snapshot, err)
	}
	// Retaining a disabled dormant exclusion must remain valid.
	ids := []string{def.ID}
	revision := snapshot.State.DesiredRevision
	if _, err := reopened.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{ExpectedRevision: &revision, ExcludedDefinitionIDs: &ids, ExclusionContext: &rc}); err != nil {
		t.Fatal(err)
	}
	revision++
	empty := []string{}
	restored, err := reopened.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{ExpectedRevision: &revision, ExcludedDefinitionIDs: &empty, ExclusionContext: &rc})
	if err != nil || len(restored.DefinitionIDs) != 0 || len(restored.ExcludedDefinitionIDs) != 0 {
		t.Fatalf("restore added a dormant definition: %#v %v", restored, err)
	}
	current, err := repo.GetMCPServerDefinition(ctx, def.WorkspaceID, def.ID)
	if err != nil || current.Enabled {
		t.Fatalf("restore enabled definition: %#v %v", current, err)
	}
}

func TestMCPSessionExclusionCleanup(t *testing.T) {
	for _, kind := range []string{"task", "workspace", "definition", "definition_direct", "session"} {
		t.Run(kind, func(t *testing.T) {
			repo := newTestRepo(t).(*sqliteRepository)
			origin, _, def := seedSessionExclusion(t, repo)
			ctx := context.Background()
			var err error
			switch kind {
			case "task":
				err = repo.DeleteMCPTaskData(ctx, "task", []string{"s"})
			case "workspace":
				err = repo.DeleteMCPWorkspaceData(ctx, def.WorkspaceID)
			case "definition":
				err = repo.DeleteMCPServerDefinitionWithSelections(ctx, def.WorkspaceID, def.ID, def.Revision)
			case "definition_direct":
				err = repo.DeleteMCPServerDefinition(ctx, def.WorkspaceID, def.ID, def.Revision)
			case "session":
				cleaner, ok := any(repo).(interface {
					DeleteMCPSessionData(context.Context, string) error
				})
				if !ok {
					t.Fatal("missing individual-session cleanup")
				}
				err = cleaner.DeleteMCPSessionData(ctx, "s")
			}
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := repo.ReadMCPSelectionSnapshot(ctx, origin)
			if err != nil || len(snapshot.ExcludedDefinitionIDs) != 0 {
				t.Fatalf("orphan exclusion after %s: %#v %v", kind, snapshot, err)
			}
			if kind != "definition" && kind != "definition_direct" && snapshot.State != nil {
				t.Fatalf("orphan apply state after %s", kind)
			}
		})
	}
}
