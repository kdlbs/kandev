//go:build linux

package inventoryrepair

import (
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
