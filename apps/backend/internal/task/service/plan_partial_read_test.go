package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/contract"
	"github.com/stretchr/testify/require"
)

func planReadPointer[T any](value T) *T { return &value }

// @covers AC-TASKS-PLAN-READ-001.1 AC-TASKS-PLAN-READ-001.4 AC-TASKS-PLAN-READ-001.5
func TestPlanPartialReadRangesAndNoMutation(t *testing.T) {
	svc, eventBus, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "range-plan"
	seedTask(t, ctx, repo, taskID)
	content := "a\r\n猫😀e\u0301\nlast"
	_, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: taskID, Content: content})
	require.NoError(t, err)
	seedSession(t, ctx, repo, taskID, "range-session")
	_, err = svc.MarkImplementationStarted(ctx, MarkImplementationStartedRequest{
		TaskID: taskID, SessionID: "range-session", Actor: "user",
	})
	require.NoError(t, err)
	plan, err := svc.GetPlanSnapshot(ctx, taskID)
	require.NoError(t, err)
	comments, err := svc.CreatePlanComment(ctx, CreatePlanCommentRequest{
		TaskID: taskID, PlanID: plan.ID, ID: "d8d97d5b-2663-45aa-aad4-64580a9ae07f",
		Body: "Keep this comment", SelectedText: "a", AnchorFrom: 0, AnchorTo: 1,
	})
	require.NoError(t, err)
	before, err := svc.GetPlanSnapshot(ctx, taskID)
	require.NoError(t, err)
	history, err := svc.ListRevisions(ctx, taskID)
	require.NoError(t, err)
	eventCount := len(eventBus.GetPublishedEvents())
	require.Positive(t, eventCount, "fixture writes must publish events")

	cases := []struct {
		name    string
		offset  *int64
		limit   *int64
		content string
		count   int64
		next    *int64
	}{
		{"full", nil, nil, content, 0, nil},
		{"offset enables default limit", planReadPointer(int64(0)), nil, content, 12, nil},
		{"limit defaults offset", nil, planReadPointer(int64(3)), "a\r\n", 3, planReadPointer(int64(3))},
		{"unicode", planReadPointer(int64(3)), planReadPointer(int64(4)), "猫😀e\u0301", 4, planReadPointer(int64(7))},
		{"suffix", planReadPointer(int64(8)), planReadPointer(int64(8192)), "last", 4, nil},
		{"EOF", planReadPointer(int64(12)), nil, "", 0, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := svc.GetPlanRead(ctx, taskID, contract.PlanReadOptions{
				Offset: tc.offset, Limit: tc.limit, ExpectedVersion: &before.WriteVersion,
			})
			require.NoError(t, err)
			require.Equal(t, tc.content, result.Plan.Content)
			require.Equal(t, before.WriteVersion, result.Plan.WriteVersion)
			if tc.offset == nil && tc.limit == nil {
				require.Nil(t, result.Range)
				return
			}
			require.Equal(t, tc.count, result.Range.ReturnedCharacters)
			require.Equal(t, tc.next, result.Range.NextOffset)
			require.Equal(t, tc.next != nil, result.Range.HasMore)
			require.Equal(t, int64(12), result.Range.TotalCharacters)
			require.Equal(t, len(content), result.Range.TotalContentBytes)
			require.Equal(t, len(tc.content), result.Range.ContentBytes)
		})
	}
	_, err = svc.GetPlanRead(ctx, taskID, contract.PlanReadOptions{Offset: planReadPointer(int64(13))})
	require.ErrorIs(t, err, ErrPlanReadOffsetOutOfRange)
	stale, err := svc.GetPlanRead(ctx, taskID, contract.PlanReadOptions{ExpectedVersion: planReadPointer("old")})
	require.ErrorIs(t, err, ErrPlanVersionConflict)
	require.Nil(t, stale)
	after, err := svc.GetPlanSnapshot(ctx, taskID)
	require.NoError(t, err)
	require.Equal(t, before, after)
	afterHistory, err := svc.ListRevisions(ctx, taskID)
	require.NoError(t, err)
	require.Equal(t, history, afterHistory)
	afterComments, err := svc.ListPlanComments(ctx, taskID)
	require.NoError(t, err)
	require.Equal(t, comments, afterComments)
	require.Len(t, eventBus.GetPublishedEvents(), eventCount)
}

