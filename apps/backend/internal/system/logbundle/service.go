// Package logbundle creates bounded, identity-owned diagnostic archives from
// retained backend log files and explicit browser console snapshots.
package logbundle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	defaultCaptureWindow  = 15 * time.Second
	defaultBuildLifetime  = 5 * time.Minute
	defaultReadyLifetime  = 15 * time.Minute
	maxActiveJobs         = 8
	maxBrowserProfiles    = 4
	maxBrowserEntries     = 10_000
	maxBrowserBytes       = 20 * 1024 * 1024
	maxFrontendBytes      = 80 * 1024 * 1024
	maxEntryBytes         = 64 * 1024
	maxCaptureMetadata    = 8 * 1024
	maxIdentifierBytes    = 256
	maxTemporaryDiskBytes = int64(384 * 1024 * 1024)
)

type IdentityNotifier interface {
	SendToIdentity(userID string, message *ws.Message) int
}

type Config struct {
	HomeDir        string
	Version        string
	Commit         string
	BuildTime      string
	Log            *logger.Logger
	Now            func() time.Time
	CaptureWindow  time.Duration
	BuildLifetime  time.Duration
	ReadyLifetime  time.Duration
	AvailableBytes func(path string) (uint64, error)
}

type Service struct {
	config Config

	mu       sync.Mutex
	jobs     map[string]*job
	active   map[string]string
	latest   map[string]string
	queue    chan string
	notifier IdentityNotifier
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	start    sync.Once
	stop     sync.Once
}

func New(config Config) *Service {
	if config.HomeDir == "" {
		config.HomeDir = "."
	}
	if config.Log == nil {
		config.Log = logger.Default()
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.CaptureWindow <= 0 {
		config.CaptureWindow = defaultCaptureWindow
	}
	if config.BuildLifetime <= 0 {
		config.BuildLifetime = defaultBuildLifetime
	}
	if config.ReadyLifetime <= 0 {
		config.ReadyLifetime = defaultReadyLifetime
	}
	if config.AvailableBytes == nil {
		config.AvailableBytes = availableDiskBytes
	}
	return &Service{
		config: config, jobs: make(map[string]*job), active: make(map[string]string),
		latest: make(map[string]string), queue: make(chan string, maxActiveJobs),
	}
}

func (s *Service) SetNotifier(notifier IdentityNotifier) {
	s.mu.Lock()
	s.notifier = notifier
	s.mu.Unlock()
}

func (s *Service) Start(parent context.Context) {
	s.start.Do(func() {
		s.ctx, s.cancel = context.WithCancel(parent)
		if err := prepareRootDirectory(s.rootDir()); err != nil {
			s.config.Log.Warn("Failed to prepare diagnostic bundle directory", zap.Error(err))
		}
		s.wg.Add(2)
		go s.buildWorker()
		go s.cleanupWorker()
	})
}

func (s *Service) Stop() {
	s.stop.Do(func() {
		if s.cancel != nil {
			s.cancel()
			s.wg.Wait()
		}
		s.mu.Lock()
		for _, item := range s.jobs {
			closeBrowserFiles(item)
		}
		s.mu.Unlock()
	})
}

func (s *Service) Create(owner string, sources []string) (JobView, bool, error) {
	normalized, sourceKey, err := normalizeSources(sources)
	if err != nil {
		return JobView{}, false, err
	}
	if owner == "" {
		return JobView{}, false, newError(ErrorInvalid, "authenticated owner is required")
	}
	now := s.config.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)

	latestKey := owner + "\x00" + sourceKey
	if existing, reused, admissionErr := s.admitLocked(owner, latestKey); reused || admissionErr != nil {
		return existing, reused, admissionErr
	}
	id, workDir, err := s.createWorkDirectory()
	if err != nil {
		return JobView{}, false, err
	}
	item := &job{
		ID: id, Owner: owner, Sources: normalized, SourceKey: sourceKey,
		CreatedAt: now, BuildDeadline: now.Add(s.config.BuildLifetime),
		WorkDir: workDir, Browsers: make(map[string]*browserCapture),
	}
	if slices.Contains(normalized, "frontend") {
		deadline := now.Add(s.config.CaptureWindow)
		item.Status = StatusCollecting
		item.CaptureDeadline = &deadline
	} else {
		item.Status = StatusBuilding
	}
	s.jobs[id] = item
	s.active[owner] = id
	s.latest[latestKey] = id

	if item.Status == StatusCollecting {
		go s.beginCollection(owner, id, *item.CaptureDeadline)
	} else {
		s.enqueueLocked(id)
	}
	return item.view(), false, nil
}

