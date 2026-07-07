package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// Predefined e2e test scenarios with fixed timing for deterministic test assertions.

// scenarioRegistry maps scenario names to their handler functions.
var scenarioRegistry = map[string]func(e *emitter){
	"simple-message":          scenarioSimpleMessage,
	"read-and-edit":           scenarioReadAndEdit,
	"permission-flow":         scenarioPermissionFlow,
	"error":                   scenarioError,
	"subagent":                scenarioSubagent,
	"all-tools":               scenarioAllTools,
	"multi-turn":              scenarioMultiTurn,
	"diff-expansion-setup":    scenarioDiffExpansionSetup,
	"diff-update-setup":       scenarioDiffUpdateSetup,
	"diff-update-modify":      scenarioDiffUpdateModify,
	"diff-update-streaming":   scenarioDiffUpdateStreaming,
	"multi-file-setup":        scenarioMultiFileSetup,
	"multi-file-modify":       scenarioMultiFileModify,
	"untracked-file-setup":    scenarioUntrackedFileSetup,
	"untracked-file-modify":   scenarioUntrackedFileModify,
	"clarification":           scenarioClarification,
	"clarification-multi":     scenarioClarificationMulti,
	"clarification-timeout":   scenarioClarificationTimeout,
	"multi-permission":        scenarioMultiPermission,
	"kandev-mcp-permission":   scenarioKandevMCPPermission,
	"review-cumulative-setup": scenarioReviewCumulativeSetup,
	"walkthrough-basic":       scenarioWalkthroughBasic,
	"walkthrough-reemit":      scenarioWalkthroughReemit,
	"symlink-file-setup":      scenarioSymlinkFileSetup,
	"markdown-table":          scenarioMarkdownTable,
	"empty-turn":              scenarioEmptyTurn,
}

// scenarioEmptyTurn emits no content and no tool calls, so the turn ends
// cleanly with no agent output. Reproduces the case where an agent treats a
// prompt (e.g. an unsupported slash command) as a no-op and returns an empty
// end_turn. Drives the frontend "empty turn" notice.
func scenarioEmptyTurn(e *emitter) {
	_ = e
	// Stay "running" for a few seconds rather than returning instantly. The
	// empty-turn notice is driven by the live turn.completed event, so the turn
	// must outlast the client's initial WS subscribe (especially on mobile,
	// where a fast auto-start turn can finish before the chat subscribes).
	fixedDelay(3000)
}

// emitPredefinedScenario dispatches to a named e2e scenario.
func emitPredefinedScenario(e *emitter, name string) {
	if fn, ok := scenarioRegistry[name]; ok {
		fn(e)
	} else {
		names := make([]string, 0, len(scenarioRegistry))
		for k := range scenarioRegistry {
			names = append(names, k)
		}
		e.text("Unknown e2e scenario: " + name + ". Available: " + strings.Join(names, ", "))
	}
}

// scenarioSimpleMessage: text only with fixed 100ms delays.
func scenarioSimpleMessage(e *emitter) {
	fixedDelay(100)
	e.thought("Processing the request...")

	fixedDelay(100)
	e.text("This is a simple mock response for e2e testing.")
}

// scenarioReadAndEdit: read -> edit -> text with fixed delays, using real files.
func scenarioReadAndEdit(e *emitter) {
	f := randomFile()
	snippet := readFileSnippet(f.absPath, 20)
	oldStr, newStr := pickEditableFragment(f.absPath)
	fixedDelay(50)

	// Read file
	readID := nextToolID()
	e.startTool(readID, "Read "+f.relPath, acp.ToolKindRead,
		map[string]any{"file_path": f.absPath},
		acp.ToolCallLocation{Path: f.absPath})
	fixedDelay(50)
	e.completeTool(readID, map[string]any{"content": snippet})

	fixedDelay(50)

	// Edit file (with permission)
	editID := nextToolID()
	editInput := map[string]any{
		"file_path":  f.absPath,
		"old_string": oldStr,
		"new_string": newStr,
	}
	e.startTool(editID, "Edit "+f.relPath, acp.ToolKindEdit, editInput,
		acp.ToolCallLocation{Path: f.absPath})

	allowed := e.requestPermission(editID, "Edit "+f.relPath, acp.ToolKindEdit, editInput)

	fixedDelay(50)
	if allowed {
		e.completeTool(editID, map[string]any{"result": "File edited successfully: " + f.absPath})
	} else {
		e.completeTool(editID, map[string]any{"result": "Edit was denied"})
		e.text("Edit was denied.")
	}

	fixedDelay(50)
	e.text("Read and edit scenario complete.")
}

