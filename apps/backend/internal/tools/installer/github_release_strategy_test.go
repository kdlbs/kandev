package installer

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type cancelingAssetReader struct {
	cancel context.CancelFunc
	sent   bool
}

func (r *cancelingAssetReader) Read(data []byte) (int, error) {
	if r.sent {
		return 0, io.EOF
	}
	r.sent = true
	copy(data, "partial-binary")
	r.cancel()
	return len("partial-binary"), nil
}

func TestWriteReleaseAssetDoesNotPublishCanceledDownload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	outPath := filepath.Join(t.TempDir(), "rust-analyzer")

	err := writeReleaseAsset(ctx, &cancelingAssetReader{cancel: cancel}, "rust-analyzer", outPath)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("writeReleaseAsset() error = %v, want context cancellation", err)
	}
	if _, err := os.Stat(outPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("published output stat error = %v, want not-exist", err)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(outPath), ".rust-analyzer.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary assets remain after cancellation: %v", matches)
	}
}
