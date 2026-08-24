package plugins

import (
	"context"
	"errors"
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

// ── permission writes ───────────────────────────────────────────────────

func TestPluginHost_Interactions_RespondToPermissionDerivesOutcomeFromOptionKind(t *testing.T) {
	cases := []struct {
		name      string
		optionID  string
		cancelled bool
	}{
		{"allow option is forwarded", "allow", false},
		{"reject option is forwarded", "deny", false},
		{"dismissal carries no option", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newTestDataHost(readWriteCaps())
			withInteraction(d, pendingPermissionInteraction())
			resolved := pendingPermissionInteraction()
			resolved.Status = taskmodels.InteractionStatusApproved
			d.interactions.afterWrite = resolved

			got, err := d.host.Interactions().RespondToPermission(context.Background(),
				pluginsdk.PermissionResponse{InteractionID: "pending-1", OptionID: tc.optionID, Cancelled: tc.cancelled})
			if err != nil {
				t.Fatalf("RespondToPermission: %v", err)
			}
			if d.responder.permission == nil {
				t.Fatal("responder not called")
			}
			if d.responder.permission.in.OptionID != tc.optionID {
				t.Fatalf("option = %q, want %q", d.responder.permission.in.OptionID, tc.optionID)
			}
			if d.responder.permission.in.Cancelled != tc.cancelled {
				t.Fatalf("cancelled = %v, want %v", d.responder.permission.in.Cancelled, tc.cancelled)
			}
			// Identity always comes from the durable record, never from the
			// plugin: kandev's resolution is keyed on all four ids.
			if d.responder.permission.in.SessionID != "session-1" ||
				d.responder.permission.in.TaskID != "task-1" ||
				d.responder.permission.in.RequestID != "request-1" ||
				d.responder.permission.in.PendingID != "pending-1" {
				t.Fatalf("identity = %+v, want it taken from the durable record", d.responder.permission.in)
			}
			if got.Status != pluginsdk.InteractionStatusApproved {
				t.Fatalf("returned status = %q, want the post-write state", got.Status)
			}
		})
	}
}

func TestPluginHost_Interactions_RespondToPermissionValidatesOption(t *testing.T) {
	cases := []struct {
		name string
		in   pluginsdk.PermissionResponse
	}{
		{"option the agent never offered", pluginsdk.PermissionResponse{InteractionID: "pending-1", OptionID: "sudo"}},
		{"no option and no cancel", pluginsdk.PermissionResponse{InteractionID: "pending-1"}},
		{"option together with cancel", pluginsdk.PermissionResponse{
			InteractionID: "pending-1", OptionID: "allow", Cancelled: true,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newTestDataHost(readWriteCaps())
			withInteraction(d, pendingPermissionInteraction())

			_, err := d.host.Interactions().RespondToPermission(context.Background(), tc.in)
			assertCode(t, err, codes.InvalidArgument)
			if d.responder.writeCalled {
				t.Fatal("responder reached with an invalid choice")
			}
		})
	}
}

// TestPluginHost_Interactions_TerminalOnce pins the idempotency contract: the
// first terminal response wins, and a replay reports FailedPrecondition rather
// than dispatching a second response — which would either fail deep in the
// agent transport or answer whatever request reused the pending slot.
func TestPluginHost_Interactions_TerminalOnce(t *testing.T) {
	resolved := pendingPermissionInteraction()
	resolved.Status = taskmodels.InteractionStatusRejected
	d := newTestDataHost(readWriteCaps())
	withInteraction(d, resolved)

	_, err := d.host.Interactions().RespondToPermission(context.Background(),
		pluginsdk.PermissionResponse{InteractionID: "pending-1", OptionID: "allow"})
	assertCode(t, err, codes.FailedPrecondition)
	if d.responder.writeCalled {
		t.Fatal("responder reached for an already-resolved interaction")
	}

	answeredBundle := pendingClarificationInteraction()
	answeredBundle.Status = taskmodels.InteractionStatusAnswered
	d2 := newTestDataHost(readWriteCaps())
	withInteraction(d2, answeredBundle)
	_, err = d2.host.Interactions().AnswerClarification(context.Background(), pluginsdk.ClarificationResponse{
		InteractionID: "pending-2", Answers: []pluginsdk.ClarificationAnswer{{QuestionID: "q1"}},
	})
	assertCode(t, err, codes.FailedPrecondition)
}

func TestPluginHost_Interactions_KindMismatchIsRefused(t *testing.T) {
	d := newTestDataHost(readWriteCaps())
	withInteraction(d, pendingPermissionInteraction())

	_, err := d.host.Interactions().AnswerClarification(context.Background(), pluginsdk.ClarificationResponse{
		InteractionID: "pending-1", Answers: []pluginsdk.ClarificationAnswer{{QuestionID: "q1"}},
	})
	assertCode(t, err, codes.FailedPrecondition)

	d2 := newTestDataHost(readWriteCaps())
	withInteraction(d2, pendingClarificationInteraction())
	_, err = d2.host.Interactions().RespondToPermission(context.Background(),
		pluginsdk.PermissionResponse{InteractionID: "pending-2", Cancelled: true})
	assertCode(t, err, codes.FailedPrecondition)
}

// ── clarification writes ────────────────────────────────────────────────

