//go:build linux

package oslimits

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// fakeProcBuilder builds a minimal fake /proc tree for testing.
type fakeProcBuilder struct {
	root string
	t    *testing.T
}

func newFakeProcBuilder(t *testing.T) *fakeProcBuilder {
	t.Helper()
	root := t.TempDir()
	return &fakeProcBuilder{root: root, t: t}
}

func (b *fakeProcBuilder) setSysctl(maxInstances, maxWatches uint64) {
	b.t.Helper()
	dir := filepath.Join(b.root, "sys/fs/inotify")
	mustMkdir(b.t, dir)
	mustWriteFile(b.t, filepath.Join(dir, "max_user_instances"), fmt.Sprintf("%d\n", maxInstances))
	mustWriteFile(b.t, filepath.Join(dir, "max_user_watches"), fmt.Sprintf("%d\n", maxWatches))
}

func (b *fakeProcBuilder) addPID(pid int, comm string, inotifyFDs int, watchesPerFD int) {
	b.t.Helper()
	pidDir := filepath.Join(b.root, strconv.Itoa(pid))
	fdDir := filepath.Join(pidDir, "fd")
	fdinfoDir := filepath.Join(pidDir, "fdinfo")
	mustMkdir(b.t, fdDir)
	mustMkdir(b.t, fdinfoDir)
	mustWriteFile(b.t, filepath.Join(pidDir, "comm"), comm+"\n")

	for i := range inotifyFDs {
		fdName := strconv.Itoa(i)
		// Create symlink pointing to anon_inode:inotify
		if err := os.Symlink("anon_inode:inotify", filepath.Join(fdDir, fdName)); err != nil {
			b.t.Fatalf("symlink inotify fd: %v", err)
		}
		// Create fdinfo with watchesPerFD inotify lines
		var content string
		content += "pos:\t0\nflags:\t02\nmnt_id:\t12\n"
		for w := range watchesPerFD {
			content += fmt.Sprintf("inotify wd:%d mask:fff ignored_mask:0 ino:abc sdev:1\n", w+1)
		}
		mustWriteFile(b.t, filepath.Join(fdinfoDir, fdName), content)
	}
}

func (b *fakeProcBuilder) addNonInotifyFD(pid int) {
	b.t.Helper()
	pidDir := filepath.Join(b.root, strconv.Itoa(pid))
	fdDir := filepath.Join(pidDir, "fd")
	mustMkdir(b.t, fdDir)
	mustMkdir(b.t, filepath.Join(pidDir, "fdinfo"))
	mustWriteFile(b.t, filepath.Join(pidDir, "comm"), "someproc\n")
	// socket symlink — not inotify
	if err := os.Symlink("socket:[12345]", filepath.Join(fdDir, "0")); err != nil {
		b.t.Fatalf("symlink socket fd: %v", err)
	}
	fdinfo := "pos:\t0\nflags:\t02\nmnt_id:\t12\n"
	mustWriteFile(b.t, filepath.Join(pidDir, "fdinfo", "0"), fdinfo)
}

