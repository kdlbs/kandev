package process

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/stretchr/testify/require"
)

type gatedStderrReader struct {
	started chan<- struct{}
	release <-chan struct{}
	data    []byte
	done    bool
}

func (r *gatedStderrReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	close(r.started)
	<-r.release
	r.done = true
	return copy(p, r.data), nil
}

func (r *gatedStderrReader) Close() error { return nil }

func TestWaitForExitWaitsForStderrReaderBeforePublishingError(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	cmd := exec.Command(os.Args[0], "-test.run=TestManagerProcessExitHelper")
	cmd.Env = append(os.Environ(), "KANDEV_MANAGER_PROCESS_EXIT_HELPER=1")
	cmd.Stderr = io.Discard

	m := &Manager{
		cmd:        cmd,
		stderr:     &gatedStderrReader{started: started, release: release, data: []byte("npm error code ETARGET\n")},
		stderrDone: make(chan struct{}),
		logger:     newTestLogger(t),
		doneCh:     make(chan struct{}),
		updatesCh:  make(chan adapter.AgentEvent, 1),
	}
	m.status.Store(StatusRunning)
	require.NoError(t, cmd.Start())
	m.wg.Add(2)
	go m.readStderr()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("stderr reader did not start")
	}

	waitDone := make(chan struct{})
	go func() {
		m.waitForExit()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		t.Fatal("waitForExit published the exit before stderr drained")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case <-waitDone:
	case <-time.After(time.Second):
		t.Fatal("waitForExit did not finish after stderr drained")
	}
	m.wg.Wait()

	event := <-m.updatesCh
	recent, ok := event.Data["recent_stderr"].([]string)
	if !ok {
		t.Fatalf("recent_stderr = %#v", event.Data["recent_stderr"])
	}
	require.Equal(t, []string{"npm error code ETARGET"}, recent)
}

func TestReadStderrPreservesSafeManagedNpmResolutionLines(t *testing.T) {
	m := &Manager{
		stderr: io.NopCloser(strings.NewReader(strings.Join([]string{
			"npm error code ETARGET",
			"npm error notarget No matching version found for opencode-ai@1.18.18",
			"npm error path /tmp/private-path",
		}, "\n"))),
		stderrSanitizer: stderrLineSanitizerFunc(func(string) (string, bool) {
			return "", false
		}),
		logger: newTestLogger(t),
	}
	m.wg.Add(1)
	m.readStderr()

	require.Equal(t, []string{
		"npm error code ETARGET",
		"npm error notarget No matching version found for opencode-ai@1.18.18",
	}, m.GetRecentStderr())
}
