import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { SidebarTaskColorAutomation } from "../../../lib/task-color-automation-settings";

const MOBILE_REPOSITORY_RULE_ID = "mobile-repository-rule";

test.describe("Mobile sidebar automatic task colors", () => {
  test("keeps repository selection in the drawer and applies the stored rule", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await apiClient.createTask(seedData.workspaceId, "Mobile automatic color task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const navTask = await apiClient.seedTask(seedData.workspaceId, "Automatic colors mobile nav", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const automation: SidebarTaskColorAutomation = {
      enabled: true,
      rules: [
        {
          id: MOBILE_REPOSITORY_RULE_ID,
          enabled: true,
          condition: {
            dimension: "repository",
            value: {
              kind: "local",
              path: seedData.repositoryPath,
            },
            label: "E2E Repo",
          },
          output: { kind: "fixed", color: "purple" },
        },
      ],
    };
    await apiClient.saveUserSettings({ sidebar_task_color_automation: automation });

    await testPage.goto(`/t/${navTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").click();
    const sheet = testPage.getByRole("dialog", { name: "Tasks" });
    await expect(sheet.getByTestId("sidebar-filter-bar")).toBeVisible();

    const row = sheet
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Mobile automatic color task" })
      .first();
    await expect(row).toBeVisible();
    await expect(row.getByTestId("task-item-color-marker")).toHaveAttribute(
      "data-color-token",
      "purple",
    );

    await sheet.getByTestId("sidebar-filter-gear").tap();
    const popover = testPage.getByTestId("sidebar-filter-popover");
    await expect(popover).toBeVisible();
    const automaticSettings = popover.getByTestId("automatic-color-settings");
    await automaticSettings.getByTestId("automatic-color-settings-toggle").tap();

    const repositoryTrigger = automaticSettings.getByTestId(
      `automatic-color-repository-trigger-${MOBILE_REPOSITORY_RULE_ID}`,
    );
    await expect(repositoryTrigger).toBeVisible();
    await repositoryTrigger.tap();
    const repositoryPane = automaticSettings.getByTestId("automatic-color-repository-pane");
    await expect(repositoryPane).toBeVisible();
    const repositoryOption = repositoryPane.getByRole("button", { name: "E2E Repo", exact: false });
    await expect(repositoryOption).toBeVisible();
    await expect(testPage.getByTestId("sidebar-filter-drawer")).toContainText("E2E Repo");
    await prCapture.screenshot("mobile-automatic-task-colors", {
      caption: "Mobile repository target picker for automatic task colors",
    });

    await repositoryOption.tap();
    await expect(repositoryPane).toBeHidden();

    await testPage.reload();
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").click();
    const reloadedSheet = testPage.getByRole("dialog", { name: "Tasks" });
    await expect(reloadedSheet.getByTestId("sidebar-filter-bar")).toBeVisible();
    await expect(
      reloadedSheet
        .getByTestId("sidebar-task-item")
        .filter({ hasText: "Mobile automatic color task" })
        .getByTestId("task-item-color-marker"),
    ).toHaveAttribute("data-color-token", "purple");
    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.sidebar_task_color_automation)
      .toEqual(automation);
  });
});
