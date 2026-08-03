package updates

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/system/jobs"
)

func TestService_ApplyQueuesSelfUpdateJobAndWritesIntent(t *testing.T) {
	homeDir := t.TempDir()
	metadataPath, _ := writeServiceInstallForTest(t, homeDir, serviceInstallMetadata{
		Manager:     "systemd",
		Mode:        "user",
		Kind:        "npm",
		HomeDir:     homeDir,
		LogDir:      filepath.Join(homeDir, "logs"),
		ServicePath: filepath.Join(homeDir, "kandev.service"),
		NodePath:    "/usr/bin/node",
		CLIEntry:    "/usr/lib/node_modules/kandev/bin/cli.js",
		Port:        38429,
	})
	t.Setenv(envRunningAsService, "true")
	t.Setenv(envServiceMode, "user")
	t.Setenv(envServiceManager, "systemd")
	t.Setenv(envInstallKind, "npm")
	t.Setenv(envServiceMetadata, metadataPath)

	pool := newTestPool(t)
	if err := persistence.WriteLatestVersion(pool.Writer(), "v1.0.1", "https://example/v1.0.1", time.Now().UTC()); err != nil {
		t.Fatalf("write latest: %v", err)
	}
	tracker := jobs.NewTracker(nil, logger.Default())
	var gotReq applyRequest
	svc := NewService(
		pool,
		"v1.0.0",
		nil,
		logger.Default(),
		WithHomeDir(homeDir),
		WithJobs(tracker),
		WithApplyRunner(func(_ context.Context, req applyRequest) (map[string]interface{}, error) {
			gotReq = req
			return map[string]interface{}{"status": "started"}, nil
		}),
	)

	jobID, err := svc.Apply(context.Background(), "UPDATE", "v1.0.1")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	waitForJobState(t, tracker, jobID, jobs.StateSucceeded)
	if gotReq.IntentPath == "" {
		t.Fatalf("runner did not receive intent path")
	}
	data, err := os.ReadFile(gotReq.IntentPath)
	if err != nil {
		t.Fatalf("read intent: %v", err)
	}
	var intent updateIntent
	if err := json.Unmarshal(data, &intent); err != nil {
		t.Fatalf("unmarshal intent: %v", err)
	}
	if intent.TargetVersion != "1.0.1" {
		t.Fatalf("TargetVersion=%q want 1.0.1", intent.TargetVersion)
	}
	if intent.Install.Port != 38429 {
		t.Fatalf("Port=%d want 38429", intent.Install.Port)
	}
}

