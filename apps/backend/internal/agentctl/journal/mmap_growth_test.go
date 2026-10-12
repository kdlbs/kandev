package journal

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	systemmetrics "github.com/kandev/kandev/internal/system/metrics"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4
func TestCapacityAccountingDuringMmapGrowth(t *testing.T) {
	if os.Getenv("KANDEV_JOURNAL_MMAP_CHILD") != "1" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestCapacityAccountingDuringMmapGrowth$")
		cmd.Env = append(os.Environ(), "KANDEV_JOURNAL_MMAP_CHILD=1", "GORACE=halt_on_error=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("concurrent journal growth: %v\n%s", err, output)
		}
		return
	}
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{Path: path, DiskCapacityCacheTTL: time.Hour,
		DiskCapacityReader: func(context.Context, string) (systemmetrics.DiskCapacity, error) {
			return systemmetrics.DiskCapacity{TotalBytes: 1 << 40, AvailableBytes: 1 << 40}, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		for ctx.Err() == nil {
			if _, err := j.Capacity(ctx, "stream"); err != nil && ctx.Err() == nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	<-started
	for index := 0; index < 1000; index++ {
		event, err := j.Append(ctx, Event{SessionID: "session", IncarnationID: "incarnation",
			HarnessGeneration: 1, StreamID: "stream", Type: "message", Payload: make([]byte, 32<<10)})
		if err != nil {
			t.Fatal(err)
		}
		if event.Sequence != uint64(index+1) {
			t.Fatalf("committed sequence = %d, want %d", event.Sequence, index+1)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 32<<20 {
		t.Fatalf("journal size = %d; mapping did not grow", info.Size())
	}
}