// @covers AC-TASKS-PLAN-READ-001.2 AC-TASKS-PLAN-READ-002.4
func TestPlanPartialReadInvalidOptionsAndAuthorization(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "private-range"
	seedTask(t, ctx, repo, taskID)
	_, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: taskID, Content: "secret"})
	require.NoError(t, err)
	for _, options := range []contract.PlanReadOptions{
		{Offset: planReadPointer(int64(-1))}, {Offset: planReadPointer(contract.MaxPlanReadOffset + 1)},
		{Limit: planReadPointer(int64(0))}, {Limit: planReadPointer(int64(8193))},
		{ExpectedVersion: planReadPointer("")},
	} {
		result, err := svc.GetPlanRead(ctx, taskID, options)
		require.Nil(t, result)
		var invalid *contract.PlanReadValidationError
		require.ErrorAs(t, err, &invalid)
	}
	denied := errors.New("access denied")
	svc.SetTaskAuthorizer(func(context.Context, string) error { return denied })
	for _, options := range []contract.PlanReadOptions{
		{}, {Offset: planReadPointer(contract.MaxPlanReadOffset)}, {ExpectedVersion: planReadPointer("stale")},
	} {
		result, err := svc.GetPlanRead(ctx, taskID, options)
		require.Nil(t, result)
		require.ErrorIs(t, err, denied)
	}
}

// @covers AC-TASKS-PLAN-READ-001.5 AC-TASKS-PLAN-READ-002.1
func TestPlanPartialReadMissingAndUnreadable(t *testing.T) {
	svc, eventBus, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "missing-range")
	result, err := svc.GetPlanRead(ctx, "missing-range", contract.PlanReadOptions{Offset: planReadPointer(int64(0))})
	require.NoError(t, err)
	require.Nil(t, result)
	result, err = svc.GetPlanRead(ctx, "missing-range", contract.PlanReadOptions{ExpectedVersion: planReadPointer("deleted")})
	require.ErrorIs(t, err, ErrPlanVersionConflict)
	require.Nil(t, result)
	flaky := newFlakyPlanService(t, repo, eventBus, 1)
	result, err = flaky.GetPlanRead(ctx, "missing-range", contract.PlanReadOptions{})
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrTaskPlanNotFound)
	require.Nil(t, result)
}

// @covers AC-TASKS-PLAN-READ-001.6
func TestPlanPartialReadOversizedLongLine(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "legacy-long-range"
	seedTask(t, ctx, repo, taskID)
	_, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: taskID, Content: "initial"})
	require.NoError(t, err)
	content := strings.Repeat("猫", 100000)
	_, err = repo.DB().ExecContext(ctx, "UPDATE task_plans SET content = ? WHERE task_id = ?", content, taskID)
	require.NoError(t, err)
	for _, options := range []contract.PlanReadOptions{
		{Offset: planReadPointer(int64(0))},
		{Offset: planReadPointer(int64(80000)), Limit: planReadPointer(int64(8192))},
	} {
		result, err := svc.GetPlanRead(ctx, taskID, options)
		require.NoError(t, err)
		want := int64(4096)
		if options.Limit != nil {
			want = *options.Limit
		}
		require.Equal(t, strings.Repeat("猫", int(want)), result.Plan.Content)
		require.Equal(t, int64(100000), result.Range.TotalCharacters)
		require.Equal(t, 300000, result.Range.TotalContentBytes)
	}
	_, err = repo.DB().ExecContext(ctx, "UPDATE task_plans SET content = '' WHERE task_id = ?", taskID)
	require.NoError(t, err)
	empty, err := svc.GetPlanRead(ctx, taskID, contract.PlanReadOptions{Offset: planReadPointer(int64(0))})
	require.NoError(t, err)
	require.Empty(t, empty.Plan.Content)
	require.Equal(t, int64(0), empty.Range.TotalCharacters)
	require.Nil(t, empty.Range.NextOffset)
}
