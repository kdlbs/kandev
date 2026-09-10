package retention

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
)

// newTestRetentionRouter mirrors production wiring: GET is member-readable,
// PUT requires admin, matching storage's and sleep-inhibition's split
// read/admin groups.
func newTestRetentionRouter(handler *Handler) *gin.Engine {
	return newTestRetentionRouterAs(handler, authn.RoleAdmin)
}

func newTestRetentionRouterAs(handler *Handler, role authn.Role) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "user-1", Role: role})
		c.Next()
	})
	read := router.Group("/api/v1/system")
	admin := read.Group("", authn.RequireAdmin())
	RegisterRoutes(read, admin, handler)
	return router
}

func newTestHandler(t *testing.T) (*Handler, *Sweeper) {
	t.Helper()
	sweeper, _ := newTestSweeper(t)
	handler := NewHandler(HandlerConfig{SettingsStore: sweeper.settingsStore, Sweeper: sweeper})
	return handler, sweeper
}

func doRequest(router *gin.Engine, method, path string, body []byte) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestGetRetention_FreshInstallReturnsDefaultsAndNilLastSweep(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newTestHandler(t)
	router := newTestRetentionRouter(handler)

	response := doRequest(router, http.MethodGet, "/api/v1/system/retention", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}

	var status Status
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if status.LastSweep != nil {
		t.Fatalf("LastSweep = %+v, want nil before the first sweep (AC-004.7)", status.LastSweep)
	}
	if status.Settings != DefaultSettings() {
		t.Fatalf("Settings = %+v, want defaults", status.Settings)
	}
	if status.SkipCount != 0 {
		t.Fatalf("SkipCount = %d, want 0", status.SkipCount)
	}
}

func TestGetRetention_NonAdminMemberCanRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newTestHandler(t)
	router := newTestRetentionRouterAs(handler, authn.RoleMember)

	response := doRequest(router, http.MethodGet, "/api/v1/system/retention", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("member GET status = %d, want 200: %s", response.Code, response.Body.String())
	}
}

func TestPutRetention_NonAdminMemberIsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newTestHandler(t)
	router := newTestRetentionRouterAs(handler, authn.RoleMember)

	response := doRequest(router, http.MethodPut, "/api/v1/system/retention", []byte(`{}`))
	if response.Code != http.StatusForbidden {
		t.Fatalf("member PUT status = %d, want 403: %s", response.Code, response.Body.String())
	}
}

func TestGetRetention_ReflectsLastSweepAndCensus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, sweeper := newTestHandler(t)
	router := newTestRetentionRouter(handler)
	ctx := t.Context()

	sweeper.RunCensus(ctx)
	sweeper.RunSweep(ctx) // preview pass; still sets LastSweep

	response := doRequest(router, http.MethodGet, "/api/v1/system/retention", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}

	var status Status
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if status.LastSweep == nil {
		t.Fatal("LastSweep = nil, want non-nil after a sweep has run")
	}
	if status.RetainedCounts.OfficeRoutineRuns.State != CensusFresh {
		t.Fatalf("RetainedCounts.OfficeRoutineRuns.State = %v, want fresh", status.RetainedCounts.OfficeRoutineRuns.State)
	}
}

func TestPutRetention_OmittedFieldTakesDocumentedDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newTestHandler(t)
	router := newTestRetentionRouter(handler)

	body := []byte(`{"enabled": false}`)
	response := doRequest(router, http.MethodPut, "/api/v1/system/retention", body)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}

	var saved Settings
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if saved.Enabled {
		t.Fatal("Enabled = true, want false (explicitly set)")
	}
	want := DefaultSettings()
	if saved.SweepIntervalHours != want.SweepIntervalHours {
		t.Fatalf("SweepIntervalHours = %d, want the default %d (omitted field)", saved.SweepIntervalHours, want.SweepIntervalHours)
	}
	if saved.RoutineRuns != want.RoutineRuns {
		t.Fatalf("RoutineRuns = %+v, want the default %+v (omitted field)", saved.RoutineRuns, want.RoutineRuns)
	}
}

func TestPutRetention_RepeatedIdenticalWriteIsANoOp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, sweeper := newTestHandler(t)
	router := newTestRetentionRouter(handler)

	body, err := json.Marshal(DefaultSettings())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	first := doRequest(router, http.MethodPut, "/api/v1/system/retention", body)
	second := doRequest(router, http.MethodPut, "/api/v1/system/retention", body)
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("status = %d, %d, want 200, 200", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("identical writes returned different documents:\n%s\n%s", first.Body.String(), second.Body.String())
	}

	stored, err := sweeper.settingsStore.GetSettings(t.Context())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if stored != DefaultSettings() {
		t.Fatalf("stored = %+v, want unchanged defaults", stored)
	}
}

func TestPutRetention_ExplicitNullIsRejectedNamingTheField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, sweeper := newTestHandler(t)
	router := newTestRetentionRouter(handler)

	body := []byte(`{"runs": {"window_days": null}}`)
	response := doRequest(router, http.MethodPut, "/api/v1/system/retention", body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "runs.window_days") {
		t.Fatalf("body = %s, want it to name runs.window_days", response.Body.String())
	}

	// Nothing written: a decade-long window must survive a null-rejected PUT.
	settings := DefaultSettings()
	settings.Runs.WindowDays = 3650
	if _, err := sweeper.settingsStore.SaveSettings(t.Context(), settings); err != nil {
		t.Fatalf("seed SaveSettings: %v", err)
	}
	response = doRequest(router, http.MethodPut, "/api/v1/system/retention", body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	stored, err := sweeper.settingsStore.GetSettings(t.Context())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if stored.Runs.WindowDays != 3650 {
		t.Fatalf("Runs.WindowDays = %d, want 3650 unchanged (a rejected write must not destroy configured history)", stored.Runs.WindowDays)
	}
}

