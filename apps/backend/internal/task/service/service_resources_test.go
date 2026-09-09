package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type failingWorkspaceBootstrapper struct {
	err error
}

type failingTransactionalWorkspaceSecretDeleter struct{}

func (failingTransactionalWorkspaceSecretDeleter) DeleteWorkspaceSecrets(context.Context, string) error {
	return errors.New("legacy cleanup should not be used")
}

func (failingTransactionalWorkspaceSecretDeleter) DeleteWorkspaceSecretsTx(context.Context, *sqlx.Tx, string) error {
	return errors.New("injected transactional secret cleanup failure")
}

func (b *failingWorkspaceBootstrapper) CreateWorkspaceWithKanban(
	context.Context,
	*models.Workspace,
) (*models.Workflow, error) {
	return nil, b.err
}

func TestService_CreateWorkspaceKanbanBootstrapPublishesParentBeforeWorkflow(t *testing.T) {
	svc, eventBus, _ := createTestService(t)

	_, err := svc.CreateWorkspace(context.Background(), &CreateWorkspaceRequest{
		Name:                    "Kanban",
		BootstrapKanbanWorkflow: true,
	})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	published := eventBus.GetPublishedEvents()
	if len(published) != 2 {
		t.Fatalf("published events = %d, want 2", len(published))
	}
	if published[0].Type != events.WorkspaceCreated || published[1].Type != events.WorkflowCreated {
		t.Fatalf("event order = %q, %q; want workspace.created, workflow.created", published[0].Type, published[1].Type)
	}
}

func TestService_CreateWorkspaceLogsKanbanBootstrapFailures(t *testing.T) {
	testCases := []struct {
		name         string
		bootstrapper WorkspaceBootstrapper
		wantErr      string
	}{
		{
			name:    "missing bootstrapper",
			wantErr: "workspace bootstrapper is not configured",
		},
		{
			name:         "bootstrap persistence failure",
			bootstrapper: &failingWorkspaceBootstrapper{err: errors.New("insert failed")},
			wantErr:      "insert failed",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := createTestService(t)
			svc.SetWorkspaceBootstrapper(tc.bootstrapper)
			core, logs := observer.New(zapcore.ErrorLevel)
			log, err := commonlogger.NewFromZap(zap.New(core))
			if err != nil {
				t.Fatalf("create logger: %v", err)
			}
			svc.logger = log

			_, err = svc.CreateWorkspace(context.Background(), &CreateWorkspaceRequest{
				Name:                    "Kanban",
				BootstrapKanbanWorkflow: true,
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("CreateWorkspace error = %v, want %q", err, tc.wantErr)
			}
			if logs.FilterMessage("failed to create workspace with Kanban bootstrap").Len() != 1 {
				t.Fatalf("bootstrap failure log count = %d, want 1", logs.Len())
			}
		})
	}
}

func TestService_CreateRepositoryCanonicalizesExplicitLocalPath(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	discoveryRoot := t.TempDir()
	svc.discoveryConfig.Roots = []string{discoveryRoot}
	repoPath := filepath.Join(t.TempDir(), "outside-repo")
	makeRepo(t, repoPath)
	uncleanPath := repoPath + string(os.PathSeparator) + ".." + string(os.PathSeparator) + filepath.Base(repoPath)

	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-1",
		Name:        "Outside Repo",
		SourceType:  sourceTypeLocal,
		LocalPath:   uncleanPath,
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	canonicalPath, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if created.LocalPath != canonicalPath {
		t.Fatalf("LocalPath = %q, want canonical path %q", created.LocalPath, canonicalPath)
	}
	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.LocalPath != canonicalPath {
		t.Fatalf("stored LocalPath = %q, want %q", stored.LocalPath, canonicalPath)
	}
}

func TestService_RepositorySecretBindingsValidateScopeAndReplaceAtomically(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-secret-bindings", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	secretDB := sqlx.NewDb(repo.DB(), "sqlite3")
	crypto, err := secrets.NewMasterKeyProvider(t.TempDir())
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	secretStore, closeSecrets, err := secrets.Provide(secretDB, secretDB, crypto)
	if err != nil {
		t.Fatalf("secret store: %v", err)
	}
	t.Cleanup(func() { _ = closeSecrets() })
	svc.SetSecretStore(secretStore)

	global := &secrets.SecretWithValue{Secret: secrets.Secret{Name: "global"}, Value: "global-value"}
	workspace := &secrets.SecretWithValue{Secret: secrets.Secret{
		Name: "workspace", Scope: secrets.ScopeWorkspace, WorkspaceID: "ws-secret-bindings",
	}, Value: "workspace-value"}
	if err := secretStore.Create(ctx, global); err != nil {
		t.Fatalf("create global secret: %v", err)
	}
	if err := secretStore.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace secret: %v", err)
	}

	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-secret-bindings",
		Name:        "app",
		SecretBindings: []RepositorySecretBindingInput{
			{Key: "GLOBAL_TOKEN", SecretID: global.ID},
			{Key: "WORKSPACE_TOKEN", SecretID: workspace.ID},
		},
	})
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if len(created.SecretBindings) != 2 {
		t.Fatalf("created bindings = %+v, want two", created.SecretBindings)
	}

	bad, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:    "ws-secret-bindings",
		Name:           "bad",
		SecretBindings: []RepositorySecretBindingInput{{Key: "BAD", SecretID: "missing-secret"}},
	})
	if err == nil || bad != nil || !errors.Is(err, ErrInvalidRepositorySettings) {
		t.Fatalf("missing binding result = %v, %+v; want invalid settings", err, bad)
	}

	clear := []RepositorySecretBindingInput{}
	updated, err := svc.UpdateRepository(ctx, created.ID, &UpdateRepositoryRequest{SecretBindings: &clear})
	if err != nil {
		t.Fatalf("clear bindings: %v", err)
	}
	if len(updated.SecretBindings) != 0 {
		t.Fatalf("bindings after clear = %+v, want empty", updated.SecretBindings)
	}
	loaded, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("get after clear: %v", err)
	}
	if len(loaded.SecretBindings) != 0 {
		t.Fatalf("persisted bindings after clear = %+v, want empty", loaded.SecretBindings)
	}
}

func TestService_ExecutorProfileSecretRefsRequireGlobalScope(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-profile-secrets", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	secretDB := sqlx.NewDb(repo.DB(), "sqlite3")
	crypto, err := secrets.NewMasterKeyProvider(t.TempDir())
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	secretStore, closeSecrets, err := secrets.Provide(secretDB, secretDB, crypto)
	if err != nil {
		t.Fatalf("secret store: %v", err)
	}
	t.Cleanup(func() { _ = closeSecrets() })
	svc.SetSecretStore(secretStore)

	workspaceSecret := &secrets.SecretWithValue{Secret: secrets.Secret{
		ID: "workspace-profile-secret", Scope: secrets.ScopeWorkspace, WorkspaceID: "ws-profile-secrets",
	}, Value: "workspace-value"}
	if err := secretStore.Create(ctx, workspaceSecret); err != nil {
		t.Fatalf("create workspace secret: %v", err)
	}
	if err := svc.validateGlobalProfileEnvRefs(ctx, []models.ProfileEnvVar{{Key: "TOKEN", SecretID: workspaceSecret.ID}}); err == nil {
		t.Fatal("workspace secret accepted in executor profile")
	}
}

