// Filename starts with "mobile-" so the mobile-chrome project exercises the
// touch path for the Fix Comments overview.
import { test, expect } from "../../fixtures/test-base";
import {
  loadSession,
  openDialogWithChanges,
  seedComments,
  seedReviewTask,
} from "./review-fix-comments-popover-flow";

test.describe("Review dialog Fix Comments popover on mobile", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("tapping Fix Comments opens the comments overview before sending", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await seedReviewTask(testPage, apiClient, seedData);
    const sessionId = task.session_id!;
    expect(sessionId).toBeTruthy();
    const comments = await seedComments(testPage, sessionId);

    await loadSession(testPage, task.id);
    const dialog = await openDialogWithChanges(testPage);

    const button = dialog.getByTestId("review-fix-comments-button");
    await expect(button).toBeVisible();
    await button.tap();

    const overview = testPage.getByTestId("review-comments-overview");
    await expect(overview).toBeVisible();
    await expect(overview).toContainText(`${comments.length} pending review comments`);
    await expect(dialog).toBeVisible();
  });
});
