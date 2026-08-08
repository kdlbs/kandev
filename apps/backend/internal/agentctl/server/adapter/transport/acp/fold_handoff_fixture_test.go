package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// foldFixture is a trimmed ACP transcript recorded against the real
// claude-agent-acp bridge by scripts/probe-acp-midturn-fold. Volatile values
// (timings, session ids, token totals for unrelated fields) are dropped so the
// fixture pins the SHAPE the adapter must survive rather than one recorded run.
//
// ADR 0049 admits prompt handoff only for a provider bridge that is
// fixture-tested to accept the next human prompt echo while the earlier RPC is
// still open. These fixtures are that evidence, made executable.
type foldFixture struct {
	Kind   string          `json:"kind"`
	Frames []foldFixtFrame `json:"frames"`
}

type foldFixtFrame struct {
	Dir               string         `json:"dir"`
	Method            string         `json:"method,omitempty"`
	Update            string         `json:"update,omitempty"`
	Text              string         `json:"text,omitempty"`
	ToolCallID        string         `json:"toolCallId,omitempty"`
	Title             string         `json:"title,omitempty"`
	Status            string         `json:"status,omitempty"`
	StopReason        string         `json:"stopReason,omitempty"`
	PromptResponseFor *int           `json:"promptResponseFor,omitempty"`
	Usage             map[string]int `json:"usage,omitempty"`
	ID                int            `json:"id,omitempty"`
}

func loadFoldFixture(t *testing.T, name string) foldFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var fixture foldFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
	if len(fixture.Frames) == 0 {
		t.Fatalf("fixture %s has no frames", name)
	}
	return fixture
}

func usageIsZero(usage map[string]int) bool {
	for _, v := range usage {
		if v != 0 {
			return false
		}
	}
	return true
}

// TestFoldHandoffFixtureShape pins the observed contract that task 04's
// attribution work has to satisfy: the predecessor prompt resolves end_turn
// BEFORE it has produced any answer text and with zeroed usage, and the answer
// plus the real usage land on the successor prompt. An adapter that treats that
// early settlement as the predecessor's completion corrupts the transcript.
func TestFoldHandoffFixtureShape(t *testing.T) {
	fixture := loadFoldFixture(t, "acp-fold-handoff.json")
	if fixture.Kind != "fold_handoff" {
		t.Fatalf("fixture kind = %q, want fold_handoff", fixture.Kind)
	}

	var (
		sentPrompts        []int
		firstResponseIdx   = -1
		firstResponseForID int
		textBeforeFirst    int
		predecessorUsage   map[string]int
		successorUsage     map[string]int
		predecessorStop    string
		sleepToolCalls     int
		answer             strings.Builder
	)

	for i, f := range fixture.Frames {
		switch {
		case f.Dir == "sent" && f.Method == "session/prompt":
			sentPrompts = append(sentPrompts, f.ID)
		case f.PromptResponseFor != nil:
			if firstResponseIdx == -1 {
				firstResponseIdx = i
				firstResponseForID = *f.PromptResponseFor
				predecessorUsage = f.Usage
				predecessorStop = f.StopReason
			} else {
				successorUsage = f.Usage
			}
		case f.Update == "agent_message_chunk":
			if firstResponseIdx == -1 {
				textBeforeFirst++
			}
			answer.WriteString(f.Text)
		case f.Update == "tool_call" && strings.Contains(strings.ToLower(f.Title), "sleep"):
			sleepToolCalls++
		case f.Update == "tool_call_update" && strings.Contains(strings.ToLower(f.Title), "sleep"):
			sleepToolCalls++
		}
	}

	if len(sentPrompts) != 2 {
		t.Fatalf("sent prompts = %d, want 2 (predecessor + steer)", len(sentPrompts))
	}
	// The steer must have been written while the predecessor was still open —
	// otherwise this transcript proves nothing about concurrent prompts.
	secondSentIdx := -1
	for i, f := range fixture.Frames {
		if f.Dir == "sent" && f.Method == "session/prompt" && f.ID == sentPrompts[1] {
			secondSentIdx = i
		}
	}
	if secondSentIdx == -1 || secondSentIdx > firstResponseIdx {
		t.Fatalf("steer was not sent before the predecessor resolved (sent idx %d, response idx %d)",
			secondSentIdx, firstResponseIdx)
	}
	if firstResponseForID != sentPrompts[0] {
		t.Fatalf("first response answered prompt %d, want the predecessor %d",
			firstResponseForID, sentPrompts[0])
	}
	if predecessorStop != "end_turn" {
		t.Fatalf("predecessor stopReason = %q, want end_turn", predecessorStop)
	}
	if textBeforeFirst != 0 {
		t.Fatalf("predecessor produced %d text chunk(s) before resolving; the fold "+
			"contract is that it resolves with no answer of its own", textBeforeFirst)
	}
	if !usageIsZero(predecessorUsage) {
		t.Fatalf("predecessor usage = %v, want all zeros", predecessorUsage)
	}
	if usageIsZero(successorUsage) {
		t.Fatalf("successor usage = %v, want the turn's real usage", successorUsage)
	}
	if got := answer.String(); !strings.Contains(got, "STEERED") {
		t.Fatalf("answer = %q, want the steered reply", got)
	}
	if sleepToolCalls == 0 {
		t.Fatal("fixture has no sleep tool frames; it cannot show the steer took effect")
	}
}

// TestBackgroundSurvivesHandoffFixtureShape pins two facts task 04 depends on:
// a launch tool card reports completed long before its workload does, and the
// workload's eventual completion arrives as assistant text with NO tool update,
// after every prompt has already resolved. That is why background-work
// protection is required at the handoff boundary and why liveness cannot be
// tracked on the tool channel.
func TestBackgroundSurvivesHandoffFixtureShape(t *testing.T) {
	fixture := loadFoldFixture(t, "acp-fold-handoff-background.json")
	if fixture.Kind != "background_survives_handoff" {
		t.Fatalf("fixture kind = %q, want background_survives_handoff", fixture.Kind)
	}

	responses := 0
	lastResponseIdx := -1
	launchCompletedIdx := -1
	for i, f := range fixture.Frames {
		if f.PromptResponseFor != nil {
			responses++
			lastResponseIdx = i
		}
		if f.Update == "tool_call_update" && f.Status == "completed" && launchCompletedIdx == -1 {
			launchCompletedIdx = i
		}
	}
	if responses != 2 {
		t.Fatalf("prompt responses = %d, want 2", responses)
	}
	if launchCompletedIdx == -1 {
		t.Fatal("no completed tool_call_update; fixture cannot show launch-card completion")
	}
	if launchCompletedIdx > lastResponseIdx {
		t.Fatal("launch card completed after the last prompt resolved; fixture does not " +
			"show the card settling ahead of the workload")
	}

	// Everything after the final prompt response is orphan activity: a model
	// cycle with no owning session/prompt RPC.
	orphanText := 0
	orphanToolUpdates := 0
	for _, f := range fixture.Frames[lastResponseIdx+1:] {
		switch f.Update {
		case "agent_message_chunk":
			orphanText++
		case "tool_call", "tool_call_update":
			orphanToolUpdates++
		}
	}
	if orphanText == 0 {
		t.Fatal("no assistant text after the last prompt resolved; fixture does not show " +
			"background work outliving the handoff")
	}
	if orphanToolUpdates != 0 {
		t.Fatalf("orphan cycle carried %d tool update(s); the observed contract is that a "+
			"settled background workload reports only as text", orphanToolUpdates)
	}
}
