package system

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	"github.com/kandev/kandev/internal/system/storage"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

func TestRegisterRoutesExposesStorageSummary(t *testing.T) {
	router := newStorageRoutesTestRouter(t)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/storage", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/system/storage status = %d, want 200", response.Code)
	}
}

func TestStorageSettingsRejectInvalidPatch(t *testing.T) {
	router := newStorageRoutesTestRouter(t)
	settings := storage.DefaultSettings()
	settings.CheckIntervalHours = storage.MaxCheckIntervalHours + 1
	body, err := json.Marshal(map[string]any{"settings": settings})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/system/storage/settings", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("PATCH invalid storage settings status = %d, want 400", response.Code)
	}
}

func TestStorageCollectionRoutesAndConfirmationGates(t *testing.T) {
	router := newStorageRoutesTestRouter(t)
	tests := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{method: http.MethodGet, path: "/api/v1/system/storage/runs", want: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/system/storage/quarantine", want: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/system/storage/go-cache/adopt", body: `{"path":"/tmp/cache","confirm":"WRONG"}`, want: http.StatusBadRequest},
		{method: http.MethodDelete, path: "/api/v1/system/storage/quarantine/entry-1", body: `{"confirm":"WRONG"}`, want: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d", response.Code, test.want)
			}
		})
	}
}

func TestStorageAsyncRoutesAndBusyResponse(t *testing.T) {
	mutations := &fakeStorageMutations{}
	router := newStorageRoutesTestRouterWithMutations(t, mutations)
	for _, path := range []string{"/api/v1/system/storage/analyze", "/api/v1/system/storage/run"} {
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("POST %s status = %d, want 202", path, response.Code)
		}
	}

	mutations.busy = true
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/storage/run", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("busy run status = %d, want 409", response.Code)
	}
}

func TestStorageRestoreConflictReturnsConflict(t *testing.T) {
	mutations := &fakeStorageMutations{restoreErr: storage.ErrConflict}
	router := newStorageRoutesTestRouterWithMutations(t, mutations)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/system/storage/quarantine/entry-1/restore",
		nil,
	)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("restore conflict status = %d, want 409", response.Code)
	}
}

func TestStorageEarlyQuarantineDeleteReturnsConflict(t *testing.T) {
	mutations := &fakeStorageMutations{deleteErr: storage.ErrConflict}
	router := newStorageRoutesTestRouterWithMutations(t, mutations)
	request := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/system/storage/quarantine/entry-1",
		bytes.NewBufferString(`{"confirm":"DELETE"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("early delete status = %d, want 409", response.Code)
	}
}

func newStorageRoutesTestRouter(t *testing.T) *gin.Engine {
	return newStorageRoutesTestRouterWithMutations(t, &fakeStorageMutations{})
}

func newStorageRoutesTestRouterWithMutations(t *testing.T, mutations storage.Mutations) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	pool := db.NewPool(connection, connection)
	rawSettings, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	storageStore, err := storage.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{Storage: storage.NewHandler(storage.HandlerConfig{
		Settings: storage.NewSettingsStore(rawSettings), Runs: storageStore,
		Quarantine: storageStore, Overview: emptyStorageOverview{}, Mutations: mutations,
	})}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	service.RegisterRoutes(router, log)
	return router
}

type fakeStorageMutations struct {
	busy       bool
	restoreErr error
	deleteErr  error
}

func (f *fakeStorageMutations) AdoptGoCache(_ context.Context, _ string, _ string) (storage.StorageMaintenanceSettings, storage.Capabilities, error) {
	return storage.DefaultSettings(), storage.Capabilities{}, nil
}

func (f *fakeStorageMutations) Analyze(context.Context) (string, error) { return "analysis-job", nil }

func (f *fakeStorageMutations) RunNow(context.Context, []string) (string, error) {
	if f.busy {
		return "", &storage.BusyError{}
	}
	return "cleanup-job", nil
}

func (f *fakeStorageMutations) RestoreQuarantine(context.Context, string) (storage.QuarantineEntry, error) {
	return storage.QuarantineEntry{}, f.restoreErr
}

func (f *fakeStorageMutations) DeleteQuarantine(context.Context, string, string) (string, error) {
	return "delete-job", f.deleteErr
}

type emptyStorageOverview struct{}

func (emptyStorageOverview) Summary(context.Context) (storage.Summary, error) {
	return storage.Summary{}, nil
}

func (emptyStorageOverview) Capabilities(context.Context, storage.StorageMaintenanceSettings) storage.Capabilities {
	return storage.Capabilities{}
}
