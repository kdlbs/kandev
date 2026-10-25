package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
)

// newTestRouter mirrors the production wiring in internal/system: a read group
// plus an admin group guarded by authn.RequireAdmin. The identity is an admin
// (as the synthetic single-user identity is when auth is disabled), so these
// handler tests exercise behavior rather than the role guard, which
// internal/system's route tests own.
func newTestRouter(handler *Handler) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin})
		c.Next()
	})
	read := router.Group("/api/v1/system")
	RegisterRoutes(read, read.Group("", authn.RequireAdmin()), handler)
	return router
}

func TestPatchSettingsHidesInternalSaveFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestRouter(NewHandler(HandlerConfig{
		Settings: failingSettingsManager{err: errors.New("database credentials leaked")},
	}))
	body, err := json.Marshal(map[string]any{"settings": DefaultSettings()})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPatch, "/api/v1/system/storage/settings", bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	if strings.Contains(response.Body.String(), "credentials") {
		t.Fatalf("response exposed internal failure: %s", response.Body.String())
	}
}

func TestGetStorageReturnsSnapshotAnalyzedAt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	analyzedAt := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	router := newTestRouter(NewHandler(HandlerConfig{
		Settings: staticSettingsManager{}, Runs: staticRunLister{},
		Overview: staticCachedOverview{snapshot: OverviewSnapshot{
			Summary: Summary{Workspaces: map[string]any{"bytes": 42}}, AnalyzedAt: analyzedAt,
		}},
	}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var body struct {
		Summary    Summary   `json:"summary"`
		AnalyzedAt time.Time `json:"analyzed_at"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.AnalyzedAt.Equal(analyzedAt) {
		t.Fatalf("analyzed_at = %s, want %s", body.AnalyzedAt, analyzedAt)
	}
	if body.Summary.Workspaces.(map[string]any)["bytes"] != float64(42) {
		t.Fatalf("summary = %#v", body.Summary)
	}
}

func TestGetStorageReturnsBeforeColdOverviewScanCompletes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := newProgressiveOverview()
	cache := NewOverviewCache(provider)
	router := newTestRouter(NewHandler(HandlerConfig{
		Settings: staticSettingsManager{}, Runs: staticRunLister{}, Overview: cache,
	}))

	responseCh := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage", nil))
		responseCh <- response
	}()
	<-provider.started

	var response *httptest.ResponseRecorder
	select {
	case response = <-responseCh:
	case <-time.After(100 * time.Millisecond):
		close(provider.release)
		t.Fatal("storage overview request waited for the cold scan")
	}
	close(provider.release)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var body struct {
		Summary  *Summary             `json:"summary"`
		Analysis StorageAnalysisState `json:"analysis"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Summary != nil || body.Analysis.State != AnalysisStateScanning {
		t.Fatalf("cold response = %#v, want scanning state without summary", body)
	}
}

func TestGetStorageDiskReturnsIndependentCapacityResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestRouter(NewHandler(HandlerConfig{}))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/disk", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var body struct {
		Available bool `json:"available"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Available {
		t.Fatalf("unconfigured disk reader should be unavailable: %s", response.Body.String())
	}
}

func TestGetStorageDiskReturnsMeasuredFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestRouter(NewHandler(HandlerConfig{
		DiskPath: "/data",
		DiskCapacity: func(context.Context, string) (DiskCapacity, error) {
			return DiskCapacity{
				TotalBytes: 1000, UsedBytes: 750, AvailableBytes: 250, UsedPercent: 75,
			}, nil
		},
	}))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/disk", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var body DiskCapacity
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := DiskCapacity{
		Path: "/data", TotalBytes: 1000, UsedBytes: 750, AvailableBytes: 250, UsedPercent: 75, Available: true,
	}
	if body != want {
		t.Fatalf("disk response = %#v, want %#v", body, want)
	}
}

func TestGetStorageDiskIncludesMeasurementTimestampAndTemporaryRoots(t *testing.T) {
	gin.SetMode(gin.TestMode)
	observedAt := time.Date(2026, time.September, 28, 10, 30, 0, 0, time.UTC)
	router := newTestRouter(NewHandler(HandlerConfig{
		DiskPath: "/data",
		DiskCapacity: func(_ context.Context, path string) (DiskCapacity, error) {
			switch path {
			case "/data":
				return DiskCapacity{TotalBytes: 1000, UsedBytes: 750, AvailableBytes: 250, UsedPercent: 75}, nil
			case "/tmp":
				return DiskCapacity{TotalBytes: 500, UsedBytes: 500, AvailableBytes: 0, UsedPercent: 100}, nil
			default:
				return DiskCapacity{}, errors.New("statfs failed")
			}
		},
		DiskRoots: func(context.Context) ([]DiskRootCandidate, error) {
			return []DiskRootCandidate{
				{RequestedPath: "/var/tmp", Path: "/tmp", Aliases: []string{"/tmp"}},
				{RequestedPath: "/missing", Path: "/missing"},
			}, nil
		},
		DiskIdentity: func(_ context.Context, path string) (string, error) {
			if path == "/data" {
				return "home-device", nil
			}
			if path == "/tmp" {
				return "temp-device", nil
			}
			return "", errors.New("identity unavailable")
		},
		Now: func() time.Time { return observedAt },
	}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/disk", nil))

	var body DiskCapacityResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.ObservedAt.Equal(observedAt) {
		t.Fatalf("observed_at = %s, want %s", body.ObservedAt, observedAt)
	}
	if len(body.TemporaryRoots) != 2 {
		t.Fatalf("temporary root count = %d, want 2: %#v", len(body.TemporaryRoots), body.TemporaryRoots)
	}
	full := body.TemporaryRoots[0]
	if full.RequestedPath != "/var/tmp" || full.Path != "/tmp" || len(full.Aliases) != 1 || full.Aliases[0] != "/tmp" {
		t.Fatalf("temporary root path data = %#v", full)
	}
	if !full.Available || full.AvailableBytes != 0 || full.UsedPercent != 100 || full.SharedWithHome == nil || *full.SharedWithHome {
		t.Fatalf("full separate temporary root = %#v", full)
	}
	if !full.ObservedAt.Equal(observedAt) {
		t.Fatalf("temporary observed_at = %s, want %s", full.ObservedAt, observedAt)
	}
	if body.TemporaryRoots[1].Available || body.TemporaryRoots[1].Warning == "" {
		t.Fatalf("failed temporary root = %#v, want independent unavailable result", body.TemporaryRoots[1])
	}
}

func TestGetStorageDiskCollapsesKnownSharedTemporaryFilesystems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reads := map[string]int{}
	router := newTestRouter(NewHandler(HandlerConfig{
		DiskPath: "/home",
		DiskCapacity: func(_ context.Context, path string) (DiskCapacity, error) {
			reads[path]++
			return DiskCapacity{TotalBytes: 100, UsedBytes: 70, AvailableBytes: 30, UsedPercent: 70}, nil
		},
		DiskRoots: func(context.Context) ([]DiskRootCandidate, error) {
			return []DiskRootCandidate{
				{RequestedPath: "/tmp", Path: "/tmp"},
				{RequestedPath: "/var/tmp", Path: "/var/tmp"},
				{RequestedPath: "/ignored", Path: "/ignored"},
			}, nil
		},
		DiskIdentity: func(context.Context, string) (string, error) { return "shared-filesystem", nil },
	}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/disk", nil))

	var body DiskCapacityResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.TemporaryRoots) != 1 {
		t.Fatalf("temporary roots = %#v, want one shared measurement", body.TemporaryRoots)
	}
	root := body.TemporaryRoots[0]
	if root.SharedWithHome == nil || !*root.SharedWithHome {
		t.Fatalf("shared_with_home = %v, want true", root.SharedWithHome)
	}
	if len(root.Aliases) != 1 || root.Aliases[0] != "/var/tmp" {
		t.Fatalf("aliases = %#v, want the second temporary path", root.Aliases)
	}
	if reads["/tmp"] != 1 || reads["/var/tmp"] != 1 || reads["/ignored"] != 0 {
		t.Fatalf("capacity reads = %#v, want each of the first two candidates measured", reads)
	}
}

func TestGetStorageDiskUsesHealthyCandidateAfterSharedFilesystemReadFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reads := map[string]int{}
	router := newTestRouter(NewHandler(HandlerConfig{
		DiskPath: "/home",
		DiskCapacity: func(_ context.Context, path string) (DiskCapacity, error) {
			reads[path]++
			switch path {
			case "/tmp":
				return DiskCapacity{}, errors.New("first alias unavailable")
			case "/var/tmp":
				return DiskCapacity{TotalBytes: 200, UsedBytes: 50, AvailableBytes: 150, UsedPercent: 25}, nil
			default:
				return measuredDiskCapacity(), nil
			}
		},
		DiskRoots: func(context.Context) ([]DiskRootCandidate, error) {
			return []DiskRootCandidate{
				{RequestedPath: "/tmp", Path: "/tmp"},
				{RequestedPath: "/var/tmp", Path: "/var/tmp"},
			}, nil
		},
		DiskIdentity: func(_ context.Context, path string) (string, error) {
			if path == "/home" {
				return "home-filesystem", nil
			}
			return "shared-temp-filesystem", nil
		},
	}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/disk", nil))

	var body DiskCapacityResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.TemporaryRoots) != 1 {
		t.Fatalf("temporary roots = %#v, want one deduplicated measurement", body.TemporaryRoots)
	}
	root := body.TemporaryRoots[0]
	if !root.Available || root.Path != "/tmp" || root.UsedBytes != 50 || root.UsedPercent != 25 {
		t.Fatalf("temporary root = %#v, want the healthy alias measurement", root)
	}
	if len(root.Aliases) != 1 || root.Aliases[0] != "/var/tmp" {
		t.Fatalf("aliases = %#v, want /var/tmp", root.Aliases)
	}
	if reads["/tmp"] != 1 || reads["/var/tmp"] != 1 {
		t.Fatalf("candidate capacity reads = %#v, want both same-filesystem candidates measured", reads)
	}
}

func TestGetStorageDiskTimestampsCapacityAfterEachMeasurement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clock := time.Date(2026, time.September, 28, 10, 30, 0, 0, time.UTC)
	measuredAt := map[string]time.Time{}
	router := newTestRouter(NewHandler(HandlerConfig{
		DiskPath: "/home",
		DiskCapacity: func(_ context.Context, path string) (DiskCapacity, error) {
			measuredAt[path] = clock
			clock = clock.Add(time.Minute)
			return measuredDiskCapacity(), nil
		},
		DiskRoots: func(context.Context) ([]DiskRootCandidate, error) {
			return []DiskRootCandidate{{RequestedPath: "/tmp", Path: "/tmp"}}, nil
		},
		DiskIdentity: func(context.Context, string) (string, error) { return "filesystem", nil },
		Now:          func() time.Time { return clock },
	}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/disk", nil))

	var body DiskCapacityResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.ObservedAt.After(measuredAt["/home"]) {
		t.Fatalf("home observed_at = %s, want after measurement at %s", body.ObservedAt, measuredAt["/home"])
	}
	if len(body.TemporaryRoots) != 1 || !body.TemporaryRoots[0].ObservedAt.After(measuredAt["/tmp"]) {
		t.Fatalf("temporary roots = %#v, want timestamp after /tmp measurement at %s", body.TemporaryRoots, measuredAt["/tmp"])
	}
}

func TestGetStorageDiskKeepsTemporaryCapacityWhenHomeReadFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestRouter(NewHandler(HandlerConfig{
		DiskPath: "/home",
		DiskCapacity: func(_ context.Context, path string) (DiskCapacity, error) {
			if path == "/home" {
				return DiskCapacity{}, errors.New("home statfs failed")
			}
			return DiskCapacity{TotalBytes: 100, UsedBytes: 75, AvailableBytes: 25, UsedPercent: 75}, nil
		},
		DiskRoots: func(context.Context) ([]DiskRootCandidate, error) {
			return []DiskRootCandidate{{RequestedPath: "/tmp", Path: "/tmp"}}, nil
		},
		DiskIdentity: func(context.Context, string) (string, error) {
			return "", errors.New("identity unavailable")
		},
	}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/disk", nil))

	var body DiskCapacityResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Available || len(body.TemporaryRoots) != 1 || !body.TemporaryRoots[0].Available {
		t.Fatalf("disk response = %#v, want unavailable home and measured temporary capacity", body)
	}
	if body.TemporaryRoots[0].SharedWithHome != nil {
		t.Fatalf("unknown filesystem relationship = %v, want null", body.TemporaryRoots[0].SharedWithHome)
	}
}

func TestGetStorageDiskLogsReaderErrorsAndReturnsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var loggedMessage string
	router := newTestRouter(NewHandler(HandlerConfig{
		DiskPath: "/data",
		DiskCapacity: func(context.Context, string) (DiskCapacity, error) {
			return DiskCapacity{}, errors.New("statfs failed")
		},
		LogError: func(message string, _ error) { loggedMessage = message },
	}))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/disk", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if loggedMessage != "failed to read storage disk capacity" {
		t.Fatalf("logged message = %q", loggedMessage)
	}
	var body DiskCapacity
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Available || body.Warning == "" {
		t.Fatalf("error response = %#v, want unavailable warning", body)
	}
}

func TestGetStorageSettingsReturnsPolicyWithoutOverviewScan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := DefaultSettings()
	capabilities := Capabilities{
		ManagedGoCachePath:       "/data/cache/go-build",
		GoCacheAdoptionAvailable: true,
		DockerAvailable:          true,
		DockerHost:               "unix:///var/run/docker.sock",
		HostGlobalDockerCleanup:  true,
	}
	overview := &recordingOverviewReader{capabilities: capabilities}
	router := newTestRouter(NewHandler(HandlerConfig{
		Settings: staticSettingsManager{settings: settings},
		Overview: overview,
	}))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/settings", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var body struct {
		Settings     StorageMaintenanceSettings `json:"settings"`
		Capabilities Capabilities               `json:"capabilities"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Settings != settings {
		t.Fatalf("settings = %#v, want %#v", body.Settings, settings)
	}
	if body.Capabilities != capabilities {
		t.Fatalf("capabilities = %#v, want %#v", body.Capabilities, capabilities)
	}
	if overview.settingsCapabilitiesCalls != 1 {
		t.Fatalf("settings capabilities calls = %d, want 1", overview.settingsCapabilitiesCalls)
	}
	if overview.capabilitiesCalls != 0 {
		t.Fatalf("full capabilities calls = %d, want 0", overview.capabilitiesCalls)
	}
	if overview.getCalls != 0 {
		t.Fatalf("overview scans = %d, want 0", overview.getCalls)
	}
}

func TestGetStorageSettingsHidesInternalLoadFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	internalErr := errors.New("database credentials leaked")
	var loggedMessage string
	var loggedErr error
	router := newTestRouter(NewHandler(HandlerConfig{
		Settings: failingSettingsManager{getErr: internalErr},
		LogError: func(message string, err error) {
			loggedMessage, loggedErr = message, err
		},
	}))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/storage/settings", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	if strings.Contains(response.Body.String(), "credentials") ||
		!strings.Contains(response.Body.String(), "failed to load storage settings") {
		t.Fatalf("response did not use a client-safe message: %s", response.Body.String())
	}
	if loggedMessage != "failed to load storage settings" || !errors.Is(loggedErr, internalErr) {
		t.Fatalf("logged error = (%q, %v), want original error", loggedMessage, loggedErr)
	}
}

func TestDeleteQuarantineBulkValidatesConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mutations := &recordingMutations{}
	router := newTestRouter(NewHandler(HandlerConfig{Mutations: mutations}))

	for _, test := range []struct {
		body string
		want int
	}{
		{`{"scope":"eligible","confirm":"DELETE ALL NOW"}`, http.StatusBadRequest},
		{`{"scope":"all","confirm":"DELETE ELIGIBLE"}`, http.StatusBadRequest},
		{`{"scope":"eligible","confirm":"DELETE ELIGIBLE"}`, http.StatusAccepted},
		{`{"scope":"all","confirm":"DELETE ALL NOW"}`, http.StatusAccepted},
	} {
		request := httptest.NewRequest(http.MethodDelete, "/api/v1/system/storage/quarantine", strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("body %s status = %d, want %d", test.body, response.Code, test.want)
		}
	}
	if mutations.purgeCalls != 2 {
		t.Fatalf("purge calls = %d, want 2", mutations.purgeCalls)
	}
}

type recordingMutations struct{ purgeCalls int }

func (m *recordingMutations) AdoptGoCache(context.Context, string, string) (StorageMaintenanceSettings, Capabilities, error) {
	return DefaultSettings(), Capabilities{}, nil
}
func (m *recordingMutations) Analyze(context.Context) (string, error) { return "job", nil }
func (m *recordingMutations) RunNow(context.Context, []string, bool) (string, error) {
	return "job", nil
}
func (m *recordingMutations) RestoreQuarantine(context.Context, string) (QuarantineEntry, error) {
	return QuarantineEntry{}, nil
}
func (m *recordingMutations) DeleteQuarantine(context.Context, string, string) (string, error) {
	return "job", nil
}
func (m *recordingMutations) PurgeQuarantine(context.Context, QuarantinePurgeScope, string) (string, error) {
	m.purgeCalls++
	return "job", nil
}

