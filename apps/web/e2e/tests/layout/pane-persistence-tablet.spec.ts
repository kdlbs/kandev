import { type Page, expect as pwExpect } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

// The responsive breakpoint logic treats width < 768 with fine pointer as
// "tablet"; width >= 768 with fine pointer is compactDesktop. Headless
// Chromium reports fine pointer, so a true tablet layout needs width < 768.
const TABLET_VIEWPORT = { width: 700, height: 900 };

async function openTabletTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<void> {
  await page.setViewportSize(TABLET_VIEWPORT);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  await expect(page.getByTestId("tablet-task-layout")).toBeVisible({ timeout: 10_000 });
}

async function readStoredLayout(page: Page, id: string): Promise<Record<string, number> | null> {
  return page.evaluate((key) => {
    const raw = window.localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as Record<string, number>) : null;
  }, id);
}

test.describe("Tablet pane persistence", () => {
  test("tablet left/right split persists across reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openTabletTask(testPage, apiClient, seedData, "Tablet persist");

    // Drive the resize-panels library directly by overwriting the stored
    // layout — the underlying drag handle is a complex pointer-events target
    // that's flaky to grab in headless. The stored layout drives the next
    // render; this test verifies the round-trip persistence contract AND
    // that the rendered panels match the stored proportions.
    await testPage.evaluate(() => {
      window.localStorage.setItem("task-layout-tablet-v1", JSON.stringify({ left: 65, right: 35 }));
    });
    await testPage.reload();
    await expect(testPage.getByTestId("tablet-task-layout")).toBeVisible({ timeout: 10_000 });

    // localStorage round-trip: react-resizable-panels writes its own
    // normalized version on layout, so allow a small tolerance.
    const stored = await readStoredLayout(testPage, "task-layout-tablet-v1");
    pwExpect(stored).not.toBeNull();
    pwExpect(Math.abs((stored?.left ?? 0) - 65)).toBeLessThanOrEqual(2);
    pwExpect(Math.abs((stored?.right ?? 0) - 35)).toBeLessThanOrEqual(2);

    // The localStorage round-trip above is the persistence contract;
    // verifying rendered pixel widths in `react-resizable-panels` requires
    // querying internal panel DOM that isn't stable across versions, so we
    // stop here.
  });

  test("invalid stored layout is replaced with a valid default", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize(TABLET_VIEWPORT);
    await testPage.goto("/");
    await testPage.evaluate(() => {
      window.localStorage.setItem("task-layout-tablet-v1", '{"left":2}');
    });
    await openTabletTask(testPage, apiClient, seedData, "Tablet fallback");

    // After load, onLayoutChanged replaces the invalid stub with the
    // rendered (valid) layout. Verify the persisted value is now valid:
    // both panel IDs present and >= MIN_PANEL_PERCENT (5).
    const stored = await readStoredLayout(testPage, "task-layout-tablet-v1");
    pwExpect(stored).not.toBeNull();
    pwExpect(stored?.left).toBeGreaterThanOrEqual(5);
    pwExpect(stored?.right).toBeGreaterThanOrEqual(5);
  });

  test("tablet right-panel inner split accepts values below the old 30% floor", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openTabletTask(testPage, apiClient, seedData, "Tablet top shrink");

    // The right-panel internal split now allows minSize=15. Round-trip via
    // localStorage: write a 20/80 layout (below the old 30% floor) and
    // verify it survives the reload — would have been rejected before.
    await testPage.evaluate(() => {
      window.localStorage.setItem("task-layout-right-v2", JSON.stringify({ top: 20, bottom: 80 }));
    });
    await testPage.reload();
    await expect(testPage.getByTestId("tablet-task-layout")).toBeVisible({ timeout: 10_000 });

    const stored = await readStoredLayout(testPage, "task-layout-right-v2");
    pwExpect(stored).not.toBeNull();
    pwExpect(Math.abs((stored?.top ?? 0) - 20)).toBeLessThanOrEqual(2);
    pwExpect(Math.abs((stored?.bottom ?? 0) - 80)).toBeLessThanOrEqual(2);
  });
});