// scenarioMultiPermission: three sequential bash tools, each requiring permission.
// Reproduces the "approvals reappear at turn complete" bug — user approves all
// three, then when the turn ends the approval prompts must NOT come back.
func scenarioMultiPermission(e *emitter) {
	fixedDelay(50)
	e.text("Running three commands that need approval.")

	for i, label := range []string{"first", "second", "third"} {
		fixedDelay(50)
		id := nextToolID()
		input := map[string]any{
			"command":     fmt.Sprintf("echo %s", label),
			"description": fmt.Sprintf("Step %d: %s", i+1, label),
		}
		e.startTool(id, fmt.Sprintf("Run %s command", label), acp.ToolKindExecute, input)
		allowed := e.requestPermission(id, fmt.Sprintf("Run %s command", label), acp.ToolKindExecute, input)
		fixedDelay(50)
		if allowed {
			e.completeTool(id, map[string]any{"output": label})
		} else {
			e.completeTool(id, map[string]any{"output": "denied"})
		}
	}

	fixedDelay(50)
	e.text("Multi-permission scenario complete.")
}

// scenarioKandevMCPPermission emits a tool_call with a Kandev MCP tool title and
// blocks on a permission request, exercising the kandev renderer's approval UI.
func scenarioKandevMCPPermission(e *emitter) {
	fixedDelay(50)
	e.text("Calling a Kandev MCP tool that needs approval.")

	fixedDelay(50)
	id := nextToolID()
	toolName := "mcp__kandev__list_workspaces_kandev"
	input := map[string]any{}
	e.startTool(id, toolName, acp.ToolKindOther, input)
	allowed := e.requestPermission(id, toolName, acp.ToolKindOther, input)

	fixedDelay(50)
	if allowed {
		e.completeTool(id, map[string]any{
			"workspaces": []map[string]any{{"id": "w1", "name": "Main"}},
			"total":      1,
		})
	} else {
		e.completeTool(id, map[string]any{"error": "denied"})
	}

	fixedDelay(50)
	e.text("Kandev MCP permission scenario complete.")
}

// scenarioPermissionFlow: tool requiring permission with fixed delays.
func scenarioPermissionFlow(e *emitter) {
	fixedDelay(50)

	bashID := nextToolID()
	bashInput := map[string]any{
		"command":     "echo 'testing permissions'",
		"description": "Test permission flow",
	}

	e.startTool(bashID, "Test permissions", acp.ToolKindExecute, bashInput)

	allowed := e.requestPermission(bashID, "Test permissions", acp.ToolKindExecute, bashInput)

	fixedDelay(50)
	if allowed {
		e.completeTool(bashID, map[string]any{"output": "testing permissions"})
		e.text("Permission was granted and command executed.")
	} else {
		e.completeTool(bashID, map[string]any{"output": "Permission denied"})
		e.text("Permission was denied.")
	}
}

// scenarioError: error result with fixed delays.
func scenarioError(e *emitter) {
	fixedDelay(100)
	e.text("About to encounter an error...")
	fixedDelay(100)
	e.text("E2E test error: simulated failure")
}

// scenarioSubagent: a claude-style subagent (Task) tool call carrying the
// `_meta.claudeCode` Agent marker and a toolResponse with full result metrics,
// so the kandev adapter normalizes it to a subagent_task payload and the UI
// renders the subagent card with metadata chips.
func scenarioSubagent(e *emitter) {
	taskToolID := nextToolID()
	fixedDelay(50)

	e.startSubagentTool(taskToolID,
		"Explore the codebase",
		"Find all files and summarize the project structure",
		"general-purpose")

	fixedDelay(50)
	e.text("Subagent working on the task...")

	// A tool call the subagent runs internally, attributed to the Task via
	// `_meta.claudeCode.parentToolUseId` so it nests under the subagent card.
	childToolID := nextToolID()
	e.startChildTool(childToolID, taskToolID, "sleep 30", acp.ToolKindExecute,
		map[string]any{"command": "sleep 30"})
	fixedDelay(50)
	e.completeChildTool(childToolID, taskToolID, map[string]any{"output": ""})

	fixedDelay(50)
	e.completeSubagentTool(taskToolID, "E2E subagent completed", subagentResult{
		agentID:      "agent_e2e_0001",
		subagentType: "general-purpose",
		durationMs:   2200,
		totalTokens:  9987,
		toolUseCount: 3,
	})

	fixedDelay(50)
	e.text("Subagent scenario complete.")
}

// scenarioAllTools: one of each tool type with fixed delays, using real files.
func scenarioAllTools(e *emitter) {
	used := map[string]bool{}
	readFile := randomFile()
	used[readFile.absPath] = true
	grepFile := randomFileExcluding(used)
	used[grepFile.absPath] = true
	editFile := randomFileExcluding(used)

	fixedDelay(50)
	e.thought("Running all tools...")

	scenarioAllToolsReadGrep(e, readFile, grepFile)
	scenarioAllToolsEditBash(e, editFile)

	// WebFetch
	fixedDelay(50)
	webID := nextToolID()
	e.startTool(webID, "Fetch example.com", acp.ToolKindFetch,
		map[string]any{"url": "https://example.com", "prompt": "Summarize"})
	fixedDelay(50)
	e.completeTool(webID, map[string]any{"content": "Example page content"})

	fixedDelay(50)
	e.text("All tools scenario complete.")
}

