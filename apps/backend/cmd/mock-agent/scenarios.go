package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
)

// Predefined e2e test scenarios with fixed timing for deterministic test assertions.

// emitPredefinedScenario dispatches to a named e2e scenario.
func emitPredefinedScenario(enc *json.Encoder, scanner *bufio.Scanner, name string) {
	switch name {
	case "simple-message":
		scenarioSimpleMessage(enc)
	case "read-and-edit":
		scenarioReadAndEdit(enc, scanner)
	case "permission-flow":
		scenarioPermissionFlow(enc, scanner)
	case "error":
		scenarioError(enc)
	case "subagent":
		scenarioSubagent(enc, scanner)
	case "all-tools":
		scenarioAllTools(enc, scanner)
	case "multi-turn":
		scenarioMultiTurn(enc)
	default:
		emitTextBlock(enc, "Unknown e2e scenario: "+name+". Available: simple-message, read-and-edit, permission-flow, error, subagent, all-tools, multi-turn", "")
	}
}

// scenarioSimpleMessage: text only with fixed 100ms delays.
func scenarioSimpleMessage(enc *json.Encoder) {
	fixedDelay(100)
	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: BlockThinking, Thinking: "Processing the request..."},
			},
			Model: "mock-default",
			Usage: defaultUsage(),
		},
	})

	fixedDelay(100)
	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: BlockText, Text: "This is a simple mock response for e2e testing."},
			},
			Model:      "mock-default",
			StopReason: "end_turn",
			Usage:      defaultUsage(),
		},
	})
}

// scenarioReadAndEdit: read -> edit -> text with fixed delays, using real files.
func scenarioReadAndEdit(enc *json.Encoder, scanner *bufio.Scanner) {
	f := randomFile()
	snippet := readFileSnippet(f.absPath, 20)
	oldStr, newStr := pickEditableFragment(f.absPath)
	fixedDelay(50)

	// Read file
	readID := nextToolID()
	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: BlockToolUse, ID: readID, Name: ToolRead, Input: map[string]any{"file_path": f.absPath}},
			},
			Model:      "mock-default",
			StopReason: "tool_use",
			Usage:      defaultUsage(),
		},
	})

	fixedDelay(50)
	_ = enc.Encode(UserMsg{
		Type: TypeUser,
		Message: UserMsgBody{
			Role: "user",
			Content: []ContentBlock{
				{Type: BlockToolResult, ToolUseID: readID, Content: snippet},
			},
		},
	})

	fixedDelay(50)

	// Edit file (with permission)
	editID := nextToolID()
	editInput := map[string]any{
		"file_path":  f.absPath,
		"old_string": oldStr,
		"new_string": newStr,
	}
	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: BlockToolUse, ID: editID, Name: ToolEdit, Input: editInput},
			},
			Model:      "mock-default",
			StopReason: "tool_use",
			Usage:      defaultUsage(),
		},
	})

	allowed := requestPermission(enc, scanner, ToolEdit, editID, editInput)

	fixedDelay(50)
	if allowed {
		_ = enc.Encode(UserMsg{
			Type: TypeUser,
			Message: UserMsgBody{
				Role: "user",
				Content: []ContentBlock{
					{Type: BlockToolResult, ToolUseID: editID, Content: "File edited successfully: " + f.absPath},
				},
			},
		})
	} else {
		emitTextBlock(enc, "Edit was denied.", "")
	}

	fixedDelay(50)
	emitTextBlock(enc, "Read and edit scenario complete.", "")
}

// scenarioPermissionFlow: tool requiring permission with fixed delays.
func scenarioPermissionFlow(enc *json.Encoder, scanner *bufio.Scanner) {
	fixedDelay(50)

	bashID := nextToolID()
	bashInput := map[string]any{
		"command":     "echo 'testing permissions'",
		"description": "Test permission flow",
	}

	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: BlockToolUse, ID: bashID, Name: ToolBash, Input: bashInput},
			},
			Model:      "mock-default",
			StopReason: "tool_use",
			Usage:      defaultUsage(),
		},
	})

	allowed := requestPermission(enc, scanner, ToolBash, bashID, bashInput)

	fixedDelay(50)
	if allowed {
		_ = enc.Encode(UserMsg{
			Type: TypeUser,
			Message: UserMsgBody{
				Role: "user",
				Content: []ContentBlock{
					{Type: BlockToolResult, ToolUseID: bashID, Content: "testing permissions"},
				},
			},
		})
		emitTextBlock(enc, "Permission was granted and command executed.", "")
	} else {
		emitTextBlock(enc, "Permission was denied.", "")
	}
}

// scenarioError: error result with fixed delays.
func scenarioError(enc *json.Encoder) {
	fixedDelay(100)
	emitTextBlock(enc, "About to encounter an error...", "")
	fixedDelay(100)
	emitResult(enc, true, "E2E test error: simulated failure")
}

// scenarioSubagent: subagent with child messages and fixed delays.
func scenarioSubagent(enc *json.Encoder, scanner *bufio.Scanner) {
	taskToolID := nextToolID()
	fixedDelay(50)

	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: BlockToolUse, ID: taskToolID, Name: ToolTask, Input: map[string]any{
					"description": "E2E subagent test",
					"prompt":      "Run e2e subagent scenario",
				}},
			},
			Model:      "mock-default",
			StopReason: "tool_use",
			Usage:      defaultUsage(),
		},
	})

	fixedDelay(50)
	_ = enc.Encode(SystemMsg{Type: TypeSystem, SessionID: sessionID, SessionStatus: "active"})

	fixedDelay(50)
	emitTextBlock(enc, "Subagent working on the task...", taskToolID)

	fixedDelay(50)
	_ = enc.Encode(UserMsg{
		Type: TypeUser,
		Message: UserMsgBody{
			Role: "user",
			Content: []ContentBlock{
				{Type: BlockToolResult, ToolUseID: taskToolID, Content: "E2E subagent completed"},
			},
		},
	})

	fixedDelay(50)
	emitTextBlock(enc, "Subagent scenario complete.", "")
}

