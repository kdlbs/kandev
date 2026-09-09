/**
 * Mobile parity for the sidebar queued prompt count badge.
 *
 * The mobile task-switcher sheet renders through the same TaskItem rows as
 * the desktop sidebar, so the mail badge must appear there too, without any
 * hover dependency. Lives in `mobile-*.spec.ts` so the `mobile-chrome`
 * Playwright project applies the mobile device automatically.
 */
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile sidebar — queued prompt count", () => {
  test("shows the queued count badge in the task switcher sheet", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile Queued Badge Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      agentProfileId: seedData.agentProfileId,
    });
    await apiClient.queueMessage(task.id, sessionId, "Mobile queued prompt");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Open the task switcher sheet from the mobile session top bar.
    await testPage.getByTestId("mobile-session-menu").click();
    const sheet = testPage.getByRole("dialog");
    await expect(sheet.getByText("Mobile Queued Badge Task")).toBeVisible({ timeout: 10_000 });

    const badge = sheet
      .locator(`[data-testid='sortable-task-block'][data-task-id='${task.id}']`)
      .getByTestId("sidebar-task-queued-count");
    await expect(badge).toBeVisible({ timeout: 10_000 });
    await expect(badge).toHaveText("1");

    await testPage.screenshot({
      path: "e2e/test-results/screenshots/sidebar-queued-count-mobile.png",
      fullPage: false,
    });

    // No horizontal overflow from the badge on a phone viewport: the badge
    // stays within the sheet's horizontal bounds.
    const [sheetBox, badgeBox] = await Promise.all([sheet.boundingBox(), badge.boundingBox()]);
    expect(sheetBox).not.toBeNull();
    expect(badgeBox).not.toBeNull();
    expect(badgeBox!.x).toBeGreaterThanOrEqual(sheetBox!.x);
    expect(badgeBox!.x + badgeBox!.width).toBeLessThanOrEqual(sheetBox!.x + sheetBox!.width);
  });
});
