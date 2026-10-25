package process

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types"
)

// SubscribeTerminal keeps a session connection across deliberate PTY replacement.
// Each attachment captures replay atomically with the process subscription.
func (o *PassthroughOwner) SubscribeTerminal(target types.PassthroughTarget) ([]byte, <-chan []byte, func(), error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.validateTargetLocked(target); err != nil {
		return nil, nil, nil, err
	}
	if len(o.terminalOutputs) >= 8 {
		return nil, nil, nil, errors.New("remote terminal connection limit reached")
	}
	buffer, source, detach, err := o.runner.SubscribeOutput(target.ProcessID)
	if err != nil {
		return nil, nil, nil, err
	}
	id := uuid.NewString()
	output := make(chan []byte, 64)
	o.terminalOutputs[id] = output
	ctx, cancel := context.WithCancel(o.manager.lifetimeCtx)
	done := make(chan struct{})
	go func() { defer close(done); o.followTerminal(ctx, id, target, source, detach) }()
	var once sync.Once
	release := func() {
		once.Do(func() { cancel(); <-done; o.mu.Lock(); defer o.mu.Unlock(); o.closeTerminalLocked(id) })
	}
	return buffer, output, release, nil
}

func (o *PassthroughOwner) followTerminal(ctx context.Context, id string, target types.PassthroughTarget, source <-chan []byte, detach func()) {
	defer func() { o.mu.Lock(); defer o.mu.Unlock(); o.closeTerminalLocked(id) }()
	defer func() { detach() }()
	for {
		o.mu.Lock()
		current, wake := o.current.PassthroughTarget, o.wake
		o.mu.Unlock()
		if current != target {
			detach()
			buffer, next, release, err := o.runner.SubscribeOutput(current.ProcessID)
			if err == nil {
				target, source, detach = current, next, release
				if len(buffer) > 0 && !o.publishTerminal(id, buffer) {
					return
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case data, ok := <-source:
			if !ok {
				if !o.runner.outputFinished(target.ProcessID) {
					return
				}
				source = nil
				continue
			}
			if !o.publishTerminal(id, data) {
				return
			}
		}
	}
}

func (o *PassthroughOwner) closeTerminalLocked(id string) {
	if output, ok := o.terminalOutputs[id]; ok {
		delete(o.terminalOutputs, id)
		close(output)
	}
}

func (o *PassthroughOwner) publishTerminal(id string, data []byte) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	output, ok := o.terminalOutputs[id]
	if !ok {
		return false
	}
	select {
	case output <- append([]byte(nil), data...):
		return true
	default:
		o.closeTerminalLocked(id)
		return false
	}
}

func (o *PassthroughOwner) TerminalNotice(target types.PassthroughTarget, data []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.validateTargetLocked(target); err != nil {
		return err
	}
	if len(data) > 4096 {
		return errors.New("terminal notice is too long")
	}
	for id, output := range o.terminalOutputs {
		select {
		case output <- append([]byte(nil), data...):
		default:
			o.closeTerminalLocked(id)
		}
	}
	return nil
}
