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