func (s *Service) admitLocked(owner, latestKey string) (JobView, bool, error) {
	if id := s.latest[latestKey]; id != "" {
		if existing := s.jobs[id]; existing != nil && existing.Status != StatusExpired {
			return existing.view(), true, nil
		}
	}
	if id := s.active[owner]; id != "" {
		if existing := s.jobs[id]; existing != nil && isActive(existing.Status) {
			return JobView{}, false, newError(ErrorIdentityBusy, "another diagnostic bundle is active")
		}
		delete(s.active, owner)
	}
	if s.activeCountLocked() >= maxActiveJobs {
		return JobView{}, false, newError(ErrorSaturated, "diagnostic bundle capacity is full")
	}
	return JobView{}, false, nil
}

func (s *Service) createWorkDirectory() (string, string, error) {
	if err := os.MkdirAll(s.rootDir(), 0o700); err != nil {
		return "", "", newError(ErrorSaturated, "diagnostic bundle storage unavailable")
	}
	available, err := s.config.AvailableBytes(s.rootDir())
	if err != nil || available < uint64(maxTemporaryDiskBytes) {
		return "", "", newError(ErrorSaturated, "insufficient temporary disk for a diagnostic bundle")
	}
	id, err := randomID()
	if err != nil {
		return "", "", err
	}
	workDir := filepath.Join(s.rootDir(), id)
	if err := os.Mkdir(workDir, 0o700); err != nil {
		return "", "", err
	}
	return id, workDir, nil
}

func (s *Service) Get(owner, id string) (JobView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(s.config.Now().UTC())
	item := s.ownedJobLocked(owner, id)
	if item == nil {
		return JobView{}, newError(ErrorNotFound, "diagnostic bundle not found")
	}
	if item.Status == StatusExpired {
		return item.view(), newError(ErrorGone, "diagnostic bundle expired")
	}
	return item.view(), nil
}

func (s *Service) OpenArchive(owner, id string) (*os.File, JobView, error) {
	s.mu.Lock()
	s.expireLocked(s.config.Now().UTC())
	item := s.ownedJobLocked(owner, id)
	if item == nil {
		s.mu.Unlock()
		return nil, JobView{}, newError(ErrorNotFound, "diagnostic bundle not found")
	}
	view := item.view()
	switch item.Status {
	case StatusExpired:
		s.mu.Unlock()
		return nil, view, newError(ErrorGone, "diagnostic bundle expired")
	case StatusReady, StatusPartial:
	default:
		s.mu.Unlock()
		return nil, view, newError(ErrorConflict, "diagnostic bundle is not ready")
	}
	path := item.ArchivePath
	s.mu.Unlock()
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, view, newError(ErrorGone, "diagnostic bundle archive is unavailable")
		}
		return nil, view, err
	}
	return file, view, nil
}

func (s *Service) UploadChunk(owner, id string, chunk UploadChunk) (bool, error) {
	if err := validateUploadIdentifiers(chunk.BrowserID, chunk.CaptureStreamID, chunk.ChunkIndex); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.collectingJobLocked(owner, id)
	if err != nil {
		return false, err
	}
	browser, ignored, _, err := s.claimBrowserLocked(item, chunk.BrowserID, chunk.CaptureStreamID, chunk.StorageMode)
	if err != nil || ignored {
		return ignored, err
	}
	if browser.Done || chunk.ChunkIndex != browser.NextChunk {
		return false, newError(ErrorInvalid, "frontend chunks must be sequential")
	}
	chunkBytes, err := validateChunkPayload(chunk)
	if err != nil {
		return false, err
	}
	if captureLimitExceeded(item, browser, len(chunk.Entries), chunkBytes) {
		browser.Truncated = true
		item.Partial = true
		addWarning(item, "frontend capture size limit reached")
		return false, newError(ErrorTooLarge, "frontend capture limit reached")
	}
	if err := writeEntries(browser.File, chunk.Entries); err != nil {
		return false, err
	}
	browser.NextChunk++
	browser.EntryCount += len(chunk.Entries)
	browser.Bytes += chunkBytes
	item.FrontendEntries += len(chunk.Entries)
	item.FrontendBytes += chunkBytes
	if chunk.Done {
		browser.Done = true
		browser.CaptureMetadata = append(json.RawMessage(nil), chunk.CaptureMetadata...)
		if err := browser.File.Close(); err != nil {
			return false, err
		}
		browser.File = nil
	}
	return false, nil
}

