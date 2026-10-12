package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"syscall"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func entryMutationTracker(t *testing.T) (string, *WorkspaceTracker) {
	t.Helper()
	dir, tracker := setupTestDir(t)
	t.Cleanup(tracker.Stop)
	return dir, tracker
}

func entryMutationWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func entryMutationLink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
			t.Skipf("native symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
}

func entryMutationAssertLink(t *testing.T, path, target string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("entry %q must remain a symlink: info=%v err=%v", path, info, err)
		return
	}
	got, err := os.Readlink(path)
	if err != nil || got != target {
		t.Errorf("link %q = %q, %v; want stored value %q", path, got, err, target)
	}
}

func entryMutationAssertBytes(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Errorf("preserved file %q = %q, %v; want %q", path, got, err, want)
	}
}

func entryMutationAssertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("entry %q must be absent: %v", path, err)
	}
}

func entryMutationSubscribe(t *testing.T, tracker *WorkspaceTracker) types.WorkspaceStreamSubscriber {
	t.Helper()
	sub := tracker.SubscribeWorkspaceStream()
	t.Cleanup(func() { tracker.UnsubscribeWorkspaceStream(sub) })
	return sub
}

func entryMutationAssertEvents(t *testing.T, sub types.WorkspaceStreamSubscriber, operation string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		select {
		case message := <-sub:
			event := message.FileChange
			if event == nil || event.Path != path || event.Operation != operation || event.RepositoryName != "" {
				t.Errorf("event = %+v; want %s for %q from root tracker", event, operation, path)
			}
		default:
			t.Errorf("missing immediate %s event for %q", operation, path)
		}
	}
	select {
	case message := <-sub:
		t.Errorf("unexpected notification: %+v", message)
	default:
	}
}

func entryMutationSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			value, err := os.Readlink(path)
			if err != nil {
				return err
			}
			snapshot[rel] = "link:" + value
		case entry.IsDir():
			snapshot[rel] = "directory"
		default:
			value, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			snapshot[rel] = "file:" + string(value)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func entryMutationAssertSnapshot(t *testing.T, root string, before map[string]string) {
	t.Helper()
	if after := entryMutationSnapshot(t, root); !reflect.DeepEqual(before, after) {
		t.Errorf("filesystem changed under %q: before=%v after=%v", root, before, after)
	}
}

type entryMutationCase struct {
	name      string
	directory bool
	absolute  bool
	operation string
	dest      string
}

// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.1
// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.2
// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.6
func TestWorkspaceFileEntryMutations_LeafIdentity(t *testing.T) {
	for _, tc := range []entryMutationCase{
		{name: "delete-file-link", operation: types.FileOpRemove},
		{name: "rename-file-link", operation: types.FileOpRename, dest: "renamed"},
		{name: "delete-directory-link", directory: true, operation: types.FileOpRemove},
		{name: "rename-directory-link", directory: true, operation: types.FileOpRename, dest: "renamed"},
		{name: "move-file-link", operation: types.FileOpRename, dest: filepath.Join("new", "deep", "moved")},
		{name: "move-directory-link", directory: true, operation: types.FileOpRename, dest: filepath.Join("new", "deep", "moved")},
	} {
		t.Run(tc.name, func(t *testing.T) { runEntryMutationLeaf(t, tc) })
	}
}

// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.2
// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.6
func TestWorkspaceFileEntryMutations_AbsoluteLeafIdentity(t *testing.T) {
	for _, tc := range []entryMutationCase{
		{name: "rename-absolute-file-link", absolute: true, operation: types.FileOpRename, dest: "renamed"},
		{name: "rename-absolute-directory-link", directory: true, absolute: true, operation: types.FileOpRename, dest: "renamed"},
		{name: "move-absolute-file-link", absolute: true, operation: types.FileOpRename, dest: filepath.Join("new", "deep", "moved")},
		{name: "move-absolute-directory-link", directory: true, absolute: true, operation: types.FileOpRename, dest: filepath.Join("new", "deep", "moved")},
	} {
		t.Run(tc.name, func(t *testing.T) { runEntryMutationLeaf(t, tc) })
	}
}

