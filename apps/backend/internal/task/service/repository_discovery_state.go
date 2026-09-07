package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/common/fsdiagnostics"
	"github.com/kandev/kandev/internal/task/models"
)

const discoveryRefreshAge = 30 * time.Minute

var (
	ErrDesktopDiscoveryUnavailable = errors.New("desktop discovery roots are unavailable")
	ErrInvalidDiscoveryRoot        = errors.New("invalid discovery root")
)

const (
	discoveryTriggerManualRefresh = "manual_refresh"
	discoveryTriggerStaleRefresh  = "stale_refresh"
	discoveryTriggerUserSelect    = "user_select"
)

// NormalizeRepositoryDiscoveryTrigger keeps scan diagnostics within the
// triggers understood by the discovery contract. Unknown or missing causes
// are treated as an explicit manual refresh.
func NormalizeRepositoryDiscoveryTrigger(trigger string) string {
	switch trigger {
	case discoveryTriggerManualRefresh, discoveryTriggerStaleRefresh, discoveryTriggerUserSelect:
		return trigger
	default:
		return discoveryTriggerManualRefresh
	}
}

type discoveryCacheEntry struct {
	roots        []string
	repositories []LocalRepository
	rootStates   []models.DesktopDiscoveryRoot
	scanTime     *time.Time
	failedRoots  []string
}

type discoveryFlight struct {
	done   chan struct{}
	result RepositoryDiscoveryResult
	err    error
}

// DiscoveryRefreshAge is the freshness period used by the browser
// coordinator. The backend exposes the scan timestamp and keeps the actual
// scan operation explicit so callers can return cached choices immediately.
func DiscoveryRefreshAge() time.Duration {
	return discoveryRefreshAge
}

func (s *Service) ListDesktopDiscoveryRoots(ctx context.Context) ([]*models.DesktopDiscoveryRoot, error) {
	if !s.discoveryConfig.DesktopRuntime || s.desktopRootStore == nil {
		return nil, ErrDesktopDiscoveryUnavailable
	}
	return s.desktopRootStore.ListDesktopDiscoveryRoots(ctx)
}

// GetLocalRepositoryDiscovery returns the current cached snapshot. It never
// starts a filesystem walk. Call RefreshLocalRepositoryDiscovery for an
// explicit or stale refresh.
func (s *Service) GetLocalRepositoryDiscovery(ctx context.Context, root string) (RepositoryDiscoveryResult, error) {
	return s.getLocalRepositoryDiscovery(ctx, "", root)
}

// GetLocalRepositoryDiscoveryForWorkspace applies the workspace visibility
// check before returning a cached discovery snapshot.
func (s *Service) GetLocalRepositoryDiscoveryForWorkspace(
	ctx context.Context,
	workspaceID, root string,
) (RepositoryDiscoveryResult, error) {
	return s.getLocalRepositoryDiscovery(ctx, workspaceID, root)
}

func (s *Service) getLocalRepositoryDiscovery(
	ctx context.Context,
	workspaceID, root string,
) (RepositoryDiscoveryResult, error) {
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return RepositoryDiscoveryResult{}, err
	}
	roots, err := s.resolveDiscoveryRoots(ctx, root)
	if err != nil {
		return RepositoryDiscoveryResult{}, err
	}
	states, err := s.discoveryRootStates(ctx, roots)
	if err != nil {
		return RepositoryDiscoveryResult{}, err
	}
	homeConfirmation, err := s.homeConfirmationRequired(ctx)
	if err != nil {
		return RepositoryDiscoveryResult{}, err
	}
	key := discoveryCacheKey(roots, s.discoveryMaxDepth())

	s.discoveryCacheMu.Lock()
	entry, cached := s.discoveryCache[key]
	refreshing := s.discoveryFlights[key] != nil
	s.discoveryCacheMu.Unlock()

	if !cached {
		return RepositoryDiscoveryResult{
			Roots:                    roots,
			RootStates:               states,
			DesktopRuntime:           s.discoveryConfig.DesktopRuntime,
			Refreshing:               refreshing,
			HomeConfirmationRequired: homeConfirmation,
		}, nil
	}
	if len(states) == 0 {
		states = cloneRootStates(entry.rootStates)
	}
	return RepositoryDiscoveryResult{
		Roots:                    roots,
		Repositories:             cloneRepositories(entry.repositories),
		DesktopRuntime:           s.discoveryConfig.DesktopRuntime,
		RootStates:               states,
		ScanTime:                 cloneTime(entry.scanTime),
		Refreshing:               refreshing,
		Cached:                   true,
		HomeConfirmationRequired: homeConfirmation,
		FailedRoots:              append([]string(nil), entry.failedRoots...),
	}, nil
}

