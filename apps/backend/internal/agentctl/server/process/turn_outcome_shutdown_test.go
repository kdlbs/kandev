package process

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
)

func TestTerminalOutcomeDeliveryDoesNotWaitForLifecycleLock(t *testing.T) {
	for _, wired := range []bool{false, true} {
		name := "without recorder"
		if wired {
			name = "with recorder"
		}
		t.Run(name, func(t *testing.T) {
			m := &Manager{updatesCh: make(chan adapter.AgentEvent, 1)}
			recorder := &fakeTurnOutcomeRecorder{}
			if wired {
				m.SetTurnOutcomeRecorder("instance-1", recorder)
			}

			// Stop holds this lock while it joins the process-exit worker.
			m.mu.Lock()
			done := make(chan bool, 1)
			go func() {
				done <- m.sendUpdateBlocking(adapter.AgentEvent{Type: adapter.EventTypeError})
			}()
			select {
			case delivered := <-done:
				if !delivered {
					t.Error("terminal event was not delivered")
				}
			case <-time.After(time.Second):
				t.Error("terminal event delivery blocked on the lifecycle lock")
				m.mu.Unlock()
				<-done
				return
			}
			m.mu.Unlock()

			event := <-m.updatesCh
			if wired && (event.ControlTurnID != 1 || len(recorder.calls) != 1) {
				t.Fatalf("terminal event = %+v, recorder calls = %d", event, len(recorder.calls))
			}
		})
	}
}
