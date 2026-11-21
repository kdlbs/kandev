package promptsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	promptmodels "github.com/kandev/kandev/internal/prompts/models"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	"github.com/kandev/kandev/internal/reposync"
)

const genericFailureMessage = "Prompt sync failed"

// PromptApplier is the prompt-service surface Prompt Sync writes through.
type PromptApplier interface {
	ApplySyncedPrompts(ctx context.Context, input promptservice.SyncApplyInput) (*promptservice.SyncApplyResult, error)
	ReleaseSyncedPrompts(ctx context.Context, opts promptstore.ReleaseOptions) (int64, error)
	UnresolvedPromptReferences(ctx context.Context, texts []string) ([][]string, error)
	ListPrompts(ctx context.Context) ([]*promptmodels.Prompt, error)
	SavePromptVersions(ctx context.Context, id string, save promptservice.VersionSave) (*promptmodels.Prompt, error)
	GetPromptByID(ctx context.Context, id string) (*promptmodels.Prompt, error)
}

// ReferenceSource lists the workflow and step prompts that can reference a
// saved prompt, across all workspaces.
type ReferenceSource interface {
	ListPromptReferenceTexts(ctx context.Context) ([]promptservice.PromptReferenceText, error)
}

// CredentialFingerprintProvider optionally exposes a non-secret fingerprint
// that changes whenever a workspace's connection credential changes.
type CredentialFingerprintProvider interface {
	WorkspaceConnectionFingerprint(ctx context.Context, workspaceID string) (string, error)
}

type syncMode uint8

const (
	syncManual syncMode = iota
	syncAutomatic
)

// Service owns the prompt sync configuration and sync runs.
type Service struct {
	store   *Store
	github  reposync.GitHubClientProvider
	gitlab  reposync.GitLabClientProvider
	prompts PromptApplier
	refs    ReferenceSource
	logger  *logger.Logger
	// mu serializes sync runs, configuration saves, and removal, held across
	// fetch and apply so a removal cannot interleave with an in-flight apply.
	mu sync.Mutex

	workspaceAuthorizer func(context.Context, string) error
	workspaceExists     func(context.Context, string) (bool, error)
	now                 func() time.Time
	jitter              func(time.Duration) time.Duration
}

// NewService creates the prompt sync service. Either provider client may be
// nil; a configuration for that provider then fails at sync time.
func NewService(
	store *Store, githubClients reposync.GitHubClientProvider, gitlabClients reposync.GitLabClientProvider,
	prompts PromptApplier, refs ReferenceSource, log *logger.Logger,
) *Service {
	return &Service{
		store: store, github: githubClients, gitlab: gitlabClients, prompts: prompts, refs: refs,
		logger: log.WithFields(zap.String("component", "promptsync-service")),
		now:    time.Now, jitter: reposync.DefaultJitter,
	}
}

// SetWorkspaceAuthorizer installs the per-user access check applied to the
// credential workspace on every user-facing mutation and Sync now.
func (s *Service) SetWorkspaceAuthorizer(authorizer func(context.Context, string) error) {
	s.workspaceAuthorizer = authorizer
}

// SetWorkspaceExists installs the lookup that detects a deleted credential
// workspace before any provider call.
func (s *Service) SetWorkspaceExists(exists func(context.Context, string) (bool, error)) {
	s.workspaceExists = exists
}

// RunExclusive runs fn while holding the service lock, so cleanup that
// bypasses the service (E2E reset) cannot interleave with a sync.
func (s *Service) RunExclusive(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn()
}

func (s *Service) authorize(ctx context.Context, workspaceID string) error {
	if s.workspaceAuthorizer == nil {
		return nil
	}
	return s.workspaceAuthorizer(ctx, workspaceID)
}

