import { test, expect } from "../../fixtures/test-base";
import {
  DIFF_FILE,
  hoverFixComments,
  loadSession,
  openDialogWithChanges,
  seedComments,
  seedReviewTask,
} from "./review-fix-comments-popover-flow";

test.describe("Review dialog Fix Comments popover", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("hovering Fix Comments shows a scrollable per-file overview", async ({
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
    // Count badge reflects only comments on files present in the diff — here
    // the single DIFF_FILE comment.
    await expect(button).toContainText("1");

    const overview = testPage.getByTestId("review-comments-overview");
    await hoverFixComments(testPage, button, overview);

    // Header summarizes totals across every pending comment (not just the ones
    // on diff files): 41 comments across 6 files (DIFF_FILE + 5 module files).
    await expect(overview).toContainText(`${comments.length} pending review comments`);
    await expect(overview.getByText(/across 6 files/)).toBeVisible();

    // Per-file grouping: the diff file and at least one module file appear.
    await expect(overview.getByText(DIFF_FILE, { exact: true })).toBeVisible();
    await expect(overview.getByText("file-1.ts", { exact: true })).toBeVisible();

    // The overview overflows its max height, so the inner region must scroll.
    const scroller = overview.getByTestId("review-comments-overview-scroll");
    const metrics = await scroller.evaluate((el) => ({
      scrollHeight: el.scrollHeight,
      clientHeight: el.clientHeight,
    }));
    expect(metrics.scrollHeight).toBeGreaterThan(metrics.clientHeight);

    await scroller.evaluate((el) => {
      el.scrollTop = el.scrollHeight;
    });
    await expect
      .poll(async () => scroller.evaluate((el) => el.scrollTop), { timeout: 5_000 })
      .toBeGreaterThan(0);

    // The bridge keeps the popover open while the cursor is over the content.
    await scroller.hover();
    await expect(overview).toBeVisible();
  });

  test("no Fix Comments button when there are no comments", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await seedReviewTask(testPage, apiClient, seedData);
    await loadSession(testPage, task.id);
    const dialog = await openDialogWithChanges(testPage);

    await expect(dialog.getByTestId("review-fix-comments-button")).toHaveCount(0);
  });
});
