package updates

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/persistence"
)

func configureManagedNPMInstall(t *testing.T) string {
	t.Helper()
	homeDir := t.TempDir()
	metadataPath, _ := writeServiceInstallForTest(t, homeDir, serviceInstallMetadata{
		Manager:     serviceManagerSystemd,
		Mode:        installModeUser,
		Kind:        installKindNPM,
		HomeDir:     homeDir,
		LogDir:      filepath.Join(homeDir, "logs"),
		ServicePath: filepath.Join(homeDir, "kandev.service"),
		NodePath:    "/usr/bin/node",
		CLIEntry:    "/usr/lib/node_modules/kandev/bin/cli.js",
	})
	t.Setenv(envRunningAsService, "true")
	t.Setenv(envServiceMode, installModeUser)
	t.Setenv(envServiceManager, serviceManagerSystemd)
	t.Setenv(envInstallKind, installKindNPM)
	t.Setenv(envServiceMetadata, metadataPath)
	return homeDir
}

type memorySettingsStore struct {
	mu      sync.Mutex
	value   []byte
	present bool
	getErr  error
	saveErr error
}

type cancelAwareSettingsStore struct {
	calls atomic.Int32
}

type failingSettingsStore struct {
	getCalls atomic.Int32
}

func (s *failingSettingsStore) Get(context.Context, string) ([]byte, bool, error) {
	s.getCalls.Add(1)
	return nil, false, errors.New("settings unavailable")
}

func (*failingSettingsStore) Save(context.Context, string, []byte) error {
	return errors.New("settings unavailable")
}

func (s *cancelAwareSettingsStore) Get(ctx context.Context, _ string) ([]byte, bool, error) {
	if s.calls.Add(1) == 1 {
		return []byte(ChannelNightly), true, nil
	}
	return nil, false, ctx.Err()
}

func (*cancelAwareSettingsStore) Save(context.Context, string, []byte) error {
	return nil
}

func (s *memorySettingsStore) Get(context.Context, string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.value...), s.present, s.getErr
}

func (s *memorySettingsStore) Save(_ context.Context, _ string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.value = append([]byte(nil), value...)
	s.present = true
	return nil
}

func TestSelectedChannelDefaultsInvalidAndMissingValuesToStable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		store   *memorySettingsStore
		wantErr bool
	}{
		{name: "missing", store: &memorySettingsStore{}},
		{name: "invalid", store: &memorySettingsStore{value: []byte("preview"), present: true}},
		{name: "read failure", store: &memorySettingsStore{getErr: errors.New("db down")}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default(), WithSettingsStore(tc.store))
			got, err := svc.selectedChannel(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("selectedChannel error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("selectedChannel: %v", err)
			}
			if got != ChannelStable {
				t.Fatalf("channel=%q want %q", got, ChannelStable)
			}
		})
	}
}

func TestPersistedNightlyChannelSurvivesServiceRestart(t *testing.T) {
	store := &memorySettingsStore{}
	first := NewService(newTestPool(t), "v1.0.0", nil, logger.Default(), WithSettingsStore(store))
	if err := first.persistChannel(context.Background(), ChannelNightly); err != nil {
		t.Fatalf("persistChannel: %v", err)
	}
	second := NewService(first.pool, "v1.0.0", nil, logger.Default(), WithSettingsStore(store))
	got, err := second.selectedChannel(context.Background())
	if err != nil {
		t.Fatalf("selectedChannel: %v", err)
	}
	if got != ChannelNightly {
		t.Fatalf("channel=%q want nightly", got)
	}

	if err := second.persistChannel(context.Background(), Channel("preview")); err == nil {
		t.Fatal("persistChannel accepted invalid channel")
	}
}

func TestGetReadsOnlyTheSelectedChannelCache(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	pool := newTestPool(t)
	stableAt := time.Unix(1_700_000_000, 0).UTC()
	nightlyAt := stableAt.Add(time.Hour)
	if err := persistence.WriteLatestVersion(pool.Writer(), "v1.2.3", "https://example/stable", stableAt); err != nil {
		t.Fatal(err)
	}
	if err := persistence.WriteLatestNightlyVersion(
		pool.Writer(),
		"1.2.4-nightly.shaabc123def456",
		"https://example/nightly",
		nightlyAt,
	); err != nil {
		t.Fatal(err)
	}
	store := &memorySettingsStore{value: []byte(ChannelNightly), present: true}
	svc := NewService(pool, "v1.2.3", nil, logger.Default(), WithHomeDir(homeDir), WithSettingsStore(store))
	resp, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Latest != "1.2.4-nightly.shaabc123def456" || resp.LatestURL != "https://example/nightly" || !resp.LatestCheckedAt.Equal(nightlyAt) {
		t.Fatalf("Get returned wrong cache: %+v", resp)
	}
}

func TestNightlyAvailabilityTreatsUnequalSHAsAsNewTargets(t *testing.T) {
	svc := NewService(nil, "v1.2.4-nightly.shaffffffffffff", nil, logger.Default())
	if !svc.updateAvailableFor(ChannelNightly, "1.2.4-nightly.sha000000000000") {
		t.Fatal("unequal authoritative nightly SHA should be available regardless of lexical order")
	}
	if svc.updateAvailableFor(ChannelNightly, "1.2.4-nightly.shaffffffffffff") {
		t.Fatal("identical nightly version should not be available")
	}
}