func scenarioAllToolsReadGrep(e *emitter, readFile, grepFile fileInfo) {
	// Read
	fixedDelay(50)
	readID := nextToolID()
	snippet := readFileSnippet(readFile.absPath, 20)
	e.startTool(readID, "Read "+readFile.relPath, acp.ToolKindRead,
		map[string]any{"file_path": readFile.absPath},
		acp.ToolCallLocation{Path: readFile.absPath})
	fixedDelay(50)
	e.completeTool(readID, map[string]any{"content": snippet})

	// Grep
	fixedDelay(50)
	grepID := nextToolID()
	e.startTool(grepID, "Search for \"func \"", acp.ToolKindSearch,
		map[string]any{"pattern": "func ", "path": grepFile.absPath})
	fixedDelay(50)
	paths := randomFilePaths(3)
	var grepResults []string
	for i, p := range paths {
		grepResults = append(grepResults, fmt.Sprintf("%s:%d: func found here", p, (i+1)*10))
	}
	e.completeTool(grepID, map[string]any{"matches": strings.Join(grepResults, "\n")})
}

func scenarioAllToolsEditBash(e *emitter, editFile fileInfo) {
	// Edit (with permission)
	fixedDelay(50)
	editID := nextToolID()
	oldStr, newStr := pickEditableFragment(editFile.absPath)
	editInput := map[string]any{"file_path": editFile.absPath, "old_string": oldStr, "new_string": newStr}
	e.startTool(editID, "Edit "+editFile.relPath, acp.ToolKindEdit, editInput,
		acp.ToolCallLocation{Path: editFile.absPath})
	allowed := e.requestPermission(editID, "Edit "+editFile.relPath, acp.ToolKindEdit, editInput)
	fixedDelay(50)
	if allowed {
		e.completeTool(editID, map[string]any{"result": "File edited successfully: " + editFile.absPath})
	} else {
		e.completeTool(editID, map[string]any{"result": "Edit denied"})
		e.text("Edit denied.")
	}

	// Bash (with permission)
	fixedDelay(50)
	bashID := nextToolID()
	bashInput := map[string]any{"command": "echo done", "description": "Print done"}
	e.startTool(bashID, "Print done", acp.ToolKindExecute, bashInput)
	allowed = e.requestPermission(bashID, "Print done", acp.ToolKindExecute, bashInput)
	fixedDelay(50)
	if allowed {
		e.completeTool(bashID, map[string]any{"output": "done"})
	} else {
		e.completeTool(bashID, map[string]any{"output": "Permission denied"})
		e.text("Bash denied.")
	}
}

// scenarioMultiTurn: minimal response for multi-turn test.
func scenarioMultiTurn(e *emitter) {
	fixedDelay(50)
	e.text("Multi-turn response ready. Send another message to continue.")
}

// scenarioDiffExpansionSetup: creates a committed file and then modifies it,
// leaving an uncommitted diff that the UI can display with expansion enabled.
func scenarioDiffExpansionSetup(e *emitter) {
	fixedDelay(50)

	wd, err := os.Getwd()
	if err != nil {
		e.text("diff-expansion-setup: getwd failed: " + err.Error())
		return
	}

	const totalLines = 200
	originalLines := make([]string, totalLines)
	for i := 0; i < totalLines; i++ {
		originalLines[i] = fmt.Sprintf("func original_%03d() { /* line %d */ }", i+1, i+1)
	}
	original := strings.Join(originalLines, "\n") + "\n"

	filePath := "expansion_test.go"
	if err := os.WriteFile(filePath, []byte(original), 0o644); err != nil {
		e.text("diff-expansion-setup: write failed: " + err.Error())
		return
	}

	runGitCmd := makeGitRunner(wd)
	_ = runGitCmd("rm", "--force", filePath)
	_ = runGitCmd("commit", "-m", "cleanup expansion_test.go")

	if err := os.WriteFile(filePath, []byte(original), 0o644); err != nil {
		e.text("diff-expansion-setup: re-write failed: " + err.Error())
		return
	}

	if err := runGitCmd("add", filePath); err != nil {
		e.text("diff-expansion-setup: git add failed")
		return
	}
	if err := runGitCmd("commit", "-m", "add expansion_test.go for e2e diff expansion test"); err != nil {
		e.text("diff-expansion-setup: git commit failed")
		return
	}

	modifiedLines := make([]string, totalLines)
	copy(modifiedLines, originalLines)
	modifiedLines[49] = "func modified_mid_top() { /* HUNK_TOP - modified line 50 */ }"
	modifiedLines[149] = "func modified_mid_bottom() { /* HUNK_BOTTOM - modified line 150 */ }"
	modified := strings.Join(modifiedLines, "\n") + "\n"

	if err := os.WriteFile(filePath, []byte(modified), 0o644); err != nil {
		e.text("diff-expansion-setup: write modified failed: " + err.Error())
		return
	}

	fixedDelay(100)
	e.text("diff-expansion-setup complete: expansion_test.go has two-hunk uncommitted diff (lines 50 and 150 of 200).")
}

