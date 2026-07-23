import type { Page, Route } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";
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
  expect(box.y).toBeGreaterThanOrEqual(0);
  expect(box.width).toBeLessThanOrEqual(viewport.width + 1);
  expect(box.height).toBeLessThanOrEqual(viewport.height + 1);
}

test.describe("Mobile task loading state", () => {
  test("shows an inner loading state instead of a blank task detail pane", async ({
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

  test("shows route loading, then recovers the destination task when optional hydration fails", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const title = "Mobile route hydration destination";
    const responseText = "route hydration destination response";
    const destination = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      title,
      seedData.agentProfileId,
      {
        description: `e2e:message("${responseText}")`,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!destination.session_id) throw new Error("route hydration destination has no session");

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    let requestStarted = false;
    let releaseRequest: () => void = () => {};
    let markHandlerSettled: () => void = () => {};
    const requestReleased = new Promise<void>((resolve) => {
      releaseRequest = resolve;
    });
    const handlerSettled = new Promise<void>((resolve) => {
      markHandlerSettled = resolve;
    });
    let markRequestStarted: () => void = () => {};
    const requestObserved = new Promise<void>((resolve) => {
      markRequestStarted = resolve;
    });
    const sessionPattern = `**/api/v1/task-sessions/${destination.session_id}`;
    const sessionRoute = async (route: Route) => {
      requestStarted = true;
      markRequestStarted();
      await requestReleased;
      try {
        await route.fulfill({ status: 503, contentType: "application/json", body: "{}" });
      } finally {
        markHandlerSettled();
      }
    };

    await testPage.route(sessionPattern, sessionRoute);
    try {
      await mobile.taskCard(destination.id).tap();
      await expect(testPage).toHaveURL(new RegExp(`/t/${destination.id}$`));
      await requestObserved;
      const routeLoading = testPage.getByRole("status").filter({ hasText: "Loading task…" });
      await expect(routeLoading).toBeVisible();

      releaseRequest();
      await handlerSettled;
      await expect(routeLoading).toBeHidden();
      const session = new SessionPage(testPage);
      const mobileTopBar = testPage
        .getByTestId("mobile-session-menu")
        .locator("xpath=ancestor::header");
      await expect(mobileTopBar.getByText(title, { exact: true })).toBeVisible();
      await expect(session.activeChat().getByText(responseText).last()).toBeVisible({
        timeout: 30_000,
      });
    } finally {
      releaseRequest();
      if (requestStarted) await handlerSettled;
      await testPage.unroute(sessionPattern, sessionRoute);
    }
  });
});
