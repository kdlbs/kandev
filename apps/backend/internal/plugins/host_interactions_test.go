package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── fakes ───────────────────────────────────────────────────────────────

type fakeInteractionDataSource struct {
	pending     []*taskmodels.Interaction
	byID        map[string]*taskmodels.Interaction
	listErr     error
	getErr      error
	lastFilter  taskmodels.PendingInteractionFilter
	getCalls    int
	afterWrite  *taskmodels.Interaction
	writeCalled *bool
}

func (f *fakeInteractionDataSource) ListPendingInteractions(
	_ context.Context, filter taskmodels.PendingInteractionFilter,
) ([]*taskmodels.Interaction, error) {
	f.lastFilter = filter
	return f.pending, f.listErr
}

func (f *fakeInteractionDataSource) GetInteraction(
	_ context.Context, pendingID string,
) (*taskmodels.Interaction, error) {
	f.getCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	// After a successful write the host re-reads to report the new terminal
	// state; serve the post-write snapshot on that second read.
	if f.writeCalled != nil && *f.writeCalled && f.afterWrite != nil {
		return f.afterWrite, nil
	}
	return f.byID[pendingID], nil
}

type recordedPermissionResponse struct {
	in PluginPermissionResponse
}

type fakeInteractionResponder struct {
	permission  *recordedPermissionResponse
	answers     []PluginClarificationAnswer
	answered    string
	declined    string
	reason      string
	err         error
	writeCalled bool
}

func (f *fakeInteractionResponder) RespondToPermission(
	_ context.Context, in PluginPermissionResponse,
) error {
	if f.err != nil {
		return f.err
	}
	f.permission = &recordedPermissionResponse{in: in}
	f.writeCalled = true
	return nil
}

func (f *fakeInteractionResponder) AnswerClarification(
	_ context.Context, pendingID string, answers []PluginClarificationAnswer,
) error {
	if f.err != nil {
		return f.err
	}
	f.answered, f.answers, f.writeCalled = pendingID, answers, true
	return nil
}

func (f *fakeInteractionResponder) DeclineClarification(
	_ context.Context, pendingID, reason string,
) error {
	if f.err != nil {
		return f.err
	}
	f.declined, f.reason, f.writeCalled = pendingID, reason, true
	return nil
}

func pendingPermissionInteraction() *taskmodels.Interaction {
	return &taskmodels.Interaction{
		ID: "pending-1", Kind: taskmodels.InteractionKindPermission,
		TaskID: "task-1", SessionID: "session-1", TurnID: "turn-1",
		Status: taskmodels.InteractionStatusPending, RequestID: "request-1", Title: "Run a command?",
		Options: []taskmodels.InteractionOption{
			{ID: "allow", Label: "Allow once", Kind: "allow_once"},
			{ID: "deny", Label: "Deny", Kind: "reject_once"},
		},
		CreatedAt: time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, time.August, 21, 12, 0, 0, 0, time.UTC),
	}
}

func pendingClarificationInteraction() *taskmodels.Interaction {
	return &taskmodels.Interaction{
		ID: "pending-2", Kind: taskmodels.InteractionKindClarification,
		TaskID: "task-2", SessionID: "session-2", TurnID: "turn-2",
		Status:    taskmodels.InteractionStatusPending,
		Questions: []taskmodels.InteractionQuestion{{ID: "q1", Prompt: "Which?"}},
	}
}

func withInteraction(d *testDataHost, interaction *taskmodels.Interaction) {
	d.interactions.byID = map[string]*taskmodels.Interaction{interaction.ID: interaction}
	d.interactions.writeCalled = &d.responder.writeCalled
}

func readWriteCaps() manifest.Capabilities {
	return manifest.Capabilities{APIRead: []string{"interactions"}, APIWrite: []string{"interactions"}}
}

func assertCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want %v", want)
	}
	if got := status.Code(err); got != want {
		t.Fatalf("code = %v (%v), want %v", got, err, want)
	}
}

// ── capability gating ───────────────────────────────────────────────────