func TestService_ExecutorProfileRejectsBackendOwnedSecretID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	secretDB := sqlx.NewDb(repo.DB(), "sqlite3")
	crypto, err := secrets.NewMasterKeyProvider(t.TempDir())
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	secretStore, closeSecrets, err := secrets.Provide(secretDB, secretDB, crypto)
	if err != nil {
		t.Fatalf("secret store: %v", err)
	}
	t.Cleanup(func() { _ = closeSecrets() })
	svc.SetSecretStore(secrets.NewUserVisibleStore(secretStore))

	internal := &secrets.SecretWithValue{Secret: secrets.Secret{
		ID: "github:user:workspace:user:access", Scope: secrets.ScopeGlobal,
	}, Value: "backend-owned"}
	if err := secretStore.Create(ctx, internal); err != nil {
		t.Fatalf("create internal secret: %v", err)
	}
	if err := svc.validateGlobalProfileEnvRefs(ctx, []models.ProfileEnvVar{{Key: "TOKEN", SecretID: internal.ID}}); err == nil {
		t.Fatal("backend-owned secret ID accepted in executor profile")
	}
}

// TestService_CreateRepositoryResolvesRemoteURLForSelfHostedGitLabOrigin
// covers the bug where a local repository cloned from a self-hosted GitLab
// instance (e.g. gitlab.example.com, not gitlab.com) got no RemoteURL
// persisted at creation time: resolveRepositoryProviderIdentity only tagged
// Provider/ProviderHost/ProviderOwner/ProviderName for the well-known
// github.com/gitlab.com hosts and never populated RemoteURL at all, so
// downstream GitLab MR-task-link identity matching had nothing to compare
// against and rejected every attach attempt for self-hosted repositories.
func TestService_CreateRepositoryResolvesRemoteURLForSelfHostedGitLabOrigin(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	repoPath := t.TempDir()
	makeRepo(t, repoPath)
	config := `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = git@gitlab.example.com:clients/socodevi/laravel/co-up.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`
	if err := os.WriteFile(filepath.Join(repoPath, ".git", "config"), []byte(config), 0o644); err != nil {
		t.Fatalf("write .git/config: %v", err)
	}

	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-1",
		Name:        "co-up",
		SourceType:  sourceTypeLocal,
		LocalPath:   repoPath,
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	// ResolveGitRemoteIdentity normalizes scp-style remotes to an ssh://
	// origin; the GitLab MR-link identity matcher already treats ssh:// and
	// https:// origins as equivalent for the same host+path.
	const wantRemoteURL = "ssh://gitlab.example.com/clients/socodevi/laravel/co-up"
	if created.RemoteURL != wantRemoteURL {
		t.Fatalf("RemoteURL = %q, want %q", created.RemoteURL, wantRemoteURL)
	}
	// Self-hosted hosts are deliberately left untagged: only github.com and
	// gitlab.com get a durable Provider/ProviderHost identity.
	if created.Provider != "" {
		t.Fatalf("Provider = %q, want empty for self-hosted host", created.Provider)
	}
	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.RemoteURL != wantRemoteURL {
		t.Fatalf("stored RemoteURL = %q, want %q", stored.RemoteURL, wantRemoteURL)
	}
}

// TestService_CreateRepositoryPrefersCanonicalCloneOriginForKnownProviders
// is a regression guard: for a local checkout whose origin is a well-known
// provider host (gitlab.com here), the RemoteURL fallback must resolve to
// canonicalCloneOrigin's exact ".git"-suffixed canonical form and not the
// broader ResolveGitRemoteIdentity-based form (which omits the suffix).
// Other code, including E2E test fixtures that rewrite Git's clone
// transport via an "insteadOf" config keyed on that exact canonical URL,
// depends on this byte-identical form to route the clone correctly.
func TestService_CreateRepositoryPrefersCanonicalCloneOriginForKnownProviders(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	repoPath := t.TempDir()
	makeRepo(t, repoPath)
	config := `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = https://gitlab.com/fixture/docker-second-source.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`
	if err := os.WriteFile(filepath.Join(repoPath, ".git", "config"), []byte(config), 0o644); err != nil {
		t.Fatalf("write .git/config: %v", err)
	}

	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:   "ws-1",
		Name:          "fixture/docker-second-source",
		SourceType:    sourceTypeLocal,
		LocalPath:     repoPath,
		Provider:      "gitlab",
		ProviderHost:  "https://gitlab.com",
		ProviderOwner: "fixture",
		ProviderName:  "docker-second-source",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	const wantRemoteURL = "https://gitlab.com/fixture/docker-second-source.git"
	if created.RemoteURL != wantRemoteURL {
		t.Fatalf("RemoteURL = %q, want %q", created.RemoteURL, wantRemoteURL)
	}
}

// TestService_CreateRepositoryDoesNotOverwriteExplicitRemoteURL ensures the
// new local-discovery RemoteURL fallback only fills a gap and never clobbers
// a caller-supplied RemoteURL.
func TestService_CreateRepositoryDoesNotOverwriteExplicitRemoteURL(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	repoPath := t.TempDir()
	makeRepo(t, repoPath)
	config := `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = git@gitlab.example.com:group/discovered-repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`
	if err := os.WriteFile(filepath.Join(repoPath, ".git", "config"), []byte(config), 0o644); err != nil {
		t.Fatalf("write .git/config: %v", err)
	}
	const explicitRemoteURL = "https://gitlab.example.com/group/explicit-repo"

	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-1",
		Name:        "explicit-repo",
		SourceType:  sourceTypeLocal,
		LocalPath:   repoPath,
		RemoteURL:   explicitRemoteURL,
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if created.RemoteURL != explicitRemoteURL {
		t.Fatalf("RemoteURL = %q, want unmodified explicit value %q", created.RemoteURL, explicitRemoteURL)
	}
}