// RefreshLocalRepositoryDiscovery performs one scan for the normalized root
// set. Concurrent callers share the same in-flight scan. A failed root keeps
// the previous successful repository list in the cache.
func (s *Service) RefreshLocalRepositoryDiscovery(ctx context.Context, root string) (RepositoryDiscoveryResult, error) {
	return s.refreshLocalRepositoryDiscovery(ctx, "", root, discoveryTriggerManualRefresh)
}

// RefreshLocalRepositoryDiscoveryForWorkspace applies the workspace
// visibility check before starting or joining a discovery scan.
func (s *Service) RefreshLocalRepositoryDiscoveryForWorkspace(
	ctx context.Context,
	workspaceID, root string,
) (RepositoryDiscoveryResult, error) {
	return s.RefreshLocalRepositoryDiscoveryForWorkspaceWithTrigger(
		ctx, workspaceID, root, discoveryTriggerManualRefresh,
	)
}

// RefreshLocalRepositoryDiscoveryForWorkspaceWithTrigger applies the
// workspace visibility check before starting or joining a scan. The validated
// trigger is retained in scan diagnostics so support can distinguish stale
// activation from a user refresh or root selection.
func (s *Service) RefreshLocalRepositoryDiscoveryForWorkspaceWithTrigger(
	ctx context.Context,
	workspaceID, root, trigger string,
) (RepositoryDiscoveryResult, error) {
	return s.refreshLocalRepositoryDiscovery(
		ctx,
		workspaceID,
		root,
		NormalizeRepositoryDiscoveryTrigger(trigger),
	)
}

func (s *Service) refreshLocalRepositoryDiscovery(
	ctx context.Context,
	workspaceID, root, trigger string,
) (RepositoryDiscoveryResult, error) {
	trigger = NormalizeRepositoryDiscoveryTrigger(trigger)
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return RepositoryDiscoveryResult{}, err
	}
	roots, err := s.resolveDiscoveryRoots(ctx, root)
	if err != nil {
		return RepositoryDiscoveryResult{}, err
	}
	states, err := s.discoveryRootStates(ctx, roots)
	if err != nil {
		return RepositoryDiscoveryResult{}, err
	}
	homeConfirmation, err := s.homeConfirmationRequired(ctx)
	if err != nil {
		return RepositoryDiscoveryResult{}, err
	}
	if len(roots) == 0 {
		s.logFilesystemInfo("repository discovery scan skipped", "repository.discovery.scan", root, trigger)
		return RepositoryDiscoveryResult{
			Roots:                    roots,
			RootStates:               states,
			DesktopRuntime:           s.discoveryConfig.DesktopRuntime,
			HomeConfirmationRequired: homeConfirmation,
		}, nil
	}

	key := discoveryCacheKey(roots, s.discoveryMaxDepth())
	s.discoveryCacheMu.Lock()
	if flight := s.discoveryFlights[key]; flight != nil {
		done := flight.done
		s.discoveryCacheMu.Unlock()
		select {
		case <-done:
			if errors.Is(flight.err, context.Canceled) || errors.Is(flight.err, context.DeadlineExceeded) {
				return s.refreshLocalRepositoryDiscovery(ctx, workspaceID, root, trigger)
			}
			return flight.result, flight.err
		case <-ctx.Done():
			return RepositoryDiscoveryResult{}, ctx.Err()
		}
	}
	previous, hasPrevious := s.discoveryCache[key]
	flight := &discoveryFlight{done: make(chan struct{})}
	s.discoveryFlights[key] = flight
	s.discoveryCacheMu.Unlock()

	result, scanErr := s.scanDiscoveryRoots(ctx, roots, previous, hasPrevious, homeConfirmation, trigger)
	s.discoveryCacheMu.Lock()
	flight.result = result
	flight.err = scanErr
	delete(s.discoveryFlights, key)
	close(flight.done)
	s.discoveryCacheMu.Unlock()
	return result, scanErr
}

