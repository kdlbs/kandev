package client

import (
	"encoding/json"
	"testing"
)

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

func TestAgentEventQueueRejectsByteOverflowAndReleasesCapacityOnDequeue(t *testing.T) {
	queue := newAgentEventQueue()
	queue.limit = 10
	queue.byteLimit = len(mustMarshalAgentEvent(t, AgentEvent{Type: "message_chunk", Text: "one"}))

	if !queue.enqueue(AgentEvent{Type: "message_chunk", Text: "one"}) {
		t.Fatal("first event was rejected")
	}
	if queue.enqueue(AgentEvent{Type: "message_chunk", Text: "two"}) {
		t.Fatal("queue accepted an event beyond its byte limit")
	}

	if _, ok := queue.dequeue(); !ok {
		t.Fatal("queued event was not dequeued")
	}
	if !queue.enqueue(AgentEvent{Type: "message_chunk", Text: "two"}) {
		t.Fatal("dequeue did not release byte capacity")
	}
}

func mustMarshalAgentEvent(t *testing.T, event AgentEvent) []byte {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return payload
}