func (b *fakeProcBuilder) probe() *InotifyProbe {
	return &InotifyProbe{procRoot: b.root}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// --- Tests ---

func TestInotifyProbe_ReadsMaxValues(t *testing.T) {
	fb := newFakeProcBuilder(t)
	fb.setSysctl(256, 65536)

	samples, err := fb.probe().Samples(context.Background())
	if err != nil {
		t.Fatalf("Samples() error: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}
	if samples[0].Limit != 256 {
		t.Errorf("instances limit = %d, want 256", samples[0].Limit)
	}
	if samples[1].Limit != 65536 {
		t.Errorf("watches limit = %d, want 65536", samples[1].Limit)
	}
}

func TestInotifyProbe_CountsInotifyFDs(t *testing.T) {
	fb := newFakeProcBuilder(t)
	fb.setSysctl(128, 8192)
	fb.addPID(100, "node", 3, 0)

	samples, err := fb.probe().Samples(context.Background())
	if err != nil {
		t.Fatalf("Samples() error: %v", err)
	}
	if samples[0].Used != 3 {
		t.Errorf("inotify instances used = %d, want 3", samples[0].Used)
	}
}

func TestInotifyProbe_SkipsNonInotifyFDs(t *testing.T) {
	fb := newFakeProcBuilder(t)
	fb.setSysctl(128, 8192)
	fb.addNonInotifyFD(200)

	samples, err := fb.probe().Samples(context.Background())
	if err != nil {
		t.Fatalf("Samples() error: %v", err)
	}
	if samples[0].Used != 0 {
		t.Errorf("expected 0 inotify instances (socket fds should be skipped), got %d", samples[0].Used)
	}
}

func TestInotifyProbe_CountsWatches(t *testing.T) {
	fb := newFakeProcBuilder(t)
	fb.setSysctl(128, 8192)
	// 2 fds, each with 3 watches = 6 total watches
	fb.addPID(300, "webpack", 2, 3)

	samples, err := fb.probe().Samples(context.Background())
	if err != nil {
		t.Fatalf("Samples() error: %v", err)
	}
	if samples[1].Used != 6 {
		t.Errorf("inotify watches used = %d, want 6", samples[1].Used)
	}
}

func TestInotifyProbe_SkipsOtherUID(t *testing.T) {
	fb := newFakeProcBuilder(t)
	fb.setSysctl(128, 8192)
	fb.addPID(400, "myproc", 2, 1)

	probe := fb.probe()
	// Scan with a UID that does not own the fake proc dirs (current uid + 1).
	// Since the fake dirs are owned by the test process, passing a different UID
	// should produce zero results.
	differentUID := uint32(os.Getuid() + 1)
	stats, err := probe.scanProc(differentUID)
	if err != nil {
		t.Fatalf("scanProc error: %v", err)
	}
	if stats.totalInstances != 0 {
		t.Errorf("expected 0 instances for different UID, got %d", stats.totalInstances)
	}
}

func TestInotifyProbe_HandlesDisappearingProcess(t *testing.T) {
	fb := newFakeProcBuilder(t)
	fb.setSysctl(128, 8192)
	fb.addPID(500, "gone", 1, 0)

	// Remove the fd dir to simulate the process disappearing mid-scan.
	if err := os.RemoveAll(filepath.Join(fb.root, "500", "fd")); err != nil {
		t.Fatalf("remove fd dir: %v", err)
	}

	// Must not panic or return an error.
	samples, err := fb.probe().Samples(context.Background())
	if err != nil {
		t.Fatalf("Samples() error: %v", err)
	}
	if samples[0].Used != 0 {
		t.Errorf("expected 0 instances after process disappears, got %d", samples[0].Used)
	}
}

func TestInotifyProbe_TopNConsumers(t *testing.T) {
	fb := newFakeProcBuilder(t)
	fb.setSysctl(128, 8192)
	// Add 7 PIDs — only top 5 should be returned.
	for i := range 7 {
		fb.addPID(600+i, fmt.Sprintf("proc%d", i), i+1, 0)
	}

	samples, err := fb.probe().Samples(context.Background())
	if err != nil {
		t.Fatalf("Samples() error: %v", err)
	}
	if len(samples[0].TopConsumers) != maxTopConsumers {
		t.Errorf("expected %d top consumers, got %d", maxTopConsumers, len(samples[0].TopConsumers))
	}
}

func TestInotifyProbe_SortedByFDCount(t *testing.T) {
	fb := newFakeProcBuilder(t)
	fb.setSysctl(128, 8192)
	// Intentionally add in ascending order.
	fb.addPID(700, "low", 1, 0)
	fb.addPID(701, "high", 4, 0)
	fb.addPID(702, "mid", 2, 0)

	samples, err := fb.probe().Samples(context.Background())
	if err != nil {
		t.Fatalf("Samples() error: %v", err)
	}
	consumers := samples[0].TopConsumers
	if len(consumers) < 2 {
		t.Fatalf("expected at least 2 consumers, got %d", len(consumers))
	}
	if consumers[0].FDCount < consumers[1].FDCount {
		t.Errorf("consumers not sorted descending: [0].FDCount=%d < [1].FDCount=%d",
			consumers[0].FDCount, consumers[1].FDCount)
	}
}
