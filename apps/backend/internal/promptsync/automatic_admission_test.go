package promptsync

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	"github.com/kandev/kandev/internal/reposync"
)

// pacedGitHub defers the first read of every file the way GitHub background
// pacing defers the download that follows a listing.
type pacedGitHub struct {
	reposync.GitHubClientProvider

	mu           sync.Mutex
	files        map[string]string
	fingerprint  string
	lists        int
	commits      int
	fileAttempts map[string]int
	deferred     map[string]bool
	// gate, when set, is the admission-change signal a deferral waits on.
	gate chan struct{}
	// onDefer, when set, is closed after the first deferral is returned.
	onDefer chan struct{}
}

func newPacedGitHub() *pacedGitHub {
	return &pacedGitHub{
		files:        map[string]string{"a.md": "A", "b.md": "B"},
		fingerprint:  "fp-1",
		fileAttempts: make(map[string]int),
		deferred:     make(map[string]bool),
	}
}

func (p *pacedGitHub) BindWorkspaceGitHubHost(ctx context.Context, _, _ string) (context.Context, error) {
	return ctx, nil
}

func (p *pacedGitHub) CommitWorkspaceGitHubHost(ctx context.Context, _ string, fn func(context.Context) error) error {
	p.mu.Lock()
	p.commits++
	p.mu.Unlock()
	return fn(ctx)
}

func (p *pacedGitHub) ListRepoDirectoryForWorkspace(_ context.Context, _, _, _, path, _ string) ([]github.RepoContentEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lists++
	entries := make([]github.RepoContentEntry, 0, len(p.files))
	for name := range p.files {
		entries = append(entries, github.RepoContentEntry{Name: name, Path: path + "/" + name, Type: "file"})
	}
	return entries, nil
}

func (p *pacedGitHub) GetRepoFileContentForWorkspace(_ context.Context, _, _, _, path, _ string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fileAttempts[path]++
	if !p.deferred[path] {
		p.deferred[path] = true
		deferral := &github.AdmissionDeferredError{Delay: time.Millisecond, Reason: "background_pacing"}
		if p.gate != nil {
			deferral.Delay, deferral.TrackerChanged = time.Hour, p.gate
		}
		if p.onDefer != nil {
			close(p.onDefer)
			p.onDefer = nil
		}
		return nil, deferral
	}
	return []byte(p.files[path[len(DefaultPath)+1:]]), nil
}

func (p *pacedGitHub) WorkspaceConnectionFingerprint(context.Context, string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fingerprint, nil
}

func (p *pacedGitHub) setFingerprint(fingerprint string) {
	p.mu.Lock()
	p.fingerprint = fingerprint
	p.mu.Unlock()
}

// stopPacing admits every later read without a deferral.
func (p *pacedGitHub) stopPacing() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for name := range p.files {
		p.deferred[DefaultPath+"/"+name] = true
	}
}

func (p *pacedGitHub) counts() (lists, commits int, attempts map[string]int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	attempts = make(map[string]int, len(p.fileAttempts))
	for path, n := range p.fileAttempts {
		attempts[path] = n
	}
	return p.lists, p.commits, attempts
}

func newPacedFixture(t *testing.T, gh *pacedGitHub) (*Service, *promptservice.Service) {
	t.Helper()
	database := openTestDB(t)
	repo, repoCleanup, err := promptstore.Provide(database, database)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, repoCleanup()) })
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	prompts := promptservice.NewService(repo)
	svc := NewService(newTestStore(t, database), gh, nil, prompts, nil, log)
	_, err = svc.SetConfig(context.Background(), &SetConfigRequest{
		WorkspaceID: "ws-1", Provider: reposync.ProviderGitHub, RepoOwner: "acme", RepoName: "eng",
	})
	require.NoError(t, err)
	return svc, prompts
}

// startWaitingAutomaticSync runs SyncIfDue until its first read is deferred
// and returns a channel closed when the run ends.
func startWaitingAutomaticSync(t *testing.T, ctx context.Context, svc *Service, gh *pacedGitHub) <-chan struct{} {
	t.Helper()
	deferredSignal := make(chan struct{})
	gh.mu.Lock()
	gh.gate, gh.onDefer = make(chan struct{}), deferredSignal
	gh.mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.SyncIfDue(ctx)
	}()
	select {
	case <-deferredSignal:
	case <-time.After(2 * time.Second):
		t.Fatal("automatic sync never reached a deferred read")
	}
	return done
}

