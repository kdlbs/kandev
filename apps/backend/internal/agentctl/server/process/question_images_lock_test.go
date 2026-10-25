package process

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

var questionImageDecodeFixtureID atomic.Uint64

func TestQuestionImageDecodeDoesNotBlockWorkspaceUpdates(t *testing.T) {
	root := t.TempDir()
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	// A unique fixture signature gates the real decode boundary without affecting PNGs.
	magic := fmt.Sprintf("question-image-decode-%d;", questionImageDecodeFixtureID.Add(1))
	image.RegisterFormat("png", magic, func(io.Reader) (image.Image, error) {
		close(started)
		<-release
		return image.NewRGBA(image.Rect(0, 0, 2, 2)), nil
	}, func(io.Reader) (image.Config, error) {
		return image.Config{ColorModel: color.RGBAModel, Width: 2, Height: 2}, nil
	})
	require.NoError(t, os.WriteFile(filepath.Join(root, "screen.png"), []byte(magic), 0600))
	mgr := NewManager(&config.InstanceConfig{WorkDir: root}, logger.Default())
	done := make(chan error, 1)
	go func() {
		_, err := mgr.ReadQuestionImage(t.Context(), "screen.png", "")
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("image decode did not start")
	}
	available := mgr.repoTrackersMu.TryLock()
	if available {
		mgr.repoTrackersMu.Unlock()
	}
	unblock()
	require.NoError(t, <-done)
	require.True(t, available, "decoding must not hold the workspace lock")
}
