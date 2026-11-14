package plugins

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/analytics"
	analyticsmodels "github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type changingCanonicalPageSource struct {
	pendingUsageProjectionSource
	generation string
	lastFilter analyticsmodels.SessionUsageFilter
}

func (s *changingCanonicalPageSource) ListCanonicalSessionUsagePage(_ context.Context, filter analyticsmodels.SessionUsageFilter) ([]analyticsmodels.SessionUsageMeasurement, analyticsmodels.UsageProjectionState, error) {
	s.lastFilter = filter
	state := analyticsmodels.UsageProjectionState{Status: "ready", Generation: s.generation}
	if filter.ProjectionGeneration != "" && filter.ProjectionGeneration != s.generation {
		return nil, state, analytics.ErrUsageProjectionChanged
	}
	rows := []analyticsmodels.SessionUsageMeasurement{{ID: "a"}, {ID: "b"}}
	return rows[min(filter.Offset, len(rows)):], state, nil
}

func TestUsageHistoryHostCanonicalCursorPinsPublicationAndFilter(t *testing.T) {
	fixture := newTestDataHost(manifest.Capabilities{APIRead: []string{"session_usage"}})
	fixture.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	source := &changingCanonicalPageSource{generation: "opaque/generation.7"}
	fixture.host.usageDep = func() sessionUsageSource { return source }
	host := dialPluginHostOverWire(t, fixture.host)
	filter := pluginsdk.SessionUsageFilter{WorkspaceID: "ws-1", Provider: "provider"}
	rows, first, err := host.Usage().ListCanonical(t.Context(), filter, pluginsdk.Page{Limit: 1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, first.HasMore)
	rows, second, err := host.Usage().ListCanonical(t.Context(), filter, pluginsdk.Page{Limit: 1, Cursor: first.NextCursor})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.False(t, second.HasMore)
	require.Equal(t, first.ProjectionGeneration, source.lastFilter.ProjectionGeneration, "the next saved page must request the first complete publication")
	require.Equal(t, 1, source.lastFilter.Offset)
	changedFilter := filter
	changedFilter.Provider = "other-provider"
	_, _, err = host.Usage().ListCanonical(t.Context(), changedFilter, pluginsdk.Page{Limit: 1, Cursor: first.NextCursor})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	source.generation = "opaque/generation.8"
	_, _, err = host.Usage().ListCanonical(t.Context(), filter, pluginsdk.Page{Limit: 1, Cursor: first.NextCursor})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "a client must retry instead of combining two publications")
}
