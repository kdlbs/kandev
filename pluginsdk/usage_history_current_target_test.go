package pluginsdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUsageCurrentTargetRejectsHostIgnoringSelector(t *testing.T) {
	for _, snapshot := range []string{"older-snapshot", ""} {
		impl := &recordingUsageHistoryReader{page: UsageCollectionTargetPageV1{SnapshotID: snapshot}}
		host := dialHostOverBufconn(t, &usageHistoryRecordingHost{reader: impl})
		reader, ok := HostUsageHistory(host)
		require.True(t, ok)
		var query UsageCollectionTargetQueryV1
		require.NoError(t, json.Unmarshal([]byte(`{"WorkspaceID":"ws","CurrentTarget":{"TaskID":"task","SessionID":"session","ExternalSessionID":"transcript","SubjectBinding":"binding"}}`), &query))
		_, err := reader.ListTargets(context.Background(), query)
		require.Equal(t, codes.Unimplemented, status.Code(err), "point lookup must never fall back to archive enumeration")
	}
}

func TestUsageCurrentTargetRoundTripAndValidation(t *testing.T) {
	impl := &recordingUsageHistoryReader{page: UsageCollectionTargetPageV1{
		CurrentTarget: true, Targets: []UsageCollectionTargetV1{{TaskID: "task", SessionID: stringPointer("session"), SubjectBinding: "binding", ExternalSessionID: stringPointer("transcript"), SourceScopeStatus: "eligible"}},
	}}
	reader, ok := HostUsageHistory(dialHostOverBufconn(t, &usageHistoryRecordingHost{reader: impl}))
	require.True(t, ok)
	query := UsageCollectionTargetQueryV1{WorkspaceID: "ws", CurrentTarget: &UsageCollectionCurrentTargetQueryV1{TaskID: "task", SessionID: "session", ExternalSessionID: "transcript", SubjectBinding: "binding"}}
	page, err := reader.ListTargets(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, query, impl.query)
	require.Equal(t, impl.page, page)
	for _, bad := range []UsageCollectionTargetQueryV1{
		{CurrentTarget: query.CurrentTarget}, {WorkspaceID: "ws", CurrentTarget: &UsageCollectionCurrentTargetQueryV1{}},
		{WorkspaceID: "ws", CurrentTarget: query.CurrentTarget, SnapshotID: "snapshot"},
		{WorkspaceID: "ws", CurrentTarget: query.CurrentTarget, Cursor: "cursor"},
	} {
		_, err := reader.ListTargets(context.Background(), bad)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	}
	for _, bad := range []UsageCollectionTargetPageV1{
		{CurrentTarget: true, Preparing: true}, {CurrentTarget: true, SnapshotID: "snapshot"},
		{CurrentTarget: true, SnapshotUpperBound: 1}, {CurrentTarget: true, HasMore: true},
		{CurrentTarget: true, NextCursor: "cursor"}, {CurrentTarget: true, Targets: []UsageCollectionTargetV1{{}, {}}},
	} {
		impl.page = bad
		_, err := reader.ListTargets(context.Background(), query)
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
	}
}