func TestService_CreateRepositoryRejectsInvalidLocalPathWithoutPersistence(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	missingPath := filepath.Join(t.TempDir(), "missing")
	filePath := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(filePath, []byte("not a repository"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	type invalidPathCase struct {
		name string
		path string
	}
	invalidPaths := []invalidPathCase{
		{name: "missing", path: missingPath},
		{name: "file", path: filePath},
		{name: "plain directory", path: t.TempDir()},
	}
	metadataOwner := filepath.Join(t.TempDir(), "metadata-owner")
	makeRepo(t, metadataOwner)
	forgedWorktree := filepath.Join(t.TempDir(), "forged-worktree")
	if err := os.MkdirAll(forgedWorktree, 0o755); err != nil {
		t.Fatalf("MkdirAll forged worktree: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(forgedWorktree, ".git"),
		[]byte("gitdir: "+filepath.Join(metadataOwner, ".git")+"\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile forged .git pointer: %v", err)
	}
	invalidPaths = append(invalidPaths, invalidPathCase{name: "forged gitdir pointer", path: forgedWorktree})
	forgedCommonDir := filepath.Join(t.TempDir(), "forged-common-dir")
	makeRepo(t, forgedCommonDir)
	if err := os.WriteFile(
		filepath.Join(forgedCommonDir, ".git", "commondir"),
		[]byte(filepath.Join(metadataOwner, ".git")+"\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile forged commondir: %v", err)
	}
	invalidPaths = append(invalidPaths, invalidPathCase{name: "forged common directory", path: forgedCommonDir})
	loopPath := filepath.Join(t.TempDir(), "loop")
	if err := os.Symlink(loopPath, loopPath); err == nil {
		invalidPaths = append(invalidPaths, invalidPathCase{name: "symlink loop", path: loopPath})
	}

	for _, testCase := range invalidPaths {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
				WorkspaceID: "ws-1",
				Name:        "Invalid Repo",
				SourceType:  sourceTypeLocal,
				LocalPath:   testCase.path,
			})
			if !errors.Is(err, ErrInvalidRepositorySettings) || !errors.Is(err, ErrInvalidRepositoryPath) {
				t.Fatalf("CreateRepository error = %v, want typed invalid path error", err)
			}
		})
	}
	repositories, err := repo.ListRepositories(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 0 {
		t.Fatalf("invalid repository was persisted: %+v", repositories)
	}
}

func TestService_UpdateRepositoryRejectsInvalidLocalPathWithoutPersistence(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-1",
		Name:        "Provider Repo",
		SourceType:  sourceTypeProvider,
		Provider:    "github",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	invalidPath := t.TempDir()
	_, err = svc.UpdateRepository(ctx, created.ID, &UpdateRepositoryRequest{LocalPath: &invalidPath})
	if !errors.Is(err, ErrInvalidRepositorySettings) {
		t.Fatalf("UpdateRepository error = %v, want ErrInvalidRepositorySettings", err)
	}
	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.LocalPath != "" {
		t.Fatalf("invalid LocalPath persisted as %q", stored.LocalPath)
	}
}

func TestService_UpdateRepositoryCanonicalizesExplicitLocalPath(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-1",
		Name:        "Provider Repo",
		SourceType:  sourceTypeProvider,
		Provider:    "github",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	repoPath := filepath.Join(t.TempDir(), "outside-repo")
	makeRepo(t, repoPath)
	uncleanPath := repoPath + string(os.PathSeparator) + ".." + string(os.PathSeparator) + filepath.Base(repoPath)
	updated, err := svc.UpdateRepository(ctx, created.ID, &UpdateRepositoryRequest{LocalPath: &uncleanPath})
	if err != nil {
		t.Fatalf("UpdateRepository: %v", err)
	}
	canonicalPath, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if updated.LocalPath != canonicalPath {
		t.Fatalf("LocalPath = %q, want canonical path %q", updated.LocalPath, canonicalPath)
	}
}

func TestService_CreateRepositoryAllowsPathlessProvider(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-1",
		Name:        "owner/repo",
		SourceType:  sourceTypeProvider,
		Provider:    "github",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if created.LocalPath != "" {
		t.Fatalf("LocalPath = %q, want empty", created.LocalPath)
	}
	if created.ProviderHost != "https://github.com" {
		t.Fatalf("ProviderHost = %q, want GitHub default", created.ProviderHost)
	}
}

func TestService_FindOrCreateRepositoryMatchesGitHubRepositoryWithoutExplicitHost(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	repoPath := filepath.Join(t.TempDir(), "github-repo")
	makeRepo(t, repoPath)
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:   "ws-1",
		Name:          "owner/repo",
		LocalPath:     repoPath,
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	resolved, wasCreated, err := svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID:   "ws-1",
		Provider:      "github",
		ProviderHost:  "https://github.com",
		ProviderOwner: "owner",
		ProviderName:  "repo",
	})
	if err != nil {
		t.Fatalf("FindOrCreateRepository: %v", err)
	}
	if wasCreated || resolved.ID != created.ID {
		t.Fatalf("resolved repository = %q (created=%t), want existing %q", resolved.ID, wasCreated, created.ID)
	}
}

func TestService_FindOrCreateRepositoryNormalizesProviderIdentityWhitespace(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, wasCreated, err := svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID: "ws-1", Provider: "custom-provider", ProviderHost: "https://forge.example.test",
		ProviderRepoID: "repo-99", ProviderOwner: "TEAM", ProviderName: "widgets",
		RemoteURL: "https://forge.example.test/scm/TEAM/widgets.git",
	})
	if err != nil || !wasCreated {
		t.Fatalf("initial FindOrCreateRepository() = %+v, %t, %v", created, wasCreated, err)
	}

	resolved, duplicateCreated, err := svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID: " ws-1 ", Provider: " custom-provider ", ProviderHost: " https://forge.example.test/context ",
		ProviderRepoID: " repo-99 ", ProviderOwner: " TEAM ", ProviderName: " widgets ",
		RemoteURL: " https://forge.example.test/scm/TEAM/widgets.git ",
	})
	if err != nil {
		t.Fatalf("second FindOrCreateRepository() error = %v", err)
	}
	if duplicateCreated || resolved.ID != created.ID {
		t.Fatalf("second repository = %q (created=%t), want existing %q", resolved.ID, duplicateCreated, created.ID)
	}
}

func TestService_FindOrCreateRepositoryRejectsInvalidLocalPathBackfill(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:   "ws-1",
		Name:          "owner/repo",
		SourceType:    sourceTypeProvider,
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	invalidPath := t.TempDir()
	_, _, err = svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID:   "ws-1",
		Provider:      "github",
		ProviderOwner: "owner",
		ProviderName:  "repo",
		LocalPath:     invalidPath,
	})
	if !errors.Is(err, ErrInvalidRepositorySettings) {
		t.Fatalf("FindOrCreateRepository error = %v, want ErrInvalidRepositorySettings", err)
	}
	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.LocalPath != "" {
		t.Fatalf("invalid LocalPath backfill persisted as %q", stored.LocalPath)
	}
}

