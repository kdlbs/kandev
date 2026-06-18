package acp

import (
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/readselector"
)

// Tool operation type constants.
const (
	toolKindEdit    = "edit"
	toolKindRead    = "read"
	toolKindExecute = "execute"
	toolKindGlob    = "glob"
	toolKindGrep    = "grep"
	toolKindSearch  = "search"

	toolTypeEdit    = "tool_edit"
	toolTypeRead    = "tool_read"
	toolTypeExecute = "tool_execute"
	toolTypeSearch  = "tool_search"
	toolTypeGeneric = "tool_call"

	toolStatusComplete   = "complete"
	toolStatusError      = "error"
	toolStatusInProgress = "in_progress"
	toolStatusCancelled  = "cancelled"

	// args map keys the adapter stashes so the normalizer can detect subagent
	// (Task) tool calls without changing NormalizeToolCall's signature.
	argKeyTitle  = "title"
	argKeyMeta   = "meta"
	keyLocations = "locations"
	keyPath      = "path"

	readTypeDirectory  = "directory"
	genericLabelFile   = "file"
	genericLabelFolder = "folder"
)

// DetectToolOperationType determines the specific tool operation type from ACP tool data.
// Used for logging and backwards compatibility.
func DetectToolOperationType(toolKind string, args map[string]any) string {
	// Check Auggie's "kind" field first
	if kind, ok := args["kind"].(string); ok {
		switch kind {
		case toolKindEdit:
			return toolTypeEdit
		case toolKindRead:
			// Check if this is a directory read (file listing)
			if rawInput, ok := args["raw_input"].(map[string]any); ok {
				if readType, ok := rawInput["type"].(string); ok && readType == readTypeDirectory {
					return toolTypeSearch
				}
			}
			return toolTypeRead
		case toolKindExecute:
			return toolTypeExecute
		}
	}

	// Fallback to tool kind/name matching
	switch strings.ToLower(toolKind) {
	case toolKindEdit:
		return toolTypeEdit
	case toolKindRead, "view":
		return toolTypeRead
	case toolKindExecute, "bash", "run":
		return toolTypeExecute
	case toolKindGlob, toolKindGrep, toolKindSearch:
		return toolTypeSearch
	default:
		return toolTypeGeneric // Generic fallback (intentional: different from tool type constants)
	}
}

// Normalizer converts ACP protocol tool data to NormalizedPayload.
type Normalizer struct {
	agentID string
}

// NewNormalizer creates a new ACP normalizer. agentID selects per-agent enrichers
// (e.g. "codex-acp"); pass "" for common-layer-only normalization in tests.
func NewNormalizer(agentID string) *Normalizer {
	return &Normalizer{agentID: agentID}
}

// NormalizeToolCall converts ACP tool call data to NormalizedPayload.
func (n *Normalizer) NormalizeToolCall(toolName string, args map[string]any) *streams.NormalizedPayload {
	// Subagent (Task) tool calls are detected from meta/title/rawInput before
	// the kind switch — they otherwise fall through to normalizeGeneric.
	if payload, ok := n.normalizeSubagent(args); ok {
		return payload
	}

	// ACP uses "kind" field to identify tool type
	kind, _ := args["kind"].(string)
	if kind == "" {
		kind = toolName
	}

	var payload *streams.NormalizedPayload
	switch strings.ToLower(kind) {
	case toolKindEdit:
		payload = n.normalizeEdit(args)
	case toolKindRead, "view":
		payload = n.normalizeRead(args)
	case toolKindExecute, "bash", "run", "shell":
		payload = n.normalizeExecute(args)
	case toolKindGlob, toolKindGrep, toolKindSearch:
		payload = n.normalizeCodeSearch(toolName, args)
	default:
		payload = n.normalizeGeneric(toolName, args)
	}
	applyAgentEnrichment(n.agentID, payload, enrichFrameFromArgs(args))
	return payload
}

