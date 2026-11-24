package promptsync

import (
	"context"
	"errors"
	"fmt"
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
	// rejectCommit makes the commit fence reject the apply.
	rejectCommit bool
	// commitStarted and commitRelease hold the commit fence before it calls
	// the apply callback.
	commitStarted chan struct{}
	commitRelease chan struct{}
	// unpaced admits every read.
	unpaced bool
	// moveBranchOnDefer simulates a branch rename after a file read is deferred.
	moveBranchOnDefer bool
	branchMoved       bool
	resolvedRef       string
	resolvedRefs      []string
	resolveCalls      int
	readRefs          []string
	// revertFingerprint, when set, replaces fingerprint after one lookup.
	revertFingerprint string
}

// gateClosed reports whether a gate is set and admission has not opened.
func (p *pacedGitHub) gateClosed() bool {
	if p.gate == nil {
		return false
	}
	select {
	case <-p.gate:
		return false
	default:
		return true
	}
}

func newPacedGitHub() *pacedGitHub {
	return &pacedGitHub{
		files:        map[string]string{"a.md": "A", "b.md": "B"},
		fingerprint:  "fp-1",
		resolvedRef:  "commit-a",
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
	reject := p.rejectCommit
	started, release := p.commitStarted, p.commitRelease
	p.mu.Unlock()
	if reject {
		return fmt.Errorf("commit fence: %w", github.ErrGitHubHostMismatch)
	}
	if started != nil {
		close(started)
		<-release
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return fn(ctx)
}

func (p *pacedGitHub) ResolveRepoRefForWorkspace(_ context.Context, _, _, _, ref string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resolveCalls++
	p.resolvedRefs = append(p.resolvedRefs, ref)
	return p.resolvedRef, nil
}

func (p *pacedGitHub) ListRepoDirectoryForWorkspace(_ context.Context, _, _, _, path, ref string) ([]github.RepoContentEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lists++
	p.readRefs = append(p.readRefs, ref)
	entries := make([]github.RepoContentEntry, 0, len(p.files))
	for name := range p.files {
		entries = append(entries, github.RepoContentEntry{Name: name, Path: path + "/" + name, Type: "file"})
	}
	return entries, nil
}

func (p *pacedGitHub) GetRepoFileContentForWorkspace(_ context.Context, _, _, _, path, ref string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fileAttempts[path]++
	p.readRefs = append(p.readRefs, ref)
	if !p.unpaced && (!p.deferred[path] || p.gateClosed()) {
		p.deferred[path] = true
		if p.moveBranchOnDefer {
			p.branchMoved = true
		}
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
	if p.branchMoved && ref != p.resolvedRef {
		return nil, &github.GitHubAPIError{StatusCode: 404}
	}
	return []byte(p.files[path[len(DefaultPath)+1:]]), nil
}

func (p *pacedGitHub) WorkspaceConnectionFingerprint(context.Context, string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fingerprint := p.fingerprint
	if p.revertFingerprint != "" {
		p.fingerprint, p.revertFingerprint = p.revertFingerprint, ""
	}
	return fingerprint, nil
}

// flashFingerprint reports fingerprint once, then the current value again.
func (p *pacedGitHub) flashFingerprint(fingerprint string) {
	p.mu.Lock()
	p.revertFingerprint, p.fingerprint = p.fingerprint, fingerprint
	p.mu.Unlock()
}

// stopPacing admits every later read without a deferral.
func (p *pacedGitHub) stopPacing() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.unpaced = true
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

func (p *pacedGitHub) refs() (reads, resolved []string, calls int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.readRefs...), append([]string(nil), p.resolvedRefs...), p.resolveCalls
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
// and returns a channel closed when the run ends. Cleanup cancels the run and
// waits for it, so a failed test cannot leave it running past teardown.
func startWaitingAutomaticSync(t *testing.T, ctx context.Context, svc *Service, gh *pacedGitHub) <-chan struct{} {
	t.Helper()
	deferredSignal := make(chan struct{})
	gh.mu.Lock()
	gh.gate, gh.onDefer = make(chan struct{}), deferredSignal
	gh.mu.Unlock()
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.SyncIfDue(runCtx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("automatic sync did not stop after cleanup")
		}
	})
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

func TestAutomaticSyncPinsResolvedCommitAcrossDeferredRead(t *testing.T) {
	gh := newPacedGitHub()
	gh.resolvedRef = "commit-a"
	gh.moveBranchOnDefer = true
	svc, _ := newPacedFixture(t, gh)

	done := startWaitingAutomaticSync(t, context.Background(), svc, gh)
	openGateAndWait(t, gh, done)

	cfg, err := svc.GetConfig(context.Background())
	require.NoError(t, err)
	assert.True(t, cfg.LastOk, "a branch rename during admission wait must not turn the pinned file read into a 404")
	assert.False(t, cfg.PollSuspended)
	refs, resolvedRefs, resolveCalls := gh.refs()
	assert.Equal(t, 1, resolveCalls)
	assert.Equal(t, []string{"main"}, resolvedRefs)
	assert.NotEmpty(t, refs)
	for _, ref := range refs {
		assert.Equal(t, "commit-a", ref, "the listing and every retry must use the resolved commit")
	}
}

func TestAutomaticSyncCancellationDuringCommitFenceDoesNotRecordFailure(t *testing.T) {
	gh := newPacedGitHub()
	gh.resolvedRef = "commit-a"
	gh.unpaced = true
	svc, _ := newPacedFixture(t, gh)
	gh.commitStarted = make(chan struct{})
	gh.commitRelease = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.SyncIfDue(ctx)
	}()
	select {
	case <-gh.commitStarted:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("automatic sync did not reach the GitHub commit fence")
	}
	cancel()
	close(gh.commitRelease)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("automatic sync did not stop after commit fence cancellation")
	}

	cfg, err := svc.GetConfig(context.Background())
	require.NoError(t, err)
	assert.False(t, cfg.LastOk)
	assert.Equal(t, 0, cfg.ConsecutiveFailures)
	assert.Empty(t, cfg.LastErrorClass, "shutdown cancellation must not persist a failure")
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

	waitForRun(t, done)

	_, commits, _ := gh.counts()
	assert.Equal(t, commitsAfterManual, commits, "the later manual result supersedes the automatic run")
}