// scenarioDiffUpdateSetup creates a simple file, commits it, then modifies it.
func scenarioDiffUpdateSetup(e *emitter) {
	fixedDelay(50)

	wd, err := os.Getwd()
	if err != nil {
		e.text("diff-update-setup: getwd failed: " + err.Error())
		return
	}

	filePath := "diff_update_test.txt"
	originalContent := "line 1: original\nline 2: unchanged\nline 3: original\n"

	if err := os.WriteFile(filePath, []byte(originalContent), 0o644); err != nil {
		e.text("diff-update-setup: write failed: " + err.Error())
		return
	}

	runGitCmd := makeGitRunner(wd)
	_ = runGitCmd("rm", "--force", filePath)
	_ = runGitCmd("commit", "-m", "cleanup diff_update_test.txt")

	if err := os.WriteFile(filePath, []byte(originalContent), 0o644); err != nil {
		e.text("diff-update-setup: re-write failed: " + err.Error())
		return
	}

	if err := runGitCmd("add", filePath); err != nil {
		e.text("diff-update-setup: git add failed")
		return
	}
	if err := runGitCmd("commit", "-m", "add diff_update_test.txt"); err != nil {
		e.text("diff-update-setup: git commit failed")
		return
	}

	modifiedContent := "line 1: FIRST_MODIFICATION\nline 2: unchanged\nline 3: original\n"
	if err := os.WriteFile(filePath, []byte(modifiedContent), 0o644); err != nil {
		e.text("diff-update-setup: write modified failed: " + err.Error())
		return
	}

	fixedDelay(100)
	e.text("diff-update-setup complete: diff_update_test.txt has FIRST_MODIFICATION")
}

// scenarioDiffUpdateModify modifies the diff_update_test.txt file again.
func scenarioDiffUpdateModify(e *emitter) {
	fixedDelay(50)

	filePath := "diff_update_test.txt"
	modifiedContent := "line 1: SECOND_MODIFICATION\nline 2: unchanged\nline 3: ALSO_CHANGED\n"
	if err := os.WriteFile(filePath, []byte(modifiedContent), 0o644); err != nil {
		e.text("diff-update-modify: write failed: " + err.Error())
		return
	}

	fixedDelay(100)
	e.text("diff-update-modify complete: diff_update_test.txt now has SECOND_MODIFICATION")
}

// scenarioDiffUpdateStreaming modifies the file mid-turn, emitting text both
// before and after the write so the agent's turn stays active for several
// seconds. Used to assert that open file editor / diff panels auto-update
// while the agent is still streaming.
func scenarioDiffUpdateStreaming(e *emitter) {
	fixedDelay(50)
	e.text("diff-update-streaming: starting work")

	fixedDelay(1000)

	filePath := "diff_update_test.txt"
	modifiedContent := "line 1: SECOND_MODIFICATION\nline 2: unchanged\nline 3: ALSO_CHANGED\n"
	if err := os.WriteFile(filePath, []byte(modifiedContent), 0o644); err != nil {
		e.text("diff-update-streaming: write failed: " + err.Error())
		return
	}

	fixedDelay(500)
	e.text("diff-update-streaming: file written, continuing")

	// Long tail keeps the turn active long enough for polling+sync to fire.
	fixedDelay(6000)
	e.text("diff-update-streaming complete")
}

// scenarioMultiFileSetup commits three tracked files, then modifies all three
// so each shows up in git status with FIRST_MODIFICATION. Used to test the
// case where the user has multiple file editors / diff panels open at once.
func scenarioMultiFileSetup(e *emitter) {
	fixedDelay(50)

	wd, err := os.Getwd()
	if err != nil {
		e.text("multi-file-setup: getwd failed: " + err.Error())
		return
	}

	runGitCmd := makeGitRunner(wd)
	files := []string{"multi_a.txt", "multi_b.txt", "multi_c.txt"}

	// Cleanup any leftovers from a prior run.
	for _, f := range files {
		_ = runGitCmd("rm", "--force", f)
	}
	_ = runGitCmd("commit", "-m", "cleanup multi-file fixtures")

	// Commit pristine versions.
	for _, f := range files {
		original := fmt.Sprintf("%s line 1: original\n%s line 2: unchanged\n%s line 3: original\n", f, f, f)
		if err := os.WriteFile(f, []byte(original), 0o644); err != nil {
			e.text("multi-file-setup: write failed: " + err.Error())
			return
		}
		if err := runGitCmd("add", f); err != nil {
			e.text("multi-file-setup: git add failed for " + f)
			return
		}
	}
	if err := runGitCmd("commit", "-m", "add multi-file fixtures"); err != nil {
		e.text("multi-file-setup: git commit failed")
		return
	}

	// Modify all three so each appears in git status with FIRST_MODIFICATION.
	for _, f := range files {
		modified := fmt.Sprintf("%s line 1: FIRST_MODIFICATION\n%s line 2: unchanged\n%s line 3: original\n", f, f, f)
		if err := os.WriteFile(f, []byte(modified), 0o644); err != nil {
			e.text("multi-file-setup: write modified failed: " + err.Error())
			return
		}
	}

	fixedDelay(100)
	e.text("multi-file-setup complete: 3 files have FIRST_MODIFICATION")
}

