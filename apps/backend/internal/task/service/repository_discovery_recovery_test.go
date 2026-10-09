package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestDiscoveryRecoveryMixedRootsReplaceOnlySuccessfulRoot(t *testing.T) {
	rootA := filepath.Join(t.TempDir(), "root-a")
	rootB := filepath.Join(t.TempDir(), "root-b")
	svc := newDiscoveryService(t, rootA)
	svc.discoveryConfig.Roots = []string{rootA, rootB}

	oldA := LocalRepository{Path: filepath.Join(rootA, "old-a"), Name: "old-a"}
	oldB := LocalRepository{Path: filepath.Join(rootB, "old-b"), Name: "old-b"}
	newA := LocalRepository{Path: filepath.Join(rootA, "new-a"), Name: "new-a"}
	newB := LocalRepository{Path: filepath.Join(rootB, "new-b"), Name: "new-b"}
	scanNumber := 0
	svc.discoveryScanRoot = func(_ context.Context, root string, _ int) (repositoryDiscoveryScanResult, error) {
		switch scanNumber {
		case 0:
			if root == rootA {
				return repositoryDiscoveryScanResult{repositories: []LocalRepository{oldA}}, nil
			}
			return repositoryDiscoveryScanResult{repositories: []LocalRepository{oldB}}, nil
		case 1:
			if root == rootA {
				return repositoryDiscoveryScanResult{repositories: []LocalRepository{newA}}, nil
			}
			return repositoryDiscoveryScanResult{}, errors.Join(
				os.ErrNotExist,
				errors.New("clone root is missing"),
			)
		default:
			if root == rootA {
				return repositoryDiscoveryScanResult{}, nil
			}
			return repositoryDiscoveryScanResult{repositories: []LocalRepository{newB}}, nil
		}
	}

	first, err := svc.RefreshLocalRepositoryDiscovery(context.Background(), "")
	if err != nil {
		t.Fatalf("initial discovery refresh: %v", err)
	}
	assertRepositoryPaths(t, first.Repositories, oldA.Path, oldB.Path)
	scanTime := first.ScanTime

	scanNumber++
	second, err := svc.RefreshLocalRepositoryDiscovery(context.Background(), "")
	if err != nil {
		t.Fatalf("partial discovery refresh: %v", err)
	}
	assertRepositoryPaths(t, second.Repositories, newA.Path, oldB.Path)
	if !second.Cached {
		t.Fatal("partial failure should retain the warm cache marker")
	}
	if second.ScanTime == nil || scanTime == nil || !second.ScanTime.Equal(*scanTime) {
		t.Fatalf("scan time = %v, want previous scan time %v", second.ScanTime, scanTime)
	}
	if len(second.FailedRoots) != 1 || second.FailedRoots[0] != rootB {
		t.Fatalf("partial failed roots = %v, want [%q]", second.FailedRoots, rootB)
	}

	scanNumber++
	third, err := svc.RefreshLocalRepositoryDiscovery(context.Background(), "")
	if err != nil {
		t.Fatalf("recovered discovery refresh: %v", err)
	}
	assertRepositoryPaths(t, third.Repositories, newB.Path)
	if len(third.FailedRoots) != 0 {
		t.Fatalf("recovered failed roots = %v, want none", third.FailedRoots)
	}
}

// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.10, AC-WORKSPACES-LOCAL-REPOSITORIES-003.11
func TestDiscoveryRecoveryRootSetChangesRetainUnchangedRootSnapshots(t *testing.T) {
	t.Run("add root", func(t *testing.T) {
		rootA := t.TempDir()
		rootB := t.TempDir()
		svc := newDiscoveryService(t, rootA)
		svc.discoveryConfig = RepositoryDiscoveryConfig{DesktopRuntime: true, MaxDepth: 6}
		oldA := LocalRepository{Path: filepath.Join(rootA, "old-a"), Name: "old-a"}
		newB := LocalRepository{Path: filepath.Join(rootB, "new-b"), Name: "new-b"}
		failA := false
		svc.discoveryScanRoot = func(_ context.Context, root string, _ int) (repositoryDiscoveryScanResult, error) {
			if root == rootA {
				if failA {
					return repositoryDiscoveryScanResult{}, os.ErrPermission
				}
				return repositoryDiscoveryScanResult{repositories: []LocalRepository{oldA}}, nil
			}
			return repositoryDiscoveryScanResult{repositories: []LocalRepository{newB}}, nil
		}

		if _, err := svc.AddDesktopDiscoveryRoot(context.Background(), rootA); err != nil {
			t.Fatalf("add initial root: %v", err)
		}
		failA = true
		if _, err := svc.AddDesktopDiscoveryRoot(context.Background(), rootB); err != nil {
			t.Fatalf("add second root: %v", err)
		}

		snapshot, err := svc.GetLocalRepositoryDiscovery(context.Background(), "")
		if err != nil {
			t.Fatalf("get discovery snapshot: %v", err)
		}
		assertRepositoryPaths(t, snapshot.Repositories, oldA.Path, newB.Path)
		if len(snapshot.FailedRoots) != 1 || snapshot.FailedRoots[0] != rootA {
			t.Fatalf("failed roots = %v, want [%q]", snapshot.FailedRoots, rootA)
		}
	})

	t.Run("reconnect root", func(t *testing.T) {
		rootA := t.TempDir()
		rootB := t.TempDir()
		newA := t.TempDir()
		svc := newDiscoveryService(t, rootA)
		svc.discoveryConfig = RepositoryDiscoveryConfig{DesktopRuntime: true, MaxDepth: 6}
		oldA := LocalRepository{Path: filepath.Join(rootA, "old-a"), Name: "old-a"}
		oldB := LocalRepository{Path: filepath.Join(rootB, "old-b"), Name: "old-b"}
		newARepo := LocalRepository{Path: filepath.Join(newA, "new-a"), Name: "new-a"}
		failB := false
		svc.discoveryScanRoot = func(_ context.Context, root string, _ int) (repositoryDiscoveryScanResult, error) {
			if root == rootB {
				if failB {
					return repositoryDiscoveryScanResult{}, os.ErrPermission
				}
				return repositoryDiscoveryScanResult{repositories: []LocalRepository{oldB}}, nil
			}
			if root == newA {
				return repositoryDiscoveryScanResult{repositories: []LocalRepository{newARepo}}, nil
			}
			return repositoryDiscoveryScanResult{repositories: []LocalRepository{oldA}}, nil
		}

		if _, err := svc.AddDesktopDiscoveryRoot(context.Background(), rootA); err != nil {
			t.Fatalf("add initial root: %v", err)
		}
		if _, err := svc.AddDesktopDiscoveryRoot(context.Background(), rootB); err != nil {
			t.Fatalf("add second root: %v", err)
		}
		failB = true
		if _, err := svc.ReconnectDesktopDiscoveryRoot(context.Background(), rootA, newA); err != nil {
			t.Fatalf("reconnect root: %v", err)
		}

		snapshot, err := svc.GetLocalRepositoryDiscovery(context.Background(), "")
		if err != nil {
			t.Fatalf("get discovery snapshot: %v", err)
		}
		assertRepositoryPaths(t, snapshot.Repositories, oldB.Path, newARepo.Path)
		if len(snapshot.FailedRoots) != 1 || snapshot.FailedRoots[0] != rootB {
			t.Fatalf("failed roots = %v, want [%q]", snapshot.FailedRoots, rootB)
		}
	})
}

func TestDiscoveryRecoveryAllFailedRootsRetainWarmSnapshot(t *testing.T) {
	rootA := filepath.Join(t.TempDir(), "root-a")
	rootB := filepath.Join(t.TempDir(), "root-b")
	svc := newDiscoveryService(t, rootA)
	svc.discoveryConfig.Roots = []string{rootA, rootB}
	oldA := LocalRepository{Path: filepath.Join(rootA, "old-a"), Name: "old-a"}
	oldB := LocalRepository{Path: filepath.Join(rootB, "old-b"), Name: "old-b"}
	fail := false
	svc.discoveryScanRoot = func(_ context.Context, root string, _ int) (repositoryDiscoveryScanResult, error) {
		if fail {
			return repositoryDiscoveryScanResult{}, os.ErrPermission
		}
		if root == rootA {
			return repositoryDiscoveryScanResult{repositories: []LocalRepository{oldA}}, nil
		}
		return repositoryDiscoveryScanResult{repositories: []LocalRepository{oldB}}, nil
	}

	first, err := svc.RefreshLocalRepositoryDiscovery(context.Background(), "")
	if err != nil {
		t.Fatalf("initial discovery refresh: %v", err)
	}
	scanTime := first.ScanTime
	fail = true
	second, err := svc.RefreshLocalRepositoryDiscovery(context.Background(), "")
	if err != nil {
		t.Fatalf("all-root failure refresh: %v", err)
	}
	assertRepositoryPaths(t, second.Repositories, oldA.Path, oldB.Path)
	if !second.Cached {
		t.Fatal("all-root failure should retain the warm cache marker")
	}
	if len(second.FailedRoots) != 2 || second.FailedRoots[0] != rootA || second.FailedRoots[1] != rootB {
		t.Fatalf("failed roots = %v, want [%q %q]", second.FailedRoots, rootA, rootB)
	}
	if second.ScanTime == nil || scanTime == nil || !second.ScanTime.Equal(*scanTime) {
		t.Fatalf("scan time = %v, want previous scan time %v", second.ScanTime, scanTime)
	}
}

