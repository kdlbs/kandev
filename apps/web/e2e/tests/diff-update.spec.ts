import { test, expect } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";
import type { ApiClient } from "../helpers/api-client";
import type { SeedData } from "../fixtures/test-base";
import type { Page } from "@playwright/test";

// Shared selector constant for diff container
const DIFFS_CONTAINER = "diffs-container";

/** Options for seeding a task with an agent. */
type SeedTaskOptions = {
  title: string;
  scenarioCommand: string;
  completionText: string;
};

/**
 * Generic helper to seed a task using a mock scenario and navigate to
 * its session page, waiting for the agent turn to complete.
 */
async function seedTaskWithScenario(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  options: SeedTaskOptions,
): Promise<{ session: SessionPage; sessionId: string }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    options.title,
    seedData.agentProfileId,
    {
      description: options.scenarioCommand,
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
  await expect(session.chat.getByText(options.completionText, { exact: false })).toBeVisible({
    timeout: 45_000,
  });

  return { session, sessionId: task.session_id };
}

/** Seed a task for untracked file testing. */
function seedUntrackedFileTask(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  return seedTaskWithScenario(testPage, apiClient, seedData, {
    title: "Untracked File E2E",
    scenarioCommand: "/e2e:untracked-file-setup",
    completionText: "untracked-file-setup complete",
  });
}

/** Seed a task for diff update testing. */
function seedDiffUpdateTask(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  return seedTaskWithScenario(testPage, apiClient, seedData, {
    title: "Diff Update E2E",
    scenarioCommand: "/e2e:diff-update-setup",
    completionText: "diff-update-setup complete",
  });
}

/** Click the Changes dockview tab. */
async function openChangesTab(testPage: Page) {
  const changesTab = testPage.locator(".dv-default-tab", { hasText: "Changes" });
  await expect(changesTab).toBeVisible({ timeout: 10_000 });
  await changesTab.click();
}

/** Click a file row by name to open its diff view. */
async function openFileDiff(testPage: Page, fileName: string) {
  const fileRow = testPage
    .locator("button, [role='button'], [class*='file']")
    .filter({ hasText: fileName })
    .first();
  await expect(fileRow).toBeVisible({ timeout: 10_000 });
  await fileRow.click();
}

/** Helper to get the diffs container locator. */
function getDiffsContainer(testPage: Page) {
  return testPage.locator(DIFFS_CONTAINER);
}

test.describe("Diff update on file change", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("shows initial diff with FIRST_MODIFICATION", async ({ testPage, apiClient, seedData }) => {
    await seedDiffUpdateTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openFileDiff(testPage, "diff_update_test.txt");

    // The Pierre Diffs viewer should show the initial modification.
    // Playwright's getByText auto-pierces shadow DOM and auto-retries, so we use it
    // directly with a generous timeout to handle async web worker initialization.
    const diffsContainer = getDiffsContainer(testPage);
    await expect(diffsContainer).toBeVisible({ timeout: 15_000 });
    await expect(diffsContainer.getByText("FIRST_MODIFICATION", { exact: true })).toBeVisible({
      timeout: 15_000,
    });
  });

  test("diff updates when agent modifies file again", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedDiffUpdateTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openFileDiff(testPage, "diff_update_test.txt");

    // Verify initial diff content (scoped to diffs-container to avoid matching chat text)
    const diffsContainer = getDiffsContainer(testPage);
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
    await openFileDiff(testPage, "diff_update_test.txt");

    // Re-query the diffs container since the DOM may have changed after tab switch
    const updatedDiffsContainer = getDiffsContainer(testPage);
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

test.describe("Untracked file diff update", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("untracked file diff updates when modified", async ({ testPage, apiClient, seedData }) => {
    // This test verifies that modifying an untracked file triggers a git status update
    // and the diff viewer shows the updated content. This was a bug where the polling
    // mechanism didn't detect untracked file changes (git diff-files only shows tracked files).
    const { session } = await seedUntrackedFileTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openFileDiff(testPage, "untracked_test.txt");

    // Verify initial diff content shows INITIAL_CONTENT
    // Note: exact match is false because the line shows "line 1: INITIAL_CONTENT"
    const diffsContainer = getDiffsContainer(testPage);
    await expect(diffsContainer).toBeVisible({ timeout: 15_000 });
    await expect(diffsContainer.getByText("INITIAL_CONTENT")).toBeVisible({
      timeout: 15_000,
    });

    // Click on the Agent tab to make the chat input visible again
    await session.clickTab("Agent");

    // Send another message to trigger the modification
    await session.sendMessage("/e2e:untracked-file-modify");

    // Wait for the second turn to complete
    await expect(
      session.chat.getByText("untracked-file-modify complete", { exact: false }),
    ).toBeVisible({ timeout: 45_000 });

    // Wait for git polling to detect the file change (polling interval is ~1-2s)
    await testPage.waitForTimeout(3_000);

    // Switch back to Changes tab and click on the diff file again
    await openChangesTab(testPage);
    await openFileDiff(testPage, "untracked_test.txt");

    // Re-query the diffs container
    const updatedDiffsContainer = getDiffsContainer(testPage);
    await expect(updatedDiffsContainer).toBeVisible({ timeout: 15_000 });

    // The diff should now show MODIFIED_CONTENT instead of INITIAL_CONTENT
    // Note: exact match is false because the line includes prefix text
    // Give extra time for git polling to detect and refresh the diff view
    await expect(updatedDiffsContainer.getByText("MODIFIED_CONTENT")).toBeVisible({
      timeout: 45_000,
    });

    // Verify INITIAL_CONTENT is no longer shown
    await expect(updatedDiffsContainer.getByText("INITIAL_CONTENT")).toHaveCount(0);

    // Also verify the new line was added
    await expect(updatedDiffsContainer.getByText("NEW_LINE")).toBeVisible({
      timeout: 15_000,
    });
  });
});
