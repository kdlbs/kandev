import { test, expect } from "../../fixtures/test-base";
import { openBlockedTaskLoadingState } from "./task-loading-state-helpers";

test.describe("Task loading state", () => {
  test("shows a spinner instead of a blank task detail pane while task data loads", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const unblockTaskDetailRequest = await openBlockedTaskLoadingState({
      testPage,
      apiClient,
      seedData,
      title: "Task Loading State Anchor",
      unresolvedTaskId: "unresolved-task-detail-desktop",
    });

    try {
      await expect(testPage.getByTestId("task-loading-state")).toBeVisible({ timeout: 10_000 });
      await expect(testPage.getByText("Loading task...")).toBeVisible();
    } finally {
      await unblockTaskDetailRequest();
    }
  });
});
