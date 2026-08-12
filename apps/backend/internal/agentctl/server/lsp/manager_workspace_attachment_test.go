package lsp

import (
	"context"
	"errors"
	"testing"

	sharedlsp "github.com/kandev/kandev/internal/lsp"
)

func TestManagerMarksPureWorkspaceFolderReorderForRestart(t *testing.T) {
	server := newFakeLSPServer()
	server.capabilities = map[string]any{
		fieldWorkspace: map[string]any{
			fieldWorkspaceFolders: map[string]any{"supported": true, "changeNotifications": true},
		},
	}
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer { return server })
	initial := []WorkspaceFolder{
		{URI: "file:///workspace/repo-b", Name: "repo-b"},
		{URI: "file:///workspace/repo-a", Name: "repo-a"},
	}
	if _, err := manager.UpdateWorkspaceFolders(initial); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatal(err)
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)

	result, err := manager.UpdateWorkspaceFolders([]WorkspaceFolder{initial[1], initial[0]})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RestartRequiredLanguages) != 1 || result.RestartRequiredLanguages[0] != "kotlin" ||
		len(result.DynamicLanguages) != 0 {
		t.Fatalf("workspace reorder result = %#v", result)
	}
	after := manager.Snapshot("kotlin")
	started, stopped, _ := processes.counts()
	if started != 1 || stopped != 0 || len(after.WorkspaceFolders) != 2 ||
		after.WorkspaceFolders[0].Name != "repo-b" {
		t.Fatalf("reorder changed live scope: started=%d stopped=%d snapshot=%#v", started, stopped, after)
	}
}

func TestManagerMarksStaticWorkspaceFolderServerForExplicitRestart(t *testing.T) {
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer { return newFakeLSPServer() })
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatal(err)
	}
	before := waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	folders := []WorkspaceFolder{{URI: "file:///workspace/new-repo", Name: "new-repo"}}

	result, err := manager.UpdateWorkspaceFolders(folders)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RestartRequiredLanguages) != 1 || result.RestartRequiredLanguages[0] != "kotlin" ||
		len(result.DynamicLanguages) != 0 {
		t.Fatalf("workspace result = %#v", result)
	}
	after := manager.Snapshot("kotlin")
	started, stopped, _ := processes.counts()
	if started != 1 || stopped != 0 || len(after.WorkspaceFolders) != 1 ||
		after.WorkspaceFolders[0].URI != before.WorkspaceFolders[0].URI {
		t.Fatalf("static server scope changed: before=%#v after=%#v", before, after)
	}

	if _, err := manager.Restart(context.Background(), StartRequest{Language: "kotlin", Generation: 2}); err != nil {
		t.Fatal(err)
	}
	restarted := waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	if len(restarted.WorkspaceFolders) != 1 || restarted.WorkspaceFolders[0].Name != "new-repo" {
		t.Fatalf("restarted workspace folders = %#v", restarted.WorkspaceFolders)
	}
}

func TestAttachmentDisconnectDoesNotStopManagerRuntime(t *testing.T) {
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer {
		return newFakeLSPServer()
	})
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	attachment, err := manager.Attach("kotlin", 1)
	if err != nil {
		t.Fatalf("attach generation: %v", err)
	}
	drainAttached(t, attachment)
	attachment.Close()

	started, stopped, _ := processes.counts()
	if started != 1 || stopped != 0 {
		t.Fatalf("attachment disconnect changed lifecycle: started=%d stopped=%d", started, stopped)
	}
	if snapshot := manager.Snapshot("kotlin"); snapshot.Phase != sharedlsp.PhaseReady {
		t.Fatalf("snapshot after detach = %#v", snapshot)
	}
}

func TestAttachmentStaleGenerationRejectedAndOldHubClosesOnRestart(t *testing.T) {
	manager, _ := newManagerForTest(t, func(int) *fakeLSPServer {
		return newFakeLSPServer()
	})
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatalf("start generation: %v", err)
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	if _, err := manager.Attach("kotlin", 2); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale attach error = %v, want %v", err, ErrStaleGeneration)
	}
	attachment, err := manager.Attach("kotlin", 1)
	if err != nil {
		t.Fatalf("attach generation: %v", err)
	}
	drainAttached(t, attachment)

	if _, err := manager.Restart(context.Background(), StartRequest{Language: "kotlin", Generation: 2}); err != nil {
		t.Fatalf("restart generation: %v", err)
	}
	for range attachment.Messages() {
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	if _, err := manager.Attach("kotlin", 1); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("old generation attach error = %v, want %v", err, ErrStaleGeneration)
	}
}
