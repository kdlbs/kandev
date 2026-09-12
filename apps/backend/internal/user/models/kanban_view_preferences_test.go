package models

import "testing"

func TestIsValidKanbanSort(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"created_desc", true},
		{"priority_desc", true},
		{" priority_desc", true},
		{"", false},
		{"priority_asc", false},
		{"CREATED_DESC", false},
	}
	for _, tc := range cases {
		if got := IsValidKanbanSort(tc.value); got != tc.want {
			t.Errorf("IsValidKanbanSort(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestNormalizeKanbanSort(t *testing.T) {
	cases := []struct {
		value string
		want  string
	}{
		{"created_desc", "created_desc"},
		{"priority_desc", "priority_desc"},
		{" priority_desc", "priority_desc"},
		{"", KanbanSortDefault},
		{"priority_asc", KanbanSortDefault},
		{"garbage", KanbanSortDefault},
	}
	for _, tc := range cases {
		if got := NormalizeKanbanSort(tc.value); got != tc.want {
			t.Errorf("NormalizeKanbanSort(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestIsValidKanbanPriorityFilterToken(t *testing.T) {
	for _, token := range []string{"critical", "high", "medium", "low"} {
		if !IsValidKanbanPriorityFilterToken(token) {
			t.Errorf("IsValidKanbanPriorityFilterToken(%q) = false, want true", token)
		}
	}
	for _, token := range []string{"", "none", "URGENT", "critical "} {
		if token == "critical " {
			// Trimmed before comparison, so a trailing space is still valid.
			if !IsValidKanbanPriorityFilterToken(token) {
				t.Errorf("IsValidKanbanPriorityFilterToken(%q) = false, want true (trimmed)", token)
			}
			continue
		}
		if IsValidKanbanPriorityFilterToken(token) {
			t.Errorf("IsValidKanbanPriorityFilterToken(%q) = true, want false", token)
		}
	}
}

func TestKanbanPriorityFilterTokenRank(t *testing.T) {
	ranks := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
	for token, want := range ranks {
		got, ok := KanbanPriorityFilterTokenRank(token)
		if !ok || got != want {
			t.Errorf("KanbanPriorityFilterTokenRank(%q) = (%d, %v), want (%d, true)", token, got, ok, want)
		}
	}
	if _, ok := KanbanPriorityFilterTokenRank("unranked"); ok {
		t.Errorf("KanbanPriorityFilterTokenRank(unranked) ok = true, want false")
	}
}
