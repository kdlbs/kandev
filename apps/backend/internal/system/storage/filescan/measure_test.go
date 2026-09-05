package filescan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestMeasureUsesBoundedConcurrentPartitionsAndIndexedResults(t *testing.T) {
	root := t.TempDir()
	for index, contents := range []string{"one", "two", "three", "four", "five"} {
		partition := filepath.Join(root, string(rune('a'+index)))
		if err := os.Mkdir(partition, 0o700); err != nil {
			t.Fatalf("Mkdir(%s): %v", partition, err)
		}
		if err := os.WriteFile(filepath.Join(partition, "data"), []byte(contents), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", partition, err)
		}
	}

	var active atomic.Int32
	var maxActive atomic.Int32
	var started atomic.Int32
	release := make(chan struct{})
	var releaseOnce atomic.Bool
	progress := func(event Progress) {
		if event.Phase == PartitionStarted {
			current := active.Add(1)
			for {
				previous := maxActive.Load()
				if current <= previous || maxActive.CompareAndSwap(previous, current) {
					break
				}
			}
			if started.Add(1) == 2 && releaseOnce.CompareAndSwap(false, true) {
				close(release)
			}
			<-release
		}
		if event.Phase == PartitionCompleted {
			active.Add(-1)
		}
	}

	results := NewLimiter(2).Measure(context.Background(), []Root{{Path: root}}, progress)
	if len(results) != 1 {
		t.Fatalf("result count = %d, want one root result", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("root measurement error = %v", results[0].Err)
	}
	if results[0].Bytes != int64(len("one"+"two"+"three"+"four"+"five")) {
		t.Fatalf("root bytes = %d, want all partition bytes", results[0].Bytes)
	}
	if maxActive.Load() != 2 {
		t.Fatalf("max active partitions = %d, want limiter ceiling 2", maxActive.Load())
	}
}

func TestMeasurePreservesRootOptionsAndProgressOrder(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "included"), []byte("included"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "excluded"), []byte("excluded"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "included"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	var events []Progress
	results := NewLimiter(1).Measure(context.Background(), []Root{{
		Path:          root,
		SymlinkPolicy: SkipSymlinks,
		Exclude: func(path string, _ os.DirEntry) bool {
			return filepath.Base(path) == "excluded"
		},
	}}, func(event Progress) {
		events = append(events, event)
	})
	if len(results) != 1 {
		t.Fatalf("result count = %d, want one root result", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("root measurement error = %v", results[0].Err)
	}
	if results[0].Bytes != int64(len("included")) {
		t.Fatalf("root bytes = %d, want included file only", results[0].Bytes)
	}
	if len(events) == 0 || events[len(events)-1].Phase != RootCompleted {
		t.Fatalf("last progress event = %#v, want root completion", events)
	}
	if events[len(events)-1].CompletedRoots != 1 || events[len(events)-1].TotalRoots != 1 {
		t.Fatalf("root progress = %#v, want 1/1", events[len(events)-1])
	}
}

func TestMeasureRejectsSymlinkAndSupportsMissingRoots(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	results := NewLimiter(1).Measure(context.Background(), []Root{{
		Path:          root,
		SymlinkPolicy: RejectSymlinks,
	}}, nil)
	if len(results) != 1 {
		t.Fatalf("result count = %d, want one root result", len(results))
	}
	if results[0].Err == nil {
		t.Fatal("symlink measurement succeeded, want an error")
	}

	missing := NewLimiter(1).Measure(context.Background(), []Root{{Path: filepath.Join(root, "missing"), MissingOK: true}}, nil)
	if len(missing) != 1 {
		t.Fatalf("missing result count = %d, want one root result", len(missing))
	}
	if missing[0].Err != nil || missing[0].Bytes != 0 {
		t.Fatalf("missing root result = %#v, want empty successful result", missing[0])
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	results = NewLimiter(1).Measure(cancelled, []Root{{Path: root}}, nil)
	if len(results) != 1 {
		t.Fatalf("cancelled result count = %d, want one root result", len(results))
	}
	if !errors.Is(results[0].Err, context.Canceled) {
		t.Fatalf("cancelled measurement error = %v, want context.Canceled", results[0].Err)
	}
}

func BenchmarkMeasureTrees(b *testing.B) {
	root := b.TempDir()
	for index := 0; index < 8; index++ {
		partition := filepath.Join(root, string(rune('a'+index)))
		if err := os.Mkdir(partition, 0o700); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(partition, "data"), []byte("benchmark"), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	roots := []Root{{Path: root}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := NewLimiter(4).Measure(context.Background(), roots, nil)
		if results[0].Err != nil {
			b.Fatal(results[0].Err)
		}
	}
}
