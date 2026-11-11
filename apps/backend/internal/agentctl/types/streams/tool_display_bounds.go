package streams

import (
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agentctl/types/deliverylimits"
)

const (
	toolValueBytes   = int(deliverylimits.GenericValueBytes)
	toolPreviewBytes = int(deliverylimits.PreviewBytes)
	toolValueNodes   = 256
	toolValueDepth   = 16
)

// BoundGenericToolEvent is retained for callers that used the generic-only
// policy. It now delegates to the shared all-category preparation so typed
// tool reports cannot bypass the display budget.
func BoundGenericToolEvent(event AgentEvent) AgentEvent {
	return PrepareToolEvent(event)
}

func boundToolValue(value any) any {
	budget, nodes := toolValueBytes, toolValueNodes
	bounded, truncated := copyToolValue(value, &budget, &nodes, 0)
	if !truncated {
		return bounded
	}
	return truncatedToolValue(value)
}

func truncatedToolValue(value any) any {
	// Reserve the marker's nodes and depth, plus the omitted leaf's depth, so
	// another preparation accepts the preview without wrapping it again.
	budget, nodes := toolPreviewBytes, toolValueNodes-2
	preview, _ := copyToolValue(value, &budget, &nodes, 2)
	return map[string]any{"truncated": true, "preview": preview}
}

func toolContentsPreview(contents []ToolCallContentItem) string {
	var preview strings.Builder
	budget := toolPreviewBytes
	for _, item := range contents[:min(len(contents), toolValueNodes)] {
		if item.Content == nil || item.Content.Type != "text" {
			continue
		}
		text, cut := copyToolString(item.Content.Text, &budget)
		preview.WriteString(text)
		if cut || budget < 8 {
			break
		}
	}
	return preview.String()
}

func copyToolContents(contents []ToolCallContentItem) []ToolCallContentItem {
	result := slices.Clone(contents)
	for i := range result {
		if c := result[i].Content; c != nil {
			content := *c
			if content.Size != nil {
				size := *content.Size
				content.Size = &size
			}
			result[i].Content = &content
		}
		if result[i].OldText != nil {
			text := *result[i].OldText
			result[i].OldText = &text
		}
	}
	return result
}

// Byte, container and node budgets bound allocation and traversal, including
// escaped JSON strings and deeply nested or wide values.
func copyToolValue(value any, budget, nodes *int, depth int) (any, bool) {
	if *budget < 8 || *nodes == 0 || depth > toolValueDepth {
		return nil, true
	}
	*nodes--
	*budget -= 8
	if value == nil {
		return nil, false
	}
	switch v := value.(type) {
	case string:
		return copyToolString(v, budget)
	case json.RawMessage:
		if len(v) > *budget/6 {
			return nil, true
		}
		*budget -= 6 * len(v)
		return append(json.RawMessage(nil), v...), false
	case []byte:
		if len(v) > *budget/2 {
			return nil, true
		}
		*budget -= 2 * len(v)
		return append([]byte(nil), v...), false
	case json.Number:
		if len(v) > *budget {
			return nil, true
		}
		*budget -= len(v)
		return v, false
	}
	return copyToolReflectValue(reflect.ValueOf(value), budget, nodes, depth)
}

func copyToolReflectValue(v reflect.Value, budget, nodes *int, depth int) (any, bool) {
	if (v.Kind() == reflect.Map || v.Kind() == reflect.Slice) && v.IsNil() {
		return v.Interface(), false
	}
	switch v.Kind() {
	case reflect.String:
		return copyToolString(v.String(), budget)
	case reflect.Map:
		return copyToolMap(v, budget, nodes, depth)
	case reflect.Slice, reflect.Array:
		return copyToolArray(v, budget, nodes, depth)
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		*budget -= 32
		return v.Interface(), *budget < 0
	default:
		return nil, true
	}
}

func copyToolArray(value reflect.Value, budget, nodes *int, depth int) (any, bool) {
	if value.Len() > *nodes {
		return nil, true
	}
	items := make([]any, 0, value.Len())
	truncated := false
	for i := 0; i < value.Len(); i++ {
		item, cut := copyToolValue(value.Index(i).Interface(), budget, nodes, depth+1)
		items = append(items, item)
		truncated = truncated || cut
		if *budget < 8 || *nodes == 0 {
			return items, i+1 < value.Len() || truncated
		}
	}
	return items, truncated
}

func copyToolString(value string, budget *int) (string, bool) {
	if *budget < 2 {
		return "", true
	}
	remaining := *budget - 2
	end := 0
	for end < len(value) {
		r, size := utf8.DecodeRuneInString(value[end:])
		cost := toolJSONRuneBytes(r, size)
		if cost > remaining {
			break
		}
		remaining -= cost
		end += size
	}
	*budget = remaining
	return strings.Clone(value[:end]), end < len(value)
}

func toolJSONRuneBytes(r rune, size int) int {
	switch r {
	case '"', '\\', '\b', '\f', '\n', '\r', '\t':
		return 2
	case '<', '>', '&', '\u2028', '\u2029':
		return 6
	default:
		if r < 0x20 || (r == utf8.RuneError && size == 1) {
			return 6
		}
		return size
	}
}

func copyToolMap(value reflect.Value, budget, nodes *int, depth int) (any, bool) {
	if value.Type().Key().Kind() != reflect.String || value.Len() > *nodes {
		return nil, true
	}
	keys := value.MapKeys()
	keyBytes := 0
	for _, key := range keys {
		if len(key.String()) > (*budget-keyBytes)/6 {
			return nil, true
		}
		keyBytes += 6 * len(key.String())
	}
	*budget -= keyBytes
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	result := make(map[string]any, len(keys))
	truncated := false
	for i, key := range keys {
		item, cut := copyToolValue(value.MapIndex(key).Interface(), budget, nodes, depth+1)
		result[key.String()] = item
		truncated = truncated || cut
		if *budget < 8 || *nodes == 0 {
			return result, i+1 < len(keys) || truncated
		}
	}
	return result, truncated
}

func toolContentsFit(contents []ToolCallContentItem) bool {
	if len(contents) > toolValueNodes {
		return false
	}
	remaining := toolValueBytes
	for _, item := range contents {
		remaining -= len(item.Type) + len(item.Path) + len(item.NewText) + len(item.TerminalID) + 256
		if item.OldText != nil {
			remaining -= len(*item.OldText)
		}
		if c := item.Content; c != nil {
			remaining -= len(c.Type) + len(c.Text) + len(c.Data) + len(c.MimeType) + len(c.URI) + len(c.Name) + len(c.Title) + len(c.Description)
		}
		if remaining < 0 {
			return false
		}
	}
	encoded, err := json.Marshal(contents)
	return err == nil && len(encoded) <= toolValueBytes
}
