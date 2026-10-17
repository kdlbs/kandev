package process

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// SubscribeOutput captures replay and subscribes under the same output lock.
// A slow consumer is disconnected instead of blocking PTY draining.
func (r *InteractiveRunner) SubscribeOutput(processID string) ([]byte, <-chan []byte, func(), error) {
	proc, ok := r.get(processID)
	if !ok {
		return nil, nil, nil, fmt.Errorf("process not found")
	}
	proc.outputMu.Lock()
	defer proc.outputMu.Unlock()
	var buffer strings.Builder
	for _, chunk := range proc.buffer.snapshot() {
		buffer.WriteString(chunk.Data)
	}
	id := uuid.NewString()
	channel := make(chan []byte, 64)
	select {
	case <-proc.outputDone:
		close(channel)
	default:
		proc.outputSubscribers[id] = channel
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			proc.outputMu.Lock()
			defer proc.outputMu.Unlock()
			if ch, ok := proc.outputSubscribers[id]; ok {
				delete(proc.outputSubscribers, id)
				close(ch)
			}
		})
	}
	return []byte(buffer.String()), channel, release, nil
}

func (r *InteractiveRunner) HasOutputSubscribers(processID string) bool {
	proc, ok := r.get(processID)
	if !ok {
		return false
	}
	proc.outputMu.Lock()
	defer proc.outputMu.Unlock()
	return len(proc.outputSubscribers) > 0
}

func (r *InteractiveRunner) bufferAndPublishOutput(proc *interactiveProcess, chunk ProcessOutputChunk, data []byte) {
	proc.outputMu.Lock()
	defer proc.outputMu.Unlock()
	proc.buffer.append(chunk)
	for id, ch := range proc.outputSubscribers {
		select {
		case ch <- append([]byte(nil), data...):
		default:
			delete(proc.outputSubscribers, id)
			close(ch)
		}
	}
}

func (r *InteractiveRunner) closeOutputSubscriptions(proc *interactiveProcess) {
	proc.outputMu.Lock()
	defer proc.outputMu.Unlock()
	for id, ch := range proc.outputSubscribers {
		delete(proc.outputSubscribers, id)
		close(ch)
	}
}

func (r *InteractiveRunner) outputFinished(processID string) bool {
	proc, ok := r.get(processID)
	if !ok {
		return true
	}
	select {
	case <-proc.outputDone:
		return true
	default:
		return false
	}
}
