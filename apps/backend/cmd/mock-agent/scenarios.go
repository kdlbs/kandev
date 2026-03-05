package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
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
	case "diff-expansion-setup":
		scenarioDiffExpansionSetup(enc)
	case "diff-update-setup":
		scenarioDiffUpdateSetup(enc)
	case "diff-update-modify":
		scenarioDiffUpdateModify(enc)
	case "untracked-file-setup":
		scenarioUntrackedFileSetup(enc)
	case "untracked-file-modify":
		scenarioUntrackedFileModify(enc)
	case "clarification":
		scenarioClarification(enc)
	case "clarification-timeout":
		scenarioClarificationTimeout(enc)
	default:
		emitTextBlock(enc, "Unknown e2e scenario: "+name+". Available: simple-message, read-and-edit, permission-flow, error, subagent, all-tools, multi-turn, diff-expansion-setup, diff-update-setup, diff-update-modify, untracked-file-setup, untracked-file-modify, clarification, clarification-timeout", "")
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

// scenarioDiffExpansionSetup: creates a committed file and then modifies it,
// leaving an uncommitted diff that the UI can display with expansion enabled.
//
// Uses 200 lines with hunks at lines 50 (mid-top) and 150 (mid-bottom), so the
// diff viewer collapses ~90 lines between hunks and ~45 lines at each end.
// This gives three separate expand handles for the e2e test to interact with.
func scenarioDiffExpansionSetup(enc *json.Encoder) {
	fixedDelay(50)

	wd, err := os.Getwd()
	if err != nil {
		emitTextBlock(enc, "diff-expansion-setup: getwd failed: "+err.Error(), "")
		return
	}

	// Build a 200-line file with distinct content per line so assertions can
	// target specific line text after expanding collapsed regions.
	const totalLines = 200
	originalLines := make([]string, totalLines)
	for i := 0; i < totalLines; i++ {
		originalLines[i] = fmt.Sprintf("func original_%03d() { /* line %d */ }", i+1, i+1)
	}
	original := strings.Join(originalLines, "\n") + "\n"

	filePath := "expansion_test.go"
	if err := os.WriteFile(filePath, []byte(original), 0o644); err != nil {
		emitTextBlock(enc, "diff-expansion-setup: write failed: "+err.Error(), "")
		return
	}

	gitEnv := append(os.Environ(),
		"GIT_AUTHOR_NAME=Mock Agent",
		"GIT_AUTHOR_EMAIL=mock@test.local",
		"GIT_COMMITTER_NAME=Mock Agent",
		"GIT_COMMITTER_EMAIL=mock@test.local",
	)

	runGitCmd := func(args ...string) error {
		cmd := exec.Command("git", append([]string{
			"-c", "commit.gpgsign=false",
			"-c", "tag.gpgsign=false",
		}, args...)...)
		cmd.Dir = wd
		cmd.Env = gitEnv
		out, cmdErr := cmd.CombinedOutput()
		if cmdErr != nil {
			fmt.Fprintf(os.Stderr, "mock-agent: git %v failed: %v\nOutput: %s\n", args, cmdErr, out)
		}
		return cmdErr
	}

	// Clean up from any previous test run so the add+commit is never a no-op.
	// git rm --force removes the file from both the index and worktree;
	// the commit records the deletion. Errors are ignored (file may not exist).
	_ = runGitCmd("rm", "--force", filePath)
	_ = runGitCmd("commit", "-m", "cleanup expansion_test.go")

	// Re-write the original content (git rm deleted it from the worktree).
	if err := os.WriteFile(filePath, []byte(original), 0o644); err != nil {
		emitTextBlock(enc, "diff-expansion-setup: re-write failed: "+err.Error(), "")
		return
	}

	if err := runGitCmd("add", filePath); err != nil {
		emitTextBlock(enc, "diff-expansion-setup: git add failed", "")
		return
	}
	if err := runGitCmd("commit", "-m", "add expansion_test.go for e2e diff expansion test"); err != nil {
		emitTextBlock(enc, "diff-expansion-setup: git commit failed", "")
		return
	}

	// Modify line 50 (mid-top, index 49) and line 150 (mid-bottom, index 149).
	// With 3 lines of context per hunk, the diff collapses:
	//   - lines 1-46   (~46 lines) above the first hunk
	//   - lines 54-146 (~93 lines) between the two hunks
	//   - lines 154-200 (~47 lines) below the second hunk
	// Each collapsed region gets its own expand handle in the diff viewer.
	modifiedLines := make([]string, totalLines)
	copy(modifiedLines, originalLines)
	modifiedLines[49] = "func modified_mid_top() { /* HUNK_TOP - modified line 50 */ }"
	modifiedLines[149] = "func modified_mid_bottom() { /* HUNK_BOTTOM - modified line 150 */ }"
	modified := strings.Join(modifiedLines, "\n") + "\n"

	if err := os.WriteFile(filePath, []byte(modified), 0o644); err != nil {
		emitTextBlock(enc, "diff-expansion-setup: write modified failed: "+err.Error(), "")
		return
	}

	fixedDelay(100)
	emitTextBlock(enc, "diff-expansion-setup complete: expansion_test.go has two-hunk uncommitted diff (lines 50 and 150 of 200).", "")
}