// authorizeRemoval authorizes the credential workspace for removal. A deleted
// workspace has no access left to protect, so removal needs only the
// configuration-management scope the route already enforces; otherwise a
// deleted credential workspace would leave Prompt Sync impossible to remove.
func (s *Service) authorizeRemoval(ctx context.Context, workspaceID string) error {
	if s.workspaceExists != nil {
		exists, err := s.workspaceExists(ctx, workspaceID)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
	}
	return s.authorize(ctx, workspaceID)
}

// GetConfig returns the configuration with sanitized errors, or nil.
func (s *Service) GetConfig(ctx context.Context) (*Config, error) {
	cfg, err := s.store.Get(ctx)
	if err != nil || cfg == nil {
		return cfg, err
	}
	cfg.LastError = safeStoredMessage(cfg.LastError)
	cfg.PollSuspensionReason = safeStoredMessage(cfg.PollSuspensionReason)
	return cfg, nil
}

// SetConfig validates and stores the configuration without syncing.
func (s *Service) SetConfig(ctx context.Context, req *SetConfigRequest) (*Config, error) {
	if err := req.Normalize(); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, req.WorkspaceID); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(ctx, req)
}

// saveLocked stores a normalized, authorized request. The caller holds s.mu.
func (s *Service) saveLocked(ctx context.Context, req *SetConfigRequest) (*Config, error) {
	if req.Provider != reposync.ProviderGitHub || s.github == nil {
		return s.store.Upsert(ctx, req)
	}
	bound, err := s.github.BindWorkspaceGitHubHost(ctx, req.WorkspaceID, req.GitHubHost)
	if err != nil {
		return nil, err
	}
	var cfg *Config
	err = s.github.CommitWorkspaceGitHubHost(bound, req.WorkspaceID, func(commitCtx context.Context) error {
		var saveErr error
		cfg, saveErr = s.store.Upsert(commitCtx, req)
		return saveErr
	})
	return cfg, err
}

// Removal is the user's choices when Prompt Sync is removed: Choices picks the
// local or repository version for prompts that have both (others keep the
// version in use), and DeleteRepositoryOnly names the reviewed synced prompts
// that only came from the repository to delete instead of keeping them.
type Removal struct {
	Choices              map[string]promptservice.ConflictChoice
	DeleteRepositoryOnly []string
}

// DeleteConfig releases every synced prompt and override to manual
// ownership, applying the removal choices, then removes the configuration. A
// failed release keeps the configuration for a retry.
func (s *Service) DeleteConfig(ctx context.Context, removal Removal) error {
	if err := validateResolutions(removal.Choices); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.store.Get(ctx)
	if err != nil {
		return err
	}
	if cfg != nil {
		if err := s.authorizeRemoval(ctx, cfg.WorkspaceID); err != nil {
			return err
		}
	}
	prompts, err := s.prompts.ListPrompts(ctx)
	if err != nil {
		return err
	}
	released, err := s.prompts.ReleaseSyncedPrompts(ctx, promptstore.ReleaseOptions{
		UseOther:             switchingPrompts(prompts, removal.Choices),
		DeleteRepositoryOnly: removal.DeleteRepositoryOnly,
	})
	if err != nil {
		return fmt.Errorf("failed to release synced prompts: %w", err)
	}
	if released > 0 {
		s.logger.Info("released synced prompts", zap.Int64("count", released), zap.Int("repository_only_to_delete", len(removal.DeleteRepositoryOnly)))
	}
	return s.store.Delete(ctx)
}

// Sync runs one sync immediately, bypassing the automatic schedule.
func (s *Service) Sync(ctx context.Context) (*SyncResult, error) {
	return s.SyncWithResolutions(ctx, nil)
}

// SyncWithResolutions runs one sync immediately, applying the user's choices
// for name conflicts; a conflict without a choice stays unresolved. The
// configuration is read and authorized under the service lock, and that same
// snapshot drives the run, so a concurrent save cannot swap in a workspace the
// caller was never checked against.
func (s *Service) SyncWithResolutions(ctx context.Context, resolutions map[string]promptservice.ConflictChoice) (*SyncResult, error) {
	if err := validateResolutions(resolutions); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.lockedAuthorizedConfig(ctx)
	if err != nil {
		return nil, err
	}
	return s.syncLocked(ctx, syncManual, cfg, resolutions)
}

