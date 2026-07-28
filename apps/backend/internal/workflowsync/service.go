package workflowsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

// Applier applies parsed workflow definition files to a workspace and
// releases synced workflows back to manual ownership when syncing stops.
// Satisfied by the workflow service.
type Applier interface {
	ApplySyncedWorkflows(ctx context.Context, workspaceID string, files []workflowservice.SyncFileExport) (*workflowservice.SyncApplyResult, error)
	ReleaseSyncedWorkflows(ctx context.Context, workspaceID string) ([]string, error)
}

// ClientProvider exposes workspace-routed GitHub repository reads.
type ClientProvider interface {
	ListRepoDirectoryForWorkspace(
		ctx context.Context, workspaceID, owner, repo, path, ref string,
	) ([]github.RepoContentEntry, error)
	GetRepoFileContentForWorkspace(
		ctx context.Context, workspaceID, owner, repo, path, ref string,
	) ([]byte, error)
}

// Service owns workflow sync configuration and sync execution.
type Service struct {
	store   *Store
	clients ClientProvider
	applier Applier
	logger  *logger.Logger
	// locks serializes syncs and config mutations per workspace so a force
	// sync cannot interleave with a config delete/replace and apply stale
	// definitions (or re-stamp workflows that were just released). The lock
	// is deliberately held across the GitHub fetch too: a config change or
	// delete for that workspace waits (bounded by the HTTP client timeout)
	// rather than racing an in-flight apply.
	locks sync.Map // workspaceID → *sync.Mutex
}