func runEntryMutationLeaf(t *testing.T, tc entryMutationCase) {
	t.Helper()
	dir, tracker := entryMutationTracker(t)
	target := "target.txt"
	if tc.directory {
		target = "target-directory"
	}
	bytesPath := filepath.Join(dir, target)
	if tc.directory {
		bytesPath = filepath.Join(bytesPath, "nested", "precious.txt")
	}
	entryMutationWrite(t, bytesPath, "selected target bytes\n")
	entryMutationWrite(t, filepath.Join(dir, "neighbor.txt"), "unselected neighbor bytes\n")
	linkValue := target
	if tc.absolute {
		linkValue = filepath.Join(dir, target)
	}
	entryMutationLink(t, linkValue, filepath.Join(dir, "alias"))
	entryMutationLink(t, linkValue, filepath.Join(dir, "unselected-alias"))
	targetInfo, err := os.Lstat(filepath.Join(dir, target))
	if err != nil {
		t.Fatal(err)
	}
	tree, err := tracker.GetFileTree("", 1)
	if err != nil {
		t.Fatal(err)
	}
	alias := requireChild(t, tree, "alias")
	if alias.Path != "alias" || !alias.IsSymlink || alias.IsDir != tc.directory {
		t.Fatalf("actual tree alias = %+v", alias)
	}
	sub := entryMutationSubscribe(t, tracker)
	if tc.operation == types.FileOpRemove {
		err = tracker.DeleteFile(alias.Path)
	} else {
		err = tracker.RenameFile(alias.Path, tc.dest)
	}
	if err != nil {
		t.Errorf("selected %s failed: %v", tc.name, err)
	}
	entryMutationAssertBytes(t, bytesPath, "selected target bytes\n")
	currentTarget, statErr := os.Lstat(filepath.Join(dir, target))
	if statErr != nil || !os.SameFile(targetInfo, currentTarget) {
		t.Errorf("target entry identity changed: %v", statErr)
	}
	entryMutationAssertBytes(t, filepath.Join(dir, "neighbor.txt"), "unselected neighbor bytes\n")
	entryMutationAssertLink(t, filepath.Join(dir, "unselected-alias"), linkValue)
	entryMutationAssertAbsent(t, filepath.Join(dir, "alias"))
	paths := []string{"alias"}
	if tc.dest != "" {
		entryMutationAssertLink(t, filepath.Join(dir, tc.dest), linkValue)
		paths = append(paths, tc.dest)
	}
	entryMutationAssertEvents(t, sub, tc.operation, paths...)
}

// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.3
func TestWorkspaceFileEntryMutations_DestinationOccupied(t *testing.T) {
	for _, kind := range []string{"file", "directory", "readable-link", "same-target-link", "dangling-link", "loop-link"} {
		t.Run(kind, func(t *testing.T) {
			dir, tracker := entryMutationTracker(t)
			entryMutationWrite(t, filepath.Join(dir, "target.txt"), "source target\n")
			entryMutationWrite(t, filepath.Join(dir, "other.txt"), "destination target\n")
			entryMutationLink(t, "target.txt", filepath.Join(dir, "alias"))
			prepareEntryMutationDestination(t, dir, kind)
			before := entryMutationSnapshot(t, dir)
			sub := entryMutationSubscribe(t, tracker)
			if err := tracker.RenameFile("alias", "occupied"); err == nil {
				t.Error("rename accepted a distinct occupied destination")
			}
			entryMutationAssertSnapshot(t, dir, before)
			entryMutationAssertEvents(t, sub, types.FileOpRename)
		})
	}
}

func prepareEntryMutationDestination(t *testing.T, dir, kind string) {
	t.Helper()
	dest := filepath.Join(dir, "occupied")
	switch kind {
	case "file":
		entryMutationWrite(t, dest, "occupied file\n")
	case "directory":
		entryMutationWrite(t, filepath.Join(dest, "child.txt"), "occupied directory bytes\n")
	case "readable-link":
		entryMutationLink(t, "other.txt", dest)
	case "same-target-link":
		entryMutationLink(t, "target.txt", dest)
	case "dangling-link":
		entryMutationLink(t, "absent.txt", dest)
	case "loop-link":
		entryMutationLink(t, "occupied", dest)
	default:
		t.Fatalf("unknown destination fixture %q", kind)
	}
}

// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.4
func TestWorkspaceFileEntryMutations_Compatibility(t *testing.T) {
	for _, directory := range []bool{false, true} {
		kind := "-file"
		if directory {
			kind = "-directory"
		}
		for _, operation := range []string{types.FileOpRemove, types.FileOpRename} {
			t.Run(operation+kind, func(t *testing.T) {
				runEntryMutationOrdinary(t, directory, operation)
			})
		}
	}
	t.Run("contained-parent", runEntryMutationContainedParent)
	t.Run("read-edit-target", runEntryMutationReadEdit)
	t.Run("same-entry", runEntryMutationNoOp)
}

func runEntryMutationOrdinary(t *testing.T, directory bool, operation string) {
	t.Helper()
	dir, tracker := entryMutationTracker(t)
	bytesPath := filepath.Join(dir, "ordinary")
	if directory {
		bytesPath = filepath.Join(bytesPath, "nested", "file.txt")
	}
	entryMutationWrite(t, bytesPath, "ordinary selected\n")
	entryMutationWrite(t, filepath.Join(dir, "neighbor.txt"), "ordinary unselected\n")
	sub := entryMutationSubscribe(t, tracker)
	var err error
	if operation == types.FileOpRemove {
		err = tracker.DeleteFile(filepath.Join(dir, "ordinary"))
	} else {
		err = tracker.RenameFile("ordinary", filepath.Join("created", "deep", "ordinary"))
	}
	if err != nil {
		t.Fatal(err)
	}
	entryMutationAssertAbsent(t, filepath.Join(dir, "ordinary"))
	paths := []string{"ordinary"}
	if operation == types.FileOpRename {
		newPath := filepath.Join("created", "deep", "ordinary")
		paths = append(paths, newPath)
		if directory {
			newPath = filepath.Join(newPath, "nested", "file.txt")
		}
		entryMutationAssertBytes(t, filepath.Join(dir, newPath), "ordinary selected\n")
	}
	entryMutationAssertBytes(t, filepath.Join(dir, "neighbor.txt"), "ordinary unselected\n")
	entryMutationAssertEvents(t, sub, operation, paths...)
}

func runEntryMutationContainedParent(t *testing.T) {
	t.Helper()
	dir, tracker := entryMutationTracker(t)
	entryMutationWrite(t, filepath.Join(dir, "real", "ordinary.txt"), "parent selected\n")
	entryMutationWrite(t, filepath.Join(dir, "real", "neighbor.txt"), "parent neighbor\n")
	entryMutationLink(t, "real", filepath.Join(dir, "parent"))
	sub := entryMutationSubscribe(t, tracker)
	if err := tracker.RenameFile(filepath.Join("parent", "ordinary.txt"), filepath.Join("parent", "new", "renamed.txt")); err != nil {
		t.Fatal(err)
	}
	entryMutationAssertBytes(t, filepath.Join(dir, "real", "new", "renamed.txt"), "parent selected\n")
	entryMutationAssertAbsent(t, filepath.Join(dir, "real", "ordinary.txt"))
	entryMutationAssertEvents(t, sub, types.FileOpRename, filepath.Join("real", "ordinary.txt"), filepath.Join("real", "new", "renamed.txt"))
	if err := tracker.DeleteFile(filepath.Join("parent", "new", "renamed.txt")); err != nil {
		t.Fatal(err)
	}
	entryMutationAssertAbsent(t, filepath.Join(dir, "real", "new", "renamed.txt"))
	entryMutationAssertLink(t, filepath.Join(dir, "parent"), "real")
	entryMutationAssertBytes(t, filepath.Join(dir, "real", "neighbor.txt"), "parent neighbor\n")
	entryMutationAssertEvents(t, sub, types.FileOpRemove, filepath.Join("real", "new", "renamed.txt"))
}

