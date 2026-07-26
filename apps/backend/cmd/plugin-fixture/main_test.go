// Command plugin-fixture tests. Exercises fixturePlugin's Plugin methods
// (OnEvent, HandleWebhook) via direct calls — no go-plugin
// spawn needed, since fixturePlugin has no dependency on the gRPC
// transport itself (pluginsdk.Serve owns that wiring and is covered by
// pkg/pluginsdk's own tests).
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

var errBoom = errors.New("boom")

// readJSONLines reads path and decodes each non-empty line as T.
func readJSONLines[T any](t *testing.T, path string) []T {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var out []T
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var v T
		require.NoError(t, json.Unmarshal(line, &v))
		out = append(out, v)
	}
	require.NoError(t, scanner.Err())
	return out
}

func TestOnEvent_AppendsJSONLineToDataDir(t *testing.T) {
	dir := t.TempDir()
	p := &fixturePlugin{dataDir: dir}

	err := p.OnEvent(context.Background(), &pluginsdk.Event{
		EventID:   "evt_1",
		EventType: "task.created",
		Payload:   map[string]any{"task_id": "t1"},
	})
	require.NoError(t, err)

	recs := readJSONLines[deliveryRecord](t, filepath.Join(dir, "deliveries.jsonl"))
	require.Len(t, recs, 1)
	require.Equal(t, "task.created", recs[0].EventType)
	require.Equal(t, "evt_1", recs[0].EventID)
}

func TestOnEvent_AppendsMultipleDeliveriesInOrder(t *testing.T) {
	dir := t.TempDir()
	p := &fixturePlugin{dataDir: dir}

	require.NoError(t, p.OnEvent(context.Background(), &pluginsdk.Event{EventID: "e1", EventType: "task.created"}))
	require.NoError(t, p.OnEvent(context.Background(), &pluginsdk.Event{EventID: "e2", EventType: "task.updated"}))

	recs := readJSONLines[deliveryRecord](t, filepath.Join(dir, "deliveries.jsonl"))
	require.Len(t, recs, 2)
	require.Equal(t, "e1", recs[0].EventID)
	require.Equal(t, "e2", recs[1].EventID)
}

func TestOnEvent_CreatesDataDirIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "data")
	p := &fixturePlugin{dataDir: dir}

	err := p.OnEvent(context.Background(), &pluginsdk.Event{EventID: "e1", EventType: "task.created"})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "deliveries.jsonl"))
	require.NoError(t, err)
}

func TestOnEvent_FirstEventCallsHostSetState(t *testing.T) {
	dir := t.TempDir()
	p := &fixturePlugin{dataDir: dir}
	host := &fakeHost{}
	p.SetHost(host)

	require.NoError(t, p.OnEvent(context.Background(), &pluginsdk.Event{EventID: "e1", EventType: "task.created"}))
	require.NoError(t, p.OnEvent(context.Background(), &pluginsdk.Event{EventID: "e2", EventType: "task.updated"}))

	require.Len(t, host.setStateCalls, 1, "SetState should only be exercised on the first event")
	call := host.setStateCalls[0]
	require.Equal(t, "instance", call.scope)
	require.Equal(t, "", call.scopeID)
	require.Equal(t, "last_event", call.key)
	require.Equal(t, "e1", call.value["event_id"])
}

func TestOnEvent_IgnoresHostSetStateError(t *testing.T) {
	dir := t.TempDir()
	p := &fixturePlugin{dataDir: dir}
	host := &fakeHost{setStateErr: errBoom}
	p.SetHost(host)

	err := p.OnEvent(context.Background(), &pluginsdk.Event{EventID: "e1", EventType: "task.created"})
	require.NoError(t, err, "Host.SetState failures must be best-effort and not fail OnEvent")
}

func TestHandleWebhook_AppendsJSONLineAndRespondsOK(t *testing.T) {
	dir := t.TempDir()
	p := &fixturePlugin{dataDir: dir}

	resp, err := p.HandleWebhook(context.Background(), &pluginsdk.WebhookRequest{
		WebhookKey: "test-hook",
		Method:     "POST",
		Body:       []byte(`{"ping":true}`),
	})
	require.NoError(t, err)
	require.Equal(t, int32(200), resp.Status)
	require.Equal(t, "ok", string(resp.Body))

	recs := readJSONLines[webhookRecord](t, filepath.Join(dir, "webhooks.jsonl"))
	require.Len(t, recs, 1)
	require.Equal(t, "test-hook", recs[0].WebhookKey)
}

