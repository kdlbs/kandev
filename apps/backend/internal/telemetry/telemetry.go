// Package telemetry implements strictly opt-in product telemetry.
//
// Nothing is ever collected or sent unless the user has explicitly granted
// consent (stored install-wide via internal/system/settings). The stored
// consent is the single user-facing control; two environment conditions
// hard-disable collection before consent is even consulted: the
// DO_NOT_TRACK=1 convention, and e2e test mode (KANDEV_E2E_MOCK), so CI
// runs can never emit real events. When either applies the service starts
// no goroutines, subscribes to nothing, and the HTTP surface reports
// env_disabled so the frontend hides its consent prompts.
//
// Events are allowlisted by name and may only carry enum-like identifier
// properties — never free text, repository names, paths, prompts, or task
// titles. The full event table is documented in docs/public/telemetry.md;
// keep the two in sync when adding events.
package telemetry

import (
	"os"
	"regexp"
	"runtime"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/profiles"
)

// Built-in PostHog EU ingestion defaults. The API key is a write-only
// project key (it can only append events, not read anything), so shipping
// it in the binary is safe and standard practice for product analytics.
// Both can be overridden via KANDEV_TELEMETRY_ENDPOINT / _API_KEY.
const (
	defaultEndpoint = "https://eu.i.posthog.com"
	defaultAPIKey   = "phc_yfDW5DdbvXqDuajS6UQ9gThUTEgY8Wk5qY4SfstUyVvA"
)

// Telemetry event names. snake_case, stable, and documented in
// docs/public/telemetry.md.
const (
	EventInstallHeartbeat  = "install_heartbeat"
	EventTelemetryEnabled  = "telemetry_enabled"
	EventTaskCreated       = "task_created"
	EventTaskDeleted       = "task_deleted"
	EventAgentRunStarted   = "agent_run_started"
	EventAgentRunCompleted = "agent_run_completed"
	EventAgentRunFailed    = "agent_run_failed"
	EventTurnCompleted     = "turn_completed"
	EventWorkspaceCreated  = "workspace_created"
	EventAutomationRun     = "automation_run_created"
	EventUIPageViewed      = "ui_page_viewed"
	EventUIAction          = "ui_action"
	EventFeatureUsed       = "feature_used"
)

// Event is a single allowlisted telemetry event pending delivery.
type Event struct {
	Name       string
	Properties map[string]string
	Timestamp  time.Time
}

// busEventAllowlist maps internal event-bus subjects to the telemetry
// event emitted when they fire. Deliberately name-only: the bus payloads
// carry task titles, repository names, and branch names, none of which
// may ever leave the machine, so the collector counts occurrences and
// forwards nothing from the payload itself.
var busEventAllowlist = map[string]string{
	events.TaskCreated:          EventTaskCreated,
	events.TaskDeleted:          EventTaskDeleted,
	events.AgentStarted:         EventAgentRunStarted,
	events.AgentCompleted:       EventAgentRunCompleted,
	events.AgentFailed:          EventAgentRunFailed,
	events.TurnCompleted:        EventTurnCompleted,
	events.WorkspaceCreated:     EventWorkspaceCreated,
	events.AutomationRunCreated: EventAutomationRun,
}

// uiEventAllowlist declares which events the frontend may submit via
// POST /api/v1/telemetry/events and the single enum property each may
// carry. Anything else is dropped server-side.
var uiEventAllowlist = map[string]string{
	EventUIPageViewed: "page",
	EventUIAction:     "action",
	EventFeatureUsed:  "feature",
}

// safeValueRe bounds UI-submitted property values to short enum-like
// identifiers so free text can never ride along.
var safeValueRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,63}$`)

// maxUIEventsPerRequest caps a single POST /events payload.
const maxUIEventsPerRequest = 20

// sanitizeUIEvent validates a frontend-submitted event against the
// allowlist. Returns the cleaned event and whether it is acceptable.
func sanitizeUIEvent(name string, properties map[string]string) (Event, bool) {
	propKey, ok := uiEventAllowlist[name]
	if !ok {
		return Event{}, false
	}
	value, ok := properties[propKey]
	if !ok || !safeValueRe.MatchString(value) {
		return Event{}, false
	}
	return Event{Name: name, Properties: map[string]string{propKey: value}}, true
}

// EnvDisabled reports whether telemetry is hard-disabled by environment.
// Checked before consent: when true the service never starts and the UI
// hides its consent prompts. DO_NOT_TRACK follows the consoledonottrack.com
// convention; e2e mode piggybacks on the existing KANDEV_E2E_MOCK selector
// so playwright-spawned backends can never emit real events even if a spec
// grants consent. The stored opt-in is otherwise the only control.
func EnvDisabled() bool {
	switch os.Getenv("DO_NOT_TRACK") {
	case "1", "true", "yes", "on":
		return true
	}
	return profiles.DetectEnvironment() == profiles.EnvE2E
}

// detectDeployMode classifies how this install runs, as a coarse enum.
func detectDeployMode() string {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return "k8s"
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "docker"
	}
	if os.Getenv("KANDEV_DESKTOP_HEALTH_TOKEN") != "" {
		return "desktop"
	}
	return "local"
}

// baseProperties returns the coarse context attached to every event.
func (s *Service) baseProperties() map[string]string {
	return map[string]string{
		"app_version": s.opts.Version,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"deploy_mode": s.opts.DeployMode,
	}
}