func (s *Service) scanDiscoveryRoots(
	ctx context.Context,
	roots []string,
	previous discoveryCacheEntry,
	hasPrevious bool,
	homeConfirmation bool,
	trigger string,
) (RepositoryDiscoveryResult, error) {
	trigger = NormalizeRepositoryDiscoveryTrigger(trigger)
	found := make([]LocalRepository, 0)
	failed := make([]string, 0)
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return RepositoryDiscoveryResult{}, err
		}
		s.logFilesystemInfo("repository discovery scan started", "repository.discovery.scan", root, trigger)
		repositories, err := s.discoveryScanRoot(ctx, root, s.discoveryMaxDepth())
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return RepositoryDiscoveryResult{}, err
			}
			failed = append(failed, root)
			s.logFilesystemFailure("repository.discovery.scan", root, trigger, err)
			s.recordDesktopRootFailure(ctx, root, err)
			continue
		}
		found = appendUniqueRepositories(found, repositories)
		s.recordDesktopRootSuccess(ctx, root)
	}

	states, err := s.discoveryRootStates(ctx, roots)
	if err != nil {
		return RepositoryDiscoveryResult{}, err
	}
	if len(failed) > 0 {
		repositories := found
		scanTime := (*time.Time)(nil)
		cached := hasPrevious
		if hasPrevious {
			repositories = cloneRepositories(previous.repositories)
			scanTime = cloneTime(previous.scanTime)
		}
		entry := discoveryCacheEntry{
			roots:        append([]string(nil), roots...),
			repositories: cloneRepositories(repositories),
			rootStates:   cloneRootStates(states),
			scanTime:     cloneTime(scanTime),
			failedRoots:  append([]string(nil), failed...),
		}
		s.storeDiscoveryCache(roots, entry)
		return RepositoryDiscoveryResult{
			Roots:                    roots,
			Repositories:             repositories,
			DesktopRuntime:           s.discoveryConfig.DesktopRuntime,
			RootStates:               states,
			ScanTime:                 scanTime,
			Cached:                   cached,
			HomeConfirmationRequired: homeConfirmation,
			FailedRoots:              failed,
		}, nil
	}

	now := s.discoveryClockNow()
	entry := discoveryCacheEntry{
		roots:        append([]string(nil), roots...),
		repositories: cloneRepositories(found),
		rootStates:   cloneRootStates(states),
		scanTime:     &now,
	}
	s.storeDiscoveryCache(roots, entry)
	return RepositoryDiscoveryResult{
		Roots:                    roots,
		Repositories:             found,
		DesktopRuntime:           s.discoveryConfig.DesktopRuntime,
		RootStates:               states,
		ScanTime:                 &now,
		HomeConfirmationRequired: homeConfirmation,
	}, nil
}

func (s *Service) storeDiscoveryCache(roots []string, entry discoveryCacheEntry) {
	key := discoveryCacheKey(roots, s.discoveryMaxDepth())
	s.discoveryCacheMu.Lock()
	s.discoveryCache[key] = entry
	s.discoveryCacheMu.Unlock()
}

func (s *Service) resolveDiscoveryRoots(ctx context.Context, requestedRoot string) ([]string, error) {
	roots, err := s.effectiveDiscoveryRoots(ctx)
	if err != nil {
		return nil, err
	}
	if requestedRoot == "" {
		return roots, nil
	}
	absRoot, err := filepath.Abs(filepath.Clean(requestedRoot))
	if err != nil {
		return nil, fmt.Errorf("invalid root path: %w", err)
	}
	if !isPathAllowed(absRoot, roots) {
		return nil, ErrPathNotAllowed
	}
	return []string{filepath.Clean(absRoot)}, nil
}

