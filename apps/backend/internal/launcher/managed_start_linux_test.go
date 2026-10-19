//go:build linux

package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
)

const managedBackendFixtureEnv = "KANDEV_MANAGED_BACKEND_FIXTURE"

// @covers AC-EXECUTORS-SURVIVAL-001.13
func TestManagedBackendLifetimeFollowsLauncher(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bound bool
	}{
		{"service", true}, {"no-service-metadata", false}, {"other-label", false},
	} {
		t.Run(tc.name, func(t *testing.T) { testManagedBackendParentDeath(t, tc.name, tc.bound) })
	}
	t.Run("start-error", func(t *testing.T) {
		env := upsertEnv(os.Environ(), backendPIDFileEnv, "")
		env = upsertEnv(env, "KANDEV_SERVICE_MANAGER", nativeServiceManagerSystemd)
		proc, _, err := startProcess(filepath.Join(t.TempDir(), "absent-binary"), nil, "", env, true, managedBackendLabel, newSupervisor())
		if err == nil || proc != nil {
			t.Fatalf("missing backend must return start error and no process: %v %v", proc, err)
		}
	})
}

func testManagedBackendParentDeath(t *testing.T, mode string, bound bool) {
	t.Helper()
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestManagedBackendFixture$", "--", "managed-backend-fixture", "launcher", dir, mode)
	cmd.Env = append(os.Environ(), managedBackendFixtureEnv+"="+dir)
	var launcher processidentity.Identity
	var backend serviceActorIdentity
	joined := false
	done := make(chan error, 1)
	t.Cleanup(func() {
		if cmd.Process != nil && !joined {
			if launcher.PID == 0 {
				_ = cmd.Process.Kill()
			} else {
				killServiceActor(t, launcher)
			}
			select {
			case <-done:
				joined = true
			case <-time.After(3 * time.Second):
				t.Error("fixture launcher did not join")
			}
		}
		if backend.Process.PID == 0 {
			backend = readServiceActor(filepath.Join(dir, "backend.json"))
		}
		if backend.Process.PID != 0 {
			if state, err := processidentity.Inspect(backend.Process); err == nil && state == processidentity.StateAlive {
				killServiceActor(t, backend.Process)
			}
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cleanupCancel()
			awaitServiceProcessExit(t, cleanupCtx, backend.Process)
		}
		t.Logf("owned launcher=%d backend=%d cleaned/joined=%t", launcher.PID, backend.Process.PID, joined)
	})
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	var err error
	launcher, err = processidentity.Capture(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	backend = awaitServiceActor(t, ctx, filepath.Join(dir, "backend.json"))
	awaitBackendCallerReturn(t, ctx, dir)
	t.Logf("mode=%s launcher=%d birth=%s backend=%d birth=%s", mode, launcher.PID, launcher.BirthToken, backend.Process.PID, backend.Process.BirthToken)
	assertBackendNoTerm(t, ctx, dir, backend.Process, 2*time.Second)
	killServiceActor(t, launcher)
	select {
	case err := <-done:
		joined = true
		if err == nil {
			t.Fatal("fixture launcher must exit from SIGKILL")
		}
	case <-ctx.Done():
		t.Fatal("killed launcher did not join")
	}
	if bound {
		exitCtx, exitCancel := context.WithTimeout(ctx, 3*time.Second)
		defer exitCancel()
		awaitServiceProcessExit(t, exitCtx, backend.Process)
		if _, err := os.Stat(filepath.Join(dir, "backend-term")); err != nil {
			t.Fatalf("backend must receive SIGTERM after launcher death: %v", err)
		}
	} else {
		assertBackendNoTerm(t, ctx, dir, backend.Process, 200*time.Millisecond)
	}
}

func awaitBackendCallerReturn(t *testing.T, ctx context.Context, dir string) {
	t.Helper()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(filepath.Join(dir, "caller-returned")); err == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("starting caller did not return")
		case <-tick.C:
		}
	}
}

func assertBackendNoTerm(t *testing.T, ctx context.Context, dir string, identity processidentity.Identity, duration time.Duration) {
	t.Helper()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	end := time.NewTimer(duration)
	defer end.Stop()
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend-term")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("backend received unexpected SIGTERM, marker error=%v", err)
		}
		if state, err := processidentity.Inspect(identity); err != nil || state != processidentity.StateAlive {
			t.Fatalf("healthy backend not alive: %s %v", state, err)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-end.C:
			return
		case <-tick.C:
		}
	}
}

func TestManagedBackendFixture(t *testing.T) {
	if len(os.Args) != 7 || os.Args[2] != "--" || os.Args[3] != "managed-backend-fixture" || os.Getenv(managedBackendFixtureEnv) != os.Args[5] {
		t.Skip("explicit managed-backend fixture only")
	}
	role, dir, mode := os.Args[4], os.Args[5], os.Args[6]
	switch role {
	case "launcher":
		runManagedBackendLauncher(t, dir, mode)
	case managedBackendLabel:
		runManagedBackendActor(t, dir)
	default:
		t.Fatal("invalid managed-backend role")
	}
}

func runManagedBackendLauncher(t *testing.T, dir, mode string) {
	t.Helper()
	env := upsertEnv(os.Environ(), backendPIDFileEnv, "")
	manager, label := nativeServiceManagerSystemd, managedBackendLabel
	if mode == "no-service-metadata" {
		manager = ""
	}
	if mode == "other-label" {
		label = "fixture"
	}
	env = upsertEnv(env, "KANDEV_SERVICE_MANAGER", manager)
	started := make(chan error, 1)
	go func() {
		// Returning with this thread locked retires the caller's thread.
		runtime.LockOSThread()
		_, _, err := startProcess(os.Args[0], []string{"-test.run=^TestManagedBackendFixture$", "--", "managed-backend-fixture", managedBackendLabel, dir, mode}, "", env, true, label, newSupervisor())
		started <- err
	}()
	if err := <-started; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "caller-returned"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	<-time.After(19 * time.Second)
	os.Exit(0)
}

func runManagedBackendActor(t *testing.T, dir string) {
	t.Helper()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	identity, err := processidentity.Capture(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(serviceActorIdentity{Process: identity})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backend.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-signals:
		if err := os.WriteFile(filepath.Join(dir, "backend-term"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	case <-time.After(19 * time.Second):
	}
	os.Exit(0)
}
