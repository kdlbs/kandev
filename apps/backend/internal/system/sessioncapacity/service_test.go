package sessioncapacity

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestServicePersistsBeforeApplyingAndRetainsRememberedMaximum(t *testing.T) {
	raw := &memoryRawStore{}
	target := &fakeTarget{}
	service := NewService(NewStore(raw), target, Environment{}, testLogger(t))

	response, err := service.Update(context.Background(), SettingsPatch{Enabled: boolPointer(true)})
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if response.Settings != (Settings{Enabled: true, MaxSessions: DefaultMaxSessions}) {
		t.Fatalf("enabled settings = %+v", response.Settings)
	}
	if response.Effective != (Effective{Enabled: true, MaxSessions: DefaultMaxSessions, Source: SourceSetting}) {
		t.Fatalf("enabled effective = %+v", response.Effective)
	}
	if target.Capacity() != DefaultMaxSessions {
		t.Fatalf("live capacity = %d, want %d", target.Capacity(), DefaultMaxSessions)
	}

	response, err = service.Update(context.Background(), SettingsPatch{MaxSessions: intPointer(9)})
	if err != nil {
		t.Fatalf("change maximum: %v", err)
	}
	if response.Settings != (Settings{Enabled: true, MaxSessions: 9}) || target.Capacity() != 9 {
		t.Fatalf("changed maximum response=%+v live=%d", response, target.Capacity())
	}

	response, err = service.Update(context.Background(), SettingsPatch{Enabled: boolPointer(false)})
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if response.Settings != (Settings{Enabled: false, MaxSessions: 9}) {
		t.Fatalf("disabled settings = %+v, want remembered maximum", response.Settings)
	}
	if response.Effective != (Effective{Enabled: false, MaxSessions: 0, Source: SourceSetting}) {
		t.Fatalf("disabled effective = %+v", response.Effective)
	}
	if target.Capacity() != 0 {
		t.Fatalf("disabled live capacity = %d, want 0", target.Capacity())
	}
}

func TestServiceSaveFailureDoesNotMutateTarget(t *testing.T) {
	raw := &memoryRawStore{saveErr: errors.New("storage unavailable")}
	target := &fakeTarget{capacity: 3}
	service := NewService(NewStore(raw), target, Environment{}, testLogger(t))

	_, err := service.Update(context.Background(), SettingsPatch{Enabled: boolPointer(true)})
	if err == nil {
		t.Fatal("update succeeded, want save error")
	}
	if target.Capacity() != 3 || target.CallCount() != 0 {
		t.Fatalf("target mutated after save failure: capacity=%d calls=%d", target.Capacity(), target.CallCount())
	}
	if raw.saveCalls != 1 {
		t.Fatalf("save calls = %d, want 1", raw.saveCalls)
	}
}

func TestServiceEnvironmentLockAndTargetAvailability(t *testing.T) {
	raw := &memoryRawStore{}
	target := &fakeTarget{capacity: 7}
	service := NewService(
		NewStore(raw), target, Environment{Value: "7", Present: true}, testLogger(t),
	)

	_, err := service.Update(context.Background(), SettingsPatch{MaxSessions: intPointer(4)})
	if !errors.Is(err, ErrEnvironmentLocked) {
		t.Fatalf("locked update error = %v, want ErrEnvironmentLocked", err)
	}
	if raw.saveCalls != 0 || target.CallCount() != 0 {
		t.Fatalf("locked update mutated state: saves=%d calls=%d", raw.saveCalls, target.CallCount())
	}

	noTargetRaw := &memoryRawStore{}
	noTarget := NewService(NewStore(noTargetRaw), nil, Environment{}, testLogger(t))
	_, err = noTarget.Update(context.Background(), SettingsPatch{Enabled: boolPointer(true)})
	if !errors.Is(err, ErrTargetUnavailable) {
		t.Fatalf("missing target error = %v, want ErrTargetUnavailable", err)
	}
	if noTargetRaw.saveCalls != 0 {
		t.Fatalf("missing target saved settings %d times", noTargetRaw.saveCalls)
	}
}

func TestServiceRejectsInvalidPatchBeforeSavingOrApplying(t *testing.T) {
	raw := &memoryRawStore{}
	target := &fakeTarget{capacity: 3}
	service := NewService(NewStore(raw), target, Environment{}, testLogger(t))

	_, err := service.Update(context.Background(), SettingsPatch{MaxSessions: intPointer(0)})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid patch error = %v, want ErrValidation", err)
	}
	if raw.saveCalls != 0 || target.CallCount() != 0 {
		t.Fatalf("invalid patch mutated state: saves=%d calls=%d", raw.saveCalls, target.CallCount())
	}
}

func TestServiceSerializesConcurrentPartialUpdates(t *testing.T) {
	raw := &memoryRawStore{}
	target := &fakeTarget{}
	service := NewService(NewStore(raw), target, Environment{}, testLogger(t))

	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := service.Update(context.Background(), SettingsPatch{Enabled: boolPointer(true)})
		results <- err
	}()
	go func() {
		<-start
		_, err := service.Update(context.Background(), SettingsPatch{MaxSessions: intPointer(8)})
		results <- err
	}()
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent service update: %v", err)
		}
	}

	response, err := service.Get(context.Background())
	if err != nil {
		t.Fatalf("get final settings: %v", err)
	}
	if response.Settings != (Settings{Enabled: true, MaxSessions: 8}) || target.Capacity() != 8 {
		t.Fatalf("final response=%+v live=%d", response, target.Capacity())
	}
}

func TestServiceLogsInvalidCapturedEnvironment(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("new observer logger: %v", err)
	}
	service := NewService(
		NewStore(&memoryRawStore{}), &fakeTarget{},
		Environment{Value: "bad", Present: true}, log,
	)
	if _, err := service.Get(context.Background()); err != nil {
		t.Fatalf("get: %v", err)
	}
	if logs.Len() != 1 || logs.All()[0].Message != "Ignoring invalid session capacity environment value" {
		t.Fatalf("warning logs = %+v", logs.All())
	}
}

func TestReadEnvironmentUsesPresenceSeparateFromValue(t *testing.T) {
	t.Setenv(EnvironmentVariable, "0")
	if got := ReadEnvironment(); got != (Environment{Value: "0", Present: true}) {
		t.Fatalf("present environment = %+v", got)
	}
	t.Setenv(EnvironmentVariable, "")
	if got := ReadEnvironment(); got != (Environment{Value: "", Present: true}) {
		t.Fatalf("blank environment = %+v", got)
	}
}

type fakeTarget struct {
	mu       sync.Mutex
	capacity int
	calls    []int
}

func (t *fakeTarget) SetSessionCapacity(capacity int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.capacity = capacity
	t.calls = append(t.calls, capacity)
}

func (t *fakeTarget) Capacity() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.capacity
}

func (t *fakeTarget) CallCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.calls)
}

func boolPointer(value bool) *bool { return &value }

func intPointer(value int) *int { return &value }

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log
}
