package process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

var dirtyMonitorPaths = []struct{ name, path string }{
	{"plain", "plain.txt"},
	{"leading_space", " leading.txt"},
	{"trailing_space", "trailing.txt "},
	{"tab", "tab\tname.txt"},
	{"newline", "line\nname.txt"},
	{"double_quote", "quote\"name.txt"},
	{"unicode", "café.txt"},
	{"punctuation", "semi;colon.txt"},
}

func requireMonitorFilename(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" && (strings.ContainsAny(path, "\t\n\"") || strings.HasSuffix(path, " ")) {
		t.Skip("Windows does not support this filename shape")
	}
}

func writeMonitorVersion(t *testing.T, dir, path, version string, offset int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, path, version+"\n")
	stamp := time.Unix(1700000000+int64(offset)*10, 0)
	if err := os.Chtimes(filepath.Join(dir, path), stamp, stamp); err != nil {
		t.Fatal(err)
	}
}

func dirtyMonitorFixture(t *testing.T, path, quoteMode string) (*WorkspaceTracker, []string) {
	t.Helper()
	requireMonitorFilename(t, path)
	dir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	if quoteMode != "default" {
		runGit(t, dir, "config", "core.quotePath", quoteMode)
	}
	writeMonitorVersion(t, dir, path, "baseline", 0)
	runGit(t, dir, "add", "--", path)
	runGit(t, dir, "commit", "-m", "tracked monitor fixture")
	// Ignored decoys cannot independently trigger the untracked fingerprint.
	if err := os.WriteFile(filepath.Join(dir, ".git", "info", "exclude"), []byte("*\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidates := []string{strings.TrimSpace(path), strconv.Quote(path), strconv.QuoteToASCII(path),
		strings.NewReplacer("\t", "\\t", "\n", "\\n", "\"", "\\\"").Replace(path)}
	decoys := make([]string, 0)
	seen := map[string]bool{path: true}
	for _, candidate := range candidates {
		if seen[candidate] || (runtime.GOOS == "windows" && strings.ContainsAny(candidate, "\"")) {
			continue
		}
		seen[candidate] = true
		writeMonitorVersion(t, dir, candidate, "ignored decoy", -2)
		decoys = append(decoys, candidate)
	}
	tracker := NewWorkspaceTracker(dir, newTestLogger(t))
	t.Cleanup(tracker.Stop)
	return tracker, decoys
}

func readMonitorState(t *testing.T, tracker *WorkspaceTracker) workspaceState {
	t.Helper()
	state, err := tracker.getWorkspaceState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func requireSameMonitorDependencies(t *testing.T, before, after workspaceState) {
	t.Helper()
	if before.indexMtime != after.indexMtime || before.untrackedID != after.untrackedID {
		t.Fatalf("index or untracked evidence changed: before %+v, after %+v", before, after)
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.42
func TestWorkspaceStateDirtyExactPaths(t *testing.T) {
	for _, quoteMode := range []string{"default", "true", "false"} {
		for _, test := range dirtyMonitorPaths {
			t.Run(quoteMode+"/"+test.name, func(t *testing.T) {
				tracker, decoys := dirtyMonitorFixture(t, test.path, quoteMode)
				clean := readMonitorState(t, tracker)
				writeMonitorVersion(t, tracker.workDir, test.path, "first dirty", 1)
				first := readMonitorState(t, tracker)
				requireSameMonitorDependencies(t, clean, first)
				if !first.changed(clean) {
					t.Fatal("first dirty write was not detected")
				}
				writeMonitorVersion(t, tracker.workDir, test.path, "second dirty", 2)
				second := readMonitorState(t, tracker)
				requireSameMonitorDependencies(t, first, second)
				if !second.changed(first) {
					t.Errorf("second edit to exact dirty path %q was not detected", test.path)
				}
				if readMonitorState(t, tracker).changed(second) {
					t.Fatal("stable worktree changed its fingerprint")
				}
				for _, decoy := range decoys {
					writeMonitorVersion(t, tracker.workDir, decoy, "changed ignored decoy", 3)
				}
				if readMonitorState(t, tracker).changed(second) {
					t.Fatal("ignored similar decoy supplied dirty-path modification evidence")
				}
			})
		}
	}
}

func captureMonitorMessages(sub types.WorkspaceStreamSubscriber) []types.WorkspaceStreamMessage {
	var messages []types.WorkspaceStreamMessage
	for {
		select {
		case message := <-sub:
			messages = append(messages, message)
		default:
			return messages
		}
	}
}

func requireMonitorRefresh(t *testing.T, tracker *WorkspaceTracker, sub types.WorkspaceStreamSubscriber, path, version string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// A cached detail wait settles the tick's worker without starting a fresh read.
	status, err := tracker.GetGitStatusWithDetails(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if !status.FilesComplete || status.RepositoryName != "monitor-repo" || !strings.Contains(status.Files[path].Diff, "+"+version) {
		t.Errorf("tick status did not publish exact new file content for %q: %+v", path, status)
	}
	refreshes, statuses := 0, 0
	for _, message := range captureMonitorMessages(sub) {
		if message.GitStatus != nil {
			statuses++
			if message.GitStatus.RepositoryName != "monitor-repo" {
				t.Errorf("status has wrong repository: %+v", message.GitStatus)
			}
		}
		if message.FileChange != nil {
			refreshes++
			event := message.FileChange
			if event.Operation != types.FileOpRefresh || event.Path != "" || event.RepositoryName != "monitor-repo" || event.Timestamp.IsZero() {
				t.Errorf("wrong file refresh payload: %+v", event)
			}
		}
	}
	if refreshes != 1 || statuses == 0 {
		t.Errorf("tick published %d file refreshes and %d statuses, want one refresh and accepted status", refreshes, statuses)
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.42
func TestMonitorTickDirtyExactPaths(t *testing.T) {
	for _, quoteMode := range []string{"default", "true", "false"} {
		for _, test := range dirtyMonitorPaths {
			t.Run(quoteMode+"/"+test.name, func(t *testing.T) {
				tracker, _ := dirtyMonitorFixture(t, test.path, quoteMode)
				tracker.repositoryName = "monitor-repo"
				sub := make(types.WorkspaceStreamSubscriber, 32)
				tracker.workspaceStreamSubscribers[sub] = struct{}{}
				t.Cleanup(func() { tracker.DetachWorkspaceStreamSubscriber(sub) })
				last := readMonitorState(t, tracker)
				failures := 0
				for i, version := range []string{"first dirty", "second dirty with more content"} {
					previous := tracker.currentFiles.Timestamp
					writeMonitorVersion(t, tracker.workDir, test.path, version, i+1)
					if tracker.monitorTick(context.Background(), &last, &failures) || failures != 0 {
						t.Fatal("healthy direct tick stopped or failed")
					}
					requireMonitorRefresh(t, tracker, sub, test.path, version)
					if !tracker.currentFiles.Timestamp.After(previous) || tracker.currentFiles.RepositoryName != "monitor-repo" {
						t.Errorf("tick did not recapture repository file cache: %+v", tracker.currentFiles)
					}
					foundControl := false
					for _, file := range tracker.currentFiles.Files {
						foundControl = foundControl || file.Path == "README.md"
					}
					if !foundControl {
						t.Error("refreshed file cache lost tracked README control")
					}
				}
				beforeStatus := tracker.currentGitStatus()
				beforeFiles := tracker.currentFiles.Timestamp
				tracker.monitorTick(context.Background(), &last, &failures)
				if len(captureMonitorMessages(sub)) != 0 || tracker.currentFiles.Timestamp != beforeFiles || tracker.currentGitStatus().SnapshotRevision != beforeStatus.SnapshotRevision {
					t.Fatal("settled no-op tick refreshed cache or subscribers")
				}
			})
		}
	}
}

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.14
// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.42
func TestWorkspaceStateDirtyPathTransitions(t *testing.T) {
	t.Run("tracked_dependency_and_deletion", func(t *testing.T) {
		path := "node_modules/tracked.txt"
		tracker, _ := dirtyMonitorFixture(t, path, "default")
		writeMonitorVersion(t, tracker.workDir, path, "first dirty", 1)
		first := readMonitorState(t, tracker)
		writeMonitorVersion(t, tracker.workDir, path, "second dirty", 2)
		second := readMonitorState(t, tracker)
		requireSameMonitorDependencies(t, first, second)
		if !second.changed(first) {
			t.Fatal("tracked dependency edit was not detected")
		}
		if err := os.Remove(filepath.Join(tracker.workDir, path)); err != nil {
			t.Fatal(err)
		}
		deleted := readMonitorState(t, tracker)
		requireSameMonitorDependencies(t, second, deleted)
		if !deleted.changed(second) || readMonitorState(t, tracker).changed(deleted) {
			t.Fatal("dirty deletion was not detected or was unstable")
		}
		status, err := tracker.GetGitStatus(context.Background(), true)
		if err != nil || status.Files[path].Status != fileStatusDeleted {
			t.Fatalf("deleted membership = %+v, error %v", status.Files[path], err)
		}
		if _, err := tracker.sanitizePath("../outside.txt"); err == nil {
			t.Fatal("workspace traversal was accepted")
		}
	})
	t.Run("staged_rename_destination", func(t *testing.T) {
		path := " renamed.txt"
		tracker, _ := dirtyMonitorFixture(t, "original.txt", "false")
		runGit(t, tracker.workDir, "mv", "--", "original.txt", path)
		renamed := readMonitorState(t, tracker)
		writeMonitorVersion(t, tracker.workDir, path, "first dirty", 1)
		first := readMonitorState(t, tracker)
		writeMonitorVersion(t, tracker.workDir, path, "second dirty", 2)
		second := readMonitorState(t, tracker)
		requireSameMonitorDependencies(t, renamed, second)
		if !first.changed(renamed) || !second.changed(first) || readMonitorState(t, tracker).changed(second) {
			t.Fatal("renamed destination dirty edits were not detected or were unstable")
		}
		status, err := tracker.GetGitStatus(context.Background(), true)
		file := status.Files[path]
		if err != nil || file.OldPath != "original.txt" || file.StagedChange == nil || file.UnstagedChange == nil {
			t.Fatalf("mixed rename membership = %+v, error %v", file, err)
		}
	})
}
