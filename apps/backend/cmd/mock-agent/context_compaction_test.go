package main

import (
	"context"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestInitializeRecordsClientCompactionSupport(t *testing.T) {
	tests := []struct {
		name string
		req  acp.InitializeRequest
		want bool
	}{
		{name: "not advertised"},
		{
			name: "advertised",
			req: acp.InitializeRequest{ClientCapabilities: acp.ClientCapabilities{
				Session: &acp.ClientSessionCapabilities{Compaction: &acp.CompactionCapabilities{}},
			}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &mockAgent{}
			if _, err := agent.Initialize(context.Background(), tt.req); err != nil {
				t.Fatalf("initialize: %v", err)
			}
			if got := agent.clientSupportsCompaction; got != tt.want {
				t.Fatalf("clientSupportsCompaction = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestScenarioContextCompactionRequiresCapability(t *testing.T) {
	t.Run("without capability", func(t *testing.T) {
		e, updater := newTestEmitter()
		scenarioContextCompaction(e)
		for _, update := range updater.getUpdates() {
			if update.notification.Update.CompactionUpdate != nil || update.notification.Update.CompactionSummaryChunk != nil {
				t.Fatal("scenario emitted a compaction update without the client capability")
			}
		}
	})

	t.Run("with capability", func(t *testing.T) {
		e, updater := newTestEmitter()
		e.supportsCompaction = true
		scenarioContextCompaction(e)
		var updates, chunks int
		for _, frame := range updater.getUpdates() {
			if frame.notification.Update.CompactionUpdate != nil {
				updates++
			}
			if frame.notification.Update.CompactionSummaryChunk != nil {
				chunks++
			}
		}
		if updates != 3 || chunks != 2 {
			t.Fatalf("scenario emitted %d update frames and %d summary chunks, want 3 and 2", updates, chunks)
		}
	})
}