// scenarioMultiFileModify modifies all three multi-file fixtures within a
// single turn, with a long trailing delay so the panels must auto-update
// while the agent is still streaming. Reproduces the user's reported case
// where multiple open editor / diff panels go stale on a real edit.
func scenarioMultiFileModify(e *emitter) {
	fixedDelay(50)
	e.text("multi-file-modify: starting work")

	fixedDelay(800)

	files := []string{"multi_a.txt", "multi_b.txt", "multi_c.txt"}
	for i, f := range files {
		modified := fmt.Sprintf(
			"%s line 1: SECOND_MODIFICATION\n%s line 2: unchanged\n%s line 3: ALSO_CHANGED_%d\n",
			f, f, f, i,
		)
		if err := os.WriteFile(f, []byte(modified), 0o644); err != nil {
			e.text("multi-file-modify: write failed: " + err.Error())
			return
		}
		// Stagger the writes slightly to mimic a real agent doing one tool
		// call per file rather than all writes in a single instant.
		fixedDelay(250)
	}

	e.text("multi-file-modify: writes done, continuing")

	// Long tail keeps the turn active so we can assert mid-turn updates.
	fixedDelay(5000)
	e.text("multi-file-modify complete")
}

// scenarioUntrackedFileSetup creates a new untracked file.
func scenarioUntrackedFileSetup(e *emitter) {
	fixedDelay(50)

	filePath := "untracked_test.txt"
	content := "line 1: INITIAL_CONTENT\nline 2: some text\n"

	_ = os.Remove(filePath)

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		e.text("untracked-file-setup: write failed: " + err.Error())
		return
	}

	fixedDelay(100)
	e.text("untracked-file-setup complete: untracked_test.txt has INITIAL_CONTENT")
}

// scenarioUntrackedFileModify modifies the untracked file.
func scenarioUntrackedFileModify(e *emitter) {
	fixedDelay(50)

	filePath := "untracked_test.txt"
	content := "line 1: MODIFIED_CONTENT\nline 2: some text\nline 3: NEW_LINE\n"

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		e.text("untracked-file-modify: write failed: " + err.Error())
		return
	}

	fixedDelay(100)
	e.text("untracked-file-modify complete: untracked_test.txt now has MODIFIED_CONTENT")
}

// MCP argument keys used by the clarification scenarios. Pulled out so goconst
// stays happy when the same string would otherwise repeat across both helpers.
const (
	clarificationOptionsKey = "options"
	clarificationLabelKey   = "label"
	clarificationDescKey    = "description"
	clarificationPromptKey  = "prompt"
	clarificationIDKey      = "id"
)

func mockOption(label, description string) map[string]any {
	return map[string]any{clarificationLabelKey: label, clarificationDescKey: description}
}

// clarificationQuestionArgs returns the MCP arguments for a single-question
// clarification call. Used by the simplest e2e scenario where a one-question
// bundle exercises the same code path as multi-question.
func clarificationQuestionArgs() map[string]any {
	return map[string]any{
		"questions": []map[string]any{
			{
				clarificationIDKey:     "db",
				clarificationPromptKey: "Which database should we use for this project?",
				clarificationOptionsKey: []map[string]any{
					mockOption("PostgreSQL", "Relational database with strong consistency"),
					mockOption("MongoDB", "Document database for flexible schemas"),
					mockOption("SQLite", "Embedded database for simplicity"),
				},
			},
		},
	}
}

// clarificationMultiQuestionArgs returns the MCP arguments for a 3-question
// clarification bundle that exercises the multi-question UI flow.
func clarificationMultiQuestionArgs() map[string]any {
	return map[string]any{
		"questions": []map[string]any{
			{
				clarificationIDKey:     "db",
				clarificationPromptKey: "Which database should we use?",
				clarificationOptionsKey: []map[string]any{
					mockOption("PostgreSQL", "Relational database with strong consistency"),
					mockOption("MongoDB", "Document database for flexible schemas"),
					mockOption("SQLite", "Embedded database for simplicity"),
				},
			},
			{
				clarificationIDKey:     "language",
				clarificationPromptKey: "Which language should we use?",
				clarificationOptionsKey: []map[string]any{
					mockOption("Go", "Strong typing, fast compile"),
					mockOption("TypeScript", "Familiar to the team"),
					mockOption("Rust", "Best for systems work"),
				},
			},
			{
				clarificationIDKey:     "deploy",
				clarificationPromptKey: "How should we deploy?",
				clarificationOptionsKey: []map[string]any{
					mockOption("Docker", "Containerized deploy"),
					mockOption("Bare metal", "Run directly on hosts"),
				},
			},
		},
		"context": "Picking the foundational stack — answer all three so we can move forward.",
	}
}