type failingSettingsManager struct {
	err    error
	getErr error
}

func (f failingSettingsManager) GetSettings(context.Context) (StorageMaintenanceSettings, error) {
	if f.getErr != nil {
		return DefaultSettings(), f.getErr
	}
	return DefaultSettings(), nil
}

type staticSettingsManager struct{ settings StorageMaintenanceSettings }

func (s staticSettingsManager) GetSettings(context.Context) (StorageMaintenanceSettings, error) {
	if s.settings == (StorageMaintenanceSettings{}) {
		return DefaultSettings(), nil
	}
	return s.settings, nil
}

func (staticSettingsManager) SaveSettingsWithConfirmations(context.Context, StorageMaintenanceSettings, SaveConfirmations) (StorageMaintenanceSettings, error) {
	return DefaultSettings(), nil
}

type staticRunLister struct{}

func (staticRunLister) ListRuns(context.Context, int) ([]MaintenanceRun, error) { return nil, nil }

type staticCachedOverview struct{ snapshot OverviewSnapshot }

func (o staticCachedOverview) Get(context.Context) (OverviewSnapshot, error) { return o.snapshot, nil }

func (o staticCachedOverview) Capabilities(context.Context, StorageMaintenanceSettings) Capabilities {
	return Capabilities{}
}

func (o staticCachedOverview) SettingsCapabilities(
	context.Context,
	StorageMaintenanceSettings,
) Capabilities {
	return Capabilities{}
}

type recordingOverviewReader struct {
	capabilities              Capabilities
	getCalls                  int
	capabilitiesCalls         int
	settingsCapabilitiesCalls int
}

func (o *recordingOverviewReader) Get(context.Context) (OverviewSnapshot, error) {
	o.getCalls++
	return OverviewSnapshot{}, nil
}

func (o *recordingOverviewReader) Capabilities(context.Context, StorageMaintenanceSettings) Capabilities {
	o.capabilitiesCalls++
	return o.capabilities
}

func (o *recordingOverviewReader) SettingsCapabilities(
	context.Context,
	StorageMaintenanceSettings,
) Capabilities {
	o.settingsCapabilitiesCalls++
	return o.capabilities
}

func (f failingSettingsManager) SaveSettingsWithConfirmations(
	context.Context,
	StorageMaintenanceSettings,
	SaveConfirmations,
) (StorageMaintenanceSettings, error) {
	return StorageMaintenanceSettings{}, f.err
}
