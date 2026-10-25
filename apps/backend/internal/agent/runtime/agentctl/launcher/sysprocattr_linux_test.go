//go:build linux

package launcher

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
	"golang.org/x/sys/unix"
	"testing"
)

// TestBuildSysProcAttrOmitsPdeathsigWhenSurvivalEnabled pins kill-path #2
// from design 01's kill-paths list ("Pdeathsig"): AC-EXECUTORS-SURVIVAL-001.2
// requires agentctl to keep running when the backend exits without a
// graceful shutdown, including SIGKILL, "without depending on any backend
// shutdown step" -- Pdeathsig fires from the kernel on ANY parent exit, so it
// must not be armed at all when the capability is engaged for this launch.
func TestBuildSysProcAttrOmitsPdeathsigWhenSurvivalEnabled(t *testing.T) {
	attr := buildSysProcAttr(true)
	if attr.Pdeathsig != 0 {
		t.Fatalf("Pdeathsig = %v, want unset (0) when the capability is engaged", attr.Pdeathsig)
	}
	if !attr.Setsid {
		t.Error("Setsid must stay true regardless of the capability: it isolates the owned runtime process tree")
	}
}

// TestBuildSysProcAttrSetsPdeathsigWhenSurvivalDisabled is the regression
// companion: today's behavior is unchanged when the capability is off.
func TestBuildSysProcAttrSetsPdeathsigWhenSurvivalDisabled(t *testing.T) {
	attr := buildSysProcAttr(false)
	if attr.Pdeathsig != syscall.SIGTERM {
		t.Fatalf("Pdeathsig = %v, want SIGTERM when the capability is disabled", attr.Pdeathsig)
	}
}

// @covers AC-EXECUTORS-SURVIVAL-001.12
func TestBuildSysProcAttrParentDeath(t *testing.T) {
	for _, survival := range []bool{false, true} {
		t.Run(strconv.FormatBool(survival), func(t *testing.T) {
			dir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestParentDeathActor$", "--", "pdeath-fixture", "parent", dir, strconv.FormatBool(survival))
			cmd.Env = append(os.Environ(), "KANDEV_PDEATH_FIXTURE="+dir)
			var child processidentity.Identity
			joined := false
			t.Cleanup(func() {
				if !joined && cmd.Process != nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
				if child.PID != 0 {
					stopParentDeathChild(t, child)
				}
			})
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for child.PID == 0 && time.Now().Before(deadline) {
				if data, err := os.ReadFile(filepath.Join(dir, "child.json")); err == nil {
					_ = json.Unmarshal(data, &child)
				}
				if child.PID == 0 {
					time.Sleep(10 * time.Millisecond)
				}
			}
			if err := child.Validate(); err != nil {
				t.Fatalf("child readiness: %v", err)
			}
			if state, err := processidentity.Inspect(child); err != nil || state != processidentity.StateAlive {
				t.Fatalf("child not live: %s %v", state, err)
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			err := cmd.Wait()
			joined = true
			if err == nil {
				t.Fatal("parent must have been killed")
			}
			until := time.Now().Add(200 * time.Millisecond)
			if !survival {
				until = time.Now().Add(3 * time.Second)
			}
			for {
				state, err := processidentity.Inspect(child)
				if err != nil {
					t.Fatal(err)
				}
				if !survival && state == processidentity.StateExited {
					break
				}
				if survival && state != processidentity.StateAlive {
					t.Fatalf("surviving child state: %s", state)
				}
				if time.Now().After(until) {
					if !survival {
						t.Fatalf("parent-death guard left child alive: %+v", child)
					}
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

func TestParentDeathActor(t *testing.T) {
	if len(os.Args) != 7 || os.Args[2] != "--" || os.Args[3] != "pdeath-fixture" || os.Getenv("KANDEV_PDEATH_FIXTURE") != os.Args[5] {
		t.Skip("isolated parent-death actor only")
	}
	role, dir := os.Args[4], os.Args[5]
	runtime.LockOSThread()
	switch role {
	case "parent":
		survival, err := strconv.ParseBool(os.Args[6])
		if err != nil {
			t.Fatal(err)
		}
		child := exec.Command(os.Args[0], "-test.run=^TestParentDeathActor$", "--", "pdeath-fixture", "child", dir, os.Args[6])
		child.Env = os.Environ()
		child.SysProcAttr = buildSysProcAttr(survival)
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		go func() { _ = child.Wait() }()
	case "child":
		identity, err := processidentity.Capture(os.Getpid())
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(identity)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "child.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("invalid actor role")
	}
	<-time.After(20 * time.Second)
	os.Exit(0)
}

func stopParentDeathChild(t *testing.T, identity processidentity.Identity) {
	t.Helper()
	state, err := processidentity.Inspect(identity)
	if err != nil {
		t.Error(err)
		return
	}
	if state == processidentity.StateExited {
		return
	}
	fd, err := unix.PidfdOpen(identity.PID, 0)
	if err != nil {
		t.Error(err)
		return
	}
	defer func() { _ = unix.Close(fd) }()
	if state, err := processidentity.Inspect(identity); err != nil || state != processidentity.StateAlive {
		t.Errorf("child identity changed: %s %v", state, err)
		return
	}
	if err := unix.PidfdSendSignal(fd, syscall.SIGKILL, nil, 0); err != nil {
		t.Error(err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		if state, err := processidentity.Inspect(identity); err == nil && state == processidentity.StateExited {
			return
		}
		select {
		case <-ctx.Done():
			t.Error("fixture child did not exit")
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
}
