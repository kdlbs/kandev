// Package updates implements the kandev background updates poller and the
// HTTP surface for the System -> Updates page. It polls GitHub Releases every
// 6 hours, persists the latest tag in the kandev_meta key/value table, and
// exposes a 30s rate-limited "check now" handler.
package updates

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/system/jobs"
)

// PollInterval is the cadence at which the background goroutine polls GitHub.
const PollInterval = 6 * time.Hour

// ManualCheckWindow is the minimum gap between two manual /check calls.
const ManualCheckWindow = 30 * time.Second

// applyGuardTTL bounds how long a single self-update launch blocks further
// applies. It matches the frontend progress timeout: past this window we assume
// the helper failed without restarting the backend and allow a retry, instead
// of wedging apply behind a permanent 409.
const applyGuardTTL = 5 * time.Minute

// ErrRateLimited indicates that a manual check was denied because the
// per-process rate limiter window has not yet elapsed.
var ErrRateLimited = errors.New("updates check rate limited")

// ErrApplyConfirm indicates that POST /updates/apply did not include the
// explicit confirmation token.
var ErrApplyConfirm = errors.New("update apply requires confirm=UPDATE")

// ErrApplyUnsupported indicates that the current installation cannot be
// updated safely from the UI.
var ErrApplyUnsupported = errors.New("update apply unsupported")

// ErrNoUpdateAvailable indicates that the cached release state does not show
// a newer semver than the running binary.
var ErrNoUpdateAvailable = errors.New("no update available")

// ErrApplyInProgress indicates that a self-update helper has already been
// launched and not yet completed, so a second apply is refused.
var ErrApplyInProgress = errors.New("a self-update is already in progress")

// devVersion is the sentinel used by the ldflags-injected current version
// when no release tag was baked in (i.e. local dev builds).
const devVersion = "dev"

// UpdatesResponse is the JSON shape returned by both Get() and Check().
type UpdatesResponse struct {
	Current                string               `json:"current"`
	Latest                 string               `json:"latest"`
	LatestURL              string               `json:"latest_url"`
	LatestCheckedAt        time.Time            `json:"latest_checked_at"`
	UpdateAvailable        bool                 `json:"update_available"`
	Install                InstallStateResponse `json:"install"`
	ApplySupported         bool                 `json:"apply_supported"`
	ApplyUnsupportedReason string               `json:"apply_unsupported_reason,omitempty"`
	ManualCommands         []string             `json:"manual_commands,omitempty"`
}

// ApplyResponse is the 202 payload returned when a self-update helper has
// been queued.
type ApplyResponse struct {
	JobID string `json:"job_id"`
}

// InstallStateResponse describes whether this backend is currently running
// under a kandev-managed service unit/plist. The UI uses this to hard-gate
// one-click updates.
type InstallStateResponse struct {
	RunningAsService bool   `json:"running_as_service"`
	ManagedService   bool   `json:"managed_service"`
	Mode             string `json:"mode,omitempty"`
	Manager          string `json:"manager,omitempty"`
	Kind             string `json:"kind,omitempty"`
	MetadataPath     string `json:"metadata_path,omitempty"`
}

// releaseURL is the GitHub endpoint the service polls. Tests override it via
// SetReleaseURL.
//
// Service holds the wiring needed to drive the poller and serve the two
// HTTP endpoints.
type Service struct {
	pool       *db.Pool
	current    string
	httpClient *http.Client
	log        *logger.Logger
	limiter    *Limiter
	jobs       *jobs.Tracker
	homeDir    string
	getenv     func(string) string
	now        func() time.Time
	applyRun   applyRunner

	// applyStartedAt holds the unix-nano timestamp of the last self-update launch
	// (0 = none in flight). It guards against two concurrent /updates/apply calls
	// each launching a helper that would race the reinstall/restart, and
	// self-expires after applyGuardTTL so a helper that exits 0 but never restarts
	// the backend cannot wedge apply behind a permanent 409.
	applyStartedAt atomic.Int64

	// releaseURL is the GitHub endpoint hit by Check + the poller; defaults
	// to DefaultReleaseURL and can be overridden by SetReleaseURL for tests.
	releaseURL string

	// fetcher is the function used to retrieve the latest release. Defaults
	// to FetchLatestReleaseFrom(httpClient). Tests inject a deterministic
	// stub via SetFetcher so the synctest poller test does not block on
	// real network I/O (which sits outside the fake-time bubble and prevents
	// synctest.Wait from settling).
	fetcher Fetcher

	// mu protects pollerStarted/cancel/wg under concurrent Start calls.
	mu             sync.Mutex
	pollerStarted  bool
	pollerCancel   context.CancelFunc
	pollerWg       sync.WaitGroup
	pollerInterval time.Duration
}