func (s *Service) effectiveDiscoveryRoots(ctx context.Context) ([]string, error) {
	roots := make([]string, 0, len(s.discoveryConfig.Roots)+2)
	if len(s.discoveryConfig.Roots) > 0 {
		roots = append(roots, s.discoveryConfig.Roots...)
	} else if !s.discoveryConfig.DesktopRuntime {
		if home, err := os.UserHomeDir(); err == nil {
			roots = append(roots, home)
		}
	}
	if s.discoveryConfig.DesktopRuntime && s.desktopRootStore != nil {
		saved, err := s.desktopRootStore.ListDesktopDiscoveryRoots(ctx)
		if err != nil {
			return nil, err
		}
		for _, root := range saved {
			if root != nil {
				roots = append(roots, root.Path)
			}
		}
	}
	if s.repoCloneLocation != nil {
		if base, err := s.repoCloneLocation.ExpandedBasePath(); err == nil && base != "" {
			roots = append(roots, base)
		}
	}
	return normalizeRoots(roots), nil
}

func (s *Service) homeConfirmationRequired(ctx context.Context) (bool, error) {
	if !s.discoveryConfig.DesktopRuntime || s.desktopRootStore == nil || len(s.discoveryConfig.Roots) > 0 {
		return false, nil
	}
	roots, err := s.desktopRootStore.ListDesktopDiscoveryRoots(ctx)
	if err != nil {
		return false, err
	}
	if len(roots) > 0 {
		return false, nil
	}
	migration, err := s.desktopRootStore.GetDesktopDiscoveryMigration(ctx)
	if err != nil {
		return false, err
	}
	return migration != nil && migration.HomeConfirmationRequired, nil
}

func (s *Service) discoveryRootStates(ctx context.Context, roots []string) ([]models.DesktopDiscoveryRoot, error) {
	stored := make(map[string]*models.DesktopDiscoveryRoot)
	if s.discoveryConfig.DesktopRuntime && s.desktopRootStore != nil {
		items, err := s.desktopRootStore.ListDesktopDiscoveryRoots(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item != nil {
				stored[filepath.Clean(item.Path)] = item
			}
		}
	}
	states := make([]models.DesktopDiscoveryRoot, 0, len(roots))
	for _, root := range roots {
		if item := stored[filepath.Clean(root)]; item != nil {
			states = append(states, *cloneRoot(item))
			continue
		}
		states = append(states, models.DesktopDiscoveryRoot{
			Path:        root,
			DisplayPath: displayDiscoveryPath(root),
			State:       models.DesktopDiscoveryRootConnected,
		})
	}
	return states, nil
}

// AddDesktopDiscoveryRoot persists a canonical install-wide root and starts
// its first immediate scan. A scan failure is represented by root state so the
// caller can offer recovery without treating an inaccessible folder as a
// malformed request.
func (s *Service) AddDesktopDiscoveryRoot(ctx context.Context, path string) (*models.DesktopDiscoveryRoot, error) {
	if !s.discoveryConfig.DesktopRuntime || s.desktopRootStore == nil {
		return nil, ErrDesktopDiscoveryUnavailable
	}
	canonical, err := canonicalDiscoveryRoot(path)
	if err != nil {
		return nil, err
	}
	root, err := s.desktopRootStore.GetDesktopDiscoveryRoot(ctx, canonical)
	if err != nil {
		return nil, err
	}
	if root == nil {
		root = &models.DesktopDiscoveryRoot{
			ID:          uuid.NewString(),
			Path:        canonical,
			DisplayPath: displayDiscoveryPath(canonical),
			State:       models.DesktopDiscoveryRootConnected,
		}
		if err := s.desktopRootStore.CreateDesktopDiscoveryRoot(ctx, root); err != nil {
			return nil, err
		}
	} else {
		root.State = models.DesktopDiscoveryRootConnected
		root.LastFailureAt = nil
		root.LastFailureCode = ""
		if err := s.desktopRootStore.UpdateDesktopDiscoveryRoot(ctx, root); err != nil {
			return nil, err
		}
	}
	if isCurrentHomePath(canonical) {
		if err := s.clearHomeConfirmation(ctx); err != nil {
			return nil, err
		}
	}
	s.invalidateDiscoveryCache()
	if _, err := s.refreshLocalRepositoryDiscovery(ctx, "", "", discoveryTriggerUserSelect); err != nil {
		return nil, err
	}
	return s.getRequiredDesktopDiscoveryRoot(ctx, canonical)
}

