package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
)

var toolCallCounter int

func nextToolID() string {
	toolCallCounter++
	return fmt.Sprintf("mock_tool_%04d", toolCallCounter)
}

func defaultUsage() *Usage {
	return &Usage{
		InputTokens:  1200,
		OutputTokens: 350,
	}
}

// --- Atomic emitters ---

// emitThinking emits an assistant message with a thinking block.
func emitThinking(enc *json.Encoder, model string) {
	randomDelay(model)
	emitThinkingBlock(enc, "Analyzing the request and considering the best approach...", "")
}

// emitThinkingBlock emits an assistant message with a thinking content block.
func emitThinkingBlock(enc *json.Encoder, thought, parentToolUseID string) {
	_ = enc.Encode(AssistantMsg{
		Type:            TypeAssistant,
		ParentToolUseID: parentToolUseID,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: BlockThinking, Thinking: thought},
			},
			Model: "mock-default",
			Usage: defaultUsage(),
		},
	})
}

// emitTextBlock emits an assistant message with a text content block.
func emitTextBlock(enc *json.Encoder, text, parentToolUseID string) {
	_ = enc.Encode(AssistantMsg{
		Type:            TypeAssistant,
		ParentToolUseID: parentToolUseID,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: BlockText, Text: text},
			},
			Model:      "mock-default",
			StopReason: "end_turn",
			Usage:      defaultUsage(),
		},
	})
}

// emitReadFile emits a Read tool_use followed by a tool_result using a real workspace file.
func emitReadFile(enc *json.Encoder, model string) {
	toolID := nextToolID()
	f := randomFile()
	snippet := readFileSnippet(f.absPath, 30)
	randomDelay(model)

	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{
					Type: BlockToolUse,
					ID:   toolID,
					Name: ToolRead,
					Input: map[string]any{
						"file_path": f.absPath,
					},
				},
			},
			Model:      "mock-default",
			StopReason: "tool_use",
			Usage:      defaultUsage(),
		},
	})

	randomDelay(model)

	_ = enc.Encode(UserMsg{
		Type: TypeUser,
		Message: UserMsgBody{
			Role: "user",
			Content: []ContentBlock{
				{
					Type:      BlockToolResult,
					ToolUseID: toolID,
					Content:   snippet,
				},
			},
		},
	})
}

// emitEditFile emits an Edit tool_use with a real file and real content fragment.
func emitEditFile(enc *json.Encoder, scanner *bufio.Scanner, model string) {
	toolID := nextToolID()
	f := randomFile()
	oldStr, newStr := pickEditableFragment(f.absPath)
	randomDelay(model)

	input := map[string]any{
		"file_path":  f.absPath,
		"old_string": oldStr,
		"new_string": newStr,
	}

	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{
					Type:  BlockToolUse,
					ID:    toolID,
					Name:  ToolEdit,
					Input: input,
				},
			},
			Model:      "mock-default",
			StopReason: "tool_use",
			Usage:      defaultUsage(),
		},
	})

	allowed := requestPermission(enc, scanner, ToolEdit, toolID, input)

	if allowed {
		_ = enc.Encode(UserMsg{
			Type: TypeUser,
			Message: UserMsgBody{
				Role: "user",
				Content: []ContentBlock{
					{
						Type:      BlockToolResult,
						ToolUseID: toolID,
						Content:   "File edited successfully: " + f.absPath,
					},
				},
			},
		})
	} else {
		emitTextBlock(enc, "Permission denied for Edit, skipping.", "")
	}
}

// emitShellExec emits a Bash tool_use with permission request, then tool_result.
func emitShellExec(enc *json.Encoder, scanner *bufio.Scanner, model string) {
	toolID := nextToolID()
	randomDelay(model)

	input := map[string]any{
		"command":     "go test ./...",
		"description": "Run all tests",
	}

	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{
					Type:  BlockToolUse,
					ID:    toolID,
					Name:  ToolBash,
					Input: input,
				},
			},
			Model:      "mock-default",
			StopReason: "tool_use",
			Usage:      defaultUsage(),
		},
	})

	allowed := requestPermission(enc, scanner, ToolBash, toolID, input)

	if allowed {
		_ = enc.Encode(UserMsg{
			Type: TypeUser,
			Message: UserMsgBody{
				Role: "user",
				Content: []ContentBlock{
					{
						Type:      BlockToolResult,
						ToolUseID: toolID,
						Content:   "ok  \tgithub.com/example/project\t0.042s\nPASS",
					},
				},
			},
		})
	} else {
		emitTextBlock(enc, "Permission denied for Bash, skipping.", "")
	}
}