// scenarioClarification: happy path — ask a question via MCP and wait for the answer.
func scenarioClarification(e *emitter) {
	fixedDelay(100)
	e.text("Let me ask you a question about the project setup.")

	result, err := callMCPTool("kandev", "ask_user_question_kandev", clarificationQuestionArgs())
	if err != nil {
		e.text(fmt.Sprintf("Question failed: %s", err))
		return
	}

	fixedDelay(50)
	e.text(fmt.Sprintf("You answered: %s", result))
}

// scenarioClarificationMulti: stress the multi-question path — 3 questions in
// one bundle, all required, single MCP call.
func scenarioClarificationMulti(e *emitter) {
	fixedDelay(100)
	e.text("Let me ask you a few questions about the project setup.")

	result, err := callMCPTool("kandev", "ask_user_question_kandev", clarificationMultiQuestionArgs())
	if err != nil {
		e.text(fmt.Sprintf("Questions failed: %s", err))
		return
	}

	fixedDelay(50)
	e.text(fmt.Sprintf("You answered: %s", result))
}

// scenarioClarificationTimeout: ask a question with a short timeout, then continue.
func scenarioClarificationTimeout(e *emitter) {
	fixedDelay(100)
	e.text("Let me ask you a question about the project setup.")

	ctx, cancel := contextWithTimeout(5)
	defer cancel()

	result, err := callMCPToolCtx(ctx, "kandev", "ask_user_question_kandev", clarificationQuestionArgs())
	if err != nil {
		fixedDelay(50)
		if ctx.Err() != nil {
			e.text("Question timed out, continuing without answer.")
		} else {
			e.text(fmt.Sprintf("Question failed: %s", err))
		}
		return
	}

	fixedDelay(50)
	e.text(fmt.Sprintf("You answered: %s", result))
}

// scenarioReviewCumulativeSetup creates a file, commits it, then modifies and
// commits again, then makes a final uncommitted modification.  This produces a
// file with both committed and uncommitted changes relative to the session's
// base commit, exercising the cumulative diff (base → working tree).
func scenarioReviewCumulativeSetup(e *emitter) {
	fixedDelay(50)

	wd, err := os.Getwd()
	if err != nil {
		e.text("review-cumulative-setup: getwd failed: " + err.Error())
		return
	}

	filePath := "review_cumulative_test.txt"
	baseContent := "line 1: BASE_CONTENT\nline 2: unchanged\nline 3: BASE_CONTENT\n"

	// Clean up any leftover file from a previous run.
	runGitCmd := makeGitRunner(wd)
	_ = runGitCmd("rm", "--force", filePath)
	_ = runGitCmd("commit", "-m", "cleanup "+filePath)

	// Write original content and commit.
	if err := os.WriteFile(filePath, []byte(baseContent), 0o644); err != nil {
		e.text("review-cumulative-setup: write base failed: " + err.Error())
		return
	}
	if err := runGitCmd("add", filePath); err != nil {
		e.text("review-cumulative-setup: git add base failed")
		return
	}
	if err := runGitCmd("commit", "-m", "add "+filePath+" with base content"); err != nil {
		e.text("review-cumulative-setup: git commit base failed")
		return
	}

	// Second commit: modify the file.
	committedContent := "line 1: COMMITTED_CHANGE\nline 2: unchanged\nline 3: BASE_CONTENT\n"
	if err := os.WriteFile(filePath, []byte(committedContent), 0o644); err != nil {
		e.text("review-cumulative-setup: write committed failed: " + err.Error())
		return
	}
	if err := runGitCmd("add", filePath); err != nil {
		e.text("review-cumulative-setup: git add committed failed")
		return
	}
	if err := runGitCmd("commit", "-m", "modify "+filePath+" with committed change"); err != nil {
		e.text("review-cumulative-setup: git commit committed failed")
		return
	}

	// Leave an uncommitted modification on top.
	uncommittedContent := "line 1: COMMITTED_CHANGE\nline 2: unchanged\nline 3: UNCOMMITTED_CHANGE\n"
	if err := os.WriteFile(filePath, []byte(uncommittedContent), 0o644); err != nil {
		e.text("review-cumulative-setup: write uncommitted failed: " + err.Error())
		return
	}

	fixedDelay(100)
	e.text("review-cumulative-setup complete: " + filePath + " has COMMITTED_CHANGE and UNCOMMITTED_CHANGE")
}

// walkthroughDemoArgs is the show_walkthrough_kandev payload used by
// scenarioWalkthroughBasic. task_id is resolved server-side from the session's
// env (KANDEV_TASK_ID), mirroring the clarification path, so it is omitted here.
// Walkthrough step JSON keys, hoisted to constants to satisfy goconst.
const (
	wtKeyTitle   = "title"
	wtKeyFile    = "file"
	wtKeyLine    = "line"
	wtKeyLineEnd = "line_end"
	wtKeyText    = "text"
	wtKeySteps   = "steps"
)

// wtStep builds one show_walkthrough step map. Keeping the JSON keys in one
// place avoids repeating string literals (goconst).
func wtStep(title, file, text string, line, lineEnd int) map[string]interface{} {
	m := map[string]interface{}{wtKeyTitle: title, wtKeyFile: file, wtKeyLine: line, wtKeyText: text}
	if lineEnd > 0 {
		m[wtKeyLineEnd] = lineEnd
	}
	return m
}