func TestPluginHost_Interactions_ReadDeniedWithoutCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIWrite: []string{"interactions"}})

	_, _, err := d.host.Interactions().ListPending(context.Background(), pluginsdk.InteractionFilter{}, pluginsdk.Page{})
	assertPermissionDenied(t, err, "api_read:interactions")

	_, err = d.host.Interactions().Get(context.Background(), "pending-1")
	assertPermissionDenied(t, err, "api_read:interactions")
}

// TestPluginHost_Interactions_WriteDeniedWithReadOnly is the case an inbox
// plugin depends on: read-only really is read-only, so declaring
// api_read:interactions never smuggles in the ability to answer on the user's
// behalf.
func TestPluginHost_Interactions_WriteDeniedWithReadOnly(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"interactions"}})
	withInteraction(d, pendingPermissionInteraction())
	ctx := context.Background()

	_, err := d.host.Interactions().RespondToPermission(ctx, pluginsdk.PermissionResponse{
		InteractionID: "pending-1", OptionID: "allow",
	})
	assertPermissionDenied(t, err, "api_write:interactions")

	_, err = d.host.Interactions().AnswerClarification(ctx, pluginsdk.ClarificationResponse{
		InteractionID: "pending-2",
		Answers:       []pluginsdk.ClarificationAnswer{{QuestionID: "q1", CustomText: "yes"}},
	})
	assertPermissionDenied(t, err, "api_write:interactions")

	_, err = d.host.Interactions().CancelClarification(ctx, "pending-2", "nope")
	assertPermissionDenied(t, err, "api_write:interactions")

	if d.responder.writeCalled {
		t.Fatal("responder reached despite missing api_write:interactions")
	}
}

// ── reads ───────────────────────────────────────────────────────────────

func TestPluginHost_Interactions_ListPendingMapsAndFilters(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"interactions"}})
	d.interactions.pending = []*taskmodels.Interaction{pendingPermissionInteraction(), nil}

	got, info, err := d.host.Interactions().ListPending(context.Background(), pluginsdk.InteractionFilter{
		SessionIDs: []string{"session-1"}, Kinds: []string{"permission"},
	}, pluginsdk.Page{})
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("interactions = %d, want 1 (nil rows dropped)", len(got))
	}
	if got[0].ID != "pending-1" || got[0].Kind != pluginsdk.InteractionKindPermission ||
		got[0].Status != pluginsdk.InteractionStatusPending {
		t.Fatalf("dto = %+v", got[0])
	}
	if got[0].CreatedAt != "2026-08-21T12:00:00Z" {
		t.Fatalf("created_at = %q, want RFC3339 UTC", got[0].CreatedAt)
	}
	if len(got[0].Options) != 2 || got[0].Options[0].OptionID != "allow" {
		t.Fatalf("options = %+v", got[0].Options)
	}
	if info == nil || info.HasMore {
		t.Fatalf("page info = %+v", info)
	}
	if len(d.interactions.lastFilter.SessionIDs) != 1 || d.interactions.lastFilter.SessionIDs[0] != "session-1" {
		t.Fatalf("filter not forwarded: %+v", d.interactions.lastFilter)
	}
}

func TestPluginHost_Interactions_ListPendingRejectsUnknownKind(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"interactions"}})
	_, _, err := d.host.Interactions().ListPending(context.Background(),
		pluginsdk.InteractionFilter{Kinds: []string{"permision"}}, pluginsdk.Page{})
	assertCode(t, err, codes.InvalidArgument)
}

// TestPluginHost_Interactions_GetResolvesTerminal is what makes a plugin's
// event cache reconcilable: an id it saw in an event still resolves after the
// interaction was answered, instead of vanishing into NotFound.
func TestPluginHost_Interactions_GetResolvesTerminal(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"interactions"}})
	resolved := pendingPermissionInteraction()
	resolved.Status = taskmodels.InteractionStatusApproved
	withInteraction(d, resolved)

	got, err := d.host.Interactions().Get(context.Background(), "pending-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != pluginsdk.InteractionStatusApproved {
		t.Fatalf("status = %q, want approved", got.Status)
	}
}