// Fetcher abstracts the GitHub release call so tests can drive
// the poller and Check() without spinning up an httptest server.
type Fetcher func(ctx context.Context) (tag, url string, err error)

// Option customises Service construction without growing NewService's public
// parameter list.
type Option func(*Service)

// WithJobs wires the system job tracker used by Apply.
func WithJobs(tracker *jobs.Tracker) Option {
	return func(s *Service) {
		s.jobs = tracker
	}
}

// WithHomeDir sets the resolved Kandev home directory. It is used as a
// fallback for service metadata discovery.
func WithHomeDir(homeDir string) Option {
	return func(s *Service) {
		s.homeDir = homeDir
	}
}

// WithEnvReader overrides environment reads. Intended for tests.
func WithEnvReader(getenv func(string) string) Option {
	return func(s *Service) {
		if getenv != nil {
			s.getenv = getenv
		}
	}
}

// WithApplyRunner overrides helper launch. Intended for tests and e2e mocks.
func WithApplyRunner(r applyRunner) Option {
	return func(s *Service) {
		if r != nil {
			s.applyRun = r
		}
	}
}

// NewService constructs the updates service. When httpClient is nil, a fresh
// client carrying defaultClientTimeout is allocated — http.DefaultClient has
// no timeout, so handing it the poller would let a stalled socket hang a
// goroutine for hours. current is the running binary version (typically
// injected via ldflags); the sentinel "dev" disables UpdateAvailable.
func NewService(pool *db.Pool, current string, httpClient *http.Client, log *logger.Logger, opts ...Option) *Service {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultClientTimeout}
	}
	if log == nil {
		log = logger.Default()
	}
	homeDir := ""
	if dir, err := os.UserHomeDir(); err == nil {
		homeDir = filepath.Join(dir, ".kandev")
	}
	s := &Service{
		pool:           pool,
		current:        current,
		httpClient:     httpClient,
		log:            log,
		limiter:        NewLimiter(ManualCheckWindow),
		releaseURL:     DefaultReleaseURL,
		pollerInterval: PollInterval,
		homeDir:        homeDir,
		getenv:         os.Getenv,
		now:            time.Now,
	}
	s.fetcher = s.defaultFetcher
	s.applyRun = s.defaultApplyRunner
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// defaultFetcher delegates to FetchLatestReleaseFrom using the current
// releaseURL + httpClient. Re-evaluating both on each call honours
// SetReleaseURL after construction.
func (s *Service) defaultFetcher(ctx context.Context) (string, string, error) {
	s.mu.Lock()
	url := s.releaseURL
	s.mu.Unlock()
	return FetchLatestReleaseFrom(ctx, s.httpClient, url)
}

// SetFetcher overrides the GitHub fetch implementation. Intended for tests
// that need deterministic behaviour inside testing/synctest.
func (s *Service) SetFetcher(f Fetcher) {
	s.mu.Lock()
	if f == nil {
		s.fetcher = s.defaultFetcher
	} else {
		s.fetcher = f
	}
	s.mu.Unlock()
}

// SetReleaseURL overrides the GitHub endpoint used by the poller and Check().
// Intended for tests that point at a httptest stub server.
func (s *Service) SetReleaseURL(url string) {
	s.mu.Lock()
	s.releaseURL = url
	s.mu.Unlock()
}

// Get returns the last-known state from kandev_meta without contacting
// GitHub. Safe to call on every page load.
func (s *Service) Get() (UpdatesResponse, error) {
	version, url, checkedAt, err := persistence.ReadLatestVersion(s.pool.Reader())
	if err != nil {
		return UpdatesResponse{}, err
	}
	return s.buildResponse(version, url, checkedAt), nil
}

// Check forces a synchronous poll against GitHub. Rate-limited per process by
// the 30s Limiter. On success the result is persisted and returned. On
// failure the previously persisted values are returned unchanged.
func (s *Service) Check(ctx context.Context) (UpdatesResponse, error) {
	if ok, _ := s.limiter.Allow(); !ok {
		return UpdatesResponse{}, ErrRateLimited
	}
	return s.fetchAndPersist(ctx)
}

