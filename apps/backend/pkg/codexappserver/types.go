package codexappserver

import "encoding/json"

const (
	SupportedCodexVersion = "codex-cli 0.154.0"
	SchemaPathV0154       = "schema/v0.154.0/codex_app_server_protocol.v2.schemas.json"
	SchemaSHA256V0154     = "7b9e7d385fffef8d428cc5490b56ce9c393bd3ed7bc7ccd730956387e723ec05"
)

// ToolRequestUserInputParams is the pinned Codex app-server server request
// payload for item/tool/requestUserInput.
type ToolRequestUserInputParams struct {
	ThreadID         string                         `json:"threadId"`
	TurnID           string                         `json:"turnId"`
	ItemID           string                         `json:"itemId"`
	Questions        []ToolRequestUserInputQuestion `json:"questions"`
	IsBlocking       bool                           `json:"isBlocking"`
	AutoResolutionMs *int64                         `json:"autoResolutionMs"`
}

// ToolRequestUserInputQuestion is one question sent by Codex to its client.
type ToolRequestUserInputQuestion struct {
	ID       string                       `json:"id"`
	Header   string                       `json:"header"`
	Question string                       `json:"question"`
	IsOther  bool                         `json:"isOther"`
	IsSecret bool                         `json:"isSecret"`
	Options  []ToolRequestUserInputOption `json:"options"`
}

// ToolRequestUserInputOption is an option offered for a Codex input question.
type ToolRequestUserInputOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// ToolRequestUserInputResponse is the app-server's required answer envelope.
type ToolRequestUserInputResponse struct {
	Answers map[string]ToolRequestUserInputAnswer `json:"answers"`
}

// ToolRequestUserInputAnswer contains the selected labels and/or free text for
// one question.
type ToolRequestUserInputAnswer struct {
	Answers []string `json:"answers"`
}

const (
	MethodInitialize                  = "initialize"
	MethodInitialized                 = "initialized"
	MethodModelList                   = "model/list"
	MethodExperimentalFeatureList     = "experimentalFeature/list"
	MethodThreadStart                 = "thread/start"
	MethodThreadRead                  = "thread/read"
	MethodThreadResume                = "thread/resume"
	MethodThreadFork                  = "thread/fork"
	MethodBackgroundTerminalsList     = "thread/backgroundTerminals/list"
	MethodTurnStart                   = "turn/start"
	MethodTurnInterrupt               = "turn/interrupt"
	MethodMCPStatusList               = "mcpServerStatus/list"
	MethodMCPToolCall                 = "mcpServer/tool/call"
	MethodAccountUsageRead            = "account/usage/read"
	NotificationTurnComplete          = "turn/completed"
	NotificationTokenUsage            = "thread/tokenUsage/updated"
	NotificationRawResponse           = "rawResponse/completed"
	NotificationServerRequestResolved = "serverRequest/resolved"

	ServerRequestCommandExecutionApproval = "item/commandExecution/requestApproval"
	ServerRequestFileChangeApproval       = "item/fileChange/requestApproval"
	ServerRequestToolUserInput            = "item/tool/requestUserInput"
	ServerRequestMCPElicitation           = "mcpServer/elicitation/request"
	ServerRequestPermissionsApproval      = "item/permissions/requestApproval"
	ServerRequestDynamicToolCall          = "item/tool/call"
	ServerRequestAuthTokensRefresh        = "account/chatgptAuthTokens/refresh"
	ServerRequestAttestationGenerate      = "attestation/generate"
	ServerRequestApplyPatchApproval       = "applyPatchApproval"
	ServerRequestExecCommandApproval      = "execCommandApproval"
)