func validateChunkPayload(chunk UploadChunk) (int64, error) {
	if !chunk.Done && len(chunk.CaptureMetadata) != 0 && string(chunk.CaptureMetadata) != "null" {
		return 0, newError(ErrorInvalid, "capture metadata is allowed only on the final chunk")
	}
	if len(chunk.CaptureMetadata) > maxCaptureMetadata {
		return 0, newError(ErrorTooLarge, "capture metadata is too large")
	}
	var chunkBytes int64
	for _, entry := range chunk.Entries {
		if len(entry) > maxEntryBytes || !json.Valid(entry) {
			return 0, newError(ErrorTooLarge, "frontend entry is invalid or too large")
		}
		chunkBytes += int64(len(entry) + 1)
	}
	return chunkBytes, nil
}

func captureLimitExceeded(item *job, browser *browserCapture, entries int, bytes int64) bool {
	return browser.EntryCount+entries > maxBrowserEntries ||
		browser.Bytes+bytes > maxBrowserBytes ||
		item.FrontendBytes+bytes > maxFrontendBytes
}

func writeEntries(file *os.File, entries []json.RawMessage) error {
	for _, entry := range entries {
		if _, err := file.Write(entry); err != nil {
			return err
		}
		if _, err := file.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	return nil
}

// ClaimStream atomically reserves a browser profile for the first responding
// tab. Handlers call it before decoding the entries array so a losing tab is
// acknowledged without per-entry work.
func (s *Service) ClaimStream(
	owner, id, browserID, streamID, storageMode string,
) (ignored, newlyClaimed bool, err error) {
	if err := validateUploadIdentifiers(browserID, streamID, 0); err != nil {
		return false, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.collectingJobLocked(owner, id)
	if err != nil {
		return false, false, err
	}
	_, ignored, newlyClaimed, err = s.claimBrowserLocked(item, browserID, streamID, storageMode)
	return ignored, newlyClaimed, err
}

// ReleaseEmptyClaim rolls back a just-created reservation after request
// validation fails. It never removes a stream that has accepted a chunk.
func (s *Service) ReleaseEmptyClaim(owner, id, browserID, streamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.ownedJobLocked(owner, id)
	if item == nil {
		return
	}
	browser := item.Browsers[browserID]
	if browser == nil || browser.StreamID != streamID || browser.NextChunk != 0 || browser.EntryCount != 0 {
		return
	}
	if browser.File != nil {
		_ = browser.File.Close()
	}
	_ = os.Remove(browser.Path)
	delete(item.Browsers, browserID)
}

func (s *Service) collectingJobLocked(owner, id string) (*job, error) {
	s.expireLocked(s.config.Now().UTC())
	item := s.ownedJobLocked(owner, id)
	if item == nil {
		return nil, newError(ErrorNotFound, "diagnostic bundle not found")
	}
	if item.Status == StatusExpired {
		return nil, newError(ErrorGone, "diagnostic bundle expired")
	}
	if item.Status != StatusCollecting {
		return nil, newError(ErrorConflict, "frontend collection has ended")
	}
	return item, nil
}

func (s *Service) claimBrowserLocked(
	item *job, browserID, streamID, storageMode string,
) (*browserCapture, bool, bool, error) {
	browser := item.Browsers[browserID]
	if browser != nil && browser.StreamID != streamID {
		return browser, true, false, nil
	}
	if browser != nil {
		return browser, false, false, nil
	}
	if len(item.Browsers) >= maxBrowserProfiles {
		item.Partial = true
		addWarning(item, "frontend browser profile limit reached")
		return nil, false, false, newError(ErrorProfileLimit, "frontend browser profile limit reached")
	}
	browser = &browserCapture{
		ID: browserID, StreamID: streamID,
		Index: len(item.Browsers) + 1, StorageMode: storageMode,
	}
	browser.Path = filepath.Join(item.WorkDir, "frontend-"+twoDigit(browser.Index)+".jsonl")
	file, err := os.OpenFile(browser.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, false, false, err
	}
	browser.File = file
	item.Browsers[browserID] = browser
	return browser, false, true, nil
}

func validateUploadIdentifiers(browserID, streamID string, chunkIndex int) error {
	if browserID == "" || len(browserID) > maxIdentifierBytes ||
		streamID == "" || len(streamID) > maxIdentifierBytes ||
		chunkIndex < 0 {
		return newError(ErrorInvalid, "invalid frontend capture identifiers")
	}
	return nil
}

func (s *Service) beginCollection(owner, id string, deadline time.Time) {
	s.mu.Lock()
	notifier := s.notifier
	ctx := s.ctx
	s.mu.Unlock()
	if notifier != nil {
		message, err := ws.NewNotification("system.logs.capture_requested", map[string]any{
			"bundle_id": id, "capture_deadline": deadline,
			"max_chunk_bytes": 1024 * 1024, "max_browser_profiles": maxBrowserProfiles,
		})
		if err == nil {
			notifier.SendToIdentity(owner, message)
		}
	}
	delay := deadline.Sub(s.config.Now().UTC())
	timer := time.NewTimer(max(0, delay))
	defer timer.Stop()
	if ctx == nil {
		<-timer.C
	} else {
		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
	}
	s.finishCollection(id)
}

func (s *Service) finishCollection(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.jobs[id]
	if item == nil || item.Status != StatusCollecting {
		return
	}
	for _, browser := range item.Browsers {
		if browser.File != nil {
			_ = browser.File.Close()
			browser.File = nil
		}
		if !browser.Done {
			item.Partial = true
			addWarning(item, "frontend capture did not finish before the deadline")
		}
	}
	if len(item.Browsers) == 0 {
		item.Partial = true
		addWarning(item, "no frontend browser responded")
	}
	item.Status = StatusBuilding
	s.enqueueLocked(id)
}

func (s *Service) enqueueLocked(id string) {
	select {
	case s.queue <- id:
	default:
		item := s.jobs[id]
		item.Status = StatusFailed
		addWarning(item, "diagnostic bundle build queue is unavailable")
		delete(s.active, item.Owner)
	}
}

func (s *Service) buildWorker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case id := <-s.queue:
			s.build(id)
		}
	}
}