// errWorkspaceRepo is a WorkspaceRepository that always returns an error from
// ListWorkspaces. Used to exercise the DB-error path of GetOfficeWorkflowIDs.
type errWorkspaceRepo struct {
	// Membership is not exercised by this fake; the embedded default
	// reports no membership, which is the narrower answer.
	repository.UnsupportedWorkspaceMembers
	// embed the real repo for all methods except ListWorkspaces.
	WorkspaceRepositoryStub
}

type blockingWorktreeCleanup struct {
	release   chan struct{}
	active    atomic.Int32
	maxActive atomic.Int32
}

func (c *blockingWorktreeCleanup) OnTaskDeleted(context.Context, string) error {
	return nil
}

func (c *blockingWorktreeCleanup) GetAllByTaskID(context.Context, string) ([]*worktree.Worktree, error) {
	return nil, nil
}

func (c *blockingWorktreeCleanup) CleanupWorktrees(ctx context.Context, _ []*worktree.Worktree) error {
	active := c.active.Add(1)
	updateMaxActive(&c.maxActive, active)
	defer c.active.Add(-1)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.release:
		return nil
	}
}

func TestWorkspaceDeleteDurableCleanupSignalsOwnedWorker(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	snapshot, err := json.Marshal(taskResourceCleanupSnapshot{
		Worktrees: []*worktree.Worktree{{ID: "workspace-delete-worktree", TaskID: "workspace-delete-task"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "workspace-delete-job", OperationID: "workspace_delete:workspace-delete-task",
		TaskID: "workspace-delete-task", Trigger: models.TaskResourceCleanupTriggerWorkspaceDelete,
		State: models.TaskResourceCleanupStatePrepared, ResourceSnapshot: string(snapshot),
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	barrier := newCancellableCleanupBarrier()
	taskSvc.SetWorktreeCleanup(barrier)
	wake := make(chan struct{}, 1)
	taskSvc.cleanupWorkerMu.Lock()
	taskSvc.cleanupWorkerWake = wake
	taskSvc.cleanupWorkerMu.Unlock()
	done := make(chan struct{})
	go func() {
		taskSvc.runWorkspaceDeleteTaskCleanup(workspaceDeleteTaskCleanup{cleanupJob: job})
		close(done)
	}()
	select {
	case <-done:
	case <-barrier.started:
		close(barrier.release)
		select {
		case <-barrier.stopped:
		case <-time.After(time.Second):
			t.Fatal("synchronous workspace cleanup did not stop after release")
		}
		t.Fatal("workspace deletion processed durable cleanup synchronously")
	case <-time.After(time.Second):
		close(barrier.release)
		t.Fatal("workspace deletion did not return")
	}
	select {
	case <-wake:
	default:
		t.Fatal("workspace deletion did not wake owned cleanup worker")
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	if got.State != models.TaskResourceCleanupStatePending {
		t.Fatalf("cleanup state = %q, want pending", got.State)
	}
}

func updateMaxActive(maxActive *atomic.Int32, value int32) {
	for {
		current := maxActive.Load()
		if value <= current || maxActive.CompareAndSwap(current, value) {
			return
		}
	}
}

func waitForActiveCleanups(t *testing.T, cleanup *blockingWorktreeCleanup, want int32) {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if cleanup.active.Load() >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("active cleanups = %d, want at least %d", cleanup.active.Load(), want)
		case <-ticker.C:
		}
	}
}

// WorkspaceRepositoryStub satisfies the full WorkspaceRepository interface
// with no-op / panic stubs. Only methods under test need real implementations.
type WorkspaceRepositoryStub struct{}

func (WorkspaceRepositoryStub) CreateWorkspace(_ context.Context, _ *models.Workspace) error {
	panic("not implemented")
}
func (WorkspaceRepositoryStub) GetWorkspace(_ context.Context, _ string) (*models.Workspace, error) {
	panic("not implemented")
}
func (WorkspaceRepositoryStub) UpdateWorkspace(_ context.Context, _ *models.Workspace) error {
	panic("not implemented")
}
func (WorkspaceRepositoryStub) DeleteWorkspace(_ context.Context, _ string) error {
	panic("not implemented")
}
func (WorkspaceRepositoryStub) DeleteWorkspaceCascade(_ context.Context, _ string) ([]*models.Task, []*models.Workflow, error) {
	panic("not implemented")
}
func (WorkspaceRepositoryStub) DeleteWorkspaceCascadeWithName(_ context.Context, _, _ string) ([]*models.Task, []*models.Workflow, error) {
	panic("not implemented")
}
func (WorkspaceRepositoryStub) ListWorkspaces(_ context.Context) ([]*models.Workspace, error) {
	panic("not implemented")
}

type renameBeforeConfirmedDeleteRepo struct {
	repository.WorkspaceRepository
}

func (r renameBeforeConfirmedDeleteRepo) DeleteWorkspaceCascadeWithName(
	ctx context.Context,
	id, name string,
) ([]*models.Task, []*models.Workflow, error) {
	workspace, err := r.GetWorkspace(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	workspace.Name = "Renamed"
	if err := r.UpdateWorkspace(ctx, workspace); err != nil {
		return nil, nil, err
	}
	return r.WorkspaceRepository.DeleteWorkspaceCascadeWithName(ctx, id, name)
}

type createDuringConfirmedDeleteRepo struct {
	repository.WorkspaceRepository
	tasks     repository.TaskRepository
	workflows repository.WorkflowRepository
}

func (r createDuringConfirmedDeleteRepo) DeleteWorkspaceCascadeWithName(
	ctx context.Context,
	id, name string,
) ([]*models.Task, []*models.Workflow, error) {
	if err := r.workflows.CreateWorkflow(ctx, &models.Workflow{
		ID:          "wf-raced",
		WorkspaceID: id,
		Name:        "Raced",
	}); err != nil {
		return nil, nil, err
	}
	if err := r.tasks.CreateTask(ctx, &models.Task{
		ID:             "task-raced",
		WorkspaceID:    id,
		WorkflowID:     "wf-raced",
		WorkflowStepID: "step-raced",
		Title:          "Raced task",
	}); err != nil {
		return nil, nil, err
	}
	return r.WorkspaceRepository.DeleteWorkspaceCascadeWithName(ctx, id, name)
}

type failingListTaskSessionsRepo struct {
	repository.SessionRepository
	err error
}

func (r failingListTaskSessionsRepo) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	return nil, r.err
}

func (e errWorkspaceRepo) ListWorkspaces(_ context.Context) ([]*models.Workspace, error) {
	return nil, errors.New("db unavailable")
}

func TestService_GetOfficeWorkflowIDs_Empty(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	// No workspaces → empty result.
	ids := svc.GetOfficeWorkflowIDs(ctx)
	if len(ids) != 0 {
		t.Errorf("expected empty map, got %v", ids)
	}

	// Workspace with no office_workflow_id → still excluded.
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-no-office", Name: "No Office"})
	ids = svc.GetOfficeWorkflowIDs(ctx)
	if len(ids) != 0 {
		t.Errorf("expected empty map for workspace without office_workflow_id, got %v", ids)
	}
}

func TestService_GetOfficeWorkflowIDs_SingleWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{
		ID:               "ws-1",
		Name:             "WS 1",
		OfficeWorkflowID: "wf-office-1",
	})

	ids := svc.GetOfficeWorkflowIDs(ctx)
	if _, ok := ids["wf-office-1"]; !ok {
		t.Errorf("expected wf-office-1 in result, got %v", ids)
	}
	if len(ids) != 1 {
		t.Errorf("expected exactly 1 id, got %d", len(ids))
	}
}

func TestService_GetOfficeWorkflowIDs_MultipleWorkflows(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	workspaces := []struct {
		id   string
		wfID string
	}{
		{"ws-a", "wf-office-a"},
		{"ws-b", "wf-office-b"},
		{"ws-c", ""},
	}
	for _, ws := range workspaces {
		_ = repo.CreateWorkspace(ctx, &models.Workspace{
			ID:               ws.id,
			Name:             ws.id,
			OfficeWorkflowID: ws.wfID,
		})
	}

	ids := svc.GetOfficeWorkflowIDs(ctx)
	if _, ok := ids["wf-office-a"]; !ok {
		t.Errorf("expected wf-office-a")
	}
	if _, ok := ids["wf-office-b"]; !ok {
		t.Errorf("expected wf-office-b")
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 ids (ws-c has no office wf), got %d: %v", len(ids), ids)
	}
}

func TestService_GetOfficeWorkflowIDs_DBError(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	// Seed a workspace first so we know the real repo would return something.
	_ = repo.CreateWorkspace(ctx, &models.Workspace{
		ID:               "ws-ok",
		Name:             "OK",
		OfficeWorkflowID: "wf-ok",
	})

	// Replace the workspace repo with one that always errors.
	svc.workspaces = errWorkspaceRepo{}

	ids := svc.GetOfficeWorkflowIDs(ctx)
	if ids != nil {
		t.Errorf("expected nil on DB error, got %v", ids)
	}
}

func TestService_DeleteWorkspaceDeletesWorkspaceOwnedTasksAndWorkflows(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-delete", Name: "Delete Me"})
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-keep", Name: "Keep Me"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-delete", WorkspaceID: "ws-delete", Name: "Doomed"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-keep", WorkspaceID: "ws-keep", Name: "Keep"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-delete",
		WorkspaceID:    "ws-delete",
		WorkflowID:     "wf-delete",
		WorkflowStepID: "step-delete",
		Title:          "Delete task",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := svc.DeleteWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	if _, err := repo.GetWorkspace(ctx, "ws-delete"); err == nil {
		t.Fatalf("workspace should be deleted")
	}
	if _, err := repo.GetTask(ctx, "task-delete"); err == nil {
		t.Fatalf("workspace task should be deleted")
	}
	workflows, err := repo.ListWorkflows(ctx, "ws-delete", true)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(workflows) != 0 {
		t.Fatalf("workspace workflows should be deleted, got %d", len(workflows))
	}
	if _, err := repo.GetWorkflow(ctx, "wf-keep"); err != nil {
		t.Fatalf("unrelated workflow should remain: %v", err)
	}
}

func TestService_DeleteWorkspaceRollsBackCascadeWhenSecretCleanupFails(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	svc.SetWorkspaceSecretDeleter(failingTransactionalWorkspaceSecretDeleter{})
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-delete", Name: "Delete Me"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-delete", WorkspaceID: "ws-delete", Name: "Doomed"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-delete", WorkspaceID: "ws-delete", WorkflowID: "wf-delete", WorkflowStepID: "step-delete", Title: "Delete task",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	eventBus.ClearEvents()

	if err := svc.DeleteWorkspace(ctx, "ws-delete"); err == nil {
		t.Fatal("DeleteWorkspace succeeded, want secret cleanup failure")
	}
	if _, err := repo.GetWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("workspace was deleted after cleanup failure: %v", err)
	}
	if _, err := repo.GetTask(ctx, "task-delete"); err != nil {
		t.Fatalf("task was deleted after cleanup failure: %v", err)
	}
	if events := eventBus.GetPublishedEvents(); len(events) != 0 {
		t.Fatalf("events after rolled-back delete = %#v, want none", events)
	}
}

