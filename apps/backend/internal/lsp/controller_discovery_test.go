package lsp

import (
	"context"
	"errors"
	"testing"
)

func TestWorkspaceSourcesChangedAuthorizesBeforeTaskLookup(t *testing.T) {
	denied := errors.New("hidden task")
	tasks := &fakeControllerTasks{authErr: denied}
	controller := newTestController(tasks, newMemoryLSPStore(), &fakeLSPSettings{}, &fakeLSPRuntimes{})

	err := controller.WorkspaceSourcesChanged(context.Background(), "hidden")
	if !errors.Is(err, denied) {
		t.Fatalf("error = %v, want authorization error", err)
	}
	if got := tasks.callsSnapshot(); len(got) != 1 || got[0] != "authorize:hidden" {
		t.Fatalf("calls before denial = %v", got)
	}
}
