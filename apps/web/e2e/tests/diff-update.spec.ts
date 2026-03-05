import { test, expect } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";
import type { ApiClient } from "../helpers/api-client";
import type { SeedData } from "../fixtures/test-base";
import type { Page } from "@playwright/test";

/**
 * Seed a task using the diff-update-setup mock scenario and navigate to
 * its session page, waiting for the agent turn to complete.
 *
 * The scenario creates a simple text file, commits it, then modifies line 1
 * to contain "FIRST_MODIFICATION", leaving an uncommitted diff.
 */
async function seedDiffUpdateTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{ session: SessionPage; sessionId: string }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Diff Update E2E",
    seedData.agentProfileId,
    {
      description: "/e2e:diff-update-setup",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/s/${task.session_id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();

  // Wait for the first turn to complete
  await expect(session.chat.getByText("diff-update-setup complete", { exact: false })).toBeVisible({
    timeout: 45_000,
  });

  return { session, sessionId: task.session_id };
}

/** Click the Changes dockview tab. */
async function openChangesTab(testPage: Page) {
  const changesTab = testPage.locator(".dv-default-tab", { hasText: "Changes" });
  await expect(changesTab).toBeVisible({ timeout: 10_000 });
  await changesTab.click();
}

/** Click the file row for diff_update_test.txt to open its diff view. */
async function openDiffUpdateFileDiff(testPage: Page) {
  const fileRow = testPage
    .locator("button, [role='button'], [class*='file']")
    .filter({ hasText: "diff_update_test.txt" })
    .first();
  await expect(fileRow).toBeVisible({ timeout: 10_000 });
  await fileRow.click();
}

test.describe("Diff update on file change", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("shows initial diff with FIRST_MODIFICATION", async ({ testPage, apiClient, seedData }) => {
    await seedDiffUpdateTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openDiffUpdateFileDiff(testPage);

    // The Pierre Diffs viewer should show the initial modification
    const diffsContainer = testPage.locator("diffs-container");
    await expect(diffsContainer).toBeVisible({ timeout: 15_000 });
    await expect(diffsContainer.getByText("FIRST_MODIFICATION", { exact: true })).toBeVisible({
      timeout: 15_000,
    });
  });

  test("diff updates when agent modifies file again", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedDiffUpdateTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openDiffUpdateFileDiff(testPage);

    // Verify initial diff content (scoped to diffs-container to avoid matching chat text)
    const diffsContainer = testPage.locator("diffs-container");
    await expect(diffsContainer).toBeVisible({ timeout: 15_000 });
    await expect(diffsContainer.getByText("FIRST_MODIFICATION", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    // Click on the Agent tab to make the chat input visible again
    await session.clickTab("Agent");

    // Send another message to trigger the second modification
    await session.sendMessage("/e2e:diff-update-modify");

    // Wait for the second turn to complete
    await expect(
      session.chat.getByText("diff-update-modify complete", { exact: false }),
    ).toBeVisible({ timeout: 45_000 });

    // Switch back to Changes tab and click on the diff file again to see the updated diff.
    // The git status (with diff data) should have been updated via polling when
    // the file changed - this is the bug we're testing for.
    await openChangesTab(testPage);
    await openDiffUpdateFileDiff(testPage);

    // Re-query the diffs container since the DOM may have changed after tab switch
    const updatedDiffsContainer = testPage.locator("diffs-container");
    await expect(updatedDiffsContainer).toBeVisible({ timeout: 15_000 });

    // The diff should now show SECOND_MODIFICATION instead of FIRST_MODIFICATION.
    // Allow extra time for git polling to detect the change and re-render the diff.
    await expect(
      updatedDiffsContainer.getByText("SECOND_MODIFICATION", { exact: true }),
    ).toBeVisible({ timeout: 30_000 });

    // Verify FIRST_MODIFICATION is no longer shown (replaced, not merged)
    await expect(
      updatedDiffsContainer.getByText("FIRST_MODIFICATION", { exact: true }),
    ).toHaveCount(0);

    // Also verify the additional change on line 3
    await expect(updatedDiffsContainer.getByText("ALSO_CHANGED", { exact: true })).toBeVisible({
      timeout: 15_000,
    });
  });
});
