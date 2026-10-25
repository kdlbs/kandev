// AC-UI-NEEDS-YOU-INBOX-001.30: the answer-in-place golden path must also work
// on a phone-sized viewport, where rows and controls fall back to the 44px
// touch target (control-sizing.tsx's coarse-pointer breakpoint) instead of the
// desktop-density button. This mirrors needs-you-inbox-answer.spec.ts's
// desktop coverage rather than duplicating its assertions.
import { test, expect } from "../../fixtures/test-base";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";

const MIN_TOUCH_TARGET_PX = 44;

test.describe("Needs-you Inbox answer flow on mobile", () => {
  test("answering a row removes it and resumes the task without navigation (AC .30)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const title = "Mobile Needs-you Inbox Answer Flow";
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

    await testPage.goto("/needs-you-inbox");

    const row = testPage.getByTestId("needs-you-inbox-row").filter({ hasText: title });
    await expect(row).toBeVisible({ timeout: 30_000 });

    const toggle = row.getByTestId("needs-you-inbox-row-toggle");
    const toggleBox = await toggle.boundingBox();
    expect(toggleBox, "row toggle should have a box").not.toBeNull();
    expect(toggleBox!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);

    await toggle.click();
    const overlay = row.getByTestId("clarification-overlay");
    await expect(overlay).toBeVisible({ timeout: 15_000 });

    await overlay.getByTestId("clarification-option").filter({ hasText: "PostgreSQL" }).click();

    await expect(row).toHaveCount(0, { timeout: 15_000 });
    await waitForAgentMessage(apiClient, task.session_id, "You answered:", 30_000);

    // The document itself must not scroll horizontally on a phone viewport.
    const overflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);
  });
});
