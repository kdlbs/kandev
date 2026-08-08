package main

import "testing"

func TestSlowResponseBareNumberUsesSeconds(t *testing.T) {
	e, updates := newTestEmitter()
	emitSlowResponse(e, "/slow 1", "mock-fast")

	const want = "Running slow response (1s total)..."
	for _, update := range updates.getUpdates() {
		if getTextContent(update) == want {
			return
		}
	}
	t.Fatalf("slow response did not report %q", want)
}