// SyncIfDue runs an automatic sync when the configuration is due. Failures
// are recorded on the configuration and logged, never returned.
func (s *Service) SyncIfDue(ctx context.Context) {
	if !s.mu.TryLock() {
		return
	}
	defer s.mu.Unlock()
	cfg, err := s.store.Get(ctx)
	if err != nil {
		s.logger.Warn("failed to load prompt sync config", zap.Error(err))
		return
	}
	if cfg == nil {
		return
	}
	s.refreshCredentialFingerprint(ctx, cfg)
	if cfg.PollSuspended {
		incPollSkipped("suspended")
		return
	}
	if !cfg.due(s.now().UTC()) {
		return
	}
	_, _ = s.syncLocked(ctx, syncAutomatic, cfg, nil)
}

// refreshCredentialFingerprint resumes a suspended or backed-off config as
// soon as its connection credential changes.
func (s *Service) refreshCredentialFingerprint(ctx context.Context, cfg *Config) {
	provider, ok := s.fingerprintProvider(cfg.Provider)
	if !ok {
		return
	}
	fingerprint, err := provider.WorkspaceConnectionFingerprint(ctx, cfg.WorkspaceID)
	if err != nil || fingerprint == "" || fingerprint == cfg.CredentialFingerprint {
		return
	}
	reset := cfg.CredentialFingerprint != "" || cfg.PollSuspended
	if err := s.store.RecordCredentialFingerprint(ctx, fingerprint, reset); err != nil {
		s.logger.Warn("failed to persist prompt sync credential fingerprint", zap.Error(err))
		return
	}
	cfg.CredentialFingerprint = fingerprint
	if reset {
		cfg.PollSuspended, cfg.NextAttemptAt, cfg.ConsecutiveFailures = false, nil, 0
		cfg.LastSyncedAt = nil
	}
}

func (s *Service) fingerprintProvider(provider string) (CredentialFingerprintProvider, bool) {
	if provider == reposync.ProviderGitLab {
		p, ok := s.gitlab.(CredentialFingerprintProvider)
		return p, ok
	}
	p, ok := s.github.(CredentialFingerprintProvider)
	return p, ok
}

// syncLocked runs one sync for cfg. The caller holds s.mu and passes the
// configuration snapshot it loaded (and, for manual runs, authorized).
func (s *Service) syncLocked(
	ctx context.Context, mode syncMode, cfg *Config, resolutions map[string]promptservice.ConflictChoice,
) (*SyncResult, error) {
	var err error
	if err = s.checkWorkspace(ctx, cfg.WorkspaceID); err != nil {
		s.recordRunError(ctx, cfg, mode, err)
		return nil, err
	}
	useGitHub := cfg.Provider != reposync.ProviderGitLab && s.github != nil
	if useGitHub {
		if mode == syncAutomatic {
			// The service lock is held for the whole run, so a background run
			// must not wait for GitHub's quota; a deferred run is retried later.
			ctx = github.WithNonBlockingGitHubAdmission(github.WithGitHubWorkClass(ctx, github.WorkClassBackground))
		}
		if ctx, err = s.github.BindWorkspaceGitHubHost(ctx, cfg.WorkspaceID, cfg.GitHubHost); err != nil {
			s.recordRunError(ctx, cfg, mode, err)
			return nil, err
		}
	}
	files, err := s.fetch(ctx, cfg)
	if err != nil {
		s.recordRunError(ctx, cfg, mode, err)
		return nil, err
	}
	var result *SyncResult
	apply := func(applyCtx context.Context) error {
		var applyErr error
		result, applyErr = s.apply(applyCtx, files, resolutions)
		return applyErr
	}
	if useGitHub {
		err = s.github.CommitWorkspaceGitHubHost(ctx, cfg.WorkspaceID, apply)
	} else {
		err = apply(ctx)
	}
	if err != nil {
		s.recordRunError(ctx, cfg, mode, err)
		return nil, err
	}
	if err := s.store.RecordSuccess(ctx, result.Warnings, contentHash(files), s.now().UTC()); err != nil {
		return nil, err
	}
	s.recordSuccess(cfg, result)
	return result, nil
}