// scenarioAllTools: one of each tool type with fixed delays, using real files.
func scenarioAllTools(enc *json.Encoder, scanner *bufio.Scanner) {
	used := map[string]bool{}
	readFile := randomFile()
	used[readFile.absPath] = true
	grepFile := randomFileExcluding(used)
	used[grepFile.absPath] = true
	editFile := randomFileExcluding(used)

	fixedDelay(50)
	emitThinkingBlock(enc, "Running all tools...", "")

	scenarioAllToolsReadGrep(enc, readFile, grepFile)
	scenarioAllToolsEditBash(enc, scanner, editFile)

	// WebFetch
	fixedDelay(50)
	webID := nextToolID()
	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role:    "assistant",
			Content: []ContentBlock{{Type: BlockToolUse, ID: webID, Name: ToolWebFetch, Input: map[string]any{"url": "https://example.com", "prompt": "Summarize"}}},
			Model:   "mock-default", StopReason: "tool_use", Usage: defaultUsage(),
		},
	})
	fixedDelay(50)
	_ = enc.Encode(UserMsg{Type: TypeUser, Message: UserMsgBody{Role: "user", Content: []ContentBlock{{Type: BlockToolResult, ToolUseID: webID, Content: "Example page content"}}}})

	fixedDelay(50)
	emitTextBlock(enc, "All tools scenario complete.", "")
}

func scenarioAllToolsReadGrep(enc *json.Encoder, readFile, grepFile fileInfo) {
	// Read
	fixedDelay(50)
	readID := nextToolID()
	snippet := readFileSnippet(readFile.absPath, 20)
	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role:    "assistant",
			Content: []ContentBlock{{Type: BlockToolUse, ID: readID, Name: ToolRead, Input: map[string]any{"file_path": readFile.absPath}}},
			Model:   "mock-default", StopReason: "tool_use", Usage: defaultUsage(),
		},
	})
	fixedDelay(50)
	_ = enc.Encode(UserMsg{Type: TypeUser, Message: UserMsgBody{Role: "user", Content: []ContentBlock{{Type: BlockToolResult, ToolUseID: readID, Content: snippet}}}})

	// Grep
	fixedDelay(50)
	grepID := nextToolID()
	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role:    "assistant",
			Content: []ContentBlock{{Type: BlockToolUse, ID: grepID, Name: ToolGrep, Input: map[string]any{"pattern": "func ", "path": grepFile.absPath}}},
			Model:   "mock-default", StopReason: "tool_use", Usage: defaultUsage(),
		},
	})
	fixedDelay(50)
	paths := randomFilePaths(3)
	var grepResults []string
	for i, p := range paths {
		grepResults = append(grepResults, fmt.Sprintf("%s:%d: func found here", p, (i+1)*10))
	}
	_ = enc.Encode(UserMsg{Type: TypeUser, Message: UserMsgBody{Role: "user", Content: []ContentBlock{{Type: BlockToolResult, ToolUseID: grepID, Content: strings.Join(grepResults, "\n")}}}})
}

func scenarioAllToolsEditBash(enc *json.Encoder, scanner *bufio.Scanner, editFile fileInfo) {
	// Edit (with permission)
	fixedDelay(50)
	editID := nextToolID()
	oldStr, newStr := pickEditableFragment(editFile.absPath)
	editInput := map[string]any{"file_path": editFile.absPath, "old_string": oldStr, "new_string": newStr}
	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role:    "assistant",
			Content: []ContentBlock{{Type: BlockToolUse, ID: editID, Name: ToolEdit, Input: editInput}},
			Model:   "mock-default", StopReason: "tool_use", Usage: defaultUsage(),
		},
	})
	allowed := requestPermission(enc, scanner, ToolEdit, editID, editInput)
	fixedDelay(50)
	if allowed {
		_ = enc.Encode(UserMsg{Type: TypeUser, Message: UserMsgBody{Role: "user", Content: []ContentBlock{{Type: BlockToolResult, ToolUseID: editID, Content: "File edited successfully: " + editFile.absPath}}}})
	} else {
		emitTextBlock(enc, "Edit denied.", "")
	}

	// Bash (with permission)
	fixedDelay(50)
	bashID := nextToolID()
	bashInput := map[string]any{"command": "echo done", "description": "Print done"}
	_ = enc.Encode(AssistantMsg{
		Type: TypeAssistant,
		Message: AssistantBody{
			Role:    "assistant",
			Content: []ContentBlock{{Type: BlockToolUse, ID: bashID, Name: ToolBash, Input: bashInput}},
			Model:   "mock-default", StopReason: "tool_use", Usage: defaultUsage(),
		},
	})
	allowed = requestPermission(enc, scanner, ToolBash, bashID, bashInput)
	fixedDelay(50)
	if allowed {
		_ = enc.Encode(UserMsg{Type: TypeUser, Message: UserMsgBody{Role: "user", Content: []ContentBlock{{Type: BlockToolResult, ToolUseID: bashID, Content: "done"}}}})
	} else {
		emitTextBlock(enc, "Bash denied.", "")
	}
}

// scenarioMultiTurn: minimal response for multi-turn test.
func scenarioMultiTurn(enc *json.Encoder) {
	fixedDelay(50)
	emitTextBlock(enc, "Multi-turn response ready. Send another message to continue.", "")
}
