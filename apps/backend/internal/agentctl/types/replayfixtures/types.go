// Package replayfixtures is the leaf package for the ACP provider-error
// replay fixture matrix. It has no production dependencies so both the ACP
// transport package and the orchestrator package can import it without an
// import cycle, and it embeds the fixture corpus via go:embed so both test
// packages read one source of truth instead of two drifting copies.
//
// See docs/specs/platform/system-design/provider-error-recovery-02.md for the
// full contract this package implements.
package replayfixtures

// Gateway is the closed enumeration of envelope-shape labels a fixture may
// declare. It is not an adapter selector: all four gateways front the same
// ACP agent, and the fixture's AgentID field carries the actual adapter
// identity.
type Gateway string

const (
	GatewayClaudeDirect Gateway = "claude-direct"
	GatewayTeamClaude   Gateway = "teamclaude"
	GatewayOpenRouter   Gateway = "openrouter"
	GatewayLiteLLM      Gateway = "litellm"
)

// classifyingGateways is the matrix's own table of which gateways have a
// catalogue rule for their terminal envelope today. The loader rejects a
// fixture whose declared Classifying disagrees with this table.
var classifyingGateways = map[Gateway]bool{
	GatewayClaudeDirect: true,
	GatewayTeamClaude:   true,
	GatewayOpenRouter:   false,
	GatewayLiteLLM:      false,
}

// knownAgentIDs is the closed enumeration of backend agent identities a
// fixture's AgentID may declare. All four gateways front the same ACP agent
// (see Gateway's doc comment); a fixture names which one it replays through.
var knownAgentIDs = map[string]bool{
	"claude-acp":   true,
	"codex-acp":    true,
	"opencode-acp": true,
	"grok-acp":     true,
}

// Case is the closed enumeration of transport cases a fixture may declare.
type Case string

const (
	CaseMatched      Case = "matched"
	CaseMismatched   Case = "mismatched"
	CaseLaterOutput  Case = "later-output"
	CaseQueued       Case = "queued"
	CaseProseMatched Case = "prose-matched"
	CaseProseTool    Case = "prose-tool"
	CaseProseOutput  Case = "prose-output"
)

// requiredForAllGateways are the cases every gateway (classifying or not)
// must provide a fixture for.
var requiredForAllGateways = []Case{CaseMatched, CaseMismatched, CaseLaterOutput, CaseQueued}

// requiredForClassifyingGateways are the additional cases a classifying
// gateway must provide; they are vacuous for a non-classifying gateway, whose
// terminal envelope never classifies, so they are not required there.
var requiredForClassifyingGateways = []Case{CaseProseMatched, CaseProseTool, CaseProseOutput}

// Capture declares how a fixture's frames were produced.
type Capture string

const (
	CaptureRecorded      Capture = "recorded"
	CaptureReconstructed Capture = "reconstructed"
)

// FrameKind is the closed enumeration of frame kinds a fixture's Frames may
// contain.
type FrameKind string

const (
	FrameMessageChunk FrameKind = "message_chunk"
	FrameThoughtChunk FrameKind = "thought_chunk"
	FrameToolCall     FrameKind = "tool_call"
	FrameToolUpdate   FrameKind = "tool_update"
	FrameModelSettled FrameKind = "model_settled"
	FramePromptError  FrameKind = "prompt_error"
)

// Frame is one entry in a fixture's ordered Frames array. Which fields are
// meaningful depends on Kind; see the frame table in
// provider-error-recovery-02.md#fixture-document.
type Frame struct {
	Kind FrameKind `json:"kind"`

	// message_chunk only.
	Role string `json:"role,omitempty"`
	// message_chunk, thought_chunk.
	Text string `json:"text,omitempty"`

	// tool_call, tool_update.
	ToolCallID string `json:"toolCallId,omitempty"`
	// tool_call only.
	Title string `json:"title,omitempty"`
	// tool_call, tool_update.
	Status string `json:"status,omitempty"`

	// model_settled only.
	ModelID string `json:"modelId,omitempty"`

	// prompt_error only.
	Code    int            `json:"code,omitempty"`
	Message string         `json:"message,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

// Identity carries the recovery-evidence layer's fencing keys. Required on
// every fixture: promptAttemptPreResultSafe rejects an empty SessionID, an
// empty ExecutionID, or a zero PromptGeneration, so a fixture missing any of
// these could only ever assert PreResultSafe: false, for the wrong reason.
type Identity struct {
	SessionID        string `json:"sessionId"`
	ExecutionID      string `json:"executionId"`
	PromptGeneration uint64 `json:"promptGeneration"`
}

// ProviderErrorExpectation is the allowlisted-metadata projection a fixture
// asserts. ErrorKind and ModelID default to absent (the zero value) when
// omitted from the JSON document; that omission is itself an assertion of
// absence, never "don't care".
type ProviderErrorExpectation struct {
	Source     string `json:"source"`
	RPCCode    int    `json:"rpcCode"`
	ErrorKind  string `json:"errorKind,omitempty"`
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelId,omitempty"`
}

// Expect is a fixture's full declared outcome. Every field is required
// except ProviderError.ErrorKind and ProviderError.ModelID.
type Expect struct {
	Events         []string                 `json:"events"`
	ProviderError  ProviderErrorExpectation `json:"providerError"`
	DiagnosticCode string                   `json:"diagnosticCode"`
	PreResultSafe  bool                     `json:"preResultSafe"`
	// RecordedDiagnosticCode is the recovery-evidence layer's own recorded
	// diagnostic code after replaying every frame but before the terminal
	// prompt_error is evaluated. Empty means no diagnostic is recorded at
	// that point — either none classified during replay, or a later
	// unmarked chunk cleared it. Non-empty means one is still recorded,
	// independent of whether it will go on to satisfy containment against
	// the terminal message. Declared explicitly rather than defaulting so a
	// fixture cannot assert PreResultSafe without also pinning why.
	RecordedDiagnosticCode string `json:"recordedDiagnosticCode"`
}

// Fixture is one replay fixture document: one JSON file, one (Gateway, Case)
// identity.
type Fixture struct {
	Gateway     Gateway  `json:"gateway"`
	AgentID     string   `json:"agentId"`
	Classifying bool     `json:"classifying"`
	Case        Case     `json:"case"`
	Capture     Capture  `json:"capture"`
	CapturedAt  string   `json:"capturedAt,omitempty"`
	Source      string   `json:"source"`
	Identity    Identity `json:"identity"`
	Frames      []Frame  `json:"frames"`
	Expect      Expect   `json:"expect"`

	// FileName is the loader-assigned corpus file name (e.g.
	// "teamclaude-matched.json"), not part of the JSON document itself. Tests
	// use it to identify a fixture in failure messages.
	FileName string `json:"-"`
}
