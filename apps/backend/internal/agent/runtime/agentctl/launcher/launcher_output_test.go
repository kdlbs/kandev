package launcher

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestPipeOutputOversizedRecordContinuesDraining(t *testing.T) {
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			core, observed := observer.New(zapcore.DebugLevel)
			parentLog, err := logger.NewFromZap(zap.New(core))
			require.NoError(t, err)
			launcher := &Launcher{logger: parentLog}
			reader, writer, err := os.Pipe()
			require.NoError(t, err)

			readerDone := make(chan struct{})
			writeDone := make(chan struct{})
			var writeErr error
			t.Cleanup(func() {
				_ = writer.Close()
				_ = reader.Close()
				waitForPipeOutputTests(t, writeDone, readerDone)
			})

			go func() {
				launcher.pipeOutput(stream, reader)
				close(readerDone)
			}()

			const normalLine = "2026-09-22T12:00:00.000Z\tINFO\tlauncher_test.go:1\tcontinued after oversized record"
			payload := strings.Repeat("x", 128*1024) + "\n" + normalLine + "\n" + strings.Repeat("y", 128*1024) + "\n"
			go func() {
				_, writeErr = io.WriteString(writer, payload)
				close(writeDone)
			}()

			select {
			case <-writeDone:
				require.NoError(t, writeErr)
			case <-time.After(3 * time.Second):
				t.Fatal("child writer blocked behind an oversized record")
			}

			select {
			case <-readerDone:
				t.Fatal("reader stopped before the child closed the stream")
			default:
			}

			found := false
			for _, entry := range observed.All() {
				if entry.Message == normalLine && entry.Level == zapcore.InfoLevel && entry.ContextMap()["stream"] == stream {
					found = true
					break
				}
			}
			require.True(t, found, "the ordinary record after the oversized record must be forwarded")

			require.NoError(t, writer.Close())
			select {
			case <-readerDone:
			case <-time.After(3 * time.Second):
				t.Fatal("reader did not stop after EOF")
			}
		})
	}
}

func TestPipeOutputRecordBoundaries(t *testing.T) {
	const recordLimit = 64 * 1024
	const normalLine = "2026-09-22T12:00:00.000Z\tINFO\tlauncher_test.go:1\tboundary continuation"
	tests := []struct {
		name         string
		input        string
		want         []string
		wantOversize bool
	}{
		{name: "limit minus one", input: strings.Repeat("a", recordLimit-1) + "\n" + normalLine, want: []string{strings.Repeat("a", recordLimit-1), normalLine}},
		{name: "exact limit", input: strings.Repeat("b", recordLimit) + "\n" + normalLine, want: []string{strings.Repeat("b", recordLimit), normalLine}},
		{name: "limit plus one", input: strings.Repeat("c", recordLimit+1) + "\n" + normalLine, want: []string{normalLine}, wantOversize: true},
		{name: "CRLF", input: "record\r\n", want: []string{"record"}},
		{name: "blank line", input: "\n", want: []string{""}},
		{name: "final record without LF", input: "final record", want: []string{"final record"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core, observed := observer.New(zapcore.DebugLevel)
			parentLog, err := logger.NewFromZap(zap.New(core))
			require.NoError(t, err)
			(&Launcher{logger: parentLog}).pipeOutput("stdout", strings.NewReader(test.input))

			var got []string
			oversizeWarnings := 0
			for _, entry := range observed.All() {
				if entry.Message == "discarded oversized agentctl log record" {
					oversizeWarnings++
					continue
				}
				got = append(got, entry.Message)
			}
			require.Len(t, got, len(test.want))
			for i := range test.want {
				if got[i] != test.want[i] {
					t.Fatalf("record %d differs: got length %d, want length %d", i, len(got[i]), len(test.want[i]))
				}
			}
			if test.wantOversize {
				require.Equal(t, 1, oversizeWarnings)
			} else {
				require.Zero(t, oversizeWarnings)
			}
		})
	}
}

func TestPipeOutputOversizedWithoutNewline(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	parentLog, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	(&Launcher{logger: parentLog}).pipeOutput("stdout", &repeatingReader{remaining: 2 * 1024 * 1024})

	entries := observed.All()
	require.Len(t, entries, 1)
	require.Equal(t, "discarded oversized agentctl log record", entries[0].Message)
	require.Equal(t, "stdout", entries[0].ContextMap()["stream"])
	require.Equal(t, int64(64*1024), entries[0].ContextMap()["limit_bytes"])
	require.NotContains(t, entries[0].Message, "xxxxx")
}