func TestService_DeleteWorkspacePublishesChildEventsAndCleansResources(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))
	cleanup := &recordingWorktreeCleanup{
		worktrees: []*worktree.Worktree{{ID: "wt-delete", TaskID: "task-delete"}},
	}
	svc.SetWorktreeCleanup(cleanup)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-delete", Name: "Delete Me"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-delete", WorkspaceID: "ws-delete", Name: "Doomed"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-delete",
		WorkspaceID:    "ws-delete",
		WorkflowID:     "wf-delete",
		WorkflowStepID: "step-delete",
		Title:          "Delete task",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	eventBus.ClearEvents()

	if err := svc.DeleteWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	waitForCleanupDone(t, svc)

	eventCounts := make(map[string]int)
	for _, event := range eventBus.GetPublishedEvents() {
		eventCounts[event.Type]++
	}
	if eventCounts[events.TaskDeleted] != 1 {
		t.Fatalf("task deleted events = %d, want 1", eventCounts[events.TaskDeleted])
	}
	if eventCounts[events.WorkflowDeleted] != 1 {
		t.Fatalf("workflow deleted events = %d, want 1", eventCounts[events.WorkflowDeleted])
	}
	if eventCounts[events.WorkspaceDeleted] != 1 {
		t.Fatalf("workspace deleted events = %d, want 1", eventCounts[events.WorkspaceDeleted])
	}
	cleanedIDs := cleanup.cleanedIDs()
	if len(cleanedIDs) != 1 || cleanedIDs[0] != "wt-delete" {
		t.Fatalf("cleaned worktrees = %#v, want wt-delete", cleanedIDs)
	}
}

func TestService_DeleteWorkspaceStopsBeforeCascadeWhenSessionInventoryFails(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-delete", Name: "Delete Me"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-delete", WorkspaceID: "ws-delete", Name: "Doomed"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-delete",
		WorkspaceID:    "ws-delete",
		WorkflowID:     "wf-delete",
		WorkflowStepID: "step-delete",
		Title:          "Delete task",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	listErr := errors.New("session inventory unavailable")
	svc.sessions = failingListTaskSessionsRepo{SessionRepository: repo, err: listErr}

	err := svc.DeleteWorkspace(ctx, "ws-delete")
	if !errors.Is(err, listErr) {
		t.Fatalf("expected session inventory error, got %v", err)
	}
	if _, err := repo.GetWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("workspace should remain: %v", err)
	}
	if _, err := repo.GetTask(ctx, "task-delete"); err != nil {
		t.Fatalf("workspace task should remain: %v", err)
	}
	if _, err := repo.GetWorkflow(ctx, "wf-delete"); err != nil {
		t.Fatalf("workspace workflow should remain: %v", err)
	}
}