func runEntryMutationReadEdit(t *testing.T) {
	t.Helper()
	dir, tracker := entryMutationTracker(t)
	entryMutationWrite(t, filepath.Join(dir, "target.txt"), "readable target\n")
	entryMutationLink(t, "target.txt", filepath.Join(dir, "alias"))
	content, _, _, resolved, err := tracker.GetFileContent("alias")
	if err != nil || content != "readable target\n" || resolved != "target.txt" {
		t.Fatalf("read through link = %q, %q, %v", content, resolved, err)
	}
	desired := "edited target\n"
	_, resolution, err := tracker.ApplyFileDiff(context.Background(), "alias", "alias", "", "stale hash", &desired)
	if err != nil || resolution != ResolutionOverwritten {
		t.Fatalf("edit through link: resolution=%s error=%v", resolution, err)
	}
	entryMutationAssertBytes(t, filepath.Join(dir, "target.txt"), desired)
	entryMutationAssertLink(t, filepath.Join(dir, "alias"), "target.txt")
}

func runEntryMutationNoOp(t *testing.T) {
	t.Helper()
	dir, tracker := entryMutationTracker(t)
	entryMutationWrite(t, filepath.Join(dir, "target.txt"), "no-op target\n")
	entryMutationLink(t, "target.txt", filepath.Join(dir, "alias"))
	before := entryMutationSnapshot(t, dir)
	sub := entryMutationSubscribe(t, tracker)
	if err := tracker.RenameFile("alias", filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}
	entryMutationAssertSnapshot(t, dir, before)
	entryMutationAssertEvents(t, sub, types.FileOpRename)
}

// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.5
func TestWorkspaceFileEntryMutations_Authority(t *testing.T) {
	t.Run("registered-parent", runEntryMutationRegisteredParent)
	for _, source := range []string{".", "root-alias", "registered", "external-link", "other-root-link", "dangling", "loop", "unregistered/precious.txt", "../escape.txt"} {
		for _, operation := range []string{types.FileOpRemove, types.FileOpRename} {
			t.Run(operation+"/"+source, func(t *testing.T) { runEntryMutationAuthorityRejection(t, source, operation) })
		}
	}
	t.Run("registered-root-no-op", func(t *testing.T) { runEntryMutationAuthorityRejection(t, "registered", "no-op") })
	t.Run("workspace-root-no-op", func(t *testing.T) { runEntryMutationAuthorityRejection(t, "root-alias", "no-op") })
	t.Run("empty-allowlist", func(t *testing.T) { runEntryMutationSourceAllowlist(t, false) })
	t.Run("stale-allowlist", func(t *testing.T) { runEntryMutationSourceAllowlist(t, true) })
	t.Run("cross-root-link-move", runEntryMutationCrossRoot)
}

func runEntryMutationRegisteredParent(t *testing.T) {
	t.Helper()
	dir, tracker := entryMutationTracker(t)
	source, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entryMutationWrite(t, filepath.Join(source, "target.txt"), "registered target\n")
	entryMutationWrite(t, filepath.Join(source, "ordinary.txt"), "registered ordinary\n")
	entryMutationLink(t, "target.txt", filepath.Join(source, "alias"))
	entryMutationLink(t, source, filepath.Join(dir, "registered"))
	tracker.SetAllowedSourceRoots([]string{source})
	sub := entryMutationSubscribe(t, tracker)
	for _, name := range []string{"ordinary.txt", "alias"} {
		if err := tracker.RenameFile(filepath.Join("registered", name), filepath.Join("registered", "renamed-"+name)); err != nil {
			t.Fatal(err)
		}
		entryMutationAssertAbsent(t, filepath.Join(source, name))
		entryMutationAssertEvents(t, sub, types.FileOpRename, filepath.Join(source, name), filepath.Join(source, "renamed-"+name))
		if name == "alias" {
			entryMutationAssertLink(t, filepath.Join(source, "renamed-"+name), "target.txt")
		} else {
			entryMutationAssertBytes(t, filepath.Join(source, "renamed-"+name), "registered ordinary\n")
		}
		if err := tracker.DeleteFile(filepath.Join("registered", "renamed-"+name)); err != nil {
			t.Fatal(err)
		}
		entryMutationAssertAbsent(t, filepath.Join(source, "renamed-"+name))
		entryMutationAssertEvents(t, sub, types.FileOpRemove, filepath.Join(source, "renamed-"+name))
	}
	entryMutationAssertBytes(t, filepath.Join(source, "target.txt"), "registered target\n")
	entryMutationAssertLink(t, filepath.Join(dir, "registered"), source)
}

