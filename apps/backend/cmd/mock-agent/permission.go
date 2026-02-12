package main

import (
	"bufio"
	"encoding/json"
	"fmt"
)

// requestPermission sends a control_request for tool permission and waits for the response.
// Returns true if permission was granted, false if denied.
func requestPermission(enc *json.Encoder, scanner *bufio.Scanner, toolName, toolUseID string, input map[string]any) bool {
	requestID := fmt.Sprintf("mock-perm-%s-%s", toolName, toolUseID)

	// Emit control_request to stdout
	_ = enc.Encode(ControlRequestMsg{
		Type:      TypeControlRequest,
		RequestID: requestID,
		Request: ControlRequestBody{
			Subtype:   "can_use_tool",
			ToolName:  toolName,
			Input:     input,
			ToolUseID: toolUseID,
		},
	})

	// Read stdin for control_response
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg IncomingMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		if msg.Type == TypeControlResponse && msg.Response != nil {
			if msg.Response.Result != nil {
				return msg.Response.Result.Behavior == "allow"
			}
			// Error response means denied
			return false
		}
	}

	return false
}