// ReconnectDesktopDiscoveryRoot replaces the path for an inaccessible root,
// then performs one immediate scan under the new user-selected path.
func (s *Service) ReconnectDesktopDiscoveryRoot(ctx context.Context, oldPath, newPath string) (*models.DesktopDiscoveryRoot, error) {
	if !s.discoveryConfig.DesktopRuntime || s.desktopRootStore == nil {
		return nil, ErrDesktopDiscoveryUnavailable
	}
	canonical, err := canonicalDiscoveryRoot(newPath)
	if err != nil {
		return nil, err
	}
	oldLookupPath, err := normalizeDiscoveryRootLookupPath(oldPath)
	if err != nil {
		return nil, err
	}
	old, err := s.desktopRootStore.GetDesktopDiscoveryRoot(ctx, oldLookupPath)
	if err != nil {
		return nil, err
	}
	if old == nil {
		return s.AddDesktopDiscoveryRoot(ctx, canonical)
	}
	conflict, err := s.desktopRootStore.GetDesktopDiscoveryRoot(ctx, canonical)
	if err != nil {
		return nil, err
	}
	if conflict != nil && conflict.ID != old.ID {
		return nil, fmt.Errorf("%w: root already exists", ErrInvalidDiscoveryRoot)
	}
	old.Path = canonical
	old.DisplayPath = displayDiscoveryPath(canonical)
	old.State = models.DesktopDiscoveryRootConnected
	old.LastScanAt = nil
	old.LastFailureAt = nil
	old.LastFailureCode = ""
	if err := s.desktopRootStore.UpdateDesktopDiscoveryRoot(ctx, old); err != nil {
		return nil, err
	}
	if isCurrentHomePath(canonical) {
		if err := s.clearHomeConfirmation(ctx); err != nil {
			return nil, err
		}
	}
	s.invalidateDiscoveryCache()
	if _, err := s.refreshLocalRepositoryDiscovery(ctx, "", "", discoveryTriggerUserSelect); err != nil {
		return nil, err
	}
	return s.getRequiredDesktopDiscoveryRoot(ctx, canonical)
}

func (s *Service) getRequiredDesktopDiscoveryRoot(
	ctx context.Context, path string,
) (*models.DesktopDiscoveryRoot, error) {
	root, err := s.desktopRootStore.GetDesktopDiscoveryRoot(ctx, path)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, errors.New("desktop discovery root disappeared after scan")
	}
	return root, nil
}

func normalizeDiscoveryRootLookupPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%w: path is required", ErrInvalidDiscoveryRoot)
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidDiscoveryRoot, err)
	}
	if canonical, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(canonical), nil
	}
	return filepath.Clean(abs), nil
}

func (s *Service) RemoveDesktopDiscoveryRoot(ctx context.Context, path string) error {
	if !s.discoveryConfig.DesktopRuntime || s.desktopRootStore == nil {
		return ErrDesktopDiscoveryUnavailable
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%w: path is required", ErrInvalidDiscoveryRoot)
	}
	canonical, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidDiscoveryRoot, err)
	}
	if err := s.desktopRootStore.DeleteDesktopDiscoveryRoot(ctx, filepath.Clean(canonical)); err != nil {
		return err
	}
	s.invalidateDiscoveryCache()
	return nil
}

func (s *Service) clearHomeConfirmation(ctx context.Context) error {
	return s.desktopRootStore.SetDesktopDiscoveryMigration(ctx, &models.DesktopDiscoveryMigration{})
}