func TestNightlyAvailabilityRejectsLowerNumericVersion(t *testing.T) {
	svc := NewService(nil, "v1.2.5-nightly.shaaaaaaaaaaaaa", nil, logger.Default())
	if svc.updateAvailableFor(ChannelNightly, "v1.2.4-nightly.shabbbbbbbbbbbb") {
		t.Fatal("lower numeric nightly version must not be offered as an update")
	}
	if !svc.updateAvailableFor(ChannelNightly, "v1.2.6-nightly.shabbbbbbbbbbbb") {
		t.Fatal("higher numeric nightly version should be offered as an update")
	}
}

func TestNightlyChannelDoesNotOfferPrereleaseOfInstalledStable(t *testing.T) {
	svc := NewService(nil, "v1.2.4", nil, logger.Default())
	if svc.updateAvailableFor(ChannelNightly, "v1.2.4-nightly.shaaaaaaaaaaaaa") {
		t.Fatal("a prerelease of the installed stable version must not be offered as an update")
	}
	if !svc.updateAvailableFor(ChannelNightly, "v1.2.5-nightly.shaaaaaaaaaaaaa") {
		t.Fatal("the next patch nightly should be offered to a stable install")
	}
}

func TestCheckUsesNightlyResolverAndWritesOnlyNightlyCache(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	pool := newTestPool(t)
	store := &memorySettingsStore{value: []byte(ChannelNightly), present: true}
	svc := NewService(pool, "v1.2.3", nil, logger.Default(), WithHomeDir(homeDir), WithSettingsStore(store))
	stableCalled := false
	svc.SetFetcher(func(context.Context) (string, string, error) {
		stableCalled = true
		return "", "", errors.New("stable resolver should not run")
	})
	svc.SetNightlyFetcher(func(context.Context) (string, string, error) {
		return "1.2.4-nightly.shaabc123def456", "https://example/nightly", nil
	})

	resp, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if stableCalled {
		t.Fatal("stable resolver ran for nightly channel")
	}
	if resp.Latest != "1.2.4-nightly.shaabc123def456" {
		t.Fatalf("latest=%q", resp.Latest)
	}
	stable, _, _, err := persistence.ReadLatestVersion(pool.Reader())
	if err != nil {
		t.Fatal(err)
	}
	nightly, _, _, err := persistence.ReadLatestNightlyVersion(pool.Reader())
	if err != nil {
		t.Fatal(err)
	}
	if stable != "" || nightly != "1.2.4-nightly.shaabc123def456" {
		t.Fatalf("cache isolation failed: stable=%q nightly=%q", stable, nightly)
	}
}

