package codexdbg

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCodexdbgHelperProcess(t *testing.T) {
	if os.Getenv("KANDEV_CODEXDBG_TEST_PROCESS") != "1" {
		return
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestCaptureAndCleanup(t *testing.T) {
	capturePath := filepath.Join(t.TempDir(), "runner.jsonl")
	runner, err := StartRunner(context.Background(), RunnerConfig{
		Executable:        os.Args[0],
		Args:              []string{"-test.run=^TestCodexdbgHelperProcess$"},
		Environment:       []string{"KANDEV_CODEXDBG_TEST_PROCESS=1"},
		CapturePath:       capturePath,
		ExecutableVersion: "codex-cli test-helper",
		Operation:         "runner-test",
	})
	if err != nil {
		t.Fatalf("start runner: %v", err)
	}
	workdir := runner.Workdir()
	if info, err := os.Stat(workdir); err != nil || !info.IsDir() {
		t.Fatalf("temporary workdir = %q, stat error = %v", workdir, err)
	}

	if err := runner.Close("cancelled"); err != nil {
		t.Fatalf("close runner: %v", err)
	}
	select {
	case <-runner.processDone:
	case <-time.After(time.Second):
		t.Fatal("owned process did not exit")
	}
	if _, err := os.Stat(workdir); !os.IsNotExist(err) {
		t.Fatalf("temporary workdir remains: %v", err)
	}
	entries, err := ReadCapture(capturePath)
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	if entries[len(entries)-1].Event != captureEventClose || entries[len(entries)-1].Meta["reason"] != "cancelled" {
		t.Fatalf("capture close entry = %#v", entries[len(entries)-1])
	}
}
