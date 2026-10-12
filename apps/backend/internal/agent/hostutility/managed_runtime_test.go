package hostutility

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

type managedRuntimeSelectionReader struct {
	selection managedruntime.Selection
	found     bool
	err       error
}

type managedRuntimeOpenCodeSelectionStore struct {
	managedRuntimeSelectionReader
	openCodeSelection managedruntime.OpenCodeSelection
}

func (r managedRuntimeOpenCodeSelectionStore) GetOpenCodeSelection(context.Context) (managedruntime.OpenCodeSelection, bool, error) {
	return r.openCodeSelection, true, nil
}

func (r managedRuntimeOpenCodeSelectionStore) Save(context.Context, string, string, string) error {
	return nil
}
func (r managedRuntimeOpenCodeSelectionStore) Delete(context.Context, string, string) error {
	return nil
}

func (r managedRuntimeSelectionReader) Get(
	context.Context,
	string,
	string,
) (managedruntime.Selection, bool, error) {
	return r.selection, r.found, r.err
}

func TestResolveInferenceCommandUsesActiveExactVersion(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	manager := &Manager{managedRuntimeSelections: managedRuntimeSelectionReader{
		selection: managedruntime.Selection{Package: "opencode-ai", Version: "1.18.5"},
		found:     true,
	}}
	agent := agents.NewOpenCodeACP()

	command, err := manager.resolveInferenceCommand(context.Background(), agent.ID(), agent, agents.Command{})
	if err != nil {
		t.Fatalf("resolveInferenceCommand: %v", err)
	}
	want := agent.ManagedNPMRuntime().ACPCommand("1.18.5").Args()
	if got := command.Args(); !equalStrings(got, want) {
		t.Fatalf("command = %#v, want %#v", got, want)
	}
}

func TestResolveInferenceCommandPreservesLegacyAndReportsSelectionErrors(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	agent := agents.NewOpenCodeACP()
	manager := &Manager{managedRuntimeSelections: managedRuntimeSelectionReader{}}
	command, err := manager.resolveInferenceCommand(context.Background(), agent.ID(), agent, agents.Command{})
	if err != nil {
		t.Fatalf("legacy resolveInferenceCommand: %v", err)
	}
	if got, want := command.Args(), agent.ManagedNPMRuntime().CachedACPCommand().Args(); !equalStrings(got, want) {
		t.Fatalf("legacy command = %#v, want %#v", got, want)
	}

	wantErr := errors.New("selection read failed")
	manager.managedRuntimeSelections = managedRuntimeSelectionReader{err: wantErr}
	if _, err := manager.resolveInferenceCommand(context.Background(), agent.ID(), agent, agents.Command{}); !errors.Is(err, wantErr) {
		t.Fatalf("selection error = %v, want %v", err, wantErr)
	}
}

func TestResolveInferenceCommandUsesSelectedOpenCodeFamily(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	selectionStore := managedRuntimeOpenCodeSelectionStore{openCodeSelection: managedruntime.OpenCodeSelection{
		SchemaVersion: 1, Family: managedruntime.OpenCodeFamilyV2, Source: managedruntime.OpenCodeSourceManaged,
		Package: "@opencode/cli", SelectedVersion: "2.0.18", AppliedDefaultVersion: "2.0.18", Revision: 1,
	}}
	manager := &Manager{managedRuntimeSelections: selectionStore}
	openCode := agents.NewOpenCodeACP()
	command, err := manager.resolveInferenceCommand(context.Background(), openCode.ID(), openCode, agents.Command{})
	if err != nil {
		t.Fatalf("resolveInferenceCommand: %v", err)
	}
	want := []string{"npx", "--yes", "--prefer-offline", "--prefix", managedruntime.NPMProjectPrefix,
		"@opencode/cli@2.0.18", "acp", "--print-logs"}
	if !equalStrings(command.Args(), want) {
		t.Fatalf("command = %#v, want %#v", command.Args(), want)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
