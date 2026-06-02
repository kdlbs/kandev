import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { REVIEW_SIDEBAR_LIMITS } from "../../../hooks/use-review-sidebar-resize";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { Page, Locator } from "@playwright/test";

const STORAGE_KEY = REVIEW_SIDEBAR_LIMITS.storageKey;

async function seedReviewTask(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Review Sidebar Resize E2E",
    seedData.agentProfileId,
    {
      description: "/e2e:review-cumulative-setup",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(
    session.chat.getByText("review-cumulative-setup complete", { exact: false }),
  ).toBeVisible({ timeout: 45_000 });
  return task;
}

async function openDialogWithChanges(testPage: Page) {
  const changesTab = testPage.locator(".dv-default-tab", { hasText: "Changes" });
  await expect(changesTab).toBeVisible({ timeout: 10_000 });
  await changesTab.click();
  await expect(testPage.getByTestId("file-row-review_cumulative_test.txt")).toBeVisible({
    timeout: 15_000,
  });
  await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
  const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  return dialog;
}

async function getSidebarWidth(sidebar: Locator): Promise<number> {
  const box = await sidebar.boundingBox();
  if (!box) throw new Error("sidebar has no bounding box");
  return Math.round(box.width);
}

// boundingBox returns floats and sub-pixel rounding can shift the measured
// width by a pixel across renderers, so compare with a small tolerance.
function expectWidthNear(actual: number, expected: number, tolerance = 1) {
  expect(actual).toBeGreaterThanOrEqual(expected - tolerance);
  expect(actual).toBeLessThanOrEqual(expected + tolerance);
}

async function dragHandle(testPage: Page, handle: Locator, deltaX: number) {
  const box = await handle.boundingBox();
  if (!box) throw new Error("resize handle has no bounding box");
  const cx = box.x + box.width / 2;
  const cy = box.y + box.height / 2;
  await testPage.mouse.move(cx, cy);
  await testPage.mouse.down();
  await testPage.mouse.move(cx + deltaX, cy, { steps: 10 });
  await testPage.mouse.up();
}

test.describe("Review dialog sidebar resize", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("dragging the handle resizes the sidebar and persists to sessionStorage", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedReviewTask(testPage, apiClient, seedData);
    const dialog = await openDialogWithChanges(testPage);

    const sidebar = dialog.getByTestId("review-dialog-sidebar");
    const handle = dialog.getByTestId("review-dialog-sidebar-resize");
    await expect(sidebar).toBeVisible();
    await expect(handle).toBeVisible();

    const before = await getSidebarWidth(sidebar);
    expectWidthNear(before, 220); // default width

    await dragHandle(testPage, handle, 120); // drag right by 120px

    await expect
      .poll(async () => getSidebarWidth(sidebar), { timeout: 5_000 })
      .toBeGreaterThan(before + 80);
    const after = await getSidebarWidth(sidebar);

    // sessionStorage persistence (within 1px of the rendered width)
    const stored = await testPage.evaluate(
      (key) => window.sessionStorage.getItem(key),
      STORAGE_KEY,
    );
    expect(stored).not.toBeNull();
    expectWidthNear(Number(stored), after);
  });

  test("clamps to max width when dragged far past the limit", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedReviewTask(testPage, apiClient, seedData);
    const dialog = await openDialogWithChanges(testPage);
    const sidebar = dialog.getByTestId("review-dialog-sidebar");
    const handle = dialog.getByTestId("review-dialog-sidebar-resize");

    await dragHandle(testPage, handle, 2000); // way past max=600

    // At default Playwright viewport (~1280px) the dialog is ~1024px, so the
    // (containerWidth - minDiffPaneWidth) clamp (~704) is wider than the
    // hard max (600); we should land on the hard max.
    await expect
      .poll(async () => getSidebarWidth(sidebar), { timeout: 5_000 })
      .toBeGreaterThanOrEqual(599);
    const finalWidth = await getSidebarWidth(sidebar);
    expectWidthNear(finalWidth, 600);
  });

  test("persisted width is reapplied when the dialog is reopened in the same tab", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedReviewTask(testPage, apiClient, seedData);

    // Pre-seed sessionStorage so the dialog mounts with the saved width.
    await testPage.evaluate(({ key, val }) => window.sessionStorage.setItem(key, val), {
      key: STORAGE_KEY,
      val: "340",
    });

    const dialog = await openDialogWithChanges(testPage);
    const sidebar = dialog.getByTestId("review-dialog-sidebar");
    await expect
      .poll(async () => getSidebarWidth(sidebar), { timeout: 5_000 })
      .toBeGreaterThanOrEqual(339);
    expectWidthNear(await getSidebarWidth(sidebar), 340);
  });
});
