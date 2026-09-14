// AC-UI-INBOX-FAILED-001.29: observing the Failed tab's count before that tab
// is ever selected, then selecting it, observing a failed task listed with
// its reason and relative failure time, opening the task from the row, and
// observing that the sidebar count did not change when the failed task
// appeared.
import { test, expect } from "../../fixtures/test-base";

test.describe("Inbox Failed tab", () => {
  test("lists a failed task without inflating the sidebar count (AC .29)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const title = "Inbox Failed Tab Fixture";
    const reason = "Simulated failure for the Inbox Failed tab";

    const { task_id: taskId } = await apiClient.seedTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.seedTaskSession(taskId, {
      state: "FAILED",
      completedAt: new Date().toISOString(),
      errorMessage: reason,
    });
    await apiClient.updateTaskState(taskId, "FAILED");

    await testPage.goto("/needs-you-inbox");

    // The sidebar's "someone is blocked on you" count must never move when a
    // failed task exists (AC .14) -- captured before the Failed tab is ever
    // selected, so an accidental read into the shared count would show here.
    const sidebarInbox = testPage.getByTestId("sidebar-needs-you-inbox");
    await expect(sidebarInbox).toBeVisible();
    const sidebarTextBeforeFailedTabSelected = await sidebarInbox.textContent();

    // The tab strip's own Failed count becomes visible on Inbox mount --
    // before the Failed tab is ever the selected one (AC .29's ordering).
    const failedBadge = testPage.getByTestId("inbox-tab-failed-badge");
    await expect(failedBadge).toHaveText("1", { timeout: 30_000 });

    await testPage.getByRole("tab", { name: /Failed/ }).click();

    const row = testPage.getByTestId("failed-inbox-row").filter({ hasText: title });
    await expect(row).toBeVisible({ timeout: 15_000 });
    await expect(row).toContainText(reason);

    // A real failure instant renders a relative time, not the "unknown" fallback.
    await expect(row.getByTestId("failed-inbox-unknown-time")).toHaveCount(0);

    await row.getByTestId("failed-inbox-open-task").click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${taskId}$`));

    const sidebarTextAfterOpeningTask = await sidebarInbox.textContent();
    expect(sidebarTextAfterOpeningTask).toBe(sidebarTextBeforeFailedTabSelected);
  });
});
