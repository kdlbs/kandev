package acp

import "testing"

func TestPlanEntriesFromTodosResult_MetadataTodos(t *testing.T) {
	entries, ok := planEntriesFromTodosResult(map[string]any{
		"metadata": map[string]any{
			"todos": []any{
				map[string]any{"content": "Gather PR state", "status": "in_progress", "priority": "high"},
				map[string]any{"content": "Fix CI", "status": "pending", "priority": "high"},
			},
			"truncated": false,
		},
		"output": "[]",
	})
	if !ok {
		t.Fatal("planEntriesFromTodosResult did not recognize metadata.todos")
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Description != "Gather PR state" ||
		entries[0].Status != "in_progress" ||
		entries[0].Priority != "high" {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].Description != "Fix CI" || entries[1].Status != "pending" {
		t.Fatalf("unexpected second entry: %+v", entries[1])
	}
}

func TestPlanEntriesFromTodosResult_IgnoresUnrelatedOutput(t *testing.T) {
	if entries, ok := planEntriesFromTodosResult(map[string]any{"output": "hello"}); ok || entries != nil {
		t.Fatalf("planEntriesFromTodosResult = (%+v, %v), want nil false", entries, ok)
	}
}
