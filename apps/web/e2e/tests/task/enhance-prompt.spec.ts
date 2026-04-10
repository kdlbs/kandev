import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Enhance prompt button in task creation", () => {
  test("enhance button is visible and enabled when utility agent is configured", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Seed a default utility agent pointing at mock-agent (by name, not UUID).
    // The mock agent isn't inference-capable so the actual enhance call won't
    // succeed, but the button should render as enabled.
    const { agents } = await apiClient.listAgents();
    const mockAgent = agents.find((a) => a.name === "mock-agent");
    expect(mockAgent, "mock-agent must be registered").toBeTruthy();
    await apiClient.saveUserSettings({
      default_utility_agent_id: mockAgent!.name,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto(seedData.workspaceId);
    await kanban.createTaskButton.first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Fill the description textarea
    const textarea = testPage.getByTestId("task-description-input");
    await textarea.fill("fix the login bug");

    // The enhance button should be visible and enabled
    const enhanceBtn = testPage.getByTestId("enhance-prompt-button");
    await expect(enhanceBtn).toBeVisible();
    await expect(enhanceBtn).toBeEnabled();
  });

  test("enhance button is disabled when no utility agent is configured", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Clear any default utility agent
    await apiClient.saveUserSettings({
      default_utility_agent_id: "",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto(seedData.workspaceId);
    await kanban.createTaskButton.first().click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // The enhance button should exist but be disabled
    const enhanceBtn = testPage.getByTestId("enhance-prompt-button");
    await expect(enhanceBtn).toBeVisible();
    await expect(enhanceBtn).toBeDisabled();
  });
});
