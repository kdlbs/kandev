import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const COMPACT_DESKTOP_VIEWPORT = { width: 900, height: 800 };

// Exercises the regular task-create dialog (New Task in the sidebar); run with office off.
useRegularMode();

test.describe("compact desktop responsive layout", () => {
  test("task page keeps the Dockview workbench at half-screen width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize(COMPACT_DESKTOP_VIEWPORT);

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
  });

  test("kanban keeps desktop controls and usable columns at half-screen width", async ({
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

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const desktopLayout = testPage.getByTestId("desktop-kanban-layout");
    await expect(desktopLayout).toHaveCount(1);
    await expect(desktopLayout).toBeVisible();
    await expect(desktopLayout).toHaveAttribute("style", /minmax\(260px, 1fr\)/);
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
  });
});