// NormalizeToolResult updates the payload with tool result data.
func (n *Normalizer) NormalizeToolResult(payload *streams.NormalizedPayload, result any) {
	// Extract rawOutput.output if result is wrapped
	output := extractRawOutput(result)

	switch payload.Kind() {
	case streams.ToolKindReadFile:
		if payload.ReadFile() != nil && output != "" {
			lines := strings.Count(output, "\n")
			if !strings.HasSuffix(output, "\n") && len(output) > 0 {
				lines++ // Count the last line if it doesn't end with newline
			}
			payload.ReadFile().Output = &streams.ReadFileOutput{
				Content:   output,
				LineCount: lines,
			}
		}
	case streams.ToolKindCodeSearch:
		if payload.CodeSearch() != nil && output != "" {
			// Parse output as file listing (one file per line)
			files := parseFileList(output)
			payload.CodeSearch().Output = &streams.CodeSearchOutput{
				Files:     files,
				FileCount: len(files),
			}
		}
	case streams.ToolKindShellExec:
		if payload.ShellExec() != nil && output != "" {
			// Parse ACP's XML-like shell output format
			exitCode, stdout, stderr := parseShellOutput(output)
			payload.ShellExec().Output = &streams.ShellExecOutput{
				ExitCode: exitCode,
				Stdout:   stdout,
				Stderr:   stderr,
			}
		}
	case streams.ToolKindGeneric:
		if payload.Generic() != nil {
			payload.Generic().Output = result
		}
	}
}

// extractRawOutput gets the output string from ACP result data.
// ACP wraps results in {"rawOutput": {"output": "..."}}
func extractRawOutput(result any) string {
	if result == nil {
		return ""
	}

	// Try direct string
	if s, ok := result.(string); ok {
		return s
	}

	// Try rawOutput.output pattern
	resultMap, ok := result.(map[string]any)
	if !ok {
		return ""
	}

	// Check for rawOutput wrapper
	if rawOutput, ok := resultMap["rawOutput"].(map[string]any); ok {
		if output, ok := rawOutput["output"].(string); ok {
			return output
		}
	}

	// Check for direct output field
	if output, ok := resultMap["output"].(string); ok {
		return output
	}

	return ""
}

// parseFileList parses a newline-separated file listing into a slice of paths.
func parseFileList(output string) []string {
	var files []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip header lines that don't look like paths
		if strings.HasPrefix(line, "Here's") || strings.HasPrefix(line, "Files") {
			continue
		}
		files = append(files, line)
	}
	return files
}

// parseShellOutput parses ACP's XML-like shell output format.
// Format: "...<return-code>N</return-code>...<output>...</output>..."
// Falls back to treating the entire string as stdout when no XML tags are found
// (e.g. Claude Code sends plain string rawOutput).
// Returns exit code, stdout, and stderr (stderr from <stderr> tag if present).
func parseShellOutput(output string) (exitCode int, stdout, stderr string) {
	hasXMLTags := strings.Contains(output, "<return-code>") ||
		strings.Contains(output, "<output>") ||
		strings.Contains(output, "<stderr>")

	if !hasXMLTags {
		// Plain string output (e.g. Claude Code ACP) — treat entire string as stdout
		return 0, strings.TrimSpace(output), ""
	}

	// Extract return code
	if start := strings.Index(output, "<return-code>"); start != -1 {
		start += len("<return-code>")
		if end := strings.Index(output[start:], "</return-code>"); end != -1 {
			codeStr := strings.TrimSpace(output[start : start+end])
			if code, err := strconv.Atoi(codeStr); err == nil {
				exitCode = code
			}
		}
	}

	// Extract stdout from <output> tag
	if start := strings.Index(output, "<output>"); start != -1 {
		start += len("<output>")
		if end := strings.Index(output[start:], "</output>"); end != -1 {
			stdout = strings.TrimSpace(output[start : start+end])
		}
	}

	// Extract stderr from <stderr> tag if present
	if start := strings.Index(output, "<stderr>"); start != -1 {
		start += len("<stderr>")
		if end := strings.Index(output[start:], "</stderr>"); end != -1 {
			stderr = strings.TrimSpace(output[start : start+end])
		}
	}

	return exitCode, stdout, stderr
}

// UpdatePayloadInput updates a stored NormalizedPayload with new rawInput data.
// This handles agents (e.g. Claude Code) that send rawInput incrementally
// via tool_call_update events after the initial tool_call. supplemental may
// carry update-only fields such as locations (OpenCode tool_call_update).
func (n *Normalizer) UpdatePayloadInput(payload *streams.NormalizedPayload, rawInput any, supplemental map[string]any) {
	if payload == nil {
		return
	}
	inputMap, ok := rawInput.(map[string]any)
	if rawInput != nil && !ok {
		return
	}
	if inputMap == nil {
		inputMap = map[string]any{}
	}
	if len(inputMap) == 0 && supplemental == nil {
		return
	}

	if se := payload.ShellExec(); se != nil {
		updateShellExecInput(se, inputMap)
	}
	// Claude ACP sends file_path in incremental rawInput updates; OpenCode uses filePath.
	if mf := payload.ModifyFile(); mf != nil {
		updateModifyFileInput(mf, supplemental, inputMap)
	}
	if rf := payload.ReadFile(); rf != nil {
		updateReadFileInput(rf, supplemental, inputMap)
	}
	if cs := payload.CodeSearch(); cs != nil {
		updateCodeSearchInput(cs, supplemental, inputMap)
	}
	// Subagent (Task) calls send description/prompt/subagent_type in a later
	// tool_call_update rawInput (Claude/OpenCode); fill empty fields only.
	if sa := payload.SubagentTask(); sa != nil {
		updateSubagentTaskInput(sa, inputMap)
	}
}

