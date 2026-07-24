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
)

func TestPatchSettingsHidesInternalSaveFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/system")
	RegisterRoutes(group, NewHandler(HandlerConfig{
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
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/system"), NewHandler(HandlerConfig{
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

type failingSettingsManager struct{ err error }

func (f failingSettingsManager) GetSettings(context.Context) (StorageMaintenanceSettings, error) {
	return DefaultSettings(), nil
}

type staticSettingsManager struct{}

func (staticSettingsManager) GetSettings(context.Context) (StorageMaintenanceSettings, error) {
	return DefaultSettings(), nil
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

func (f failingSettingsManager) SaveSettingsWithConfirmations(
	context.Context,
	StorageMaintenanceSettings,
	SaveConfirmations,
) (StorageMaintenanceSettings, error) {
	return StorageMaintenanceSettings{}, f.err
}
