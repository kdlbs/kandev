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

func TestPlanEntriesFromTodosResult_TopLevelTodos(t *testing.T) {
	entries, ok := planEntriesFromTodosResult(map[string]any{
		"todos": []any{
			map[string]any{"text": "Read README", "status": "pending"},
		},
	})
	if !ok || len(entries) != 1 {
		t.Fatalf("planEntriesFromTodosResult = (%+v, %v), want one entry ok=true", entries, ok)
	}
	if entries[0].Description != "Read README" || entries[0].Status != "pending" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}

func TestPlanEntriesFromTodosResult_RawOutputWrapped(t *testing.T) {
	entries, ok := planEntriesFromTodosResult(map[string]any{
		"rawOutput": map[string]any{
			"todos": []any{
				map[string]any{"description": "Investigate", "status": "in_progress"},
			},
		},
	})
	if !ok || len(entries) != 1 {
		t.Fatalf("planEntriesFromTodosResult = (%+v, %v), want one entry ok=true", entries, ok)
	}
	if entries[0].Description != "Investigate" || entries[0].Status != "in_progress" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}

func TestPlanEntriesFromTodosResult_SkipsItemsWithoutContent(t *testing.T) {
	entries, ok := planEntriesFromTodosResult(map[string]any{
		"todos": []any{
			map[string]any{"status": "pending"},
			map[string]any{"text": "Real item", "status": "in_progress"},
		},
	})
	if !ok || len(entries) != 1 {
		t.Fatalf("planEntriesFromTodosResult = (%+v, %v), want one valid entry", entries, ok)
	}
	if entries[0].Description != "Real item" {
		t.Fatalf("unexpected description: %q", entries[0].Description)
	}
}

func TestPlanEntriesFromTodosResult_EmptyArrayIsNotMatched(t *testing.T) {
	if entries, ok := planEntriesFromTodosResult(map[string]any{"todos": []any{}}); ok || entries != nil {
		t.Fatalf("planEntriesFromTodosResult = (%+v, %v), want nil false (empty array must not wipe indicator)", entries, ok)
	}
}

func TestPlanEntriesFromTodosResult_AllMalformedIsNotMatched(t *testing.T) {
	if entries, ok := planEntriesFromTodosResult(map[string]any{
		"todos": []any{
			map[string]any{"status": "pending"},
			"not-an-object",
		},
	}); ok || entries != nil {
		t.Fatalf("planEntriesFromTodosResult = (%+v, %v), want nil false (no recognisable items)", entries, ok)
	}
}

func TestPlanEntriesFromTodosResult_IgnoresUnrelatedOutput(t *testing.T) {
	if entries, ok := planEntriesFromTodosResult(map[string]any{"output": "hello"}); ok || entries != nil {
		t.Fatalf("planEntriesFromTodosResult = (%+v, %v), want nil false", entries, ok)
	}
}

func TestPlanEntriesFromTodosResult_IgnoresNonArrayTodos(t *testing.T) {
	if entries, ok := planEntriesFromTodosResult(map[string]any{
		"todos": map[string]any{"status": "pending"},
	}); ok || entries != nil {
		t.Fatalf("planEntriesFromTodosResult = (%+v, %v), want nil false (non-array todos should be ignored)", entries, ok)
	}
}
