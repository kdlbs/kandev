package updates

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

	jobID, err := svc.Apply(context.Background(), "UPDATE")
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

func TestService_ApplyRejectsUnsupportedInstall(t *testing.T) {
	pool := newTestPool(t)
	if err := persistence.WriteLatestVersion(pool.Writer(), "v1.0.1", "https://example/v1.0.1", time.Now().UTC()); err != nil {
		t.Fatalf("write latest: %v", err)
	}
	svc := NewService(pool, "v1.0.0", nil, logger.Default(), WithJobs(jobs.NewTracker(nil, logger.Default())))

	_, err := svc.Apply(context.Background(), "UPDATE")
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
	svc := NewService(pool, "v1.0.0", nil, logger.Default(),
		WithHomeDir(homeDir),
		WithJobs(jobs.NewTracker(nil, logger.Default())),
		WithApplyRunner(func(_ context.Context, _ applyRequest) (map[string]interface{}, error) {
			<-release
			return map[string]interface{}{"status": "started"}, nil
		}),
	)

	if _, err := svc.Apply(context.Background(), "UPDATE"); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if _, err := svc.Apply(context.Background(), "UPDATE"); !errors.Is(err, ErrApplyInProgress) {
		t.Fatalf("second Apply err=%v want ErrApplyInProgress", err)
	}
	close(release)
}

func TestService_ApplyRejectsWrongConfirm(t *testing.T) {
	svc := NewService(newTestPool(t), "v1.0.0", nil, logger.Default())
	_, err := svc.Apply(context.Background(), "NOPE")
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