// scenarioDiffUpdateSetup creates a simple file, commits it, then modifies it
// to leave an uncommitted diff. This is the first step of the diff-update test.
// The file content is simple so assertions can verify exact diff content.
func scenarioDiffUpdateSetup(enc *json.Encoder) {
	fixedDelay(50)

	wd, err := os.Getwd()
	if err != nil {
		emitTextBlock(enc, "diff-update-setup: getwd failed: "+err.Error(), "")
		return
	}

	filePath := "diff_update_test.txt"
	originalContent := "line 1: original\nline 2: unchanged\nline 3: original\n"

	// Write original content
	if err := os.WriteFile(filePath, []byte(originalContent), 0o644); err != nil {
		emitTextBlock(enc, "diff-update-setup: write failed: "+err.Error(), "")
		return
	}

	gitEnv := append(os.Environ(),
		"GIT_AUTHOR_NAME=Mock Agent",
		"GIT_AUTHOR_EMAIL=mock@test.local",
		"GIT_COMMITTER_NAME=Mock Agent",
		"GIT_COMMITTER_EMAIL=mock@test.local",
	)

	runGitCmd := func(args ...string) error {
		cmd := exec.Command("git", append([]string{
			"-c", "commit.gpgsign=false",
			"-c", "tag.gpgsign=false",
		}, args...)...)
		cmd.Dir = wd
		cmd.Env = gitEnv
		out, cmdErr := cmd.CombinedOutput()
		if cmdErr != nil {
			fmt.Fprintf(os.Stderr, "mock-agent: git %v failed: %v\nOutput: %s\n", args, cmdErr, out)
		}
		return cmdErr
	}

	// Clean up from any previous test run
	_ = runGitCmd("rm", "--force", filePath)
	_ = runGitCmd("commit", "-m", "cleanup diff_update_test.txt")

	// Re-write and commit original content
	if err := os.WriteFile(filePath, []byte(originalContent), 0o644); err != nil {
		emitTextBlock(enc, "diff-update-setup: re-write failed: "+err.Error(), "")
		return
	}

	if err := runGitCmd("add", filePath); err != nil {
		emitTextBlock(enc, "diff-update-setup: git add failed", "")
		return
	}
	if err := runGitCmd("commit", "-m", "add diff_update_test.txt"); err != nil {
		emitTextBlock(enc, "diff-update-setup: git commit failed", "")
		return
	}

	// Modify the file to create an uncommitted diff
	modifiedContent := "line 1: FIRST_MODIFICATION\nline 2: unchanged\nline 3: original\n"
	if err := os.WriteFile(filePath, []byte(modifiedContent), 0o644); err != nil {
		emitTextBlock(enc, "diff-update-setup: write modified failed: "+err.Error(), "")
		return
	}

	fixedDelay(100)
	emitTextBlock(enc, "diff-update-setup complete: diff_update_test.txt has FIRST_MODIFICATION", "")
}