func TestDiscoveryRecoveryOverlappingRootsDeduplicateByPath(t *testing.T) {
	rootA := filepath.Join(t.TempDir(), "root-a")
	rootB := filepath.Join(t.TempDir(), "root-b")
	svc := newDiscoveryService(t, rootA)
	svc.discoveryConfig.Roots = []string{rootA, rootB}
	sharedPath := filepath.Join(t.TempDir(), "shared")
	svc.discoveryScanRoot = func(_ context.Context, _ string, _ int) (repositoryDiscoveryScanResult, error) {
		return repositoryDiscoveryScanResult{
			repositories: []LocalRepository{{Path: sharedPath, Name: "shared"}},
		}, nil
	}

	result, err := svc.RefreshLocalRepositoryDiscovery(context.Background(), "")
	if err != nil {
		t.Fatalf("overlapping discovery refresh: %v", err)
	}
	assertRepositoryPaths(t, result.Repositories, sharedPath)
}

func TestDiscoveryRecoveryReturnedRepositoriesDoNotMutateCache(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "project")
	svc := newDiscoveryService(t, root)
	svc.discoveryConfig.Roots = []string{root}
	svc.discoveryScanRoot = func(_ context.Context, _ string, _ int) (repositoryDiscoveryScanResult, error) {
		return repositoryDiscoveryScanResult{
			repositories: []LocalRepository{{Path: repositoryPath, Name: "project"}},
		}, nil
	}

	result, err := svc.RefreshLocalRepositoryDiscovery(context.Background(), "")
	if err != nil {
		t.Fatalf("discovery refresh: %v", err)
	}
	result.Repositories[0].Name = "mutated"

	snapshot, err := svc.GetLocalRepositoryDiscovery(context.Background(), "")
	if err != nil {
		t.Fatalf("get discovery snapshot: %v", err)
	}
	if len(snapshot.Repositories) != 1 || snapshot.Repositories[0].Name != "project" {
		t.Fatalf("cached repositories = %+v, want original repository", snapshot.Repositories)
	}
}

func TestScanRootForReposRootAccessFailureRemainsFatal(t *testing.T) {
	missingRoot := filepath.Join(t.TempDir(), "missing")
	_, err := scanRootForRepos(context.Background(), missingRoot, 6)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing root error = %v, want not-exist", err)
	}
}

// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.9, AC-WORKSPACES-LOCAL-REPOSITORIES-004.2
func TestDiscoveryRecoveryDescendantPermissionKeepsSiblingsAndLogsTarget(t *testing.T) {
	root := t.TempDir()
	before := filepath.Join(root, "before")
	private := filepath.Join(root, "Pictures", "Photo Booth Library")
	after := filepath.Join(root, "after")
	core, logs := observer.New(zapcore.WarnLevel)
	observedLogger, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}

	svc := newDiscoveryService(t, root)
	svc.logger = observedLogger
	svc.discoveryScanRoot = func(ctx context.Context, scanRoot string, maxDepth int) (repositoryDiscoveryScanResult, error) {
		return scanRootForReposWithWalk(
			ctx,
			scanRoot,
			maxDepth,
			permissionDiscoveryWalk(before, private, after),
		)
	}

	result, err := svc.refreshLocalRepositoryDiscovery(
		context.Background(), "", "", discoveryTriggerStaleRefresh,
	)
	if err != nil {
		t.Fatalf("refresh discovery: %v", err)
	}
	assertRepositoryPaths(t, result.Repositories, after, before)
	if len(result.FailedRoots) != 0 {
		t.Fatalf("failed roots = %v, want none", result.FailedRoots)
	}

	entries := logs.FilterMessage("filesystem.access_denied").All()
	if len(entries) != 1 {
		t.Fatalf("access-denied warnings = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["target"] != private {
		t.Fatalf("warning target = %v, want %q", fields["target"], private)
	}
	if fields["operation"] != "repository.discovery.scan" {
		t.Fatalf("warning operation = %v", fields["operation"])
	}
	if fields["trigger"] != discoveryTriggerStaleRefresh {
		t.Fatalf("warning trigger = %v", fields["trigger"])
	}
	if fields["runtime"] != "server" {
		t.Fatalf("warning runtime = %v", fields["runtime"])
	}
}

// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.9
func TestDiscoveryRecoveryDescendantPermissionWarnsOncePerPath(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, "private")
	result, err := scanRootForReposWithWalk(
		context.Background(),
		root,
		6,
		func(root string, visit fs.WalkDirFunc) error {
			for _, item := range []struct {
				path string
				err  error
			}{
				{path: root},
				{path: private, err: errors.Join(os.ErrPermission, errors.New("first denial"))},
				{path: private, err: errors.Join(os.ErrPermission, errors.New("second denial"))},
			} {
				if err := visit(item.path, nil, item.err); err != nil {
					return err
				}
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("scan root: %v", err)
	}
	if len(result.warnings) != 1 || result.warnings[0].path != private {
		t.Fatalf("scan warnings = %+v, want one warning for %q", result.warnings, private)
	}
}

// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.7
func TestDiscoveryRecoveryCancellationDoesNotPublishPartialSnapshot(t *testing.T) {
	rootA := filepath.Join(t.TempDir(), "root-a")
	rootB := filepath.Join(t.TempDir(), "root-b")
	svc := newDiscoveryService(t, rootA)
	svc.discoveryConfig.Roots = []string{rootA, rootB}
	partial := LocalRepository{Path: filepath.Join(rootA, "partial"), Name: "partial"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.discoveryScanRoot = func(ctx context.Context, root string, _ int) (repositoryDiscoveryScanResult, error) {
		if root == rootA {
			return repositoryDiscoveryScanResult{repositories: []LocalRepository{partial}}, nil
		}
		cancel()
		return repositoryDiscoveryScanResult{}, ctx.Err()
	}

	if _, err := svc.RefreshLocalRepositoryDiscovery(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled refresh error = %v, want context canceled", err)
	}

	snapshot, err := svc.GetLocalRepositoryDiscovery(context.Background(), "")
	if err != nil {
		t.Fatalf("get cancelled snapshot: %v", err)
	}
	if snapshot.Cached || len(snapshot.Repositories) != 0 || len(snapshot.FailedRoots) != 0 {
		t.Fatalf("cancelled snapshot = %+v, want no published data", snapshot)
	}
}

type fakeDiscoveryDirEntry struct {
	name string
	dir  bool
}

func (entry fakeDiscoveryDirEntry) Name() string      { return entry.name }
func (entry fakeDiscoveryDirEntry) IsDir() bool       { return entry.dir }
func (entry fakeDiscoveryDirEntry) Type() fs.FileMode { return modeForDiscoveryDirEntry(entry.dir) }
func (entry fakeDiscoveryDirEntry) Info() (fs.FileInfo, error) {
	return nil, errors.New("not available")
}

func modeForDiscoveryDirEntry(isDir bool) fs.FileMode {
	if isDir {
		return fs.ModeDir
	}
	return 0
}

func permissionDiscoveryWalk(before, private, after string) repositoryDiscoveryWalkDir {
	return func(root string, visit fs.WalkDirFunc) error {
		items := []struct {
			path  string
			entry fs.DirEntry
			err   error
		}{
			{path: root, entry: fakeDiscoveryDirEntry{name: filepath.Base(root), dir: true}},
			{path: before, entry: fakeDiscoveryDirEntry{name: filepath.Base(before), dir: true}},
			{path: filepath.Join(before, ".git"), entry: fakeDiscoveryDirEntry{name: ".git"}},
			{path: private, err: errors.Join(os.ErrPermission, errors.New("photo library denied"))},
			{path: after, entry: fakeDiscoveryDirEntry{name: filepath.Base(after), dir: true}},
			{path: filepath.Join(after, ".git"), entry: fakeDiscoveryDirEntry{name: ".git"}},
		}
		for _, item := range items {
			if err := visit(item.path, item.entry, item.err); err != nil {
				return err
			}
		}
		return nil
	}
}

func assertRepositoryPaths(t *testing.T, repositories []LocalRepository, expected ...string) {
	t.Helper()
	if len(repositories) != len(expected) {
		t.Fatalf("repository paths = %#v, want %#v", repositoryPaths(repositories), expected)
	}
	for index, path := range expected {
		if repositories[index].Path != path {
			t.Fatalf("repository paths = %#v, want %#v", repositoryPaths(repositories), expected)
		}
	}
}

func repositoryPaths(repositories []LocalRepository) []string {
	paths := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		paths = append(paths, repository.Path)
	}
	return paths
}
