package runtime

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type blockingKillProcess struct {
	fakeSpawnedProcess
	entered chan struct{}
	release chan struct{}
}

func (p *blockingKillProcess) Kill() {
	close(p.entered)
	<-p.release
}

func TestRemoteLookupReturnsWhileFailedProcessKillIsBlocked(t *testing.T) {
	blocked := &blockingKillProcess{entered: make(chan struct{}), release: make(chan struct{})}
	p := newTestProcess(t, func() (spawnedProcess, error) { return nil, errors.New("spawn failed") }, &statusRecorder{})
	p.current = blocked
	done := make(chan struct{})
	go func() { p.handleFailureAndRestart(errors.New("failed")); close(done) }()
	t.Cleanup(func() { close(blocked.release); <-done })
	<-blocked.entered
	available := make(chan bool, 1)
	go func() { _, ok := p.remote(); available <- ok }()
	select {
	case ok := <-available:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("runtime lookup waited for process shutdown")
	}
}
