import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { seedIncompatibleAgentScenario, seedLockedWorkflow } from "./agent-compatibility-helpers";

useRegularMode();

const MOBILE_WIDTH = 390;

test.describe("Task creation agent compatibility on mobile", () => {
  // @covers AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.5
  // @covers AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.9
  test("shows the workflow-locked note and keeps the credentials link reachable", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    await backend.restart({ KANDEV_MOCK_PROVIDERS: "codex-acp" });
    const scenario = await seedIncompatibleAgentScenario(
      apiClient,
      testPage,
      seedData.agentProfileId,
      {
        executor: "E2E Mobile Docker Locked Agent",
        dockerProfile: "Mobile Docker Locked Auth",
        compatibleProfile: "Mobile Codex Unlocked",
      },
    );
    const workflow = await seedLockedWorkflow(
      apiClient,
      seedData.workspaceId,
      seedData.workflowId,
      "Mobile Locked Agent",
      seedData.agentProfileId,
    );

    try {
      await testPage.setViewportSize({ width: MOBILE_WIDTH, height: 844 });
      const mobile = new MobileKanbanPage(testPage);
      await mobile.goto();
      await mobile.mobileFab.tap();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("workflow-selector-trigger")).toContainText(
        "Mobile Locked Agent",
      );
      await dialog.getByTestId("task-title-input").fill("Mobile locked agent task");
      await dialog.getByTestId("task-description-input").fill("workflow-locked on a phone");

      const executorSelector = dialog.getByTestId("executor-profile-selector");
      await expect(async () => {
        await executorSelector.tap();
        await testPage.getByRole("option", { name: /Mobile Docker Locked Auth/i }).click();
        await expect(executorSelector).toContainText("Mobile Docker Locked Auth", {
          timeout: 1_000,
        });
      }).toPass({ timeout: 10_000 });

      const note = dialog.getByTestId("agent-profile-incompatible-note");
      await expect(note).toBeVisible();
      await expect(note).toContainText("Mobile Locked Agent");
      await expect(note).toContainText(scenario.seedProfileName);
      await expect(note).toContainText("Mobile Docker Locked Auth");
      const link = dialog.getByRole("link", { name: "Configure credentials" });
      await expect(link).toBeVisible();
      await expect(link).toHaveAttribute("href", `/settings/executors/${scenario.dockerProfileId}`);
      await expect(dialog.getByTestId("agent-profile-empty-state")).toHaveCount(0);
      await expect(dialog.getByTestId("submit-start-agent")).toBeDisabled();
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(MOBILE_WIDTH);
    } finally {
      await workflow.cleanup();
      await scenario.cleanup();
      await backend.restart();
    }
  });
});
