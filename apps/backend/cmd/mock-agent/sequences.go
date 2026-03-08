package main

import (
	"fmt"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

// scriptState holds mutable state shared across script commands and sequence emitters.
type scriptState struct {
	toolCallCounter int
	lastToolID      string
}

// state is the process-wide script state. Use resetState() in tests
// to get a clean starting point instead of touching fields directly.
var state = &scriptState{}

func resetState() {
	state = &scriptState{}
}

func nextToolID() acp.ToolCallId {
	state.toolCallCounter++
	id := fmt.Sprintf("mock_tool_%04d", state.toolCallCounter)
	state.lastToolID = id
	return acp.ToolCallId(id)
}

// --- Atomic emitters ---

// emitThinking emits a thinking block with random delay.
func emitThinking(e *emitter, model string) {
	randomDelay(model)
	e.thought("Analyzing the request and considering the best approach...")
}

// emitReadFile emits a Read tool call using a real workspace file.
func emitReadFile(e *emitter, model string) {
	toolID := nextToolID()
	f := randomFile()
	snippet := readFileSnippet(f.absPath, 30)
	randomDelay(model)

	e.startTool(toolID, "Read "+f.relPath, acp.ToolKindRead,
		map[string]any{"file_path": f.absPath},
		acp.ToolCallLocation{Path: f.absPath})

	randomDelay(model)
	e.completeTool(toolID, map[string]any{"content": snippet})
}

// emitEditFile emits an Edit tool call with permission request.
func emitEditFile(e *emitter, model string) {
	toolID := nextToolID()
	f := randomFile()
	oldStr, newStr := pickEditableFragment(f.absPath)
	randomDelay(model)

	input := map[string]any{
		"file_path":  f.absPath,
		"old_string": oldStr,
		"new_string": newStr,
	}

	e.startTool(toolID, "Edit "+f.relPath, acp.ToolKindEdit, input,
		acp.ToolCallLocation{Path: f.absPath})

	if e.requestPermission(toolID, "Edit "+f.relPath, acp.ToolKindEdit, input) {
		e.completeTool(toolID, map[string]any{"result": "File edited successfully: " + f.absPath})
	} else {
		e.completeTool(toolID, map[string]any{"result": "Permission denied for Edit"})
		e.text("Permission denied for Edit, skipping.")
	}
}

// emitShellExec emits a Bash tool call with permission request.
func emitShellExec(e *emitter, model string) {
	toolID := nextToolID()
	randomDelay(model)

	input := map[string]any{
		"command":     "go test ./...",
		"description": "Run all tests",
	}

	e.startTool(toolID, "Run tests", acp.ToolKindExecute, input)

	if e.requestPermission(toolID, "Run tests", acp.ToolKindExecute, input) {
		e.completeTool(toolID, map[string]any{"output": "ok  \tgithub.com/example/project\t0.042s\nPASS"})
	} else {
		e.completeTool(toolID, map[string]any{"output": "Permission denied"})
		e.text("Permission denied for Bash, skipping.")
	}
}

// emitCodeSearch emits a Grep tool call using real workspace file paths.
func emitCodeSearch(e *emitter, model string) {
	toolID := nextToolID()
	randomDelay(model)

	searchPatterns := []string{"func ", "import ", "TODO", "return ", "error", "type "}
	pattern := searchPatterns[state.toolCallCounter%len(searchPatterns)]

	f := randomFile()
	e.startTool(toolID, "Search for \""+pattern+"\"", acp.ToolKindSearch,
		map[string]any{"pattern": pattern, "path": f.absPath})

	randomDelay(model)

	paths := randomFilePaths(3)
	var results []string
	for i, p := range paths {
		results = append(results, fmt.Sprintf("%s:%d:%s found here", p, (i+1)*10, strings.TrimSpace(pattern)))
	}
	e.completeTool(toolID, map[string]any{"matches": strings.Join(results, "\n")})
}

// emitSubagent emits a Task tool call with child tool calls.
func emitSubagent(e *emitter, model string) {
	taskToolID := nextToolID()
	randomDelay(model)

	e.startTool(taskToolID, "Explore codebase", acp.ToolKindOther,
		map[string]any{
			"description": "Explore codebase",
			"prompt":      "Find all files and summarize the project structure",
		})

	randomDelay(model)
	e.thought("Exploring the project structure...")
	randomDelay(model)

	paths := randomFilePaths(5)
	e.text(fmt.Sprintf("Found %d files. The project structure looks well-organized.", len(paths)))
	randomDelay(model)

	// Child glob tool call
	childToolID := nextToolID()
	e.startTool(childToolID, "Glob **/*", acp.ToolKindSearch,
		map[string]any{"pattern": "**/*"})
	randomDelay(model)
	e.completeTool(childToolID, map[string]any{"files": strings.Join(paths, "\n")})
	randomDelay(model)

	e.text("Project structure analysis complete.")
	randomDelay(model)
	e.completeTool(taskToolID, map[string]any{
		"result": fmt.Sprintf("Subagent completed: Found %d files across the project.", len(paths)),
	})
}

// emitTodo emits a TodoWrite tool call.
func emitTodo(e *emitter, model string) {
	toolID := nextToolID()
	randomDelay(model)

	e.startTool(toolID, "Update todo list", acp.ToolKindOther, map[string]any{
		"todos": []map[string]any{
			{"id": "1", "content": "Review code changes", "status": "in_progress"},
			{"id": "2", "content": "Run tests", "status": "pending"},
			{"id": "3", "content": "Update documentation", "status": "pending"},
		},
	})

	randomDelay(model)
	e.completeTool(toolID, map[string]any{"result": "Todo list updated: 3 items (1 in progress, 2 pending)"})
}

// emitMermaidSequence emits a rich markdown message containing mermaid diagrams.
func emitMermaidSequence(e *emitter, model string) {
	emitThinking(e, model)
	randomDelay(model)

	e.text("Here's an overview of the system architecture with diagrams:\n\n" +
		"## System Flow\n\n" +
		"The following flowchart shows the request processing pipeline:\n\n" +
		"```mermaid\n" +
		"flowchart TD\n" +
		"    A([Client Request]) --> B{Auth Check}\n" +
		"    B -->|Valid| C[API Gateway]\n" +
		"    B -->|Invalid| D[401 Unauthorized]\n" +
		"    C --> E[Load Balancer]\n" +
		"    E --> F[Service A]\n" +
		"    E --> G[Service B]\n" +
		"    F --> H[(Database)]\n" +
		"    G --> H\n" +
		"    H --> I[Response Builder]\n" +
		"    I --> J([Client Response])\n" +
		"```\n\n" +
		"## Sequence Diagram\n\n" +
		"Here's how the authentication flow works between components:\n\n" +
		"```mermaid\n" +
		"sequenceDiagram\n" +
		"    participant U as User\n" +
		"    participant FE as Frontend\n" +
		"    participant API as API Server\n" +
		"    participant DB as Database\n" +
		"    U->>FE: Login Request\n" +
		"    FE->>API: POST /auth/login\n" +
		"    API->>DB: Verify Credentials\n" +
		"    DB-->>API: User Record\n" +
		"    API-->>FE: JWT Token\n" +
		"    FE-->>U: Redirect to Dashboard\n" +
		"```\n\n" +
		"### Key Points\n\n" +
		"- All requests go through the **API Gateway** for rate limiting\n" +
		"- Authentication uses **JWT tokens** with a 24h expiry\n" +
		"- Services communicate via `gRPC` internally\n" +
		"- Database connections use a **connection pool** (max 50)\n")
}

// emitWebFetch emits a WebFetch tool call.
func emitWebFetch(e *emitter, model string) {
	toolID := nextToolID()
	randomDelay(model)

	e.startTool(toolID, "Fetch API docs", acp.ToolKindFetch, map[string]any{
		"url":    "https://example.com/api/docs",
		"prompt": "Extract the API endpoints and their descriptions",
	})

	randomDelay(model)
	e.completeTool(toolID, map[string]any{
		"content": "API Documentation:\n- GET /api/v1/users - List all users\n- POST /api/v1/users - Create a new user\n- GET /api/v1/users/:id - Get user by ID\n- PUT /api/v1/users/:id - Update user\n- DELETE /api/v1/users/:id - Delete user",
	})
}
