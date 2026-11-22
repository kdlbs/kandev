package process

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/subproc"
)

const (
	worktreeListTimeout = 10 * time.Second
	// worktreeFullListEveryNTicks catches a branch switch inside a worktree
	// that stayed in detached HEAD: the worktrees directory stat does not move
	// in that case.
	worktreeFullListEveryNTicks = 10
)

// worktreeObserverState is owned by the tracker's git poll goroutine.
type worktreeObserverState struct {
	commonDir        string
	worktreesStamp   string
	ticks            int
	lastEntries      []types.WorktreeInventoryEntry
	publishedOnce    bool
	commonDirFailure int
}

// SetWorkspaceRoot sets the task workspace root used to contain discovery.
// The root is resolved so a symlinked workspace and its resolved worktrees
// compare on the same canonical path.
func (wt *WorkspaceTracker) SetWorkspaceRoot(root string) {
	if strings.TrimSpace(root) == "" {
		wt.workspaceRoot = ""
		return
	}
	cleaned := filepath.Clean(root)
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		cleaned = resolved
	}
	wt.workspaceRoot = cleaned
}

// observeWorktreeInventory publishes a filtered linked-worktree snapshot when
// the list changes. It never writes Git state and never mutates the tracker
// graph.
func (wt *WorkspaceTracker) observeWorktreeInventory(ctx context.Context) {
	if wt.workspaceRoot == "" || wt.gitIndexPath == "" || wt.IsSubmodule() {
		return
	}
	commonDir, err := wt.resolveWorktreeCommonDir(ctx)
	if err != nil || commonDir == "" {
		wt.worktreeObserver.commonDirFailure++
		return
	}
	wt.worktreeObserver.commonDir = commonDir
	wt.worktreeObserver.commonDirFailure = 0
	stamp := worktreesDirectoryStamp(commonDir)
	wt.worktreeObserver.ticks++
	full := wt.worktreeObserver.ticks == 1 ||
		wt.worktreeObserver.ticks%worktreeFullListEveryNTicks == 0
	if !full && stamp == wt.worktreeObserver.worktreesStamp {
		return
	}
	entries, err := wt.listWorkspaceWorktrees(ctx)
	if err != nil {
		return
	}
	wt.worktreeObserver.worktreesStamp = stamp
	if sameWorktreeInventory(entries, wt.worktreeObserver.lastEntries) &&
		wt.worktreeObserver.publishedOnce {
		return
	}
	wt.worktreeObserver.lastEntries = entries
	wt.worktreeObserver.publishedOnce = true
	inventory := &types.WorktreeInventory{
		RepositoryName: wt.repositoryName,
		CommonDir:      commonDir,
		Entries:        entries,
		ObservedAt:     time.Now().UTC(),
	}
	wt.worktreeInventoryMu.Lock()
	wt.latestWorktreeInventory = inventory
	wt.worktreeInventoryMu.Unlock()
	if !wt.notifyWorkspaceStreamWorktreeInventory(inventory) {
		// A full subscriber queue dropped this change. Force the next tick to
		// publish it again instead of treating the list as already delivered.
		wt.worktreeObserver.publishedOnce = false
		wt.worktreeObserver.worktreesStamp = ""
	}
}

func (wt *WorkspaceTracker) replayWorktreeInventory(sub types.WorkspaceStreamSubscriber) {
	wt.worktreeInventoryMu.RLock()
	inventory := wt.latestWorktreeInventory
	wt.worktreeInventoryMu.RUnlock()
	if inventory == nil {
		return
	}
	select {
	case sub <- types.NewWorkspaceWorktreeInventory(inventory):
	default:
	}
}

func (wt *WorkspaceTracker) resolveWorktreeCommonDir(ctx context.Context) (string, error) {
	if wt.worktreeObserver.commonDir != "" && wt.worktreeObserver.commonDirFailure == 0 {
		return wt.worktreeObserver.commonDir, nil
	}
	execCtx, cancel := context.WithTimeout(ctx, worktreeListTimeout)
	defer cancel()
	cmd := subproc.NewGitCommand(execCtx, "-C", wt.workDir, "rev-parse", "--git-common-dir")
	out, err := subproc.RunGitOutputClass(execCtx, subproc.GitBackground, cmd)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", os.ErrNotExist
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(wt.workDir, path)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func worktreesDirectoryStamp(commonDir string) string {
	worktreesDir := filepath.Join(commonDir, "worktrees")
	info, err := os.Stat(worktreesDir)
	if err != nil || !info.IsDir() {
		return "absent"
	}
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return "unreadable"
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return info.ModTime().UTC().Format(time.RFC3339Nano) + "|" + strings.Join(names, ",")
}

func (wt *WorkspaceTracker) listWorkspaceWorktrees(ctx context.Context) ([]types.WorktreeInventoryEntry, error) {
	execCtx, cancel := context.WithTimeout(ctx, worktreeListTimeout)
	defer cancel()
	cmd := subproc.NewGitCommand(execCtx, "-C", wt.workDir, "worktree", "list", "--porcelain", "-z")
	out, err := subproc.RunGitOutputClass(execCtx, subproc.GitBackground, cmd)
	if err != nil {
		return nil, err
	}
	records := parseWorktreePorcelain(out)
	entries := make([]types.WorktreeInventoryEntry, 0, len(records))
	self, _ := filepath.EvalSymlinks(filepath.Clean(wt.workDir))
	for _, record := range records {
		if record.bare || record.prunable || record.detached || record.branch == "" || record.path == "" {
			continue
		}
		resolved, err := filepath.EvalSymlinks(filepath.Clean(record.path))
		if err != nil {
			continue
		}
		if resolved == self || !pathWithinWorkspaceRoot(resolved, wt.workspaceRoot) {
			continue
		}
		entries = append(entries, types.WorktreeInventoryEntry{
			Path:    resolved,
			Branch:  strings.TrimPrefix(record.branch, "refs/heads/"),
			HeadSHA: record.head,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

type worktreePorcelainRecord struct {
	path     string
	head     string
	branch   string
	bare     bool
	prunable bool
	detached bool
}

func parseWorktreePorcelain(out []byte) []worktreePorcelainRecord {
	var records []worktreePorcelainRecord
	var current worktreePorcelainRecord
	started := false
	flush := func() {
		if started {
			records = append(records, current)
		}
		current = worktreePorcelainRecord{}
		started = false
	}
	for _, field := range strings.Split(string(out), "\x00") {
		if field == "" {
			flush()
			continue
		}
		key, value, _ := strings.Cut(field, " ")
		switch key {
		case "worktree":
			flush()
			started = true
			current.path = value
		case "HEAD":
			current.head = value
		case "branch":
			current.branch = value
		case "detached":
			current.detached = true
		case "bare":
			current.bare = true
		case "prunable":
			current.prunable = true
		}
	}
	flush()
	return records
}

func pathWithinWorkspaceRoot(path, root string) bool {
	if path == "" || root == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func sameWorktreeInventory(left, right []types.WorktreeInventoryEntry) bool {
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

func (wt *WorkspaceTracker) notifyWorkspaceStreamWorktreeInventory(inventory *types.WorktreeInventory) bool {
	wt.workspaceSubMu.RLock()
	defer wt.workspaceSubMu.RUnlock()

	msg := types.NewWorkspaceWorktreeInventory(inventory)
	delivered := true
	for sub := range wt.workspaceStreamSubscribers {
		select {
		case sub <- msg:
		default:
			delivered = false
		}
	}
	return delivered
}
