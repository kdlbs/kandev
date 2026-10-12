// AC-UI-INBOX-FAILED-001.29: the Failed tab golden path must also work on a
// phone-sized viewport, where the row's only control falls back to the 44px
// touch target (control-sizing.tsx's coarse-pointer breakpoint) instead of
// the desktop-density button. This mirrors inbox-failed-tab.spec.ts's
// desktop coverage rather than duplicating its assertions.
import { test, expect } from "../../fixtures/test-base";

const MIN_TOUCH_TARGET_PX = 44;

test.describe("Inbox Failed tab on mobile", () => {
  test("lists a failed task with a touch-sized open-task control (AC .29)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const title = "Mobile Inbox Failed Tab Fixture";
    const reason = "Simulated failure for the Inbox Failed tab";

    const { task_id: taskId } = await apiClient.seedTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.seedTaskSession(taskId, {
      // Keep the session non-terminal so its completed_at remains NULL. The
      // task is still failed below, which exercises the row's unknown-time
      // fallback without violating the harness contract for terminal
      // sessions.
      state: "IDLE",
      errorMessage: reason,
    });
    await apiClient.updateTaskState(taskId, "FAILED");

    await testPage.goto("/needs-you-inbox");

    // The Failed count is visible before the Failed tab is ever selected
    // (AC .29's ordering), same as the desktop coverage.
    const failedBadge = testPage.getByTestId("inbox-tab-failed-badge");
    await expect(failedBadge).toHaveText("1", { timeout: 30_000 });

    // The tab strip itself (not just the row's open-task control) is a
    // coarse-pointer surface an operator must tap to reach this tab.
    const failedTab = testPage.getByRole("tab", { name: /Failed/ });
    const failedTabBox = await failedTab.boundingBox();
    expect(failedTabBox, "Failed tab trigger should have a box").not.toBeNull();
    expect(failedTabBox!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);

    await failedTab.click();

    const row = testPage.getByTestId("failed-inbox-row").filter({ hasText: title });
    await expect(row).toBeVisible({ timeout: 15_000 });
    await expect(row).toContainText(reason);

    const openTask = row.getByTestId("failed-inbox-open-task");
    const openTaskBox = await openTask.boundingBox();
    expect(openTaskBox, "open-task control should have a box").not.toBeNull();
    expect(openTaskBox!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);

    const unknownTime = row.getByTestId("failed-inbox-unknown-time");
    await expect(unknownTime).toBeVisible();
    const rowBox = await row.boundingBox();
    const unknownTimeBox = await unknownTime.boundingBox();
    expect(rowBox, "failed row should have a box").not.toBeNull();
    expect(unknownTimeBox, "unknown failure time should have a box").not.toBeNull();
    expect(openTaskBox!.x + openTaskBox!.width).toBeLessThanOrEqual(rowBox!.x + rowBox!.width);
    expect(unknownTimeBox!.x + unknownTimeBox!.width).toBeLessThanOrEqual(
      rowBox!.x + rowBox!.width,
    );

    // The Inbox/Failed-tab page itself must not scroll horizontally on a
    // phone viewport -- checked here, before navigating away, so this
    // measures the page the test is actually about rather than the
    // task-detail page the next step opens.
    const overflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);

    await openTask.click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${taskId}$`));
  });
});
