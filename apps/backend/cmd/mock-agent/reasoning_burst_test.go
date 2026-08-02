package main

import (
	"strings"
	"testing"
)

func TestParseReasoningBurstCommand(t *testing.T) {
	for _, test := range []struct {
		line  string
		count int
		ok    bool
	}{
		{line: "e2e:reasoning_burst(12)", count: 12, ok: true},
		{line: "e2e:reasoning-burst(12)", count: 12, ok: true},
		{line: "e2e:reasoning_burst(0)", count: reasoningBurstDefaultCount, ok: true},
		{line: "e2e:message(12)", count: 0, ok: false},
	} {
		count, ok := parseReasoningBurstCommand(test.line)
		if count != test.count || ok != test.ok {
			t.Errorf("parseReasoningBurstCommand(%q) = (%d, %v), want (%d, %v)", test.line, count, ok, test.count, test.ok)
		}
	}
	if count, _ := parseReasoningBurstCommand("e2e:reasoning_burst(999999)"); count != reasoningBurstMaxCount {
		t.Fatalf("burst count should be capped at %d, got %d", reasoningBurstMaxCount, count)
	}
}

func TestEmitReasoningBurstProducesExactContentAndMarker(t *testing.T) {
	const count = 37
	e, mock := newTestEmitter()
	emitReasoningBurst(e, count)

	updates := mock.getUpdates()
	if len(updates) != count+1 {
		t.Fatalf("expected %d updates, got %d", count+1, len(updates))
	}
	var thought strings.Builder
	for index := 0; index < count; index++ {
		if !isThoughtUpdate(updates[index]) {
			t.Fatalf("update %d should be a thought", index)
		}
		thought.WriteString(getThoughtContent(updates[index]))
	}
	if got, want := thought.String(), reasoningBurstContent(count); got != want {
		t.Fatalf("reasoning content mismatch: got %q, want %q", got, want)
	}
	if got, want := getTextContent(updates[count]), "reasoning-burst-produced:37"; got != want {
		t.Fatalf("produced marker = %q, want %q", got, want)
	}
}

func TestExecuteScriptReasoningBurst(t *testing.T) {
	e, mock := newTestEmitter()
	executeScript(e, "", "e2e:reasoning_burst(3)")
	updates := mock.getUpdates()
	if len(updates) != 4 {
		t.Fatalf("expected three thought chunks plus marker, got %d", len(updates))
	}
}
