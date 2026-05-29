import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

// Regression: the AppSidebar is mounted globally, so clicking a task in its
// Tasks list from a non-task page (the board) must NAVIGATE to the task route
// and mount the dockview. A prior refactor left this doing an in-place layout
// switch that only rewrote the URL (history.replaceState) without ever
// mounting the dockview, so the click appeared to do nothing.
useRegularMode();

test.describe("Sidebar task open", () => {
  test("clicking a sidebar task from the board opens the dockview", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Sidebar Open Target",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    const item = session.sidebarTaskItem("Sidebar Open Target").first();
    await expect(item).toBeVisible({ timeout: 10_000 });
    await item.click();

    // Must reach the task route and mount the dockview — not just rewrite the URL.
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`), { timeout: 15_000 });
    await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible({ timeout: 15_000 });
  });

  test("returning Home clears the selected-task highlight in the sidebar", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Home Deselect Target",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${task.id}`);
    await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible({ timeout: 15_000 });

    const item = session.sidebarTaskItem("Home Deselect Target").first();
    await expect(item).toHaveAttribute("data-active", "true", { timeout: 10_000 });

    // Back to Home — the global sidebar must drop the selection highlight.
    await testPage.getByRole("link", { name: "Home", exact: true }).click();
    await expect(item).toHaveAttribute("data-active", "false", { timeout: 10_000 });
  });
});
