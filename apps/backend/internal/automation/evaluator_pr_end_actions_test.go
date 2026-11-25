package automation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/github"
)

func newEndActionFixture(t *testing.T, merged, closed PREndAction) (*prFollowUpFixture, *fakePRTaskController) {
	t.Helper()
	f := newPRFollowUpFixture(t, followUpPR{Number: 17, Head: "a"}, followUpPR{Number: 18, Head: "a"})
	controller := &fakePRTaskController{}
	f.svc.SetPRTaskController(controller)
	policy := ContinuationPolicyReusePerPR
	_, err := f.svc.UpdateAutomation(context.Background(), f.automation.ID, &UpdateAutomationRequest{
		ContinuationPolicy: &policy, PRMergedAction: &merged, PRClosedAction: &closed,
	})
	require.NoError(t, err)
	f.addTrigger(t, newCommitsEvents)
	ctx := context.Background()
	require.NoError(t, f.svc.SetPRContinuationTaskID(ctx, f.automation.ID, "acme/widget#17", "task-17"))
	require.NoError(t, f.svc.SetPRContinuationTaskID(ctx, f.automation.ID, "acme/widget#18", "task-18"))
	f.transport.ended = map[int]string{}
	return f, controller
}

func (f *prFollowUpFixture) mapping(t *testing.T, subject string) string {
	t.Helper()
	taskID, err := f.svc.Store().GetPRContinuationTaskID(context.Background(), f.automation.ID, subject)
	require.NoError(t, err)
	return taskID
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.2
// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.4
func TestEndOfPRActionsChooseByOutcome(t *testing.T) {
	f, controller := newEndActionFixture(t, PREndActionArchive, PREndActionDelete)
	f.evaluate()
	require.Equal(t, "task-17", f.mapping(t, "acme/widget#17"), "open PRs keep their mapping")

	f.transport.ended[17] = "merged"
	f.transport.ended[18] = "closed"
	f.setPRs()
	f.evaluate()

	require.Equal(t, []string{"task-17"}, controller.archived, "merged PR uses pr_merged_action")
	require.Equal(t, []string{"task-18"}, controller.deleted, "closed PR uses pr_closed_action")
	require.Empty(t, f.mapping(t, "acme/widget#17"))
	require.Empty(t, f.mapping(t, "acme/widget#18"))
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.4
func TestEndOfPRKeepRemovesMappingOnly(t *testing.T) {
	f, controller := newEndActionFixture(t, PREndActionKeep, PREndActionKeep)
	f.transport.ended[17] = "merged"
	f.setPRs(followUpPR{Number: 18, Head: "a"})
	f.evaluate()

	require.Empty(t, controller.archived)
	require.Empty(t, controller.deleted)
	require.Empty(t, f.mapping(t, "acme/widget#17"), "keep still ends the mapping")
	require.Equal(t, "task-18", f.mapping(t, "acme/widget#18"))
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.3
func TestEndOfPRActionWaitsWhileTaskBusy(t *testing.T) {
	for _, action := range []PREndAction{PREndActionArchive, PREndActionKeep} {
		t.Run(string(action), func(t *testing.T) {
			f, controller := newEndActionFixture(t, action, action)
			controller.busy = true
			f.transport.ended[17] = "merged"
			f.setPRs(followUpPR{Number: 18, Head: "a"})
			f.evaluate()
			require.Empty(t, controller.archived)
			require.Equal(t, "task-17", f.mapping(t, "acme/widget#17"), "a busy task keeps its mapping for retry")

			controller.busy = false
			f.evaluate()
			require.Empty(t, f.mapping(t, "acme/widget#17"))
			if action == PREndActionArchive {
				require.Equal(t, []string{"task-17"}, controller.archived)
			} else {
				require.Empty(t, controller.archived)
			}
		})
	}
}

func TestEndOfPRActionSkipsPartialListingAndFailedRead(t *testing.T) {
	f, controller := newEndActionFixture(t, PREndActionArchive, PREndActionArchive)
	f.setPRs(followUpPR{Number: 18, Head: "a"})

	f.transport.partial = true
	f.transport.ended[17] = "merged"
	f.evaluate()
	require.Empty(t, controller.archived, "a partial listing runs no end-of-PR action")

	f.transport.partial = false
	delete(f.transport.ended, 17)
	f.evaluate()
	require.Empty(t, controller.archived, "a failed PR read is retried later")
	require.Equal(t, "task-17", f.mapping(t, "acme/widget#17"))
}

func TestEndOfPRActionsIgnoreOtherPolicies(t *testing.T) {
	f := newPRFollowUpFixture(t)
	controller := &fakePRTaskController{}
	f.svc.SetPRTaskController(controller)
	f.addTrigger(t, newCommitsEvents)
	require.NoError(t, f.svc.SetPRContinuationTaskID(context.Background(), f.automation.ID, "acme/widget#17", "task-17"))
	f.transport.ended = map[int]string{17: "merged"}
	f.evaluate()
	require.Empty(t, controller.archived)
	require.Equal(t, "task-17", f.mapping(t, "acme/widget#17"))
}

// An automation disabled after the poll began never applies a stale action.
func TestEndOfPRActionSkippedWhenAutomationDisabled(t *testing.T) {
	f, controller := newEndActionFixture(t, PREndActionDelete, PREndActionDelete)
	disabled := false
	_, err := f.svc.UpdateAutomation(context.Background(), f.automation.ID, &UpdateAutomationRequest{Enabled: &disabled})
	require.NoError(t, err)
	f.transport.ended[17] = "closed"
	f.setPRs(followUpPR{Number: 18, Head: "a"})
	evalAutomation, err := f.svc.GetAutomation(context.Background(), f.automation.ID)
	require.NoError(t, err)
	evalAutomation.Enabled = true // the snapshot taken before the provider read
	f.evaluator.runEndOfPRActions(context.Background(), evalAutomation, repoFilterForTest(), map[int]bool{18: true})

	require.Empty(t, controller.deleted)
	require.Equal(t, "task-17", f.mapping(t, "acme/widget#17"))
}

func repoFilterForTest() github.RepoFilter { return github.RepoFilter{Owner: "acme", Name: "widget"} }
