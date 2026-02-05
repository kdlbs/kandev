package streams

import "encoding/json"

// ToolKind categorizes the normalized tool operation
type ToolKind string

const (
	ToolKindReadFile     ToolKind = "read_file"
	ToolKindModifyFile   ToolKind = "modify_file"
	ToolKindShellExec    ToolKind = "shell_exec"
	ToolKindCodeSearch   ToolKind = "code_search"
	ToolKindHttpRequest  ToolKind = "http_request"
	ToolKindGeneric      ToolKind = "generic"
	ToolKindCreateTask   ToolKind = "create_task"
	ToolKindSubagentTask ToolKind = "subagent_task"
	ToolKindShowPlan     ToolKind = "show_plan"
	ToolKindManageTodos  ToolKind = "manage_todos"
	ToolKindMisc         ToolKind = "misc"
)

// ToMessageType maps the ToolKind to a frontend message type.
// This allows the frontend to use specialized renderers for different tool categories.
func (k ToolKind) ToMessageType() string {
	switch k {
	case ToolKindReadFile:
		return "tool_read"
	case ToolKindCodeSearch:
		return "tool_search"
	case ToolKindModifyFile:
		return "tool_edit"
	case ToolKindShellExec:
		return "tool_execute"
	case ToolKindManageTodos:
		return "todo"
	default:
		return "tool_call"
	}
}

// NormalizedPayload is the normalized tool data (discriminated union).
// Exactly one of the kind-specific fields will be set based on Kind.
// Fields are unexported to enforce use of factory functions (NewReadFile, NewShellExec, etc.)
type NormalizedPayload struct {
	kind         ToolKind
	readFile     *ReadFilePayload
	modifyFile   *ModifyFilePayload
	shellExec    *ShellExecPayload
	codeSearch   *CodeSearchPayload
	httpRequest  *HttpRequestPayload
	generic      *GenericPayload
	createTask   *CreateTaskPayload
	subagentTask *SubagentTaskPayload
	showPlan     *ShowPlanPayload
	manageTodos  *ManageTodosPayload
	misc         *MiscPayload
}

// --- Getters for NormalizedPayload ---

func (p *NormalizedPayload) Kind() ToolKind                  { return p.kind }
func (p *NormalizedPayload) ReadFile() *ReadFilePayload      { return p.readFile }
func (p *NormalizedPayload) ModifyFile() *ModifyFilePayload  { return p.modifyFile }
func (p *NormalizedPayload) ShellExec() *ShellExecPayload    { return p.shellExec }
func (p *NormalizedPayload) CodeSearch() *CodeSearchPayload  { return p.codeSearch }
func (p *NormalizedPayload) HttpRequest() *HttpRequestPayload { return p.httpRequest }
func (p *NormalizedPayload) Generic() *GenericPayload        { return p.generic }
func (p *NormalizedPayload) CreateTask() *CreateTaskPayload  { return p.createTask }
func (p *NormalizedPayload) SubagentTask() *SubagentTaskPayload { return p.subagentTask }
func (p *NormalizedPayload) ShowPlan() *ShowPlanPayload      { return p.showPlan }
func (p *NormalizedPayload) ManageTodos() *ManageTodosPayload { return p.manageTodos }
func (p *NormalizedPayload) Misc() *MiscPayload              { return p.misc }

