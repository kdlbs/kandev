import { test, expect } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";
import type { ApiClient } from "../helpers/api-client";
import type { SeedData } from "../fixtures/test-base";
import type { Page } from "@playwright/test";

/**
 * Set the diff-viewer provider in localStorage before any navigation.
 * Must be called before goto() so the zustand persist store picks it up.
 */
async function setDiffViewerProvider(testPage: Page, provider: "monaco" | "pierre-diffs") {
  await testPage.addInitScript(
    (prov: string) => {
      const store = {
        state: {
          providers: {
            "code-editor": "monaco",
            "diff-viewer": prov,
            "chat-code-block": "shiki",
            "chat-diff": "pierre-diffs",
            "plan-editor": "tiptap",
          },
        },
        version: 3,
      };
      localStorage.setItem("kandev-editor-providers", JSON.stringify(store));
    },
    provider,
  );
}

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

  await testPage.goto(`/s/${task.session_id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();

  await expect(
    session.chat.getByText("diff-expansion-setup complete", { exact: false }),
  ).toBeVisible({ timeout: 30_000 });

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
  test.describe.configure({ retries: 1 });

  test.beforeEach(async ({ testPage }) => {
    await setDiffViewerProvider(testPage, "pierre-diffs");
  });

  test("renders Pierre Diffs viewer and shows both hunks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);

    // Pierre Diffs renders a diffs-container custom element
    await expect(testPage.locator("diffs-container")).toBeVisible({ timeout: 15_000 });

    await expect(testPage.getByText("HUNK_TOP", { exact: false })).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText("HUNK_BOTTOM", { exact: false })).toBeVisible({
      timeout: 5_000,
    });
  });

  test("shows expand controls between hunks", async ({ testPage, apiClient, seedData }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    await expect(testPage.getByText("HUNK_TOP", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // Pierre Diffs renders expand buttons inside shadow DOM
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

  test("clicking expand reveals the collapsed middle lines", async ({
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

    // Line 25 sits in the middle of the collapsed gap (lines 6–45).
    await expect(testPage.getByText("original_25", { exact: false })).toBeVisible({
      timeout: 10_000,
    });
  });
});

test.describe("Diff expansion — Monaco provider", () => {
  test.describe.configure({ retries: 1 });

  test.beforeEach(async ({ testPage }) => {
    await setDiffViewerProvider(testPage, "monaco");
  });

  test("renders Monaco diff viewer and shows both hunks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    // Monaco renders with the .monaco-diff-viewer wrapper
    await expect(testPage.locator(".monaco-diff-viewer")).toBeVisible({ timeout: 15_000 });
    // Should NOT have a Pierre Diffs container
    await expect(testPage.locator("diffs-container")).toHaveCount(0);

    await expect(testPage.getByText("HUNK_TOP", { exact: false })).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText("HUNK_BOTTOM", { exact: false })).toBeVisible({
      timeout: 5_000,
    });
  });

  test("shows hidden unchanged region widget between hunks", async ({
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

    // Monaco's hideUnchangedRegions renders a .diff-hidden-lines-widget
    // with a "Show Unchanged Region" link in the center.
    const hiddenWidget = testPage.locator(".diff-hidden-lines-widget").first();
    await expect(hiddenWidget).toBeVisible({ timeout: 20_000 });
  });

  test("clicking the top reveal area expands collapsed lines", async ({
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

    // Click "Show Unchanged Region" to reveal all hidden lines
    const showRegionLink = testPage
      .locator('.diff-hidden-lines a[title="Show Unchanged Region"]')
      .first();
    await expect(showRegionLink).toBeVisible({ timeout: 20_000 });
    await showRegionLink.click();

    // Line 25 from the middle of the file should now be visible
    await expect(testPage.getByText("original_25", { exact: false })).toBeVisible({
      timeout: 10_000,
    });
  });
});
