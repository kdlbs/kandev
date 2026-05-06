package wakeup_test

import (
	"testing"

	"github.com/kandev/kandev/internal/office/wakeup"
)

func TestMarshalUnmarshal_Comment(t *testing.T) {
	in := wakeup.CommentPayload{TaskID: "t-1", CommentID: "c-1"}
	raw, err := wakeup.MarshalPayload(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out wakeup.CommentPayload
	if err := wakeup.UnmarshalPayload(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TaskID != "t-1" || out.CommentID != "c-1" {
		t.Errorf("comment roundtrip: %+v", out)
	}
}

func TestMarshalUnmarshal_AgentError(t *testing.T) {
	in := wakeup.AgentErrorPayload{
		AgentProfileID: "agent-1", RunID: "run-7", Error: "boom",
	}
	raw, _ := wakeup.MarshalPayload(in)
	var out wakeup.AgentErrorPayload
	if err := wakeup.UnmarshalPayload(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("agent error roundtrip: %+v vs %+v", out, in)
	}
}

func TestMarshalUnmarshal_Routine(t *testing.T) {
	in := wakeup.RoutinePayload{
		RoutineID: "r-1",
		Variables: map[string]any{"hour": float64(9), "today": "monday"},
	}
	raw, _ := wakeup.MarshalPayload(in)
	var out wakeup.RoutinePayload
	if err := wakeup.UnmarshalPayload(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.RoutineID != "r-1" {
		t.Errorf("routine_id: %q", out.RoutineID)
	}
	if out.Variables["today"] != "monday" {
		t.Errorf("variables today=%v", out.Variables["today"])
	}
}

func TestMarshalPayload_NilReturnsEmptyObject(t *testing.T) {
	raw, err := wakeup.MarshalPayload(nil)
	if err != nil {
		t.Fatalf("marshal nil: %v", err)
	}
	if raw != "{}" {
		t.Errorf("expected {}, got %q", raw)
	}
}

func TestUnmarshalPayload_EmptyIsNoop(t *testing.T) {
	var p wakeup.CommentPayload
	if err := wakeup.UnmarshalPayload("", &p); err != nil {
		t.Fatalf("empty: %v", err)
	}
	if err := wakeup.UnmarshalPayload("{}", &p); err != nil {
		t.Fatalf("braces: %v", err)
	}
	if p.TaskID != "" {
		t.Errorf("expected zero TaskID, got %q", p.TaskID)
	}
}
