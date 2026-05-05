package slack

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotConfigured is returned when a Slack operation is attempted without a
// stored configuration or credentials. Wording is install-wide ("not
// configured", not "workspace not configured") because the WS path surfaces
// this raw string to clients via err.Error() without rewriting; the HTTP
// path overrides with its own friendlier message in writeClientError.
var ErrNotConfigured = errors.New("slack: not configured")

// Client is the minimal Slack Web API surface this integration needs. The real
// implementation in cookie_client.go authenticates as the user via the xoxc-
// token + d cookie pair; tests substitute a fake.
type Client interface {
	// AuthTest hits the `auth.test` endpoint, which is the cheapest call
	// that confirms credentials are still valid and returns the
	// authenticated user/team identifiers. Used by the health poller and the
	// connection-test handler.
	AuthTest(ctx context.Context) (*TestConnectionResult, error)

	// SearchMessages runs Slack's `search.messages` with a query string —
	// the trigger uses this to find the user's own messages with the
	// configured command prefix. Returns a flat list of matches.
	SearchMessages(ctx context.Context, query string) ([]SlackMessage, error)

	// ConversationsReplies fetches the full thread for a parent message.
	// When threadTS is empty (the message isn't part of a thread), returns
	// just the single message anchored at triggerTS — implementations must
	// pin to triggerTS, not "the channel's latest message", or concurrent
	// activity in the channel between trigger and processing will give the
	// agent unrelated context.
	ConversationsReplies(ctx context.Context, channelID, threadTS, triggerTS string) ([]SlackMessage, error)

	// ChatGetPermalink resolves a message's canonical Slack URL.
	ChatGetPermalink(ctx context.Context, channelID, ts string) (string, error)

	// ChatPostMessage posts a reply (in cookie mode, as the authenticated
	// user). When threadTS is non-empty, the message is posted in-thread.
	ChatPostMessage(ctx context.Context, channelID, threadTS, text string) error

	// ReactionsAdd adds an emoji reaction to a message. `name` is the bare
	// emoji name (no surrounding colons), e.g. "eyes" or "white_check_mark".
	// Implementations swallow Slack's `already_reacted` error so the trigger
	// can call this idempotently across watermark retries.
	ReactionsAdd(ctx context.Context, channelID, ts, name string) error
}

// APIError captures a Slack API failure. Slack returns 200 with `{"ok": false,
// "error": "..."}` for almost every failure mode, so StatusCode is the HTTP
// code (usually 200) and Message is Slack's `error` string.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("slack api: status %d: %s", e.StatusCode, e.Message)
}