func TestPipeOutputDiagnosticsAreBounded(t *testing.T) {
	const normalLine = "2026-09-22T12:00:00.000Z\tINFO\tlauncher_test.go:1\tafter repeated oversized records"
	input := strings.Repeat(strings.Repeat("private-record-content", 6000)+"\n", 3) + normalLine + "\n"
	core, observed := observer.New(zapcore.DebugLevel)
	parentLog, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	(&Launcher{logger: parentLog}).pipeOutput("stderr", strings.NewReader(input))

	warnings := 0
	foundNormal := false
	for _, entry := range observed.All() {
		if entry.Message == "discarded oversized agentctl log record" {
			warnings++
			require.Equal(t, "stderr", entry.ContextMap()["stream"])
			require.Equal(t, int64(64*1024), entry.ContextMap()["limit_bytes"])
			require.NotContains(t, entry.Message, "private-record-content")
		}
		if entry.Message == normalLine && entry.ContextMap()["stream"] == "stderr" {
			foundNormal = true
		}
		require.NotContains(t, entry.Message, "private-record-content")
	}
	require.Equal(t, 1, warnings)
	require.True(t, foundNormal, "a bounded diagnostic burst must not stop later records")
}

func TestPipeOutputReadFailure(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantClass string
	}{
		{name: "read failure", err: errors.New("private read failure text"), wantClass: "read_failure"},
		{name: "unexpected EOF", err: io.ErrUnexpectedEOF, wantClass: "unexpected_eof"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core, observed := observer.New(zapcore.DebugLevel)
			parentLog, err := logger.NewFromZap(zap.New(core))
			require.NoError(t, err)
			input := &failingReader{payload: []byte("partial private record"), err: test.err}
			(&Launcher{logger: parentLog}).pipeOutput("stderr", input)

			entries := observed.All()
			require.Len(t, entries, 1)
			require.Equal(t, "agentctl log stream read failed", entries[0].Message)
			require.Equal(t, map[string]any{"stream": "stderr", "error_class": test.wantClass}, entries[0].ContextMap())
			require.NotContains(t, entries[0].Message, "private")
		})
	}
}

func TestPipeOutputEOFAndClosure(t *testing.T) {
	t.Run("final record at EOF", func(t *testing.T) {
		core, observed := observer.New(zapcore.DebugLevel)
		parentLog, err := logger.NewFromZap(zap.New(core))
		require.NoError(t, err)
		reader, writer := io.Pipe()
		readerDone := make(chan struct{})
		t.Cleanup(func() {
			_ = writer.Close()
			_ = reader.Close()
			waitForPipeOutputTests(t, readerDone)
		})
		go func() {
			(&Launcher{logger: parentLog}).pipeOutput("stdout", reader)
			close(readerDone)
		}()

		_, err = io.WriteString(writer, "final record without a delimiter")
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		waitForPipeOutputTests(t, readerDone)
		require.Len(t, observed.All(), 1)
		require.Equal(t, "final record without a delimiter", observed.All()[0].Message)
	})

	t.Run("closure while discarding", func(t *testing.T) {
		core, observed := observer.New(zapcore.DebugLevel)
		parentLog, err := logger.NewFromZap(zap.New(core))
		require.NoError(t, err)
		reader, writer := io.Pipe()
		readerDone := make(chan struct{})
		writeDone := make(chan struct{})
		var writeErr error
		t.Cleanup(func() {
			_ = writer.Close()
			_ = reader.Close()
			waitForPipeOutputTests(t, writeDone, readerDone)
		})
		go func() {
			(&Launcher{logger: parentLog}).pipeOutput("stderr", reader)
			close(readerDone)
		}()
		go func() {
			_, writeErr = io.WriteString(writer, strings.Repeat("x", 128*1024))
			close(writeDone)
		}()

		select {
		case <-writeDone:
			require.NoError(t, writeErr)
		case <-time.After(3 * time.Second):
			t.Fatal("writer blocked while the oversized record was being discarded")
		}
		require.NoError(t, writer.Close())
		waitForPipeOutputTests(t, readerDone)
		entries := observed.All()
		require.Len(t, entries, 1)
		require.Equal(t, "discarded oversized agentctl log record", entries[0].Message)
	})

	t.Run("supervised descriptor close", func(t *testing.T) {
		core, observed := observer.New(zapcore.DebugLevel)
		parentLog, err := logger.NewFromZap(zap.New(core))
		require.NoError(t, err)
		reader, writer, err := os.Pipe()
		require.NoError(t, err)
		readerDone := make(chan struct{})
		t.Cleanup(func() {
			_ = reader.Close()
			_ = writer.Close()
			waitForPipeOutputTests(t, readerDone)
		})
		go func() {
			(&Launcher{logger: parentLog}).pipeOutput("stdout", reader)
			close(readerDone)
		}()

		require.NoError(t, reader.Close())
		waitForPipeOutputTests(t, readerDone)
		require.Empty(t, observed.All())
	})
}

