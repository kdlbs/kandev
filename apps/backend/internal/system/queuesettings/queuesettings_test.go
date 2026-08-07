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
		{name: "default", want: responseFor(10, 10, SourceDefault, false, true)},
		{name: "setting", configured: &Settings{MaxPerSession: 6, MergeEnabled: true}, want: responseFor(6, 6, SourceSetting, false, true)},
		{name: "environment", configured: &Settings{MaxPerSession: 6, MergeEnabled: true}, environment: Environment{Value: "20", Present: true}, want: responseFor(6, 20, SourceEnvironment, true, true)},
		{name: "zero environment is unlimited", configured: &Settings{MaxPerSession: 6, MergeEnabled: true}, environment: Environment{Value: "0", Present: true}, want: responseFor(6, 0, SourceEnvironment, true, true)},
		{name: "negative environment is unlimited", configured: &Settings{MaxPerSession: 6, MergeEnabled: true}, environment: Environment{Value: "-3", Present: true}, want: responseFor(6, 0, SourceEnvironment, true, true)},
		{name: "invalid environment is ignored", configured: &Settings{MaxPerSession: 6, MergeEnabled: true}, environment: Environment{Value: "many", Present: true}, want: responseFor(6, 6, SourceSetting, false, true), invalidEnv: true},
		{name: "merge disabled setting", configured: &Settings{MaxPerSession: 6, MergeEnabled: false}, want: responseFor(6, 6, SourceSetting, false, false)},
		{
			name:        "merge disabled setting survives an environment override of max_per_session",
			configured:  &Settings{MaxPerSession: 6, MergeEnabled: false},
			environment: Environment{Value: "20", Present: true},
			want:        responseFor(6, 20, SourceEnvironment, true, false),
		},
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
	if err := store.Save(context.Background(), Settings{MaxPerSession: 8, MergeEnabled: true}); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err = store.Load(context.Background())
	if err != nil || loaded == nil || loaded.MaxPerSession != 8 || !loaded.MergeEnabled {
		t.Fatalf("round trip = %+v, %v", loaded, err)
	}
	if err := store.Save(context.Background(), Settings{MaxPerSession: -1}); !errors.Is(err, ErrValidation) {
		t.Fatalf("negative save error = %v, want validation", err)
	}
}

// TestStoreLoadDefaultsMergeEnabledForPreExistingRecords guards a real
// upgrade hazard: an installation that persisted max_per_session before
// merge_enabled existed has a stored JSON object with no "merge_enabled"
// key. json.Unmarshal into a plain bool decodes that as false, which would
// silently disable merging on upgrade instead of leaving it enabled by
// default. Store.Load must treat the missing key as "unset" (-> true), not
// "explicitly false".
func TestStoreLoadDefaultsMergeEnabledForPreExistingRecords(t *testing.T) {
	raw := &fakeRawStore{raw: []byte(`{"max_per_session":6}`), found: true}
	store := NewStore(raw)

	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load pre-existing record: %v", err)
	}
	if loaded == nil || loaded.MaxPerSession != 6 || !loaded.MergeEnabled {
		t.Fatalf("loaded = %+v, want {MaxPerSession:6 MergeEnabled:true}", loaded)
	}
}

// TestStoreLoadPreservesExplicitMergeDisabled is the companion case: once a
// record has been re-saved through this code path with merge_enabled
// explicitly false, Load must not treat that as "unset" and re-default it to
// true.
func TestStoreLoadPreservesExplicitMergeDisabled(t *testing.T) {
	raw := &fakeRawStore{raw: []byte(`{"max_per_session":6,"merge_enabled":false}`), found: true}
	store := NewStore(raw)

	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load explicit-false record: %v", err)
	}
	if loaded == nil || loaded.MergeEnabled {
		t.Fatalf("loaded = %+v, want MergeEnabled:false preserved", loaded)
	}
}

