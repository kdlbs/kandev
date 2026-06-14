package acp

import (
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func planEntriesFromTodosResult(rawOutput any) ([]streams.PlanEntry, bool) {
	items, ok := extractTodoItems(rawOutput)
	if !ok {
		return nil, false
	}
	entries := make([]streams.PlanEntry, len(items))
	for i, item := range items {
		entries[i] = streams.PlanEntry{
			Description: item.Description,
			Status:      item.Status,
			Priority:    item.Priority,
		}
	}
	return entries, true
}

type acpTodoItem struct {
	Description string
	Status      string
	Priority    string
}

func extractTodoItems(raw any) ([]acpTodoItem, bool) {
	todosRaw, ok := findTodosRaw(raw)
	if !ok {
		return nil, false
	}
	todosSlice, ok := todosRaw.([]any)
	if !ok {
		return nil, false
	}
	items := make([]acpTodoItem, 0, len(todosSlice))
	for _, rawItem := range todosSlice {
		item, ok := todoItemFromRaw(rawItem)
		if !ok {
			continue
		}
		items = append(items, item)
	}
	return items, true
}

func findTodosRaw(raw any) (any, bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	if todos, ok := m["todos"]; ok {
		return todos, true
	}
	if meta, ok := m["metadata"].(map[string]any); ok {
		if todos, ok := meta["todos"]; ok {
			return todos, true
		}
	}
	if rawOutput, ok := m["rawOutput"].(map[string]any); ok {
		return findTodosRaw(rawOutput)
	}
	return nil, false
}

func todoItemFromRaw(raw any) (acpTodoItem, bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		return acpTodoItem{}, false
	}
	desc := stringFromTodoMap(m, "content", "text", "description")
	if desc == "" {
		return acpTodoItem{}, false
	}
	return acpTodoItem{
		Description: desc,
		Status:      stringFromTodoMap(m, "status"),
		Priority:    stringFromTodoMap(m, "priority"),
	}, true
}

func stringFromTodoMap(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}
