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
  test.describe.configure({ retries: 1, timeout: 120_000 });

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
    await openExpansionFileDiff(testPage);

    // Pierre Diffs renders a diffs-container custom element
    await expect(testPage.locator("diffs-container")).toBeVisible({ timeout: 15_000 });

    await expect(testPage.getByText("HUNK_TOP", { exact: false })).toBeVisible({ timeout: 15_000 });
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
});

test.describe("Diff expansion — Monaco provider", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

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
});
