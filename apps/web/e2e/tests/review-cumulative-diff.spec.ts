import { test, expect } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";
import type { ApiClient } from "../helpers/api-client";
import type { SeedData } from "../fixtures/test-base";
import type { Page } from "@playwright/test";

/**
 * Seed a task that creates a file with BASE_CONTENT, commits it,
 * then commits a COMMITTED_CHANGE, then leaves an UNCOMMITTED_CHANGE.
 */
async function seedReviewCumulativeTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{ session: SessionPage; sessionId: string }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Review Cumulative Diff E2E",
    seedData.agentProfileId,
    {
      description: "/e2e:review-cumulative-setup",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/s/${task.session_id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();

  await expect(
    session.chat.getByText("review-cumulative-setup complete", { exact: false }),
  ).toBeVisible({ timeout: 45_000 });

  return { session, sessionId: task.session_id };
}

/** Open the review dialog via the custom DOM event. */
async function openReviewDialog(testPage: Page) {
  await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
}

test.describe("Review dialog cumulative diff", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("shows full diff from base commit including committed and uncommitted changes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedReviewCumulativeTask(testPage, apiClient, seedData);

    // Open the Changes tab first so the review dialog has git status data
    const changesTab = testPage.locator(".dv-default-tab", { hasText: "Changes" });
    await expect(changesTab).toBeVisible({ timeout: 10_000 });
    await changesTab.click();

    // Wait for git status to populate (file should appear in the changes panel)
    await expect(testPage.getByTestId("file-row-review_cumulative_test.txt")).toBeVisible({
      timeout: 15_000,
    });

    // Open the review dialog
    await openReviewDialog(testPage);

    // The review dialog renders inside a Dialog — find it via the sr-only title
    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible({ timeout: 10_000 });

    // Click on the file in the tree to ensure its diff is scrolled into view
    await dialog.getByText("review_cumulative_test.txt").first().click();

    // The diff should show COMMITTED_CHANGE — this is the key assertion.
    // With the old bug (git diff base..HEAD + uncommitted-first priority),
    // only the HEAD→working-tree diff would appear, so COMMITTED_CHANGE
    // (which was committed, not in the uncommitted diff) would be missing.
    await expect(dialog).toContainText("COMMITTED_CHANGE", { timeout: 30_000 });

    // The diff should also show UNCOMMITTED_CHANGE (current working tree state)
    await expect(dialog).toContainText("UNCOMMITTED_CHANGE", { timeout: 15_000 });
  });
});
