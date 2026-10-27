import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { openPhoneList, waitForSidebarQuery } from "./mobile-list-helpers";
import {
  readPreviousSidebarViewState,
  restoreSidebarViewState,
} from "./sidebar-running-first-activity-sort-helpers";

test("temporary phone filters apply and clear without changing the selected saved view", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const otherWorkflow = await apiClient.createWorkflow(
    seedData.workspaceId,
    "Phone other workflow",
    "simple",
  );
  const { steps: otherSteps } = await apiClient.listWorkflowSteps(otherWorkflow.id);
  const primary = await apiClient.createTask(seedData.workspaceId, "Phone filter primary", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const other = await apiClient.createTask(seedData.workspaceId, "Phone filter other", {
    workflow_id: otherWorkflow.id,
    workflow_step_id: otherSteps[0].id,
  });
  const savedView = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);

  try {
    await openPhoneList(testPage);
    const list = testPage.getByTestId("mobile-tasks-list-scroll");
    const primaryRow = list.getByTestId("sidebar-task-item").filter({ hasText: primary.title });
    const otherRow = list.getByTestId("sidebar-task-item").filter({ hasText: other.title });
    await expect(primaryRow).toBeVisible();
    await expect(otherRow).toBeVisible();
    await testPage.getByTestId("mobile-list-search-toggle").tap();

    const trigger = testPage.getByTestId("mobile-list-result-filters");
    expect((await trigger.boundingBox())!.height).toBeGreaterThanOrEqual(40);
    await trigger.tap();
    const drawer = testPage.getByTestId("mobile-list-result-filters-drawer");
    await expect(drawer).toBeVisible();
    await waitForFiniteAnimations(drawer);
    const scrollBody = drawer.locator(".overflow-y-auto");
    await expect(scrollBody).toHaveCount(1);
    await expect(scrollBody).toHaveCSS("overscroll-behavior-y", "contain");
    const drawerBox = (await drawer.boundingBox())!;
    expect(drawerBox.x).toBeGreaterThanOrEqual(0);
    expect(drawerBox.x + drawerBox.width).toBeLessThanOrEqual(testPage.viewportSize()!.width + 1);
    expect(drawerBox.y + drawerBox.height).toBeLessThanOrEqual(testPage.viewportSize()!.height + 1);

    const workflows = (await apiClient.listWorkflows(seedData.workspaceId)).workflows;
    const workflowName = workflows.find((workflow) => workflow.id === seedData.workflowId)!.name;
    const workflowFilter = drawer.getByTestId("mobile-list-filter-workflow");
    expect((await workflowFilter.boundingBox())!.height).toBeGreaterThanOrEqual(40);
    await workflowFilter.tap();
    await testPage.getByRole("option", { name: workflowName, exact: true }).tap();
    await testPage.keyboard.press("Escape");
    await expect(drawer).toBeHidden();
    await expect(primaryRow).toBeVisible();
    await expect(otherRow).toBeVisible();
    await expect(testPage.getByTestId("mobile-list-result-summary")).toHaveCount(0);

    await trigger.tap();
    const reopened = testPage.getByTestId("mobile-list-result-filters-drawer");
    await reopened.getByTestId("mobile-list-filter-workflow").tap();
    await testPage.getByRole("option", { name: workflowName, exact: true }).tap();
    await reopened.getByTestId("mobile-list-filters-apply").tap();
    await expect(reopened).toBeHidden();
    await expect(trigger).toBeFocused();
    await expect(primaryRow).toBeVisible();
    await expect(otherRow).toHaveCount(0);
    await expect(testPage.getByTestId("mobile-list-temporary-filter-summary")).toContainText(
      workflowName,
    );
    expect(await readPreviousSidebarViewState(apiClient, seedData.workspaceId)).toEqual(savedView);

    await trigger.tap();
    const clearDrawer = testPage.getByTestId("mobile-list-result-filters-drawer");
    await clearDrawer.getByTestId("mobile-list-filters-clear").tap();
    await clearDrawer.getByTestId("mobile-list-filters-apply").tap();
    await expect(primaryRow).toBeVisible();
    await expect(otherRow).toBeVisible();
    await expect(testPage.getByTestId("mobile-list-result-summary")).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage);
  } finally {
    await restoreSidebarViewState(apiClient, seedData.workspaceId, savedView);
  }
});

test("empty phone results can clear temporary refinements without changing the saved view", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Phone empty-state recovery task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const savedView = await readPreviousSidebarViewState(apiClient, seedData.workspaceId);

  try {
    await openPhoneList(testPage);
    await testPage.getByTestId("mobile-list-search-toggle").tap();
    const input = testPage.getByTestId("mobile-list-search-input");
    const searched = waitForSidebarQuery(
      testPage,
      (query) => query.query === "no matching phone task",
    );
    await input.fill("no matching phone task");
    await searched;

    const list = testPage.getByTestId("mobile-tasks-list-scroll");
    await expect(list.getByText("No tasks match these filters", { exact: true })).toBeVisible();
    await list.getByRole("button", { name: "Clear filters", exact: true }).tap();
    await expect(input).toHaveValue("");
    await expect(
      list.getByTestId("sidebar-task-item").filter({ hasText: task.title }),
    ).toBeVisible();
    expect(await readPreviousSidebarViewState(apiClient, seedData.workspaceId)).toEqual(savedView);
  } finally {
    await restoreSidebarViewState(apiClient, seedData.workspaceId, savedView);
  }
});

test("failed archive link reads show an error and retry without losing the link scope", async ({
  testPage,
  seedData,
}) => {
  await openPhoneList(testPage);
  let failed = true;
  await testPage.route(/\/api\/v1\/workspaces\/[^/]+\/sidebar\/query(?:\?|$)/, async (route) => {
    if (failed) {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "Temporarily unavailable" }),
      });
    } else {
      await route.continue();
    }
  });
  await testPage.goto(
    `/tasks?workspace=${seedData.workspaceId}&archive=archived&workflow=all&repository=all&query=No+matching+task`,
  );
  const searchToggle = testPage.getByTestId("mobile-list-search-toggle");
  if ((await searchToggle.getAttribute("aria-expanded")) !== "true") await searchToggle.tap();
  await expect(testPage.getByTestId("mobile-list-link-scope")).toBeVisible();
  await expect(testPage.getByTestId("sidebar-task-page-load-error")).toBeVisible();

  failed = false;
  const retried = waitForSidebarQuery(testPage, (query) =>
    query.filters.some((filter) => filter.dimension === "archived" && filter.value === true),
  );
  await testPage.getByRole("button", { name: "Retry", exact: true }).tap({ force: true });
  await retried;
  await expect(testPage.getByTestId("sidebar-task-page-load-error")).toHaveCount(0);
  await expect(testPage.getByTestId("mobile-list-link-scope")).toBeVisible();
  await expect(
    testPage.getByTestId("mobile-tasks-list-scroll").getByTestId("sidebar-task-item"),
  ).toHaveCount(0);
});
