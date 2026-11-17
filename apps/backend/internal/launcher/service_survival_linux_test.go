//go:build linux

package launcher

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
	"golang.org/x/sys/unix"
)

const serviceActorEnv = "KANDEV_ISSUE4552_ACTOR"

type serviceActorIdentity struct {
	Process processidentity.Identity
	Cgroup  string
}

// @covers AC-EXECUTORS-SURVIVAL-001.11
// @covers AC-EXECUTORS-SURVIVAL-001.12
func TestSystemdServiceAgentLifetime(t *testing.T) {
	if os.Getenv("KANDEV_TEST_SYSTEMD_SURVIVAL") != "1" {
		t.Skip("set KANDEV_TEST_SYSTEMD_SURVIVAL=1 to exercise isolated user systemd units")
	}
	for _, binary := range []string{"systemctl", "systemd-run"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("%s unavailable: %v", binary, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "systemctl", "--user", "show", "--property=Version").CombinedOutput(); err != nil {
		t.Skipf("user systemd manager unavailable: %v: %s", err, out)
	}
	rendered := renderSystemdUnit(nativeServiceUnitInput{Executable: "/fixture/kandev", HomeDir: "/fixture/home"})
	mode := ""
	for _, line := range strings.Split(rendered, "\n") {
		if value, ok := strings.CutPrefix(line, "KillMode="); ok {
			mode = value
		}
	}
	if mode == "" {
		t.Fatal("generated service has no explicit KillMode")
	}
	for _, tc := range []struct {
		name, mode                string
		ordinary, survives, crash bool
	}{
		{"retained", mode, false, true, false},
		{"ordinary", mode, true, false, false},
		{"mixed-control", "mixed", false, false, false},
		{"crash-retained", mode, false, true, true},
		{"crash-ordinary", mode, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) { runServiceLifetimeCase(t, tc.name, tc.mode, tc.ordinary, tc.survives, tc.crash) })
	}
}

func runServiceLifetimeCase(t *testing.T, name, mode string, ordinary, survives, crash bool) {
	t.Helper()
	dir := t.TempDir()
	nonce := rand.Text()
	unit := "kandev-issue4552-" + nonce + "-" + name + ".service"
	description := "issue4552-owned-" + nonce
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var parent, backend, child serviceActorIdentity
	t.Cleanup(func() {
		if parent.Process.PID == 0 {
			parent = readServiceActor(filepath.Join(dir, "parent.json"))
		}
		if backend.Process.PID == 0 {
			backend = readServiceActor(filepath.Join(dir, "backend.json"))
		}
		if child.Process.PID == 0 {
			child = readServiceActor(filepath.Join(dir, "child.json"))
		}
		cleanupServiceActors(t, unit, description, parent, backend, child)
	})
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"--user", "--quiet", "--collect", "--unit=" + unit,
		"--property=Description=" + description, "--property=KillMode=" + mode,
		"--property=Type=simple", "--property=TimeoutStopSec=3s", "--property=RuntimeMaxSec=20s",
		"--setenv=" + serviceActorEnv + "=" + dir, "--setenv=KANDEV_SERVICE_MANAGER=" + nativeServiceManagerSystemd, binary, "-test.run=^TestSystemdServiceActor$", "--", "issue4552", "parent", dir, strconv.FormatBool(ordinary)}
	if out, err := exec.CommandContext(ctx, "systemd-run", args...).CombinedOutput(); err != nil {
		t.Fatalf("create %s: %v: %s", unit, err, out)
	}
	parent = awaitServiceActor(t, ctx, filepath.Join(dir, "parent.json"))
	backend = awaitServiceActor(t, ctx, filepath.Join(dir, "backend.json"))
	child = awaitServiceActor(t, ctx, filepath.Join(dir, "child.json"))
	properties := ownedServiceProperties(t, unit, description)
	if properties["MainPID"] != strconv.Itoa(parent.Process.PID) || properties["Id"] != unit {
		t.Fatalf("unit identity mismatch: %v parent=%+v", properties, parent)
	}
	if parent.Cgroup != backend.Cgroup || backend.Cgroup != child.Cgroup || !strings.Contains(child.Cgroup, "/"+unit) || child.Process.SessionID == backend.Process.SessionID || backend.Process.GroupID != backend.Process.PID {
		t.Fatalf("fixture must share service cgroup with a separate SID: parent=%+v backend=%+v child=%+v", parent, backend, child)
	}
	if state, err := processidentity.Inspect(child.Process); err != nil || state != processidentity.StateAlive {
		t.Fatalf("child not live before stop: %s %v", state, err)
	}
	t.Logf("unit=%s mode=%s crash=%t parent=%d birth=%s backend=%d birth=%s child=%d birth=%s cgroup=%s", unit, mode, crash, parent.Process.PID, parent.Process.BirthToken, backend.Process.PID, backend.Process.BirthToken, child.Process.PID, child.Process.BirthToken, child.Cgroup)
	if crash {
		killServiceActor(t, parent.Process)
	} else {
		serviceCommand(t, ctx, "stop", unit)
	}
	awaitServiceProcessExit(t, ctx, parent.Process)
	backendCtx, backendCancel := context.WithTimeout(ctx, 3*time.Second)
	defer backendCancel()
	awaitServiceProcessExit(t, backendCtx, backend.Process)
	if _, err := os.Stat(filepath.Join(dir, "backend-term")); err != nil {
		t.Fatalf("backend must receive SIGTERM: %v", err)
	}
	awaitServiceUnitStopped(t, ctx, unit, description)
	if survives {
		assertServiceProcessRetained(t, child.Process)
	} else {
		exitCtx, exitCancel := context.WithTimeout(ctx, 3*time.Second)
		defer exitCancel()
		awaitServiceProcessExit(t, exitCtx, child.Process)
	}
}