// ServerRequestMethodsV0154 is the complete pinned server-to-client method
// union. Codex CLI's generated v2 JSON schema omits this union; the source is
// the protocol file pinned in schema/v0.154.0/README.md.
func ServerRequestMethodsV0154() []string {
	return []string{
		ServerRequestCommandExecutionApproval,
		ServerRequestFileChangeApproval,
		ServerRequestToolUserInput,
		ServerRequestMCPElicitation,
		ServerRequestPermissionsApproval,
		ServerRequestDynamicToolCall,
		ServerRequestAuthTokensRefresh,
		ServerRequestAttestationGenerate,
		ServerRequestApplyPatchApproval,
		ServerRequestExecCommandApproval,
	}
}

// InitializeParams is the client declaration accepted by the pinned schema.
type InitializeParams struct {
	ClientInfo   ClientInfo             `json:"clientInfo"`
	Capabilities InitializeCapabilities `json:"capabilities"`
}

type ClientInfo struct {
	Name    string  `json:"name"`
	Title   *string `json:"title"`
	Version string  `json:"version"`
}

type InitializeCapabilities struct {
	ExperimentalAPI          bool `json:"experimentalApi"`
	RequestAttestation       bool `json:"requestAttestation"`
	MCPOpenAIFormElicitation bool `json:"mcpServerOpenaiFormElicitation,omitempty"`
}

type InitializeResponse struct {
	UserAgent      string `json:"userAgent"`
	CodexHome      string `json:"codexHome"`
	PlatformOS     string `json:"platformOs"`
	PlatformFamily string `json:"platformFamily"`
}

type ModelListParams struct {
	Cursor        *string `json:"cursor,omitempty"`
	Limit         *int    `json:"limit,omitempty"`
	IncludeHidden bool    `json:"includeHidden,omitempty"`
}

type ModelListResponse struct {
	Data       []Model `json:"data"`
	NextCursor *string `json:"nextCursor"`
}

type ExperimentalFeatureListResponse struct {
	Data       []json.RawMessage `json:"data"`
	NextCursor *string           `json:"nextCursor"`
}

type Model struct {
	ID                        string            `json:"id"`
	Model                     string            `json:"model"`
	DisplayName               string            `json:"displayName"`
	Description               string            `json:"description"`
	SupportedReasoningEfforts []json.RawMessage `json:"supportedReasoningEfforts"`
	DefaultReasoningEffort    json.RawMessage   `json:"defaultReasoningEffort"`
	InputModalities           []string          `json:"inputModalities"`
	SupportsPersonality       bool              `json:"supportsPersonality"`
	ServiceTiers              []json.RawMessage `json:"serviceTiers"`
	IsDefault                 bool              `json:"isDefault"`
}

type ThreadStartParams struct {
	Model          *string        `json:"model,omitempty"`
	ModelProvider  *string        `json:"modelProvider,omitempty"`
	CWD            string         `json:"cwd,omitempty"`
	Ephemeral      *bool          `json:"ephemeral,omitempty"`
	ApprovalPolicy any            `json:"approvalPolicy,omitempty"`
	Sandbox        any            `json:"sandbox,omitempty"`
	Permissions    *string        `json:"permissions,omitempty"`
	Config         map[string]any `json:"config,omitempty"`
}

type ThreadResumeParams struct {
	ThreadID       string         `json:"threadId"`
	Model          *string        `json:"model,omitempty"`
	CWD            string         `json:"cwd,omitempty"`
	ExcludeTurns   bool           `json:"excludeTurns,omitempty"`
	ApprovalPolicy any            `json:"approvalPolicy,omitempty"`
	Sandbox        any            `json:"sandbox,omitempty"`
	Config         map[string]any `json:"config,omitempty"`
}

type ThreadReadParams struct {
	ThreadID     string `json:"threadId"`
	IncludeTurns bool   `json:"includeTurns,omitempty"`
}

type ThreadReadResponse struct {
	Thread Thread `json:"thread"`
}

type BackgroundTerminalsListParams struct {
	ThreadID string `json:"threadId"`
}

type BackgroundTerminal struct {
	Command   string `json:"command"`
	CWD       string `json:"cwd"`
	ItemID    string `json:"itemId"`
	ProcessID string `json:"processId"`
}

