package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Sink delivers a batch of events keyed by the anonymous install ID.
// The production implementation posts to PostHog; tests inject fakes.
type Sink interface {
	Send(ctx context.Context, distinctID string, events []Event) error
}

// postHogSink posts batches to the PostHog /batch/ endpoint using the
// write-only project API key. Events are sent in anonymous mode
// ($process_person_profile: false) so no person profile is ever built.
type postHogSink struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

func newPostHogSink(endpoint, apiKey string) *postHogSink {
	return &postHogSink{
		endpoint: strings.TrimRight(endpoint, "/"),
		apiKey:   apiKey,
		client:   &http.Client{Timeout: sendTimeout},
	}
}

type postHogItem struct {
	Event      string         `json:"event"`
	DistinctID string         `json:"distinct_id"`
	Timestamp  string         `json:"timestamp"`
	Properties map[string]any `json:"properties"`
}

func (p *postHogSink) Send(ctx context.Context, distinctID string, events []Event) error {
	items := make([]postHogItem, 0, len(events))
	for _, event := range events {
		properties := map[string]any{
			// Singular per PostHog's capture API: $process_person_profile
			// keeps events personless so no person profiles are created.
			"$process_person_profile": false,
			"$lib":                    "kandev",
		}
		for k, v := range event.Properties {
			properties[k] = v
		}
		items = append(items, postHogItem{
			Event:      event.Name,
			DistinctID: distinctID,
			Timestamp:  event.Timestamp.UTC().Format(time.RFC3339),
			Properties: properties,
		})
	}
	body, err := json.Marshal(map[string]any{"api_key": p.apiKey, "batch": items})
	if err != nil {
		return fmt.Errorf("marshal telemetry batch: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/batch/", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build telemetry request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("post telemetry batch: %w", err)
	}
	defer func() {
		// Drain (bounded) so the keep-alive connection is reusable.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("telemetry endpoint returned status %d", resp.StatusCode)
	}
	return nil
}
