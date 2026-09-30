import { exerciseSidebarViewReuse } from "./sidebar-view-reuse-fixtures";
import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import {
  archiveSidebarPaginationTasks,
  seedSidebarPaginationTasks,
} from "./sidebar-task-pagination-fixtures";

test("phone sidebar pages 101 matching tasks and preserves the active chat", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const token = `Mobile sidebar page ${Date.now()}`;
  const matchingTaskIds = await seedSidebarPaginationTasks(apiClient, seedData, token);
  const current = await apiClient.createTask(
    seedData.workspaceId,
    "Open mobile sidebar conversation",
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
    content: "mobile sidebar page conversation remains",
  });
  await apiClient.updateTaskState(current.id, "COMPLETED");
  await apiClient.archiveTask(current.id);

  await testPage.goto(`/t/${current.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(
    session.activeChat().getByText("mobile sidebar page conversation remains").last(),
  ).toBeVisible({ timeout: 30_000 });

  const documentRequests: string[] = [];
  testPage.on("request", (request) => {
    if (request.isNavigationRequest() && request.frame() === testPage.mainFrame()) {
      documentRequests.push(request.url());
    }
  });
  const documentRequestCount = documentRequests.length;
  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  const sheet = testPage.getByRole("dialog", { name: "Tasks" });
  const filters = new SidebarFilterPopoverPage(testPage);
  const rows = sheet.locator("[data-task-row-id]");
  const controls = sheet.getByTestId("sidebar-page-controls");
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
  const scrollArea = sheet.getByTestId("mobile-task-switcher-list");
  await scrollArea.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  await controls.getByRole("button").last().tap();
  await expect(controls.getByText("Page 2 of 2")).toBeVisible();
  await expect(rows).toHaveCount(1);
  await expect.poll(() => scrollArea.evaluate((element) => element.scrollTop)).toBe(0);

  await controls.getByRole("button").first().tap();
  await expect(controls.getByText("Page 1 of 2")).toBeVisible();
  await filters.open();
  await filters.saveAs("Mobile sidebar pagination active");
  await filters.close();
  const viewChips = sheet.getByTestId("sidebar-view-chip-row");
  await expect(
    viewChips
      .getByTestId("sidebar-view-chip")
      .filter({ hasText: "Mobile sidebar pagination active" }),
  ).toHaveAttribute("data-active", "true");
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
  // These fixture mutations use the API directly, outside the app's task
  // action/realtime path. Changing the query makes the list request a fresh
  // server page before checking the resulting archive state.
  await filters.addFilterRow();
  await filters.setClauseDimension(1, "Archived");
  await filters.setClauseBooleanValue(1, false);
  await filters.close();
  await expect(rows).toHaveCount(0, { timeout: 20_000 });
  await expect(controls).toBeHidden();
  await filters.open();
  await filters.setClauseBooleanValue(1, true);
  await filters.close();
  await expect(rows).toHaveCount(100, { timeout: 20_000 });
  await expect(controls).toBeVisible();
  await expect(controls.getByText("Page 1 of 2")).toBeVisible();
  await filters.open();
  await filters.saveAs("Mobile sidebar pagination archived");
  await filters.close();
  await expect(
    viewChips
      .getByTestId("sidebar-view-chip")
      .filter({ hasText: "Mobile sidebar pagination archived" }),
  ).toHaveAttribute("data-active", "true");
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

  const widths = await testPage.evaluate(() => ({
    document: document.documentElement.scrollWidth,
    viewport: window.innerWidth,
  }));
  expect(widths.document).toBeLessThanOrEqual(widths.viewport);

  await expect(testPage).toHaveURL(new RegExp(`/t/${current.id}$`));
  await expect(
    session.activeChat().getByText("mobile sidebar page conversation remains").last(),
  ).toBeVisible();
  expect(documentRequests).toHaveLength(documentRequestCount);
  const nextBox = await controls.getByRole("button").last().boundingBox();
  expect(nextBox?.height).toBeGreaterThanOrEqual(44);
});

test("phone app navigation task list pages the shared sidebar view", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const token = `App navigation sidebar page ${Date.now()}`;
  await seedSidebarPaginationTasks(apiClient, seedData, token);
  const documentRequests: string[] = [];
  testPage.on("request", (request) => {
    if (request.isNavigationRequest() && request.frame() === testPage.mainFrame()) {
      documentRequests.push(request.url());
    }
  });

  await testPage.goto("/stats");
  const documentRequestCount = documentRequests.length;
  await testPage.getByTestId("app-nav-trigger").tap();
  const navigation = testPage.getByTestId("app-nav-sheet");
  const navigationToggle = navigation.getByTestId("mobile-navigation-tasks-toggle");
  if ((await navigationToggle.getAttribute("aria-expanded")) !== "true") {
    await navigationToggle.tap();
  }
  await expect(navigationToggle).toHaveAttribute("aria-expanded", "true");
  const navigationRows = navigation.locator("[data-task-row-id]");
  const navigationControls = navigation.getByTestId("sidebar-page-controls");
  await expect(navigationRows).toHaveCount(100, { timeout: 20_000 });
  await expect(navigationControls.getByText("Page 1 of 2")).toBeVisible();
  await expect(testPage).toHaveURL(/\/stats$/);
  const nextPage = navigationControls.getByRole("button", { name: "Next" });
  await expect(nextPage).toBeEnabled();
  await nextPage.click();
  await expect(navigation).toBeVisible();
  await expect(navigationControls.getByText("Page 2 of 2")).toBeVisible();
  await expect(navigationRows).toHaveCount(1);
  expect(documentRequests).toHaveLength(documentRequestCount);
  const nextBox = await navigationControls.getByRole("button").last().boundingBox();
  expect(nextBox?.height).toBeGreaterThanOrEqual(44);
  const widths = await testPage.evaluate(() => ({
    document: document.documentElement.scrollWidth,
    viewport: window.innerWidth,
  }));
  expect(widths.document).toBeLessThanOrEqual(widths.viewport);
});

test("sidebar view reuse preserves rows and offers one recovery action", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await exerciseSidebarViewReuse(testPage, apiClient, seedData, true, prCapture);
});