func TestService_RunWorkspaceDeleteTaskCleanupsCapsConcurrency(t *testing.T) {
	svc, _, _ := createTestService(t)
	taskCount := workspaceDeleteCleanupConcurrency + 3
	svc.setCleanupDoneForTestHook(make(chan struct{}, taskCount))
	cleanup := &blockingWorktreeCleanup{release: make(chan struct{})}
	svc.SetWorktreeCleanup(cleanup)

	cleanups := make([]workspaceDeleteTaskCleanup, 0, taskCount)
	deletedTasks := make([]*models.Task, 0, taskCount)
	for i := 0; i < taskCount; i++ {
		taskID := fmt.Sprintf("task-%02d", i)
		task := &models.Task{ID: taskID}
		cleanups = append(cleanups, workspaceDeleteTaskCleanup{
			task:      task,
			worktrees: []*worktree.Worktree{{ID: "wt-" + taskID, TaskID: taskID}},
		})
		deletedTasks = append(deletedTasks, task)
	}

	svc.runWorkspaceDeleteTaskCleanups(cleanups, deletedTasks)
	waitForActiveCleanups(t, cleanup, workspaceDeleteCleanupConcurrency)
	close(cleanup.release)
	for i := 0; i < taskCount; i++ {
		waitForCleanupDone(t, svc)
	}

	if got := cleanup.maxActive.Load(); got > workspaceDeleteCleanupConcurrency {
		t.Fatalf("max active cleanups = %d, want <= %d", got, workspaceDeleteCleanupConcurrency)
	}
}

func TestService_DeleteWorkspaceWithConfirmNameDeletesWhenNameMatches(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-delete", Name: "Delete Me"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-delete", WorkspaceID: "ws-delete", Name: "Doomed"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-delete",
		WorkspaceID:    "ws-delete",
		WorkflowID:     "wf-delete",
		WorkflowStepID: "step-delete",
		Title:          "Delete task",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := svc.DeleteWorkspaceWithConfirmName(ctx, "ws-delete", "Delete Me"); err != nil {
		t.Fatalf("DeleteWorkspaceWithConfirmName: %v", err)
	}

	if _, err := repo.GetWorkspace(ctx, "ws-delete"); err == nil {
		t.Fatalf("workspace should be deleted")
	}
	if _, err := repo.GetTask(ctx, "task-delete"); err == nil {
		t.Fatalf("workspace task should be deleted")
	}
	workflows, err := repo.ListWorkflows(ctx, "ws-delete", true)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(workflows) != 0 {
		t.Fatalf("workspace workflows should be deleted, got %d", len(workflows))
	}
}

func TestService_DeleteWorkspaceWithConfirmNamePublishesChildEventsAndCleansResources(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))
	cleanup := &recordingWorktreeCleanup{
		worktrees: []*worktree.Worktree{{ID: "wt-delete", TaskID: "task-delete"}},
	}
	svc.SetWorktreeCleanup(cleanup)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-delete", Name: "Delete Me"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-delete", WorkspaceID: "ws-delete", Name: "Doomed"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-delete",
		WorkspaceID:    "ws-delete",
		WorkflowID:     "wf-delete",
		WorkflowStepID: "step-delete",
		Title:          "Delete task",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	eventBus.ClearEvents()

	if err := svc.DeleteWorkspaceWithConfirmName(ctx, "ws-delete", "Delete Me"); err != nil {
		t.Fatalf("DeleteWorkspaceWithConfirmName: %v", err)
	}
	waitForCleanupDone(t, svc)

	eventCounts := make(map[string]int)
	for _, event := range eventBus.GetPublishedEvents() {
		eventCounts[event.Type]++
	}
	if eventCounts[events.TaskDeleted] != 1 {
		t.Fatalf("task deleted events = %d, want 1", eventCounts[events.TaskDeleted])
	}
	if eventCounts[events.WorkflowDeleted] != 1 {
		t.Fatalf("workflow deleted events = %d, want 1", eventCounts[events.WorkflowDeleted])
	}
	if eventCounts[events.WorkspaceDeleted] != 1 {
		t.Fatalf("workspace deleted events = %d, want 1", eventCounts[events.WorkspaceDeleted])
	}
	cleanedIDs := cleanup.cleanedIDs()
	if len(cleanedIDs) != 1 || cleanedIDs[0] != "wt-delete" {
		t.Fatalf("cleaned worktrees = %#v, want wt-delete", cleanedIDs)
	}
}

func TestService_DeleteWorkspaceWithConfirmNamePublishesEventsForAllCascadeDeletedChildren(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	svc.setCleanupDoneForTestHook(make(chan struct{}, 2))
	cleanup := &recordingWorktreeCleanup{
		worktreesByTaskID: map[string][]*worktree.Worktree{
			"task-delete": {{ID: "wt-delete", TaskID: "task-delete"}},
			"task-raced":  {{ID: "wt-raced", TaskID: "task-raced"}},
		},
	}
	svc.SetWorktreeCleanup(cleanup)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-delete", Name: "Delete Me"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-delete", WorkspaceID: "ws-delete", Name: "Doomed"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-delete",
		WorkspaceID:    "ws-delete",
		WorkflowID:     "wf-delete",
		WorkflowStepID: "step-delete",
		Title:          "Delete task",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	svc.workspaces = createDuringConfirmedDeleteRepo{
		WorkspaceRepository: repo,
		tasks:               repo,
		workflows:           repo,
	}
	eventBus.ClearEvents()

	if err := svc.DeleteWorkspaceWithConfirmName(ctx, "ws-delete", "Delete Me"); err != nil {
		t.Fatalf("DeleteWorkspaceWithConfirmName: %v", err)
	}
	waitForCleanupDone(t, svc)
	waitForCleanupDone(t, svc)

	// This covers event publication from the repository cascade return value.
	// Runtime cleanup is prepared before the cascade and topped up from the
	// cascade return value for children that appear after that first snapshot.
	eventCounts := make(map[string]int)
	for _, event := range eventBus.GetPublishedEvents() {
		eventCounts[event.Type]++
	}
	if eventCounts[events.TaskDeleted] != 2 {
		t.Fatalf("task deleted events = %d, want 2", eventCounts[events.TaskDeleted])
	}
	if eventCounts[events.WorkflowDeleted] != 2 {
		t.Fatalf("workflow deleted events = %d, want 2", eventCounts[events.WorkflowDeleted])
	}
	if _, err := repo.GetTask(ctx, "task-raced"); err == nil {
		t.Fatalf("late-created task should be deleted")
	}
	if _, err := repo.GetWorkflow(ctx, "wf-raced"); err == nil {
		t.Fatalf("late-created workflow should be deleted")
	}
	cleaned := make(map[string]bool)
	for _, id := range cleanup.cleanedIDs() {
		cleaned[id] = true
	}
	if len(cleaned) != 2 || !cleaned["wt-delete"] || !cleaned["wt-raced"] {
		t.Fatalf("cleaned worktrees = %#v, want wt-delete and wt-raced", cleaned)
	}
}

