package logbundle

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestBackendOnlyBundleContainsOnlyRetainedRegularLogFiles(t *testing.T) {
	home := t.TempDir()
	logDir := filepath.Join(home, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	writeTestFile(t, filepath.Join(logDir, "backend-logs.log"), "current\n")
	writeTestFile(t, filepath.Join(logDir, "backend-logs-2026-07-29.log"), "yesterday\n")
	writeTestFile(t, filepath.Join(logDir, "backend-logs-2026-07-27.log"), "expired\n")
	writeTestFile(t, filepath.Join(logDir, "notes.log"), "private\n")
	if err := os.Symlink(filepath.Join(logDir, "notes.log"), filepath.Join(logDir, "backend-logs-2026-07-28.log")); err != nil {
		t.Fatal(err)
	}

	service := newTestService(t, Config{HomeDir: home, Now: func() time.Time { return now }})
	created, reused, err := service.Create("user-1", []string{"backend"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if reused {
		t.Fatal("new job was unexpectedly reused")
	}
	job := waitForTerminalJob(t, service, "user-1", created.ID)
	if job.Status != StatusReady {
		t.Fatalf("status = %q, warnings = %v", job.Status, job.Warnings)
	}

	file, _, err := service.OpenArchive("user-1", created.ID)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer func() { _ = file.Close() }()
	stat, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(file, stat.Size())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range reader.File {
		names = append(names, entry.Name)
		if entry.Method != zip.Store {
			t.Fatalf("%s method = %d, want Store", entry.Name, entry.Method)
		}
	}
	slices.Sort(names)
	want := []string{
		"backend/backend-logs-2026-07-29.log",
		"backend/backend-logs.log",
		"manifest.json",
	}
	if !slices.Equal(names, want) {
		t.Fatalf("archive entries = %v, want %v", names, want)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Fatalf("archive mode = %o, want 600", stat.Mode().Perm())
	}
}

func TestCreateCoalescesEquivalentJobAndRejectsConflictingActiveJob(t *testing.T) {
	service := newTestService(t, Config{
		HomeDir:       t.TempDir(),
		CaptureWindow: time.Hour,
	})
	first, reused, err := service.Create("user-1", []string{"frontend", "backend"})
	if err != nil || reused {
		t.Fatalf("first create reused=%v err=%v", reused, err)
	}
	second, reused, err := service.Create("user-1", []string{"backend", "frontend"})
	if err != nil || !reused || second.ID != first.ID {
		t.Fatalf("equivalent create = %#v reused=%v err=%v", second, reused, err)
	}
	if _, _, err := service.Create("user-1", []string{"frontend"}); !IsKind(err, ErrorIdentityBusy) {
		t.Fatalf("conflicting create error = %v, want identity busy", err)
	}
}

func TestFrontendUploadBindsFirstStreamAndRequiresSequentialChunks(t *testing.T) {
	service := newTestService(t, Config{
		HomeDir:       t.TempDir(),
		CaptureWindow: time.Hour,
	})
	job, _, err := service.Create("user-1", []string{"frontend"})
	if err != nil {
		t.Fatal(err)
	}
	entry := json.RawMessage(`{"level":"error","message":"broken"}`)
	ignored, err := service.UploadChunk("user-1", job.ID, UploadChunk{
		BrowserID: "browser-a", CaptureStreamID: "stream-first", ChunkIndex: 0,
		Entries: []json.RawMessage{entry},
	})
	if err != nil || ignored {
		t.Fatalf("first chunk ignored=%v err=%v", ignored, err)
	}
	ignored, err = service.UploadChunk("user-1", job.ID, UploadChunk{
		BrowserID: "browser-a", CaptureStreamID: "stream-loser", ChunkIndex: 999,
		Entries: []json.RawMessage{entry},
	})
	if err != nil || !ignored {
		t.Fatalf("losing stream ignored=%v err=%v", ignored, err)
	}
	if _, err := service.UploadChunk("user-1", job.ID, UploadChunk{
		BrowserID: "browser-a", CaptureStreamID: "stream-first", ChunkIndex: 2,
	}); !IsKind(err, ErrorInvalid) {
		t.Fatalf("out-of-order error = %v, want invalid", err)
	}
}

func TestCreateEnforcesGlobalActiveJobLimit(t *testing.T) {
	service := newTestService(t, Config{HomeDir: t.TempDir(), CaptureWindow: time.Hour})
	for index := 0; index < maxActiveJobs; index++ {
		if _, _, err := service.Create("user-"+twoDigit(index), []string{"frontend"}); err != nil {
			t.Fatalf("create active job %d: %v", index, err)
		}
	}
	if _, _, err := service.Create("overflow", []string{"frontend"}); !IsKind(err, ErrorSaturated) {
		t.Fatalf("overflow error = %v, want saturated", err)
	}
}

func TestCreateRejectsInsufficientTemporaryDisk(t *testing.T) {
	service := newTestService(t, Config{
		HomeDir: t.TempDir(),
		AvailableBytes: func(string) (uint64, error) {
			return uint64(maxTemporaryDiskBytes - 1), nil
		},
	})
	if _, _, err := service.Create("user-1", []string{"backend"}); !IsKind(err, ErrorSaturated) {
		t.Fatalf("insufficient disk error = %v, want saturated", err)
	}
}

func TestActiveAndReadyJobsExpireAtTheirHardDeadlines(t *testing.T) {
	service := newTestService(t, Config{
		HomeDir: t.TempDir(), CaptureWindow: time.Hour,
		BuildLifetime: 10 * time.Millisecond, ReadyLifetime: 15 * time.Minute,
	})
	active, _, err := service.Create("user-1", []string{"frontend"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := service.Get("user-1", active.ID); !IsKind(err, ErrorGone) {
		t.Fatalf("expired active job error = %v, want gone", err)
	}
}

func TestFrontendCreateNotifiesOnlyThroughIdentityNotifier(t *testing.T) {
	service := newTestService(t, Config{HomeDir: t.TempDir(), CaptureWindow: time.Hour})
	notifier := &recordingNotifier{called: make(chan struct{}, 1)}
	service.SetNotifier(notifier)
	job, _, err := service.Create("user-1", []string{"frontend"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-notifier.called:
	case <-time.After(time.Second):
		t.Fatal("capture notification was not sent")
	}
	if notifier.owner != "user-1" {
		t.Fatalf("notifier owner = %q", notifier.owner)
	}
	if notifier.message == nil || notifier.message.Action != "system.logs.capture_requested" {
		t.Fatalf("notification = %#v", notifier.message)
	}
	var payload struct {
		BundleID           string `json:"bundle_id"`
		MaxChunkBytes      int    `json:"max_chunk_bytes"`
		MaxBrowserProfiles int    `json:"max_browser_profiles"`
	}
	if err := json.Unmarshal(notifier.message.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.BundleID != job.ID || payload.MaxChunkBytes != 1024*1024 ||
		payload.MaxBrowserProfiles != maxBrowserProfiles {
		t.Fatalf("capture payload = %#v", payload)
	}
}

type recordingNotifier struct {
	owner   string
	message *ws.Message
	called  chan struct{}
}

func (n *recordingNotifier) SendToIdentity(owner string, message *ws.Message) int {
	n.owner = owner
	n.message = message
	select {
	case n.called <- struct{}{}:
	default:
	}
	return 1
}

func newTestService(t *testing.T, config Config) *Service {
	t.Helper()
	if config.Log == nil {
		var err error
		config.Log, err = logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if config.CaptureWindow == 0 {
		config.CaptureWindow = 20 * time.Millisecond
	}
	service := New(config)
	ctx, cancel := context.WithCancel(context.Background())
	service.Start(ctx)
	t.Cleanup(func() {
		cancel()
		service.Stop()
	})
	return service
}

func waitForTerminalJob(t *testing.T, service *Service, owner, id string) JobView {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := service.Get(owner, id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status == StatusReady || job.Status == StatusPartial || job.Status == StatusFailed {
			return job
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not become terminal")
	return JobView{}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(file, contents); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
