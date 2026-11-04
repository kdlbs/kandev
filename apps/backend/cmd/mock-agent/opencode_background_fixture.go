package main

import (
	"fmt"
	"strconv"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

// openCodeBackgroundJobs are the deterministic launches the OpenCode fixture
// reports. They mirror the investigated shapes: ordinary polling and test
// commands that OpenCode started with background=true.
var openCodeBackgroundJobs = []struct {
	title   string
	command string
	shellID string
}{
	{"CI poller", "gh pr checks 4821 --watch", "ses_e2e_shell_ci"},
	{"PR await", "./scripts/pr-await 4821", "ses_e2e_shell_await"},
	{"Local tests", "go test ./...", "ses_e2e_shell_tests"},
}

// emitOpenCodeBackgroundFixture emits the captured OpenCode 2.2.0 launch
// sequence: one execute tool call per job with background=true, then a
// completed launch result carrying metadata.shellID and metadata.status=running.
// The foreground prompt then returns normally. A runtime that ends delivery with
// the prompt cannot send a later job outcome, so Kandev must keep the launches
// inspectable and settle them as unknown rather than inventing completion.
//
// Usage: /opencode-background-fixture [count], default 3 (max 3).
func emitOpenCodeBackgroundFixture(e *emitter, cmd string) {
	count := len(openCodeBackgroundJobs)
	if parts := strings.Fields(cmd); len(parts) >= 2 {
		if parsed, err := strconv.Atoi(parts[1]); err == nil {
			count = min(max(parsed, 1), len(openCodeBackgroundJobs))
		}
	}

	for _, job := range openCodeBackgroundJobs[:count] {
		id := nextToolID()
		e.startTool(id, job.title, acp.ToolKindExecute, map[string]any{
			"command":    job.command,
			"background": true,
		})
		e.completeTool(id, map[string]any{
			"title":  job.title,
			"output": "Background process started",
			"metadata": map[string]any{
				"shellID": job.shellID,
				"status":  "running",
			},
		})
	}

	e.text(fmt.Sprintf(
		"Started %d background jobs. I will report when they finish.",
		count,
	))
}