func runEntryMutationAuthorityRejection(t *testing.T, sourcePath, operation string) {
	t.Helper()
	dir, tracker := entryMutationTracker(t)
	registered, external := t.TempDir(), t.TempDir()
	entryMutationWrite(t, filepath.Join(dir, "neighbor.txt"), "workspace authority\n")
	entryMutationWrite(t, filepath.Join(registered, "precious.txt"), "registered authority\n")
	entryMutationWrite(t, filepath.Join(external, "precious.txt"), "external authority\n")
	entryMutationLink(t, ".", filepath.Join(dir, "root-alias"))
	entryMutationLink(t, registered, filepath.Join(dir, "registered"))
	entryMutationLink(t, external, filepath.Join(dir, "unregistered"))
	entryMutationLink(t, filepath.Join(external, "precious.txt"), filepath.Join(dir, "external-link"))
	entryMutationLink(t, filepath.Join(registered, "precious.txt"), filepath.Join(dir, "other-root-link"))
	entryMutationLink(t, "absent.txt", filepath.Join(dir, "dangling"))
	entryMutationLink(t, "loop", filepath.Join(dir, "loop"))
	tracker.SetAllowedSourceRoots([]string{registered})
	before, sourceBefore, externalBefore := entryMutationSnapshot(t, dir), entryMutationSnapshot(t, registered), entryMutationSnapshot(t, external)
	sub := entryMutationSubscribe(t, tracker)
	if sourcePath == "../escape.txt" {
		var err error
		sourcePath, err = filepath.Rel(dir, filepath.Join(external, "precious.txt"))
		if err != nil {
			t.Fatal(err)
		}
	}
	sourcePath = filepath.FromSlash(sourcePath)
	var err error
	switch operation {
	case types.FileOpRemove:
		err = tracker.DeleteFile(sourcePath)
	case "no-op":
		err = tracker.RenameFile(sourcePath, sourcePath)
	default:
		err = tracker.RenameFile(sourcePath, "renamed")
	}
	if err == nil {
		t.Errorf("%s unexpectedly admitted %q", operation, sourcePath)
	}
	entryMutationAssertSnapshot(t, dir, before)
	entryMutationAssertSnapshot(t, registered, sourceBefore)
	entryMutationAssertSnapshot(t, external, externalBefore)
	entryMutationAssertEvents(t, sub, types.FileOpRename)
}

func runEntryMutationSourceAllowlist(t *testing.T, stale bool) {
	t.Helper()
	dir, tracker := entryMutationTracker(t)
	source, replacement := t.TempDir(), t.TempDir()
	entryMutationWrite(t, filepath.Join(source, "precious.txt"), "original source\n")
	entryMutationWrite(t, filepath.Join(replacement, "precious.txt"), "replacement source\n")
	entryMutationLink(t, source, filepath.Join(dir, "registered"))
	if stale {
		tracker.SetAllowedSourceRoots([]string{source})
		if err := os.Remove(filepath.Join(dir, "registered")); err != nil {
			t.Fatal(err)
		}
		entryMutationLink(t, replacement, filepath.Join(dir, "registered"))
	}
	before, sourceBefore, replacementBefore := entryMutationSnapshot(t, dir), entryMutationSnapshot(t, source), entryMutationSnapshot(t, replacement)
	sub := entryMutationSubscribe(t, tracker)
	path := filepath.Join("registered", "precious.txt")
	if err := tracker.DeleteFile(path); err == nil {
		t.Error("delete admitted absent/stale registered source authority")
	}
	if err := tracker.RenameFile(path, filepath.Join("registered", "renamed.txt")); err == nil {
		t.Error("rename admitted absent/stale registered source authority")
	}
	entryMutationAssertSnapshot(t, dir, before)
	entryMutationAssertSnapshot(t, source, sourceBefore)
	entryMutationAssertSnapshot(t, replacement, replacementBefore)
	entryMutationAssertEvents(t, sub, types.FileOpRename)
}

