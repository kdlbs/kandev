package logbundle

import (
	"context"
	"io"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
)

const (
	SourceBackend  = "backend"
	SourceFrontend = "frontend"
	SourceRuntime  = "runtime"
	SourceACP      = "acp"
	maxACPSelect   = 10
	maxRuntimeRows = 500
	maxSessionID   = 256
	maxSessionAge  = 72 * time.Hour
)

// DiagnosticSession is the intentionally small, message-free projection used
// by the runtime index and ACP session picker. TaskTitle is picker-only and
// must remain excluded from archive JSON. Providers must not populate it from
// a generic task/session serialization path.
type DiagnosticSession struct {
	TaskID          string    `json:"task_id"`
	TaskTitle       string    `json:"-"`
	SessionID       string    `json:"session_id"`
	ACPSessionID    string    `json:"-"`
	Agent           string    `json:"agent,omitempty"`
	Provider        string    `json:"provider,omitempty"`
	Model           string    `json:"model,omitempty"`
	Status          string    `json:"status,omitempty"`
	ExecutorType    string    `json:"executor_type,omitempty"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	LastActivityAt  time.Time `json:"last_activity_at,omitempty"`
	ACPAvailability string    `json:"acp_availability,omitempty"`
}

// DiagnosticSessionProvider supplies only sessions visible to the caller.
// selectedIDs lets the provider include explicitly selected sessions that are
// older than the normal three-day catalogue window.
type DiagnosticSessionProvider interface {
	ListDiagnosticSessions(
		ctx context.Context,
		identity authn.Identity,
		since time.Time,
		selectedIDs []string,
	) ([]DiagnosticSession, error)
}

// ACPExporter is an optional executor-side source. It returns a bounded ZIP
// containing only the exact selected ACP session's raw/normalized files.
type ACPExporter interface {
	ExportACP(ctx context.Context, session DiagnosticSession, maxBytes int64) (io.ReadCloser, error)
}

type CapabilitiesView struct {
	Sources         []string `json:"sources"`
	ACPDebugEnabled bool     `json:"acp_debug_enabled"`
	ACPMaxSessions  int      `json:"acp_max_sessions"`
}

func normalizeBundleRequest(sources, sessionIDs []string) ([]string, []string, string, error) {
	if len(sources) == 0 || len(sources) > 4 {
		return nil, nil, "", newError(ErrorInvalid, "sources must select at least one diagnostic source")
	}
	seen := make(map[string]bool, len(sources))
	for _, source := range sources {
		if source != SourceBackend && source != SourceFrontend &&
			source != SourceRuntime && source != SourceACP {
			return nil, nil, "", newError(ErrorInvalid, "unsupported diagnostic source")
		}
		if seen[source] {
			return nil, nil, "", newError(ErrorInvalid, "diagnostic sources must be unique")
		}
		seen[source] = true
	}
	normalizedSources := make([]string, 0, len(seen))
	for source := range seen {
		normalizedSources = append(normalizedSources, source)
	}
	slicesSortStrings(normalizedSources)

	normalizedSessions, err := normalizeSessionIDs(sessionIDs)
	if err != nil {
		return nil, nil, "", err
	}
	if seen[SourceACP] {
		if len(normalizedSessions) == 0 || len(normalizedSessions) > maxACPSelect {
			return nil, nil, "", newError(ErrorInvalid, "ACP bundles require one to ten sessions")
		}
	} else if len(normalizedSessions) > 0 {
		return nil, nil, "", newError(ErrorInvalid, "session IDs require the ACP source")
	}
	key := joinSourceKey(normalizedSources, normalizedSessions)
	return normalizedSources, normalizedSessions, key, nil
}

func normalizeSessionIDs(sessionIDs []string) ([]string, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(sessionIDs))
	result := make([]string, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		if id == "" || len(id) > maxSessionID {
			return nil, newError(ErrorInvalid, "invalid ACP session ID")
		}
		if seen[id] {
			return nil, newError(ErrorInvalid, "ACP session IDs must be unique")
		}
		seen[id] = true
		result = append(result, id)
	}
	slicesSortStrings(result)
	return result, nil
}

func joinSourceKey(sources, sessionIDs []string) string {
	if len(sessionIDs) == 0 {
		return joinStrings(sources, ",")
	}
	return joinStrings(sources, ",") + "\x00" + joinStrings(sessionIDs, ",")
}

// Small local helpers keep this contracts file independent from the service's
// broader collection implementation while preserving deterministic keys.
func slicesSortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func joinStrings(values []string, separator string) string {
	if len(values) == 0 {
		return ""
	}
	result := values[0]
	for _, value := range values[1:] {
		result += separator + value
	}
	return result
}