func TestPluginHost_Interactions_GetUnknownIsNotFound(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"interactions"}})
	d.interactions.byID = map[string]*taskmodels.Interaction{}

	_, err := d.host.Interactions().Get(context.Background(), "nope")
	assertCode(t, err, codes.NotFound)

	_, err = d.host.Interactions().Get(context.Background(), "")
	assertCode(t, err, codes.InvalidArgument)
}

// Legacy write methods cannot carry an observed resource version or human
// response receipt. They remain in the v1 interface for source compatibility,
// but all responses must use the exact Host v2 commands.
func TestPluginHost_Interactions_LegacyResponsesRequireHumanReceipt(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testDataHost) error
	}{
		{"permission", func(d *testDataHost) error {
			_, err := d.host.Interactions().RespondToPermission(context.Background(), pluginsdk.PermissionResponse{InteractionID: "pending-1", OptionID: "allow"})
			return err
		}},
		{"clarification", func(d *testDataHost) error {
			_, err := d.host.Interactions().AnswerClarification(context.Background(), pluginsdk.ClarificationResponse{InteractionID: "pending-2", Answers: []pluginsdk.ClarificationAnswer{{QuestionID: "q1"}}})
			return err
		}},
		{"clarification cancellation", func(d *testDataHost) error {
			_, err := d.host.Interactions().CancelClarification(context.Background(), "pending-2", "cancel")
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := newTestDataHost(readWriteCaps())
			withInteraction(d, pendingPermissionInteraction())
			assertCode(t, test.run(d), codes.PermissionDenied)
			if d.responder.writeCalled {
				t.Fatal("legacy response reached the agent interaction responder")
			}
		})
	}
}

// ── unwired host ────────────────────────────────────────────────────────

func TestPluginHost_Interactions_UnimplementedReadAndReceiptRequiredWrite(t *testing.T) {
	host := &pluginHost{pluginID: "p1", capabilities: readWriteCaps()}
	ctx := context.Background()

	_, _, err := host.Interactions().ListPending(ctx, pluginsdk.InteractionFilter{}, pluginsdk.Page{})
	assertCode(t, err, codes.Unimplemented)

	_, err = host.Interactions().RespondToPermission(ctx, pluginsdk.PermissionResponse{InteractionID: "x", Cancelled: true})
	assertCode(t, err, codes.PermissionDenied)
}

// TestServiceHostCarriesInteractionDependencies pins the wiring hop between
// Service and the host it hands each plugin. Every capability check and every
// accessor test above runs against a hand-built pluginHost, so a
// SetDataSources or SetInteractionResponder field that never reaches
// hostForPlugin would leave the whole feature inert while every other test in
// this file still passed.
func TestServiceHostCarriesInteractionDependencies(t *testing.T) {
	svc, _, _ := newTestService(t)
	source := &fakeInteractionDataSource{
		byID: map[string]*taskmodels.Interaction{"pending-1": pendingPermissionInteraction()},
	}
	responder := &fakeInteractionResponder{}
	svc.SetDataSources(nil, nil, nil, nil, nil, nil, source, nil)
	svc.SetInteractionResponder(responder)

	host, ok := svc.hostForPlugin("p1").(*pluginHost)
	if !ok {
		t.Fatal("hostForPlugin did not return a *pluginHost")
	}
	if host.interactionData == nil {
		t.Fatal("interaction data source did not reach the plugin host")
	}
	if host.interactionDeps == nil || host.interactionDeps() == nil {
		t.Fatal("interaction responder did not reach the plugin host")
	}
	// The host is built from an uninstalled plugin id, so it declares no
	// capabilities: the reachable dependency must still be gated.
	if _, err := host.Interactions().Get(context.Background(), "pending-1"); err == nil ||
		status.Code(err) != codes.PermissionDenied {
		t.Fatalf("Get without api_read:interactions = %v, want PermissionDenied", err)
	}
}
