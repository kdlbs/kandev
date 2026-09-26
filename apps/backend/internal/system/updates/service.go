// Package updates implements the kandev background updates poller and the
// HTTP surface for the System -> Updates page. It resolves stable releases
// from GitHub or nightlies from npm, persists isolated channel caches, and
// exposes a 30s rate-limited "check now" handler.
package updates

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/system/jobs"
)

// PollInterval is the cadence at which the background goroutine polls the
// selected update source.
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
	Current                  string               `json:"current"`
	Latest                   string               `json:"latest"`
	LatestURL                string               `json:"latest_url"`
	LatestCheckedAt          time.Time            `json:"latest_checked_at"`
	UpdateAvailable          bool                 `json:"update_available"`
	Channel                  Channel              `json:"channel"`
	ChannelEditable          bool                 `json:"channel_editable"`
	ChannelUnsupportedReason string               `json:"channel_unsupported_reason"`
	Install                  InstallStateResponse `json:"install"`
	ApplySupported           bool                 `json:"apply_supported"`
	ApplyUnsupportedReason   string               `json:"apply_unsupported_reason,omitempty"`
	ManualCommands           []string             `json:"manual_commands,omitempty"`
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
	settings   settingsStore

	// notifier routes update availability through the canonical notification
	// service. It is nil-safe for isolated updates tests and minimal startup.
	notifier UpdateNotifier

	// applyStartedAt holds the unix-nano timestamp of the last self-update launch
	// (0 = none in flight). It guards against two concurrent /updates/apply calls
	// each launching a helper that would race the reinstall/restart, and
	// self-expires after applyGuardTTL so a helper that exits 0 but never restarts
	// the backend cannot wedge apply behind a permanent 409.
	applyStartedAt atomic.Int64

	// updateMu serializes source resolution and cache persistence across the
	// poller, manual checks, channel changes, and apply preflight. A slower,
	// older resolution therefore cannot overwrite a newer operation's cache.
	updateMu sync.Mutex

	// releaseURL is the GitHub endpoint hit by Check + the poller; defaults
	// to DefaultReleaseURL and can be overridden by SetReleaseURL for tests.
	releaseURL string
	nightlyURL string

	// fetcher is the function used to retrieve the latest release. Defaults
	// to FetchLatestReleaseFrom(httpClient). Tests inject a deterministic
	// stub via SetFetcher so the synctest poller test does not block on
	// real network I/O (which sits outside the fake-time bubble and prevents
	// synctest.Wait from settling).
	fetcher        Fetcher
	nightlyFetcher Fetcher

	// mu protects pollerStarted/cancel/wg under concurrent Start calls.
	mu             sync.Mutex
	pollerStarted  bool
	pollerCancel   context.CancelFunc
	pollerWg       sync.WaitGroup
	pollerInterval time.Duration
}

// Fetcher abstracts update-source resolution so tests can drive the poller
// and Check() without spinning up an httptest server.
type Fetcher func(ctx context.Context) (tag, url string, err error)

// UpdateNotifier is the narrow canonical notification-service boundary used
// by release detection. Provider policy and occurrence de-duplication remain
// owned by that service.
type UpdateNotifier interface {
	HandleUpdateAvailable(ctx context.Context, version, releaseURL string)
}

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
		nightlyURL:     DefaultNPMRegistryURL,
		pollerInterval: PollInterval,
		homeDir:        homeDir,
		getenv:         os.Getenv,
		now:            time.Now,
	}
	s.fetcher = s.defaultFetcher
	s.nightlyFetcher = s.defaultNightlyFetcher
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