func ownedServiceProperties(t *testing.T, unit, description string) map[string]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", "--user", "show", unit, "--property=Id,Description,MainPID,ActiveState,LoadState").CombinedOutput()
	properties := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			properties[key] = value
		}
	}
	if properties["LoadState"] == "not-found" {
		return map[string]string{}
	}
	if err != nil || properties["Description"] != description || properties["Id"] != unit {
		t.Fatalf("cannot verify owned unit %s: %v: %s", unit, err, out)
	}
	return properties
}

func serviceCommand(t *testing.T, ctx context.Context, action, unit string) {
	t.Helper()
	if out, err := exec.CommandContext(ctx, "systemctl", "--user", action, unit).CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v: %s", action, unit, err, out)
	}
}

func awaitServiceActor(t *testing.T, ctx context.Context, path string) serviceActorIdentity {
	t.Helper()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		var actor serviceActorIdentity
		if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &actor) == nil && actor.Process.Validate() == nil {
			return actor
		}
		select {
		case <-ctx.Done():
			t.Fatalf("actor readiness %s: %v", path, ctx.Err())
		case <-tick.C:
		}
	}
}

func awaitServiceProcessExit(t *testing.T, ctx context.Context, identity processidentity.Identity) {
	t.Helper()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		state, err := processidentity.Inspect(identity)
		if err == nil && state == processidentity.StateExited {
			return
		}
		if err != nil || state == processidentity.StateReused {
			t.Fatalf("identity changed: %+v: %s %v", identity, state, err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("process did not exit: %+v: %v", identity, ctx.Err())
		case <-tick.C:
		}
	}
}

func assertServiceProcessRetained(t *testing.T, identity processidentity.Identity) {
	t.Helper()
	end := time.NewTimer(200 * time.Millisecond)
	defer end.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if state, err := processidentity.Inspect(identity); err != nil || state != processidentity.StateAlive {
			t.Fatalf("retained process exited: %+v: %s %v", identity, state, err)
		}
		select {
		case <-end.C:
			return
		case <-tick.C:
		}
	}
}