func updateShellExecInput(se *streams.ShellExecPayload, inputMap map[string]any) {
	if cmd := shared.GetString(inputMap, "command"); cmd != "" && se.Command == "" {
		se.Command = cmd
	}
	if cwd := shared.GetString(inputMap, "cwd"); cwd != "" && se.WorkDir == "" {
		se.WorkDir = cwd
	}
	if desc := shared.GetString(inputMap, "description"); desc != "" && se.Description == "" {
		se.Description = desc
	}
}

func updateModifyFileInput(mf *streams.ModifyFilePayload, supplemental, inputMap map[string]any) {
	if path := pathFromArgs(supplemental, inputMap); path != "" && mf.FilePath == "" {
		mf.FilePath = path
	}
}

// updateReadFileInput fills a ReadFile payload from a tool_call_update: it sets
// FilePath (when still empty) to the selector-stripped path and populates
// Offset/Limit from the parsed line range when those fields are unset.
func updateReadFileInput(rf *streams.ReadFilePayload, supplemental, inputMap map[string]any) {
	// Parse the selector on every update — a later tool_call_update can carry the
	// line range (e.g. "main.go:50+150") even when an earlier frame already set
	// FilePath, so range metadata must not be gated on FilePath being empty.
	path := pathFromArgs(supplemental, inputMap)
	if path == "" {
		return
	}
	files := readselector.SplitFiles(path)
	// Multiple comma-joined files can't share one Offset/Limit; keep the raw path
	// for the UI to split (mirrors normalizeRead).
	if len(files) > 1 {
		// Mirror normalizeRead: a multi-file read keeps the raw comma-joined path
		// (the UI splits it) and carries no single Offset/Limit. Overwrite any
		// stale single-file state from an earlier update that streamed a partial,
		// single-file path before the full multi-file path arrived.
		rf.FilePath = path
		rf.Offset = 0
		rf.Limit = 0
		return
	}
	if rf.FilePath == "" {
		rf.FilePath = files[0].Path
	}
	if rf.Offset == 0 {
		rf.Offset = files[0].StartLine
	}
	if rf.Limit == 0 {
		rf.Limit = files[0].LineCount
	}
}

func updateCodeSearchInput(cs *streams.CodeSearchPayload, supplemental, inputMap map[string]any) {
	if v := stringFromMap(inputMap, "query", "pattern", "search_term"); v != "" && cs.Query == "" {
		cs.Query = v
	}
	if v := stringFromMap(inputMap, "pattern", "glob", "glob_pattern"); v != "" && cs.Pattern == "" && cs.Glob == "" {
		cs.Pattern = v
	}
	if path := pathFromArgs(supplemental, inputMap); path != "" && cs.Path == "" {
		cs.Path = path
	}
}

func updateSubagentTaskInput(sa *streams.SubagentTaskPayload, inputMap map[string]any) {
	if v := shared.GetString(inputMap, subagentKeyDescription); v != "" && sa.Description == "" {
		sa.Description = v
	}
	if v := shared.GetString(inputMap, subagentKeyPrompt); v != "" && sa.Prompt == "" {
		sa.Prompt = v
	}
	if v := shared.GetString(inputMap, subagentKeySubagentType); v != "" && sa.SubagentType == "" {
		sa.SubagentType = v
	}
}

// EnrichFromToolCallUpdate applies agent-specific enrichment from a tool_call_update
// frame (title, meta, rawInput, supplemental locations). Safe to call after
// UpdatePayloadInput; only fills empty normalized fields.
func (n *Normalizer) EnrichFromToolCallUpdate(
	payload *streams.NormalizedPayload,
	title *string,
	meta map[string]any,
	rawInput any,
	supplemental map[string]any,
) {
	applyAgentEnrichment(n.agentID, payload, enrichFrameFromUpdate(title, meta, rawInput, supplemental))
}