// wtArgs assembles show_walkthrough_kandev args from a title and steps.
func wtArgs(title string, steps ...map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{wtKeyTitle: title, wtKeySteps: steps}
}

// scenarioWalkthroughReemit emits one walkthrough, waits, then emits a second
// (different) one — exercising the live `task.walkthrough.updated` path so the
// UI must reflect the re-emit without a page reload.
func scenarioWalkthroughReemit(e *emitter) {
	fixedDelay(50)
	wd, err := os.Getwd()
	if err != nil {
		e.text("walkthrough-reemit: getwd failed: " + err.Error())
		return
	}
	runGitCmd := makeGitRunner(wd)
	if !writeWalkthroughFile(e, runGitCmd, "reemit.txt", "line 1\nline 2\n", "line 1\nREEMIT_CHANGE\n") {
		return
	}

	e.text("First tour incoming.")
	if _, err := callMCPTool("kandev", "show_walkthrough_kandev", wtArgs("First",
		wtStep("First step", "reemit.txt", "REEMIT_FIRST step one.", 1, 0),
		wtStep("First step 2", "reemit.txt", "REEMIT_FIRST step two.", 2, 0),
	)); err != nil {
		e.text(fmt.Sprintf("show_walkthrough failed: %s", err))
		return
	}
	e.text("reemit-first-done")

	fixedDelay(200)

	if _, err := callMCPTool("kandev", "show_walkthrough_kandev", wtArgs("Second",
		wtStep("Second step", "reemit.txt", "REEMIT_SECOND step one.", 1, 0),
		wtStep("Second step 2", "reemit.txt", "REEMIT_SECOND step two.", 2, 0),
		wtStep("Second step 3", "reemit.txt", "REEMIT_SECOND step three.", 1, 0),
	)); err != nil {
		e.text(fmt.Sprintf("show_walkthrough re-emit failed: %s", err))
		return
	}
	e.text("reemit-second-done")
}

// walkthroughDemoArgs builds a 5-step tour spanning three changed files plus
// one unchanged file (exercising both the diff-anchored card and the full-file
// / editor-mode floating window).
func walkthroughDemoArgs() map[string]interface{} {
	return map[string]interface{}{
		wtKeyTitle: "Tour of the change",
		wtKeySteps: []map[string]interface{}{
			wtStep("Entry point", "walkthrough_a.txt",
				"Execution starts here. **Step 1** explains the entry point in file A.", 1, 0),
			wtStep("The change in A", "walkthrough_a.txt",
				"Step 2: this is WALKTHROUGH_CHANGE_A the tour narrates.", 2, 3),
			wtStep("File B", "walkthrough_b.txt",
				"Step 3: WALKTHROUGH_CHANGE_B lives in file B.", 2, 0),
			wtStep("File C", "walkthrough_c.txt",
				"Step 4: WALKTHROUGH_CHANGE_C lives in file C.", 2, 0),
			wtStep("Unchanged file", "walkthrough_base.txt",
				"Step 5: WALKTHROUGH_UNCHANGED — this base file did not change; shown from its current state.", 1, 0),
		},
	}
}

// writeWalkthroughFile writes content and commits it, then (when changed is
// non-empty) leaves an uncommitted modification so the file appears in review.
func writeWalkthroughFile(e *emitter, runGitCmd func(args ...string) error, path, base, changed string) bool {
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		e.text("walkthrough-basic: write base failed: " + err.Error())
		return false
	}
	_ = runGitCmd("add", path)
	_ = runGitCmd("commit", "-m", "add "+path)
	if changed == "" {
		return true
	}
	if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
		e.text("walkthrough-basic: write change failed: " + err.Error())
		return false
	}
	return true
}

// scenarioWalkthroughBasic creates a small committed file plus an uncommitted
// change (so the review surface has a diff), then emits a 2-step code
// walkthrough over it via the show_walkthrough_kandev MCP tool.
func scenarioWalkthroughBasic(e *emitter) {
	fixedDelay(50)

	wd, err := os.Getwd()
	if err != nil {
		e.text("walkthrough-basic: getwd failed: " + err.Error())
		return
	}
	runGitCmd := makeGitRunner(wd)

	// Three changed files (diff-anchored steps) + one unchanged file (editor-mode step).
	ok := writeWalkthroughFile(e, runGitCmd, "walkthrough_a.txt",
		"line 1: ENTRY\nline 2: BASE\nline 3: BASE\n",
		"line 1: ENTRY\nline 2: WALKTHROUGH_CHANGE_A\nline 3: WALKTHROUGH_CHANGE_A\n")
	ok = ok && writeWalkthroughFile(e, runGitCmd, "walkthrough_b.txt",
		"line 1: B\nline 2: BASE\n", "line 1: B\nline 2: WALKTHROUGH_CHANGE_B\n")
	ok = ok && writeWalkthroughFile(e, runGitCmd, "walkthrough_c.txt",
		"line 1: C\nline 2: BASE\n", "line 1: C\nline 2: WALKTHROUGH_CHANGE_C\n")
	if !ok {
		return
	}

	fixedDelay(100)
	e.text("Let me walk you through the change.")
	if _, err := callMCPTool("kandev", "show_walkthrough_kandev", walkthroughDemoArgs()); err != nil {
		e.text(fmt.Sprintf("show_walkthrough failed: %s", err))
		return
	}
	fixedDelay(50)
	e.text("walkthrough-basic complete: 5-step tour emitted")
}

