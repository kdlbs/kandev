package pluginsdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// interactionRecordingHost is a Go-native Host whose interaction accessor is
// backed by in-memory fixtures, used to prove grpcHostServer -> grpcHostClient
// reachability for the interaction RPCs. Everything else is inherited
// (unimplemented) from UnimplementedHostData.
type interactionRecordingHost struct {
	UnimplementedHostData

	pending    []Interaction
	pageInfo   *PageInfo
	byID       map[string]Interaction
	lastFilter InteractionFilter
	lastPage   Page
	lastPerm   PermissionResponse
	lastAnswer ClarificationResponse
	lastCancel [2]string
}

func (h *interactionRecordingHost) GetState(context.Context, string, string, string) (map[string]any, bool, error) {
	return nil, false, nil
}
func (h *interactionRecordingHost) SetState(context.Context, string, string, string, map[string]any) error {
	return nil
}
func (h *interactionRecordingHost) DeleteState(context.Context, string, string, string) error {
	return nil
}
func (h *interactionRecordingHost) ListState(context.Context, string, string) ([]StateEntry, error) {
	return nil, nil
}
func (h *interactionRecordingHost) GetConfig(context.Context) (map[string]any, error) {
	return map[string]any{}, nil
}
func (h *interactionRecordingHost) RevealSecret(context.Context, string) (string, error) {
	return "", nil
}
func (h *interactionRecordingHost) GetSecret(context.Context, string) (string, bool, error) {
	return "", false, nil
}
func (h *interactionRecordingHost) SetSecret(context.Context, string, string) error { return nil }
func (h *interactionRecordingHost) DeleteSecret(context.Context, string) error      { return nil }
func (h *interactionRecordingHost) EmitEvent(context.Context, string, map[string]any) error {
	return nil
}

func (h *interactionRecordingHost) Interactions() InteractionAccessor {
	return &recordingInteractionAccessor{host: h}
}

type recordingInteractionAccessor struct{ host *interactionRecordingHost }

func (a *recordingInteractionAccessor) ListPending(
	_ context.Context, filter InteractionFilter, page Page,
) ([]Interaction, *PageInfo, error) {
	a.host.lastFilter, a.host.lastPage = filter, page
	return a.host.pending, a.host.pageInfo, nil
}

func (a *recordingInteractionAccessor) Get(_ context.Context, id string) (*Interaction, error) {
	interaction, ok := a.host.byID[id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "interaction %q not found", id)
	}
	return &interaction, nil
}

func (a *recordingInteractionAccessor) RespondToPermission(
	_ context.Context, in PermissionResponse,
) (*Interaction, error) {
	a.host.lastPerm = in
	interaction := a.host.byID[in.InteractionID]
	interaction.Status = InteractionStatusApproved
	return &interaction, nil
}

func (a *recordingInteractionAccessor) AnswerClarification(
	_ context.Context, in ClarificationResponse,
) (*Interaction, error) {
	a.host.lastAnswer = in
	interaction := a.host.byID[in.InteractionID]
	interaction.Status = InteractionStatusAnswered
	return &interaction, nil
}

func (a *recordingInteractionAccessor) CancelClarification(
	_ context.Context, id, reason string,
) (*Interaction, error) {
	a.host.lastCancel = [2]string{id, reason}
	interaction := a.host.byID[id]
	interaction.Status = InteractionStatusRejected
	return &interaction, nil
}

func fixtureInteractions() (Interaction, Interaction) {
	permission := Interaction{
		ID: "pending-perm", Kind: InteractionKindPermission,
		TaskID: "task-1", SessionID: "session-1", TurnID: "turn-1",
		Status: InteractionStatusPending, Title: "Run a command?",
		ToolCallID: "call-1", ActionType: "execute",
		Options: []InteractionOption{
			{OptionID: "allow", Label: "Allow once", Kind: "allow_once"},
			{OptionID: "deny", Label: "Deny", Kind: "reject_once"},
		},
		CreatedAt: "2026-08-21T12:00:00Z", UpdatedAt: "2026-08-21T12:00:00Z",
	}
	clarification := Interaction{
		ID: "pending-clar", Kind: InteractionKindClarification,
		TaskID: "task-2", SessionID: "session-2", TurnID: "turn-2",
		Status: InteractionStatusPending, Title: "Which database?",
		Context: "migration planning", AgentDisconnected: true,
		Questions: []InteractionQuestion{{
			ID: "q1", Title: "DB", Prompt: "Which database?",
			Options: []InteractionOption{
				{OptionID: "pg", Label: "Postgres", Description: "the default"},
				{OptionID: "sqlite", Label: "SQLite"},
			},
		}},
		CreatedAt: "2026-08-21T13:00:00Z", UpdatedAt: "2026-08-21T13:05:00Z",
	}
	return permission, clarification
}

