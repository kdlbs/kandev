package lsp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	sharedlsp "github.com/kandev/kandev/internal/lsp"
)

func TestRuntimeInitializeRequestContextUsesLifetime(t *testing.T) {
	lifetime, cancel := context.WithCancel(context.Background())
	runtime := &runtime{ctx: lifetime}
	initializeCtx := runtime.initializeRequestContext()

	if deadline, ok := initializeCtx.Deadline(); ok {
		t.Fatalf("initialize context has automatic deadline %s", deadline.Format(time.RFC3339Nano))
	}
	cancel()
	select {
	case <-initializeCtx.Done():
		if initializeCtx.Err() != context.Canceled {
			t.Fatalf("initialize context error = %v, want canceled", initializeCtx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("initialize context did not follow runtime cancellation")
	}
}

func TestNewRuntimePinsDocumentWorkspaceBeforeBrowserAttachment(t *testing.T) {
	workspacePath := t.TempDir()
	runtime := newRuntime(runtimeConfig{
		workspace: Config{WorkDir: workspacePath, WorkspaceURI: WorkspaceFileURI(workspacePath)},
	})
	t.Cleanup(runtime.hub.Close)
	want := WorkspaceFileURI(filepath.Join(workspacePath, "Main.kt"))

	got, err := runtime.hub.workspace.CanonicalURI(want)
	if err != nil {
		t.Fatalf("authorize runtime document before attachment: %v", err)
	}
	if got != want {
		t.Fatalf("canonical URI = %q, want %q", got, want)
	}
}

func TestManagerStopReapsRuntimeWhileInitializePending(t *testing.T) {
	server := newFakeLSPServer()
	server.holdInitialize = make(chan struct{})
	defer close(server.holdInitialize)
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer { return server })

	if _, err := manager.Start(context.Background(), StartRequest{
		Language: "kotlin", Generation: 1,
	}); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	<-server.initializeSeen
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseInitializing)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snapshot, err := manager.Stop(ctx, StopRequest{Language: "kotlin", Generation: 1})
	if err != nil {
		t.Fatalf("stop initializing generation: %v", err)
	}
	started, stopped, _ := processes.counts()
	if snapshot.Phase != sharedlsp.PhaseOff || started != 1 || stopped != 1 {
		t.Fatalf("snapshot=%#v started=%d stopped=%d", snapshot, started, stopped)
	}
}