func TestServiceUpdatePersistsBeforeLiveApply(t *testing.T) {
	raw := &fakeRawStore{}
	target := &fakeTarget{max: 10}
	service := NewService(NewStore(raw), target, func() Environment { return Environment{} }, testLogger(t))

	response, err := service.Update(context.Background(), SettingsPatch{MaxPerSession: new(4)})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if target.max != 4 || response.Effective.MaxPerSession != 4 || response.Effective.Source != SourceSetting {
		t.Fatalf("update result=%+v target=%d", response, target.max)
	}

	raw.saveErr = errors.New("disk full")
	_, err = service.Update(context.Background(), SettingsPatch{MaxPerSession: new(2)})
	if err == nil {
		t.Fatal("expected persistence error")
	}
	if target.max != 4 {
		t.Fatalf("target applied before failed save: %d", target.max)
	}
}

// TestServiceUpdatePatchPreservesUnspecifiedFields guards against a patch
// that touches only one field silently resetting the other to its zero
// value: a max_per_session-only patch must not disable merging, and a
// merge_enabled-only patch must not reset the capacity.
func TestServiceUpdatePatchPreservesUnspecifiedFields(t *testing.T) {
	raw := &fakeRawStore{}
	target := &fakeTarget{max: 10}
	service := NewService(NewStore(raw), target, func() Environment { return Environment{} }, testLogger(t))

	if _, err := service.Update(context.Background(), SettingsPatch{MergeEnabled: new(true)}); err != nil {
		t.Fatalf("seed merge_enabled: %v", err)
	}

	response, err := service.Update(context.Background(), SettingsPatch{MaxPerSession: new(4)})
	if err != nil {
		t.Fatalf("max-only update: %v", err)
	}
	if !response.Settings.MergeEnabled || !target.MergeEnabled() {
		t.Fatalf("max-only patch reset merge_enabled: response=%+v target=%v", response, target.MergeEnabled())
	}

	response, err = service.Update(context.Background(), SettingsPatch{MergeEnabled: new(false)})
	if err != nil {
		t.Fatalf("merge-only update: %v", err)
	}
	if response.Settings.MaxPerSession != 4 || target.MaxPerSession() != 4 {
		t.Fatalf("merge-only patch reset max_per_session: response=%+v target=%d", response, target.MaxPerSession())
	}
	if response.Settings.MergeEnabled || target.MergeEnabled() {
		t.Fatalf("merge-only patch did not apply: response=%+v target=%v", response, target.MergeEnabled())
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
			if response != responseFor(10, 10, SourceDefault, false, true) {
				t.Fatalf("fallback response = %+v", response)
			}

			response, err = service.Update(context.Background(), SettingsPatch{MaxPerSession: new(7)})
			if err != nil {
				t.Fatalf("replace invalid persisted setting: %v", err)
			}
			if response != responseFor(7, 7, SourceSetting, false, true) || target.max != 7 {
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
		_, err := service.Update(context.Background(), SettingsPatch{MaxPerSession: new(7)})
		firstDone <- err
	}()
	<-raw.firstSaveWritten

	secondDone := make(chan error, 1)
	go func() {
		_, err := service.Update(context.Background(), SettingsPatch{MaxPerSession: new(9)})
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

	_, err := service.Update(context.Background(), SettingsPatch{MaxPerSession: new(4)})
	if !errors.Is(err, ErrEnvironmentLocked) {
		t.Fatalf("update error = %v, want environment lock", err)
	}
	if raw.saveCalls != 0 || target.max != 20 {
		t.Fatalf("locked update mutated state: saves=%d target=%d", raw.saveCalls, target.max)
	}
}

// TestServiceEnvironmentLockAllowsMergeOnlyUpdate asserts the environment
// lock only blocks a patch that touches max_per_session — merge_enabled has
// no environment override and must stay editable.
func TestServiceEnvironmentLockAllowsMergeOnlyUpdate(t *testing.T) {
	raw := &fakeRawStore{}
	target := &fakeTarget{max: 20}
	service := NewService(NewStore(raw), target, func() Environment {
		return Environment{Value: "20", Present: true}
	}, testLogger(t))

	response, err := service.Update(context.Background(), SettingsPatch{MergeEnabled: new(false)})
	if err != nil {
		t.Fatalf("merge-only update under max_per_session lock: %v", err)
	}
	if response.Settings.MergeEnabled || target.MergeEnabled() {
		t.Fatalf("merge-only update did not apply: response=%+v target=%v", response, target.MergeEnabled())
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

	body, _ := json.Marshal(SettingsPatch{MaxPerSession: new(4)})
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
	if err := store.Save(context.Background(), Settings{MaxPerSession: 6, MergeEnabled: true}); err != nil {
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
	want := responseFor(6, 20, SourceEnvironment, true, true)
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

// TestHandlerUpdateOmittingMergeEnabledPreservesIt is the HTTP-level
// regression test for the patch-not-replace contract: a PATCH body that
// only names max_per_session must never flip a previously-saved
// merge_enabled=true back to false.
func TestHandlerUpdateOmittingMergeEnabledPreservesIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	raw := &fakeRawStore{}
	target := &fakeTarget{max: 10, mergeEnabled: true}
	service := NewService(NewStore(raw), target, func() Environment { return Environment{} }, testLogger(t))
	router := gin.New()
	group := router.Group("/api/v1/system")
	RegisterRoutes(group, group, service)

	request := httptest.NewRequest(
		http.MethodPatch, "/api/v1/system/message-queue/settings",
		bytes.NewBufferString(`{"max_per_session":5}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var payload Response
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Settings.MergeEnabled || !target.MergeEnabled() {
		t.Fatalf("max_per_session-only PATCH disabled merging: payload=%+v target=%v", payload, target.MergeEnabled())
	}
	if payload.Settings.MaxPerSession != 5 || target.MaxPerSession() != 5 {
		t.Fatalf("max_per_session was not applied: payload=%+v target=%d", payload, target.MaxPerSession())
	}
}

func responseFor(configured, effective int, source Source, locked bool, mergeEnabled bool) Response {
	return Response{
		Settings:  Settings{MaxPerSession: configured, MergeEnabled: mergeEnabled},
		Effective: Effective{MaxPerSession: effective, Source: source, Locked: locked, MergeEnabled: mergeEnabled},
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
	f.raw = value
	f.found = true
	return nil
}

type fakeTarget struct {
	max          int
	mergeEnabled bool
}

func (f *fakeTarget) MaxPerSession() int     { return f.max }
func (f *fakeTarget) SetMaxPerSession(n int) { f.max = n }
func (f *fakeTarget) MergeEnabled() bool     { return f.mergeEnabled }
func (f *fakeTarget) SetMergeEnabled(v bool) { f.mergeEnabled = v }

type blockingRawStore struct {
	mu                sync.Mutex
	raw               []byte
	found             bool
	saveCount         int
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
	return s.raw, s.found, nil
}

func (s *blockingRawStore) Save(_ context.Context, _ string, value []byte) error {
	s.mu.Lock()
	s.saveCount++
	count := s.saveCount
	s.mu.Unlock()

	if count == 1 {
		s.mu.Lock()
		s.raw = value
		s.found = true
		s.mu.Unlock()
		close(s.firstSaveWritten)
		<-s.releaseFirst
		return nil
	}

	close(s.secondSaveEntered)
	s.mu.Lock()
	s.raw = value
	s.found = true
	s.mu.Unlock()
	return nil
}

type atomicTarget struct {
	max          atomic.Int64
	mergeEnabled atomic.Bool
}

func (t *atomicTarget) MaxPerSession() int     { return int(t.max.Load()) }
func (t *atomicTarget) SetMaxPerSession(n int) { t.max.Store(int64(n)) }
func (t *atomicTarget) MergeEnabled() bool     { return t.mergeEnabled.Load() }
func (t *atomicTarget) SetMergeEnabled(v bool) { t.mergeEnabled.Store(v) }

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log
}
