//go:build linux

package inventoryrepair

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestProcessInspectionRefusesLiveCheckoutAndOpenFile(t *testing.T) {
	for _, openFile := range []bool{false, true} {
		t.Run(strconv.FormatBool(openFile), func(t *testing.T) {
			f := newFixture(t)
			root := f.plan.Repairs[0].SourcePath
			cmd := exec.Command("sleep", "30")
			if openFile {
				file, err := os.Open(filepath.Join(root, "tracked"))
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = file.Close() }()
				cmd.ExtraFiles = []*os.File{file}
			} else {
				cmd.Dir = root
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
			if err := inspectProcess(filepath.Join("/proc", strconv.Itoa(cmd.Process.Pid)), []string{root}, f.plan); err == nil {
				t.Fatal("live process admitted")
			}
		})
	}
}

func TestUnknownProcessInspectionRefuses(t *testing.T) {
	if err := inspectProcess(t.TempDir(), []string{"/selected"}, Plan{}); err == nil {
		t.Fatal("unreadable process admitted")
	}
}

func TestProcessCensusRefusesConsumerWithDifferentHomeOwner(t *testing.T) {
	f := newFixture(t)
	cmd := exec.Command("sleep", "30")
	cmd.Dir = f.plan.Repairs[0].SourcePath
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	// Linux reserves this UID, so no host process can share this installation
	// owner. The live consumer must still block repair, as must unknown liveness
	// if the test user cannot inspect another host process.
	if err := inspectHostProcesses(context.Background(), f.plan, ^uint32(0)); err == nil {
		t.Fatal("different installation owner hid a live checkout consumer")
	}
}

func TestProcessInspectionRecognizesOnlyStatusFields(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		inactive     bool
	}{
		{"kernel thread", "Name:\tkworker\nState:\tS (sleeping)\nKthread:\t1\n", true},
		{"zombie", "Name:\tsleep\nState:\tZ (zombie)\n", true},
		{"misleading name", "Name:\tState:\tZ\nState:\tS (sleeping)\nKthread:\t0\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := t.TempDir()
			if err := os.WriteFile(filepath.Join(path, "status"), []byte(tc.status), 0600); err != nil {
				t.Fatal(err)
			}
			err := inspectProcess(path, []string{"/selected"}, Plan{})
			if (err == nil) != tc.inactive {
				t.Fatalf("inactive=%v, inspection error=%v", tc.inactive, err)
			}
		})
	}
}
