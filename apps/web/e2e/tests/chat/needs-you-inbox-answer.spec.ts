// AC-UI-NEEDS-YOU-INBOX-001.30: listing an answerable bundle, expanding it,
// and answering it in place must remove the row (the convergence re-read)
// and resume the blocked task, without ever navigating to the task page.
import { test, expect } from "../../fixtures/test-base";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";

test.describe("Needs-you Inbox answer flow", () => {
  test("answering a row removes it and resumes the task without navigation (AC .30)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const title = "Needs-you Inbox Answer Flow";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      title,
      seedData.agentProfileId,
      {
        description: "/e2e:clarification",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected an active session for the Inbox answer flow");

    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "clarification session should reach its blocking state before the Inbox is opened",
      timeout: 60_000,
    });

    // Never visits /t/:id: the row is listed, expanded, and answered entirely
    // from the Inbox route.
    await testPage.goto("/needs-you-inbox");

    const row = testPage.getByTestId("needs-you-inbox-row").filter({ hasText: title });
    await expect(row).toBeVisible({ timeout: 30_000 });

    await row.getByTestId("needs-you-inbox-row-toggle").click();
    const overlay = row.getByTestId("clarification-overlay");
    await expect(overlay).toBeVisible({ timeout: 15_000 });

    await overlay.getByTestId("clarification-option").filter({ hasText: "PostgreSQL" }).click();

    // The outcome callback fires a convergence re-read; the row disappears
    // because the answered bundle is no longer answerable, not because the
    // client spliced it out locally.
    await expect(row).toHaveCount(0, { timeout: 15_000 });

    // `WAITING_FOR_INPUT` is the session's normal resting state once a turn
    // ends, so it is reached both while blocked on the clarification AND
    // again once the agent's post-answer turn finishes -- it cannot
    // distinguish "still blocked" from "resumed and settled". The mock
    // agent's own post-answer message is the unambiguous proof the blocked
    // tool call was actually released and the turn ran to completion.
    await waitForAgentMessage(apiClient, task.session_id, "You answered:", 30_000);
  });
});
