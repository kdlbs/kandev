package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDesktopDiscoveryConfirmHomeAddsCanonicalHomeAndScansOnce(t *testing.T) {
	targetHome, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve target Home: %v", err)
	}
	homeAlias := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(targetHome, homeAlias); err != nil {
		t.Skipf("create Home symlink: %v", err)
	}
	t.Setenv("HOME", homeAlias)

	svc, _, repo := createTestService(t)
	svc.discoveryConfig = RepositoryDiscoveryConfig{DesktopRuntime: true, MaxDepth: 6}
	svc.desktopRootStore = repo
	if err := repo.SetDesktopDiscoveryMigration(context.Background(), &models.DesktopDiscoveryMigration{
		HomeConfirmationRequired: true,
	}); err != nil {
		t.Fatalf("set migration state: %v", err)
	}
	var scanCalls int
	svc.discoveryScanRoot = func(_ context.Context, root string, _ int) (repositoryDiscoveryScanResult, error) {
		scanCalls++
		if root != targetHome {
			t.Fatalf("scan root = %q, want canonical Home %q", root, targetHome)
		}
		return repositoryDiscoveryScanResult{}, nil
	}

	root, err := svc.ConfirmHomeDesktopDiscovery(context.Background())
	if err != nil {
		t.Fatalf("confirm Home: %v", err)
	}
	if root.Path != targetHome {
		t.Fatalf("confirmed root = %q, want canonical Home %q", root.Path, targetHome)
	}
	if scanCalls != 1 {
		t.Fatalf("scan calls = %d, want one", scanCalls)
	}

	root, err = svc.ConfirmHomeDesktopDiscovery(context.Background())
	if err != nil {
		t.Fatalf("retry Home confirmation: %v", err)
	}
	if root.Path != targetHome || scanCalls != 1 {
		t.Fatalf("retry root/scan calls = %q/%d, want %q/1", root.Path, scanCalls, targetHome)
	}
	roots, err := repo.ListDesktopDiscoveryRoots(context.Background())
	if err != nil {
		t.Fatalf("list roots: %v", err)
	}
	if len(roots) != 1 || roots[0].Path != targetHome {
		t.Fatalf("roots = %+v, want only canonical Home", roots)
	}
}

func TestDesktopDiscoveryConfirmHomeRecoversDisconnectedSavedHome(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve Home path: %v", err)
	}
	t.Setenv("HOME", home)
	svc, _, repo := createTestService(t)
	svc.discoveryConfig = RepositoryDiscoveryConfig{DesktopRuntime: true, MaxDepth: 6}
	svc.desktopRootStore = repo
	root := &models.DesktopDiscoveryRoot{
		ID:          "home-root",
		Path:        home,
		DisplayPath: "~",
		State:       models.DesktopDiscoveryRootReconnectRequired,
	}
	if err := repo.CreateDesktopDiscoveryRoot(context.Background(), root); err != nil {
		t.Fatalf("create disconnected Home root: %v", err)
	}
	if err := repo.SetDesktopDiscoveryMigration(context.Background(), &models.DesktopDiscoveryMigration{
		HomeConfirmationRequired: true,
	}); err != nil {
		t.Fatalf("set migration state: %v", err)
	}
	var scanCalls int
	svc.discoveryScanRoot = func(_ context.Context, path string, _ int) (repositoryDiscoveryScanResult, error) {
		scanCalls++
		if path != home {
			t.Fatalf("scan root = %q, want Home %q", path, home)
		}
		return repositoryDiscoveryScanResult{}, nil
	}

	confirmed, err := svc.ConfirmHomeDesktopDiscovery(context.Background())
	if err != nil {
		t.Fatalf("confirm disconnected Home: %v", err)
	}
	if confirmed.State != models.DesktopDiscoveryRootConnected || scanCalls != 1 {
		t.Fatalf("confirmed root/scan calls = %+v/%d, want connected root and one scan", confirmed, scanCalls)
	}
	migration, err := repo.GetDesktopDiscoveryMigration(context.Background())
	if err != nil {
		t.Fatalf("read migration state: %v", err)
	}
	if migration == nil || migration.HomeConfirmationRequired {
		t.Fatalf("migration = %+v, want confirmation cleared after recovery", migration)
	}
}

func TestDesktopDiscoveryConfirmHomeRejectsStaleAndNonDesktopRequests(t *testing.T) {
	t.Run("another root cleared the migration", func(t *testing.T) {
		home, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatalf("resolve Home: %v", err)
		}
		t.Setenv("HOME", home)
		svc, _, repo := createTestService(t)
		svc.discoveryConfig = RepositoryDiscoveryConfig{DesktopRuntime: true, MaxDepth: 6}
		svc.desktopRootStore = repo
		if err := repo.SetDesktopDiscoveryMigration(context.Background(), &models.DesktopDiscoveryMigration{
			HomeConfirmationRequired: true,
		}); err != nil {
			t.Fatalf("set migration state: %v", err)
		}
		otherRoot, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatalf("resolve other root: %v", err)
		}
		var scanCalls int
		svc.discoveryScanRoot = func(_ context.Context, root string, _ int) (repositoryDiscoveryScanResult, error) {
			scanCalls++
			if root != otherRoot {
				t.Fatalf("scan root = %q, want chosen root %q", root, otherRoot)
			}
			return repositoryDiscoveryScanResult{}, nil
		}
		if _, err := svc.AddDesktopDiscoveryRoot(context.Background(), otherRoot); err != nil {
			t.Fatalf("choose another root: %v", err)
		}

		if _, err := svc.ConfirmHomeDesktopDiscovery(context.Background()); !errors.Is(err, ErrHomeDiscoveryConfirmationStale) {
			t.Fatalf("stale confirmation error = %v, want %v", err, ErrHomeDiscoveryConfirmationStale)
		}
		roots, err := repo.ListDesktopDiscoveryRoots(context.Background())
		if err != nil {
			t.Fatalf("list roots: %v", err)
		}
		if len(roots) != 1 || roots[0].Path != otherRoot || scanCalls != 1 {
			t.Fatalf("stale request changed roots/scans: %+v/%d", roots, scanCalls)
		}
	})

	t.Run("server mode is unavailable", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		svc.desktopRootStore = repo
		if err := repo.SetDesktopDiscoveryMigration(context.Background(), &models.DesktopDiscoveryMigration{
			HomeConfirmationRequired: true,
		}); err != nil {
			t.Fatalf("set migration state: %v", err)
		}
		if _, err := svc.ConfirmHomeDesktopDiscovery(context.Background()); !errors.Is(err, ErrDesktopDiscoveryUnavailable) {
			t.Fatalf("server confirmation error = %v, want %v", err, ErrDesktopDiscoveryUnavailable)
		}
		roots, err := repo.ListDesktopDiscoveryRoots(context.Background())
		if err != nil {
			t.Fatalf("list roots: %v", err)
		}
		if len(roots) != 0 {
			t.Fatalf("server confirmation added roots: %+v", roots)
		}
	})
}
