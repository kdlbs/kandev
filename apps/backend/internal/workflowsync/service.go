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
	"github.com/kandev/kandev/internal/gitlab"
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

// GitHubClientProvider exposes workspace-routed GitHub repository reads.
type GitHubClientProvider interface {
	ListRepoDirectoryForWorkspace(
		ctx context.Context, workspaceID, owner, repo, path, ref string,
	) ([]github.RepoContentEntry, error)
	GetRepoFileContentForWorkspace(
		ctx context.Context, workspaceID, owner, repo, path, ref string,
	) ([]byte, error)
}

// GitLabClientProvider exposes workspace-routed GitLab repository reads.
// Satisfied by gitlab.Service.
type GitLabClientProvider interface {
	ListRepoTreeForWorkspace(
		ctx context.Context, workspaceID, projectPath, path, ref string,
	) ([]gitlab.RepoTreeEntry, error)
	GetRepoFileContentForWorkspace(
		ctx context.Context, workspaceID, projectPath, path, ref string,
	) ([]byte, error)
}

// Compile-time checks that both integrations' real services satisfy the
// interfaces above, so drift in either package's workspace-routed methods
// breaks the build rather than surfacing only at DI-wiring time.
var (
	_ GitHubClientProvider = (*github.Service)(nil)
	_ GitLabClientProvider = (*gitlab.Service)(nil)
)

// dirEntry is a provider-neutral directory listing entry. It exists only to
// share the file-selection and content-fetch loop in fetchFiles between the
// two providers' native listing shapes.
type dirEntry struct {
	name   string
	path   string
	isFile bool
}

// Service owns workflow sync configuration and sync execution.
type Service struct {
	store         *Store
	githubClients GitHubClientProvider
	gitlabClients GitLabClientProvider
	applier       Applier
	logger        *logger.Logger
	// locks serializes syncs and config mutations per workspace so a force
	// sync cannot interleave with a config delete/replace and apply stale
	// definitions (or re-stamp workflows that were just released). The lock
	// is deliberately held across the fetch too: a config change or delete
	// for that workspace waits (bounded by the HTTP client timeout) rather
	// than racing an in-flight apply.
	locks sync.Map // workspaceID → *sync.Mutex

	// workspaceAuthorizer enforces per-user workspace scoping. Nil (unit
	// tests, or a caller with no identity in context — internal callers like
	// the periodic poller) means unscoped, matching every other integration
	// service's default before auth is wired up.
	workspaceAuthorizer func(context.Context, string) error
}

// SetWorkspaceAuthorizer installs the per-user workspace access boundary
// applied before every user-facing config read/write and force sync.
func (s *Service) SetWorkspaceAuthorizer(authorizer func(context.Context, string) error) {
	if s != nil {
		s.workspaceAuthorizer = authorizer
	}
}

func (s *Service) authorizeWorkspaceAccess(ctx context.Context, workspaceID string) error {
	if s == nil || s.workspaceAuthorizer == nil {
		return nil
	}
	return s.workspaceAuthorizer(ctx, workspaceID)
}