func TestService_ApplyUsesCachedExactNightlyTarget(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	pool := newTestPool(t)
	const packageVersion = "1.2.4-nightly.shaabc123def456"
	const targetTag = "v" + packageVersion
	if err := persistence.WriteLatestNightlyVersion(
		pool.Writer(),
		targetTag,
		"https://example/cached-nightly",
		time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	store := &memorySettingsStore{value: []byte(ChannelNightly), present: true}
	tracker := jobs.NewTracker(nil, logger.Default())
	var gotReq applyRequest
	svc := NewService(
		pool,
		"v1.2.3",
		nil,
		logger.Default(),
		WithHomeDir(homeDir),
		WithSettingsStore(store),
		WithJobs(tracker),
		WithApplyRunner(func(_ context.Context, req applyRequest) (map[string]interface{}, error) {
			gotReq = req
			return map[string]interface{}{"status": "started"}, nil
		}),
	)
	var resolutions atomic.Int32
	svc.SetNightlyFetcher(func(context.Context) (string, string, error) {
		resolutions.Add(1)
		return "v1.2.5-nightly.shafedcba654321", "https://example/new-nightly", nil
	})

	jobID, err := svc.Apply(context.Background(), "UPDATE", targetTag)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	waitForJobState(t, tracker, jobID, jobs.StateSucceeded)
	if gotReq.Intent.TargetTag != targetTag || gotReq.Intent.TargetVersion != packageVersion {
		t.Fatalf(
			"intent target tag=%q version=%q want tag=%q version=%q",
			gotReq.Intent.TargetTag,
			gotReq.Intent.TargetVersion,
			targetTag,
			packageVersion,
		)
	}
	if got := resolutions.Load(); got != 0 {
		t.Fatalf("nightly resolutions=%d want 0", got)
	}
}

func TestService_ApplyRejectsChangedCachedTarget(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	pool := newTestPool(t)
	const cachedTarget = "v1.2.5-nightly.shafedcba654321"
	if err := persistence.WriteLatestNightlyVersion(
		pool.Writer(),
		cachedTarget,
		"https://example/new-nightly",
		time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	svc := NewService(
		pool,
		"v1.2.3",
		nil,
		logger.Default(),
		WithHomeDir(homeDir),
		WithSettingsStore(&memorySettingsStore{value: []byte(ChannelNightly), present: true}),
		WithJobs(jobs.NewTracker(nil, logger.Default())),
	)

	_, err := svc.Apply(context.Background(), "UPDATE", "v1.2.4-nightly.shaabc123def456")
	if !errors.Is(err, ErrUpdateTargetChanged) {
		t.Fatalf("Apply error=%v want ErrUpdateTargetChanged", err)
	}
}

func TestService_ApplyRejectsConcurrentNightlyWhileFirstApplyIsRunning(t *testing.T) {
	homeDir := configureManagedNPMInstall(t)
	pool := newTestPool(t)
	release := make(chan struct{})
	closeRelease := sync.OnceFunc(func() { close(release) })
	t.Cleanup(closeRelease)
	svc := NewService(
		pool,
		"v1.2.3",
		nil,
		logger.Default(),
		WithHomeDir(homeDir),
		WithSettingsStore(&memorySettingsStore{value: []byte(ChannelNightly), present: true}),
		WithJobs(jobs.NewTracker(nil, logger.Default())),
		WithApplyRunner(func(_ context.Context, _ applyRequest) (map[string]interface{}, error) {
			<-release
			return map[string]interface{}{"status": "started"}, nil
		}),
	)
	const target = "v1.2.4-nightly.shaabc123def456"
	if err := persistence.WriteLatestNightlyVersion(
		pool.Writer(),
		target,
		"https://example/nightly",
		time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Apply(context.Background(), "UPDATE", target); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if _, err := svc.Apply(context.Background(), "UPDATE", target); !errors.Is(err, ErrApplyInProgress) {
		t.Fatalf("second Apply error=%v want ErrApplyInProgress", err)
	}
	closeRelease()
}

func TestWriteApplyIntentPreservesNativeNoBootStart(t *testing.T) {
	homeDir := t.TempDir()
	metadataPath := filepath.Join(homeDir, "service", "install.json")
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{
		"version": 1,
		"manager": "systemd",
		"mode": "user",
		"kind": "npm",
		"home_dir": "/home/alice/.kandev",
		"log_dir": "/home/alice/.kandev/logs",
		"service_path": "/home/alice/.config/systemd/user/kandev.service",
		"launcher_path": "/usr/local/lib/node_modules/@kdlbs/runtime-linux-x64/bin/kandev",
		"no_boot_start": true,
		"installed_at": "2026-07-25T00:00:00Z"
	}`)
	if err := os.WriteFile(metadataPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	metadata, err := readServiceMetadata(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default(), WithHomeDir(homeDir))
	intentPath, _, err := svc.writeApplyIntent(UpdatesResponse{Latest: "v1.2.3"}, metadata)
	if err != nil {
		t.Fatal(err)
	}
	intentData, err := os.ReadFile(intentPath)
	if err != nil {
		t.Fatal(err)
	}
	var intent map[string]interface{}
	if err := json.Unmarshal(intentData, &intent); err != nil {
		t.Fatal(err)
	}
	install, _ := intent["install"].(map[string]interface{})
	if install["no_boot_start"] != true {
		t.Fatalf("intent install.no_boot_start = %#v, want true; intent=%v", install["no_boot_start"], intent)
	}
}

func TestService_ApplyRejectsUnsupportedInstall(t *testing.T) {
	pool := newTestPool(t)
	if err := persistence.WriteLatestVersion(pool.Writer(), "v1.0.1", "https://example/v1.0.1", time.Now().UTC()); err != nil {
		t.Fatalf("write latest: %v", err)
	}
	svc := NewService(pool, "v1.0.0", nil, logger.Default(), WithJobs(jobs.NewTracker(nil, logger.Default())))

	_, err := svc.Apply(context.Background(), "UPDATE", "v1.0.1")
	if !errors.Is(err, ErrApplyUnsupported) {
		t.Fatalf("err=%v want ErrApplyUnsupported", err)
	}
}

func TestService_ApplyRejectsConcurrentSelfUpdate(t *testing.T) {
	homeDir := t.TempDir()
	metadataPath, _ := writeServiceInstallForTest(t, homeDir, serviceInstallMetadata{
		Manager:     "systemd",
		Mode:        "user",
		Kind:        "npm",
		HomeDir:     homeDir,
		LogDir:      filepath.Join(homeDir, "logs"),
		ServicePath: filepath.Join(homeDir, "kandev.service"),
		NodePath:    "/usr/bin/node",
		CLIEntry:    "/usr/lib/node_modules/kandev/bin/cli.js",
	})
	t.Setenv(envRunningAsService, "true")
	t.Setenv(envServiceMode, "user")
	t.Setenv(envServiceManager, "systemd")
	t.Setenv(envInstallKind, "npm")
	t.Setenv(envServiceMetadata, metadataPath)

	pool := newTestPool(t)
	if err := persistence.WriteLatestVersion(pool.Writer(), "v1.0.1", "https://example/v1.0.1", time.Now().UTC()); err != nil {
		t.Fatalf("write latest: %v", err)
	}
	// Block the first helper so its job stays running while the second apply is
	// attempted, making the in-flight guard deterministic.
	release := make(chan struct{})
	closeRelease := sync.OnceFunc(func() { close(release) })
	t.Cleanup(closeRelease)
	svc := NewService(pool, "v1.0.0", nil, logger.Default(),
		WithHomeDir(homeDir),
		WithJobs(jobs.NewTracker(nil, logger.Default())),
		WithApplyRunner(func(_ context.Context, _ applyRequest) (map[string]interface{}, error) {
			<-release
			return map[string]interface{}{"status": "started"}, nil
		}),
	)

	if _, err := svc.Apply(context.Background(), "UPDATE", "v1.0.1"); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if _, err := svc.Apply(context.Background(), "UPDATE", "v1.0.1"); !errors.Is(err, ErrApplyInProgress) {
		t.Fatalf("second Apply err=%v want ErrApplyInProgress", err)
	}
	closeRelease()
}

func TestService_ApplyGuardExpiresAfterTTL(t *testing.T) {
	homeDir := t.TempDir()
	metadataPath, _ := writeServiceInstallForTest(t, homeDir, serviceInstallMetadata{
		Manager:     "systemd",
		Mode:        "user",
		Kind:        "npm",
		HomeDir:     homeDir,
		LogDir:      filepath.Join(homeDir, "logs"),
		ServicePath: filepath.Join(homeDir, "kandev.service"),
		NodePath:    "/usr/bin/node",
		CLIEntry:    "/usr/lib/node_modules/kandev/bin/cli.js",
	})
	t.Setenv(envRunningAsService, "true")
	t.Setenv(envServiceMode, "user")
	t.Setenv(envServiceManager, "systemd")
	t.Setenv(envInstallKind, "npm")
	t.Setenv(envServiceMetadata, metadataPath)

	pool := newTestPool(t)
	if err := persistence.WriteLatestVersion(pool.Writer(), "v1.0.1", "https://example/v1.0.1", time.Now().UTC()); err != nil {
		t.Fatalf("write latest: %v", err)
	}
	svc := NewService(pool, "v1.0.0", nil, logger.Default(),
		WithHomeDir(homeDir),
		WithJobs(jobs.NewTracker(nil, logger.Default())),
		WithApplyRunner(func(_ context.Context, _ applyRequest) (map[string]interface{}, error) {
			// Launch "succeeds" (helper started) but never restarts the backend,
			// so the guard is not released by an error path.
			return map[string]interface{}{"status": "started"}, nil
		}),
	)
	clock := time.Now()
	svc.now = func() time.Time { return clock }

	if _, err := svc.Apply(context.Background(), "UPDATE", "v1.0.1"); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	// Within the TTL a second apply is still refused.
	if _, err := svc.Apply(context.Background(), "UPDATE", "v1.0.1"); !errors.Is(err, ErrApplyInProgress) {
		t.Fatalf("within TTL err=%v want ErrApplyInProgress", err)
	}
	// Past the TTL the guard expires (helper assumed dead) and a retry succeeds.
	clock = clock.Add(applyGuardTTL + time.Second)
	if _, err := svc.Apply(context.Background(), "UPDATE", "v1.0.1"); err != nil {
		t.Fatalf("after TTL Apply: %v", err)
	}
}

func TestService_ApplyRejectsWrongConfirm(t *testing.T) {
	svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default())
	_, err := svc.Apply(context.Background(), "NOPE", "v1.0.1")
	if !errors.Is(err, ErrApplyConfirm) {
		t.Fatalf("err=%v want ErrApplyConfirm", err)
	}
}

func TestSystemdSelfUpdateArgsPropagateUpdateEnvironment(t *testing.T) {
	t.Setenv("PATH", "/opt/homebrew/bin:/usr/bin")
	t.Setenv("npm_config_prefix", "/tmp/npm-global")
	t.Setenv("NPM_CONFIG_PREFIX", "/tmp/npm-global")

	req := applyRequest{
		IntentPath: "/tmp/intent.json",
		Intent: updateIntent{Install: serviceInstallMetadata{
			NodePath: "/opt/homebrew/bin/node",
			CLIEntry: "/tmp/npm-global/lib/node_modules/kandev/bin/cli.js",
		}},
	}

	got := systemdSelfUpdateArgs(req, "kandev-self-update-test")
	want := []string{
		"--user",
		"--unit", "kandev-self-update-test",
		"--collect",
		"--setenv=PATH=/opt/homebrew/bin:/usr/bin",
		"--setenv=npm_config_prefix=/tmp/npm-global",
		"--setenv=NPM_CONFIG_PREFIX=/tmp/npm-global",
		"/opt/homebrew/bin/node",
		"/tmp/npm-global/lib/node_modules/kandev/bin/cli.js",
		"service",
		"self-update",
		"--intent",
		"/tmp/intent.json",
	}
	if !stringSlicesEqual(got, want) {
		t.Fatalf("args=%#v want %#v", got, want)
	}
}

func TestSystemdSelfUpdateArgsUseNativeLauncherPath(t *testing.T) {
	var req applyRequest
	data := []byte(`{
		"intent_path": "/tmp/unused",
		"intent": {
			"install": {
				"launcher_path": "/opt/homebrew/Cellar/kandev/1.2.3/libexec/bin/kandev"
			}
		}
	}`)
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	req.IntentPath = "/tmp/intent.json"

	got := systemdSelfUpdateArgs(req, "kandev-self-update-test")
	wantTail := []string{
		"/opt/homebrew/Cellar/kandev/1.2.3/libexec/bin/kandev",
		"service",
		"self-update",
		"--intent",
		"/tmp/intent.json",
	}
	if len(got) < len(wantTail) || !stringSlicesEqual(got[len(got)-len(wantTail):], wantTail) {
		t.Fatalf("args tail=%#v, want %#v; all args=%#v", got, wantTail, got)
	}
}

func TestRenderLaunchdHelperPlistRunsOnceWithUpdateEnvironment(t *testing.T) {
	t.Setenv("PATH", "/opt/homebrew/bin:/usr/bin")
	t.Setenv("npm_config_prefix", "/tmp/npm-global")
	t.Setenv("NPM_CONFIG_PREFIX", "/tmp/npm-global")

	req := applyRequest{
		IntentPath: "/tmp/intent.json",
		Intent: updateIntent{Install: serviceInstallMetadata{
			NodePath: "/opt/homebrew/bin/node",
			CLIEntry: "/tmp/npm-global/lib/node_modules/kandev/bin/cli.js",
			LogDir:   "/tmp/kandev/logs",
		}},
	}

	plist := renderLaunchdHelperPlist("com.kdlbs.kandev.self-update.test", req)

	// The whole point of this change: the helper must run exactly once. A
	// KeepAlive=true (the implicit behaviour of `launchctl submit`) would make
	// launchd re-run the one-shot updater forever.
	if !strings.Contains(plist, "<key>KeepAlive</key>\n  <false/>") {
		t.Fatalf("plist must set KeepAlive=false to run once:\n%s", plist)
	}
	for _, want := range []string{
		"<string>com.kdlbs.kandev.self-update.test</string>",
		"<key>RunAtLoad</key>\n  <true/>",
		"<string>/opt/homebrew/bin/node</string>",
		"<string>/tmp/npm-global/lib/node_modules/kandev/bin/cli.js</string>",
		"<string>self-update</string>",
		"<string>/tmp/intent.json</string>",
		"<key>PATH</key>\n    <string>/opt/homebrew/bin:/usr/bin</string>",
		"<key>NPM_CONFIG_PREFIX</key>\n    <string>/tmp/npm-global</string>",
		"<string>/tmp/kandev/logs/self-update-launchd.log</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q:\n%s", want, plist)
		}
	}
	// Must not regress to the looping `launchctl submit` mechanism.
	if strings.Contains(plist, "submit") {
		t.Fatalf("plist should not reference submit:\n%s", plist)
	}
}

func TestRenderLaunchdHelperPlistUsesNativeLauncherPath(t *testing.T) {
	var req applyRequest
	data := []byte(`{
		"install": {
			"launcher_path": "/opt/homebrew/Cellar/kandev/1.2.3/libexec/bin/kandev",
			"log_dir": "/tmp/kandev/logs"
		}
	}`)
	if err := json.Unmarshal(data, &req.Intent); err != nil {
		t.Fatal(err)
	}
	req.IntentPath = "/tmp/intent.json"

	plist := renderLaunchdHelperPlist("com.kdlbs.kandev.self-update.test", req)
	if !strings.Contains(plist,
		"<string>/opt/homebrew/Cellar/kandev/1.2.3/libexec/bin/kandev</string>\n"+
			"    <string>service</string>") {
		t.Fatalf("plist does not invoke native launcher directly:\n%s", plist)
	}
}

func TestLaunchdSelfUpdateDomain(t *testing.T) {
	user := applyRequest{Intent: updateIntent{Install: serviceInstallMetadata{Mode: installModeUser}}}
	if got := launchdSelfUpdateDomain(user, 501); got != "gui/501" {
		t.Fatalf("user domain=%q want gui/501", got)
	}
	system := applyRequest{Intent: updateIntent{Install: serviceInstallMetadata{Mode: installModeSystem}}}
	if got := launchdSelfUpdateDomain(system, 501); got != "system" {
		t.Fatalf("system domain=%q want system", got)
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func waitForJobState(t *testing.T, tracker *jobs.Tracker, id string, want jobs.State) *jobs.Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job := tracker.Get(id)
		if job != nil && job.State == want {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach %s", id, want)
	return nil
}
