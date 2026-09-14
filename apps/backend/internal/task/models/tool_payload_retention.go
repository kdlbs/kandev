package models

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const ToolPayloadMaxBytes = 32 * 1024 * 1024
const payloadRetentionKey = "payload_retention"
const payloadMalformed = "malformed"

type PayloadReduction struct {
	Metadata     []byte
	RemovedBytes int64
	Reason       string
}
type rawPayloadObject = map[string]json.RawMessage

// ToolPayloadRemoved reports a persisted removal tombstone, including future versions.
func ToolPayloadRemoved(metadata map[string]any) bool {
	_, removed := metadata[payloadRetentionKey]
	return removed
}

func ReduceToolPayload(messageType string, metadata []byte, removedAt time.Time) (PayloadReduction, error) {
	result := PayloadReduction{Metadata: metadata}
	if len(metadata) > ToolPayloadMaxBytes {
		result.Reason = "oversize"
		return result, nil
	}
	root, ok := payloadObject(metadata)
	if !ok {
		result.Reason = payloadMalformed
		return result, nil
	}
	if _, exists := root[payloadRetentionKey]; exists {
		result.Reason = "already_removed"
		return result, nil
	}
	norm, ok := payloadObject(root["normalized"])
	if !ok {
		result.Reason = payloadMalformed
		return result, nil
	}
	var kind string
	_ = json.Unmarshal(norm["kind"], &kind)
	expected := map[string]string{"shell_exec": "tool_execute", "read_file": "tool_read", "modify_file": "tool_edit", "code_search": "tool_search", "generic": "tool_call", "http_request": "tool_call"}
	if expected[kind] == "" || expected[kind] != messageType || protectedPayload(norm) {
		result.Reason = "unsupported"
		return result, nil
	}
	body, ok := payloadObject(norm[kind])
	if !ok {
		result.Reason = payloadMalformed
		return result, nil
	}
	if kind == "generic" && !reducibleGeneric(body, root["result"]) {
		result.Reason = "unsupported"
		return result, nil
	}
	paths, ok := removePayloadFields(kind, body)
	if !ok {
		result.Reason = payloadMalformed
		return result, nil
	}
	if _, exists := root["result"]; exists {
		delete(root, "result")
		paths = append(paths, "result")
	}
	if len(paths) == 0 {
		result.Reason = "no_payload"
		return result, nil
	}
	norm[kind], _ = json.Marshal(body)
	root["normalized"], _ = json.Marshal(norm)
	return encodePayloadRemoval(root, paths, metadata, removedAt)
}

func encodePayloadRemoval(root rawPayloadObject, paths []string, metadata []byte, removedAt time.Time) (PayloadReduction, error) {
	marker := map[string]any{"version": 1, "removed_at": removedAt.UTC().Format(time.RFC3339Nano), "removed_fields": paths}
	// Fixed-width numeric notation keeps the receipt size independent of the
	// positive byte count's decimal width, including power-of-ten boundaries.
	marker["removed_bytes"] = json.Number(strconv.FormatFloat(0, 'e', 8, 64))
	root[payloadRetentionKey], _ = json.Marshal(marker)
	encoded, _ := json.Marshal(root)
	delta := int64(len(metadata) - len(encoded))
	if delta <= 0 {
		return PayloadReduction{Metadata: metadata, Reason: "no_payload"}, nil
	}
	marker["removed_bytes"] = json.Number(strconv.FormatFloat(float64(delta), 'e', 8, 64))
	root[payloadRetentionKey], _ = json.Marshal(marker)
	encoded, _ = json.Marshal(root)
	delta = int64(len(metadata) - len(encoded))
	if delta <= 0 {
		return PayloadReduction{Metadata: metadata, Reason: "no_payload"}, nil
	}
	return PayloadReduction{Metadata: encoded, RemovedBytes: delta}, nil
}

func payloadObject(raw []byte) (rawPayloadObject, bool) {
	var obj rawPayloadObject
	err := json.Unmarshal(raw, &obj)
	return obj, err == nil && obj != nil
}
func protectedPayload(norm rawPayloadObject) bool {
	for _, key := range []string{"background_work", "monitor", "create_task", "subagent_task", "show_plan", "manage_todos"} {
		if _, ok := norm[key]; ok {
			return true
		}
	}
	return false
}
func reducibleGeneric(body rawPayloadObject, result json.RawMessage) bool {
	var name string
	if json.Unmarshal(body["name"], &name) != nil || name == "" {
		return false
	}
	// MCP control tools and provider plan/task tools have durable result contracts.
	lower := strings.ToLower(name)
	for _, word := range []string{"kandev", "plan", "todo", "task", "agent", "attachment", "resource", "canvas", "question", "permission"} {
		if strings.Contains(lower, word) {
			return false
		}
	}
	return plainPayloadResult(body["output"]) && plainPayloadResult(result)
}
func plainPayloadResult(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return true
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	// Providers can wrap structured MCP envelopes in result text.
	var structured any
	if json.Unmarshal([]byte(value), &structured) == nil {
		switch structured.(type) {
		case map[string]any, []any:
			return false
		}
	}
	return true
}
func removePayloadFields(kind string, body rawPayloadObject) ([]string, bool) {
	prefix := "normalized." + kind + "."
	switch kind {
	case "generic":
		return removePayloadKeys(body, prefix, "input", "output")
	case "http_request":
		return removePayloadKeys(body, prefix, "response")
	case "modify_file":
		return removeMutationPayloads(body, prefix)
	default:
		return removeOutputPayloads(kind, body, prefix)
	}
}

func removePayloadKeys(obj rawPayloadObject, prefix string, keys ...string) ([]string, bool) {
	var paths []string
	for _, key := range keys {
		raw, exists := obj[key]
		if !exists {
			continue
		}
		if key != "input" {
			var target any = new(string)
			if key == "files" {
				target = new([]string)
			}
			if json.Unmarshal(raw, target) != nil {
				return nil, false
			}
		}
		delete(obj, key)
		paths = append(paths, prefix+key)
	}
	return paths, true
}

func removeMutationPayloads(body rawPayloadObject, prefix string) ([]string, bool) {
	raw, exists := body["mutations"]
	if !exists {
		return nil, true
	}
	var mutations []rawPayloadObject
	if json.Unmarshal(raw, &mutations) != nil {
		return nil, false
	}
	var paths []string
	for _, mutation := range mutations {
		if mutation == nil {
			return nil, false
		}
		removed, ok := removePayloadKeys(mutation, prefix+"mutations.*.", "content", "old_content", "new_content", "diff")
		if !ok {
			return nil, false
		}
		paths = append(paths, removed...)
	}
	body["mutations"], _ = json.Marshal(mutations)
	return paths, true
}

func removeOutputPayloads(kind string, body rawPayloadObject, prefix string) ([]string, bool) {
	raw, exists := body["output"]
	if !exists || string(raw) == "null" {
		return nil, true
	}
	output, ok := payloadObject(raw)
	if !ok {
		return nil, false
	}
	fields := map[string][]string{"shell_exec": {"stdout", "stderr"}, "read_file": {"content"}, "code_search": {"files"}}
	paths, ok := removePayloadKeys(output, prefix+"output.", fields[kind]...)
	if !ok {
		return nil, false
	}
	body["output"], _ = json.Marshal(output)
	return paths, true
}