// TestHandleWebhook_WriteKeyRoundTripsHostWrites proves the "write" webhook
// drives the Host data API write RPCs (CreateTask then SendMessage to the
// returned task) and records the outcome to write-probe.json.
func TestHandleWebhook_WriteKeyRoundTripsHostWrites(t *testing.T) {
	dir := t.TempDir()
	p := &fixturePlugin{dataDir: dir}
	host := &fakeHost{createdTaskID: "task-42"}
	p.SetHost(host)

	resp, err := p.HandleWebhook(context.Background(), &pluginsdk.WebhookRequest{WebhookKey: "write"})
	require.NoError(t, err)
	require.Equal(t, "ok", string(resp.Body))

	require.Equal(t, "fixture write probe", host.lastCreateInput.Title)
	require.Equal(t, "task-42", host.lastSendTask, "the message must target the task the Host returned")
	require.Equal(t, "fixture probe message", host.lastSendText)

	rec := readSingleJSON[writeProbeRecord](t, filepath.Join(dir, "write-probe.json"))
	require.Equal(t, "task-42", rec.TaskID)
	require.Equal(t, "queued", rec.MessageStatus)
	require.Empty(t, rec.TaskError)
}

// readSingleJSON reads path and decodes its whole contents as a single T.
func readSingleJSON[T any](t *testing.T, path string) T {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var v T
	require.NoError(t, json.Unmarshal(data, &v))
	return v
}

func TestResolveDataDir_UsesEnvWhenSet(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", "/tmp/kandev-plugin-e2e-data")
	require.Equal(t, "/tmp/kandev-plugin-e2e-data", resolveDataDir())
}

func TestResolveDataDir_FallsBackToCwdWhenUnset(t *testing.T) {
	t.Setenv("KANDEV_PLUGIN_DATA_DIR", "")
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, wd, resolveDataDir())
}

// setStateCall records one fakeHost.SetState invocation.
type setStateCall struct {
	scope, scopeID, key string
	value               map[string]any
}

// fakeHost is a minimal pluginsdk.Host test double that only records
// SetState calls; the fixture plugin never exercises the other methods. It
// embeds UnimplementedHostData to satisfy the Host data API (ADR 0043)
// sub-accessors without wiring them.
type fakeHost struct {
	pluginsdk.UnimplementedHostData

	setStateCalls []setStateCall
	setStateErr   error

	// Host data API write recording (ADR 0043 phase 2).
	createdTaskID   string
	lastCreateInput pluginsdk.CreateTaskInput
	lastSendTask    string
	lastSendText    string
}

func (h *fakeHost) Tasks() pluginsdk.TaskReader       { return fakeHostTaskReader{h} }
func (h *fakeHost) Messages() pluginsdk.MessageReader { return fakeHostMessageReader{h} }

type fakeHostTaskReader struct{ h *fakeHost }

func (fakeHostTaskReader) List(context.Context, pluginsdk.TaskFilter, pluginsdk.Page) ([]pluginsdk.Task, *pluginsdk.PageInfo, error) {
	return nil, nil, nil
}
func (fakeHostTaskReader) Get(context.Context, string) (*pluginsdk.Task, error) { return nil, nil }
func (fakeHostTaskReader) Update(context.Context, pluginsdk.UpdateTaskInput) (*pluginsdk.Task, error) {
	return nil, nil
}

func (r fakeHostTaskReader) Create(_ context.Context, in pluginsdk.CreateTaskInput) (*pluginsdk.Task, error) {
	r.h.lastCreateInput = in
	return &pluginsdk.Task{ID: r.h.createdTaskID, Title: in.Title}, nil
}

type fakeHostMessageReader struct{ h *fakeHost }

func (fakeHostMessageReader) List(context.Context, pluginsdk.MessageFilter, pluginsdk.Page) ([]pluginsdk.Message, *pluginsdk.PageInfo, error) {
	return nil, nil, nil
}

func (r fakeHostMessageReader) Send(_ context.Context, taskID, sessionID, text string) (*pluginsdk.MessageDispatch, error) {
	r.h.lastSendTask = taskID
	r.h.lastSendText = text
	return &pluginsdk.MessageDispatch{SessionID: "session-1", Status: "queued"}, nil
}

func (h *fakeHost) GetState(context.Context, string, string, string) (map[string]any, bool, error) {
	return nil, false, nil
}

func (h *fakeHost) SetState(_ context.Context, scope, scopeID, key string, value map[string]any) error {
	h.setStateCalls = append(h.setStateCalls, setStateCall{scope: scope, scopeID: scopeID, key: key, value: value})
	return h.setStateErr
}

func (h *fakeHost) DeleteState(context.Context, string, string, string) error { return nil }

func (h *fakeHost) ListState(context.Context, string, string) ([]pluginsdk.StateEntry, error) {
	return nil, nil
}

func (h *fakeHost) GetConfig(context.Context) (map[string]any, error)       { return nil, nil }
func (h *fakeHost) GetSecret(context.Context, string) (string, bool, error) { return "", false, nil }
func (h *fakeHost) SetSecret(context.Context, string, string) error         { return nil }
func (h *fakeHost) DeleteSecret(context.Context, string) error              { return nil }
func (h *fakeHost) RevealSecret(context.Context, string) (string, error)    { return "", nil }

func (h *fakeHost) EmitEvent(context.Context, string, map[string]any) error { return nil }

var _ pluginsdk.Host = (*fakeHost)(nil)