// Apply validates the cached update and current service install, writes an
// intent file, and queues the OS-manager helper that performs the actual
// package upgrade + service reinstall.
func (s *Service) Apply(ctx context.Context, confirm string) (string, error) {
	if confirm != "UPDATE" {
		return "", ErrApplyConfirm
	}
	if s.jobs == nil {
		return "", ErrApplyUnsupported
	}
	resp, metadata, err := s.applyPreflight()
	if err != nil {
		return "", err
	}
	// Claim the in-flight guard across the launch. A successful launch keeps it
	// held because the helper restarts the backend shortly (which clears it by
	// replacing the process); a launch error releases it for an immediate retry.
	// The guard self-expires after applyGuardTTL (see claimApplyGuard) so a
	// helper that exits 0 but never restarts cannot block apply forever.
	if !s.claimApplyGuard() {
		return "", ErrApplyInProgress
	}
	intentPath, intent, err := s.writeApplyIntent(resp, metadata)
	if err != nil {
		s.applyStartedAt.Store(0)
		return "", err
	}
	req := applyRequest{
		IntentPath: intentPath,
		Intent:     intent,
	}
	jobID := s.jobs.Start(ctx, "self-update", func(jobCtx context.Context) (map[string]interface{}, error) {
		result, runErr := s.applyRun(jobCtx, req)
		if runErr != nil {
			s.applyStartedAt.Store(0)
		}
		return result, runErr
	})
	return jobID, nil
}

// claimApplyGuard atomically reserves the self-update guard. It succeeds when no
// apply is in flight or the previous one started more than applyGuardTTL ago
// (assumed dead); it returns false while a recent apply still holds it.
func (s *Service) claimApplyGuard() bool {
	now := s.now().UnixNano()
	for {
		prev := s.applyStartedAt.Load()
		if prev != 0 && now-prev < applyGuardTTL.Nanoseconds() {
			return false
		}
		if s.applyStartedAt.CompareAndSwap(prev, now) {
			return true
		}
	}
}

// RetryAfter exposes the limiter's remaining window so the HTTP handler can
// surface a Retry-After value to clients.
func (s *Service) RetryAfter() time.Duration {
	_, retry := s.peekLimiter()
	return retry
}

func (s *Service) peekLimiter() (bool, time.Duration) {
	// peek without consuming: use a separate limiter with the same window/clock
	// is overkill; instead we synthesise a dry-run by inspecting state.
	s.limiter.mu.Lock()
	defer s.limiter.mu.Unlock()
	now := s.limiter.now()
	if s.limiter.last.IsZero() || now.Sub(s.limiter.last) >= s.limiter.window {
		return true, 0
	}
	retry := s.limiter.window - now.Sub(s.limiter.last)
	if retry <= 0 {
		retry = time.Nanosecond
	}
	return false, retry
}

// fetchAndPersist hits GitHub, persists the result on success, and always
// returns the now-current view of kandev_meta. A fetch failure preserves the
// previously persisted state and returns the underlying error.
func (s *Service) fetchAndPersist(ctx context.Context) (UpdatesResponse, error) {
	s.mu.Lock()
	fetch := s.fetcher
	s.mu.Unlock()

	tag, releaseURL, err := fetch(ctx)
	if err != nil {
		// Preserve persisted state; surface the error to caller.
		current, _ := s.Get()
		return current, err
	}
	now := time.Now().UTC()
	if werr := persistence.WriteLatestVersion(s.pool.Writer(), tag, releaseURL, now); werr != nil {
		s.log.Warn("updates: persist latest version failed", zap.Error(werr))
		return UpdatesResponse{}, werr
	}
	return s.buildResponse(tag, releaseURL, now), nil
}

func (s *Service) buildResponse(latest, url string, checkedAt time.Time) UpdatesResponse {
	install, _ := s.detectInstallState()
	return s.buildResponseFrom(install, latest, url, checkedAt)
}

// buildResponseFrom assembles the response from an already-detected install
// state. Apply reuses this so the ApplySupported gate and the intent file are
// derived from a single install-state snapshot rather than two independent
// reads (closing a narrow TOCTOU window).
func (s *Service) buildResponseFrom(install InstallStateResponse, latest, url string, checkedAt time.Time) UpdatesResponse {
	applySupported, reason := install.applySupport()
	return UpdatesResponse{
		Current:                s.current,
		Latest:                 latest,
		LatestURL:              url,
		LatestCheckedAt:        checkedAt,
		UpdateAvailable:        s.updateAvailable(latest),
		Install:                install,
		ApplySupported:         applySupported,
		ApplyUnsupportedReason: reason,
		ManualCommands:         manualCommands(install, latest),
	}
}

// updateAvailable returns true iff latest is a valid semver strictly greater
// than current. Current = "dev" or empty disables the flag.
func (s *Service) updateAvailable(latest string) bool {
	if latest == "" || !isValidSemver(latest) {
		return false
	}
	if s.current == "" || s.current == devVersion {
		return false
	}
	if !isValidSemver(s.current) {
		return false
	}
	return compareSemver(latest, s.current) > 0
}
