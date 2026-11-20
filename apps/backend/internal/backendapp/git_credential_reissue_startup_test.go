package backendapp

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/secrets"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestGitCredentialBrokerDefaultReissue(t *testing.T) {
	cfg := newReissueStartupConfig(t)
	masterKey := make([]byte, secrets.MasterKeySize)
	for i := range masterKey {
		masterKey[i] = byte(i)
	}
	writeReissueMasterKey(t, cfg, masterKey)
	repos := provideReissueTestRepositories(t, cfg, newTestLogger())
	if repos.gitCredentialReissueSignerSource != gitCredentialReissueSignerInstallation {
		t.Fatalf("signer source = %q, want installation", repos.gitCredentialReissueSignerSource)
	}
	beforeRestart := newGitCredentialReissueTestBroker(repos.gitCredentialReissueSigner)
	lease, capability, err := beforeRestart.IssueWithReissueCapability(t.Context(), gitCredentialReissueTestScope())
	if err != nil {
		t.Fatalf("IssueWithReissueCapability() using startup signer: %v", err)
	}
	if lease.Token == "" || capability.Token == "" {
		t.Fatalf("startup broker lease/capability are empty: lease=%v capability=%v", lease.Token != "", capability.Token != "")
	}

	masterKeyProvider, err := secrets.NewMasterKeyProvider(cfg.ResolvedDataDir())
	if err != nil {
		t.Fatalf("reopen installation master key: %v", err)
	}
	afterRestartSigner, source, err := selectGitCredentialReissueSigner(cfg.GitHubCredentialBroker, masterKeyProvider.Key())
	if err != nil {
		t.Fatalf("select reissue signer after restart: %v", err)
	}
	if source != gitCredentialReissueSignerInstallation {
		t.Fatalf("restarted signer source = %q, want installation", source)
	}
	afterRestart := newGitCredentialReissueTestBroker(afterRestartSigner)
	if got := afterRestart.ActiveLeaseCount(); got != 0 {
		t.Fatalf("restarted broker lease count = %d, want empty map", got)
	}
	reissued, err := afterRestart.Reissue(t.Context(), gitCredentialReissueTestRequest(capability.Token))
	if err != nil {
		t.Fatalf("reissue after restart with reopened installation key: %v", err)
	}
	if reissued.Token == lease.Token {
		t.Fatal("restart reissue returned the original lease")
	}
}

func TestGitCredentialBrokerConfiguredReissue(t *testing.T) {
	const override = "legacy-configured-signing-key"
	scope := gitCredentialReissueTestScope()
	legacySigner, err := gitcredentials.NewReissueCapabilitySigner(override)
	if err != nil {
		t.Fatalf("create legacy signer: %v", err)
	}
	capability, err := legacySigner.Issue(scope, time.Now())
	if err != nil {
		t.Fatalf("issue legacy capability: %v", err)
	}
	selected, source, err := selectGitCredentialReissueSigner(config.GitHubCredentialBrokerConfig{ReissueSigningKey: override}, make([]byte, secrets.MasterKeySize))
	if err != nil {
		t.Fatalf("select configured signer: %v", err)
	}
	if source != gitCredentialReissueSignerConfigured {
		t.Fatalf("configured signer source = %q", source)
	}
	if _, err := newGitCredentialReissueTestBroker(selected).Reissue(t.Context(), gitCredentialReissueTestRequest(capability.Token)); err != nil {
		t.Fatalf("configured signer did not recover an existing capability: %v", err)
	}
}

func TestGitCredentialBrokerDisabledReissue(t *testing.T) {
	const override = "configured-but-disabled-signing-key"
	selected, source, err := selectGitCredentialReissueSigner(config.GitHubCredentialBrokerConfig{
		DisableReissue: true, ReissueSigningKey: override,
	}, make([]byte, secrets.MasterKeySize))
	if err != nil {
		t.Fatalf("select disabled signer: %v", err)
	}
	if selected != nil || source != gitCredentialReissueSignerDisabled {
		t.Fatalf("disabled signer = %v, source=%q", selected, source)
	}
	broker := newGitCredentialReissueTestBroker(selected)
	lease, err := broker.Issue(t.Context(), gitCredentialReissueTestScope())
	if err != nil || lease.Token == "" {
		t.Fatalf("lease-only issuance = %v, %v", lease.Token != "", err)
	}
	if _, _, err := broker.IssueWithReissueCapability(t.Context(), gitCredentialReissueTestScope()); !errors.Is(err, gitcredentials.ErrReissueUnavailable) {
		t.Fatalf("IssueWithReissueCapability() error = %v, want reissue unavailable", err)
	}
	legacySigner, err := gitcredentials.NewReissueCapabilitySigner(override)
	if err != nil {
		t.Fatalf("create existing capability signer: %v", err)
	}
	capability, err := legacySigner.Issue(gitCredentialReissueTestScope(), time.Now())
	if err != nil {
		t.Fatalf("issue existing capability: %v", err)
	}
	if _, err := broker.Reissue(t.Context(), gitCredentialReissueTestRequest(capability.Token)); !errors.Is(err, gitcredentials.ErrReissueUnavailable) {
		t.Fatalf("Reissue() while disabled error = %v, want reissue unavailable", err)
	}
}

