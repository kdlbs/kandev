package lsp

import (
	"context"
	"errors"
	"sync"
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

func TestWorkspaceSourcesChangedHoldsTaskAdmissionThroughRuntimeRefresh(t *testing.T) {
	admission := &sync.RWMutex{}
	tasks := &fakeControllerTasks{admissionLock: admission}
	host := newFakeLSPHost()
	host.workspaceRefreshEntered = make(chan struct{})
	host.workspaceRefreshRelease = make(chan struct{})
	controller := newTestController(tasks, newMemoryLSPStore(), &fakeLSPSettings{}, &fakeLSPRuntimes{host: host})

	done := make(chan error, 1)
	go func() {
		done <- controller.WorkspaceSourcesChanged(context.Background(), "task-1")
	}()
	<-host.workspaceRefreshEntered
	if admission.TryLock() {
		admission.Unlock()
		close(host.workspaceRefreshRelease)
		<-done
		t.Fatal("terminal task mutation acquired admission while workspace refresh was active")
	}
	close(host.workspaceRefreshRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !admission.TryLock() {
		t.Fatal("workspace refresh did not release task admission")
	}
	admission.Unlock()
}
