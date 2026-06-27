package launcher

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLimitedBufferKeepsOnlyTail(t *testing.T) {
	buf := newLimitedBuffer(5)
	if _, err := buf.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if _, err := buf.Write([]byte("world")); err != nil {
		t.Fatal(err)
	}

	if got := string(buf.Bytes()); got != "world" {
		t.Fatalf("buffer tail = %q, want world", got)
	}
}

func TestLimitedBufferBytesReturnsSnapshot(t *testing.T) {
	buf := newLimitedBuffer(10)
	if _, err := buf.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	snapshot := buf.Bytes()
	snapshot[0] = 'x'

	if !bytes.Equal(buf.Bytes(), []byte("abc")) {
		t.Fatalf("mutating snapshot changed buffer: %q", buf.Bytes())
	}
}

func TestProcessOutputKeepsDrainingAfterSinkBreaks(t *testing.T) {
	var output bytes.Buffer
	oldStatusOutput := launcherStatusOutput
	launcherStatusOutput = &output
	t.Cleanup(func() {
		launcherStatusOutput = oldStatusOutput
	})

	sink := &failingWriter{err: errors.New("broken pipe")}
	buf := newLimitedBuffer(20)
	out := newProcessOutput(buf, sink, nil, "test")

	if n, err := out.Write([]byte("first")); err != nil || n != len("first") {
		t.Fatalf("first Write() = (%d, %v), want (%d, nil)", n, err, len("first"))
	}
	if n, err := out.Write([]byte("second")); err != nil || n != len("second") {
		t.Fatalf("second Write() = (%d, %v), want (%d, nil)", n, err, len("second"))
	}
	if sink.calls != 1 {
		t.Fatalf("sink calls = %d, want 1", sink.calls)
	}
	if got := string(buf.Bytes()); got != "firstsecond" {
		t.Fatalf("buffer = %q, want firstsecond", got)
	}
	if got := output.String(); !strings.Contains(got, "warning: disabling test output sink") {
		t.Fatalf("status output did not warn about disabled sink: %q", got)
	}
}

func TestProcessOutputFallsBackAfterSinkBreaks(t *testing.T) {
	sink := &failingWriter{err: errors.New("broken pipe")}
	var fallback bytes.Buffer
	out := newProcessOutput(nil, sink, &fallback, "test")

	if n, err := out.Write([]byte("first")); err != nil || n != len("first") {
		t.Fatalf("first Write() = (%d, %v), want (%d, nil)", n, err, len("first"))
	}
	if n, err := out.Write([]byte("second")); err != nil || n != len("second") {
		t.Fatalf("second Write() = (%d, %v), want (%d, nil)", n, err, len("second"))
	}

	if sink.calls != 1 {
		t.Fatalf("sink calls = %d, want 1", sink.calls)
	}
	if got := fallback.String(); got != "firstsecond" {
		t.Fatalf("fallback output = %q, want firstsecond", got)
	}
}

func TestSummarizeShutdownCountsGracefulForceKilledAndFailed(t *testing.T) {
	errStop := errors.New("stop failed")
	got := summarizeShutdown([]managedProcessShutdownResult{
		{graceful: true},
		{forceKilled: true},
		{forceKilled: true, err: errStop},
	})

	if got.graceful != 1 {
		t.Fatalf("graceful = %d, want 1", got.graceful)
	}
	if got.forceKilled != 2 {
		t.Fatalf("forceKilled = %d, want 2", got.forceKilled)
	}
	if got.failed != 1 {
		t.Fatalf("failed = %d, want 1", got.failed)
	}
}

func TestSupervisorShutdownRunsOnce(t *testing.T) {
	var output bytes.Buffer
	oldStatusOutput := launcherStatusOutput
	launcherStatusOutput = &output
	t.Cleanup(func() {
		launcherStatusOutput = oldStatusOutput
	})

	supervisor := newSupervisor()
	supervisor.shutdown("signal interrupt")
	supervisor.shutdown("backend exit")

	got := output.String()
	if count := strings.Count(got, "graceful shutdown started"); count != 1 {
		t.Fatalf("shutdown start log count = %d, want 1; output:\n%s", count, got)
	}
	if count := strings.Count(got, "graceful shutdown complete"); count != 1 {
		t.Fatalf("shutdown complete log count = %d, want 1; output:\n%s", count, got)
	}
	if strings.Contains(got, "backend exit") {
		t.Fatalf("second shutdown reason was logged; output:\n%s", got)
	}
}

func TestWaitForManagedProcessKillDoneReturnsOnClose(t *testing.T) {
	done := make(chan struct{})
	close(done)

	if !waitForManagedProcessKillDone(done, time.Second) {
		t.Fatal("expected closed done channel to return true")
	}
}

func TestWaitForManagedProcessKillDoneTimesOut(t *testing.T) {
	done := make(chan struct{})
	start := time.Now()

	if waitForManagedProcessKillDone(done, 10*time.Millisecond) {
		t.Fatal("expected open done channel to time out")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout wait took too long: %s", elapsed)
	}
}

type failingWriter struct {
	calls int
	err   error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.calls++
	return 0, w.err
}
