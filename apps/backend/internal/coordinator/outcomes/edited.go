package outcomes

import (
	"encoding/json"
	"reflect"
	"sort"
)

// EditedFields returns the sorted top-level JSON keys whose value differs
// between the proposed spec and the frozen spec. An empty or unreadable frozen
// spec means no edits. Values are compared and dropped, never returned.
func EditedFields(spec, final []byte) []string {
	out := []string{}
	if len(final) == 0 {
		return out
	}
	var before, after map[string]json.RawMessage
	if json.Unmarshal(spec, &before) != nil || json.Unmarshal(final, &after) != nil {
		return out
	}
	for key, a := range after {
		b, ok := before[key]
		if !ok || !jsonEqual(a, b) {
			out = append(out, key)
		}
	}
	for key := range before {
		if _, ok := after[key]; !ok {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return string(a) == string(b)
	}
	return reflect.DeepEqual(x, y)
}
