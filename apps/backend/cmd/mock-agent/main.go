// Package main implements a mock agent binary that speaks the claude-code
// stream-json protocol over stdin/stdout. It generates simulated responses
// for rapid feature testing and e2e web app tests.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// sessionID is a unique identifier for this mock-agent process instance.
// Each session spawns its own process, so using PID ensures uniqueness
// across parallel sessions.
var sessionID = fmt.Sprintf("mock-session-%d", os.Getpid())

func main() {
	model := parseModelFlag()

	scanner := bufio.NewScanner(os.Stdin)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg IncomingMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case TypeControlRequest:
			handleControlRequest(enc, msg)
		case TypeUser:
			if msg.Message != nil {
				handleUserPrompt(enc, scanner, msg.Message.Content, model)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "mock-agent: scanner error: %v\n", err)
		os.Exit(1)
	}
}

// parseModelFlag extracts --model value from command line args.
func parseModelFlag() string {
	return parseModelFromArgs(os.Args)
}

// parseModelFromArgs extracts --model value from the given args slice.
func parseModelFromArgs(args []string) string {
	for i, arg := range args[1:] {
		if arg == "--model" && i+1 < len(args)-1 {
			return args[i+2]
		}
		if strings.HasPrefix(arg, "--model=") {
			return strings.TrimPrefix(arg, "--model=")
		}
	}
	return "mock-default"
}

// handleControlRequest responds to control requests from the backend.
func handleControlRequest(enc *json.Encoder, msg IncomingMessage) {
	// We only handle the initialize request from the backend
	if msg.RequestID != "" {
		resp := ControlResponseMsg{
			Type: TypeControlResponse,
			Response: ControlResponseBody{
				Subtype:   "success",
				RequestID: msg.RequestID,
				Response: &InitializeResponse{
					Commands: []Command{
						{Name: "all", Description: "Demo all message types"},
						{Name: "error", Description: "Simulate an error result"},
						{Name: "slow", Description: "Random response with slow delays", ArgumentHint: "<duration e.g. 5s, 30s, 2m>"},
						{Name: "thinking", Description: "Extended thinking/reasoning blocks"},
						{Name: "tool:read", Description: "Single file read"},
						{Name: "tool:edit", Description: "Single file edit (with permission)"},
						{Name: "tool:exec", Description: "Single shell command (with permission)"},
						{Name: "tool:search", Description: "Single code search"},
						{Name: "tool:webfetch", Description: "Single web fetch"},
						{Name: "subagent", Description: "Subagent Task with nested child messages"},
						{Name: "todo", Description: "Todo management sequence"},
						{Name: "mermaid", Description: "Rich markdown with mermaid diagrams"},
						{Name: "e2e:simple-message", Description: "E2E: text only, fixed timing"},
						{Name: "e2e:read-and-edit", Description: "E2E: read + edit + text"},
						{Name: "e2e:permission-flow", Description: "E2E: tool requiring permission"},
						{Name: "e2e:error", Description: "E2E: error result"},
						{Name: "e2e:subagent", Description: "E2E: subagent with child messages"},
						{Name: "e2e:all-tools", Description: "E2E: one of each tool type"},
						{Name: "e2e:multi-turn", Description: "E2E: minimal multi-turn response"},
					},
					Agents: []string{"Bash", "Read", "Edit", "Grep", "Glob", "Task"},
				},
			},
		}
		_ = enc.Encode(resp)
	}
}
