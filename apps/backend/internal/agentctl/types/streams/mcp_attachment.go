package streams

import (
	"context"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

type mcpAttachmentContextKey struct{}

// MCPAttachmentContext is backend-owned identity attached to one new, load,
// reset, or passthrough attempt. It never accepts identity from agent protocol
// frames.
type MCPAttachmentContext struct {
	Attempt MCPAttachmentAttempt
}

// WithMCPAttachmentContext carries an attempt through an adapter call so its
// protocol-level observations can be attributed without creating a side
// channel.
func WithMCPAttachmentContext(ctx context.Context, attempt MCPAttachmentAttempt) context.Context {
	return context.WithValue(ctx, mcpAttachmentContextKey{}, MCPAttachmentContext{Attempt: attempt})
}

// MCPAttachmentContextFromContext returns backend-owned attachment identity.
func MCPAttachmentContextFromContext(ctx context.Context) (MCPAttachmentContext, bool) {
	value, ok := ctx.Value(mcpAttachmentContextKey{}).(MCPAttachmentContext)
	return value, ok && value.Attempt.AttemptID != ""
}

const (
	// MCPAttachmentSchemaVersion versions persisted session attachment metadata.
	MCPAttachmentSchemaVersion = 1

	// MaxMCPAttachmentAttempts retains the current attachment attempt and its
	// two immediate predecessors for release-safe diagnostics.
	MaxMCPAttachmentAttempts = 3

	// MaxMCPAttachmentEvidenceEvents bounds one attempt's diagnostic timeline.
	MaxMCPAttachmentEvidenceEvents = 64

	// MaxMCPAttachmentErrorSummaryBytes caps persisted error summaries after
	// redaction so an agent error cannot inflate session metadata.
	MaxMCPAttachmentErrorSummaryBytes = 256
)

// MCPAttachmentStatus is the strongest user-visible attachment evidence for a
// server in the current attempt. Missing evidence is deliberately not failure.
type MCPAttachmentStatus string

const (
	MCPAttachmentStatusUnknown     MCPAttachmentStatus = "unknown"
	MCPAttachmentStatusDelivered   MCPAttachmentStatus = "delivered"
	MCPAttachmentStatusConnected   MCPAttachmentStatus = "connected"
	MCPAttachmentStatusActive      MCPAttachmentStatus = "active"
	MCPAttachmentStatusFailed      MCPAttachmentStatus = "failed"
	MCPAttachmentStatusFiltered    MCPAttachmentStatus = "filtered"
	MCPAttachmentStatusUnavailable MCPAttachmentStatus = "unavailable"
)

// MCPAttachmentEvidenceKind identifies one safe-to-persist attachment fact.
type MCPAttachmentEvidenceKind string

const (
	MCPAttachmentEvidenceConfigured         MCPAttachmentEvidenceKind = "configured"
	MCPAttachmentEvidenceFiltered           MCPAttachmentEvidenceKind = "filtered"
	MCPAttachmentEvidenceUnavailable        MCPAttachmentEvidenceKind = "unavailable"
	MCPAttachmentEvidenceDelivered          MCPAttachmentEvidenceKind = "delivered"
	MCPAttachmentEvidenceSessionAccepted    MCPAttachmentEvidenceKind = "session_accepted"
	MCPAttachmentEvidenceInitializeObserved MCPAttachmentEvidenceKind = "initialize_observed"
	MCPAttachmentEvidenceToolsListObserved  MCPAttachmentEvidenceKind = "tools_list_observed"
	MCPAttachmentEvidenceToolCallObserved   MCPAttachmentEvidenceKind = "tool_call_observed"
	MCPAttachmentEvidenceExplicitError      MCPAttachmentEvidenceKind = "explicit_error"
	MCPAttachmentEvidenceConnectionClosed   MCPAttachmentEvidenceKind = "connection_closed"
	MCPAttachmentEvidenceAttemptSuperseded  MCPAttachmentEvidenceKind = "attempt_superseded"
)

// MCPServerSource identifies how an MCP server entered an attachment attempt.
type MCPServerSource string

const (
	MCPServerSourceKandev  MCPServerSource = "kandev"
	MCPServerSourceProfile MCPServerSource = "profile"
)

// MCPAttachmentTestResult records a user-requested endpoint reachability test.
// It intentionally remains separate from agent attachment evidence.
type MCPAttachmentTestResult struct {
	Succeeded  bool      `json:"succeeded"`
	TestedAt   time.Time `json:"tested_at"`
	ReasonCode string    `json:"reason_code,omitempty"`
	Summary    string    `json:"summary,omitempty"`
}

// MCPServerAttachment is the safe display projection for one MCP server.
// It deliberately has no headers, environment, command arguments, tool input,
// tool result, or full endpoint URL fields.
type MCPServerAttachment struct {
	Name           string                   `json:"name"`
	Source         MCPServerSource          `json:"source,omitempty"`
	Transport      string                   `json:"transport,omitempty"`
	Target         string                   `json:"target,omitempty"`
	Status         MCPAttachmentStatus      `json:"status"`
	ReasonCode     string                   `json:"reason_code,omitempty"`
	Summary        string                   `json:"summary,omitempty"`
	ConnectionID   string                   `json:"connection_id,omitempty"`
	ToolCount      int                      `json:"tool_count,omitempty"`
	ConfiguredAt   *time.Time               `json:"configured_at,omitempty"`
	DeliveredAt    *time.Time               `json:"delivered_at,omitempty"`
	ConnectedAt    *time.Time               `json:"connected_at,omitempty"`
	ToolsListedAt  *time.Time               `json:"tools_listed_at,omitempty"`
	UsedAt         *time.Time               `json:"used_at,omitempty"`
	FailedAt       *time.Time               `json:"failed_at,omitempty"`
	DisconnectedAt *time.Time               `json:"disconnected_at,omitempty"`
	EndpointTest   *MCPAttachmentTestResult `json:"endpoint_test,omitempty"`
}

// MCPAttachmentEvidence is a safe normalized event. The producer may supply a
// raw network/stdio target or error summary, but Apply persists only the
// sanitized projection.
type MCPAttachmentEvidence struct {
	AttemptID    string                    `json:"attachment_attempt_id"`
	ServerName   string                    `json:"server_name,omitempty"`
	Kind         MCPAttachmentEvidenceKind `json:"kind"`
	OccurredAt   time.Time                 `json:"occurred_at"`
	Source       MCPServerSource           `json:"source,omitempty"`
	Transport    string                    `json:"transport,omitempty"`
	Target       string                    `json:"target,omitempty"`
	ConnectionID string                    `json:"connection_id,omitempty"`
	ToolCount    int                       `json:"tool_count,omitempty"`
	ReasonCode   string                    `json:"reason_code,omitempty"`
	Summary      string                    `json:"summary,omitempty"`
}

// MCPAttachmentAttempt is one ACP new/load/reset or passthrough process start.
type MCPAttachmentAttempt struct {
	AttemptID      string                  `json:"attachment_attempt_id"`
	TaskID         string                  `json:"task_id,omitempty"`
	SessionID      string                  `json:"session_id,omitempty"`
	ExecutionID    string                  `json:"execution_id,omitempty"`
	AgentID        string                  `json:"agent_id,omitempty"`
	AgentProfileID string                  `json:"agent_profile_id,omitempty"`
	ACPSessionID   string                  `json:"acp_session_id,omitempty"`
	StartedAt      time.Time               `json:"started_at"`
	UpdatedAt      time.Time               `json:"updated_at,omitempty"`
	SupersededAt   *time.Time              `json:"superseded_at,omitempty"`
	Servers        []MCPServerAttachment   `json:"servers,omitempty"`
	Evidence       []MCPAttachmentEvidence `json:"evidence,omitempty"`
	SessionError   string                  `json:"session_error,omitempty"`
}

// MCPAttachmentHistory is the bounded persisted state for one Kandev session.
type MCPAttachmentHistory struct {
	Version  int                    `json:"version"`
	Current  MCPAttachmentAttempt   `json:"current"`
	Previous []MCPAttachmentAttempt `json:"previous,omitempty"`
}

// StartAttempt makes attempt current and marks the previous current attempt as
// historical, even when both use the same agent execution.
func (h *MCPAttachmentHistory) StartAttempt(attempt MCPAttachmentAttempt) {
	if attempt.AttemptID == "" {
		return
	}
	now := attempt.StartedAt
	if now.IsZero() {
		now = time.Now().UTC()
		attempt.StartedAt = now
	}
	attempt.UpdatedAt = now
	h.Version = MCPAttachmentSchemaVersion
	if h.Current.AttemptID != "" && h.Current.AttemptID != attempt.AttemptID {
		superseded := h.Current
		superseded.SupersededAt = &now
		superseded.UpdatedAt = now
		h.Previous = append([]MCPAttachmentAttempt{superseded}, h.Previous...)
	}
	h.Current = attempt
	if len(h.Previous) > MaxMCPAttachmentAttempts-1 {
		h.Previous = h.Previous[:MaxMCPAttachmentAttempts-1]
	}
}

// Apply records current-attempt evidence and returns false for stale or empty
// attempts. It is intentionally single-threaded; its lifecycle owner performs
// synchronization before reducing stream events.
func (h *MCPAttachmentHistory) Apply(evidence MCPAttachmentEvidence) bool {
	if h.Current.AttemptID == "" || evidence.AttemptID == "" || evidence.AttemptID != h.Current.AttemptID {
		return false
	}
	evidence = sanitizeMCPAttachmentEvidence(evidence)
	if evidence.OccurredAt.IsZero() {
		evidence.OccurredAt = time.Now().UTC()
	}
	h.Current.UpdatedAt = evidence.OccurredAt
	h.Current.Evidence = appendBoundedMCPAttachmentEvidence(h.Current.Evidence, evidence)
	if evidence.ServerName == "" {
		if evidence.Kind == MCPAttachmentEvidenceExplicitError {
			h.Current.SessionError = evidence.Summary
		}
		return true
	}

	index := mcpAttachmentServerIndex(h.Current.Servers, evidence.ServerName)
	if index == -1 {
		h.Current.Servers = append(h.Current.Servers, MCPServerAttachment{Name: evidence.ServerName, Status: MCPAttachmentStatusUnknown})
		index = len(h.Current.Servers) - 1
	}
	h.Current.Servers[index].applyEvidence(evidence)
	return true
}

// CurrentServer returns the current attempt's server display projection.
func (h MCPAttachmentHistory) CurrentServer(name string) (MCPServerAttachment, bool) {
	index := mcpAttachmentServerIndex(h.Current.Servers, name)
	if index == -1 {
		return MCPServerAttachment{}, false
	}
	return h.Current.Servers[index], true
}

// Valid reports whether a history has the current schema and current attempt.
func (h MCPAttachmentHistory) Valid() bool {
	return h.Version == MCPAttachmentSchemaVersion && h.Current.AttemptID != ""
}

func (s *MCPServerAttachment) applyEvidence(evidence MCPAttachmentEvidence) {
	s.applyEvidenceDetails(evidence)
	s.applyEvidenceStatus(evidence)
}

func (s *MCPServerAttachment) applyEvidenceDetails(evidence MCPAttachmentEvidence) {
	if evidence.Source != "" {
		s.Source = evidence.Source
	}
	if evidence.Transport != "" {
		s.Transport = evidence.Transport
	}
	if evidence.Target != "" {
		s.Target = evidence.Target
	}
	if evidence.ConnectionID != "" && evidence.Kind != MCPAttachmentEvidenceConnectionClosed {
		s.ConnectionID = evidence.ConnectionID
	}
	if evidence.Kind == MCPAttachmentEvidenceToolsListObserved {
		s.ToolCount = evidence.ToolCount
	}
	if evidence.ReasonCode != "" {
		s.ReasonCode = evidence.ReasonCode
	}
	if evidence.Summary != "" {
		s.Summary = evidence.Summary
	}

}

func (s *MCPServerAttachment) applyEvidenceStatus(evidence MCPAttachmentEvidence) {
	when := evidence.OccurredAt
	switch evidence.Kind {
	case MCPAttachmentEvidenceConfigured:
		s.ConfiguredAt = &when
	case MCPAttachmentEvidenceFiltered:
		s.Status = MCPAttachmentStatusFiltered
	case MCPAttachmentEvidenceUnavailable:
		s.Status = MCPAttachmentStatusUnavailable
	case MCPAttachmentEvidenceDelivered, MCPAttachmentEvidenceSessionAccepted:
		s.DeliveredAt = &when
		if s.Status == MCPAttachmentStatusUnknown || s.Status == MCPAttachmentStatusFiltered || s.Status == MCPAttachmentStatusUnavailable {
			s.Status = MCPAttachmentStatusDelivered
		}
	case MCPAttachmentEvidenceInitializeObserved:
		s.applyInitializeObserved(when)
	case MCPAttachmentEvidenceToolsListObserved:
		s.applyToolsListObserved(when)
	case MCPAttachmentEvidenceToolCallObserved:
		s.UsedAt = &when
	case MCPAttachmentEvidenceExplicitError:
		s.FailedAt = &when
		s.Status = MCPAttachmentStatusFailed
	case MCPAttachmentEvidenceConnectionClosed:
		s.applyConnectionClosed(when, evidence.ConnectionID)
	}
}

func (s *MCPServerAttachment) applyInitializeObserved(when time.Time) {
	s.ConnectedAt = &when
	if s.Status != MCPAttachmentStatusActive && s.Status != MCPAttachmentStatusFailed {
		s.Status = MCPAttachmentStatusConnected
	}
}

func (s *MCPServerAttachment) applyToolsListObserved(when time.Time) {
	s.ToolsListedAt = &when
	if s.Status != MCPAttachmentStatusFailed {
		s.Status = MCPAttachmentStatusActive
	}
}

func (s *MCPServerAttachment) applyConnectionClosed(when time.Time, connectionID string) {
	if connectionID == "" || connectionID == s.ConnectionID {
		s.DisconnectedAt = &when
		if s.Status == MCPAttachmentStatusActive {
			s.Status = MCPAttachmentStatusConnected
		}
	}
}

func mcpAttachmentServerIndex(servers []MCPServerAttachment, name string) int {
	for index := range servers {
		if servers[index].Name == name {
			return index
		}
	}
	return -1
}

func appendBoundedMCPAttachmentEvidence(existing []MCPAttachmentEvidence, evidence MCPAttachmentEvidence) []MCPAttachmentEvidence {
	existing = append(existing, evidence)
	if len(existing) > MaxMCPAttachmentEvidenceEvents {
		return existing[len(existing)-MaxMCPAttachmentEvidenceEvents:]
	}
	return existing
}

func sanitizeMCPAttachmentEvidence(evidence MCPAttachmentEvidence) MCPAttachmentEvidence {
	if evidence.Transport == "stdio" {
		evidence.Target = SanitizeMCPStdioTarget(evidence.Target)
	} else if evidence.Target != "" {
		evidence.Target = SanitizeMCPNetworkTarget(evidence.Target)
	}
	evidence.Summary = SanitizeMCPErrorSummary(evidence.Summary)
	return evidence
}

var (
	mcpURLPattern = regexp.MustCompile(`(?i)(?:https?|wss?)://[^\s"'<>]+`)
	// Labeled configuration often carries short secrets that generic token
	// redaction cannot recognize. Drop the label too: names such as API_KEY can
	// reveal more configuration detail than a release diagnostic needs.
	mcpLabeledConfigPattern = regexp.MustCompile(`(?i)\b(?:headers?|env|environment|args?|arguments|command)\s*[:=]\s*\S+`)
	mcpArgumentsPattern     = regexp.MustCompile(`(?i)\b(?:args?|arguments|command)\s*[:=]\s*[^\r\n]*`)
	mcpAbsolutePathPattern  = regexp.MustCompile(`(^|[\s"'(=])/(?:[^\s"'<>]+)`)
)

// SanitizeMCPNetworkTarget retains only a network target's scheme and host.
func SanitizeMCPNetworkTarget(target string) string {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "<invalid endpoint>"
	}
	return parsed.Scheme + "://" + parsed.Host
}

// SanitizeMCPStdioTarget retains only the executable basename. Splitting first
// protects callers that accidentally pass command arguments as part of target.
func SanitizeMCPStdioTarget(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return ""
	}
	return filepath.Base(fields[0])
}

// SanitizeMCPErrorSummary removes credential-like values and full URLs, then
// enforces the smaller MCP attachment persistence cap.
func SanitizeMCPErrorSummary(summary string) string {
	summary = mcpArgumentsPattern.ReplaceAllString(summary, "<redacted configuration>")
	summary = mcpLabeledConfigPattern.ReplaceAllString(summary, "<redacted configuration>")
	summary = routingerr.Sanitize(summary)
	summary = mcpAbsolutePathPattern.ReplaceAllString(summary, "${1}<redacted path>")
	summary = mcpURLPattern.ReplaceAllStringFunc(summary, SanitizeMCPNetworkTarget)
	if len(summary) > MaxMCPAttachmentErrorSummaryBytes {
		return summary[:MaxMCPAttachmentErrorSummaryBytes]
	}
	return summary
}
