package websocket

import (
	"encoding/json"
	"fmt"
	"strings"
)

func replaceJSONRPCID(original []byte, id json.RawMessage) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(original, &raw); err != nil {
		return nil, err
	}
	raw["id"] = append([]byte(nil), id...)
	return json.Marshal(raw)
}

func jsonRPCIDKey(id json.RawMessage) string {
	return strings.TrimSpace(string(id))
}

func jsonRPCStringID(id string) json.RawMessage {
	encoded, _ := json.Marshal(id)
	return encoded
}

func jsonRPCNotification(method string, params any) []byte {
	message, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	return message
}

func jsonRPCResultResponse(id json.RawMessage, result any) []byte {
	encoded, _ := json.Marshal(result)
	message, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": json.RawMessage(encoded)})
	return message
}

func jsonRPCErrorResponseRaw(id json.RawMessage, code int, message string) []byte {
	response, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message},
	})
	return response
}

func marshalBoundedJSON(value any, limit int) ([]byte, error) {
	message, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(message) > limit {
		return nil, fmt.Errorf("JSON frame is %d bytes; maximum is %d", len(message), limit)
	}
	return message, nil
}

func cloneStringAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return make(map[string]any)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return make(map[string]any)
	}
	var output map[string]any
	if err := json.Unmarshal(encoded, &output); err != nil || output == nil {
		return make(map[string]any)
	}
	return output
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	return cloneStringAnyMap(input)
}