// scenarioSymlinkFileSetup creates a file and a symlink to it, commits both,
// then modifies the target file leaving an uncommitted diff.
func scenarioSymlinkFileSetup(e *emitter) {
	fixedDelay(50)

	wd, err := os.Getwd()
	if err != nil {
		e.text("symlink-file-setup: getwd failed: " + err.Error())
		return
	}

	runGitCmd := makeGitRunner(wd)

	// Create target file and commit it.
	targetFile := "real-file.txt"
	if err := os.WriteFile(targetFile, []byte("Hello from symlink target!\n"), 0o644); err != nil {
		e.text("symlink-file-setup: write target failed: " + err.Error())
		return
	}
	_ = runGitCmd("add", targetFile)
	_ = runGitCmd("commit", "-m", "add real-file.txt")

	// Create symlink and commit it.
	linkFile := "link-file.txt"
	_ = os.Remove(linkFile) // clean up if leftover
	if err := os.Symlink(targetFile, linkFile); err != nil {
		e.text("symlink-file-setup: symlink failed: " + err.Error())
		return
	}
	_ = runGitCmd("add", linkFile)
	_ = runGitCmd("commit", "-m", "add link-file.txt symlink")

	// Modify target file, leaving an uncommitted diff.
	if err := os.WriteFile(targetFile, []byte("Modified symlink target content!\n"), 0o644); err != nil {
		e.text("symlink-file-setup: write modified failed: " + err.Error())
		return
	}

	fixedDelay(100)
	e.text("symlink-file-setup complete")
}

// makeGitRunner returns a function that runs git commands in the given directory.
func makeGitRunner(wd string) func(args ...string) error {
	gitEnv := append(os.Environ(),
		"GIT_AUTHOR_NAME=Mock Agent",
		"GIT_AUTHOR_EMAIL=mock@test.local",
		"GIT_COMMITTER_NAME=Mock Agent",
		"GIT_COMMITTER_EMAIL=mock@test.local",
	)
	return func(args ...string) error {
		cmd := exec.Command("git", append([]string{
			"-c", "commit.gpgsign=false",
			"-c", "tag.gpgsign=false",
		}, args...)...)
		cmd.Dir = wd
		cmd.Env = gitEnv
		out, cmdErr := cmd.CombinedOutput()
		if cmdErr != nil {
			_, _ = fmt.Fprintf(logOutput, "mock-agent: git %v failed: %v\nOutput: %s\n", args, cmdErr, out)
		}
		return cmdErr
	}
}

// contextWithTimeout creates a context with timeout in seconds.
func contextWithTimeout(seconds int) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
}

// scenarioMarkdownTable: emits a dense label/value markdown table modeled on a
// real agent bug-report summary. The table has narrow header labels in column 1
// and long inline-code identifiers in column 2 — the shape that previously
// caused header words to wrap character-by-character ("Failin g test") because
// the value column starved the label column.
func scenarioMarkdownTable(e *emitter) {
	fixedDelay(100)
	e.text("Pushed.\n\n" +
		"## Summary\n\n" +
		"| | |\n" +
		"|---|---|\n" +
		"| **Failing test** | `TestHandleAgentBootReady_DrainsOrphanedQueuedMessage/already_WAITING_FOR_INPUT_(boot_raced_persistResumeState)` |\n" +
		"| **Symptom** | `session.State = \"RUNNING\", want WAITING_FOR_INPUT` |\n" +
		"| **Root cause** | Pre-existing race, not introduced by this PR. `handleAgentBootReady` synchronously flips state to `WAITING_FOR_INPUT` then spawns a goroutine that calls `PromptTask` → flips state to `RUNNING`. The test asserted on `WAITING_FOR_INPUT` immediately after the handler returned — on faster CI scheduling the goroutine wins the race. The kandev-ci container apparently schedules tighter than the github-hosted ubuntu, so it loses where the previous env got lucky. |\n" +
		"| **Fix** | Cherry-picked `b8d06ea8 test(backend): fix race in TestHandleAgentBootReady_DrainsOrphanedQueuedMessage` from `feature/subtask-with-repo-se-vhz` (a parallel branch that already addressed this). The test now accepts either `WAITING_FOR_INPUT` or `RUNNING` — both prove the boot-ready flip landed and rule out the original `STARTING + queue still full` regression. |\n" +
		"| **Local verification** | `go test -race -count=5` → 5/5 PASS. |\n")
}