func openGateAndWait(t *testing.T, gh *pacedGitHub, done <-chan struct{}) {
	t.Helper()
	gh.mu.Lock()
	close(gh.gate)
	gh.mu.Unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("automatic sync did not finish after admission opened")
	}
}

func TestAutomaticSyncCompletesUnderBackgroundPacing(t *testing.T) {
	gh := newPacedGitHub()
	svc, prompts := newPacedFixture(t, gh)
	ctx := context.Background()

	svc.SyncIfDue(ctx)

	for _, name := range []string{"a", "b"} {
		_, err := prompts.GetPromptByName(ctx, name)
		require.NoError(t, err, "prompt %q is applied by the paced automatic run", name)
	}
	cfg, err := svc.GetConfig(ctx)
	require.NoError(t, err)
	assert.True(t, cfg.LastOk)
	require.NotNil(t, cfg.LastSyncedAt)
	assert.Equal(t, 0, cfg.ConsecutiveFailures)

	lists, _, attempts := gh.counts()
	assert.Equal(t, 1, lists, "a deferral does not repeat the listing")
	assert.Equal(t, 2, attempts[DefaultPath+"/a.md"], "one deferred attempt and one read")
	assert.Equal(t, 2, attempts[DefaultPath+"/b.md"], "one deferred attempt and one read")
}

func TestAutomaticSyncDoesNotBlockManualWorkWhileWaiting(t *testing.T) {
	gh := newPacedGitHub()
	svc, _ := newPacedFixture(t, gh)
	ctx := context.Background()
	done := startWaitingAutomaticSync(t, ctx, svc, gh)
	gh.stopPacing()

	manualDone := make(chan error, 1)
	go func() {
		_, err := svc.Sync(ctx)
		manualDone <- err
	}()
	select {
	case err := <-manualDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Sync now waited for a deferred automatic sync")
	}
	_, commitsAfterManual, _ := gh.counts()

	openGateAndWait(t, gh, done)

	_, commits, _ := gh.counts()
	assert.Equal(t, commitsAfterManual, commits, "the later manual result supersedes the automatic run")
}

func TestAutomaticSyncDiscardsResultAfterChange(t *testing.T) {
	cases := map[string]func(t *testing.T, svc *Service, gh *pacedGitHub){
		"configuration saved": func(t *testing.T, svc *Service, _ *pacedGitHub) {
			_, err := svc.SetConfig(context.Background(), &SetConfigRequest{
				WorkspaceID: "ws-1", Provider: reposync.ProviderGitHub, RepoOwner: "acme", RepoName: "other",
			})
			require.NoError(t, err)
		},
		"configuration removed": func(t *testing.T, svc *Service, _ *pacedGitHub) {
			require.NoError(t, svc.DeleteConfig(context.Background(), Removal{}))
		},
		"credential changed": func(_ *testing.T, _ *Service, gh *pacedGitHub) {
			gh.setFingerprint("fp-2")
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			gh := newPacedGitHub()
			svc, prompts := newPacedFixture(t, gh)
			ctx := context.Background()
			done := startWaitingAutomaticSync(t, ctx, svc, gh)

			change(t, svc, gh)
			_, commitsBefore, _ := gh.counts()
			openGateAndWait(t, gh, done)

			_, commits, _ := gh.counts()
			assert.Equal(t, commitsBefore, commits, "the superseded result is not applied")
			list, err := prompts.ListPrompts(ctx)
			require.NoError(t, err)
			for _, prompt := range list {
				assert.NotEqual(t, "sync", prompt.Source, "no prompt is synced by the superseded run")
			}
		})
	}
}

func TestAutomaticSyncCancelledDuringFetchRecordsNothing(t *testing.T) {
	gh := newPacedGitHub()
	svc, _ := newPacedFixture(t, gh)
	ctx, cancel := context.WithCancel(context.Background())
	done := startWaitingAutomaticSync(t, ctx, svc, gh)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled automatic sync kept waiting")
	}

	cfg, err := svc.GetConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, cfg.ConsecutiveFailures)
	assert.Empty(t, cfg.LastError)
	assert.Nil(t, cfg.LastSyncedAt)
	_, commits, _ := gh.counts()
	assert.Equal(t, 1, commits, "only the configuration save committed")
}
