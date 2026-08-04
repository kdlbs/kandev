package queuesettings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestResolvePrecedenceAndNormalization(t *testing.T) {
	tests := []struct {
		name        string
		configured  *Settings
		environment Environment
		want        Response
		invalidEnv  bool
	}{
		{name: "default", want: responseFor(10, 10, SourceDefault, false)},
		{name: "setting", configured: &Settings{MaxPerSession: 6}, want: responseFor(6, 6, SourceSetting, false)},
		{name: "environment", configured: &Settings{MaxPerSession: 6}, environment: Environment{Value: "20", Present: true}, want: responseFor(6, 20, SourceEnvironment, true)},
		{name: "zero environment is unlimited", configured: &Settings{MaxPerSession: 6}, environment: Environment{Value: "0", Present: true}, want: responseFor(6, 0, SourceEnvironment, true)},
		{name: "negative environment is unlimited", configured: &Settings{MaxPerSession: 6}, environment: Environment{Value: "-3", Present: true}, want: responseFor(6, 0, SourceEnvironment, true)},
		{name: "invalid environment is ignored", configured: &Settings{MaxPerSession: 6}, environment: Environment{Value: "many", Present: true}, want: responseFor(6, 6, SourceSetting, false), invalidEnv: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.configured, tc.environment)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got.Response != tc.want || got.InvalidEnvironment != tc.invalidEnv {
				t.Fatalf("resolution = %+v, want response=%+v invalid=%v", got, tc.want, tc.invalidEnv)
			}
		})
	}
}

func TestResolveRejectsNegativePersistedValue(t *testing.T) {
	_, err := Resolve(&Settings{MaxPerSession: -1}, Environment{})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestStoreRoundTripAndValidation(t *testing.T) {
	raw := &fakeRawStore{}
	store := NewStore(raw)
	loaded, err := store.Load(context.Background())
	if err != nil || loaded != nil {
		t.Fatalf("missing load = %+v, %v", loaded, err)
	}
	if err := store.Save(context.Background(), Settings{MaxPerSession: 8}); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err = store.Load(context.Background())
	if err != nil || loaded == nil || loaded.MaxPerSession != 8 {
		t.Fatalf("round trip = %+v, %v", loaded, err)
	}
	if err := store.Save(context.Background(), Settings{MaxPerSession: -1}); !errors.Is(err, ErrValidation) {
		t.Fatalf("negative save error = %v, want validation", err)
	}
}

func TestServiceUpdatePersistsBeforeLiveApply(t *testing.T) {
	raw := &fakeRawStore{}
	target := &fakeTarget{max: 10}
	service := NewService(NewStore(raw), target, func() Environment { return Environment{} }, testLogger(t))

	response, err := service.Update(context.Background(), Settings{MaxPerSession: 4})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if target.max != 4 || response.Effective.MaxPerSession != 4 || response.Effective.Source != SourceSetting {
		t.Fatalf("update result=%+v target=%d", response, target.max)
	}

	raw.saveErr = errors.New("disk full")
	_, err = service.Update(context.Background(), Settings{MaxPerSession: 2})
	if err == nil {
		t.Fatal("expected persistence error")
	}
	if target.max != 4 {
		t.Fatalf("target applied before failed save: %d", target.max)
	}
}

func TestServiceRecoversFromInvalidPersistedSettingAndAllowsReplacement(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "malformed JSON", raw: `{`},
		{name: "negative value", raw: `{"max_per_session":-2}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := &fakeRawStore{raw: []byte(tc.raw), found: true}
			target := &fakeTarget{max: 10}
			service := NewService(
				NewStore(raw), target, func() Environment { return Environment{} }, testLogger(t),
			)

			response, err := service.Get(context.Background())
			if err != nil {
				t.Fatalf("get with invalid persisted setting: %v", err)
			}
			if response != responseFor(10, 10, SourceDefault, false) {
				t.Fatalf("fallback response = %+v", response)
			}

			response, err = service.Update(context.Background(), Settings{MaxPerSession: 7})
			if err != nil {
				t.Fatalf("replace invalid persisted setting: %v", err)
			}
			if response != responseFor(7, 7, SourceSetting, false) || target.max != 7 {
				t.Fatalf("replacement response=%+v target=%d", response, target.max)
			}
		})
	}
}

func TestServiceSerializesPersistenceAndLiveApply(t *testing.T) {
	raw := newBlockingRawStore()
	target := &atomicTarget{}
	target.max.Store(10)
	service := NewService(
		NewStore(raw), target, func() Environment { return Environment{} }, testLogger(t),
	)

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.Update(context.Background(), Settings{MaxPerSession: 7})
		firstDone <- err
	}()
	<-raw.firstSaveWritten

	secondDone := make(chan error, 1)
	go func() {
		_, err := service.Update(context.Background(), Settings{MaxPerSession: 9})
		secondDone <- err
	}()

	select {
	case <-raw.secondSaveEntered:
		close(raw.releaseFirst)
		<-firstDone
		<-secondDone
		t.Fatal("second update persisted before first update finished applying live")
	case <-time.After(250 * time.Millisecond):
		close(raw.releaseFirst)
	}

	if err := <-firstDone; err != nil {
		t.Fatalf("first update: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second update: %v", err)
	}
	configured, err := NewStore(raw).Load(context.Background())
	if err != nil || configured == nil {
		t.Fatalf("load final setting: %+v, %v", configured, err)
	}
	if configured.MaxPerSession != 9 || target.MaxPerSession() != 9 {
		t.Fatalf("final configured=%d live=%d, want both 9", configured.MaxPerSession, target.MaxPerSession())
	}
}

func TestServiceEnvironmentLockRejectsUpdate(t *testing.T) {
	raw := &fakeRawStore{}
	target := &fakeTarget{max: 20}
	service := NewService(NewStore(raw), target, func() Environment {
		return Environment{Value: "20", Present: true}
	}, testLogger(t))

	_, err := service.Update(context.Background(), Settings{MaxPerSession: 4})
	if !errors.Is(err, ErrEnvironmentLocked) {
		t.Fatalf("update error = %v, want environment lock", err)
	}
	if raw.saveCalls != 0 || target.max != 20 {
		t.Fatalf("locked update mutated state: saves=%d target=%d", raw.saveCalls, target.max)
	}
}

func TestHandlerReturnsConflictForEnvironmentLock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	raw := &fakeRawStore{}
	service := NewService(NewStore(raw), &fakeTarget{max: 20}, func() Environment {
		return Environment{Value: "20", Present: true}
	}, testLogger(t))
	router := gin.New()
	group := router.Group("/api/v1/system")
	RegisterRoutes(group, group, service)

	body, _ := json.Marshal(Settings{MaxPerSession: 4})
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/system/message-queue/settings", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", response.Code, response.Body.String())
	}
}

func TestHandlerGetReturnsConfiguredAndEffectiveValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	raw := &fakeRawStore{}
	store := NewStore(raw)
	if err := store.Save(context.Background(), Settings{MaxPerSession: 6}); err != nil {
		t.Fatalf("save baseline: %v", err)
	}
	service := NewService(store, &fakeTarget{max: 20}, func() Environment {
		return Environment{Value: "20", Present: true}
	}, testLogger(t))
	router := gin.New()
	group := router.Group("/api/v1/system")
	RegisterRoutes(group, group, service)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(
		http.MethodGet, "/api/v1/system/message-queue/settings", nil,
	))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var payload Response
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := responseFor(6, 20, SourceEnvironment, true)
	if payload != want {
		t.Fatalf("response = %+v, want %+v", payload, want)
	}
}

func TestHandlerRejectsNegativeCapacity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/system")
	RegisterRoutes(group, group, NewService(
		NewStore(&fakeRawStore{}), &fakeTarget{max: 10},
		func() Environment { return Environment{} }, testLogger(t),
	))

	request := httptest.NewRequest(
		http.MethodPatch, "/api/v1/system/message-queue/settings",
		bytes.NewBufferString(`{"max_per_session":-1}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
	}
}

