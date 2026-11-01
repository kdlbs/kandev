package main

import "testing"

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-004.2
func TestSubagentControlScenarioEmitsRecognizedAndGenericRows(t *testing.T) {
	scenario, ok := scenarioRegistry["subagent-controls"]
	if !ok {
		t.Fatal("subagent-controls scenario is not registered")
	}
	emit, updates := newTestEmitter()
	scenario(emit)

	got := updates.getUpdates()
	if len(got) != 4 {
		t.Fatalf("scenario emitted %d updates, want 4", len(got))
	}
	sendMessage := got[0].notification.Update.ToolCall
	if sendMessage == nil || sendMessage.Title != "SendMessage" {
		t.Fatalf("first update = %+v, want a Claude SendMessage tool call", got[0].notification.Update)
	}
	claude, ok := sendMessage.Meta["claudeCode"].(map[string]any)
	if !ok || claude["toolName"] != "SendMessage" {
		t.Fatalf("SendMessage metadata = %v, want claudeCode.toolName SendMessage", sendMessage.Meta)
	}
	wait := got[2].notification.Update.ToolCall
	if wait == nil || wait.Title != "wait" || wait.Meta != nil {
		t.Fatalf("third update = %+v, want an unmarked generic wait tool call", got[2].notification.Update)
	}
}
