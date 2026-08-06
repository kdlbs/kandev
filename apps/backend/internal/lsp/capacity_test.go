package lsp

import (
	"os"
	"testing"
	"time"
)

func TestCapacityCountsTaskLanguageServersNotAttachments(t *testing.T) {
	capacity := NewCapacity(2)
	first := TaskLanguageKey{TaskID: "task-1", Language: "kotlin"}
	second := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	queued := TaskLanguageKey{TaskID: "task-2", Language: "kotlin"}

	if !capacity.Admit(first, 1, time.Unix(1, 0)) ||
		!capacity.Admit(first, 1, time.Unix(2, 0)) ||
		!capacity.Admit(second, 1, time.Unix(3, 0)) {
		t.Fatal("real task/language servers were not admitted idempotently")
	}
	if capacity.Admit(queued, 1, time.Unix(4, 0)) {
		t.Fatal("server beyond capacity was admitted")
	}
	if capacity.Active() != 2 || capacity.Queued() != 1 {
		t.Fatalf("capacity active=%d queued=%d", capacity.Active(), capacity.Queued())
	}

	next := capacity.Release(first, 1)
	if next == nil || next.Key != queued || next.Generation != 1 {
		t.Fatalf("released queue candidate = %#v", next)
	}
	if capacity.Active() != 2 || capacity.Queued() != 0 {
		t.Fatalf("promoted capacity active=%d queued=%d", capacity.Active(), capacity.Queued())
	}
}

func TestCapacityQueueOrderUsesAcceptanceThenTaskLanguage(t *testing.T) {
	capacity := NewCapacity(1)
	active := TaskLanguageKey{TaskID: "active", Language: "go"}
	if !capacity.Admit(active, 1, time.Unix(1, 0)) {
		t.Fatal("active server was not admitted")
	}
	accepted := time.Unix(5, 0)
	keys := []TaskLanguageKey{
		{TaskID: "task-b", Language: "python"},
		{TaskID: "task-a", Language: "rust"},
		{TaskID: "task-a", Language: "go"},
	}
	for _, key := range keys {
		if capacity.Admit(key, 2, accepted) {
			t.Fatalf("queued key admitted: %#v", key)
		}
	}
	want := []TaskLanguageKey{
		{TaskID: "task-a", Language: "go"},
		{TaskID: "task-a", Language: "rust"},
		{TaskID: "task-b", Language: "python"},
	}
	current := active
	currentGeneration := uint64(1)
	for index, expected := range want {
		next := capacity.Release(current, currentGeneration)
		if next == nil || next.Key != expected {
			t.Fatalf("queue %d = %#v, want %#v", index, next, expected)
		}
		current = expected
		currentGeneration = 2
	}
}

func TestCapacityEnvironmentParsingPrefersServersAndFallsBackToLegacy(t *testing.T) {
	t.Setenv("KANDEV_LSP_MAX_SERVERS", "3")
	t.Setenv("KANDEV_LSP_MAX_CONNECTIONS", "7")
	if got := NewCapacityFromEnv().Limit(); got != 3 {
		t.Fatalf("preferred capacity = %d, want 3", got)
	}

	if err := os.Unsetenv("KANDEV_LSP_MAX_SERVERS"); err != nil {
		t.Fatal(err)
	}
	if got := NewCapacityFromEnv().Limit(); got != 7 {
		t.Fatalf("legacy capacity = %d, want 7", got)
	}

	t.Setenv("KANDEV_LSP_MAX_SERVERS", "invalid")
	if got := NewCapacityFromEnv().Limit(); got != DefaultMaxServers {
		t.Fatalf("invalid preferred capacity = %d, want default %d", got, DefaultMaxServers)
	}
}

func TestCapacityAdoptsAlreadyRunningServersAboveConfiguredLimit(t *testing.T) {
	capacity := NewCapacity(1)
	first := TaskLanguageKey{TaskID: "task-1", Language: "go"}
	second := TaskLanguageKey{TaskID: "task-2", Language: "kotlin"}
	capacity.Adopt(first, 3)
	capacity.Adopt(second, 7)
	capacity.Adopt(first, 3)
	if capacity.Active() != 2 || capacity.Queued() != 0 {
		t.Fatalf("adopted active=%d queued=%d", capacity.Active(), capacity.Queued())
	}
}