func TestPipeOutputOversizedRecordDoesNotBlockChild(t *testing.T) {
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			core, observed := observer.New(zapcore.DebugLevel)
			parentLog, err := logger.NewFromZap(zap.New(core))
			require.NoError(t, err)
			launcher := &Launcher{logger: parentLog}

			cmd := exec.Command(os.Args[0], "-test.run=^TestPipeOutputSubprocessHelper$")
			cmd.Env = append(os.Environ(), "KANDEV_PIPE_OUTPUT_HELPER=1", "KANDEV_PIPE_OUTPUT_STREAM="+stream)
			stdin, err := cmd.StdinPipe()
			require.NoError(t, err)
			t.Cleanup(func() { _ = stdin.Close() })
			stdout, err := cmd.StdoutPipe()
			require.NoError(t, err)
			t.Cleanup(func() { _ = stdout.Close() })
			stderr, err := cmd.StderrPipe()
			require.NoError(t, err)
			t.Cleanup(func() { _ = stderr.Close() })
			require.NoError(t, cmd.Start())

			stdoutDone := make(chan struct{})
			stderrDone := make(chan struct{})
			readersStarted := false
			waited := false
			t.Cleanup(func() {
				if !waited {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					waited = true
				}
				if readersStarted {
					waitForPipeOutputTests(t, stdoutDone, stderrDone)
				}
			})

			readersStarted = true
			go func() {
				launcher.pipeOutput("stdout", stdout)
				close(stdoutDone)
			}()
			go func() {
				launcher.pipeOutput("stderr", stderr)
				close(stderrDone)
			}()

			const normalLine = "2026-09-22T12:00:00.000Z\tINFO\tlauncher_test.go:1\tchild continued after oversized output"
			require.Eventually(t, func() bool {
				for _, entry := range observed.All() {
					if entry.Message == normalLine && entry.ContextMap()["stream"] == stream {
						return true
					}
				}
				return false
			}, 3*time.Second, 10*time.Millisecond, "normal child output must follow the oversized record")
			_, err = stdin.Write([]byte{1})
			require.NoError(t, err)
			require.NoError(t, stdin.Close())
			waitForPipeOutputTests(t, stdoutDone, stderrDone)
			waitErr := cmd.Wait()
			waited = true
			require.NoError(t, waitErr)
		})
	}
}

func TestPipeOutputSubprocessHelper(t *testing.T) {
	if os.Getenv("KANDEV_PIPE_OUTPUT_HELPER") != "1" {
		return
	}
	stream := os.Getenv("KANDEV_PIPE_OUTPUT_STREAM")
	output := os.Stdout
	if stream == "stderr" {
		output = os.Stderr
	}
	const normalLine = "2026-09-22T12:00:00.000Z\tINFO\tlauncher_test.go:1\tchild continued after oversized output\n"
	if _, err := io.WriteString(output, strings.Repeat("x", 128*1024)+"\n"+normalLine); err != nil {
		t.Fatalf("write child output: %v", err)
	}
	var ack [1]byte
	if _, err := io.ReadFull(os.Stdin, ack[:]); err != nil {
		t.Fatalf("read parent acknowledgement: %v", err)
	}
}

type repeatingReader struct {
	remaining int
}

func (r *repeatingReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	r.remaining -= n
	return n, nil
}

type failingReader struct {
	payload []byte
	err     error
	read    bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		return copy(p, r.payload), r.err
	}
	return 0, r.err
}

func waitForPipeOutputTests(t *testing.T, done ...<-chan struct{}) {
	t.Helper()
	for _, signal := range done {
		timer := time.NewTimer(3 * time.Second)
		select {
		case <-signal:
			timer.Stop()
		case <-timer.C:
			t.Fatal("pipe-output test goroutine did not stop")
			return
		}
	}
}