// emitCodeSearch emits a Grep tool_use using real workspace file paths.
func emitCodeSearch(enc *json.Encoder, model string) {
	toolID := nextToolID()
	randomDelay(model)

	searchPatterns := []string{"func ", "import ", "TODO", "return ", "error", "type "}
	pattern := searchPatterns[toolCallCounter%len(searchPatterns)]

	f := randomFile()
	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{
					Type: BlockToolUse,
					ID:   toolID,
					Name: ToolGrep,
					Input: map[string]any{
						"pattern": pattern,
						"path":    f.absPath,
					},
				},
			},
			Model:      "mock-default",
			StopReason: "tool_use",
			Usage:      defaultUsage(),
		},
	})

	randomDelay(model)

	// Build realistic search results using real file paths
	paths := randomFilePaths(3)
	var results []string
	for i, p := range paths {
		results = append(results, fmt.Sprintf("%s:%d:%s found here", p, (i+1)*10, strings.TrimSpace(pattern)))
	}

	_ = enc.Encode(UserMsg{
		Type: TypeUser,
		Message: UserMsgBody{
			Role: "user",
			Content: []ContentBlock{
				{
					Type:      BlockToolResult,
					ToolUseID: toolID,
					Content:   strings.Join(results, "\n"),
				},
			},
		},
	})
}

// emitSubagent emits a Task tool_use with nested child messages using real files.
func emitSubagent(enc *json.Encoder, scanner *bufio.Scanner, model string) {
	taskToolID := nextToolID()
	randomDelay(model)

	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{
					Type: BlockToolUse,
					ID:   taskToolID,
					Name: ToolTask,
					Input: map[string]any{
						"description": "Explore codebase",
						"prompt":      "Find all files and summarize the project structure",
					},
				},
			},
			Model:      "mock-default",
			StopReason: "tool_use",
			Usage:      defaultUsage(),
		},
	})

	randomDelay(model)
	_ = enc.Encode(SystemMsg{Type: TypeSystem, SessionID: sessionID, SessionStatus: "active"})
	randomDelay(model)
	emitThinkingBlock(enc, "Exploring the project structure...", taskToolID)
	randomDelay(model)

	paths := randomFilePaths(5)
	emitTextBlock(enc, fmt.Sprintf("Found %d files. The project structure looks well-organized.", len(paths)), taskToolID)
	randomDelay(model)
	emitSubagentChildGlob(enc, taskToolID, model, paths)
	emitSubagentCompletion(enc, taskToolID, model, paths)
}

func emitSubagentChildGlob(enc *json.Encoder, taskToolID, model string, paths []string) {
	childToolID := nextToolID()
	_ = enc.Encode(AssistantMsg{
		Type:            TypeAssistant,
		ParentToolUseID: taskToolID,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{
					Type:  BlockToolUse,
					ID:    childToolID,
					Name:  ToolGlob,
					Input: map[string]any{"pattern": "**/*"},
				},
			},
			Model:      "mock-default",
			StopReason: "tool_use",
			Usage:      defaultUsage(),
		},
	})
	randomDelay(model)
	_ = enc.Encode(UserMsg{
		Type:            TypeUser,
		ParentToolUseID: taskToolID,
		Message: UserMsgBody{
			Role:    "user",
			Content: []ContentBlock{{Type: BlockToolResult, ToolUseID: childToolID, Content: strings.Join(paths, "\n")}},
		},
	})
	randomDelay(model)
}

// emitSubagentCompletion emits the final text block and tool result for a subagent task.
func emitSubagentCompletion(enc *json.Encoder, taskToolID, model string, paths []string) {
	emitTextBlock(enc, "Project structure analysis complete.", taskToolID)
	randomDelay(model)

	_ = enc.Encode(UserMsg{
		Type: TypeUser,
		Message: UserMsgBody{
			Role: "user",
			Content: []ContentBlock{
				{
					Type:      BlockToolResult,
					ToolUseID: taskToolID,
					Content:   fmt.Sprintf("Subagent completed: Found %d files across the project.", len(paths)),
				},
			},
		},
	})
}