func TestUnsupportedInstallForcesPersistedNightlyPreferenceToStable(t *testing.T) {
	pool := newTestPool(t)
	if err := persistence.WriteLatestVersion(pool.Writer(), "v1.2.3", "https://example/stable", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := persistence.WriteLatestNightlyVersion(
		pool.Writer(),
		"1.2.4-nightly.shaabc123def456",
		"https://example/nightly",
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	store := &memorySettingsStore{value: []byte(ChannelNightly), present: true}
	svc := NewService(pool, "v1.2.3", nil, logger.Default(), WithSettingsStore(store))
	resp, err := svc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Channel != ChannelStable || resp.Latest != "v1.2.3" || resp.ChannelEditable {
		t.Fatalf("unsupported effective response=%+v", resp)
	}
}

func TestUnsupportedInstallDoesNotReadChannelSettings(t *testing.T) {
	store := &failingSettingsStore{}
	svc := NewService(newTestPool(t), "v1.2.3", nil, logger.Default(), WithSettingsStore(store))

	resp, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Channel != ChannelStable || resp.ChannelEditable {
		t.Fatalf("unsupported effective response=%+v", resp)
	}
	if got := store.getCalls.Load(); got != 0 {
		t.Fatalf("settings reads=%d want 0", got)
	}
}

func TestManagedNPMInstallWithoutSettingsStoreIsNotChannelEditable(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	svc := NewService(newTestPool(t), "v1.2.3", nil, logger.Default(), WithHomeDir(homeDir))

	resp, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.ChannelEditable || resp.ChannelUnsupportedReason == "" {
		t.Fatalf("channel capability editable=%v reason=%q", resp.ChannelEditable, resp.ChannelUnsupportedReason)
	}
}

func TestSelectChannelWithoutSettingsStoreRejectsBeforeResolution(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	svc := NewService(newTestPool(t), "v1.2.3", nil, logger.Default(), WithHomeDir(homeDir))
	var resolutions atomic.Int32
	svc.SetNightlyFetcher(func(context.Context) (string, string, error) {
		resolutions.Add(1)
		return "v1.2.4-nightly.shaabc123def456", "https://example/nightly", nil
	})

	_, err := svc.SelectChannel(context.Background(), string(ChannelNightly))
	if !errors.Is(err, ErrChannelUnsupported) {
		t.Fatalf("SelectChannel error=%v want ErrChannelUnsupported", err)
	}
	if got := resolutions.Load(); got != 0 {
		t.Fatalf("upstream resolutions=%d want 0", got)
	}
}

func TestFailedCheckReturnsCapturedChannelCacheAfterContextCancellation(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	pool := newTestPool(t)
	checkedAt := time.Unix(1_700_000_000, 0).UTC()
	if err := persistence.WriteLatestNightlyVersion(
		pool.Writer(),
		"v1.2.4-nightly.shaabc123def456",
		"https://example/nightly",
		checkedAt,
	); err != nil {
		t.Fatal(err)
	}
	store := &cancelAwareSettingsStore{}
	svc := NewService(pool, "v1.2.3", nil, logger.Default(), WithHomeDir(homeDir), WithSettingsStore(store))
	ctx, cancel := context.WithCancel(context.Background())
	svc.SetNightlyFetcher(func(context.Context) (string, string, error) {
		cancel()
		return "", "", errors.New("registry unavailable")
	})

	resp, err := svc.fetchAndPersist(ctx)
	if err == nil || !strings.Contains(err.Error(), "registry unavailable") {
		t.Fatalf("error=%v", err)
	}
	if resp.Latest != "v1.2.4-nightly.shaabc123def456" || !resp.LatestCheckedAt.Equal(checkedAt) {
		t.Fatalf("fallback response lost cached state: %+v", resp)
	}
	if got := store.calls.Load(); got != 1 {
		t.Fatalf("settings reads=%d want 1", got)
	}
}

func TestSelectChannelSharesManualUpstreamRateLimit(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	svc := NewService(
		newTestPool(t),
		"v1.2.3",
		nil,
		logger.Default(),
		WithHomeDir(homeDir),
		WithSettingsStore(&memorySettingsStore{}),
	)
	var resolutions atomic.Int32
	svc.SetNightlyFetcher(func(context.Context) (string, string, error) {
		resolutions.Add(1)
		return "v1.2.4-nightly.shaabc123def456", "https://example/nightly", nil
	})

	if _, err := svc.SelectChannel(context.Background(), string(ChannelNightly)); err != nil {
		t.Fatalf("first SelectChannel: %v", err)
	}
	if _, err := svc.SelectChannel(context.Background(), string(ChannelNightly)); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second SelectChannel error=%v want ErrRateLimited", err)
	}
	if got := resolutions.Load(); got != 1 {
		t.Fatalf("upstream resolutions=%d want 1", got)
	}
}

func TestFetchAndPersistSerializesCacheRefreshes(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	pool := newTestPool(t)
	store := &memorySettingsStore{value: []byte(ChannelNightly), present: true}
	svc := NewService(pool, "v1.2.3", nil, logger.Default(), WithHomeDir(homeDir), WithSettingsStore(store))
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	closeReleaseFirst := sync.OnceFunc(func() { close(releaseFirst) })
	t.Cleanup(closeReleaseFirst)
	secondEntered := make(chan struct{})
	releaseSecond := make(chan struct{})
	closeReleaseSecond := sync.OnceFunc(func() { close(releaseSecond) })
	t.Cleanup(closeReleaseSecond)
	var calls atomic.Int32
	svc.SetNightlyFetcher(func(context.Context) (string, string, error) {
		if calls.Add(1) == 1 {
			close(firstEntered)
			<-releaseFirst
			return "v1.2.4-nightly.shaaaaaaaaaaaaa", "https://example/first", nil
		}
		close(secondEntered)
		<-releaseSecond
		return "v1.2.5-nightly.shabbbbbbbbbbbb", "https://example/second", nil
	})

	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		_, err := svc.fetchAndPersist(context.Background())
		firstDone <- err
	}()
	select {
	case <-firstEntered:
	case err := <-firstDone:
		t.Fatalf("first refresh returned before resolver: %v", err)
	}
	if svc.updateMu.TryLock() {
		svc.updateMu.Unlock()
		t.Fatal("refresh lock was not held while first resolver was active")
	}
	go func() {
		_, err := svc.fetchAndPersist(context.Background())
		secondDone <- err
	}()

	closeReleaseFirst()
	if err := <-firstDone; err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	select {
	case <-secondEntered:
	case err := <-secondDone:
		t.Fatalf("second refresh returned before resolver: %v", err)
	}
	version, targetURL, _, err := persistence.ReadLatestNightlyVersion(pool.Reader())
	if err != nil {
		t.Fatal(err)
	}
	if version != "v1.2.4-nightly.shaaaaaaaaaaaaa" || targetURL != "https://example/first" {
		t.Fatalf("cache before second release version=%q url=%q", version, targetURL)
	}

	closeReleaseSecond()
	if err := <-secondDone; err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	version, targetURL, _, err = persistence.ReadLatestNightlyVersion(pool.Reader())
	if err != nil {
		t.Fatal(err)
	}
	if version != "v1.2.5-nightly.shabbbbbbbbbbbb" || targetURL != "https://example/second" {
		t.Fatalf("final cache version=%q url=%q", version, targetURL)
	}
}
