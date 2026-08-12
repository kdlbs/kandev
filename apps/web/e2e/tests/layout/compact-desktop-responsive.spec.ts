import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const COMPACT_DESKTOP_VIEWPORT = { width: 900, height: 800 };

// Exercises the regular task-create dialog (New Task in the sidebar); run with office off.
useRegularMode();

test.describe("compact desktop responsive layout", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      app_status_bar_enabled: false,
    });
  });

  test("task page hands off from compact desktop to mobile below the sidebar boundary", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize(COMPACT_DESKTOP_VIEWPORT);
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      app_status_bar_enabled: true,
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Compact Desktop Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);

    const session = new SessionPage(testPage);
    await session.showSessionContext();
    await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible();
    await expect(testPage.getByTestId("tablet-task-layout")).toHaveCount(0);
    await expect(session.sidebar).toBeVisible();
    await expect(session.activeChat()).toBeVisible();
    await expect(testPage.getByTestId("dockview-add-panel-btn")).toBeVisible();

    const tabs = testPage.locator(".dv-tab");
    await expect(
      tabs.filter({ has: testPage.locator('[data-testid^="session-tab-"]') }),
    ).toBeVisible();
    await expect(tabs.filter({ hasText: "Files" })).toBeVisible();
    await expect(tabs.filter({ hasText: "Changes" })).toBeVisible();
    await expect(tabs.filter({ hasText: "Terminal" })).toBeVisible();

    await testPage.setViewportSize({ width: 700, height: 800 });

    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
    await expect(testPage.getByTestId("dockview-task-layout")).toHaveCount(0);
    await expect(testPage.getByTestId("tablet-task-layout")).toHaveCount(0);
    await expect(testPage.getByTestId("app-sidebar")).toBeHidden();
    await expect(testPage.getByTestId("mobile-session-menu")).toBeVisible();
    await expect(session.activeChat()).toBeVisible();

    await testPage.getByRole("button", { name: "Status", exact: true }).click();
    await expect(testPage.getByTestId("app-status-drawer")).toBeVisible();
  });

  test("kanban windows readable desktop lanes without leaking subtask hierarchy", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize(COMPACT_DESKTOP_VIEWPORT);

    for (const step of seedData.steps) {
      await apiClient.createTask(seedData.workspaceId, `Compact ${step.title}`, {
        workflow_id: seedData.workflowId,
        workflow_step_id: step.id,
      });
    }
    const parent = await apiClient.createTask(
      seedData.workspaceId,
      "A deliberately long parent task title in compact kanban card",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    const child = await apiClient.createTask(seedData.workspaceId, "Compact child", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const desktopLayout = testPage.getByTestId("desktop-kanban-layout");
    await expect(desktopLayout).toHaveCount(1);
    await expect(desktopLayout).toBeVisible();
    await expect(testPage.getByTestId("desktop-kanban-stage-navigator")).toHaveCount(0);
    await expect(testPage.getByTestId("tablet-kanban-layout")).toHaveCount(0);
    await expect(testPage.getByTestId("mobile-kanban-layout")).toHaveCount(0);
    await expect(testPage.getByRole("button", { name: "Open menu" })).toHaveCount(0);

    await expect(testPage.getByPlaceholder("Search tasks...")).toBeVisible();
    await expect(kanban.createTaskButton).toBeVisible();
    await expect(testPage.getByRole("button", { name: "Quick Chat" })).toBeVisible();
    await expect(kanban.viewTogglePipeline).toBeVisible();

    for (const step of seedData.steps) {
      await expect(kanban.columnByStepId(step.id)).toBeAttached();
    }
    const firstColumnBox = await kanban.columnByStepId(seedData.steps[0].id).boundingBox();
    expect(firstColumnBox).not.toBeNull();
    expect(firstColumnBox!.width).toBeGreaterThanOrEqual(280);

    const scrollWindow = testPage.getByTestId("desktop-kanban-scroll-window");
    await expect(scrollWindow).toBeVisible();
    await scrollWindow.evaluate((element) => {
      element.scrollLeft = element.scrollWidth;
    });
    const lastStep = seedData.steps.at(-1);
    expect(lastStep).toBeDefined();
    await expect(kanban.columnByStepId(lastStep!.id)).toBeInViewport();

    const relationship = kanban.taskCard(child.id).getByTestId("task-parent-relationship");
    const [cardBox, relationshipBox] = await Promise.all([
      kanban.taskCard(child.id).boundingBox(),
      relationship.boundingBox(),
    ]);
    expect(cardBox).not.toBeNull();
    expect(relationshipBox).not.toBeNull();
    expect(relationshipBox!.x).toBeGreaterThanOrEqual(cardBox!.x);
    expect(relationshipBox!.x + relationshipBox!.width).toBeLessThanOrEqual(
      cardBox!.x + cardBox!.width,
    );
    await expect(relationship).toHaveAttribute("title", /deliberately long parent task title/);
    await expect
      .poll(() => testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
      .toBe(true);

    await testPage.setViewportSize({ width: 1600, height: 800 });
    await expect(testPage.getByTestId("desktop-kanban-stage-navigator")).toHaveCount(0);

    // Below the 768px sidebar boundary the board uses mobile composition; the
    // tablet layout is a coarse-pointer fallback and is not reachable here.
    await testPage.setViewportSize({ width: 700, height: 800 });
    await expect(testPage.getByTestId("mobile-kanban-layout")).toBeVisible();
    await expect(testPage.getByTestId("desktop-kanban-stage-navigator")).toHaveCount(0);
  });
});
