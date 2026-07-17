package pluginsdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnimplementedPlugin_DefaultsAreNoOps(t *testing.T) {
	var p UnimplementedPlugin

	require.NoError(t, p.OnEvent(context.Background(), &Event{EventID: "e1"}))

	resp, err := p.InvokeTool(context.Background(), &ToolRequest{ToolName: "noop"})
	require.NoError(t, err)
	require.NotNil(t, resp)

	webhookResp, err := p.HandleWebhook(context.Background(), &WebhookRequest{Path: "/x"})
	require.NoError(t, err)
	require.NotNil(t, webhookResp)
	require.Equal(t, int32(404), webhookResp.Status)
}

func TestUnimplementedPlugin_SetHostThenHost(t *testing.T) {
	var p UnimplementedPlugin
	require.Nil(t, p.Host())

	h := &fakeHost{}
	p.SetHost(h)
	require.Equal(t, h, p.Host())
}

// fakeHost is a minimal Host implementation used only to prove SetHost/Host
// wiring; broker-backed Host behavior is covered by the integration test in
// serve_test.go. It embeds UnimplementedHostData to satisfy the Host data
// API (ADR 0043) sub-accessors without wiring them.
type fakeHost struct {
	UnimplementedHostData
}

func (f *fakeHost) GetState(context.Context, string, string, string) (map[string]any, bool, error) {
	return nil, false, nil
}
func (f *fakeHost) SetState(context.Context, string, string, string, map[string]any) error {
	return nil
}
func (f *fakeHost) DeleteState(context.Context, string, string, string) error { return nil }
func (f *fakeHost) ListState(context.Context, string, string) ([]StateEntry, error) {
	return nil, nil
}
func (f *fakeHost) GetConfig(context.Context) (map[string]any, error)       { return nil, nil }
func (f *fakeHost) GetSecret(context.Context, string) (string, bool, error) { return "", false, nil }
func (f *fakeHost) SetSecret(context.Context, string, string) error         { return nil }
func (f *fakeHost) DeleteSecret(context.Context, string) error              { return nil }
func (f *fakeHost) RevealSecret(context.Context, string) (string, error)    { return "", nil }
func (f *fakeHost) EmitEvent(context.Context, string, map[string]any) error { return nil }

var _ Host = (*fakeHost)(nil)