func TestGitCredentialReissueInstallationKeyDerivation(t *testing.T) {
	masterKey := make([]byte, secrets.MasterKeySize)
	for i := range masterKey {
		masterKey[i] = byte(i)
	}
	signer, source, err := selectGitCredentialReissueSigner(config.GitHubCredentialBrokerConfig{}, masterKey)
	if err != nil {
		t.Fatalf("select installation signer: %v", err)
	}
	if source != gitCredentialReissueSignerInstallation {
		t.Fatalf("signer source = %q, want installation", source)
	}
	const expectedEncoding = "6ONVvppBGqHHE1X36Xp_6zunSN7t-WpUK9GkO1Qe3v0"
	expected, err := gitcredentials.NewReissueCapabilitySigner(expectedEncoding)
	if err != nil {
		t.Fatalf("create expected vector signer: %v", err)
	}
	capability, err := signer.Issue(gitCredentialReissueTestScope(), time.Now())
	if err != nil {
		t.Fatalf("issue default capability: %v", err)
	}
	if _, err := expected.Validate(capability.Token, time.Now()); err != nil {
		t.Fatalf("derived capability did not match fixed key vector: %v", err)
	}

	foreignKey := append([]byte(nil), masterKey...)
	foreignKey[0] ^= 0xff
	foreignSigner, _, err := selectGitCredentialReissueSigner(config.GitHubCredentialBrokerConfig{}, foreignKey)
	if err != nil {
		t.Fatalf("select foreign installation signer: %v", err)
	}
	if _, err := foreignSigner.Validate(capability.Token, time.Now()); !errors.Is(err, gitcredentials.ErrReissueCapabilityInvalid) {
		t.Fatalf("foreign installation validated capability: %v", err)
	}
}

func TestGitCredentialReissueStartupDiagnostic(t *testing.T) {
	cfg := newReissueStartupConfig(t)
	masterKey := make([]byte, secrets.MasterKeySize)
	for i := range masterKey {
		masterKey[i] = 0xa5
	}
	writeReissueMasterKey(t, cfg, masterKey)
	core, recorded := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	provideReissueTestRepositories(t, cfg, log)

	entries := recorded.FilterMessage("git_credential_reissue.signer_selected").All()
	if len(entries) != 1 {
		t.Fatalf("signer selection events = %d, want one", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["source"] != gitCredentialReissueSignerInstallation {
		t.Fatalf("signer selection fields = %#v", fields)
	}
	if fields["master_key"] != nil || fields["signing_key"] != nil || fields["lease"] != nil || fields["capability"] != nil {
		t.Fatalf("signer selection log contains secret fields: %#v", fields)
	}
	if strings.Contains(fmt.Sprint(fields), hex.EncodeToString(masterKey)) {
		t.Fatal("signer selection event exposed the installation key")
	}
}

func TestGitCredentialReissueDiagnosticSources(t *testing.T) {
	for _, source := range []string{
		gitCredentialReissueSignerInstallation,
		gitCredentialReissueSignerConfigured,
		gitCredentialReissueSignerDisabled,
	} {
		t.Run(source, func(t *testing.T) {
			core, recorded := observer.New(zapcore.InfoLevel)
			log, err := logger.NewFromZap(zap.New(core))
			if err != nil {
				t.Fatalf("create observer logger: %v", err)
			}
			logGitCredentialReissueSignerSelected(log, source)
			entries := recorded.FilterMessage("git_credential_reissue.signer_selected").All()
			if len(entries) != 1 || entries[0].ContextMap()["source"] != source {
				t.Fatalf("selection event for %q = %#v", source, entries)
			}
		})
	}
}

func newReissueStartupConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{HomeDir: t.TempDir(), Database: config.DatabaseConfig{Driver: "sqlite"}}
}

func writeReissueMasterKey(t *testing.T, cfg *config.Config, masterKey []byte) {
	t.Helper()
	if err := os.MkdirAll(cfg.ResolvedDataDir(), 0o700); err != nil {
		t.Fatalf("create data directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.ResolvedDataDir(), secrets.MasterKeyFile), masterKey, 0o600); err != nil {
		t.Fatalf("write master key: %v", err)
	}
}

func provideReissueTestRepositories(t *testing.T, cfg *config.Config, log *logger.Logger) *Repositories {
	t.Helper()
	_, repos, cleanups, err := provideRepositories(context.Background(), cfg, log, "git-credential-reissue-test")
	t.Cleanup(func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanups[i] != nil {
				_ = cleanups[i]()
			}
		}
	})
	if err != nil {
		t.Fatalf("provideRepositories: %v", err)
	}
	return repos
}

func newGitCredentialReissueTestBroker(signer *gitcredentials.ReissueCapabilitySigner) *gitcredentials.Broker {
	repo := &fakeGitHubBrokerTaskRepository{
		task:       &taskmodels.Task{ID: "task-1", WorkspaceID: "workspace-1"},
		session:    &taskmodels.TaskSession{ID: "session-1", TaskID: "task-1", State: taskmodels.TaskSessionStateRunning},
		repository: &taskmodels.Repository{ID: "repo-1", WorkspaceID: "workspace-1", Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "widgets"},
		links:      []*taskmodels.TaskRepository{{TaskID: "task-1", RepositoryID: "repo-1"}},
	}
	source := &factoryTestGitHubCredentialSource{verifier: &fakeContributionDestinationVerifier{}}
	return newGitCredentialBroker(source, nil, repo, signer)
}

func gitCredentialReissueTestScope() gitcredentials.Scope {
	return gitcredentials.Scope{
		ProviderID: "github", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repo-1", Host: "github.com", Path: "/acme/widgets.git",
	}
}

func gitCredentialReissueTestRequest(capability string) gitcredentials.ReissueRequest {
	return gitcredentials.ReissueRequest{
		Capability: capability, TaskID: "task-1", SessionID: "session-1", RepositoryID: "repo-1",
		Host: "github.com", Path: "/acme/widgets.git",
	}
}