func newInteractionHostPair(t *testing.T) (*interactionRecordingHost, InteractionAccessor) {
	t.Helper()
	permission, clarification := fixtureInteractions()
	impl := &interactionRecordingHost{
		pending:  []Interaction{permission, clarification},
		pageInfo: &PageInfo{NextCursor: "2", HasMore: true},
		byID: map[string]Interaction{
			permission.ID: permission, clarification.ID: clarification,
		},
	}
	host := dialHostOverBufconn(t, impl)
	accessor, ok := Interactions(host)
	require.True(t, ok, "the wire client must expose the interaction extension")
	return impl, accessor
}

// TestInteractions_ListPendingRoundTrip is the field-by-field wire proof: a
// dropped field in either conversion direction would silently strip the very
// options and questions a plugin needs to render a valid response.
func TestInteractions_ListPendingRoundTrip(t *testing.T) {
	impl, accessor := newInteractionHostPair(t)
	wantPermission, wantClarification := fixtureInteractions()

	got, info, err := accessor.ListPending(context.Background(), InteractionFilter{
		SessionIDs: []string{"session-1"}, TaskIDs: []string{"task-1"}, Kinds: []string{InteractionKindPermission},
	}, Page{Limit: 10, Cursor: "1"})
	require.NoError(t, err)
	require.Equal(t, []Interaction{wantPermission, wantClarification}, got)
	require.Equal(t, &PageInfo{NextCursor: "2", HasMore: true}, info)

	require.Equal(t, []string{"session-1"}, impl.lastFilter.SessionIDs)
	require.Equal(t, []string{"task-1"}, impl.lastFilter.TaskIDs)
	require.Equal(t, []string{InteractionKindPermission}, impl.lastFilter.Kinds)
	require.Equal(t, Page{Limit: 10, Cursor: "1"}, impl.lastPage)
}

func TestInteractions_GetRoundTripAndNotFound(t *testing.T) {
	_, accessor := newInteractionHostPair(t)
	_, wantClarification := fixtureInteractions()

	got, err := accessor.Get(context.Background(), "pending-clar")
	require.NoError(t, err)
	require.Equal(t, wantClarification, *got)

	_, err = accessor.Get(context.Background(), "nope")
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestInteractions_WritesRoundTrip(t *testing.T) {
	impl, accessor := newInteractionHostPair(t)
	ctx := context.Background()

	permission, err := accessor.RespondToPermission(ctx, PermissionResponse{
		InteractionID: "pending-perm", OptionID: "deny",
	})
	require.NoError(t, err)
	require.Equal(t, InteractionStatusApproved, permission.Status)
	require.Equal(t, PermissionResponse{InteractionID: "pending-perm", OptionID: "deny"}, impl.lastPerm)

	answered, err := accessor.AnswerClarification(ctx, ClarificationResponse{
		InteractionID: "pending-clar",
		Answers: []ClarificationAnswer{
			{QuestionID: "q1", SelectedOptions: []string{"pg"}, CustomText: "prefer managed"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, InteractionStatusAnswered, answered.Status)
	require.Equal(t, "pending-clar", impl.lastAnswer.InteractionID)
	require.Equal(t, []ClarificationAnswer{
		{QuestionID: "q1", SelectedOptions: []string{"pg"}, CustomText: "prefer managed"},
	}, impl.lastAnswer.Answers)

	cancelled, err := accessor.CancelClarification(ctx, "pending-clar", "user stepped away")
	require.NoError(t, err)
	require.Equal(t, InteractionStatusRejected, cancelled.Status)
	require.Equal(t, [2]string{"pending-clar", "user stepped away"}, impl.lastCancel)
}

// TestInteractions_HostWithoutExtensionIsUnimplemented covers the
// source-compatibility promise: a Host implementation written before this
// extension still serves every other RPC, and the interaction RPCs answer
// Unimplemented rather than panicking on a nil accessor.
func TestInteractions_HostWithoutExtensionIsUnimplemented(t *testing.T) {
	host := dialHostOverBufconn(t, &recordingHost{})
	accessor, ok := Interactions(host)
	require.True(t, ok)

	_, _, err := accessor.ListPending(context.Background(), InteractionFilter{}, Page{})
	require.Equal(t, codes.Unimplemented, status.Code(err))

	_, err = accessor.RespondToPermission(context.Background(), PermissionResponse{InteractionID: "x"})
	require.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestUnimplementedHostDataInteractionsReturnsUnimplemented(t *testing.T) {
	accessor := UnimplementedHostData{}.Interactions()
	ctx := context.Background()

	_, _, err := accessor.ListPending(ctx, InteractionFilter{}, Page{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = accessor.Get(ctx, "x")
	require.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = accessor.RespondToPermission(ctx, PermissionResponse{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = accessor.AnswerClarification(ctx, ClarificationResponse{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = accessor.CancelClarification(ctx, "x", "")
	require.Equal(t, codes.Unimplemented, status.Code(err))
}