// MarshalJSON implements custom JSON marshaling for NormalizedPayload.
func (p *NormalizedPayload) MarshalJSON() ([]byte, error) {
	type jsonPayload struct {
		Kind         ToolKind             `json:"kind"`
		ReadFile     *ReadFilePayload     `json:"read_file,omitempty"`
		ModifyFile   *ModifyFilePayload   `json:"modify_file,omitempty"`
		ShellExec    *ShellExecPayload    `json:"shell_exec,omitempty"`
		CodeSearch   *CodeSearchPayload   `json:"code_search,omitempty"`
		HttpRequest  *HttpRequestPayload  `json:"http_request,omitempty"`
		Generic      *GenericPayload      `json:"generic,omitempty"`
		CreateTask   *CreateTaskPayload   `json:"create_task,omitempty"`
		SubagentTask *SubagentTaskPayload `json:"subagent_task,omitempty"`
		ShowPlan     *ShowPlanPayload     `json:"show_plan,omitempty"`
		ManageTodos  *ManageTodosPayload  `json:"manage_todos,omitempty"`
		Misc         *MiscPayload         `json:"misc,omitempty"`
	}
	return json.Marshal(jsonPayload{
		Kind:         p.kind,
		ReadFile:     p.readFile,
		ModifyFile:   p.modifyFile,
		ShellExec:    p.shellExec,
		CodeSearch:   p.codeSearch,
		HttpRequest:  p.httpRequest,
		Generic:      p.generic,
		CreateTask:   p.createTask,
		SubagentTask: p.subagentTask,
		ShowPlan:     p.showPlan,
		ManageTodos:  p.manageTodos,
		Misc:         p.misc,
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for NormalizedPayload.
// This is required because the struct has unexported fields.
func (p *NormalizedPayload) UnmarshalJSON(data []byte) error {
	type jsonPayload struct {
		Kind         ToolKind             `json:"kind"`
		ReadFile     *ReadFilePayload     `json:"read_file,omitempty"`
		ModifyFile   *ModifyFilePayload   `json:"modify_file,omitempty"`
		ShellExec    *ShellExecPayload    `json:"shell_exec,omitempty"`
		CodeSearch   *CodeSearchPayload   `json:"code_search,omitempty"`
		HttpRequest  *HttpRequestPayload  `json:"http_request,omitempty"`
		Generic      *GenericPayload      `json:"generic,omitempty"`
		CreateTask   *CreateTaskPayload   `json:"create_task,omitempty"`
		SubagentTask *SubagentTaskPayload `json:"subagent_task,omitempty"`
		ShowPlan     *ShowPlanPayload     `json:"show_plan,omitempty"`
		ManageTodos  *ManageTodosPayload  `json:"manage_todos,omitempty"`
		Misc         *MiscPayload         `json:"misc,omitempty"`
	}
	var jp jsonPayload
	if err := json.Unmarshal(data, &jp); err != nil {
		return err
	}
	p.kind = jp.Kind
	p.readFile = jp.ReadFile
	p.modifyFile = jp.ModifyFile
	p.shellExec = jp.ShellExec
	p.codeSearch = jp.CodeSearch
	p.httpRequest = jp.HttpRequest
	p.generic = jp.Generic
	p.createTask = jp.CreateTask
	p.subagentTask = jp.SubagentTask
	p.showPlan = jp.ShowPlan
	p.manageTodos = jp.ManageTodos
	p.misc = jp.Misc
	return nil
}

// --- Kind-specific payloads ---

// ReadFileOutput contains the result of a file read operation.
type ReadFileOutput struct {
	Content   string `json:"content,omitempty"`
	LineCount int    `json:"line_count,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Language  string `json:"language,omitempty"`
}

// ReadFilePayload contains normalized data for file read operations.
type ReadFilePayload struct {
	FilePath string          `json:"file_path"`
	Offset   int             `json:"offset,omitempty"`
	Limit    int             `json:"limit,omitempty"`
	Output   *ReadFileOutput `json:"output,omitempty"`
}

// ModifyFilePayload contains normalized data for file modification operations.
type ModifyFilePayload struct {
	FilePath  string         `json:"file_path"`
	Mutations []FileMutation `json:"mutations"`
}

// MutationType describes the type of file mutation.
type MutationType string

const (
	MutationCreate  MutationType = "create"
	MutationReplace MutationType = "replace"
	MutationPatch   MutationType = "patch"
	MutationDelete  MutationType = "delete"
	MutationRename  MutationType = "rename"
)

// FileMutation represents a single change to a file.
type FileMutation struct {
	Type       MutationType `json:"type"`
	Content    string       `json:"content,omitempty"`     // for create/replace
	OldContent string       `json:"old_content,omitempty"` // for patch
	NewContent string       `json:"new_content,omitempty"` // for patch
	Diff       string       `json:"diff,omitempty"`        // unified diff
	NewPath    string       `json:"new_path,omitempty"`    // for rename
	StartLine  int          `json:"start_line,omitempty"`
	EndLine    int          `json:"end_line,omitempty"`
}

// ShellExecPayload contains normalized data for shell command execution.
type ShellExecPayload struct {
	Command     string           `json:"command"`
	WorkDir     string           `json:"work_dir,omitempty"`
	Description string           `json:"description,omitempty"`
	Timeout     int              `json:"timeout,omitempty"`
	Background  bool             `json:"background,omitempty"`
	Output      *ShellExecOutput `json:"output,omitempty"`
}

// ShellExecOutput contains the result of a shell command execution.
type ShellExecOutput struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

// CodeSearchOutput contains the result of a code search operation.
type CodeSearchOutput struct {
	Files     []string `json:"files,omitempty"`
	FileCount int      `json:"file_count,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
}

// CodeSearchPayload contains normalized data for code search operations.
type CodeSearchPayload struct {
	Query   string            `json:"query,omitempty"`
	Pattern string            `json:"pattern,omitempty"`
	Path    string            `json:"path,omitempty"`
	Glob    string            `json:"glob,omitempty"`
	Output  *CodeSearchOutput `json:"output,omitempty"`
}

// HttpRequestPayload contains normalized data for HTTP request operations.
type HttpRequestPayload struct {
	URL      string `json:"url"`
	Method   string `json:"method,omitempty"`
	Response string `json:"response,omitempty"`
	IsError  bool   `json:"is_error,omitempty"`
}

// GenericPayload is the fallback for unrecognized tools.
type GenericPayload struct {
	Name   string `json:"name"`
	Input  any    `json:"input,omitempty"`
	Output any    `json:"output,omitempty"`
}

// CreateTaskPayload contains normalized data for task creation operations.
type CreateTaskPayload struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// SubagentTaskPayload contains normalized data for subagent task invocations.
type SubagentTaskPayload struct {
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
	SubagentType string `json:"subagent_type"`
}

// ShowPlanPayload contains normalized data for plan display operations.
type ShowPlanPayload struct {
	Summary string   `json:"summary"`
	Steps   []string `json:"steps,omitempty"`
}

// ManageTodosPayload contains normalized data for todo management operations.
type ManageTodosPayload struct {
	Operation string     `json:"operation"` // "add", "update", "remove", "list"
	Items     []TodoItem `json:"items,omitempty"`
}

// TodoItem represents a single todo item.
type TodoItem struct {
	ID          string `json:"id,omitempty"`
	Description string `json:"description"`
	Status      string `json:"status,omitempty"`
}

// MiscPayload is for miscellaneous operations that don't fit other categories.
type MiscPayload struct {
	Label   string `json:"label"`
	Details any    `json:"details,omitempty"`
}

// --- Factory functions for NormalizedPayload ---
// These are the ONLY way to create NormalizedPayload instances.

// NewReadFile creates a NormalizedPayload for file read operations.
func NewReadFile(filePath string, offset, limit int) *NormalizedPayload {
	return &NormalizedPayload{
		kind: ToolKindReadFile,
		readFile: &ReadFilePayload{
			FilePath: filePath,
			Offset:   offset,
			Limit:    limit,
		},
	}
}

// NewModifyFile creates a NormalizedPayload for file modification operations.
func NewModifyFile(filePath string, mutations []FileMutation) *NormalizedPayload {
	return &NormalizedPayload{
		kind: ToolKindModifyFile,
		modifyFile: &ModifyFilePayload{
			FilePath:  filePath,
			Mutations: mutations,
		},
	}
}

// NewShellExec creates a NormalizedPayload for shell command execution.
func NewShellExec(command, workDir, description string, timeout int, background bool) *NormalizedPayload {
	return &NormalizedPayload{
		kind: ToolKindShellExec,
		shellExec: &ShellExecPayload{
			Command:     command,
			WorkDir:     workDir,
			Description: description,
			Timeout:     timeout,
			Background:  background,
		},
	}
}

// NewCodeSearch creates a NormalizedPayload for code search operations.
func NewCodeSearch(query, pattern, path, glob string) *NormalizedPayload {
	return &NormalizedPayload{
		kind: ToolKindCodeSearch,
		codeSearch: &CodeSearchPayload{
			Query:   query,
			Pattern: pattern,
			Path:    path,
			Glob:    glob,
		},
	}
}

// NewHttpRequest creates a NormalizedPayload for HTTP request operations.
func NewHttpRequest(url, method string) *NormalizedPayload {
	return &NormalizedPayload{
		kind: ToolKindHttpRequest,
		httpRequest: &HttpRequestPayload{
			URL:    url,
			Method: method,
		},
	}
}

// NewGeneric creates a NormalizedPayload for unrecognized tools.
func NewGeneric(name string, input any) *NormalizedPayload {
	return &NormalizedPayload{
		kind: ToolKindGeneric,
		generic: &GenericPayload{
			Name:  name,
			Input: input,
		},
	}
}

// NewCreateTask creates a NormalizedPayload for task creation operations.
func NewCreateTask(title, description string) *NormalizedPayload {
	return &NormalizedPayload{
		kind: ToolKindCreateTask,
		createTask: &CreateTaskPayload{
			Title:       title,
			Description: description,
		},
	}
}

// NewSubagentTask creates a NormalizedPayload for subagent task invocations.
func NewSubagentTask(description, prompt, subagentType string) *NormalizedPayload {
	return &NormalizedPayload{
		kind: ToolKindSubagentTask,
		subagentTask: &SubagentTaskPayload{
			Description:  description,
			Prompt:       prompt,
			SubagentType: subagentType,
		},
	}
}

// NewShowPlan creates a NormalizedPayload for plan display operations.
func NewShowPlan(summary string, steps []string) *NormalizedPayload {
	return &NormalizedPayload{
		kind: ToolKindShowPlan,
		showPlan: &ShowPlanPayload{
			Summary: summary,
			Steps:   steps,
		},
	}
}

// NewManageTodos creates a NormalizedPayload for todo management operations.
func NewManageTodos(operation string, items []TodoItem) *NormalizedPayload {
	return &NormalizedPayload{
		kind: ToolKindManageTodos,
		manageTodos: &ManageTodosPayload{
			Operation: operation,
			Items:     items,
		},
	}
}

// NewMisc creates a NormalizedPayload for miscellaneous operations.
func NewMisc(label string, details any) *NormalizedPayload {
	return &NormalizedPayload{
		kind: ToolKindMisc,
		misc: &MiscPayload{
			Label:   label,
			Details: details,
		},
	}
}