func (s *Service) workspaceLock(workspaceID string) *sync.Mutex {
	lock, _ := s.locks.LoadOrStore(workspaceID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// NewService creates a workflow sync service.
func NewService(store *Store, clients ClientProvider, applier Applier, log *logger.Logger) *Service {
	return &Service{
		store:   store,
		clients: clients,
		applier: applier,
		logger:  log.WithFields(zap.String("component", "workflowsync-service")),
	}
}

// Store exposes the config store (e2e reset cascade).
func (s *Service) Store() *Store {
	return s.store
}

// GetConfigForWorkspace returns the workspace's config, or nil when unset.
func (s *Service) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*Config, error) {
	return s.store.GetConfigForWorkspace(ctx, workspaceID)
}

// SetConfigForWorkspace validates and stores the workspace's config.
func (s *Service) SetConfigForWorkspace(ctx context.Context, workspaceID string, req *SetConfigRequest) (*Config, error) {
	if err := req.Normalize(); err != nil {
		return nil, err
	}
	lock := s.workspaceLock(workspaceID)
	lock.Lock()
	defer lock.Unlock()
	return s.store.UpsertConfigForWorkspace(ctx, workspaceID, req)
}

// DeleteConfigForWorkspace removes the workspace's config. Previously-synced
// workflows are released back to manual ownership first so they become
// editable again — a failed release keeps the config so the user can retry.
func (s *Service) DeleteConfigForWorkspace(ctx context.Context, workspaceID string) error {
	lock := s.workspaceLock(workspaceID)
	lock.Lock()
	defer lock.Unlock()
	released, err := s.applier.ReleaseSyncedWorkflows(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to release synced workflows: %w", err)
	}
	if len(released) > 0 {
		s.logger.Info("released synced workflows",
			zap.String("workspace_id", workspaceID), zap.Int("count", len(released)))
	}
	return s.store.DeleteConfigForWorkspace(ctx, workspaceID)
}

// syncableExtensions are the file extensions read from the sync directory.
var syncableExtensions = []string{".yml", ".yaml", ".json"}

func isSyncableFile(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range syncableExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// fetchedFile is one definition file read from the repo.
type fetchedFile struct {
	path    string
	content []byte
}

// SyncWorkspace fetches the configured repo directory and reconciles the
// workspace's synced workflows with it. Every sync applies the definitions —
// including repairing local edits to synced workflows — but the applier only
// writes (and broadcasts) what actually differs, so a no-drift sync is
// silent. The outcome (including failures) is recorded on the config row so
// the UI can surface it.
func (s *Service) SyncWorkspace(ctx context.Context, workspaceID string) (*SyncResult, error) {
	lock := s.workspaceLock(workspaceID)
	lock.Lock()
	defer lock.Unlock()
	cfg, err := s.store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNotConfigured
	}

	files, err := s.fetchFiles(ctx, cfg)
	if err != nil {
		s.recordFailure(ctx, workspaceID, err)
		return nil, err
	}

	parsed, warnings := parseFiles(files)
	applied, err := s.applier.ApplySyncedWorkflows(ctx, workspaceID, parsed)
	if err != nil {
		s.recordFailure(ctx, workspaceID, err)
		return nil, err
	}
	warnings = append(warnings, applied.Warnings...)
	if err := s.store.RecordSyncStatus(ctx, workspaceID, true, "", warnings, contentHash(files), time.Now().UTC()); err != nil {
		return nil, err
	}
	return &SyncResult{
		Created:   applied.Created,
		Updated:   applied.Updated,
		Deleted:   applied.Deleted,
		Warnings:  warnings,
		Unchanged: len(applied.Created)+len(applied.Updated)+len(applied.Deleted) == 0 && len(warnings) == 0,
	}, nil
}

func (s *Service) recordFailure(ctx context.Context, workspaceID string, syncErr error) {
	// Clear the hash so the next successful fetch re-applies from scratch.
	if err := s.store.RecordSyncStatus(ctx, workspaceID, false, syncErr.Error(), nil, "", time.Now().UTC()); err != nil {
		s.logger.Warn("failed to record sync failure",
			zap.String("workspace_id", workspaceID), zap.Error(err))
	}
}

// fetchFiles lists the configured directory and downloads every workflow
// definition file in it (non-recursive).
func (s *Service) fetchFiles(ctx context.Context, cfg *Config) ([]fetchedFile, error) {
	if s.clients == nil {
		return nil, fmt.Errorf("GitHub is not authenticated; configure a GitHub token to sync workflows")
	}
	entries, err := s.clients.ListRepoDirectoryForWorkspace(
		ctx, cfg.WorkspaceID, cfg.RepoOwner, cfg.RepoName, cfg.Path, cfg.Branch,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s/%s@%s:%s: %w", cfg.RepoOwner, cfg.RepoName, cfg.Branch, cfg.Path, err)
	}
	var files []fetchedFile
	for _, entry := range entries {
		if entry.Type != "file" || !isSyncableFile(entry.Name) {
			continue
		}
		content, err := s.clients.GetRepoFileContentForWorkspace(
			ctx, cfg.WorkspaceID, cfg.RepoOwner, cfg.RepoName, entry.Path, cfg.Branch,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s: %w", entry.Path, err)
		}
		files = append(files, fetchedFile{path: entry.Path, content: content})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, nil
}

// contentHash is a stable digest of the fetched file set. It is recorded on
// the config row for observability only — every sync reconciles regardless
// (repairing local drift), with the applier writing only actual differences.
func contentHash(files []fetchedFile) string {
	h := sha256.New()
	for _, f := range files {
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", f.path, len(f.content))
		h.Write(f.content)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// parseFiles decodes and validates each fetched file. Files that fail to
// parse are reported as warnings and passed through with a nil export, which
// tells the applier to leave their previously-synced workflows untouched.
func parseFiles(files []fetchedFile) ([]workflowservice.SyncFileExport, []string) {
	parsed := make([]workflowservice.SyncFileExport, 0, len(files))
	var warnings []string
	for _, f := range files {
		export, err := parseExport(f.path, f.content)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", f.path, err))
			parsed = append(parsed, workflowservice.SyncFileExport{Path: f.path})
			continue
		}
		parsed = append(parsed, workflowservice.SyncFileExport{Path: f.path, Export: export})
	}
	return parsed, warnings
}

func parseExport(path string, data []byte) (*workflowmodels.WorkflowExport, error) {
	export := &workflowmodels.WorkflowExport{}
	var err error
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		err = json.Unmarshal(data, export)
	} else {
		err = yaml.Unmarshal(data, export)
	}
	if err != nil {
		return nil, fmt.Errorf("not a valid workflow export file: %w", err)
	}
	if err := export.Validate(); err != nil {
		return nil, err
	}
	return export, nil
}

// SyncDueConfigs runs a periodic sync for every workspace whose interval has
// elapsed. Failures are recorded on the config row and logged, never fatal.
func (s *Service) SyncDueConfigs(ctx context.Context) {
	configs, err := s.store.ListConfigs(ctx)
	if err != nil {
		s.logger.Warn("failed to list workflow sync configs", zap.Error(err))
		return
	}
	now := time.Now().UTC()
	for _, cfg := range configs {
		if ctx.Err() != nil {
			return
		}
		if !isSyncDue(cfg, now) {
			continue
		}
		if _, err := s.SyncWorkspace(ctx, cfg.WorkspaceID); err != nil {
			s.logger.Warn("periodic workflow sync failed",
				zap.String("workspace_id", cfg.WorkspaceID), zap.Error(err))
		}
	}
}

func isSyncDue(cfg *Config, now time.Time) bool {
	if !cfg.PollEnabled {
		return false
	}
	if cfg.LastSyncedAt == nil {
		return true
	}
	return now.Sub(*cfg.LastSyncedAt) >= time.Duration(cfg.IntervalSeconds)*time.Second
}