// scenarioDiffUpdateModify modifies the diff_update_test.txt file again,
// changing the diff content. This tests that the UI updates the diff display
// when files change after the initial diff was shown.
func scenarioDiffUpdateModify(enc *json.Encoder) {
	fixedDelay(50)

	filePath := "diff_update_test.txt"

	// Modify the file again with different content
	modifiedContent := "line 1: SECOND_MODIFICATION\nline 2: unchanged\nline 3: ALSO_CHANGED\n"
	if err := os.WriteFile(filePath, []byte(modifiedContent), 0o644); err != nil {
		emitTextBlock(enc, "diff-update-modify: write failed: "+err.Error(), "")
		return
	}

	fixedDelay(100)
	emitTextBlock(enc, "diff-update-modify complete: diff_update_test.txt now has SECOND_MODIFICATION", "")
}

// scenarioUntrackedFileSetup creates a new untracked file (not staged, not committed).
// This tests that the UI shows diffs for untracked files.
func scenarioUntrackedFileSetup(enc *json.Encoder) {
	fixedDelay(50)

	filePath := "untracked_test.txt"
	content := "line 1: INITIAL_CONTENT\nline 2: some text\n"

	// Remove any existing file first
	_ = os.Remove(filePath)

	// Write new untracked file
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		emitTextBlock(enc, "untracked-file-setup: write failed: "+err.Error(), "")
		return
	}

	fixedDelay(100)
	emitTextBlock(enc, "untracked-file-setup complete: untracked_test.txt has INITIAL_CONTENT", "")
}

// scenarioUntrackedFileModify modifies the untracked file created by untracked-file-setup.
// This tests that the UI updates the diff when an untracked file is modified.
func scenarioUntrackedFileModify(enc *json.Encoder) {
	fixedDelay(50)

	filePath := "untracked_test.txt"
	content := "line 1: MODIFIED_CONTENT\nline 2: some text\nline 3: NEW_LINE\n"

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		emitTextBlock(enc, "untracked-file-modify: write failed: "+err.Error(), "")
		return
	}

	fixedDelay(100)
	emitTextBlock(enc, "untracked-file-modify complete: untracked_test.txt now has MODIFIED_CONTENT", "")
}

// clarificationQuestionArgs returns the MCP arguments for the clarification question.
func clarificationQuestionArgs() map[string]any {
	return map[string]any{
		"prompt": "Which database should we use for this project?",
		"options": []map[string]any{
			{"label": "PostgreSQL", "description": "Relational database with strong consistency"},
			{"label": "MongoDB", "description": "Document database for flexible schemas"},
			{"label": "SQLite", "description": "Embedded database for simplicity"},
		},
	}
}

// scenarioClarification: happy path — ask a question via MCP and wait for the answer.
func scenarioClarification(enc *json.Encoder) {
	fixedDelay(100)
	emitTextBlock(enc, "Let me ask you a question about the project setup.", "")

	result, err := callMCPTool("kandev", "ask_user_question", clarificationQuestionArgs())
	if err != nil {
		emitTextBlock(enc, fmt.Sprintf("Question failed: %s", err), "")
		return
	}

	fixedDelay(50)
	emitTextBlock(enc, fmt.Sprintf("You answered: %s", result), "")
}

// scenarioClarificationTimeout: ask a question with a short timeout, then continue.
// The MCP call times out after 5s so the agent's turn completes. The user can still
// answer later, which triggers the event fallback path (new turn).
func scenarioClarificationTimeout(enc *json.Encoder) {
	fixedDelay(100)
	emitTextBlock(enc, "Let me ask you a question about the project setup.", "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := callMCPToolCtx(ctx, "kandev", "ask_user_question", clarificationQuestionArgs())
	if err != nil {
		fixedDelay(50)
		if ctx.Err() != nil {
			emitTextBlock(enc, "Question timed out, continuing without answer.", "")
		} else {
			emitTextBlock(enc, fmt.Sprintf("Question failed: %s", err), "")
		}
		return
	}

	fixedDelay(50)
	emitTextBlock(enc, fmt.Sprintf("You answered: %s", result), "")
}