func TestPluginHost_Interactions_AnswerAndCancelClarification(t *testing.T) {
	d := newTestDataHost(readWriteCaps())
	withInteraction(d, pendingClarificationInteraction())
	answered := pendingClarificationInteraction()
	answered.Status = taskmodels.InteractionStatusAnswered
	d.interactions.afterWrite = answered

	got, err := d.host.Interactions().AnswerClarification(context.Background(), pluginsdk.ClarificationResponse{
		InteractionID: "pending-2",
		Answers:       []pluginsdk.ClarificationAnswer{{QuestionID: "q1", SelectedOptions: []string{"a"}, CustomText: "why not"}},
	})
	if err != nil {
		t.Fatalf("AnswerClarification: %v", err)
	}
	if d.responder.answered != "pending-2" || len(d.responder.answers) != 1 ||
		d.responder.answers[0].QuestionID != "q1" || d.responder.answers[0].CustomText != "why not" {
		t.Fatalf("responder saw %q / %+v", d.responder.answered, d.responder.answers)
	}
	if got.Status != pluginsdk.InteractionStatusAnswered {
		t.Fatalf("status = %q, want answered", got.Status)
	}

	d2 := newTestDataHost(readWriteCaps())
	withInteraction(d2, pendingClarificationInteraction())
	if _, err := d2.host.Interactions().CancelClarification(context.Background(), "pending-2", "user went away"); err != nil {
		t.Fatalf("CancelClarification: %v", err)
	}
	if d2.responder.declined != "pending-2" || d2.responder.reason != "user went away" {
		t.Fatalf("decline recorded as %q / %q", d2.responder.declined, d2.responder.reason)
	}
}

func TestPluginHost_Interactions_AnswerClarificationValidatesAnswers(t *testing.T) {
	cases := []struct {
		name string
		in   pluginsdk.ClarificationResponse
	}{
		{"no answers", pluginsdk.ClarificationResponse{InteractionID: "pending-2"}},
		{"answer without a question id", pluginsdk.ClarificationResponse{
			InteractionID: "pending-2", Answers: []pluginsdk.ClarificationAnswer{{CustomText: "yes"}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newTestDataHost(readWriteCaps())
			withInteraction(d, pendingClarificationInteraction())
			_, err := d.host.Interactions().AnswerClarification(context.Background(), tc.in)
			assertCode(t, err, codes.InvalidArgument)
			if d.responder.writeCalled {
				t.Fatal("responder reached with an invalid answer set")
			}
		})
	}
}

// TestPluginHost_Interactions_WriteSurvivesFailedReread proves a delivered
// response is never reported as an error: the re-read is only there to report
// the new state, so its failure falls back to the pre-write snapshot instead
// of inviting a retry that would double-answer.
func TestPluginHost_Interactions_WriteSurvivesFailedReread(t *testing.T) {
	d := newTestDataHost(readWriteCaps())
	withInteraction(d, pendingClarificationInteraction())
	d.responder.writeCalled = false

	first := true
	source := d.interactions
	source.getErr = nil
	d.host.interactionData = &rereadFailingSource{inner: source, failAfter: &first}

	got, err := d.host.Interactions().CancelClarification(context.Background(), "pending-2", "gone")
	if err != nil {
		t.Fatalf("CancelClarification: %v", err)
	}
	if got.ID != "pending-2" {
		t.Fatalf("interaction = %+v, want the pre-write snapshot", got)
	}
	// The response contract says every write returns the interaction in its NEW
	// terminal state. Handing back the pre-write snapshot verbatim would report
	// "pending" for a response the agent already received, inviting the retry
	// terminal-once exists to prevent. A cancel is delivered as a decline of the
	// bundle, so its terminal status is rejected.
	if got.Status != pluginsdk.InteractionStatusRejected {
		t.Fatalf("status = %q, want the resolved status stamped on the fallback", got.Status)
	}
	if !d.responder.writeCalled {
		t.Fatal("write never happened")
	}
}

type rereadFailingSource struct {
	inner     *fakeInteractionDataSource
	failAfter *bool
}

func (s *rereadFailingSource) ListPendingInteractions(
	ctx context.Context, filter taskmodels.PendingInteractionFilter,
) ([]*taskmodels.Interaction, error) {
	return s.inner.ListPendingInteractions(ctx, filter)
}

func (s *rereadFailingSource) GetInteraction(
	ctx context.Context, pendingID string,
) (*taskmodels.Interaction, error) {
	if *s.failAfter {
		*s.failAfter = false
		return s.inner.GetInteraction(ctx, pendingID)
	}
	return nil, errors.New("database went away")
}

// ── unwired host ────────────────────────────────────────────────────────

func TestPluginHost_Interactions_UnimplementedWithoutDependencies(t *testing.T) {
	host := &pluginHost{pluginID: "p1", capabilities: readWriteCaps()}
	ctx := context.Background()

	_, _, err := host.Interactions().ListPending(ctx, pluginsdk.InteractionFilter{}, pluginsdk.Page{})
	assertCode(t, err, codes.Unimplemented)

	_, err = host.Interactions().RespondToPermission(ctx, pluginsdk.PermissionResponse{InteractionID: "x", Cancelled: true})
	assertCode(t, err, codes.Unimplemented)
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
