package streams

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

// NormalizedPayload is the normalized tool data (discriminated union).
// Exactly one of the kind-specific fields will be set based on Kind.
type NormalizedPayload struct {
	Kind ToolKind `json:"kind"`

	// read_file
	ReadFile *ReadFilePayload `json:"read_file,omitempty"`

	// modify_file
	ModifyFile *ModifyFilePayload `json:"modify_file,omitempty"`

	// shell_exec
	ShellExec *ShellExecPayload `json:"shell_exec,omitempty"`

	// code_search
	CodeSearch *CodeSearchPayload `json:"code_search,omitempty"`

	// http_request
	HttpRequest *HttpRequestPayload `json:"http_request,omitempty"`

	// generic (fallback)
	Generic *GenericPayload `json:"generic,omitempty"`

	// create_task
	CreateTask *CreateTaskPayload `json:"create_task,omitempty"`

	// subagent_task
	SubagentTask *SubagentTaskPayload `json:"subagent_task,omitempty"`

	// show_plan
	ShowPlan *ShowPlanPayload `json:"show_plan,omitempty"`

	// manage_todos
	ManageTodos *ManageTodosPayload `json:"manage_todos,omitempty"`

	// misc
	Misc *MiscPayload `json:"misc,omitempty"`
}

// --- Kind-specific payloads ---

// ReadFilePayload contains normalized data for file read operations.
type ReadFilePayload struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
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

// CodeSearchPayload contains normalized data for code search operations.
type CodeSearchPayload struct {
	Query   string `json:"query,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Path    string `json:"path,omitempty"`
	Glob    string `json:"glob,omitempty"`
}

// HttpRequestPayload contains normalized data for HTTP request operations.
type HttpRequestPayload struct {
	URL      string `json:"url"`
	Method   string `json:"method,omitempty"`
	Response string `json:"response,omitempty"`
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

// --- Constructor functions for NormalizedPayload ---

// NewReadFile creates a NormalizedPayload for file read operations.
func NewReadFile(filePath string, offset, limit int) *NormalizedPayload {
	return &NormalizedPayload{
		Kind: ToolKindReadFile,
		ReadFile: &ReadFilePayload{
			FilePath: filePath,
			Offset:   offset,
			Limit:    limit,
		},
	}
}

// NewModifyFile creates a NormalizedPayload for file modification operations.
func NewModifyFile(filePath string, mutations []FileMutation) *NormalizedPayload {
	return &NormalizedPayload{
		Kind: ToolKindModifyFile,
		ModifyFile: &ModifyFilePayload{
			FilePath:  filePath,
			Mutations: mutations,
		},
	}
}

// NewShellExec creates a NormalizedPayload for shell command execution.
func NewShellExec(command, workDir, description string, timeout int, background bool) *NormalizedPayload {
	return &NormalizedPayload{
		Kind: ToolKindShellExec,
		ShellExec: &ShellExecPayload{
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
		Kind: ToolKindCodeSearch,
		CodeSearch: &CodeSearchPayload{
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
		Kind: ToolKindHttpRequest,
		HttpRequest: &HttpRequestPayload{
			URL:    url,
			Method: method,
		},
	}
}

// NewGeneric creates a NormalizedPayload for unrecognized tools.
func NewGeneric(name string, input any) *NormalizedPayload {
	return &NormalizedPayload{
		Kind: ToolKindGeneric,
		Generic: &GenericPayload{
			Name:  name,
			Input: input,
		},
	}
}

// NewCreateTask creates a NormalizedPayload for task creation operations.
func NewCreateTask(title, description string) *NormalizedPayload {
	return &NormalizedPayload{
		Kind: ToolKindCreateTask,
		CreateTask: &CreateTaskPayload{
			Title:       title,
			Description: description,
		},
	}
}

// NewSubagentTask creates a NormalizedPayload for subagent task invocations.
func NewSubagentTask(description, prompt, subagentType string) *NormalizedPayload {
	return &NormalizedPayload{
		Kind: ToolKindSubagentTask,
		SubagentTask: &SubagentTaskPayload{
			Description:  description,
			Prompt:       prompt,
			SubagentType: subagentType,
		},
	}
}

// NewShowPlan creates a NormalizedPayload for plan display operations.
func NewShowPlan(summary string, steps []string) *NormalizedPayload {
	return &NormalizedPayload{
		Kind: ToolKindShowPlan,
		ShowPlan: &ShowPlanPayload{
			Summary: summary,
			Steps:   steps,
		},
	}
}

// NewManageTodos creates a NormalizedPayload for todo management operations.
func NewManageTodos(operation string, items []TodoItem) *NormalizedPayload {
	return &NormalizedPayload{
		Kind: ToolKindManageTodos,
		ManageTodos: &ManageTodosPayload{
			Operation: operation,
			Items:     items,
		},
	}
}

// NewMisc creates a NormalizedPayload for miscellaneous operations.
func NewMisc(label string, details any) *NormalizedPayload {
	return &NormalizedPayload{
		Kind: ToolKindMisc,
		Misc: &MiscPayload{
			Label:   label,
			Details: details,
		},
	}
}
