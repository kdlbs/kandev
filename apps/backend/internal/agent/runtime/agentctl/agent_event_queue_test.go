package client

import "testing"

func TestAgentEventQueueRejectsUnboundedGrowth(t *testing.T) {
	queue := newAgentEventQueue()
	queue.limit = 1
	if !queue.enqueue(AgentEvent{Type: "message_chunk"}) {
		t.Fatal("first event was rejected")
	}
	if queue.enqueue(AgentEvent{Type: "message_chunk"}) {
		t.Fatal("queue accepted an event beyond its limit")
	}
	queue.close()
	if queue.enqueue(AgentEvent{Type: "message_chunk"}) {
		t.Fatal("closed queue accepted an event")
	}
}
