package process

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"testing/synctest"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func receiptTestOwner(t *testing.T) *PassthroughOwner {
	t.Helper()
	m := &Manager{cfg: &config.InstanceConfig{SessionID: "session", InstanceID: "instance"}, logger: newTestLogger(t)}
	m.status.Store(StatusStopped)
	return newPassthroughOwner(m)
}

func TestPassthroughStartReceiptsRetireReplacedGenerations(t *testing.T) {
	o := receiptTestOwner(t)
	for i := range 140 {
		req := types.PassthroughStartRequest{OperationID: fmt.Sprint(i), ExpectedGeneration: uint64(i)}
		_, previous, err := o.reserveStart(req, sha256.Sum256([]byte(req.OperationID)))
		require.NoError(t, err)
		require.Nil(t, previous)
		_, err = o.finishStart(req.OperationID, sha256.Sum256([]byte(req.OperationID)), fmt.Errorf("missing CLI"))
		require.Error(t, err)
	}
	require.Len(t, o.starts, 1)
	_, _, err := o.reserveStart(types.PassthroughStartRequest{OperationID: "0"}, sha256.Sum256([]byte("0")))
	require.ErrorIs(t, err, ErrPassthroughConflict)
}

func TestPassthroughInputReceiptsRemainBoundedWithoutReplay(t *testing.T) {
	o := receiptTestOwner(t)
	o.current.PassthroughTarget = types.PassthroughTarget{ProcessID: "process", Generation: 1}
	var first string
	for i := range 4100 {
		id := uuid.Must(uuid.NewV7()).String()
		if i == 0 {
			first = id
		}
		result, created, err := o.reserveInput(types.PassthroughInputRequest{PassthroughTarget: o.current.PassthroughTarget, SubmissionID: id, Data: "input"})
		require.NoError(t, err)
		require.True(t, created)
		result.outcome = passthroughInputWritten
		close(result.done)
	}
	require.LessOrEqual(t, len(o.inputs), 4096)
	_, _, err := o.reserveInput(types.PassthroughInputRequest{PassthroughTarget: o.current.PassthroughTarget, SubmissionID: first, Data: "input"})
	require.ErrorIs(t, err, ErrPassthroughConflict, "expired IDs must never write again")
	_, created, err := o.reserveInput(types.PassthroughInputRequest{PassthroughTarget: o.current.PassthroughTarget, Data: "key"})
	require.NoError(t, err)
	require.True(t, created, "browser input does not consume receipt capacity")
}

func TestPassthroughTerminalDisconnectsAfterSourceOverflow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := receiptTestOwner(t)
		runner, proc := newRunnerWithProcess(t, "process")
		o.runner = runner
		proc.outputSubscribers = make(map[string]chan []byte)
		proc.outputDone = make(chan struct{})
		proc.buffer = newRingBuffer(1024)
		o.current.PassthroughTarget = types.PassthroughTarget{ProcessID: "process", Generation: 1}
		_, source, detach, err := runner.SubscribeOutput("process")
		require.NoError(t, err)
		for range 65 {
			runner.bufferAndPublishOutput(proc, ProcessOutputChunk{Data: "x"}, []byte("x"))
		}
		output := make(chan []byte, 128)
		o.terminalOutputs["terminal"] = output
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan struct{})
		go func() {
			defer close(done)
			o.followTerminal(ctx, "terminal", o.current.PassthroughTarget, source, detach)
		}()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("overflow left browser stream open without a source")
		}
		var received int
		for range output {
			received++
		}
		require.Equal(t, 64, received)
		replay, _, release, err := runner.SubscribeOutput("process")
		require.NoError(t, err)
		defer release()
		require.Len(t, replay, 65, "reconnect must replay dropped output")
	})
}

func TestPassthroughInputRetentionPreservesPendingAndAdvancingFloor(t *testing.T) {
	o := receiptTestOwner(t)
	o.current.PassthroughTarget = types.PassthroughTarget{ProcessID: "process", Generation: 1}
	ids := make([]string, 4100)
	for i := range ids {
		ids[i] = uuid.Must(uuid.NewV7()).String()
	}
	var pending *passthroughInputResult
	for i, id := range ids[:4098] {
		result, _, err := o.reserveInput(types.PassthroughInputRequest{PassthroughTarget: o.current.PassthroughTarget, SubmissionID: id, Data: "same"})
		require.NoError(t, err)
		if i == 0 {
			pending = result
			continue
		}
		result.outcome = passthroughInputWritten
		close(result.done)
	}
	require.NotEmpty(t, o.inputFloor)
	floor := o.inputFloor
	require.Same(t, pending, o.inputs["process:"+ids[0]])
	pending.outcome = passthroughInputWritten
	close(pending.done)
	_, _, err := o.reserveInput(types.PassthroughInputRequest{PassthroughTarget: o.current.PassthroughTarget, SubmissionID: ids[4098], Data: "next"})
	require.NoError(t, err)
	require.GreaterOrEqual(t, o.inputFloor, floor)
	_, _, err = o.reserveInput(types.PassthroughInputRequest{PassthroughTarget: o.current.PassthroughTarget, SubmissionID: ids[1], Data: "same"})
	require.ErrorIs(t, err, ErrPassthroughConflict)
	_, created, err := o.reserveInput(types.PassthroughInputRequest{PassthroughTarget: o.current.PassthroughTarget, SubmissionID: ids[4097], Data: "same"})
	require.NoError(t, err)
	require.False(t, created)
	_, _, err = o.reserveInput(types.PassthroughInputRequest{PassthroughTarget: o.current.PassthroughTarget, SubmissionID: ids[4097], Data: "different"})
	require.ErrorIs(t, err, ErrPassthroughConflict)
}

func TestPassthroughTerminalPreservesConnectionAfterProcessOutputEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := receiptTestOwner(t)
		runner, proc := newRunnerWithProcess(t, "process")
		o.runner = runner
		proc.outputSubscribers = make(map[string]chan []byte)
		proc.outputDone = make(chan struct{})
		proc.buffer = newRingBuffer(1024)
		o.current.PassthroughTarget = types.PassthroughTarget{ProcessID: "process", Generation: 1}
		_, source, detach, err := runner.SubscribeOutput("process")
		require.NoError(t, err)
		close(proc.outputDone)
		runner.closeOutputSubscriptions(proc)
		output := make(chan []byte, 128)
		o.terminalOutputs["terminal"] = output
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		target := o.current.PassthroughTarget
		done := make(chan struct{})
		go func() { defer close(done); o.followTerminal(ctx, "terminal", target, source, detach) }()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("normal output end disconnected the session terminal")
		default:
		}
		_, next := newRunnerWithProcess(t, "replacement")
		next.outputSubscribers = make(map[string]chan []byte)
		next.outputDone = make(chan struct{})
		next.buffer = newRingBuffer(1024)
		runner.mu.Lock()
		runner.processes["replacement"] = next
		runner.mu.Unlock()
		o.mu.Lock()
		o.current.PassthroughTarget = types.PassthroughTarget{ProcessID: "replacement", Generation: 2}
		o.emitLocked("started", nil)
		o.mu.Unlock()
		synctest.Wait()
		runner.bufferAndPublishOutput(next, ProcessOutputChunk{Data: "replacement output"}, []byte("replacement output"))
		require.Equal(t, []byte("replacement output"), <-output)
		cancel()
		<-done
	})
}
