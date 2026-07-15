package workflowsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

// Applier applies parsed workflow definition files to a workspace. Satisfied
// by the workflow service.
type Applier interface {
	ApplySyncedWorkflows(ctx context.Context, workspaceID string, files []workflowservice.SyncFileExport) (*workflowservice.SyncApplyResult, error)
}

// ClientProvider hands out the GitHub client used to read the sync repo.
// Satisfied by the GitHub service; Client() may return nil when GitHub is not
// authenticated.
type ClientProvider interface {
	Client() github.Client
}

// Service owns workflow sync configuration and sync execution.
type Service struct {
	store   *Store
	clients ClientProvider
	applier Applier
	logger  *logger.Logger
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
	return s.store.UpsertConfigForWorkspace(ctx, workspaceID, req)
}

// DeleteConfigForWorkspace removes the workspace's config. Already-synced
// workflows are left in place; they simply stop syncing.
func (s *Service) DeleteConfigForWorkspace(ctx context.Context, workspaceID string) error {
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
	client := s.clients.Client()
	if client == nil {
		return nil, fmt.Errorf("GitHub is not authenticated; configure a GitHub token to sync workflows")
	}
	entries, err := client.ListRepoDirectory(ctx, cfg.RepoOwner, cfg.RepoName, cfg.Path, cfg.Branch)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s/%s@%s:%s: %w", cfg.RepoOwner, cfg.RepoName, cfg.Branch, cfg.Path, err)
	}
	var files []fetchedFile
	for _, entry := range entries {
		if entry.Type != "file" || !isSyncableFile(entry.Name) {
			continue
		}
		content, err := client.GetRepoFileContent(ctx, cfg.RepoOwner, cfg.RepoName, entry.Path, cfg.Branch)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s: %w", entry.Path, err)
		}
		files = append(files, fetchedFile{path: entry.Path, content: content})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, nil
}

// contentHash is a stable digest of the fetched file set, used to skip
// re-applying unchanged content on periodic syncs.
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
	if cfg.LastSyncedAt == nil {
		return true
	}
	return now.Sub(*cfg.LastSyncedAt) >= time.Duration(cfg.IntervalSeconds)*time.Second
}