func (s *Service) checkWorkspace(ctx context.Context, workspaceID string) error {
	if s.workspaceExists == nil {
		return nil
	}
	exists, err := s.workspaceExists(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrCredentialWorkspaceMissing
	}
	return nil
}

func (s *Service) fetch(ctx context.Context, cfg *Config) ([]fetchedFile, error) {
	reader := reposync.Reader{GitHub: s.github, GitLab: s.gitlab}
	entries, err := reader.List(ctx, cfg.source())
	if err != nil {
		return nil, err
	}
	get, err := reader.Getter(cfg.source())
	if err != nil {
		return nil, err
	}
	if err := completeListing(cfg.Provider, entries); err != nil {
		return nil, err
	}
	paths := selectPromptFiles(entries)
	files := make([]fetchedFile, 0, len(paths))
	for _, path := range paths {
		content, err := get(ctx, path)
		if reposync.IsFileTooLarge(err) {
			files = append(files, fetchedFile{path: path, tooLarge: true})
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s: %w", path, err)
		}
		files = append(files, fetchedFile{path: path, content: content})
	}
	return files, nil
}

func (s *Service) apply(
	ctx context.Context, files []fetchedFile, resolutions map[string]promptservice.ConflictChoice,
) (*SyncResult, error) {
	desired, frozen, warnings := parsePromptFiles(files)
	var texts []promptservice.PromptReferenceText
	if s.refs != nil {
		var err error
		if texts, err = s.refs.ListPromptReferenceTexts(ctx); err != nil {
			return nil, fmt.Errorf("failed to list workflow prompt references: %w", err)
		}
	}
	applied, err := s.prompts.ApplySyncedPrompts(ctx, promptservice.SyncApplyInput{
		Desired: desired, FrozenNames: frozen, ReferenceTexts: texts, Resolutions: resolutions,
	})
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, applied.Warnings...)
	unresolved, err := s.unresolvedReferenceWarnings(ctx)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, unresolved...)
	return &SyncResult{
		Created: applied.Created, Updated: applied.Updated, Deleted: applied.Deleted, Restored: applied.Restored,
		Overridden: applied.Overridden, Conflicts: applied.Conflicts, Warnings: warnings,
		Unchanged: !applied.Changed() && len(applied.Conflicts) == 0 && len(warnings) == 0,
	}, nil
}

// unresolvedReferenceWarnings checks every prompt still owned by sync after
// the apply, including frozen prompts and prompts kept because a workflow
// references them, for @name references that resolve to no saved prompt.
func (s *Service) unresolvedReferenceWarnings(ctx context.Context) ([]string, error) {
	all, err := s.prompts.ListPrompts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list prompts: %w", err)
	}
	var synced []*promptmodels.Prompt
	var texts []string
	for _, prompt := range all {
		if prompt.IsSynced() {
			synced = append(synced, prompt)
			texts = append(texts, prompt.Content)
		}
	}
	missing, err := s.prompts.UnresolvedPromptReferences(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("failed to check prompt references: %w", err)
	}
	var warnings []string
	for i, names := range missing {
		for _, name := range names {
			warnings = append(warnings, fmt.Sprintf("%s: prompt %q references @%s, which is not a saved prompt", synced[i].SourcePath, synced[i].Name, name))
		}
	}
	return warnings, nil
}

// contentHash is a stable digest of the fetched file set, stored for
// observability only.
func contentHash(files []fetchedFile) string {
	h := sha256.New()
	for _, f := range files {
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", f.path, len(f.content))
		h.Write(f.content)
	}
	return hex.EncodeToString(h.Sum(nil))
}
