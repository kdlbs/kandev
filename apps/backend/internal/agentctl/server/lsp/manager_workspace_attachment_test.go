package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sharedlsp "github.com/kandev/kandev/internal/lsp"
)

func TestManagerSerializesWorkspaceCommitAndLiveApplyPerTask(t *testing.T) {
	server := newFakeLSPServer()
	server.workspaceChanges = make(chan json.RawMessage, 2)
	server.capabilities = map[string]any{
		fieldWorkspace: map[string]any{
			fieldWorkspaceFolders: map[string]any{"supported": true, "changeNotifications": true},
		},
	}
	manager, _ := newManagerForTest(t, func(int) *fakeLSPServer { return server })
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatal(err)
	}
	waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	firstPath := t.TempDir()
	secondPath := t.TempDir()
	first := []WorkspaceFolder{{URI: WorkspaceFileURI(firstPath), Name: "first"}}
	second := []WorkspaceFolder{{URI: WorkspaceFileURI(secondPath), Name: "second"}}
	firstCommitted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var hookOnce sync.Once
	manager.workspaceCommitted = func(_ string, config Config) {
		if len(config.WorkspaceFolders) == 1 && config.WorkspaceFolders[0].Name == "first" {
			hookOnce.Do(func() { close(firstCommitted) })
			<-releaseFirst
		}
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := manager.UpdateWorkspaceFoldersForTask("task-1", first)
		firstDone <- err
	}()
	<-firstCommitted
	secondDone := make(chan error, 1)
	go func() {
		_, err := manager.UpdateWorkspaceFoldersForTask("task-1", second)
		secondDone <- err
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("second workspace update escaped task serialization: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	manager.workspaceCommitted = nil

	config := manager.configSnapshot("task-1")
	snapshot := manager.SnapshotForTask("task-1", "kotlin")
	slot, err := manager.slotForTask("task-1", "kotlin")
	if err != nil {
		t.Fatal(err)
	}
	slot.opMu.Lock()
	runtimeFolders := append([]WorkspaceFolder(nil), slot.runtime.workspace.WorkspaceFolders...)
	workspace := slot.runtime.hub.workspace
	slot.opMu.Unlock()
	workspace.mu.RLock()
	hubRoots := append([]documentRoot(nil), workspace.roots...)
	workspace.mu.RUnlock()
	for name, folders := range map[string][]WorkspaceFolder{
		"config": config.WorkspaceFolders, "runtime": runtimeFolders, "snapshot": snapshot.WorkspaceFolders,
	} {
		if len(folders) != 1 || folders[0].Name != "second" {
			t.Fatalf("%s workspace = %#v, want second", name, folders)
		}
	}
	var foundFirst, foundSecond bool
	for _, root := range hubRoots {
		foundFirst = foundFirst || root.lexical == filepath.Clean(firstPath)
		foundSecond = foundSecond || root.lexical == filepath.Clean(secondPath)
	}
	if foundFirst || !foundSecond {
		t.Fatalf("hub roots = %#v, want only latest folder root", hubRoots)
	}

	var lastChange struct {
		Event struct {
			Added   []WorkspaceFolder `json:"added"`
			Removed []WorkspaceFolder `json:"removed"`
		} `json:"event"`
	}
	for index := 0; index < 2; index++ {
		select {
		case change := <-server.workspaceChanges:
			if index == 1 {
				if err := json.Unmarshal(change, &lastChange); err != nil {
					t.Fatal(err)
				}
			}
		case <-time.After(time.Second):
			t.Fatal("workspace notification was not observed")
		}
	}
	if len(lastChange.Event.Added) != 1 || lastChange.Event.Added[0].Name != "second" ||
		len(lastChange.Event.Removed) != 1 || lastChange.Event.Removed[0].Name != "first" {
		t.Fatalf("last workspace notification = %#v, want first removed and second added", lastChange.Event)
	}
}

func TestManagerRequiresRestartWhenTaskWorkspaceRootChanges(t *testing.T) {
	server := newFakeLSPServer()
	server.capabilities = map[string]any{
		fieldWorkspace: map[string]any{
			fieldWorkspaceFolders: map[string]any{"supported": true, "changeNotifications": true},
		},
	}
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer { return server })
	initialRoot := filepath.Join(t.TempDir(), "single-repository")
	promotedRoot := filepath.Join(t.TempDir(), "task-workspace")
	promotedRepositories := []string{
		filepath.Join(promotedRoot, "repository-a"),
		filepath.Join(promotedRoot, "repository-b"),
	}
	if _, err := manager.UpdateWorkspaceForTask("task-1", initialRoot, []string{initialRoot}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(context.Background(), StartRequest{Language: "kotlin", Generation: 1}); err != nil {
		t.Fatal(err)
	}
	before := waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)

	result, err := manager.UpdateWorkspaceForTask("task-1", promotedRoot, promotedRepositories)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RestartRequiredLanguages) != 1 || result.RestartRequiredLanguages[0] != "kotlin" ||
		len(result.DynamicLanguages) != 0 {
		t.Fatalf("root-change result = %#v", result)
	}
	select {
	case change := <-server.workspaceChanges:
		t.Fatalf("root change was sent as an in-place folder update: %s", change)
	case <-time.After(50 * time.Millisecond):
	}
	foldersOnlyResult, err := manager.UpdateWorkspaceFoldersForTask(
		"task-1",
		WorkspaceFoldersAtRoots(promotedRoot, promotedRepositories),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(foldersOnlyResult.RestartRequiredLanguages) != 1 ||
		foldersOnlyResult.RestartRequiredLanguages[0] != "kotlin" ||
		len(foldersOnlyResult.DynamicLanguages) != 0 {
		t.Fatalf("folder refresh after root change = %#v", foldersOnlyResult)
	}
	after := manager.Snapshot("kotlin")
	started, stopped, _ := processes.counts()
	if started != 1 || stopped != 0 || after.WorkspaceURI != before.WorkspaceURI ||
		after.WorkspacePath != before.WorkspacePath {
		t.Fatalf("root change mutated live generation: started=%d stopped=%d before=%#v after=%#v", started, stopped, before, after)
	}

	if _, err := manager.Restart(context.Background(), StartRequest{Language: "kotlin", Generation: 2}); err != nil {
		t.Fatal(err)
	}
	restarted := waitForPhase(t, manager, "kotlin", sharedlsp.PhaseReady)
	if restarted.WorkspacePath != promotedRoot || restarted.WorkspaceURI != WorkspaceFileURI(promotedRoot) ||
		len(restarted.WorkspaceFolders) != len(promotedRepositories) {
		t.Fatalf("restarted workspace = %#v", restarted)
	}
}

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
