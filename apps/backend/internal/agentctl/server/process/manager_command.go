package process

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	tools "github.com/kandev/kandev/internal/tools/installer"
)

const managedCommandOutputLimit = 1 << 20

// CombinedOutput runs a direct command as an instance-owned process and
// returns bounded stdout/stderr after its complete process tree is reaped.
func (m *Manager) CombinedOutput(parent context.Context, spec tools.CommandSpec) ([]byte, error) {
	ctx, release, err := m.BeginOwnedOperation(parent)
	if err != nil {
		return nil, err
	}
	defer release()

	sessionID := m.cfg.SessionID
	if sessionID == "" {
		sessionID = m.cfg.InstanceID
	}
	if sessionID == "" {
		sessionID = "managed-command"
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	proc, err := m.StartPipedProcess(PipedStartRequest{
		SessionID:  sessionID,
		Kind:       types.ProcessKindCustom,
		ScriptName: "managed-command",
		Command:    spec.Path,
		Args:       spec.Args,
		WorkingDir: spec.Dir,
		Env:        spec.Env,
		PipeStderr: true,
	})
	if err != nil {
		return nil, err
	}
	_ = proc.Stdin.Close()
	stdout := collectManagedCommandOutput(proc.Stdout)
	stderr := collectManagedCommandOutput(proc.Stderr)

	var stopErr error
	canceled := false
	finished := true
	select {
	case <-proc.Done:
	case <-ctx.Done():
		canceled = true
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		stopErr = m.StopProcess(stopCtx, StopProcessRequest{ProcessID: proc.ID})
		select {
		case <-proc.Done:
		case <-stopCtx.Done():
			finished = false
			stopErr = errors.Join(stopErr, stopCtx.Err())
		}
		cancel()
	}
	if !finished {
		return nil, errors.Join(ctx.Err(), stopErr)
	}

	output := append(<-stdout, (<-stderr)...)
	waitErr := proc.Wait()
	if canceled {
		return output, errors.Join(ctx.Err(), stopErr, waitErr)
	}
	return output, waitErr
}

func collectManagedCommandOutput(reader io.ReadCloser) <-chan []byte {
	result := make(chan []byte, 1)
	go func() {
		defer func() { _ = reader.Close() }()
		var output bytes.Buffer
		_, _ = io.Copy(&boundedOutputWriter{buffer: &output, remaining: managedCommandOutputLimit}, reader)
		result <- output.Bytes()
	}()
	return result
}

type boundedOutputWriter struct {
	buffer    *bytes.Buffer
	remaining int
}

func (w *boundedOutputWriter) Write(data []byte) (int, error) {
	written := len(data)
	if w.remaining > 0 {
		keep := min(len(data), w.remaining)
		_, _ = w.buffer.Write(data[:keep])
		w.remaining -= keep
	}
	return written, nil
}