func TestService_DeleteWorkspaceWithConfirmNameRejectsMismatchedName(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-delete", Name: "Delete Me"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-delete", WorkspaceID: "ws-delete", Name: "Doomed"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-delete",
		WorkspaceID:    "ws-delete",
		WorkflowID:     "wf-delete",
		WorkflowStepID: "step-delete",
		Title:          "Delete task",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	err := svc.DeleteWorkspaceWithConfirmName(ctx, "ws-delete", "Wrong")
	if !errors.Is(err, ErrWorkspaceConfirmNameMismatch) {
		t.Fatalf("expected ErrWorkspaceConfirmNameMismatch, got %v", err)
	}

	if _, err := repo.GetWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("workspace should remain: %v", err)
	}
	if _, err := repo.GetTask(ctx, "task-delete"); err != nil {
		t.Fatalf("workspace task should remain: %v", err)
	}
	if _, err := repo.GetWorkflow(ctx, "wf-delete"); err != nil {
		t.Fatalf("workspace workflow should remain: %v", err)
	}
}

func TestService_DeleteWorkspaceWithConfirmNameReturnsMissingWorkspaceError(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	err := svc.DeleteWorkspaceWithConfirmName(ctx, "ws-missing", "Missing")
	if err == nil {
		t.Fatalf("expected missing workspace error")
	}
	if errors.Is(err, ErrWorkspaceConfirmNameMismatch) {
		t.Fatalf("expected missing workspace error, got confirm-name mismatch")
	}
	if !errors.Is(err, repository.ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "workspace not found") {
		t.Fatalf("expected workspace not found error, got %v", err)
	}
}

func TestService_DeleteWorkspaceWithConfirmNameRejectsFinalNameMismatch(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-delete", Name: "Delete Me"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-delete", WorkspaceID: "ws-delete", Name: "Doomed"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-delete",
		WorkspaceID:    "ws-delete",
		WorkflowID:     "wf-delete",
		WorkflowStepID: "step-delete",
		Title:          "Delete task",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	svc.workspaces = renameBeforeConfirmedDeleteRepo{WorkspaceRepository: repo}

	err := svc.DeleteWorkspaceWithConfirmName(ctx, "ws-delete", "Delete Me")
	if !errors.Is(err, ErrWorkspaceConfirmNameMismatch) {
		t.Fatalf("expected ErrWorkspaceConfirmNameMismatch, got %v", err)
	}
	workspace, err := repo.GetWorkspace(ctx, "ws-delete")
	if err != nil {
		t.Fatalf("workspace should remain: %v", err)
	}
	if workspace.Name != "Renamed" {
		t.Fatalf("workspace should keep concurrent rename, got %q", workspace.Name)
	}
	if _, err := repo.GetTask(ctx, "task-delete"); err != nil {
		t.Fatalf("workspace task should remain: %v", err)
	}
	if _, err := repo.GetWorkflow(ctx, "wf-delete"); err != nil {
		t.Fatalf("workspace workflow should remain: %v", err)
	}
}

// TestService_DeleteWorkflow_ArchivesChildTasks verifies the cascade fix for
// issue #1279: workflow deletion archives any active child tasks instead of
// leaving them with a dangling workflow_id (tasks.workflow_id has no FK, so
// SQLite cannot CASCADE for us).
func TestService_DeleteWorkflow_ArchivesChildTasks(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-doomed", WorkspaceID: "ws-1", Name: "Doomed"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-keep", WorkspaceID: "ws-1", Name: "Keep"})

	tasks := []*models.Task{
		{ID: "task-a", WorkspaceID: "ws-1", WorkflowID: "wf-doomed", WorkflowStepID: "step-1", Title: "A"},
		{ID: "task-b", WorkspaceID: "ws-1", WorkflowID: "wf-doomed", WorkflowStepID: "step-1", Title: "B"},
		{ID: "task-other", WorkspaceID: "ws-1", WorkflowID: "wf-keep", WorkflowStepID: "step-1", Title: "Other"},
	}
	for _, task := range tasks {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}

	if err := svc.DeleteWorkflow(ctx, "wf-doomed"); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}

	if _, err := svc.workflows.GetWorkflow(ctx, "wf-doomed"); err == nil {
		t.Fatalf("expected workflow to be deleted")
	}

	for _, id := range []string{"task-a", "task-b"} {
		got, err := repo.GetTask(ctx, id)
		if err != nil {
			t.Fatalf("GetTask %s after cascade: %v", id, err)
		}
		if got.ArchivedAt == nil {
			t.Errorf("task %s: expected archived_at to be set, got nil", id)
		}
	}

	other, err := repo.GetTask(ctx, "task-other")
	if err != nil {
		t.Fatalf("GetTask task-other: %v", err)
	}
	if other.ArchivedAt != nil {
		t.Errorf("task in unrelated workflow should not be archived, got archived_at=%v", other.ArchivedAt)
	}
}

func TestService_DeleteWorkflow_IgnoresLegacyTaskFromAnotherWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	for _, workspace := range []*models.Workspace{
		{ID: "ws-victim", Name: "Victim", OwnerID: "user-victim"},
		{ID: "ws-foreign", Name: "Foreign", OwnerID: "user-foreign"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace %s: %v", workspace.ID, err)
		}
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-victim", WorkspaceID: "ws-victim", Name: "Victim workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	for _, task := range []*models.Task{
		{
			ID: "task-valid", WorkspaceID: "ws-victim", WorkflowID: "wf-victim",
			WorkflowStepID: "step-1", Title: "Valid",
		},
		{
			ID: "task-legacy-foreign", WorkspaceID: "ws-foreign", WorkflowID: "wf-victim",
			WorkflowStepID: "step-1", Title: "Legacy foreign",
		},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}

	if err := svc.DeleteWorkflow(ctxAs("user-victim"), "wf-victim"); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}

	if _, err := repo.GetWorkflow(ctx, "wf-victim"); err == nil {
		t.Fatal("victim workflow still exists after deletion")
	}
	valid, err := repo.GetTask(ctx, "task-valid")
	if err != nil {
		t.Fatalf("GetTask valid: %v", err)
	}
	if valid.ArchivedAt == nil {
		t.Fatal("valid same-workspace task was not archived")
	}
	foreign, err := repo.GetTask(ctx, "task-legacy-foreign")
	if err != nil {
		t.Fatalf("GetTask legacy foreign: %v", err)
	}
	if foreign.ArchivedAt != nil {
		t.Fatalf("legacy foreign task was mutated: archived_at=%v", foreign.ArchivedAt)
	}
	if foreign.WorkspaceID != "ws-foreign" || foreign.WorkflowID != "wf-victim" {
		t.Fatalf("legacy foreign task identity changed: %+v", foreign)
	}
}

