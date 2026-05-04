// Package slack implements the Slack integration: install-wide
// configuration storage, an HTTP client that authenticates as the user via
// the unofficial browser-session credentials (xoxc- token + d cookie), and a
// polling trigger that picks up `!kandev <instruction>` messages and turns
// them into Kandev tasks by handing them to a utility agent that has access
// to Kandev MCP tools.
//
// Slack credentials and triage settings are install-wide singletons (one
// Slack account per Kandev install) — the agent picks the destination Kandev
// workspace per-message via `list_workspaces_kandev`. Only "browser session"
// auth is supported in this v1 slice — bot tokens and user-OAuth modes are
// scoped out per docs/specs/slack-integration/spec.md.
package slack

import "time"

// AuthMethodCookie is the only auth method this slice supports: the xoxc-
// session token plus the `d` cookie that Slack's web client uses.
const AuthMethodCookie = "cookie"

// DefaultCommandPrefix is the marker the trigger looks for at the start of a
// user's own message. Picked because it can't collide with Slack's own slash
// commands (which start with `/`) and is short enough to type.
const DefaultCommandPrefix = "!kandev"

// DefaultPollIntervalSeconds is the cadence for checking Slack for new
// `!kandev …` matches. 30s is a comfortable middle ground: fast enough that
// captures feel responsive, slow enough to stay well inside Slack's
// `search.messages` rate limit (Tier 2 ~20/min).
const DefaultPollIntervalSeconds = 30

// MinPollIntervalSeconds is the floor users can set in the UI. Below 5s the
// `search.messages` cost dominates and a single match's utility-agent run
// takes longer than the polling interval anyway.
const MinPollIntervalSeconds = 5

// MaxPollIntervalSeconds caps the upper end of the slider. 10 minutes is
// long enough for "very low traffic" and short enough that a stuck/expired
// session shows up in the auth-status banner promptly.
const MaxPollIntervalSeconds = 600

// SlackConfig is the install-wide Slack integration configuration. The xoxc
// token and the d cookie live in the encrypted secret store under SecretKeyToken
// and SecretKeyCookie respectively.
type SlackConfig struct {
	AuthMethod    string `json:"authMethod" db:"auth_method"`
	CommandPrefix string `json:"commandPrefix" db:"command_prefix"`
	// UtilityAgentID points at a row in `utility_agents`. The agent is
	// invoked for each Slack match with Kandev MCP wired in so it can call
	// list_workflows_kandev, create_task_kandev, etc. Required — the trigger
	// skips matches when this is empty.
	UtilityAgentID      string `json:"utilityAgentId" db:"utility_agent_id"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds" db:"poll_interval_seconds"`
	// SlackTeamID and SlackUserID are captured from the most recent successful
	// probe so the trigger can scope `search.messages` to the user's own
	// authorship without an extra round-trip per poll.
	SlackTeamID string `json:"slackTeamId,omitempty" db:"slack_team_id"`
	SlackUserID string `json:"slackUserId,omitempty" db:"slack_user_id"`
	// LastSeenTS is the highest Slack message timestamp the trigger has
	// already processed.
	LastSeenTS string `json:"lastSeenTs,omitempty" db:"last_seen_ts"`
	HasToken   bool   `json:"hasToken" db:"-"`
	HasCookie  bool   `json:"hasCookie" db:"-"`
	// LastCheckedAt / LastOk / LastError are written by the auth-health poller.
	LastCheckedAt *time.Time `json:"lastCheckedAt,omitempty" db:"last_checked_at"`
	LastOk        bool       `json:"lastOk" db:"last_ok"`
	LastError     string     `json:"lastError,omitempty" db:"last_error"`
	CreatedAt     time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time  `json:"updatedAt" db:"updated_at"`
}

// SetConfigRequest is the payload sent by the UI. Empty Token / Cookie on
// update keeps the existing stored value; non-empty replaces it.
type SetConfigRequest struct {
	AuthMethod          string `json:"authMethod"`
	CommandPrefix       string `json:"commandPrefix"`
	UtilityAgentID      string `json:"utilityAgentId"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds"`
	Token               string `json:"token"`
	Cookie              string `json:"cookie"`
}

// TestConnectionResult mirrors the Linear shape so the frontend can render
// success/failure with the same component for both integrations.
type TestConnectionResult struct {
	OK          bool   `json:"ok"`
	UserID      string `json:"userId,omitempty"`
	TeamID      string `json:"teamId,omitempty"`
	TeamName    string `json:"teamName,omitempty"`
	URL         string `json:"url,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Error       string `json:"error,omitempty"`
}

// SlackMessage is the minimal shape we extract from Slack's web API for both
// the trigger search and the thread fetch.
type SlackMessage struct {
	TS        string `json:"ts"`
	ThreadTS  string `json:"threadTs,omitempty"`
	ChannelID string `json:"channelId"`
	UserID    string `json:"userId,omitempty"`
	UserName  string `json:"userName,omitempty"`
	Text      string `json:"text"`
	Permalink string `json:"permalink,omitempty"`
}

// ThreadContext bundles a triggering message with the surrounding thread the
// trigger fetched for it.
type ThreadContext struct {
	Trigger  SlackMessage   `json:"trigger"`
	Messages []SlackMessage `json:"messages"`
}

// SecretKeyToken is the secret-store key for the install-wide xoxc- token.
const SecretKeyToken = "slack:singleton:token"

// SecretKeyCookie is the secret-store key for the install-wide `d` cookie.
// Stored separately from the token so each can be rotated/replaced
// independently — Slack rotates the d cookie much more often than the token.
const SecretKeyCookie = "slack:singleton:cookie"

// LegacySecretKeyForToken returns the pre-singleton per-workspace token key,
// kept so the provider can promote an existing per-workspace token onto
// SecretKeyToken during the schema migration.
func LegacySecretKeyForToken(workspaceID string) string {
	return "slack:" + workspaceID + ":token"
}

// LegacySecretKeyForCookie returns the pre-singleton per-workspace cookie key.
func LegacySecretKeyForCookie(workspaceID string) string {
	return "slack:" + workspaceID + ":cookie"
}
