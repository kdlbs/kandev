package pluginsdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type sourceIssueWritebackHostFixture struct {
	exactHostFixture
	manager *sourceIssueWritebackManagerFixture
}

func (h *sourceIssueWritebackHostFixture) SourceIssueWriteback() ExactSourceIssueWritebackManager {
	return h.manager
}

type sourceIssueWritebackManagerFixture struct {
	query      SourceIssueCapabilitiesQuery
	comment    SourceIssueWritebackCommand
	transition SourceIssueWritebackCommand
}

func (f *sourceIssueWritebackManagerFixture) GetCapabilities(_ context.Context, query SourceIssueCapabilitiesQuery) (*CommandResult, *SourceIssueCapabilities, *HostReadReceipt, error) {
	f.query = query
	return &CommandResult{Status: CommandApplied}, &SourceIssueCapabilities{
		WorkspaceID: query.WorkspaceID, TaskID: query.TaskID, TaskResourceVersion: "task-v1",
		Provider: "linear", SourceID: "linear-id-1", Identifier: "ENG-1", URL: "https://linear.app/issue/ENG-1",
		Title: "Issue", StatusID: "state-open", StatusName: "Open",
		Transitions:     []SourceIssueTransition{{TargetID: "state-done", TargetName: "Done"}},
		ResourceVersion: "source-v1", ObservedAt: "2026-09-26T11:00:00Z",
	}, &HostReadReceipt{InstallationID: "installation-1", WorkspaceID: query.WorkspaceID, RequestID: query.RequestID}, nil
}

func (f *sourceIssueWritebackManagerFixture) Comment(_ context.Context, command SourceIssueWritebackCommand) (*CommandResult, *SourceIssueWritebackReceipt, error) {
	f.comment = command
	return &CommandResult{Status: CommandUncertain, Reason: "provider_outcome_unknown"}, &SourceIssueWritebackReceipt{
		ReceiptID: "source-receipt-1", OperationID: "operation-1", InstallationID: "installation-1",
		WorkspaceID: command.WorkspaceID, TaskID: command.TaskID, Provider: "linear", SourceID: "linear-id-1",
		Operation: "comment", State: "uncertain", Status: CommandUncertain, Reason: "provider_outcome_unknown",
		ExpectedSourceVersion: command.ExpectedSourceResourceVersion,
	}, nil
}

func (f *sourceIssueWritebackManagerFixture) Transition(_ context.Context, command SourceIssueWritebackCommand) (*CommandResult, *SourceIssueWritebackReceipt, error) {
	f.transition = command
	return &CommandResult{Status: CommandApplied}, &SourceIssueWritebackReceipt{
		ReceiptID: "source-receipt-2", OperationID: "operation-2", InstallationID: "installation-1",
		WorkspaceID: command.WorkspaceID, TaskID: command.TaskID, Provider: "linear", SourceID: "linear-id-1",
		Operation: "transition", State: "completed", Status: CommandApplied,
	}, nil
}

func TestExactSourceIssueWritebackGrpcRoundTrip(t *testing.T) {
	fixture := &sourceIssueWritebackHostFixture{manager: &sourceIssueWritebackManagerFixture{}}
	host := dialHostOverBufconn(t, fixture)
	manager, ok := HostSourceIssueWriteback(host)
	require.True(t, ok)

	query := SourceIssueCapabilitiesQuery{RequestID: "read-1", WorkspaceID: "workspace-1", TaskID: "task-1"}
	result, capabilities, readReceipt, err := manager.GetCapabilities(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, query, fixture.manager.query)
	require.Equal(t, "linear-id-1", capabilities.SourceID)
	require.Equal(t, "state-done", capabilities.Transitions[0].TargetID)
	require.Equal(t, "installation-1", readReceipt.InstallationID)

	comment := SourceIssueWritebackCommand{
		RequestID: "comment-1", WorkspaceID: "workspace-1", TaskID: "task-1", IdempotencyKey: "comment-key",
		ExpectedTaskResourceVersion: "task-v1", ExpectedSourceResourceVersion: "source-v1",
		ApprovalRevision: 3, ManifestDigest: "manifest-3", Body: "Waiting for review",
	}
	result, receipt, err := manager.Comment(context.Background(), comment)
	require.NoError(t, err)
	require.Equal(t, CommandUncertain, result.Status)
	require.Equal(t, comment, fixture.manager.comment)
	require.Equal(t, "installation-1", receipt.InstallationID)
	require.Equal(t, "linear-id-1", receipt.SourceID)
	require.Equal(t, "uncertain", receipt.State)
	require.Equal(t, CommandUncertain, receipt.Status)

	transition := comment
	transition.RequestID = "transition-1"
	transition.TargetID = "state-done"
	transition.Body = ""
	result, receipt, err = manager.Transition(context.Background(), transition)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, transition, fixture.manager.transition)
	require.Equal(t, "transition", receipt.Operation)
}