func (s *Service) recordDesktopRootSuccess(ctx context.Context, path string) {
	if !s.discoveryConfig.DesktopRuntime || s.desktopRootStore == nil {
		return
	}
	root, err := s.desktopRootStore.GetDesktopDiscoveryRoot(ctx, path)
	if err != nil || root == nil {
		return
	}
	now := s.discoveryClockNow()
	root.State = models.DesktopDiscoveryRootConnected
	root.LastScanAt = &now
	root.LastFailureAt = nil
	root.LastFailureCode = ""
	_ = s.desktopRootStore.UpdateDesktopDiscoveryRoot(ctx, root)
}

func (s *Service) recordDesktopRootFailure(ctx context.Context, path string, scanErr error) {
	if !s.discoveryConfig.DesktopRuntime || s.desktopRootStore == nil {
		return
	}
	root, err := s.desktopRootStore.GetDesktopDiscoveryRoot(ctx, path)
	if err != nil || root == nil {
		return
	}
	now := s.discoveryClockNow()
	root.State = models.DesktopDiscoveryRootReconnectRequired
	root.LastFailureAt = &now
	root.LastFailureCode = discoveryFailureCode(scanErr)
	_ = s.desktopRootStore.UpdateDesktopDiscoveryRoot(ctx, root)
}

func discoveryFailureCode(err error) string {
	switch {
	case fsdiagnostics.IsAccessDenied(err):
		return "permission_denied"
	case errors.Is(err, os.ErrNotExist):
		return "not_found"
	default:
		return "scan_failed"
	}
}

func canonicalDiscoveryRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%w: path is required", ErrInvalidDiscoveryRoot)
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidDiscoveryRoot, err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidDiscoveryRoot, err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidDiscoveryRoot, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: path is not a directory", ErrInvalidDiscoveryRoot)
	}
	return filepath.Clean(canonical), nil
}

func displayDiscoveryPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(filepath.Clean(home), filepath.Clean(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return path
	}
	if rel == "." {
		return "~"
	}
	return filepath.Join("~", rel)
}

func isCurrentHomePath(path string) bool {
	home, err := os.UserHomeDir()
	return err == nil && sameCanonicalPath(filepath.Clean(home), filepath.Clean(path))
}

func (s *Service) invalidateDiscoveryCache() {
	s.discoveryCacheMu.Lock()
	s.discoveryCache = make(map[string]discoveryCacheEntry)
	s.discoveryCacheMu.Unlock()
}

func (s *Service) discoveryClockNow() time.Time {
	if s.discoveryNow != nil {
		return s.discoveryNow().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) SetDiscoveryClockForTest(now func() time.Time) {
	s.discoveryNow = now
}

func appendUniqueRepositories(dst []LocalRepository, src []LocalRepository) []LocalRepository {
	seen := make(map[string]struct{}, len(dst)+len(src))
	for _, repo := range dst {
		seen[repo.Path] = struct{}{}
	}
	for _, repo := range src {
		if _, ok := seen[repo.Path]; ok {
			continue
		}
		seen[repo.Path] = struct{}{}
		dst = append(dst, repo)
	}
	sort.Slice(dst, func(i, j int) bool { return dst[i].Path < dst[j].Path })
	return dst
}

func discoveryCacheKey(roots []string, maxDepth int) string {
	ordered := append([]string(nil), roots...)
	sort.Strings(ordered)
	return fmt.Sprintf("%d:%s", maxDepth, strings.Join(ordered, "\x00"))
}

func cloneRepositories(repositories []LocalRepository) []LocalRepository {
	return append([]LocalRepository(nil), repositories...)
}

func cloneRootStates(states []models.DesktopDiscoveryRoot) []models.DesktopDiscoveryRoot {
	result := make([]models.DesktopDiscoveryRoot, 0, len(states))
	for i := range states {
		result = append(result, *cloneRoot(&states[i]))
	}
	return result
}

func cloneRoot(root *models.DesktopDiscoveryRoot) *models.DesktopDiscoveryRoot {
	if root == nil {
		return nil
	}
	result := *root
	if root.LastScanAt != nil {
		value := *root.LastScanAt
		result.LastScanAt = &value
	}
	if root.LastFailureAt != nil {
		value := *root.LastFailureAt
		result.LastFailureAt = &value
	}
	return &result
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
