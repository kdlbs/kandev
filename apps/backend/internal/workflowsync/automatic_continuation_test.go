package workflowsync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
)

type pacedWorkflowGitHub struct {
	mu                 sync.Mutex
	deferred           chan struct{}
	trackerChanged     chan struct{}
	deferSecondFile    bool
	fingerprint        string
	directoryReads     int
	fileReads          map[string]int
	httpDirectoryReads int
	httpFileReads      map[string]int
	failCredential     string
	failedFileStarted  chan struct{}
	releaseFailedFile  chan struct{}
	server             *httptest.Server
}

func newPacedWorkflowGitHub(t *testing.T) *pacedWorkflowGitHub {
	t.Helper()
	provider := &pacedWorkflowGitHub{
		deferred:        make(chan struct{}),
		trackerChanged:  make(chan struct{}),
		deferSecondFile: true,
		fingerprint:     "credential-1",
		fileReads:       make(map[string]int),
		httpFileReads:   make(map[string]int),
	}
	provider.server = httptest.NewServer(http.HandlerFunc(provider.serveHTTP))
	t.Cleanup(provider.server.Close)
	return provider
}

func (p *pacedWorkflowGitHub) ListRepoDirectoryForWorkspace(
	ctx context.Context, workspaceID, owner, repo, path, ref string,
) ([]github.RepoContentEntry, error) {
	p.mu.Lock()
	p.directoryReads++
	p.mu.Unlock()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.server.URL+"/directory", nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	var entries []github.RepoContentEntry
	if err := json.NewDecoder(response.Body).Decode(&entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (p *pacedWorkflowGitHub) GetRepoFileContentForWorkspace(
	ctx context.Context, workspaceID, owner, repo, path, ref string,
) ([]byte, error) {
	p.mu.Lock()
	p.fileReads[path]++
	fingerprint := p.fingerprint
	if fingerprint == p.failCredential {
		started := p.failedFileStarted
		release := p.releaseFailedFile
		p.mu.Unlock()
		close(started)
		<-release
		return nil, errors.New("credential rejected")
	}
	if strings.HasSuffix(path, "/beta.yml") && p.deferSecondFile {
		p.deferSecondFile = false
		close(p.deferred)
		p.mu.Unlock()
		return nil, &github.AdmissionDeferredError{
			Delay: time.Hour, TrackerChanged: p.trackerChanged, Reason: "test pacing",
		}
	}
	p.mu.Unlock()
	request, err := http.NewRequestWithContext(
		ctx, http.MethodGet, p.server.URL+"/file?path="+url.QueryEscape(path)+"&credential="+url.QueryEscape(fingerprint), nil,
	)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	return io.ReadAll(response.Body)
}

func (p *pacedWorkflowGitHub) serveHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	switch r.URL.Path {
	case "/directory":
		p.httpDirectoryReads++
		p.mu.Unlock()
		_ = json.NewEncoder(w).Encode([]github.RepoContentEntry{
			{Name: "alpha.yml", Path: DefaultPath + "/alpha.yml", Type: "file"},
			{Name: "beta.yml", Path: DefaultPath + "/beta.yml", Type: "file"},
		})
	case "/file":
		path := r.URL.Query().Get("path")
		fingerprint := r.URL.Query().Get("credential")
		p.httpFileReads[path]++
		p.mu.Unlock()
		_, _ = io.WriteString(w, strings.ReplaceAll(validExportYAML, "Dev Flow", "Dev Flow "+fingerprint))
	default:
		p.mu.Unlock()
		http.NotFound(w, r)
	}
}

func (p *pacedWorkflowGitHub) WorkspaceConnectionFingerprint(context.Context, string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fingerprint, nil
}

func (p *pacedWorkflowGitHub) setFingerprint(fingerprint string) {
	p.mu.Lock()
	p.fingerprint = fingerprint
	p.mu.Unlock()
}

func (p *pacedWorkflowGitHub) failCredentialRequest(fingerprint string) (<-chan struct{}, chan<- struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failCredential = fingerprint
	p.failedFileStarted = make(chan struct{})
	p.releaseFailedFile = make(chan struct{})
	return p.failedFileStarted, p.releaseFailedFile
}

func (p *pacedWorkflowGitHub) counts() (int, map[string]int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	reads := make(map[string]int, len(p.fileReads))
	for path, count := range p.fileReads {
		reads[path] = count
	}
	return p.directoryReads, reads
}

func (p *pacedWorkflowGitHub) httpCounts() (int, map[string]int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	reads := make(map[string]int, len(p.httpFileReads))
	for path, count := range p.httpFileReads {
		reads[path] = count
	}
	return p.httpDirectoryReads, reads
}

// TestAutomaticSyncResumesFileFetchAfterPacing exercises the scheduler,
// automatic service path, fetch loop, applier, and persisted sync result.
func TestAutomaticSyncResumesFileFetchAfterPacing(t *testing.T) {
	provider := newPacedWorkflowGitHub(t)
	svc, applier := setupAutomaticContinuationService(t, provider)
	configureWorkspace(t, svc, "ws-paced")
	t.Cleanup(svc.waitAutomaticSyncs)

	svc.dispatchAutomaticSync("ws-paced", false)
	<-provider.deferred

	svc.automaticMu.Lock()
	state := svc.automaticInFlight["ws-paced"]
	svc.automaticMu.Unlock()
	if state == nil {
		t.Fatal("deferred automatic job was not retained")
	}
	close(provider.trackerChanged)
	<-state.done

	directoryReads, fileReads := provider.counts()
	require.Equal(t, 1, directoryReads)
	require.Equal(t, 1, fileReads[DefaultPath+"/alpha.yml"])
	require.Equal(t, 2, fileReads[DefaultPath+"/beta.yml"], "one admission attempt and one provider read")
	httpDirectoryReads, httpFileReads := provider.httpCounts()
	require.Equal(t, 1, httpDirectoryReads)
	require.Equal(t, 1, httpFileReads[DefaultPath+"/alpha.yml"])
	require.Equal(t, 1, httpFileReads[DefaultPath+"/beta.yml"])
	require.Equal(t, 1, applier.callCount())

	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-paced")
	require.NoError(t, err)
	require.True(t, cfg.LastOk)
	require.Equal(t, 0, cfg.ConsecutiveFailures)
}

func TestAutomaticSyncDiscardsStaleContinuationAfterConfigReplacement(t *testing.T) {
	provider := newPacedWorkflowGitHub(t)
	svc, applier := setupAutomaticContinuationService(t, provider)
	configureWorkspace(t, svc, "ws-replaced")
	t.Cleanup(svc.waitAutomaticSyncs)

	svc.dispatchAutomaticSync("ws-replaced", false)
	<-provider.deferred

	svc.automaticMu.Lock()
	state := svc.automaticInFlight["ws-replaced"]
	svc.automaticMu.Unlock()
	if state == nil {
		t.Fatal("deferred automatic job was not retained")
	}
	_, err := svc.SetConfigForWorkspace(context.Background(), "ws-replaced", &SetConfigRequest{
		RepoOwner: "acme", RepoName: "flows", Path: "replacement",
	})
	require.NoError(t, err)
	<-state.done

	directoryReads, fileReads := provider.counts()
	require.Equal(t, 1, directoryReads)
	require.Len(t, fileReads, 2)
	require.Zero(t, applier.callCount())
	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-replaced")
	require.NoError(t, err)
	require.Equal(t, "replacement", cfg.Path)
	require.False(t, cfg.LastOk)
}

func TestAutomaticSyncDiscardsStaleContinuationAfterManualSuccess(t *testing.T) {
	provider := newPacedWorkflowGitHub(t)
	svc, applier := setupAutomaticContinuationService(t, provider)
	configureWorkspace(t, svc, "ws-manual")
	t.Cleanup(svc.waitAutomaticSyncs)

	svc.dispatchAutomaticSync("ws-manual", false)
	<-provider.deferred

	svc.automaticMu.Lock()
	state := svc.automaticInFlight["ws-manual"]
	svc.automaticMu.Unlock()
	if state == nil {
		t.Fatal("deferred automatic job was not retained")
	}
	_, err := svc.SyncWorkspace(context.Background(), "ws-manual")
	require.NoError(t, err)
	<-state.done

	directoryReads, fileReads := provider.counts()
	require.Equal(t, 2, directoryReads, "manual sync starts a fresh fetch")
	require.Equal(t, 2, fileReads[DefaultPath+"/alpha.yml"])
	require.Equal(t, 2, fileReads[DefaultPath+"/beta.yml"])
	require.Equal(t, 1, applier.callCount(), "only the manual result is applied")
	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-manual")
	require.NoError(t, err)
	require.True(t, cfg.LastOk)
}

func TestAutomaticSyncDiscardsContinuationOnShutdown(t *testing.T) {
	provider := newPacedWorkflowGitHub(t)
	svc, applier := setupAutomaticContinuationService(t, provider)
	configureWorkspace(t, svc, "ws-shutdown")

	svc.dispatchAutomaticSync("ws-shutdown", false)
	<-provider.deferred

	svc.automaticMu.Lock()
	state := svc.automaticInFlight["ws-shutdown"]
	svc.automaticMu.Unlock()
	if state == nil {
		t.Fatal("deferred automatic job was not retained")
	}
	svc.waitAutomaticSyncs()
	<-state.done
	require.Zero(t, applier.callCount(), "shutdown must not apply deferred data")
	require.Nil(t, state.getContinuation(), "shutdown must discard fetched progress")
}

func TestAutomaticSyncRestartsFetchAfterCredentialChange(t *testing.T) {
	provider := newPacedWorkflowGitHub(t)
	svc, applier := setupAutomaticContinuationService(t, provider)
	configureWorkspace(t, svc, "ws-credential")
	t.Cleanup(svc.waitAutomaticSyncs)

	svc.dispatchAutomaticSync("ws-credential", false)
	<-provider.deferred

	svc.automaticMu.Lock()
	state := svc.automaticInFlight["ws-credential"]
	svc.automaticMu.Unlock()
	if state == nil {
		t.Fatal("deferred automatic job was not retained")
	}
	provider.setFingerprint("credential-2")
	close(provider.trackerChanged)
	<-state.done

	directoryReads, fileReads := provider.counts()
	require.Equal(t, 2, directoryReads, "credential replacement requires a fresh listing")
	require.Equal(t, 2, fileReads[DefaultPath+"/alpha.yml"])
	require.Equal(t, 2, fileReads[DefaultPath+"/beta.yml"])
	require.Equal(t, 1, applier.callCount())
	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-credential")
	require.NoError(t, err)
	require.True(t, cfg.LastOk)
	require.Equal(t, "credential-2", cfg.CredentialFingerprint)
}

func TestAutomaticSyncRestartsAfterFailedRequestUsesRotatedCredential(t *testing.T) {
	provider := newPacedWorkflowGitHub(t)
	provider.mu.Lock()
	provider.deferSecondFile = false
	provider.mu.Unlock()
	started, release := provider.failCredentialRequest("credential-1")
	svc, applier := setupAutomaticContinuationService(t, provider)
	configureWorkspace(t, svc, "ws-credential-error")
	t.Cleanup(svc.waitAutomaticSyncs)

	svc.dispatchAutomaticSync("ws-credential-error", false)
	<-started
	svc.automaticMu.Lock()
	state := svc.automaticInFlight["ws-credential-error"]
	svc.automaticMu.Unlock()
	require.NotNil(t, state)
	provider.setFingerprint("credential-2")
	close(release)
	<-state.done

	directoryReads, fileReads := provider.counts()
	require.Equal(t, 2, directoryReads, "credential replacement requires a fresh listing")
	require.Equal(t, 2, fileReads[DefaultPath+"/alpha.yml"], "the failed old-credential request must be retried")
	require.Equal(t, 1, fileReads[DefaultPath+"/beta.yml"])
	require.Equal(t, 1, applier.callCount(), "only the complete replacement-credential result is applied")
	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-credential-error")
	require.NoError(t, err)
	require.True(t, cfg.LastOk)
	require.Equal(t, "credential-2", cfg.CredentialFingerprint)
}

func setupAutomaticContinuationService(
	t *testing.T, provider GitHubClientProvider,
) (*Service, *fakeApplier) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	applier := &fakeApplier{}
	return NewService(setupTestStore(t), provider, nil, applier, log), applier
}
