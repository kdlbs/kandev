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
	if !supportedPayload(kind, messageType, norm) {
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
	paths, ok, err := removePayloadFields(kind, body)
	if err != nil {
		return result, err
	}
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
	if err := updateReducedPayload(root, norm, body, kind); err != nil {
		return result, err
	}
	return encodePayloadRemoval(root, paths, metadata, removedAt)
}

func supportedPayload(kind, messageType string, norm rawPayloadObject) bool {
	expected := map[string]string{"shell_exec": "tool_execute", "read_file": "tool_read", "modify_file": "tool_edit", "code_search": "tool_search", "generic": "tool_call", "http_request": "tool_call"}
	return expected[kind] != "" && expected[kind] == messageType && !protectedPayload(norm)
}

func updateReducedPayload(root, norm, body rawPayloadObject, kind string) error {
	encodedBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	norm[kind] = encodedBody
	encodedNorm, err := json.Marshal(norm)
	if err != nil {
		return err
	}
	root["normalized"] = encodedNorm
	return nil
}

func encodePayloadRemoval(root rawPayloadObject, paths []string, metadata []byte, removedAt time.Time) (PayloadReduction, error) {
	marker := map[string]any{"version": 1, "removed_at": removedAt.UTC().Format(time.RFC3339Nano), "removed_fields": paths}
	// Fixed-width numeric notation keeps the receipt size independent of the
	// positive byte count's decimal width, including power-of-ten boundaries.
	marker["removed_bytes"] = json.Number(strconv.FormatFloat(0, 'e', 8, 64))
	markerJSON, err := json.Marshal(marker)
	if err != nil {
		return PayloadReduction{Metadata: metadata}, err
	}
	root[payloadRetentionKey] = markerJSON
	encoded, err := json.Marshal(root)
	if err != nil {
		return PayloadReduction{Metadata: metadata}, err
	}
	delta := int64(len(metadata) - len(encoded))
	if delta <= 0 {
		return PayloadReduction{Metadata: metadata, Reason: "no_payload"}, nil
	}
	marker["removed_bytes"] = json.Number(strconv.FormatFloat(float64(delta), 'e', 8, 64))
	markerJSON, err = json.Marshal(marker)
	if err != nil {
		return PayloadReduction{Metadata: metadata}, err
	}
	root[payloadRetentionKey] = markerJSON
	encoded, err = json.Marshal(root)
	if err != nil {
		return PayloadReduction{Metadata: metadata}, err
	}
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
func removePayloadFields(kind string, body rawPayloadObject) ([]string, bool, error) {
	prefix := "normalized." + kind + "."
	switch kind {
	case "generic":
		paths, ok := removePayloadKeys(body, prefix, "input", "output")
		return paths, ok, nil
	case "http_request":
		paths, ok := removePayloadKeys(body, prefix, "response")
		return paths, ok, nil
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

func removeMutationPayloads(body rawPayloadObject, prefix string) ([]string, bool, error) {
	raw, exists := body["mutations"]
	if !exists {
		return nil, true, nil
	}
	var mutations []rawPayloadObject
	if json.Unmarshal(raw, &mutations) != nil {
		return nil, false, nil
	}
	var paths []string
	for _, mutation := range mutations {
		if mutation == nil {
			return nil, false, nil
		}
		removed, ok := removePayloadKeys(mutation, prefix+"mutations.*.", "content", "old_content", "new_content", "diff")
		if !ok {
			return nil, false, nil
		}
		paths = append(paths, removed...)
	}
	encoded, err := json.Marshal(mutations)
	if err != nil {
		return nil, false, err
	}
	body["mutations"] = encoded
	return paths, true, nil
}

func removeOutputPayloads(kind string, body rawPayloadObject, prefix string) ([]string, bool, error) {
	raw, exists := body["output"]
	if !exists || string(raw) == "null" {
		return nil, true, nil
	}
	output, ok := payloadObject(raw)
	if !ok {
		return nil, false, nil
	}
	fields := map[string][]string{"shell_exec": {"stdout", "stderr"}, "read_file": {"content"}, "code_search": {"files"}}
	paths, ok := removePayloadKeys(output, prefix+"output.", fields[kind]...)
	if !ok {
		return nil, false, nil
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return nil, false, err
	}
	body["output"] = encoded
	return paths, true, nil
}
