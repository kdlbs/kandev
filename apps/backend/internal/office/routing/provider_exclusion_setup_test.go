package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestProviderStoredExclusionSetup(t *testing.T) {
	decl := ExclusionDeclaration{WorkflowName: "W", StepName: "Review", Role: "reviewer"}
	repo := &fakeProviderRepo{
		cfg:    &WorkspaceConfig{Enabled: true, ProviderOrder: []ProviderID{"claude-acp"}},
		agents: []*models.AgentInstance{{ID: "a", Role: models.AgentRoleCEO, Settings: "{}"}},
		decls:  []ExclusionDeclaration{decl},
	}
	p := NewProvider(repo, nil, nil, nil)
	got, err := p.StoredExclusionSetup(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("StoredExclusionSetup: %v", err)
	}
	if len(got) != 1 || got[0].Code != SetupWarningSingleProvider || got[0].ProviderID != "claude-acp" {
		t.Fatalf("warnings = %+v", got)
	}

	repo.declsErr = errors.New("boom")
	if _, err := p.StoredExclusionSetup(context.Background(), "ws-1"); err == nil {
		t.Fatal("expected declaration read error")
	}
}

func TestProviderExclusionSetupSkipsReadsWithoutDeclarations(t *testing.T) {
	repo := &fakeProviderRepo{cfg: &WorkspaceConfig{Enabled: false}}
	p := NewProvider(repo, nil, nil, nil)
	got, err := p.ExclusionSetup(context.Background(), "ws-1", nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("warnings = %+v, err = %v; want none", got, err)
	}
	got, err = p.ExclusionSetup(context.Background(), "ws-1",
		[]ExclusionDeclaration{{WorkflowName: "W", StepName: "Review", Role: "reviewer"}})
	if err != nil || len(got) != 1 || got[0].Code != SetupWarningRoutingDisabled {
		t.Fatalf("warnings = %+v, err = %v; want routing_disabled", got, err)
	}
	if repo.declReads != 0 {
		t.Fatalf("explicit declarations must not read stored ones; reads = %d", repo.declReads)
	}
}