func cleanupServiceActors(t *testing.T, unit, description string, parent, backend, child serviceActorIdentity) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if properties := ownedServiceProperties(t, unit, description); properties["ActiveState"] == "active" || properties["ActiveState"] == "activating" {
		serviceCommand(t, ctx, "stop", unit)
	}
	for _, actor := range []serviceActorIdentity{child, backend, parent} {
		if actor.Process.PID == 0 {
			continue
		}
		state, err := processidentity.Inspect(actor.Process)
		if err != nil {
			t.Errorf("cleanup identity: %v", err)
			continue
		}
		if state == processidentity.StateAlive {
			killServiceActor(t, actor.Process)
			awaitServiceProcessExit(t, ctx, actor.Process)
		}
		if state, err := processidentity.Inspect(actor.Process); err != nil || state != processidentity.StateExited {
			t.Errorf("cleanup left actor: %+v: %s %v", actor.Process, state, err)
		}
	}
	if state := ownedServiceProperties(t, unit, description)["ActiveState"]; state != "" && state != "inactive" && state != "failed" {
		t.Errorf("cleanup left unit active: %s", state)
	}
	t.Logf("cleanup unit=%s actors=%d,%d,%d complete", unit, parent.Process.PID, backend.Process.PID, child.Process.PID)
}

func awaitServiceUnitStopped(t *testing.T, ctx context.Context, unit, description string) {
	t.Helper()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		state := ownedServiceProperties(t, unit, description)["ActiveState"]
		if state == "" || state == "inactive" || state == "failed" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("unit did not stop: %s", unit)
		case <-tick.C:
		}
	}
}

// TestSystemdServiceActor only runs as an explicitly selected fixture subprocess.
func TestSystemdServiceActor(t *testing.T) {
	if len(os.Args) != 7 || os.Args[2] != "--" || os.Args[3] != "issue4552" || os.Getenv(serviceActorEnv) != os.Args[5] {
		t.Skip("isolated service actor only")
	}
	role, dir := os.Args[4], os.Args[5]
	runtime.LockOSThread()
	signals := make(chan os.Signal, 1)
	if role == "parent" || role == managedBackendLabel {
		signal.Notify(signals, syscall.SIGTERM)
	}
	var backend *managedProcess
	switch role {
	case "parent":
		backend = startServiceBackend(t, dir, os.Args[6])
	case managedBackendLabel:
		startServiceChild(t, dir, os.Args[6])
	case "child":
	default:
		t.Fatal("invalid actor role")
	}
	recordServiceActor(t, dir, role)
	select {
	case <-signals:
		if role == managedBackendLabel {
			if err := os.WriteFile(filepath.Join(dir, "backend-term"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if backend != nil {
			result := backend.kill()
			if result.err != nil {
				t.Fatal(result.err)
			}
		}
	case <-time.After(19 * time.Second):
	}
	os.Exit(0)
}

func recordServiceActor(t *testing.T, dir, role string) {
	t.Helper()
	identity, err := processidentity.Capture(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	cgroup, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(serviceActorIdentity{Process: identity, Cgroup: string(cgroup)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, role+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func startServiceBackend(t *testing.T, dir, ordinary string) *managedProcess {
	t.Helper()
	env := upsertEnv(os.Environ(), backendPIDFileEnv, "")
	proc, _, err := startProcess(os.Args[0], []string{"-test.run=^TestSystemdServiceActor$", "--", "issue4552", managedBackendLabel, dir, ordinary}, "", env, true, managedBackendLabel, newSupervisor())
	if err != nil {
		t.Fatal(err)
	}
	return proc
}

func startServiceChild(t *testing.T, dir, ordinary string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestSystemdServiceActor$", "--", "issue4552", "child", dir, ordinary)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if ordinary == "true" {
		cmd.SysProcAttr.Pdeathsig = syscall.SIGTERM
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
}

func readServiceActor(path string) serviceActorIdentity {
	var actor serviceActorIdentity
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &actor)
	}
	return actor
}

func killServiceActor(t *testing.T, identity processidentity.Identity) {
	t.Helper()
	fd, err := unix.PidfdOpen(identity.PID, 0)
	if err != nil {
		t.Errorf("open owned actor pidfd: %v", err)
		return
	}
	defer func() { _ = unix.Close(fd) }()
	if state, err := processidentity.Inspect(identity); err != nil || state != processidentity.StateAlive {
		if err != nil || state != processidentity.StateExited {
			t.Errorf("cleanup identity changed: %s %v", state, err)
		}
		return
	}
	if err := unix.PidfdSendSignal(fd, syscall.SIGKILL, nil, 0); err != nil {
		t.Errorf("kill owned actor: %v", err)
	}
}