func runEntryMutationCrossRoot(t *testing.T) {
	t.Helper()
	dir, tracker := entryMutationTracker(t)
	source := t.TempDir()
	entryMutationWrite(t, filepath.Join(source, "target.txt"), "cross-root target\n")
	entryMutationLink(t, "target.txt", filepath.Join(source, "alias"))
	entryMutationLink(t, source, filepath.Join(dir, "registered"))
	tracker.SetAllowedSourceRoots([]string{source})
	before, sourceBefore := entryMutationSnapshot(t, dir), entryMutationSnapshot(t, source)
	sub := entryMutationSubscribe(t, tracker)
	if err := tracker.RenameFile(filepath.Join("registered", "alias"), "moved"); err == nil {
		t.Error("cross-root leaf move unexpectedly succeeded")
	}
	entryMutationAssertSnapshot(t, dir, before)
	entryMutationAssertSnapshot(t, source, sourceBefore)
	entryMutationAssertEvents(t, sub, types.FileOpRename)
}

// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.5
func TestWorkspaceFileEntryMutations_ParentSwap(t *testing.T) {
	for _, operation := range []string{types.FileOpRemove, "rename-source", "rename-destination"} {
		t.Run(operation, func(t *testing.T) { runEntryMutationParentSwap(t, operation) })
	}
}

func runEntryMutationParentSwap(t *testing.T, operation string) {
	t.Helper()
	dir, tracker := entryMutationTracker(t)
	external := t.TempDir()
	entryMutationWrite(t, filepath.Join(dir, "switchable", "target.txt"), "original parent target\n")
	entryMutationLink(t, "target.txt", filepath.Join(dir, "switchable", "alias"))
	entryMutationWrite(t, filepath.Join(dir, "from.txt"), "rename source\n")
	entryMutationWrite(t, filepath.Join(external, "target.txt"), "external target\n")
	entryMutationLink(t, "target.txt", filepath.Join(external, "alias"))
	before := entryMutationSnapshot(t, external)
	sub := entryMutationSubscribe(t, tracker)
	called := false
	workspaceMutationBarrier.Store(func() {
		called = true
		if err := os.Rename(filepath.Join(dir, "switchable"), filepath.Join(dir, "retained")); err != nil {
			t.Fatal(err)
		}
		entryMutationLink(t, external, filepath.Join(dir, "switchable"))
	})
	t.Cleanup(func() { workspaceMutationBarrier.Store((func())(nil)) })
	var err error
	switch operation {
	case types.FileOpRemove:
		err = tracker.DeleteFile(filepath.Join("switchable", "alias"))
	case "rename-source":
		err = tracker.RenameFile(filepath.Join("switchable", "alias"), "moved")
	default:
		err = tracker.RenameFile("from.txt", filepath.Join("switchable", "moved"))
	}
	if !called || err == nil {
		t.Errorf("external parent swap: barrier=%t error=%v", called, err)
	}
	entryMutationAssertSnapshot(t, external, before)
	entryMutationAssertBytes(t, filepath.Join(dir, "retained", "target.txt"), "original parent target\n")
	entryMutationAssertLink(t, filepath.Join(dir, "retained", "alias"), "target.txt")
	entryMutationAssertBytes(t, filepath.Join(dir, "from.txt"), "rename source\n")
	entryMutationAssertAbsent(t, filepath.Join(dir, "moved"))
	entryMutationAssertEvents(t, sub, types.FileOpRename)
}
