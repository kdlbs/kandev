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

type failingSettingsManager struct{ err error }

func (f failingSettingsManager) GetSettings(context.Context) (StorageMaintenanceSettings, error) {
	return DefaultSettings(), nil
}

func (f failingSettingsManager) SaveSettingsWithConfirmations(
	context.Context,
	StorageMaintenanceSettings,
	SaveConfirmations,
) (StorageMaintenanceSettings, error) {
	return StorageMaintenanceSettings{}, f.err
}
