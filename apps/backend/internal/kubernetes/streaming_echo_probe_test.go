package kubernetes

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/remotecommand"
)

// cutAtStdinEOFExecutor echoes stdin like `cat`, but the first chunk comes
// back at once and every later chunk only after a delay. When stdin reaches
// EOF it ends the stream at once and drops the echo still in flight. That is
// the short read seen with Kata Containers: "returned 32768 of 65536 bytes".
type cutAtStdinEOFExecutor struct {
	chunk int
	delay time.Duration
}

func (e cutAtStdinEOFExecutor) Stream(options remotecommand.StreamOptions) error {
	return e.StreamWithContext(context.Background(), options)
}

func (e cutAtStdinEOFExecutor) StreamWithContext(ctx context.Context, options remotecommand.StreamOptions) error {
	pending := make(chan []byte, 64)
	stdinDone := make(chan struct{})
	go func() {
		defer close(stdinDone)
		buffer := make([]byte, e.chunk)
		for {
			n, err := options.Stdin.Read(buffer)
			if n > 0 {
				pending <- append([]byte(nil), buffer[:n]...)
			}
			if err != nil {
				return
			}
		}
	}()
	for first := true; ; first = false {
		select {
		case <-stdinDone:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case data := <-pending:
			if !first {
				select {
				case <-stdinDone:
					return nil
				case <-time.After(e.delay):
				}
			}
			_, _ = options.Stdout.Write(data)
		}
	}
}

// silentExecutor never echoes and ends when stdin closes.
type silentExecutor struct{}

func (silentExecutor) Stream(options remotecommand.StreamOptions) error {
	return silentExecutor{}.StreamWithContext(context.Background(), options)
}

func (silentExecutor) StreamWithContext(_ context.Context, options remotecommand.StreamOptions) error {
	_, err := bytes.NewBuffer(nil).ReadFrom(options.Stdin)
	return err
}

func probePayload() []byte {
	payload := make([]byte, streamingProbePayloadBytes)
	for i := range payload {
		payload[i] = byte(i * 7)
	}
	return payload
}

// TestExecEchoProbeKeepsStdinOpenUntilTheEchoArrives reproduces a runtime that
// cuts stdout when stdin reaches EOF. The probe must keep stdin open until the
// whole echo is back, so a working data path passes.
func TestExecEchoProbeKeepsStdinOpenUntilTheEchoArrives(t *testing.T) {
	executor := cutAtStdinEOFExecutor{chunk: 32 << 10, delay: 50 * time.Millisecond}

	err := runExecEchoProbe(context.Background(), executor, probePayload(), 5*time.Second)

	require.NoError(t, err)
}

func TestExecEchoProbeClosesStdinAfterTheWaitAndReportsShortEcho(t *testing.T) {
	done := make(chan error, 1)

	go func() {
		done <- runExecEchoProbe(context.Background(), silentExecutor{}, probePayload(), 100*time.Millisecond)
	}()

	select {
	case err := <-done:
		require.ErrorContains(t, err, "returned 0 of 65536 bytes")
	case <-time.After(5 * time.Second):
		t.Fatal("probe kept stdin open past its echo wait")
	}
}