// leakyListTaskRepo wraps the real TaskRepository and injects extra tasks
// into ListTasks results, simulating a TOCTOU race where a task is archived
// between the snapshot and the cascade loop.
type leakyListTaskRepo struct {
	repository.TaskRepository
	extra []*models.Task
}

func (l leakyListTaskRepo) ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error) {
	real, err := l.TaskRepository.ListTasks(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	return append(real, l.extra...), nil
}

// TestService_DeleteWorkflow_SkipsConcurrentlyArchivedTask covers the
// TOCTOU race window between Service.tasks.ListTasks and Service.ArchiveTask:
// if a task is archived by another caller in that window, ArchiveTask
// returns ErrTaskAlreadyArchived and the cascade must continue rather than
// abort, so the workflow row still gets deleted.
func TestService_DeleteWorkflow_SkipsConcurrentlyArchivedTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-doomed", WorkspaceID: "ws-1", Name: "Doomed"})

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-live", WorkspaceID: "ws-1", WorkflowID: "wf-doomed", WorkflowStepID: "step-1", Title: "Live",
	}); err != nil {
		t.Fatalf("CreateTask live: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-raced", WorkspaceID: "ws-1", WorkflowID: "wf-doomed", WorkflowStepID: "step-1", Title: "Raced",
	}); err != nil {
		t.Fatalf("CreateTask raced: %v", err)
	}
	if err := repo.ArchiveTask(ctx, "task-raced"); err != nil {
		t.Fatalf("pre-archive raced task: %v", err)
	}

	raced, err := repo.GetTask(ctx, "task-raced")
	if err != nil {
		t.Fatalf("GetTask raced: %v", err)
	}
	svc.tasks = leakyListTaskRepo{TaskRepository: repo, extra: []*models.Task{raced}}

	if err := svc.DeleteWorkflow(ctx, "wf-doomed"); err != nil {
		t.Fatalf("DeleteWorkflow should swallow ErrTaskAlreadyArchived: %v", err)
	}

	if _, err := svc.workflows.GetWorkflow(ctx, "wf-doomed"); err == nil {
		t.Fatalf("expected workflow to be deleted despite race")
	}

	got, err := repo.GetTask(ctx, "task-live")
	if err != nil {
		t.Fatalf("GetTask live: %v", err)
	}
	if got.ArchivedAt == nil {
		t.Errorf("live task should be archived by cascade")
	}
}

// TestService_DeleteWorkflow_PartialArchiveErrorPreservesWorkflow verifies
// the fail-fast contract: when ArchiveTask returns a non-sentinel error
// part-way through the cascade, DeleteWorkflow surfaces it, leaves the
// workflow row intact, and the tasks archived before the failure stay
// archived. Retries are safe because ListTasks filters them out.
func TestService_DeleteWorkflow_PartialArchiveErrorPreservesWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-doomed", WorkspaceID: "ws-1", Name: "Doomed"})

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-first", WorkspaceID: "ws-1", WorkflowID: "wf-doomed", WorkflowStepID: "step-1", Title: "First",
	}); err != nil {
		t.Fatalf("CreateTask first: %v", err)
	}
	// "task-ghost" never actually exists in the DB — the leaky list returns
	// it so the cascade's ArchiveTask call hits a real GetTask error.
	ghost := &models.Task{ID: "task-ghost", WorkspaceID: "ws-1", WorkflowID: "wf-doomed", WorkflowStepID: "step-1", Title: "Ghost"}
	svc.tasks = leakyListTaskRepo{TaskRepository: repo, extra: []*models.Task{ghost}}

	err := svc.DeleteWorkflow(ctx, "wf-doomed")
	if err == nil {
		t.Fatalf("expected error when ArchiveTask fails mid-cascade")
	}
	if errors.Is(err, ErrTaskAlreadyArchived) {
		t.Fatalf("non-sentinel error must propagate, got sentinel: %v", err)
	}

	if _, err := svc.workflows.GetWorkflow(ctx, "wf-doomed"); err != nil {
		t.Fatalf("workflow row must survive a partial cascade, got: %v", err)
	}
	first, err := repo.GetTask(ctx, "task-first")
	if err != nil {
		t.Fatalf("GetTask first: %v", err)
	}
	if first.ArchivedAt == nil {
		t.Errorf("task archived before the failure should remain archived")
	}
}

// TestService_ArchiveTask_ReturnsAlreadyArchivedSentinel locks in the
// sentinel-error contract DeleteWorkflow relies on.
func TestService_ArchiveTask_ReturnsAlreadyArchivedSentinel(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "T",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := svc.ArchiveTask(ctx, "task-1"); err != nil {
		t.Fatalf("first ArchiveTask: %v", err)
	}
	err := svc.ArchiveTask(ctx, "task-1")
	if !errors.Is(err, ErrTaskAlreadyArchived) {
		t.Fatalf("second ArchiveTask: want ErrTaskAlreadyArchived, got %v", err)
	}
}

// TestApplyRepositoryUpdates_CopyFilesNilLeavesUntouched verifies the
// pointer-nil convention: a nil CopyFiles field on the request must not
// clobber an existing repository value.
func TestApplyRepositoryUpdates_CopyFilesNilLeavesUntouched(t *testing.T) {
	repo := &models.Repository{CopyFiles: "existing"}
	if err := applyRepositoryUpdates(repo, &UpdateRepositoryRequest{}); err != nil {
		t.Fatalf("applyRepositoryUpdates: %v", err)
	}
	if repo.CopyFiles != "existing" {
		t.Errorf("CopyFiles = %q, want %q (nil request field must not overwrite)", repo.CopyFiles, "existing")
	}
}

// TestApplyRepositoryUpdates_CopyFilesEmptyStringClears verifies that an
// explicit empty-string pointer clears the value (distinct from "no update").
func TestApplyRepositoryUpdates_CopyFilesEmptyStringClears(t *testing.T) {
	repo := &models.Repository{CopyFiles: "existing"}
	empty := ""
	if err := applyRepositoryUpdates(repo, &UpdateRepositoryRequest{CopyFiles: &empty}); err != nil {
		t.Fatalf("applyRepositoryUpdates: %v", err)
	}
	if repo.CopyFiles != "" {
		t.Errorf("CopyFiles = %q, want empty string", repo.CopyFiles)
	}
}
