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
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/reposync"
)

const genericFailureMessage = "Prompt sync failed"

// PromptApplier is the prompt-service surface Prompt Sync writes through.
type PromptApplier interface {
	ApplySyncedPrompts(ctx context.Context, input promptservice.SyncApplyInput) (*promptservice.SyncApplyResult, error)
	ReleaseSyncedPrompts(ctx context.Context) (int64, error)
	UnresolvedPromptReferences(ctx context.Context, texts []string) ([][]string, error)
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

// Store exposes the config store (E2E reset).
func (s *Service) Store() *Store { return s.store }

func (s *Service) authorize(ctx context.Context, workspaceID string) error {
	if s.workspaceAuthorizer == nil {
		return nil
	}
	return s.workspaceAuthorizer(ctx, workspaceID)
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

// DeleteConfig releases every synced prompt to manual ownership, then removes
// the configuration. A failed release keeps the configuration for a retry.
func (s *Service) DeleteConfig(ctx context.Context) error {
	cfg, err := s.store.Get(ctx)
	if err != nil {
		return err
	}
	if cfg != nil {
		if err := s.authorize(ctx, cfg.WorkspaceID); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	released, err := s.prompts.ReleaseSyncedPrompts(ctx)
	if err != nil {
		return fmt.Errorf("failed to release synced prompts: %w", err)
	}
	if released > 0 {
		s.logger.Info("released synced prompts", zap.Int64("count", released))
	}
	return s.store.Delete(ctx)
}

// Sync runs one sync immediately, bypassing the automatic schedule.
func (s *Service) Sync(ctx context.Context) (*SyncResult, error) {
	cfg, err := s.store.Get(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNotConfigured
	}
	if err := s.authorize(ctx, cfg.WorkspaceID); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syncLocked(ctx, syncManual)
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
	_, _ = s.syncLocked(ctx, syncAutomatic)
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

func (s *Service) syncLocked(ctx context.Context, mode syncMode) (*SyncResult, error) {
	cfg, err := s.store.Get(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, ErrNotConfigured
	}
	if err := s.checkWorkspace(ctx, cfg.WorkspaceID); err != nil {
		s.recordFailure(ctx, cfg, err)
		return nil, err
	}
	useGitHub := cfg.Provider != reposync.ProviderGitLab && s.github != nil
	if useGitHub {
		if mode == syncAutomatic {
			ctx = github.WithGitHubWorkClass(ctx, github.WorkClassBackground)
		}
		if ctx, err = s.github.BindWorkspaceGitHubHost(ctx, cfg.WorkspaceID, cfg.GitHubHost); err != nil {
			s.recordFailure(ctx, cfg, err)
			return nil, err
		}
	}
	files, err := s.fetch(ctx, cfg)
	if err != nil {
		s.recordFailure(ctx, cfg, err)
		return nil, err
	}
	var result *SyncResult
	apply := func(applyCtx context.Context) error {
		var applyErr error
		result, applyErr = s.apply(applyCtx, files)
		return applyErr
	}
	if useGitHub {
		err = s.github.CommitWorkspaceGitHubHost(ctx, cfg.WorkspaceID, apply)
	} else {
		err = apply(ctx)
	}
	if err != nil {
		s.recordFailure(ctx, cfg, err)
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
	paths := selectPromptFiles(entries)
	files := make([]fetchedFile, 0, len(paths))
	for _, path := range paths {
		content, err := get(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s: %w", path, err)
		}
		files = append(files, fetchedFile{path: path, content: content})
	}
	return files, nil
}

func (s *Service) apply(ctx context.Context, files []fetchedFile) (*SyncResult, error) {
	desired, frozen, warnings := parsePromptFiles(files)
	var texts []promptservice.PromptReferenceText
	if s.refs != nil {
		var err error
		if texts, err = s.refs.ListPromptReferenceTexts(ctx); err != nil {
			return nil, fmt.Errorf("failed to list workflow prompt references: %w", err)
		}
	}
	applied, err := s.prompts.ApplySyncedPrompts(ctx, promptservice.SyncApplyInput{
		Desired: desired, FrozenNames: frozen, ReferenceTexts: texts,
	})
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, applied.Warnings...)
	unresolved, err := s.unresolvedReferenceWarnings(ctx, desired)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, unresolved...)
	return &SyncResult{
		Created: applied.Created, Updated: applied.Updated, Deleted: applied.Deleted, Warnings: warnings,
		Unchanged: len(applied.Created)+len(applied.Updated)+len(applied.Deleted) == 0 && len(warnings) == 0,
	}, nil
}

func (s *Service) unresolvedReferenceWarnings(ctx context.Context, desired []promptservice.SyncedPromptFile) ([]string, error) {
	texts := make([]string, len(desired))
	for i, file := range desired {
		texts[i] = file.Content
	}
	missing, err := s.prompts.UnresolvedPromptReferences(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("failed to check prompt references: %w", err)
	}
	var warnings []string
	for i, names := range missing {
		for _, name := range names {
			warnings = append(warnings, fmt.Sprintf("%s: prompt %q references @%s, which is not a saved prompt", desired[i].Path, desired[i].Name, name))
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
