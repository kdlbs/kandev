package updates

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/system/jobs"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newRouter(svc *Service) *gin.Engine {
	r := gin.New()
	api := r.Group("/api/v1/system")
	api.GET("/updates", HandleGet(svc))
	api.POST("/updates/check", HandleCheck(svc))
	api.PATCH("/updates/channel", HandleSetChannel(svc))
	api.POST("/updates/apply", HandleApply(svc))
	return r
}

func TestHandleGet_ReturnsZeroValues(t *testing.T) {
	pool := newTestPool(t)
	svc := NewService(pool, "v1.0.0", nil, logger.Default())
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/updates", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp UpdatesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Current != "v1.0.0" {
		t.Errorf("current=%q", resp.Current)
	}
	if resp.UpdateAvailable {
		t.Errorf("expected UpdateAvailable=false")
	}
}

func TestHandleGet_IncludesNonServiceInstallState(t *testing.T) {
	pool := newTestPool(t)
	svc := NewService(pool, "v1.0.0", nil, logger.Default())
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/updates", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	install, ok := body["install"].(map[string]interface{})
	if !ok {
		t.Fatalf("install state missing from response: %s", w.Body.String())
	}
	if got := install["running_as_service"]; got != false {
		t.Errorf("running_as_service=%v want false", got)
	}
	if got := body["apply_supported"]; got != false {
		t.Errorf("apply_supported=%v want false", got)
	}
	if got := body["channel"]; got != "stable" {
		t.Errorf("channel=%v want stable", got)
	}
	if got := body["channel_editable"]; got != false {
		t.Errorf("channel_editable=%v want false", got)
	}
	if got, ok := body["channel_unsupported_reason"].(string); !ok || got == "" {
		t.Errorf("channel_unsupported_reason=%v want non-empty string", body["channel_unsupported_reason"])
	}
}

type requestContextSettingsStore struct{}

func (requestContextSettingsStore) Get(ctx context.Context, _ string) ([]byte, bool, error) {
	return nil, false, ctx.Err()
}

func (requestContextSettingsStore) Save(context.Context, string, []byte) error {
	return nil
}

func TestHandleGetPropagatesRequestContext(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	pool := newTestPool(t)
	svc := NewService(
		pool,
		"v1.0.0",
		nil,
		logger.Default(),
		WithHomeDir(homeDir),
		WithSettingsStore(requestContextSettingsStore{}),
	)
	r := newRouter(svc)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/updates", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s want 500", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), context.Canceled.Error()) {
		t.Fatalf("response exposed settings read details: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "failed to load updates") {
		t.Fatalf("response=%s want generic error", w.Body.String())
	}
}

func TestHandleCheck_FirstCall200(t *testing.T) {
	pool := newTestPool(t)
	srv, _ := newStubGitHub(t, "v1.0.1", "https://example/v1.0.1")
	svc := NewService(pool, "v1.0.0", srv.Client(), logger.Default())
	svc.SetReleaseURL(srv.URL)
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/updates/check", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp UpdatesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Latest != "v1.0.1" {
		t.Errorf("latest=%q", resp.Latest)
	}
	if !resp.UpdateAvailable {
		t.Errorf("expected UpdateAvailable=true")
	}
}

