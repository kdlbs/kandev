package models

import (
	"encoding/json"
	"strings"
)

// ParsePayload retains the existing Office string-map decoding semantics,
// including partial data on malformed fields and the null/empty distinction.
func ParsePayload(payloadJSON string) map[string]string {
	result := make(map[string]string)
	if payloadJSON == "" || payloadJSON == "{}" {
		return result
	}
	_ = json.Unmarshal([]byte(payloadJSON), &result)
	return result
}

func PayloadTaskID(payload string) string { return strings.TrimSpace(ParsePayload(payload)["task_id"]) }
