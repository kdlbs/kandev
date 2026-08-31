import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import type { SidebarTaskColorAutomation } from "../../../lib/task-color-automation-settings";

const DESKTOP_AUTOMATION: SidebarTaskColorAutomation = {
  enabled: true,
  rules: [
    {
      id: "desktop-failed-rule",
      enabled: true,
      condition: { dimension: "task_state", value: "FAILED", label: "Failed" },
      output: { kind: "fixed", color: "red" },
    },
    {
      id: "desktop-review-rule",
      enabled: true,
      condition: { dimension: "task_state", value: "REVIEW", label: "Review" },
      output: { kind: "fixed", color: "blue" },
    },
  ],
};

async function openDesktopTask(
  testPage: import("@playwright/test").Page,
  apiClient: import("../../helpers/api-client").ApiClient,
  seedData: import("../../fixtures/test-base").SeedData,
  title: string,
  state: string,
) {
  const task = await apiClient.seedTask(seedData.workspaceId, title, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    state,
  });
  const navTask = await apiClient.seedTask(seedData.workspaceId, "Automatic colors desktop nav", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await testPage.goto(`/t/${navTask.task_id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  return { task, session };
}

function sidebarTaskRow(session: SessionPage, title: string) {
  return session.sidebar.getByTestId("sidebar-task-item").filter({ hasText: title }).first();
}

test.describe("Sidebar automatic task colors", () => {
  test("persists ordered rules and recolors a task when its state changes", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await apiClient.saveUserSettings({ sidebar_task_color_automation: DESKTOP_AUTOMATION });
    const { task, session } = await openDesktopTask(
      testPage,
      apiClient,
      seedData,
      "Desktop automatic color task",
      "FAILED",
    );
    const row = sidebarTaskRow(session, "Desktop automatic color task");
    const marker = row.getByTestId("task-item-color-marker");

    await expect(row).toBeVisible();
    await expect(marker).toHaveAttribute("data-color-token", "red");
    await expect(marker).toHaveClass(/bg-red-500/);

    const filters = new SidebarFilterPopoverPage(testPage);
    await filters.open();
    const automaticSettings = filters.popover.getByTestId("automatic-color-settings");
    await expect(automaticSettings).toBeVisible();
    await automaticSettings.getByTestId("automatic-color-settings-toggle").click();
    await expect(automaticSettings.getByTestId("automatic-color-enabled")).toHaveAttribute(
      "data-state",
      "checked",
    );
    await prCapture.screenshot("desktop-automatic-task-colors", {
      caption: "Desktop automatic task color rules",
    });
    await filters.close();

    await apiClient.updateTaskState(task.task_id, "REVIEW");
    await expect(marker).toHaveAttribute("data-color-token", "blue");
    await expect(marker).toHaveClass(/bg-blue-500/);

    await testPage.reload();
    await session.waitForLoad();
    const reloadedMarker = sidebarTaskRow(session, "Desktop automatic color task").getByTestId(
      "task-item-color-marker",
    );
    await expect(reloadedMarker).toHaveAttribute("data-color-token", "blue");
    await expect
      .poll(async () => {
        const { settings } = await apiClient.getUserSettings();
        return settings.sidebar_task_color_automation;
      })
      .toEqual(DESKTOP_AUTOMATION);
  });
});