func responseFor(configured, effective int, source Source, locked bool) Response {
	return Response{
		Settings:  Settings{MaxPerSession: configured},
		Effective: Effective{MaxPerSession: effective, Source: source, Locked: locked},
	}
}

type fakeRawStore struct {
	raw       []byte
	found     bool
	saveErr   error
	saveCalls int
}

func (f *fakeRawStore) Get(context.Context, string) ([]byte, bool, error) {
	return f.raw, f.found, nil
}

func (f *fakeRawStore) Save(_ context.Context, _ string, value []byte) error {
	f.saveCalls++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.raw = append([]byte(nil), value...)
	f.found = true
	return nil
}

type fakeTarget struct {
	max int
}

func (f *fakeTarget) MaxPerSession() int     { return f.max }
func (f *fakeTarget) SetMaxPerSession(n int) { f.max = n }

type blockingRawStore struct {
	mu                sync.Mutex
	raw               []byte
	found             bool
	saveCalls         int
	firstSaveWritten  chan struct{}
	secondSaveEntered chan struct{}
	releaseFirst      chan struct{}
}

func newBlockingRawStore() *blockingRawStore {
	return &blockingRawStore{
		firstSaveWritten:  make(chan struct{}),
		secondSaveEntered: make(chan struct{}),
		releaseFirst:      make(chan struct{}),
	}
}

func (s *blockingRawStore) Get(context.Context, string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.raw...), s.found, nil
}

func (s *blockingRawStore) Save(_ context.Context, _ string, value []byte) error {
	s.mu.Lock()
	s.saveCalls++
	call := s.saveCalls
	s.raw = append([]byte(nil), value...)
	s.found = true
	switch call {
	case 1:
		close(s.firstSaveWritten)
	case 2:
		close(s.secondSaveEntered)
	}
	s.mu.Unlock()
	if call == 1 {
		<-s.releaseFirst
	}
	return nil
}

type atomicTarget struct {
	max atomic.Int64
}

func (t *atomicTarget) MaxPerSession() int     { return int(t.max.Load()) }
func (t *atomicTarget) SetMaxPerSession(n int) { t.max.Store(int64(n)) }

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}