// normalizeEdit converts ACP edit tool data.
func (n *Normalizer) normalizeEdit(args map[string]any) *streams.NormalizedPayload {
	rawInput, _ := args["raw_input"].(map[string]any)
	if rawInput == nil {
		rawInput = args
	}

	path := pathFromArgs(args, rawInput)

	var mutations []streams.FileMutation

	// Check if this is file creation (has file_content) vs str_replace
	if fileContent, ok := rawInput["file_content"].(string); ok {
		mutations = append(mutations, streams.FileMutation{
			Type:    streams.MutationCreate,
			Content: fileContent,
		})
	} else {
		// str_replace operation
		// Only include the diff (not old/new content) to reduce payload size
		oldStr, _ := rawInput["old_str_1"].(string)
		newStr, _ := rawInput["new_str_1"].(string)

		mutation := streams.FileMutation{
			Type: streams.MutationPatch,
		}

		// Line numbers vary by agent: Claude's str-replace editor uses
		// old_str_*_line_number_1, OMP uses startLine/endLine. Accept both
		// (plus snake_case) so edit links can navigate to the changed lines.
		mutation.StartLine = firstInt(rawInput, "old_str_start_line_number_1", "startLine", "start_line")
		mutation.EndLine = firstInt(rawInput, "old_str_end_line_number_1", "endLine", "end_line")

		// Generate unified diff when at least one string is provided
		if oldStr != "" || newStr != "" {
			mutation.Diff = shared.GenerateUnifiedDiff(oldStr, newStr, path, mutation.StartLine)
		}

		mutations = append(mutations, mutation)
	}

	// Use factory function
	return streams.NewModifyFile(path, mutations)
}

// firstInt returns the first key whose value is a non-zero int, handling JSON
// numbers (decoded as float64). Line/column numbers are 1-based, so a missing
// or zero value is treated as absent.
func firstInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v := shared.GetInt(m, k); v != 0 {
			return v
		}
	}
	return 0
}

// normalizeRead converts ACP read tool data.
// If rawInput.type is "directory", this becomes a code search (file listing) operation.
func (n *Normalizer) normalizeRead(args map[string]any) *streams.NormalizedPayload {
	rawInput, _ := args["raw_input"].(map[string]any)
	if rawInput == nil {
		rawInput = args
	}

	path := pathFromArgs(args, rawInput)
	// OMP's read tool embeds a line/range/mode selector in the path
	// (e.g. "foo.go:43-94"), and can reference several comma-joined files in a
	// single read ("a.yaml:1-80,b.yaml:1-80"). Split into one entry per file so
	// links stay openable; the range is carried via offset/limit. No-op for
	// other agents.
	files := readselector.SplitFiles(path)

	// Check if this is a directory read - treat as code search (file listing).
	if readType := shared.GetString(rawInput, "type"); readType == readTypeDirectory {
		return streams.NewCodeSearch("", "", files[0].Path, "")
	}

	// A single Offset/Limit cannot describe multiple files, so for a multi-file
	// read keep the raw comma-joined path verbatim and let the UI split it into
	// one link per file.
	if len(files) > 1 {
		return streams.NewReadFile(path, 0, 0)
	}
	return streams.NewReadFile(files[0].Path, files[0].StartLine, files[0].LineCount)
}

// normalizeExecute converts ACP execute/bash tool data.
func (n *Normalizer) normalizeExecute(args map[string]any) *streams.NormalizedPayload {
	rawInput, _ := args["raw_input"].(map[string]any)
	if rawInput == nil {
		rawInput = args
	}

	command := shared.GetString(rawInput, "command")
	workDir := shared.GetString(rawInput, "cwd")
	timeout := shared.GetInt(rawInput, "max_wait_seconds")

	// Background is true if wait is explicitly false
	background := false
	if wait, ok := rawInput["wait"].(bool); ok && !wait {
		background = true
	}

	return streams.NewShellExec(command, workDir, "", timeout, background)
}

// normalizeCodeSearch converts ACP search tool data.
func (n *Normalizer) normalizeCodeSearch(toolName string, args map[string]any) *streams.NormalizedPayload {
	rawInput, _ := args["raw_input"].(map[string]any)
	if rawInput == nil {
		rawInput = args
	}

	path := pathFromArgs(args, rawInput)
	pattern := stringFromMap(rawInput, "pattern", "glob", "glob_pattern")

	var query, glob string
	switch strings.ToLower(toolName) {
	case toolKindGlob:
		glob = pattern
		if glob == "" {
			glob = stringFromMap(rawInput, "query", "search_term")
		}
	case toolKindGrep, toolKindSearch:
		query = stringFromMap(rawInput, "query", "pattern", "search_term", "regex")
	}

	return streams.NewCodeSearch(query, pattern, path, glob)
}