// waitForRun fails the test unless the automatic run ends while its
// admission gate is still closed.
func waitForRun(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("superseded automatic sync kept waiting for admission")
	}
}

func TestAutomaticSyncSupersededByLocalChangeWakesAndDiscards(t *testing.T) {
	cases := map[string]func(t *testing.T, svc *Service){
		"configuration saved": func(t *testing.T, svc *Service) {
			_, err := svc.SetConfig(context.Background(), &SetConfigRequest{
				WorkspaceID: "ws-1", Provider: reposync.ProviderGitHub, RepoOwner: "acme", RepoName: "other",
			})
			require.NoError(t, err)
		},
		"configuration removed": func(t *testing.T, svc *Service) {
			require.NoError(t, svc.DeleteConfig(context.Background(), Removal{}))
		},
		"exclusive cleanup": func(t *testing.T, svc *Service) {
			require.NoError(t, svc.RunExclusive(func() error { return nil }))
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			gh := newPacedGitHub()
			svc, prompts := newPacedFixture(t, gh)
			done := startWaitingAutomaticSync(t, context.Background(), svc, gh)

			change(t, svc)
			_, commitsBefore, _ := gh.counts()
			waitForRun(t, done)

			_, commits, _ := gh.counts()
			assert.Equal(t, commitsBefore, commits, "the superseded result is not applied")
			assertNoSyncedPrompts(t, prompts)
		})
	}
}

func TestAutomaticSyncDiscardsResultAfterCredentialChange(t *testing.T) {
	for name, fingerprint := range map[string]string{"replaced": "fp-2", "removed": ""} {
		t.Run(name, func(t *testing.T) {
			gh := newPacedGitHub()
			svc, prompts := newPacedFixture(t, gh)
			svc.credentialRecheck = 10 * time.Millisecond
			ctx := context.Background()
			svc.refreshCredentialFingerprint(ctx, mustConfig(t, svc))
			done := startWaitingAutomaticSync(t, ctx, svc, gh)

			gh.flashFingerprint(fingerprint)
			waitForRun(t, done)

			_, commits, _ := gh.counts()
			assert.Equal(t, 1, commits, "only the configuration save committed")
			assertNoSyncedPrompts(t, prompts)
			cfg := mustConfig(t, svc)
			assert.Empty(t, cfg.LastError, "a stale run records no failure")
			assert.Nil(t, cfg.LastSyncedAt)
		})
	}
}

func mustConfig(t *testing.T, svc *Service) *Config {
	t.Helper()
	cfg, err := svc.GetConfig(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cfg)
	return cfg
}

func assertNoSyncedPrompts(t *testing.T, prompts *promptservice.Service) {
	t.Helper()
	list, err := prompts.ListPrompts(context.Background())
	require.NoError(t, err)
	for _, prompt := range list {
		assert.NotEqual(t, "sync", prompt.Source, "no prompt is synced by the superseded run")
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

func TestAutomaticSyncSurvivesDeniedRemoval(t *testing.T) {
	gh := newPacedGitHub()
	svc, prompts := newPacedFixture(t, gh)
	ctx := context.Background()
	done := startWaitingAutomaticSync(t, ctx, svc, gh)

	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return errors.New("denied") })
	require.Error(t, svc.DeleteConfig(ctx, Removal{}))
	openGateAndWait(t, gh, done)

	_, err := prompts.GetPromptByName(ctx, "a")
	require.NoError(t, err, "a denied removal leaves the automatic run able to apply")
	assert.True(t, mustConfig(t, svc).LastOk)
}

func TestAutomaticSyncRejectedByCommitFenceRecordsNothing(t *testing.T) {
	gh := newPacedGitHub()
	svc, prompts := newPacedFixture(t, gh)
	gh.mu.Lock()
	gh.rejectCommit = true
	gh.mu.Unlock()

	svc.SyncIfDue(context.Background())

	cfg := mustConfig(t, svc)
	assert.Empty(t, cfg.LastError, "a stale automatic result records no failure")
	assert.Equal(t, 0, cfg.ConsecutiveFailures)
	assert.Nil(t, cfg.LastSyncedAt)
	assertNoSyncedPrompts(t, prompts)
}
