import { exerciseSidebarViewReuse } from "./sidebar-view-reuse-fixtures";
import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import { SidebarTasksPage } from "../../pages/sidebar-tasks-page";
import {
  archiveSidebarPaginationTasks,
  seedSidebarPaginationTasks,
} from "./sidebar-task-pagination-fixtures";

test("desktop sidebar pages 101 matching tasks without changing the open conversation", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const token = `Sidebar page ${Date.now()}`;
  const matchingTaskIds = await seedSidebarPaginationTasks(apiClient, seedData, token);
  const current = await apiClient.createTask(
    seedData.workspaceId,
    "Open desktop sidebar conversation",
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    },
  );
  const { session_id: sessionId } = await apiClient.seedTaskSession(current.id, {
    state: "COMPLETED",
    agentProfileId: seedData.agentProfileId,
    completedAt: new Date().toISOString(),
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: "sidebar page conversation remains",
  });
  await apiClient.updateTaskState(current.id, "COMPLETED");
  await apiClient.archiveTask(current.id);

  await testPage.goto(`/t/${current.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(
    session.activeChat().getByText("sidebar page conversation remains").last(),
  ).toBeVisible({
    timeout: 30_000,
  });

  const rows = session.sidebar.locator("[data-task-row-id]");
  const controls = session.sidebar.getByTestId("sidebar-page-controls");
  await expect(rows).toHaveCount(100, { timeout: 20_000 });
  await expect(controls).toBeVisible();

  await apiClient.archiveTask(matchingTaskIds[0]);
  await expect(rows).toHaveCount(100, { timeout: 20_000 });
  await expect(controls).toBeHidden();
  await apiClient.archiveTask(matchingTaskIds[1]);
  await expect(rows).toHaveCount(99, { timeout: 20_000 });
  await expect(controls).toBeHidden();
  await apiClient.unarchiveTask(matchingTaskIds[1]);
  await expect(rows).toHaveCount(100, { timeout: 20_000 });
  await expect(controls).toBeHidden();
  await apiClient.unarchiveTask(matchingTaskIds[0]);
  await expect(rows).toHaveCount(100, { timeout: 20_000 });
  await expect(controls).toBeVisible();

  const filters = new SidebarFilterPopoverPage(testPage);
  await filters.addFilterRow();
  await filters.setClauseDimension(0, "Title");
  await filters.setClauseTextValue(0, `${token} match row`);
  await filters.close();

  await expect(rows).toHaveCount(99, { timeout: 20_000 });
  await expect(controls).toBeHidden();

  await filters.open();
  await filters.setClauseTextValue(0, `${token} match`);
  await filters.close();
  await expect(rows).toHaveCount(100, { timeout: 20_000 });
  await expect(controls).toBeHidden();

  await filters.open();
  await filters.setClauseTextValue(0, token);
  await filters.close();
  await expect(rows).toHaveCount(100, { timeout: 20_000 });
  await expect(controls).toBeVisible();
  await expect(controls.getByText("Page 1 of 2")).toBeVisible();

  const documentRequests: string[] = [];
  testPage.on("request", (request) => {
    if (request.isNavigationRequest() && request.frame() === testPage.mainFrame()) {
      documentRequests.push(request.url());
    }
  });
  const documentRequestCount = documentRequests.length;
  const scrollArea = session.sidebar.getByTestId("task-sidebar-scroll");
  await scrollArea.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  await controls.getByRole("button").last().click();
  await expect(controls.getByText("Page 2 of 2")).toBeVisible();
  await expect(rows).toHaveCount(1);
  await expect.poll(() => scrollArea.evaluate((element) => element.scrollTop)).toBe(0);

  await expect(testPage).toHaveURL(new RegExp(`/t/${current.id}$`));
  await expect(
    session.activeChat().getByText("sidebar page conversation remains").last(),
  ).toBeVisible();
  await expect(session.sessionTabBySessionId(sessionId)).toBeVisible();
  expect(documentRequests).toHaveLength(documentRequestCount);

  await controls.getByRole("button").first().click();
  await expect(controls.getByText("Page 1 of 2")).toBeVisible();
  await filters.open();
  await filters.saveAs("Sidebar pagination active");
  await filters.close();
  await filters.expectActiveViewChip("Sidebar pagination active");
  await expect(rows).toHaveCount(100);
  await expect(controls.getByText("Page 1 of 2")).toBeVisible();

  await filters.open();
  await filters.setClauseTextValue(0, `${token} match row`);
  await filters.close();
  await expect(rows).toHaveCount(99, { timeout: 20_000 });
  await expect(controls).toBeHidden();
  await filters.open();
  await filters.saveOverwrite();
  await filters.close();
  await filters.open();
  await filters.setClauseTextValue(0, `${token} match`);
  await filters.close();
  await expect(rows).toHaveCount(100, { timeout: 20_000 });
  await expect(controls).toBeHidden();
  await filters.open();
  await filters.saveOverwrite();
  await filters.close();
  await filters.open();
  await filters.setClauseTextValue(0, token);
  await filters.close();
  await expect(rows).toHaveCount(100, { timeout: 20_000 });
  await expect(controls.getByText("Page 1 of 2")).toBeVisible();
  await filters.open();
  await filters.saveOverwrite();
  await filters.close();

  await archiveSidebarPaginationTasks(apiClient, matchingTaskIds);
  await expect(rows).toHaveCount(0, { timeout: 20_000 });
  await expect(controls).toBeHidden();
  await filters.addFilterRow();
  await filters.setClauseDimension(1, "Archived");
  await filters.setClauseBooleanValue(1, true);
  await filters.close();
  await expect(rows).toHaveCount(100, { timeout: 20_000 });
  await expect(controls).toBeVisible();
  await expect(controls.getByText("Page 1 of 2")).toBeVisible();
  await filters.open();
  await filters.saveAs("Sidebar pagination archived");
  await filters.close();
  await filters.expectActiveViewChip("Sidebar pagination archived");
  await expect(rows).toHaveCount(100);
  await expect(controls.getByText("Page 1 of 2")).toBeVisible();

  await filters.open();
  await filters.setClauseTextValue(0, `${token} match row`);
  await filters.close();
  await expect(rows).toHaveCount(99, { timeout: 20_000 });
  await expect(controls).toBeHidden();
  await filters.open();
  await filters.saveOverwrite();
  await filters.close();
  await filters.open();
  await filters.setClauseTextValue(0, `${token} match`);
  await filters.close();
  await expect(rows).toHaveCount(100, { timeout: 20_000 });
  await expect(controls).toBeHidden();
  await filters.open();
  await filters.saveOverwrite();
  await filters.close();
  await filters.open();
  await filters.setClauseTextValue(0, token);
  await filters.close();
  await expect(rows).toHaveCount(100, { timeout: 20_000 });
  await expect(controls.getByText("Page 1 of 2")).toBeVisible();
  await filters.open();
  await filters.saveOverwrite();
  await filters.close();

  await expect(testPage).toHaveURL(new RegExp(`/t/${current.id}$`));
  await expect(
    session.activeChat().getByText("sidebar page conversation remains").last(),
  ).toBeVisible();
});

test("desktop keeps multi-selection across pages and bulk archives rows from both pages", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const token = `Sidebar selection page ${Date.now()}`;
  await seedSidebarPaginationTasks(apiClient, seedData, token);
  const current = await apiClient.createTask(seedData.workspaceId, "Open selection conversation", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(current.id, {
    state: "COMPLETED",
    agentProfileId: seedData.agentProfileId,
    completedAt: new Date().toISOString(),
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: "selection paging keeps the open conversation",
  });
  await apiClient.updateTaskState(current.id, "COMPLETED");
  await apiClient.archiveTask(current.id);

  await testPage.goto(`/t/${current.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(
    session.activeChat().getByText("selection paging keeps the open conversation").last(),
  ).toBeVisible({ timeout: 30_000 });

  const sidebar = new SidebarTasksPage(testPage);
  const controls = session.sidebar.getByTestId("sidebar-page-controls");
  const firstPageRow = sidebar.root.locator("[data-testid='sortable-task-block']").first();
  await expect(firstPageRow).toBeVisible({ timeout: 20_000 });
  const firstTaskId = await firstPageRow.getAttribute("data-task-id");
  expect(firstTaskId).toBeTruthy();
  await sidebar.cmdClick(firstTaskId!);
  await expect(testPage.getByText("1 task selected", { exact: true })).toBeVisible();
  await testPage.getByRole("button", { name: "Clear selection" }).click();
  await expect(testPage.getByText("1 task selected", { exact: true })).toHaveCount(0);

  await sidebar.cmdClick(firstTaskId!);
  await controls.getByRole("button").last().click();
  await expect(controls.getByText("Page 2 of 2")).toBeVisible();
  await expect(testPage.getByText("1 task selected", { exact: true })).toBeVisible();
  const secondPageRow = sidebar.root.locator("[data-testid='sortable-task-block']").first();
  const secondTaskId = await secondPageRow.getAttribute("data-task-id");
  expect(secondTaskId).toBeTruthy();
  await sidebar.cmdClick(secondTaskId!);
  await expect(testPage.getByText("2 tasks selected", { exact: true })).toBeVisible();

  await sidebar.rightClick(secondTaskId!);
  await sidebar.bulkArchiveMenuItem(2).click();
  const confirm = sidebar.bulkArchiveConfirm();
  await expect(confirm).toBeVisible();
  await confirm.click();
  await expect(testPage.getByText("2 tasks selected", { exact: true })).toHaveCount(0);
  await expect(controls).toBeHidden({ timeout: 20_000 });
  await expect(sidebar.rows()).toHaveCount(99);
  await expect(testPage).toHaveURL(new RegExp(`/t/${current.id}$`));
  await expect(session.sessionTabBySessionId(sessionId)).toBeVisible();
  await expect(
    session.activeChat().getByText("selection paging keeps the open conversation").last(),
  ).toBeVisible();
});

test("sidebar view reuse preserves rows and offers one recovery action", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await exerciseSidebarViewReuse(testPage, apiClient, seedData, false, prCapture);
});