// normalizeGeneric wraps unknown tools as generic.
func (n *Normalizer) normalizeGeneric(toolName string, args map[string]any) *streams.NormalizedPayload {
	// Exclude the adapter-injected subagent-detection keys (title/meta) so
	// internal routing data — notably the raw `_meta.claudeCode` map — never
	// leaks into the generic payload shipped to the client.
	input := make(map[string]any, len(args))
	for k, v := range args {
		if k == argKeyTitle || k == argKeyMeta {
			continue
		}
		input[k] = v
	}
	return streams.NewGeneric(toolName, input)
}

// normalizeSubagent recognizes subagent (Task) tool calls from the meta, title,
// and rawInput the adapter stashes in args. Returns (payload, true) when the
// call spawns a subagent; the initial call usually has empty description/prompt
// /subagent_type — those arrive incrementally via UpdatePayloadInput.
func (n *Normalizer) normalizeSubagent(args map[string]any) (*streams.NormalizedPayload, bool) {
	meta, _ := args[argKeyMeta].(map[string]any)
	title, _ := args[argKeyTitle].(string)
	rawInput := args["raw_input"]
	desc, prompt, subagentType, ok := recognizeSubagent(meta, title, rawInput)
	if !ok {
		return nil, false
	}
	payload := streams.NewSubagentTask(desc, prompt, subagentType)
	// Mark Auggie subagents at recognition time so the result extractor
	// (which reads a generic `rawOutput.output` string) only runs for
	// payloads whose tool_call title actually carried the Auggie prefix.
	if _, _, isAuggie := auggieSubagentTitleFields(title); isAuggie {
		payload.SubagentTask().SetIsAuggie(true)
	}
	return payload, true
}

// EnrichSubagentResult fills the result fields of a subagent payload from a
// completion tool_call_update. Claude's result lives in meta, OpenCode's and
// Cursor's in rawOutput — so this takes both. Auggie's `rawOutput.output` is
// extracted separately and only when the payload was recognized as Auggie at
// tool_call time. No-op for non-subagent payloads.
func (n *Normalizer) EnrichSubagentResult(payload *streams.NormalizedPayload, meta map[string]any, rawOutput any) {
	if payload == nil || payload.Kind() != streams.ToolKindSubagentTask {
		return
	}
	sa := payload.SubagentTask()
	res, ok := extractSubagentResult(meta, rawOutput)
	if sa.IsAuggie() {
		if out, isMap := rawOutput.(map[string]any); isMap {
			if auggieSubagentResult(out, &res) {
				ok = true
			}
		}
	}
	if !ok {
		return
	}
	applySubagentResult(sa, res)
}

// --- Helper functions ---

func stringFromMap(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := shared.GetString(m, key); v != "" {
			return v
		}
	}
	return ""
}

// pathFromArgs resolves a file path from structured ACP fields only (raw_input
// path/file_path/filePath, top-level path, locations).
func pathFromArgs(args, rawInput map[string]any) string {
	if rawInput != nil {
		if p := stringFromMap(rawInput, "path", "file_path", "filePath"); p != "" {
			return p
		}
	}
	if args != nil {
		if p := shared.GetString(args, "path"); p != "" {
			return p
		}
		return extractPathFromLocations(args)
	}
	return ""
}

func extractPathFromLocations(args map[string]any) string {
	if args == nil {
		return ""
	}
	return pathFromLocationSlice(args[keyLocations])
}

// pathFromLocationSlice reads the first path from ACP locations. The adapter
// builds []any on initial tool_call frames and []map[string]any on
// tool_call_update supplemental maps — accept both.
func pathFromLocationSlice(locationsRaw any) string {
	switch locations := locationsRaw.(type) {
	case []any:
		if len(locations) == 0 {
			return ""
		}
		loc, ok := locations[0].(map[string]any)
		if !ok {
			return ""
		}
		path, _ := loc["path"].(string)
		return path
	case []map[string]any:
		if len(locations) == 0 {
			return ""
		}
		path, _ := locations[0]["path"].(string)
		return path
	default:
		return ""
	}
}