func TestPutRetention_UnknownFieldIsRejectedNamingTheField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newTestHandler(t)
	router := newTestRetentionRouter(handler)

	body := []byte(`{"windw_days": 30}`)
	response := doRequest(router, http.MethodPut, "/api/v1/system/retention", body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "windw_days") {
		t.Fatalf("body = %s, want it to name the misspelled field", response.Body.String())
	}
}

func TestPutRetention_FractionalNumberIsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newTestHandler(t)
	router := newTestRetentionRouter(handler)

	body := []byte(`{"batch_limit": 100.5}`)
	response := doRequest(router, http.MethodPut, "/api/v1/system/retention", body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "batch_limit") {
		t.Fatalf("body = %s, want it to name batch_limit", response.Body.String())
	}
}

func TestPutRetention_WrongTypeIsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := newTestHandler(t)
	router := newTestRetentionRouter(handler)

	body := []byte(`{"enabled": "yes"}`)
	response := doRequest(router, http.MethodPut, "/api/v1/system/retention", body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "enabled") {
		t.Fatalf("body = %s, want it to name enabled", response.Body.String())
	}
}

func TestPutRetention_OutOfRangeIsRejectedAndLeavesStoredSettingsUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, sweeper := newTestHandler(t)
	router := newTestRetentionRouter(handler)

	body := []byte(`{"batch_limit": 1}`) // below minBatchLimit=100
	response := doRequest(router, http.MethodPut, "/api/v1/system/retention", body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "batch_limit") {
		t.Fatalf("body = %s, want it to name batch_limit", response.Body.String())
	}

	stored, err := sweeper.settingsStore.GetSettings(t.Context())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if stored != DefaultSettings() {
		t.Fatalf("stored = %+v, want unchanged defaults", stored)
	}
}

func TestPutRetention_SuccessInvokesOnSettingsChanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sweeper, _ := newTestSweeper(t)

	var got Settings
	var called bool
	handler := NewHandler(HandlerConfig{
		SettingsStore: sweeper.settingsStore,
		Sweeper:       sweeper,
		OnSettingsChanged: func(s Settings) {
			called = true
			got = s
		},
	})
	router := newTestRetentionRouter(handler)

	body := []byte(`{"enabled": false}`)
	response := doRequest(router, http.MethodPut, "/api/v1/system/retention", body)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if !called {
		t.Fatal("OnSettingsChanged was not called")
	}
	if got.Enabled {
		t.Fatal("OnSettingsChanged received Enabled=true, want false")
	}
}

// TestPutRetention_ConcurrentPUTsApplyInSaveOrder is the regression test for
// the concurrent-PUT scheduler desync found in review: without serializing
// SaveSettings and OnSettingsChanged as one critical section, a second PUT
// racing between the first's save and apply could complete its own save and
// apply entirely in between, leaving the scheduler applying the first PUT's
// now-stale settings after the second PUT's newer write already committed
// (AC-004.5's last-writer-wins). testBetweenSaveAndApply fires while the
// first PUT still holds the handler's mutex; it starts a second PUT
// concurrently and proves that second PUT cannot complete until the first
// releases the mutex, so the two applications can never interleave.
func TestPutRetention_ConcurrentPUTsApplyInSaveOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sweeper, _ := newTestSweeper(t)

	var mu sync.Mutex
	var applied []bool
	handler := NewHandler(HandlerConfig{
		SettingsStore: sweeper.settingsStore,
		Sweeper:       sweeper,
		OnSettingsChanged: func(s Settings) {
			mu.Lock()
			applied = append(applied, s.Enabled)
			mu.Unlock()
		},
	})
	router := newTestRetentionRouter(handler)

	secondDone := make(chan struct{})
	secondStarted := false

	t.Cleanup(func() { testBetweenSaveAndApply = nil })
	testBetweenSaveAndApply = func() {
		testBetweenSaveAndApply = nil // only race a second request once
		secondStarted = true
		go func() {
			response := doRequest(router, http.MethodPut, "/api/v1/system/retention", []byte(`{"enabled": true}`))
			if response.Code != http.StatusOK {
				t.Errorf("second PUT status = %d, want 200: %s", response.Code, response.Body.String())
			}
			close(secondDone)
		}()

		select {
		case <-secondDone:
			t.Fatal("second PUT completed while the first still held the critical section")
		case <-time.After(100 * time.Millisecond):
		}
	}

	first := doRequest(router, http.MethodPut, "/api/v1/system/retention", []byte(`{"enabled": false}`))
	if first.Code != http.StatusOK {
		t.Fatalf("first PUT status = %d, want 200: %s", first.Code, first.Body.String())
	}
	if !secondStarted {
		t.Fatal("test hook never fired; the race was not exercised")
	}

	select {
	case <-secondDone:
	case <-time.After(5 * time.Second):
		t.Fatal("second PUT never completed after the first released the critical section")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(applied) != 2 || applied[0] != false || applied[1] != true {
		t.Fatalf("OnSettingsChanged calls = %+v, want [false, true] in save order", applied)
	}

	stored, err := sweeper.settingsStore.GetSettings(t.Context())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if !stored.Enabled {
		t.Fatal("stored Enabled = false, want true (the second, later PUT must win)")
	}
}