func (s *Service) cleanupWorker() {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			s.expireLocked(s.config.Now().UTC())
			s.mu.Unlock()
		}
	}
}

func (s *Service) expireLocked(now time.Time) {
	for _, item := range s.jobs {
		if isActive(item.Status) && !now.Before(item.BuildDeadline) {
			item.Status = StatusExpired
			closeBrowserFiles(item)
			delete(s.active, item.Owner)
			_ = os.RemoveAll(item.WorkDir)
			continue
		}
		if (item.Status == StatusReady || item.Status == StatusPartial || item.Status == StatusFailed) &&
			item.ExpiresAt != nil && !now.Before(*item.ExpiresAt) {
			item.Status = StatusExpired
			_ = os.RemoveAll(item.WorkDir)
		}
	}
}

func (s *Service) ownedJobLocked(owner, id string) *job {
	item := s.jobs[id]
	if item == nil || item.Owner != owner {
		return nil
	}
	return item
}

func (s *Service) activeCountLocked() int {
	count := 0
	for _, item := range s.jobs {
		if isActive(item.Status) {
			count++
		}
	}
	return count
}

func (s *Service) rootDir() string {
	return filepath.Join(s.config.HomeDir, "tmp", "diagnostic-bundles")
}

func normalizeSources(sources []string) ([]string, string, error) {
	if len(sources) == 0 || len(sources) > 2 {
		return nil, "", newError(ErrorInvalid, "sources must select backend and/or frontend")
	}
	seen := make(map[string]bool, len(sources))
	for _, source := range sources {
		if source != "backend" && source != "frontend" {
			return nil, "", newError(ErrorInvalid, "unsupported diagnostic source")
		}
		if seen[source] {
			return nil, "", newError(ErrorInvalid, "diagnostic sources must be unique")
		}
		seen[source] = true
	}
	normalized := make([]string, 0, len(seen))
	for source := range seen {
		normalized = append(normalized, source)
	}
	slices.Sort(normalized)
	return normalized, strings.Join(normalized, ","), nil
}

func randomID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func isActive(status Status) bool {
	return status == StatusCollecting || status == StatusBuilding
}

func addWarning(item *job, warning string) {
	if !slices.Contains(item.Warnings, warning) {
		item.Warnings = append(item.Warnings, warning)
	}
}

func closeBrowserFiles(item *job) {
	for _, browser := range item.Browsers {
		if browser.File != nil {
			_ = browser.File.Close()
			browser.File = nil
		}
	}
}

func twoDigit(value int) string {
	return string([]byte{'0' + byte(value/10), '0' + byte(value%10)})
}

func prepareRootDirectory(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return os.Mkdir(path, 0o700)
}
