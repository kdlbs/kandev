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
      await expect(executorSelector).toBeVisible();
      await expect(executorSelector).toBeEnabled();
      await executorSelector.tap();
      const executorOption = testPage.getByRole("option", {
        name: /Mobile Docker Locked Auth/i,
      });
      await expect(executorOption).toBeVisible();
      await executorOption.tap();
      await expect(executorSelector).toContainText("Mobile Docker Locked Auth");

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
      await link.tap();
      await expect(testPage).toHaveURL(
        new RegExp(`/settings/executors/${scenario.dockerProfileId}(?:\\?.*)?$`),
      );
    } finally {
      await workflow.cleanup();
      await scenario.cleanup();
      await backend.restart();
    }
  });

  // @covers AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.1
  // @covers AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.2
  // @covers AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.6
  // @covers AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.9
  test("replaces an incompatible selection on a phone", async ({
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
        executor: "E2E Mobile Docker Replace Agent",
        dockerProfile: "Mobile Docker Codex Only",
        compatibleProfile: "Mobile Codex Compatible",
      },
    );

    try {
      await testPage.setViewportSize({ width: MOBILE_WIDTH, height: 844 });
      const mobile = new MobileKanbanPage(testPage);
      await mobile.goto();
      await mobile.mobileFab.tap();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await dialog.getByTestId("task-title-input").fill("Mobile replacement task");
      await dialog.getByTestId("task-description-input").fill("executor switch on a phone");

      const agentSelector = dialog.getByTestId("agent-profile-selector");
      if (!(await agentSelector.textContent())?.includes(scenario.seedProfileName)) {
        await agentSelector.tap();
        const seedOption = testPage
          .getByRole("option", { name: new RegExp(scenario.seedProfileName) })
          .first();
        await expect(seedOption).toBeVisible();
        await seedOption.tap();
      }
      await expect(agentSelector).toContainText(scenario.seedProfileName);

      const executorSelector = dialog.getByTestId("executor-profile-selector");
      await expect(executorSelector).toBeVisible();
      await expect(executorSelector).toBeEnabled();
      await executorSelector.tap();
      const executorOption = testPage.getByRole("option", {
        name: /Mobile Docker Codex Only/i,
      });
      await expect(executorOption).toBeVisible();
      await executorOption.tap();
      await expect(executorSelector).toContainText("Mobile Docker Codex Only");

      await expect(agentSelector).toContainText(scenario.secondAgentDisplayName);
      await expect(agentSelector).not.toContainText(scenario.seedProfileName);
      await expect(dialog.getByTestId("agent-profile-empty-state")).toHaveCount(0);
      await expect(dialog.getByTestId("agent-profile-incompatible-note")).toHaveCount(0);
      await expect(dialog.getByTestId("submit-start-agent")).toBeEnabled();
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth),
      ).toBeLessThanOrEqual(MOBILE_WIDTH);
    } finally {
      await scenario.cleanup();
      await backend.restart();
    }
  });
});