// emitTodo emits a TodoWrite tool_use followed by a tool_result.
func emitTodo(enc *json.Encoder, model string) {
	toolID := nextToolID()
	randomDelay(model)

	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{
					Type: BlockToolUse,
					ID:   toolID,
					Name: ToolTodoWrite,
					Input: map[string]any{
						"todos": []map[string]any{
							{"id": "1", "content": "Review code changes", "status": "in_progress"},
							{"id": "2", "content": "Run tests", "status": "pending"},
							{"id": "3", "content": "Update documentation", "status": "pending"},
						},
					},
				},
			},
			Model:      "mock-default",
			StopReason: "tool_use",
			Usage:      defaultUsage(),
		},
	})

	randomDelay(model)

	_ = enc.Encode(UserMsg{
		Type: TypeUser,
		Message: UserMsgBody{
			Role: "user",
			Content: []ContentBlock{
				{
					Type:      BlockToolResult,
					ToolUseID: toolID,
					Content:   "Todo list updated: 3 items (1 in progress, 2 pending)",
				},
			},
		},
	})
}

// emitMermaidSequence emits a rich markdown message containing mermaid diagrams.
func emitMermaidSequence(enc *json.Encoder, model string) {
	emitThinking(enc, model)
	randomDelay(model)

	emitTextBlock(enc, "Here's an overview of the system architecture with diagrams:\n\n"+
		"## System Flow\n\n"+
		"The following flowchart shows the request processing pipeline:\n\n"+
		"```mermaid\n"+
		"flowchart TD\n"+
		"    A([Client Request]) --> B{Auth Check}\n"+
		"    B -->|Valid| C[API Gateway]\n"+
		"    B -->|Invalid| D[401 Unauthorized]\n"+
		"    C --> E[Load Balancer]\n"+
		"    E --> F[Service A]\n"+
		"    E --> G[Service B]\n"+
		"    F --> H[(Database)]\n"+
		"    G --> H\n"+
		"    H --> I[Response Builder]\n"+
		"    I --> J([Client Response])\n"+
		"```\n\n"+
		"## Sequence Diagram\n\n"+
		"Here's how the authentication flow works between components:\n\n"+
		"```mermaid\n"+
		"sequenceDiagram\n"+
		"    participant U as User\n"+
		"    participant FE as Frontend\n"+
		"    participant API as API Server\n"+
		"    participant DB as Database\n"+
		"    U->>FE: Login Request\n"+
		"    FE->>API: POST /auth/login\n"+
		"    API->>DB: Verify Credentials\n"+
		"    DB-->>API: User Record\n"+
		"    API-->>FE: JWT Token\n"+
		"    FE-->>U: Redirect to Dashboard\n"+
		"```\n\n"+
		"### Key Points\n\n"+
		"- All requests go through the **API Gateway** for rate limiting\n"+
		"- Authentication uses **JWT tokens** with a 24h expiry\n"+
		"- Services communicate via `gRPC` internally\n"+
		"- Database connections use a **connection pool** (max 50)\n", "")
}

// emitWebFetch emits a WebFetch tool_use followed by a tool_result.
func emitWebFetch(enc *json.Encoder, model string) {
	toolID := nextToolID()
	randomDelay(model)

	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{
					Type: BlockToolUse,
					ID:   toolID,
					Name: ToolWebFetch,
					Input: map[string]any{
						"url":    "https://example.com/api/docs",
						"prompt": "Extract the API endpoints and their descriptions",
					},
				},
			},
			Model:      "mock-default",
			StopReason: "tool_use",
			Usage:      defaultUsage(),
		},
	})

	randomDelay(model)

	_ = enc.Encode(UserMsg{
		Type: TypeUser,
		Message: UserMsgBody{
			Role: "user",
			Content: []ContentBlock{
				{
					Type:      BlockToolResult,
					ToolUseID: toolID,
					Content:   "API Documentation:\n- GET /api/v1/users - List all users\n- POST /api/v1/users - Create a new user\n- GET /api/v1/users/:id - Get user by ID\n- PUT /api/v1/users/:id - Update user\n- DELETE /api/v1/users/:id - Delete user",
				},
			},
		},
	})
}
