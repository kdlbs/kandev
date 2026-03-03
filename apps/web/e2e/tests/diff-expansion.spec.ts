import { test, expect } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";
import type { ApiClient } from "../helpers/api-client";
import type { SeedData } from "../fixtures/test-base";
import type { Page } from "@playwright/test";

/**
 * Seed a task using the diff-expansion-setup mock scenario and navigate to
 * its session page, waiting for the agent turn to complete.
 *
 * The scenario writes a 50-line file, commits it, then modifies two lines far
 * apart (line 3 and line 48).  The diff viewer will show two separate hunks
 * with ~44 collapsed lines between them — that gap is what the expand button
 * reveals, which is the core of the feature under test.
 */
async function seedExpansionTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Diff Expansion E2E",
    seedData.agentProfileId,
    {
      description: "/e2e:diff-expansion-setup",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/s/${task.session_id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();

  // Wait for the mock agent to finish — it emits this text on success.
  await expect(
    session.chat.getByText("diff-expansion-setup complete", { exact: false }),
  ).toBeVisible({ timeout: 30_000 });

  return session;
}

/** Click the Changes dockview tab and return a locator scoped to its content. */
async function openChangesTab(testPage: Page) {
  const changesTab = testPage.locator(".dv-default-tab", { hasText: "Changes" });
  await expect(changesTab).toBeVisible({ timeout: 10_000 });
  await changesTab.click();
}

/** Click the file row for expansion_test.go to open its diff view. */
async function openExpansionFileDiff(testPage: Page) {
  // The file name appears in the changes timeline as a clickable row.
  const fileRow = testPage
    .locator("button, [role='button'], [class*='file']")
    .filter({ hasText: "expansion_test.go" })
    .first();
  await expect(fileRow).toBeVisible({ timeout: 10_000 });
  await fileRow.click();
}

test.describe("Diff expansion in code review", () => {
  test.describe.configure({ retries: 1 });

  test("Changes tab shows two-hunk uncommitted diff", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);

    // Both modified markers should appear in the diff output.
    await expect(
      testPage.getByText("HUNK_TOP", { exact: false }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(
      testPage.getByText("HUNK_BOTTOM", { exact: false }),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("diff viewer loads expansion content and shows expand controls between hunks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    // Both hunk markers confirm the diff rendered.
    await expect(testPage.getByText("HUNK_TOP", { exact: false })).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByText("HUNK_BOTTOM", { exact: false })).toBeVisible({
      timeout: 5_000,
    });

    // The ~44 lines between the hunks are collapsed. The @pierre/diffs library
    // renders an expand button in that gap once useExpandableDiff has fetched
    // the full file content from the backend (via workspace.file.get_at_ref).
    // We wait for it — its appearance confirms the WebSocket round-trip worked.
    const expandBtn = testPage
      .locator(
        [
          'button[aria-label*="expand" i]',
          'button[title*="expand" i]',
          '[class*="expand-btn"]',
          '[class*="expandBtn"]',
          '[data-testid*="expand"]',
        ].join(", "),
      )
      .first();
    await expect(expandBtn).toBeVisible({ timeout: 20_000 });
  });

  test("clicking expand between hunks reveals the collapsed middle lines", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    // Wait for both hunks to render.
    await expect(testPage.getByText("HUNK_TOP", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // Wait for the expand button between the two hunks.
    const expandBtn = testPage
      .locator(
        [
          'button[aria-label*="expand" i]',
          'button[title*="expand" i]',
          '[class*="expand-btn"]',
          '[class*="expandBtn"]',
          '[data-testid*="expand"]',
        ].join(", "),
      )
      .first();
    await expect(expandBtn).toBeVisible({ timeout: 20_000 });
    await expandBtn.click();

    // After expanding, a previously collapsed line from the middle of the file
    // should appear.  Line 25 sits right in the middle of the gap (lines 6–45)
    // and uses the original content written by the mock scenario.
    await expect(
      testPage.getByText("original_25", { exact: false }),
    ).toBeVisible({ timeout: 10_000 });
  });
});
