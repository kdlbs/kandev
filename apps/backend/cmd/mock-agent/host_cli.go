package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// mockCLIVersion is what `mock-agent --version` prints. E2E tests assert it
// on the Agents settings card.
const mockCLIVersion = "9.9.9"

// runHostCLICommand handles the vendor-CLI shaped entry points that the
// hostcli package drives: `--version` and `app-server`. It reports whether
// one of them ran so main can return.
func runHostCLICommand(args []string, stdin io.Reader, stdout io.Writer) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "--version":
		_, _ = fmt.Fprintf(stdout, "mock-agent %s\n", mockCLIVersion)
		return true
	case "app-server":
		runMockAppServer(stdin, stdout)
		return true
	}
	return false
}

// runMockAppServer speaks the subset of the Codex app-server JSON-RPC
// protocol that hostcli.ListCodexModels uses: initialize, initialized, and
// model/list. The advertised ids are a subset of the ACP mock catalogue so
// merged lists in E2E keep their existing entries.
func runMockAppServer(stdin io.Reader, stdout io.Writer) {
	type request struct {
		ID     *int            `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil || req.ID == nil {
			continue
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"userAgent": "mock-agent/" + mockCLIVersion}
		case "model/list":
			result = map[string]any{
				"data": []map[string]any{
					{
						"id": modelFast, "model": modelFast, "displayName": "Mock Fast (CLI)",
						"description": "Fast mock model listed by the mock CLI", "hidden": false, "isDefault": true,
						"defaultReasoningEffort": reasoningEffortMed,
						"supportedReasoningEfforts": []map[string]string{
							{"reasoningEffort": reasoningEffortLow, "description": "Fast responses"},
							{"reasoningEffort": reasoningEffortMed, "description": "Balanced"},
							{"reasoningEffort": reasoningEffortHigh, "description": "Deep reasoning"},
						},
					},
					{
						"id": modelSmart, "model": modelSmart, "displayName": "Mock Smart (CLI)",
						"description": "Smart mock model listed by the mock CLI", "hidden": false, "isDefault": false,
					},
					{
						"id": "mock-hidden", "model": "mock-hidden", "displayName": "Hidden", "hidden": true,
					},
				},
				"nextCursor": nil,
			}
		default:
			response := map[string]any{"id": *req.ID, "error": map[string]any{"code": -32601, "message": "method not found"}}
			writeJSONLine(stdout, response)
			continue
		}
		writeJSONLine(stdout, map[string]any{"id": *req.ID, "result": result})
	}
}

func writeJSONLine(w io.Writer, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = w.Write(append(data, '\n'))
}
