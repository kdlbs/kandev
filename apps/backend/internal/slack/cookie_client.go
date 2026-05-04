package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SlackAPIBase is the single host for Slack's web API. The same base is used
// regardless of the user's workspace — auth scopes the request.
const SlackAPIBase = "https://slack.com/api"

const userAgent = "kandev/1.0 (+https://github.com/kdlbs/kandev)"

// CookieClient implements Client by speaking the same Web API the Slack
// browser app uses. The xoxc- session token goes in `Authorization: Bearer
// <token>` and the d cookie is sent as a `Cookie` header — Slack rejects the
// token alone for the endpoints we need.
type CookieClient struct {
	http        *http.Client
	endpoint    string
	token       string
	cookie      string
	maxBodySize int64
}

// NewCookieClient builds a client. cfg is reserved so the signature matches
// future bot/oauth clients; it isn't read today.
func NewCookieClient(_ *SlackConfig, token, cookie string) *CookieClient {
	return &CookieClient{
		http:        &http.Client{Timeout: 30 * time.Second},
		endpoint:    SlackAPIBase,
		token:       token,
		cookie:      cookie,
		maxBodySize: 8 << 20,
	}
}

// slackEnvelope captures the `{ok, error}` shape every Slack endpoint returns.
type slackEnvelope struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// post sends a form-encoded POST to /api/<method>, decodes the JSON envelope,
// and unmarshals the full body into `out` if it isn't nil. Slack expects
// form-encoded bodies (not JSON) for almost every endpoint.
func (c *CookieClient) post(ctx context.Context, method string, params url.Values, out interface{}) error {
	if c.token == "" || c.cookie == "" {
		return ErrNotConfigured
	}
	body := strings.NewReader(params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/"+method, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Cookie", "d="+c.cookie)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBodySize))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Message: summarizeBody(raw)}
	}
	var env slackEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return &APIError{StatusCode: resp.StatusCode, Message: "invalid Slack response: " + err.Error()}
	}
	if !env.OK {
		return &APIError{StatusCode: resp.StatusCode, Message: env.Error}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func summarizeBody(raw []byte) string {
	const maxMsg = 500
	s := strings.TrimSpace(string(raw))
	if len(s) > maxMsg {
		return s[:maxMsg] + "…"
	}
	if s == "" {
		return "(empty body)"
	}
	return s
}

// --- auth.test ---

type authTestResponse struct {
	slackEnvelope
	URL    string `json:"url"`
	Team   string `json:"team"`
	User   string `json:"user"`
	TeamID string `json:"team_id"`
	UserID string `json:"user_id"`
}

// AuthTest implements Client. Slack's auth.test takes no parameters and
// returns the authenticated user + team. Used as the cheapest probe.
func (c *CookieClient) AuthTest(ctx context.Context) (*TestConnectionResult, error) {
	var resp authTestResponse
	if err := c.post(ctx, "auth.test", url.Values{}, &resp); err != nil {
		var apiErr *APIError
		if asAPIErr(err, &apiErr) {
			return &TestConnectionResult{OK: false, Error: apiErr.Message}, nil
		}
		return &TestConnectionResult{OK: false, Error: err.Error()}, nil
	}
	return &TestConnectionResult{
		OK:          true,
		UserID:      resp.UserID,
		TeamID:      resp.TeamID,
		TeamName:    resp.Team,
		URL:         resp.URL,
		DisplayName: resp.User,
	}, nil
}

// asAPIErr is a small wrapper over errors.As without the import cycle of
// importing errors twice.
func asAPIErr(err error, target **APIError) bool {
	for cur := err; cur != nil; {
		if e, ok := cur.(*APIError); ok {
			*target = e
			return true
		}
		type unwrap interface{ Unwrap() error }
		u, ok := cur.(unwrap)
		if !ok {
			return false
		}
		cur = u.Unwrap()
	}
	return false
}

// --- search.messages ---

type searchMessagesResponse struct {
	slackEnvelope
	Messages struct {
		Matches []searchMatch `json:"matches"`
	} `json:"messages"`
}

type searchMatch struct {
	TS        string `json:"ts"`
	Text      string `json:"text"`
	User      string `json:"user"`
	Username  string `json:"username"`
	Permalink string `json:"permalink"`
	Channel   struct {
		ID string `json:"id"`
	} `json:"channel"`
	// Slack's search.messages returns thread context when the matched message
	// is inside a thread. We only need the parent ts here — the trigger fetches
	// the full thread separately via conversations.replies.
	ThreadTS string `json:"thread_ts,omitempty"`
}

// SearchMessages implements Client.
func (c *CookieClient) SearchMessages(ctx context.Context, query string) ([]SlackMessage, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("count", "30")
	// Newest first so the trigger sees the most recent matches first; the
	// watermark logic in trigger.go skips anything older than last_seen_ts.
	params.Set("sort", "timestamp")
	params.Set("sort_dir", "desc")
	var resp searchMessagesResponse
	if err := c.post(ctx, "search.messages", params, &resp); err != nil {
		return nil, err
	}
	out := make([]SlackMessage, 0, len(resp.Messages.Matches))
	for _, m := range resp.Messages.Matches {
		out = append(out, SlackMessage{
			TS:        m.TS,
			ThreadTS:  m.ThreadTS,
			ChannelID: m.Channel.ID,
			UserID:    m.User,
			UserName:  m.Username,
			Text:      m.Text,
			Permalink: m.Permalink,
		})
	}
	return out, nil
}

// --- conversations.replies ---

type conversationsRepliesResponse struct {
	slackEnvelope
	Messages []replyMessage `json:"messages"`
}

type replyMessage struct {
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts,omitempty"`
	User     string `json:"user"`
	Username string `json:"username"`
	Text     string `json:"text"`
}

// ConversationsReplies implements Client. When threadTS is empty, falls back
// to a single-message read pinned to triggerTS — Slack's conversations.history
// with `oldest=ts, latest=ts, inclusive=true, limit=1` returns exactly that
// message regardless of activity in the channel between trigger detection and
// processing.
func (c *CookieClient) ConversationsReplies(ctx context.Context, channelID, threadTS, triggerTS string) ([]SlackMessage, error) {
	if channelID == "" {
		return nil, fmt.Errorf("channel id required")
	}
	if threadTS == "" {
		return c.historyAt(ctx, channelID, triggerTS)
	}
	params := url.Values{}
	params.Set("channel", channelID)
	params.Set("ts", threadTS)
	params.Set("limit", "100")
	var resp conversationsRepliesResponse
	if err := c.post(ctx, "conversations.replies", params, &resp); err != nil {
		return nil, err
	}
	out := make([]SlackMessage, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		out = append(out, SlackMessage{
			TS:        m.TS,
			ThreadTS:  m.ThreadTS,
			ChannelID: channelID,
			UserID:    m.User,
			UserName:  m.Username,
			Text:      m.Text,
		})
	}
	return out, nil
}

// historyAt returns exactly the message at the given timestamp via
// conversations.history with the timestamp window pinned to ts on both sides
// and inclusive=true. Used as the non-threaded fallback for
// ConversationsReplies; an empty ts returns nothing rather than picking up
// whatever just landed in the channel.
func (c *CookieClient) historyAt(ctx context.Context, channelID, ts string) ([]SlackMessage, error) {
	if ts == "" {
		return nil, nil
	}
	params := url.Values{}
	params.Set("channel", channelID)
	params.Set("oldest", ts)
	params.Set("latest", ts)
	params.Set("inclusive", "true")
	params.Set("limit", "1")
	var resp conversationsRepliesResponse
	if err := c.post(ctx, "conversations.history", params, &resp); err != nil {
		return nil, err
	}
	out := make([]SlackMessage, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		out = append(out, SlackMessage{
			TS:        m.TS,
			ThreadTS:  m.ThreadTS,
			ChannelID: channelID,
			UserID:    m.User,
			UserName:  m.Username,
			Text:      m.Text,
		})
	}
	return out, nil
}

// --- chat.getPermalink ---

type permalinkResponse struct {
	slackEnvelope
	Permalink string `json:"permalink"`
}

// ChatGetPermalink implements Client.
func (c *CookieClient) ChatGetPermalink(ctx context.Context, channelID, ts string) (string, error) {
	params := url.Values{}
	params.Set("channel", channelID)
	params.Set("message_ts", ts)
	var resp permalinkResponse
	if err := c.post(ctx, "chat.getPermalink", params, &resp); err != nil {
		return "", err
	}
	return resp.Permalink, nil
}

// --- chat.postMessage ---

// ChatPostMessage implements Client. Posts as the authenticated user since the
// cookie auth has no bot identity.
func (c *CookieClient) ChatPostMessage(ctx context.Context, channelID, threadTS, text string) error {
	params := url.Values{}
	params.Set("channel", channelID)
	params.Set("text", text)
	if threadTS != "" {
		params.Set("thread_ts", threadTS)
	}
	return c.post(ctx, "chat.postMessage", params, nil)
}

// ReactionsAdd implements Client. Slack returns `already_reacted` (a non-2xx-
// shaped error inside a 2xx envelope) when the reaction is already present;
// we swallow that so the trigger can call this idempotently after a restart
// or watermark-retry without surfacing a misleading "failed" log line.
func (c *CookieClient) ReactionsAdd(ctx context.Context, channelID, ts, name string) error {
	params := url.Values{}
	params.Set("channel", channelID)
	params.Set("timestamp", ts)
	params.Set("name", name)
	err := c.post(ctx, "reactions.add", params, nil)
	if err == nil {
		return nil
	}
	var apiErr *APIError
	if asAPIErr(err, &apiErr) && apiErr.Message == "already_reacted" {
		return nil
	}
	return err
}