func TestHandleCheck_SecondCallReturns429(t *testing.T) {
	pool := newTestPool(t)
	srv, _ := newStubGitHub(t, "v1.0.1", "https://example/v1.0.1")
	svc := NewService(pool, "v1.0.0", srv.Client(), logger.Default())
	svc.SetReleaseURL(srv.URL)
	r := newRouter(svc)

	// First call seeds the limiter.
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/system/updates/check", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first call status=%d", w1.Code)
	}

	// Second call within window is rate-limited.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/system/updates/check", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d body=%s", w2.Code, w2.Body.String())
	}
	var body struct {
		Error             string `json:"error"`
		RetryAfterSeconds int64  `json:"retry_after_seconds"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.RetryAfterSeconds < 1 || body.RetryAfterSeconds > 30 {
		t.Errorf("retry_after_seconds out of range: %d", body.RetryAfterSeconds)
	}
	if body.Error == "" {
		t.Errorf("expected non-empty error")
	}
}

func TestHandleCheck_GitHubFailureReturns502(t *testing.T) {
	pool := newTestPool(t)
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer failSrv.Close()
	svc := NewService(pool, "v1.0.0", failSrv.Client(), logger.Default())
	svc.SetReleaseURL(failSrv.URL)
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/updates/check", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func newManagedNPMServiceForHandler(t *testing.T) (*Service, *memorySettingsStore) {
	t.Helper()
	homeDir := configureManagedNPMInstall(t)
	store := &memorySettingsStore{}
	svc := NewService(
		newTestPool(t),
		"v1.2.3",
		nil,
		logger.Default(),
		WithHomeDir(homeDir),
		WithSettingsStore(store),
	)
	return svc, store
}

func readMemorySetting(t *testing.T, store *memorySettingsStore) ([]byte, bool) {
	t.Helper()
	value, present, err := store.Get(context.Background(), updatesChannelSettingKey)
	if err != nil {
		t.Fatal(err)
	}
	return value, present
}

func TestHandleSetChannelPersistsSupportedNightlyAndReturnsResolvedTarget(t *testing.T) {
	svc, store := newManagedNPMServiceForHandler(t)
	svc.SetNightlyFetcher(func(context.Context) (string, string, error) {
		return "1.2.4-nightly.shaabc123def456", "https://example/nightly", nil
	})
	r := newRouter(svc)
	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/system/updates/channel",
		bytes.NewBufferString(`{"channel":"nightly"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp UpdatesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Channel != ChannelNightly || !resp.ChannelEditable {
		t.Fatalf("channel response=%+v", resp)
	}
	if resp.Latest != "1.2.4-nightly.shaabc123def456" || !resp.UpdateAvailable {
		t.Fatalf("resolved response=%+v", resp)
	}
	selected, err := svc.selectedChannel(context.Background())
	raw, present := readMemorySetting(t, store)
	if err != nil || selected != ChannelNightly || !present || string(raw) != string(ChannelNightly) {
		t.Fatalf("persisted channel=%q raw=%q present=%v err=%v", selected, raw, present, err)
	}
}

func TestHandleSetChannelRejectsTrailingJSON(t *testing.T) {
	svc, store := newManagedNPMServiceForHandler(t)
	svc.SetNightlyFetcher(func(context.Context) (string, string, error) {
		return "1.2.4-nightly.shaabc123def456", "https://example/nightly", nil
	})
	r := newRouter(svc)
	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/system/updates/channel",
		bytes.NewBufferString(`{"channel":"nightly"} {}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s want 400", w.Code, w.Body.String())
	}
	_, present := readMemorySetting(t, store)
	if present {
		t.Fatal("channel selection with trailing JSON was persisted")
	}
}

func TestHandleSetChannelRejectsInvalidAndUnsupportedNightly(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		svc, _ := newManagedNPMServiceForHandler(t)
		r := newRouter(svc)
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/system/updates/channel", bytes.NewBufferString(`{"channel":"preview"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("unsupported install", func(t *testing.T) {
		store := &memorySettingsStore{}
		svc := NewService(newTestPool(t), "v1.2.3", nil, logger.Default(), WithSettingsStore(store))
		r := newRouter(svc)
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/system/updates/channel", bytes.NewBufferString(`{"channel":"nightly"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		_, present := readMemorySetting(t, store)
		if present {
			t.Fatal("unsupported channel selection was persisted")
		}
	})
}

func TestHandleSetChannelResolverFailureReturns502WithoutPersisting(t *testing.T) {
	svc, store := newManagedNPMServiceForHandler(t)
	svc.SetNightlyFetcher(func(context.Context) (string, string, error) {
		return "", "", errors.New("registry down")
	})
	r := newRouter(svc)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/system/updates/channel", bytes.NewBufferString(`{"channel":"nightly"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	_, present := readMemorySetting(t, store)
	if present {
		t.Fatal("failed resolver selection was persisted")
	}
}

func TestHandleSetChannelPersistenceFailureDoesNotExposeStorageDetails(t *testing.T) {
	svc, store := newManagedNPMServiceForHandler(t)
	store.saveErr = errors.New("sqlite: secret storage detail")
	svc.SetNightlyFetcher(func(context.Context) (string, string, error) {
		return "1.2.4-nightly.shaabc123def456", "https://example/nightly", nil
	})
	r := newRouter(svc)
	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/system/updates/channel",
		bytes.NewBufferString(`{"channel":"nightly"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s want 500", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret storage detail") {
		t.Fatalf("response exposed storage details: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "failed to set update channel") {
		t.Fatalf("response=%s want generic error", w.Body.String())
	}
}

func TestHandleSetChannelRejectsCrossOrigin(t *testing.T) {
	svc, store := newManagedNPMServiceForHandler(t)
	svc.SetNightlyFetcher(func(context.Context) (string, string, error) {
		return "1.2.4-nightly.shaabc123def456", "https://example/nightly", nil
	})
	r := newRouter(svc)
	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/system/updates/channel",
		bytes.NewBufferString(`{"channel":"nightly"}`),
	)
	req.Host = "kandev.local"
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s want 403", w.Code, w.Body.String())
	}
	_, present := readMemorySetting(t, store)
	if present {
		t.Fatal("cross-origin channel selection was persisted")
	}
}

func TestHandleSetChannelRejectsCrossPortOrigin(t *testing.T) {
	svc, store := newManagedNPMServiceForHandler(t)
	svc.SetNightlyFetcher(func(context.Context) (string, string, error) {
		return "1.2.4-nightly.shaabc123def456", "https://example/nightly", nil
	})
	r := newRouter(svc)
	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/system/updates/channel",
		bytes.NewBufferString(`{"channel":"nightly"}`),
	)
	req.Host = "localhost:38429"
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s want 403", w.Code, w.Body.String())
	}
	_, present := readMemorySetting(t, store)
	if present {
		t.Fatal("cross-port channel selection was persisted")
	}
}

func TestHandleApplyRejectsChangedNightlyTarget(t *testing.T) {
	svc, store := newManagedNPMServiceForHandler(t)
	if err := store.Save(context.Background(), updatesChannelSettingKey, []byte(ChannelNightly)); err != nil {
		t.Fatal(err)
	}
	if err := persistence.WriteLatestNightlyVersion(
		svc.pool.Writer(),
		"v1.2.5-nightly.shafedcba654321",
		"https://example/new-nightly",
		time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	svc.jobs = jobs.NewTracker(nil, logger.Default())
	r := newRouter(svc)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/system/updates/apply",
		bytes.NewBufferString(`{"confirm":"UPDATE","target_version":"v1.2.4-nightly.shaabc123def456"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s want 409", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "update target changed") {
		t.Fatalf("body=%s want target-changed error", w.Body.String())
	}
}

func TestHandleApplyRejectsInvalidJSONBeforeApplying(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{
			name: "type mismatch after valid target",
			body: `{"confirm":"UPDATE","target_version":"v1.2.4","target_version":123}`,
		},
		{
			name: "truncated object after valid fields",
			body: `{"confirm":"UPDATE","target_version":"v1.2.4","broken":`,
		},
		{
			name: "trailing JSON value",
			body: `{"confirm":"UPDATE","target_version":"v1.2.4"} {}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newManagedNPMServiceForHandler(t)
			if err := persistence.WriteLatestVersion(
				svc.pool.Writer(),
				"v1.2.4",
				"https://example/v1.2.4",
				time.Now().UTC(),
			); err != nil {
				t.Fatal(err)
			}
			svc.jobs = jobs.NewTracker(nil, logger.Default())
			var runnerCalls atomic.Int32
			svc.applyRun = func(context.Context, applyRequest) (map[string]interface{}, error) {
				runnerCalls.Add(1)
				return map[string]interface{}{"status": "started"}, nil
			}

			req := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/system/updates/apply",
				bytes.NewBufferString(tc.body),
			)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			newRouter(svc).ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status=%d body=%s want 400", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "invalid update request") {
				t.Errorf("body=%s want generic invalid-request error", w.Body.String())
			}
			if got := runnerCalls.Load(); got != 0 {
				t.Errorf("apply runner calls=%d want 0", got)
			}
		})
	}
}

func TestHandleApply_RejectsCrossOrigin(t *testing.T) {
	pool := newTestPool(t)
	svc := NewService(pool, "v1.0.0", nil, logger.Default())
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/updates/apply", bytes.NewBufferString(`{"confirm":"UPDATE"}`))
	req.Host = "kandev.local"
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleApply_WrongConfirmReturns400(t *testing.T) {
	pool := newTestPool(t)
	svc := NewService(pool, "v1.0.0", nil, logger.Default())
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/updates/apply", bytes.NewBufferString(`{"confirm":"NOPE"}`))
	req.Host = "localhost:38429"
	req.Header.Set("Origin", "http://localhost:38429")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleApply_RejectsCrossScheme(t *testing.T) {
	pool := newTestPool(t)
	svc := NewService(pool, "v1.0.0", nil, logger.Default())
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/updates/apply", bytes.NewBufferString(`{"confirm":"UPDATE"}`))
	req.Host = "localhost:38429"
	// Server was reached over plain http (no TLS); an https Origin is cross-scheme.
	req.Header.Set("Origin", "https://localhost:38429")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s want 403", w.Code, w.Body.String())
	}
}

func TestHandleApply_HonorsForwardedProtoForScheme(t *testing.T) {
	pool := newTestPool(t)
	svc := NewService(pool, "v1.0.0", nil, logger.Default())
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/updates/apply", bytes.NewBufferString(`{"confirm":"UPDATE"}`))
	req.Host = "localhost:38429"
	req.Header.Set("Origin", "https://localhost:38429")
	// A reverse proxy terminated TLS upstream, so the https Origin is same-origin.
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Passes the same-origin gate, so it proceeds to the install-state check and
	// is refused there (409) rather than blocked as cross-origin (403).
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s want 409 (not 403)", w.Code, w.Body.String())
	}
}

func TestHandleApply_RejectsLoopbackDifferentPort(t *testing.T) {
	pool := newTestPool(t)
	svc := NewService(pool, "v1.0.0", nil, logger.Default())
	r := newRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/updates/apply", bytes.NewBufferString(`{"confirm":"UPDATE"}`))
	req.Host = "localhost:38429"
	req.Header.Set("Origin", "http://localhost:37429")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
