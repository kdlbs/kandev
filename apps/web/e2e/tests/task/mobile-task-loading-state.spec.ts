import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { openBlockedTaskLoadingState } from "./task-loading-state-helpers";

async function expectLoadingStateFitsViewport(testPage: Page) {
  const loadingState = testPage.getByTestId("task-loading-state");
  const box = await loadingState.boundingBox();
  const viewport = testPage.viewportSize();

  expect(box).not.toBeNull();
  expect(viewport).not.toBeNull();
  if (!box || !viewport) return;

  await expect(loadingState).toBeInViewport();
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.width).toBeLessThanOrEqual(viewport.width + 1);
}

test.describe("Mobile task loading state", () => {
  test("shows a spinner instead of a blank task detail pane while task data loads", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const unblockTaskDetailRequest = await openBlockedTaskLoadingState({
      testPage,
      apiClient,
      seedData,
      title: "Mobile Task Loading State Anchor",
      unresolvedTaskId: "unresolved-task-detail-mobile",
    });

    try {
      await expect(testPage.getByTestId("task-loading-state")).toBeVisible({ timeout: 10_000 });
      await expect(testPage.getByText("Loading task...")).toBeVisible();
      await expectLoadingStateFitsViewport(testPage);
    } finally {
      await unblockTaskDetailRequest();
    }
  });
});
