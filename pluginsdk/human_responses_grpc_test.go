package pluginsdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type humanResponseHostFixture struct {
	recordingHost
	delegation *humanResponseDelegationFixture
}

func (h *humanResponseHostFixture) HumanResponseDelegation() HumanResponseDelegationManager {
	return h.delegation
}

type humanResponseDelegationFixture struct {
	link    ExternalIdentityLinkRequest
	receipt HumanResponseReceiptRequest
}

func (f *humanResponseDelegationFixture) LinkExternalIdentity(_ context.Context, in ExternalIdentityLinkRequest) (*CommandResult, ExternalIdentityLink, error) {
	f.link = in
	return &CommandResult{Status: CommandApplied}, ExternalIdentityLink{
		ID: "link-1", Provider: in.Provider, Subject: in.Subject, DisplayName: in.DisplayName,
		UserID: "user-1", UserDisplayName: "Alice", CreatedAt: "2026-11-22T10:00:00Z",
	}, nil
}

func (f *humanResponseDelegationFixture) RequestHumanResponseReceipt(_ context.Context, in HumanResponseReceiptRequest) (*CommandResult, HumanResponseReceipt, error) {
	f.receipt = in
	return &CommandResult{Status: CommandApplied}, HumanResponseReceipt{
		ID: "receipt-1", Kind: in.Kind, TargetID: in.TargetID, ResourceVersion: in.ExpectedResourceVersion,
		ExpiresAt: "2026-11-22T10:05:00Z", Actor: InteractionActor{UserID: "user-1", DisplayName: "Alice", Via: in.Provider},
	}, nil
}

func TestHumanResponseDelegationRoundTrip(t *testing.T) {
	fixture := &humanResponseHostFixture{delegation: &humanResponseDelegationFixture{}}
	delegation, ok := HostHumanResponseDelegation(dialHostOverBufconn(t, fixture))
	require.True(t, ok)

	linkRequest := ExternalIdentityLinkRequest{RequestID: "request-link", Code: "ABCD-EFGH-JKMN", Provider: "discord", Subject: "1234", DisplayName: "alice#1"}
	result, link, err := delegation.LinkExternalIdentity(context.Background(), linkRequest)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, linkRequest, fixture.delegation.link)
	require.Equal(t, ExternalIdentityLink{
		ID: "link-1", Provider: "discord", Subject: "1234", DisplayName: "alice#1",
		UserID: "user-1", UserDisplayName: "Alice", CreatedAt: "2026-11-22T10:00:00Z",
	}, link)

	receiptRequest := HumanResponseReceiptRequest{
		RequestID: "request-receipt", WorkspaceID: "workspace-1", Provider: "discord", Subject: "1234",
		Kind: HumanResponseKindClarification, TargetID: "interaction-1", ExpectedResourceVersion: "sha256:v1",
		Answers:          []ClarificationAnswer{{QuestionID: "q1", SelectedOptions: []string{"yes"}, CustomText: "ok"}},
		ApprovalRevision: 3, ManifestDigest: "digest",
	}
	result, receipt, err := delegation.RequestHumanResponseReceipt(context.Background(), receiptRequest)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, receiptRequest, fixture.delegation.receipt)
	require.Equal(t, InteractionActor{UserID: "user-1", DisplayName: "Alice", Via: "discord"}, receipt.Actor)
	require.Equal(t, "receipt-1", receipt.ID)
}

func TestInteractionResolvedByRoundTrip(t *testing.T) {
	interaction := Interaction{ID: "interaction-1", Status: InteractionStatusApproved, ResolvedBy: &InteractionActor{UserID: "user-1", DisplayName: "Alice", Via: "discord"}}
	require.Equal(t, interaction, interactionFromProto(interaction.toProto()))
	unresolved := Interaction{ID: "interaction-2", Status: InteractionStatusPending}
	require.Nil(t, interactionFromProto(unresolved.toProto()).ResolvedBy)
}
