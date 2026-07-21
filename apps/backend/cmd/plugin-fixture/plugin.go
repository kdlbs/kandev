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
	deliveriesFileName     = "deliveries.jsonl"
	webhooksFileName       = "webhooks.jsonl"
	configSnapshotFileName = "config.json"
	secretProbeFileName    = "secret-probe.json"
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

// HandleWebhook appends a webhooks.jsonl line recording the delivery,
// best-effort snapshots the plugin's current operator config to
// config.json (evidence for e2e that the Host GetConfig RPC delivers the
// values set in Settings > Plugins, secrets in cleartext), and responds
// 200 "ok".
func (p *fixturePlugin) HandleWebhook(ctx context.Context, req *pluginsdk.WebhookRequest) (*pluginsdk.WebhookResponse, error) {
	rec := webhookRecord{WebhookKey: req.WebhookKey, Method: req.Method}
	if err := appendJSONLine(filepath.Join(p.dataDir, webhooksFileName), rec); err != nil {
		return nil, err
	}
	p.snapshotConfigBestEffort(ctx)
	p.snapshotSecretProbeBestEffort(ctx)
	return &pluginsdk.WebhookResponse{Status: 200, Body: []byte("ok")}, nil
}

// snapshotSecretProbeBestEffort exercises the plugin-scoped secret
// primitives end to end: SetSecret then GetSecret through the Host, writing
// the read-back value to secret-probe.json as evidence for e2e that a
// plugin-owned secret survives a vault round trip over the real transport.
func (p *fixturePlugin) snapshotSecretProbeBestEffort(ctx context.Context) {
	host := p.Host()
	if host == nil {
		return
	}
	if err := host.SetSecret(ctx, "probe", "s3cret-roundtrip"); err != nil {
		return
	}
	value, found, err := host.GetSecret(ctx, "probe")
	if err != nil || !found {
		return
	}
	data, err := json.Marshal(map[string]string{"probe": value})
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(p.dataDir, secretProbeFileName), data, 0o600)
}

// snapshotConfigBestEffort writes the current Host.GetConfig result to
// config.json (overwriting any previous snapshot). Errors — including "no
// Host injected yet" — are silently ignored: like recordLastEventBestEffort,
// this exists purely as e2e coverage of the Host round trip.
func (p *fixturePlugin) snapshotConfigBestEffort(ctx context.Context) {
	host := p.Host()
	if host == nil {
		return
	}
	config, err := host.GetConfig(ctx)
	if err != nil {
		return
	}
	data, err := json.Marshal(config)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(p.dataDir, configSnapshotFileName), data, 0o600)
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
