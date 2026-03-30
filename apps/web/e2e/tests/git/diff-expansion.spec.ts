import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";

/**
 * Seed a task using the diff-expansion-setup mock scenario and navigate to
 * its session page, waiting for the agent turn to complete.
 *
 * The scenario writes a 50-line file, commits it, then modifies two lines far
 * apart (line 3 and line 48).  The diff viewer will show two separate hunks
 * with ~44 collapsed lines between them.
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

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();

  await expect(
    session.chat.getByText("diff-expansion-setup complete", { exact: false }),
  ).toBeVisible({ timeout: 45_000 });

  return session;
}

/** Click the Changes dockview tab. */
async function openChangesTab(testPage: Page) {
  const changesTab = testPage.locator(".dv-default-tab", { hasText: "Changes" });
  await expect(changesTab).toBeVisible({ timeout: 10_000 });
  await changesTab.click();
}

/** Click the file row for expansion_test.go to open its diff view. */
async function openExpansionFileDiff(testPage: Page) {
  const fileRow = testPage
    .locator("button, [role='button'], [class*='file']")
    .filter({ hasText: "expansion_test.go" })
    .first();
  await expect(fileRow).toBeVisible({ timeout: 10_000 });
  await fileRow.click();
}

test.describe("Diff expansion — Pierre Diffs provider", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("renders Pierre Diffs viewer and shows both hunks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    // Pierre Diffs renders a diffs-container custom element with an open shadow DOM.
    // Playwright's getByText auto-pierces shadow DOM and auto-retries.
    await expect(testPage.locator("diffs-container")).toBeVisible({ timeout: 15_000 });
    // On cold CI runners (first test in shard, no V8 code cache), resolveLanguagesAndExecuteTask
    // triggers createJavaScriptRegexEngine() which can take 30-40s to JIT-compile.
    // diffs-container mounts immediately but content appears only after the engine is ready.
    await expect(testPage.getByText("HUNK_TOP", { exact: false })).toBeVisible({ timeout: 60_000 });
    await expect(testPage.getByText("HUNK_BOTTOM", { exact: false })).toBeVisible({
      timeout: 5_000,
    });
  });

  test("shows expand separator with unmodified line count between hunks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    await expect(testPage.getByText("HUNK_TOP", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // Pierre Diffs renders a separator between hunks showing the hidden line count.
    // The separator contains img elements (chevron arrows) for expanding.
    const middleSeparator = testPage.getByText(/\d+ unmodified lines/).nth(1);
    await expect(middleSeparator).toBeVisible({ timeout: 20_000 });
  });

  test("clicking expand arrow reveals the collapsed middle lines", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    await expect(testPage.getByText("HUNK_TOP", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // Wait for expand buttons to appear in the shadow DOM. They load
    // asynchronously after full file content is fetched via WebSocket.
    await testPage.waitForFunction(
      () => {
        const container = document.querySelector("diffs-container");
        const shadow = container?.shadowRoot;
        if (!shadow) return false;
        return shadow.querySelectorAll("[data-expand-button]").length >= 3;
      },
      null,
      { timeout: 20_000 },
    );

    // Click the middle separator's expand-up button to reveal lines from the
    // top of the collapsed gap. The middle separator is the only one without
    // data-separator-first or data-separator-last attributes.
    await testPage.evaluate(() => {
      const container = document.querySelector("diffs-container");
      const shadow = container!.shadowRoot!;
      const sel =
        "[data-separator='line-info']:not([data-separator-first]):not([data-separator-last])";
      const btn = shadow.querySelector<HTMLElement>(`${sel} [data-expand-up]`);
      if (!btn) throw new Error("Middle separator expand-up button not found");
      btn.click();
    });

    // Line 60 is within the first 20 lines revealed by expanding from the top hunk.
    await expect(testPage.getByText("original_060", { exact: false })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("expand-all button reveals all collapsed lines at once", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    // Wait for both hunks and the separator to be present
    await expect(testPage.getByText("HUNK_TOP", { exact: false })).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByText(/\d+ unmodified lines/).first()).toBeVisible({
      timeout: 20_000,
    });

    // The "Expand all lines" button is in the Pierre Diffs header toolbar.
    // It contains a tabler-icon-fold-down SVG icon.
    const expandAllBtn = testPage.locator("button:has(svg.tabler-icon-fold-down)");
    await expect(expandAllBtn).toBeVisible({ timeout: 10_000 });
    await expandAllBtn.click();

    // After expanding, all original lines should be visible — pick a line
    // from the middle of the previously collapsed region.
    await expect(testPage.getByText("original_025", { exact: false })).toBeVisible({
      timeout: 10_000,
    });
    await expect(testPage.getByText("original_040", { exact: false })).toBeVisible({
      timeout: 5_000,
    });

    // The "unmodified lines" separators should be gone
    await expect(testPage.getByText(/\d+ unmodified lines/)).toHaveCount(0, {
      timeout: 5_000,
    });
  });
});
