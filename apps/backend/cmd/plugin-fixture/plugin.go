// Package main implements fixturePlugin, the pluginsdk.Plugin backing the
// plugin-fixture binary (see the package doc comment in main.go).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	deliveriesFileName = "deliveries.jsonl"
	webhooksFileName   = "webhooks.jsonl"

	toolNameEcho = "echo"
)

// deliveryRecord is one recorded OnEvent delivery, appended as a JSON line
// to deliveries.jsonl. e2e tests poll this file as evidence that an event
// reached the plugin over the real gRPC transport.
type deliveryRecord struct {
	EventType string `json:"event_type"`
	EventID   string `json:"event_id"`
}

// webhookRecord is one recorded HandleWebhook delivery, appended as a JSON
// line to webhooks.jsonl.
type webhookRecord struct {
	WebhookKey string `json:"webhook_key"`
	Method     string `json:"method"`
}

// fixturePlugin implements pluginsdk.Plugin (via UnimplementedPlugin) for
// Go integration tests and Playwright e2e: it records every delivery to
// disk under dataDir so tests can poll for evidence without needing their
// own gRPC client.
type fixturePlugin struct {
	pluginsdk.UnimplementedPlugin

	dataDir string

	mu            sync.Mutex
	sawFirstEvent bool
}

var _ pluginsdk.Plugin = (*fixturePlugin)(nil)

// newFixturePlugin builds a fixturePlugin whose data directory is resolved
// from KANDEV_PLUGIN_DATA_DIR (falling back to the current working
// directory), per §2 of docs/plans/plugins/GRPC-CONTRACT.md.
func newFixturePlugin() *fixturePlugin {
	return &fixturePlugin{dataDir: resolveDataDir()}
}

// resolveDataDir returns KANDEV_PLUGIN_DATA_DIR if set, otherwise the
// current working directory.
func resolveDataDir() string {
	if dir := os.Getenv("KANDEV_PLUGIN_DATA_DIR"); dir != "" {
		return dir
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// OnEvent appends a deliveries.jsonl line recording the event, then — only
// for the first event this process instance has seen — best-effort
// exercises the Host.SetState round trip (errors are ignored; this is
// coverage, not a critical path).
func (p *fixturePlugin) OnEvent(ctx context.Context, e *pluginsdk.Event) error {
	rec := deliveryRecord{EventType: e.EventType, EventID: e.EventID}
	if err := appendJSONLine(filepath.Join(p.dataDir, deliveriesFileName), rec); err != nil {
		return err
	}

	if p.markFirstEvent() {
		p.recordLastEventBestEffort(ctx, e)
	}
	return nil
}

// markFirstEvent returns true exactly once (on the first call), false on
// every subsequent call.
func (p *fixturePlugin) markFirstEvent() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sawFirstEvent {
		return false
	}
	p.sawFirstEvent = true
	return true
}

// recordLastEventBestEffort calls Host.SetState("instance", "",
// "last_event", ...) if a Host has been injected. Errors (including "no
// Host yet") are silently ignored — this exists purely to exercise the
// Host round trip for e2e coverage, not to guarantee delivery.
func (p *fixturePlugin) recordLastEventBestEffort(ctx context.Context, e *pluginsdk.Event) {
	host := p.Host()
	if host == nil {
		return
	}
	_ = host.SetState(ctx, "instance", "", "last_event", map[string]any{
		"event_type": e.EventType,
		"event_id":   e.EventID,
	})
}

// InvokeTool implements the fixture's only declared tool ("echo"): it
// returns the request's input unchanged as the response output.
func (p *fixturePlugin) InvokeTool(_ context.Context, req *pluginsdk.ToolRequest) (*pluginsdk.ToolResponse, error) {
	if req.ToolName != toolNameEcho {
		return &pluginsdk.ToolResponse{Error: fmt.Sprintf("unknown tool %q", req.ToolName)}, nil
	}
	return &pluginsdk.ToolResponse{Output: req.Input}, nil
}

// HandleWebhook appends a webhooks.jsonl line recording the delivery, and
// responds 200 "ok".
func (p *fixturePlugin) HandleWebhook(_ context.Context, req *pluginsdk.WebhookRequest) (*pluginsdk.WebhookResponse, error) {
	rec := webhookRecord{WebhookKey: req.WebhookKey, Method: req.Method}
	if err := appendJSONLine(filepath.Join(p.dataDir, webhooksFileName), rec); err != nil {
		return nil, err
	}
	return &pluginsdk.WebhookResponse{Status: 200, Body: []byte("ok")}, nil
}

// appendJSONLine marshals v to a single JSON line and appends it to path,
// creating path's parent directory and the file itself as needed.
func appendJSONLine(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("plugin-fixture: creating data dir for %s: %w", path, err)
	}

	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("plugin-fixture: marshaling record: %w", err)
	}
	data = append(bytes.TrimRight(data, "\n"), '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("plugin-fixture: opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("plugin-fixture: writing %s: %w", path, err)
	}
	return f.Close()
}
