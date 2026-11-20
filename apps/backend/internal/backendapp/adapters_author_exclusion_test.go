package backendapp

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/office/routing"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

type fakeExclusionSetup struct {
	gotWorkspace string
	gotDecls     []routing.ExclusionDeclaration
	warnings     []routing.SetupWarning
	err          error
}

func (f *fakeExclusionSetup) ExclusionSetup(
	_ context.Context, workspaceID string, decls []routing.ExclusionDeclaration,
) ([]routing.SetupWarning, error) {
	f.gotWorkspace, f.gotDecls = workspaceID, decls
	return f.warnings, f.err
}

func TestImportExclusionAdvisorAdapterConvertsBothWays(t *testing.T) {
	decl := routing.ExclusionDeclaration{WorkflowName: "W", StepName: "Review", Role: "reviewer"}
	setup := &fakeExclusionSetup{warnings: []routing.SetupWarning{{
		Code: routing.SetupWarningAgentSingleProvider, Declaration: decl,
		ProviderID: "codex-acp", AgentID: "q", AgentName: "QA",
	}}}
	adapter := &importExclusionAdvisorAdapter{setup: setup}

	got, err := adapter.AdviseExclusionSetup(context.Background(), "ws-1",
		[]workflowmodels.AuthorExclusionDeclaration{{WorkflowName: "W", StepName: "Review", Role: "reviewer"}})
	if err != nil {
		t.Fatalf("advise: %v", err)
	}
	if setup.gotWorkspace != "ws-1" || !reflect.DeepEqual(setup.gotDecls, []routing.ExclusionDeclaration{decl}) {
		t.Fatalf("forwarded %q %+v", setup.gotWorkspace, setup.gotDecls)
	}
	want := []workflowservice.ImportWarning{{
		Code: "agent_single_provider", WorkflowName: "W", StepName: "Review", Role: "reviewer",
		ProviderID: "codex-acp", AgentName: "QA",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings = %+v, want %+v", got, want)
	}

	setup.err = errors.New("boom")
	if _, err := adapter.AdviseExclusionSetup(context.Background(), "ws-1",
		[]workflowmodels.AuthorExclusionDeclaration{{WorkflowName: "W"}}); err == nil {
		t.Fatal("expected error to propagate")
	}
}
