import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import { SidebarTasksPage } from "../../pages/sidebar-tasks-page";
import {
  seedColorTasks,
  expectStoredColors,
  chooseSidebarColor,
} from "../../helpers/bulk-task-colors";
import type { SidebarTaskColorAutomation } from "../../../lib/task-color-automation-settings";

// @covers AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006.1 through .4
test("sidebar colors exactly selected tasks, preserves selection, and clears after reload", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const [a, b, control] = await seedColorTasks(apiClient, seedData);
  await testPage.goto(`/t/${control.id}`);
  await new SessionPage(testPage).waitForLoad();
  const sidebar = new SidebarTasksPage(testPage);
  await sidebar.cmdClick(a.id);
  await sidebar.cmdClick(b.id);
  await sidebar.rightClick(a.id);
  await chooseSidebarColor(testPage, "Blue");
  await expectStoredColors(apiClient, [a.id, b.id], "blue");
  await expectStoredColors(apiClient, [control.id], null);
  await sidebar.expectSelected(a.id);
  await sidebar.expectSelected(b.id);
  await expect(testPage).toHaveURL(new RegExp(`/t/${control.id}$`));
  await testPage.reload();
  await new SessionPage(testPage).waitForLoad();
  for (const id of [a.id, b.id]) {
    await expect(sidebar.row(id).getByTestId("task-item-color-marker")).toHaveAttribute(
      "data-color-token",
      "blue",
    );
    await sidebar.cmdClick(id);
  }
  await sidebar.rightClick(a.id);
  await chooseSidebarColor(testPage, "None");
  await expectStoredColors(apiClient, [a.id, b.id], null);
  await sidebar.expectSelected(a.id);
});

test("board bulk colors persist while automatic rules retain precedence", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { settings: baseline } = await apiClient.getUserSettings();
  const [a, b, control] = await seedColorTasks(apiClient, seedData);
  const automation: SidebarTaskColorAutomation = {
    enabled: true,
    rules: [
      {
        id: "bulk-color-rule",
        enabled: true,
        condition: {
          dimension: "workflow_step",
          value: { workspace_id: seedData.workspaceId, step_id: seedData.startStepId },
          label: "Start",
        },
        output: { kind: "fixed", color: "red" },
      },
    ],
  };
  try {
    await apiClient.saveUserSettings({ sidebar_task_color_automation: automation });
    const board = new KanbanPage(testPage);
    await board.goto();
    await board.selectTask(a.id);
    await board.selectTask(b.id);
    await testPage.getByTestId("bulk-color-button").click();
    await testPage.getByTestId("bulk-color-option-purple").click();
    await expectStoredColors(apiClient, [a.id, b.id], "purple");
    await expectStoredColors(apiClient, [control.id], null);
    await expect(board.multiSelectToolbar).toContainText("2 selected");
    await testPage.goto(`/t/${control.id}`);
    await new SessionPage(testPage).waitForLoad();
    await testPage.reload();
    await new SessionPage(testPage).waitForLoad();
    const sidebar = new SidebarTasksPage(testPage);
    await expect(sidebar.row(a.id).getByTestId("task-item-color-marker")).toHaveAttribute(
      "data-color-token",
      "red",
    );
    await expectStoredColors(apiClient, [a.id, b.id], "purple");
  } finally {
    await apiClient.saveUserSettings({
      sidebar_task_color_automation:
        baseline.sidebar_task_color_automation as SidebarTaskColorAutomation,
    });
  }
});