type BackgroundTerminalsListResponse struct {
	Data []BackgroundTerminal `json:"data"`
}

type ThreadForkParams struct {
	ThreadID   string  `json:"threadId"`
	LastTurnID *string `json:"lastTurnId,omitempty"`
}

type ThreadForkResponse struct {
	Thread Thread `json:"thread"`
}

type TurnStartParams struct {
	ThreadID string      `json:"threadId"`
	Input    []UserInput `json:"input"`
	Model    *string     `json:"model,omitempty"`
	Effort   *string     `json:"effort,omitempty"`
}

type UserInput struct {
	Type         string `json:"type"`
	Text         string `json:"text,omitempty"`
	TextElements []any  `json:"text_elements,omitempty"`
}

type TurnInterruptParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}

type ThreadResponse struct {
	Thread Thread `json:"thread"`
}

type ThreadStartResponse struct {
	Thread        Thread  `json:"thread"`
	Model         string  `json:"model"`
	ModelProvider string  `json:"modelProvider"`
	ServiceTier   *string `json:"serviceTier"`
	CWD           string  `json:"cwd"`
}

type Thread struct {
	ID             string          `json:"id"`
	Turns          []Turn          `json:"turns,omitempty"`
	Status         json.RawMessage `json:"status,omitempty"`
	ParentThreadID *string         `json:"parentThreadId,omitempty"`
	ForkedFromID   *string         `json:"forkedFromId,omitempty"`
}

type TurnStartResponse struct {
	Turn Turn `json:"turn"`
}

type Turn struct {
	ID          string            `json:"id"`
	Items       []json.RawMessage `json:"items,omitempty"`
	Status      string            `json:"status"`
	Error       json.RawMessage   `json:"error,omitempty"`
	StartedAt   *int64            `json:"startedAt,omitempty"`
	CompletedAt *int64            `json:"completedAt,omitempty"`
}

type MCPServerStatusListParams struct {
	ThreadID *string `json:"threadId,omitempty"`
	Detail   string  `json:"detail,omitempty"`
}

type MCPServerStatusListResponse struct {
	Data []json.RawMessage `json:"data"`
}

type MCPToolCallParams struct {
	ThreadID  string          `json:"threadId"`
	Server    string          `json:"server"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type TokenUsageBreakdown struct {
	TotalTokens           int64 `json:"totalTokens"`
	InputTokens           int64 `json:"inputTokens"`
	CachedInputTokens     int64 `json:"cachedInputTokens"`
	CacheWriteInputTokens int64 `json:"cacheWriteInputTokens"`
	OutputTokens          int64 `json:"outputTokens"`
	ReasoningOutputTokens int64 `json:"reasoningOutputTokens"`
}

type RawResponseCompleted struct {
	ThreadID      string               `json:"threadId"`
	TurnID        string               `json:"turnId"`
	ResponseID    string               `json:"responseId"`
	Usage         *TokenUsageBreakdown `json:"usage"`
	UsageMetadata json.RawMessage      `json:"usageMetadata"`
}

type ThreadTokenUsageUpdated struct {
	ThreadID   string `json:"threadId"`
	TurnID     string `json:"turnId"`
	TokenUsage struct {
		Total              TokenUsageBreakdown `json:"total"`
		Last               TokenUsageBreakdown `json:"last"`
		ModelContextWindow *int64              `json:"modelContextWindow"`
	} `json:"tokenUsage"`
}

type ThreadTokenUsageUpdatedNotification struct {
	ThreadID   string `json:"threadId"`
	TurnID     string `json:"turnId"`
	TokenUsage struct {
		Total              TokenUsageBreakdown `json:"total"`
		Last               TokenUsageBreakdown `json:"last"`
		ModelContextWindow *int64              `json:"modelContextWindow"`
	} `json:"tokenUsage"`
}
