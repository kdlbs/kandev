package process

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func (o *PassthroughOwner) validateTargetLocked(target types.PassthroughTarget) error {
	if target.ProcessID == "" || target != o.current.PassthroughTarget {
		return ErrPassthroughConflict
	}
	return nil
}

func (o *PassthroughOwner) Input(ctx context.Context, req types.PassthroughInputRequest) (types.PassthroughInputResult, error) {
	ctx, release, err := o.manager.BeginOwnedOperation(ctx)
	if err != nil {
		return types.PassthroughInputResult{}, err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return types.PassthroughInputResult{}, err
	}
	result, created, err := o.reserveInput(req)
	if err != nil {
		return types.PassthroughInputResult{}, err
	}
	if !created {
		select {
		case <-result.done:
			return types.PassthroughInputResult{Outcome: result.outcome, Sequence: result.sequence}, nil
		case <-ctx.Done():
			return types.PassthroughInputResult{}, ctx.Err()
		}
	}
	err = o.runner.WriteStdin(req.ProcessID, req.Data)
	o.mu.Lock()
	defer o.mu.Unlock()
	result.outcome = passthroughInputWritten
	if err != nil {
		result.outcome = "uncertain"
	}
	if err == nil && o.validateTargetLocked(req.PassthroughTarget) == nil && strings.ContainsAny(req.Data, "\r\n") {
		o.current.WaitingForInput = false
		o.emitLocked("submitted", nil)
	}
	result.sequence = o.current.Sequence
	close(result.done)
	return types.PassthroughInputResult{Outcome: result.outcome, Sequence: result.sequence}, nil
}

func (o *PassthroughOwner) reserveInput(req types.PassthroughInputRequest) (*passthroughInputResult, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.validateTargetLocked(req.PassthroughTarget); err != nil {
		return nil, false, err
	}
	digest := sha256.Sum256([]byte(req.Data))
	key := req.ProcessID + ":" + req.SubmissionID
	if previous, ok := o.inputs[key]; req.SubmissionID != "" && ok {
		if previous.digest != digest {
			return nil, false, ErrPassthroughConflict
		}
		return previous, false, nil
	}
	if len(req.Data) > 1024*1024 || len(req.SubmissionID) > 128 {
		return nil, false, errors.New("remote passthrough input limit reached")
	}
	if req.SubmissionID != "" {
		if err := o.makeInputRoomLocked(req.SubmissionID); err != nil {
			return nil, false, err
		}
	}
	result := &passthroughInputResult{digest: digest, done: make(chan struct{})}
	if req.SubmissionID != "" {
		o.inputs[key] = result
	}
	return result, true, nil
}

func (o *PassthroughOwner) LookupInput(ctx context.Context, target types.PassthroughTarget, submissionID string) (types.PassthroughInputResult, error) {
	o.mu.Lock()
	if err := o.validateTargetLocked(target); err != nil {
		o.mu.Unlock()
		return types.PassthroughInputResult{}, err
	}
	result, ok := o.inputs[target.ProcessID+":"+submissionID]
	o.mu.Unlock()
	if !ok || submissionID == "" {
		return types.PassthroughInputResult{}, ErrPassthroughNotFound
	}
	select {
	case <-ctx.Done():
		return types.PassthroughInputResult{}, ctx.Err()
	case <-result.done:
		return types.PassthroughInputResult{Outcome: result.outcome, Sequence: result.sequence}, nil
	}
}

func (o *PassthroughOwner) Resize(req types.PassthroughResizeRequest) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.validateTargetLocked(req.PassthroughTarget); err != nil {
		return err
	}
	if req.Cols == 0 || req.Rows == 0 {
		return errors.New("terminal dimensions must be positive")
	}
	if err := o.runner.ResizeByProcessID(req.ProcessID, req.Cols, req.Rows); err != nil {
		return err
	}
	o.terminalCols, o.terminalRows = int(req.Cols), int(req.Rows)
	return nil
}

func (o *PassthroughOwner) FirstIdle(ctx context.Context, target types.PassthroughTarget) error {
	o.mu.Lock()
	err := o.validateTargetLocked(target)
	o.mu.Unlock()
	if err != nil {
		return err
	}
	if err := o.runner.WaitForFirstIdle(ctx, target.ProcessID); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.validateTargetLocked(target); err != nil {
		return err
	}
	if !o.current.FirstIdle || !o.liveLocked() {
		return ErrPassthroughNotFound
	}
	return nil
}

func (o *PassthroughOwner) Stop(ctx context.Context, target types.PassthroughTarget) error {
	o.manager.startMu.Lock()
	defer o.manager.startMu.Unlock()
	o.mu.Lock()
	err := o.validateTargetLocked(target)
	o.mu.Unlock()
	if err != nil {
		return err
	}
	if _, exists := o.runner.Get(target.ProcessID, false); !exists {
		return nil
	}
	return o.runner.Stop(ctx, target.ProcessID)
}

func (o *PassthroughOwner) StopAll(ctx context.Context) error {
	if o == nil {
		return nil
	}
	snapshot := o.Snapshot()
	if snapshot.ProcessID == "" {
		return nil
	}
	if _, ok := o.runner.Get(snapshot.ProcessID, false); !ok {
		return nil
	}
	if !o.runner.IsProcessReadyOrPending(snapshot.ProcessID) {
		return nil
	}
	return o.Stop(ctx, snapshot.PassthroughTarget)
}

func (o *PassthroughOwner) Buffer(target types.PassthroughTarget) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.validateTargetLocked(target); err != nil {
		return "", err
	}
	chunks, _ := o.runner.GetBuffer(target.ProcessID)
	var data strings.Builder
	for _, chunk := range chunks {
		data.WriteString(chunk.Data)
	}
	return data.String(), nil
}

func (o *PassthroughOwner) SubscribeOutput(target types.PassthroughTarget) ([]byte, <-chan []byte, func(), error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.validateTargetLocked(target); err != nil {
		return nil, nil, nil, err
	}
	return o.runner.SubscribeOutput(target.ProcessID)
}
