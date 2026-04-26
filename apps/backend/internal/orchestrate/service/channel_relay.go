package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// ChannelRelay handles outbound message delivery to external platforms.
type ChannelRelay struct {
	svc    *Service
	client *http.Client
}

// NewChannelRelay creates a new channel relay.
func NewChannelRelay(svc *Service) *ChannelRelay {
	return &ChannelRelay{
		svc:    svc,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// NewChannelRelayWithClient creates a channel relay with a custom HTTP client (for testing).
func NewChannelRelayWithClient(svc *Service, client *http.Client) *ChannelRelay {
	return &ChannelRelay{svc: svc, client: client}
}

// RelayComment checks whether a comment should be relayed to an external
// platform and, if so, sends it via the appropriate platform API.
func (r *ChannelRelay) RelayComment(ctx context.Context, comment *models.TaskComment) error {
	if comment.ReplyChannelID == "" {
		return nil // not a channel comment
	}
	if comment.AuthorType != "agent" {
		return nil // only relay agent comments
	}

	channel, err := r.svc.GetChannelByID(ctx, comment.ReplyChannelID)
	if err != nil {
		return fmt.Errorf("load channel %s: %w", comment.ReplyChannelID, err)
	}

	config, err := parseChannelConfig(channel.Config)
	if err != nil {
		return fmt.Errorf("parse channel config: %w", err)
	}

	sendErr := r.sendWithRetry(ctx, channel.Platform, config, comment.Body)
	if sendErr != nil {
		r.svc.LogActivity(ctx, channel.WorkspaceID, "system", "", "channel.delivery_failed",
			"channel", channel.ID,
			fmt.Sprintf(`{"error":%q,"platform":%q}`, sendErr.Error(), channel.Platform))
		return sendErr
	}

	r.svc.LogActivity(ctx, channel.WorkspaceID, "system", "", "channel.message_relayed",
		"channel", channel.ID,
		fmt.Sprintf(`{"platform":%q}`, channel.Platform))
	return nil
}

// channelConfig holds platform-specific config fields.
type channelConfig struct {
	BotToken   string `json:"bot_token"`
	ChatID     string `json:"chat_id"`
	ChannelID  string `json:"channel_id"`
	WebhookURL string `json:"webhook_url"`
}

func parseChannelConfig(raw string) (*channelConfig, error) {
	var cfg channelConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

const maxRetries = 3

// sendWithRetry dispatches a message with exponential backoff.
func (r *ChannelRelay) sendWithRetry(
	ctx context.Context, platform string, cfg *channelConfig, message string,
) error {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		lastErr = r.dispatch(ctx, platform, cfg, message)
		if lastErr == nil {
			return nil
		}
		r.svc.logger.Warn("channel relay attempt failed",
			zap.String("platform", platform),
			zap.Int("attempt", attempt+1),
			zap.Error(lastErr))
	}
	return fmt.Errorf("relay failed after %d attempts: %w", maxRetries, lastErr)
}

// dispatch routes to the platform-specific sender.
func (r *ChannelRelay) dispatch(
	ctx context.Context, platform string, cfg *channelConfig, message string,
) error {
	switch platform {
	case "telegram":
		return r.sendTelegram(ctx, cfg.BotToken, cfg.ChatID, message)
	case "slack":
		return r.sendSlack(ctx, cfg.BotToken, cfg.ChannelID, message)
	case "discord":
		return r.sendDiscord(ctx, cfg.BotToken, cfg.ChannelID, message)
	case "webhook":
		return r.sendGenericWebhook(ctx, cfg.WebhookURL, message)
	default:
		return fmt.Errorf("unsupported platform: %s", platform)
	}
}

// sendTelegram sends a message via the Telegram Bot API.
func (r *ChannelRelay) sendTelegram(ctx context.Context, botToken, chatID, message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	body := map[string]string{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "Markdown",
	}
	return r.postJSON(ctx, url, nil, body)
}

// sendSlack sends a message via the Slack Web API.
func (r *ChannelRelay) sendSlack(ctx context.Context, botToken, channelID, message string) error {
	headers := map[string]string{
		"Authorization": "Bearer " + botToken,
	}
	body := map[string]string{
		"channel": channelID,
		"text":    message,
	}
	return r.postJSON(ctx, "https://slack.com/api/chat.postMessage", headers, body)
}

// sendDiscord sends a message via the Discord API.
func (r *ChannelRelay) sendDiscord(ctx context.Context, botToken, channelID, message string) error {
	url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)
	headers := map[string]string{
		"Authorization": "Bot " + botToken,
	}
	body := map[string]string{
		"content": message,
	}
	return r.postJSON(ctx, url, headers, body)
}

// sendGenericWebhook sends a message to a generic webhook endpoint.
func (r *ChannelRelay) sendGenericWebhook(ctx context.Context, webhookURL, message string) error {
	body := map[string]string{"text": message}
	return r.postJSON(ctx, webhookURL, nil, body)
}

// postJSON is a helper that POSTs a JSON body and checks for a 2xx response.
func (r *ChannelRelay) postJSON(
	ctx context.Context, url string, headers map[string]string, body interface{},
) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("platform returned HTTP %d", resp.StatusCode)
	}
	return nil
}
