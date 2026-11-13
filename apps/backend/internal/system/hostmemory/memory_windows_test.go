//go:build windows

package hostmemory

import (
	"context"
	"testing"
)

func TestWindowsReaderReadsPhysicalAndCommit(t *testing.T) {
	reading, err := NewReader().Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reading.Complete {
		t.Fatal("Complete = false on windows")
	}
	if reading.TotalBytes == 0 {
		t.Fatal("TotalBytes = 0")
	}
	if reading.AvailableBytes > reading.TotalBytes {
		t.Fatalf("AvailableBytes %d exceeds TotalBytes %d", reading.AvailableBytes, reading.TotalBytes)
	}
	if !reading.CommitKnown {
		t.Skip("commit charge is unavailable on this host")
	}
	if reading.CommitLimitBytes == 0 {
		t.Fatal("CommitLimitBytes = 0")
	}
	if reading.CommitUsedBytes > reading.CommitLimitBytes {
		t.Fatalf("CommitUsedBytes %d exceeds CommitLimitBytes %d", reading.CommitUsedBytes, reading.CommitLimitBytes)
	}
}