func (s *Service) defaultNightlyFetcher(ctx context.Context) (string, string, error) {
	s.mu.Lock()
	url := s.nightlyURL
	s.mu.Unlock()
	return FetchLatestNightlyFrom(ctx, s.httpClient, url)
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

// SetNightlyFetcher overrides npm nightly resolution. Intended for tests.
func (s *Service) SetNightlyFetcher(f Fetcher) {
	s.mu.Lock()
	if f == nil {
		s.nightlyFetcher = s.defaultNightlyFetcher
	} else {
		s.nightlyFetcher = f
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

// SetNightlyURL overrides the npm registry endpoint used by Check + poller.
func (s *Service) SetNightlyURL(url string) {
	s.mu.Lock()
	s.nightlyURL = url
	s.mu.Unlock()
}

// SetNotifier wires the canonical notifier after gateway composition.
func (s *Service) SetNotifier(notifier UpdateNotifier) {
	s.mu.Lock()
	s.notifier = notifier
	s.mu.Unlock()
}

// Get returns the selected channel's last-known state without contacting its
// upstream. Safe to call on every page load.
func (s *Service) Get(ctx context.Context) (UpdatesResponse, error) {
	install, _ := s.detectInstallState()
	channel, err := s.effectiveChannel(ctx, install)
	if err != nil {
		return UpdatesResponse{}, err
	}
	version, url, checkedAt, err := s.readLatestVersion(channel)
	if err != nil {
		return UpdatesResponse{}, err
	}
	return s.buildResponseFromChannel(channel, install, version, url, checkedAt), nil
}

// Check forces a synchronous poll against the effective channel's source.
// It is rate-limited per process by the 30s Limiter. On failure the selected
// channel's previously persisted values remain unchanged.
func (s *Service) Check(ctx context.Context) (UpdatesResponse, error) {
	if ok, _ := s.limiter.Allow(); !ok {
		return UpdatesResponse{}, ErrRateLimited
	}
	return s.fetchAndPersist(ctx)
}

// Apply validates that the expected target still matches the cached update and
// current service install, writes an exact-version intent, and queues the
// OS-manager helper that performs the package upgrade + service reinstall.
func (s *Service) Apply(ctx context.Context, confirm, expectedTarget string) (string, error) {
	if confirm != "UPDATE" {
		return "", ErrApplyConfirm
	}
	if s.jobs == nil {
		return "", ErrApplyUnsupported
	}
	// Claim the in-flight guard across the launch. A successful launch keeps it
	// held because the helper restarts the backend shortly (which clears it by
	// replacing the process); a launch error releases it for an immediate retry.
	// The guard self-expires after applyGuardTTL (see claimApplyGuard) so a
	// helper that exits 0 but never restarts cannot block apply forever.
	if !s.claimApplyGuard() {
		return "", ErrApplyInProgress
	}
	resp, metadata, err := s.applyPreflight(ctx, expectedTarget)
	if err != nil {
		s.applyStartedAt.Store(0)
		return "", err
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

// fetchAndPersist resolves the effective channel and persists its isolated
// cache on success. A fetch failure preserves both caches.
func (s *Service) fetchAndPersist(ctx context.Context) (UpdatesResponse, error) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	install, _ := s.detectInstallState()
	channel, err := s.effectiveChannel(ctx, install)
	if err != nil {
		return UpdatesResponse{}, err
	}
	tag, releaseURL, err := s.resolveLatest(ctx, channel)
	if err != nil {
		// Preserve persisted state without re-reading settings through a context
		// the failed upstream request may already have canceled.
		version, url, checkedAt, readErr := s.readLatestVersion(channel)
		if readErr != nil {
			s.log.Warn("updates: read cached version after fetch failure", zap.Error(readErr))
			return UpdatesResponse{}, err
		}
		return s.buildResponseFromChannel(channel, install, version, url, checkedAt), err
	}
	now := s.now().UTC()
	if werr := s.writeLatestVersion(channel, tag, releaseURL, now); werr != nil {
		s.log.Warn("updates: persist latest version failed", zap.Error(werr))
		return UpdatesResponse{}, werr
	}
	s.notifyUpdateAvailableFor(ctx, channel, tag, releaseURL)
	return s.buildResponseFromChannel(channel, install, tag, releaseURL, now), nil
}

func (s *Service) resolveLatest(ctx context.Context, channel Channel) (string, string, error) {
	s.mu.Lock()
	fetch := s.fetcher
	if channel == ChannelNightly {
		fetch = s.nightlyFetcher
	}
	s.mu.Unlock()
	return fetch(ctx)
}

// ReplayCachedUpdate delivers a previously persisted newer release without
// contacting the selected upstream. It is called when the default user becomes
// eligible for Local delivery after an early startup poll.
func (s *Service) ReplayCachedUpdate(ctx context.Context) error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	install, _ := s.detectInstallState()
	channel, err := s.effectiveChannel(ctx, install)
	if err != nil {
		return err
	}
	tag, releaseURL, _, err := s.readLatestVersion(channel)
	if err != nil {
		return err
	}
	s.notifyUpdateAvailableFor(ctx, channel, tag, releaseURL)
	return nil
}

func (s *Service) notifyUpdateAvailableFor(ctx context.Context, channel Channel, tag, releaseURL string) {
	if !s.notificationAvailableFor(channel, tag) {
		return
	}
	s.mu.Lock()
	notifier := s.notifier
	s.mu.Unlock()
	if notifier == nil {
		return
	}
	notifier.HandleUpdateAvailable(ctx, tag, releaseURL)
}

func (s *Service) notificationAvailableFor(channel Channel, latest string) bool {
	if !s.updateAvailableFor(channel, latest) {
		return false
	}
	// Returning from a nightly to an older stable is an explicit user action,
	// not a newly available upgrade. Keep it actionable in the UI without
	// broadcasting an upgrade notification.
	if channel == ChannelStable && isNightlyVersion(s.current) && compareSemver(latest, s.current) <= 0 {
		return false
	}
	return true
}

// buildResponseFromChannel assembles a response from one install-state
// snapshot so capability gates and apply intent stay consistent.
func (s *Service) buildResponseFromChannel(channel Channel, install InstallStateResponse, latest, url string, checkedAt time.Time) UpdatesResponse {
	applySupported, reason := install.applySupport()
	channelEditable, channelReason := s.channelSupport(install)
	return UpdatesResponse{
		Current:                  s.current,
		Latest:                   latest,
		LatestURL:                url,
		LatestCheckedAt:          checkedAt,
		UpdateAvailable:          s.updateAvailableFor(channel, latest),
		Channel:                  channel,
		ChannelEditable:          channelEditable,
		ChannelUnsupportedReason: channelReason,
		Install:                  install,
		ApplySupported:           applySupported,
		ApplyUnsupportedReason:   reason,
		ManualCommands:           manualCommands(install, latest),
	}
}

// updateAvailable returns true iff latest is a valid semver strictly greater
// than current. Current = "dev" or empty disables the flag.
func (s *Service) updateAvailable(latest string) bool {
	return s.updateAvailableFor(ChannelStable, latest)
}

func (s *Service) updateAvailableFor(channel Channel, latest string) bool {
	if latest == "" || !isValidSemver(latest) {
		return false
	}
	if s.current == "" || s.current == devVersion {
		return false
	}
	if !isValidSemver(s.current) {
		return false
	}
	current := strings.TrimPrefix(s.current, "v")
	target := strings.TrimPrefix(latest, "v")
	if current == target {
		return false
	}
	if channel == ChannelNightly && isNightlyVersion(s.current) && isNightlyVersion(latest) {
		return compareSemverCore(latest, s.current) >= 0
	}
	if channel == ChannelStable && isNightlyVersion(s.current) && !isNightlyVersion(latest) {
		return true
	}
	return compareSemver(latest, s.current) > 0
}

func (s *Service) readLatestVersion(channel Channel) (string, string, time.Time, error) {
	if channel == ChannelNightly {
		return persistence.ReadLatestNightlyVersion(s.pool.Reader())
	}
	return persistence.ReadLatestVersion(s.pool.Reader())
}

func (s *Service) writeLatestVersion(channel Channel, version, url string, checkedAt time.Time) error {
	if channel == ChannelNightly {
		return persistence.WriteLatestNightlyVersion(s.pool.Writer(), version, url, checkedAt)
	}
	return persistence.WriteLatestVersion(s.pool.Writer(), version, url, checkedAt)
}