func (s *Service) workspaceLock(workspaceID string) *sync.Mutex {
	lock, _ := s.locks.LoadOrStore(workspaceID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// NewService creates a workflow sync service. Either client provider may be
// nil — a workspace configured for the provider whose client is nil gets an
// actionable failure at sync time rather than a construction-time error, so
// backend boot succeeds with either integration unconfigured.
func NewService(
	store *Store, githubClients GitHubClientProvider, gitlabClients GitLabClientProvider,
	applier Applier, log *logger.Logger,
) *Service {
	return &Service{
		store:         store,
		githubClients: githubClients,
		gitlabClients: gitlabClients,
		applier:       applier,
		logger:        log.WithFields(zap.String("component", "workflowsync-service")),
	}
}

// Store exposes the config store (e2e reset cascade).
func (s *Service) Store() *Store {
	return s.store
}

// GetConfigForWorkspace returns the workspace's config, or nil when unset.
func (s *Service) GetConfigForWorkspace(ctx context.Context, workspaceID string) (*Config, error) {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.store.GetConfigForWorkspace(ctx, workspaceID)
}

// SetConfigForWorkspace validates and stores the workspace's config.
func (s *Service) SetConfigForWorkspace(ctx context.Context, workspaceID string, req *SetConfigRequest) (*Config, error) {
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
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
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return err
	}
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
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}
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
// definition file in it (non-recursive), dispatching to the configured
// provider. File selection, ordering, and error wrapping are shared; only
// listing and content-fetch are provider-specific.
func (s *Service) fetchFiles(ctx context.Context, cfg *Config) ([]fetchedFile, error) {
	entries, get, err := s.listProviderEntries(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var files []fetchedFile
	for _, entry := range entries {
		if !entry.isFile || !isSyncableFile(entry.name) {
			continue
		}
		content, err := get(ctx, entry.path)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s: %w", entry.path, err)
		}
		files = append(files, fetchedFile{path: entry.path, content: content})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, nil
}

// fileGetter fetches one file's content once the provider and workspace are
// already fixed, so fetchFiles can share its loop across providers.
type fileGetter func(ctx context.Context, path string) ([]byte, error)

// listProviderEntries lists the configured directory for cfg.Provider and
// returns a fileGetter closed over the same provider and workspace.
func (s *Service) listProviderEntries(ctx context.Context, cfg *Config) ([]dirEntry, fileGetter, error) {
	if cfg.Provider == ProviderGitLab {
		return s.listGitLabEntries(ctx, cfg)
	}
	return s.listGitHubEntries(ctx, cfg)
}

func (s *Service) listGitHubEntries(ctx context.Context, cfg *Config) ([]dirEntry, fileGetter, error) {
	if s.githubClients == nil {
		return nil, nil, fmt.Errorf("GitHub is not authenticated; configure a GitHub token to sync workflows")
	}
	raw, err := s.githubClients.ListRepoDirectoryForWorkspace(
		ctx, cfg.WorkspaceID, cfg.RepoOwner, cfg.RepoName, cfg.Path, cfg.Branch,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list %s/%s@%s:%s: %w", cfg.RepoOwner, cfg.RepoName, cfg.Branch, cfg.Path, err)
	}
	entries := make([]dirEntry, len(raw))
	for i, e := range raw {
		entries[i] = dirEntry{name: e.Name, path: e.Path, isFile: e.Type == "file"}
	}
	get := func(ctx context.Context, path string) ([]byte, error) {
		return s.githubClients.GetRepoFileContentForWorkspace(ctx, cfg.WorkspaceID, cfg.RepoOwner, cfg.RepoName, path, cfg.Branch)
	}
	return entries, get, nil
}

func (s *Service) listGitLabEntries(ctx context.Context, cfg *Config) ([]dirEntry, fileGetter, error) {
	if s.gitlabClients == nil {
		return nil, nil, fmt.Errorf("GitLab is not authenticated; configure a GitLab connection to sync workflows")
	}
	raw, err := s.gitlabClients.ListRepoTreeForWorkspace(ctx, cfg.WorkspaceID, cfg.ProjectPath, cfg.Path, cfg.Branch)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list %s@%s:%s: %w", cfg.ProjectPath, cfg.Branch, cfg.Path, err)
	}
	entries := make([]dirEntry, len(raw))
	for i, e := range raw {
		entries[i] = dirEntry{name: e.Name, path: e.Path, isFile: e.Type == gitlab.TreeEntryTypeBlob}
	}
	get := func(ctx context.Context, path string) ([]byte, error) {
		return s.gitlabClients.GetRepoFileContentForWorkspace(ctx, cfg.WorkspaceID, cfg.ProjectPath, path, cfg.Branch)
	}
	return entries, get, nil
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
