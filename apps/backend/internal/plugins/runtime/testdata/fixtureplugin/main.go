// Command fixtureplugin is a minimal kandev plugin backend built from
// pkg/pluginsdk, used only by internal/plugins/runtime's tests to exercise
// a real go-plugin subprocess spawn/handshake/DeliverEvent/crash-restart
// cycle end to end. It is built at test time (see
// internal/plugins/runtime/manager_test.go's TestMain) into a temp binary;
// it is never shipped.
package main

import (
	"context"
	"os"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

// fixturePlugin embeds UnimplementedPlugin (for Host injection via
// SetHost/Host) and overrides every RPC with test-observable behavior:
//   - OnEvent records the last delivered event into Host state, so an
//     out-of-process test can observe delivery by reading that state back
//     through its own fake Host implementation.
//   - HandleWebhook echoes the request body back as the response body,
//     except webhook key "crash" which exits the process immediately (to
//     exercise crash detection + restart), and webhook key "write" which
//     drives the Host data API write RPCs (CreateTask then SendMessage to the
//     returned task) so a test can observe them arrive at the Host over the
//     real subprocess transport.
type fixturePlugin struct {
	pluginsdk.UnimplementedPlugin
}

func (p *fixturePlugin) OnEvent(ctx context.Context, e *pluginsdk.Event) error {
	host := p.Host()
	if host == nil {
		return nil
	}
	return host.SetState(ctx, "instance", "", "last_event", map[string]any{
		"event_type": e.EventType,
		"event_id":   e.EventID,
	})
}

func (p *fixturePlugin) HandleWebhook(ctx context.Context, req *pluginsdk.WebhookRequest) (*pluginsdk.WebhookResponse, error) {
	if req.WebhookKey == "crash" {
		os.Exit(1)
	}
	if req.WebhookKey == "write" {
		return p.handleWrite(ctx)
	}
	return &pluginsdk.WebhookResponse{Status: 200, Body: req.Body}, nil
}

// handleWrite exercises the Host data API write RPCs end to end: CreateTask,
// then SendMessage to the task the Host returns. It responds "ok" on success so
// the test can assert the round-trip completed; a Host error is surfaced as a
// 500 with the error text.
func (p *fixturePlugin) handleWrite(ctx context.Context) (*pluginsdk.WebhookResponse, error) {
	host := p.Host()
	if host == nil {
		return &pluginsdk.WebhookResponse{Status: 500, Body: []byte("no host")}, nil
	}
	task, err := host.Tasks().Create(ctx, pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-probe",
		WorkflowID:  "wf-probe",
		Title:       "fixture write probe",
	})
	if err != nil {
		return &pluginsdk.WebhookResponse{Status: 500, Body: []byte(err.Error())}, nil
	}
	if _, err := host.Messages().Send(ctx, task.ID, "", "fixture probe message"); err != nil {
		return &pluginsdk.WebhookResponse{Status: 500, Body: []byte(err.Error())}, nil
	}
	return &pluginsdk.WebhookResponse{Status: 200, Body: []byte("ok")}, nil
}

func main() {
	pluginsdk.Serve(&fixturePlugin{})
}
