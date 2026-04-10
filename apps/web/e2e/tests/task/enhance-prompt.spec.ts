import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Enhance prompt button in task creation", () => {
  test("enhances the prompt when utility agent is configured", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Seed a default utility agent pointing at mock-agent
    const { agents } = await apiClient.listAgents();
    const mockAgent = agents.find((a) => a.name === "mock-agent");
    expect(mockAgent, "mock-agent must be registered").toBeTruthy();
    // Set agent but no model — the mock agent doesn't support session/set_model,
    // so we leave the model unset and let the agent use its own default.
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

    // Click enhance — it calls the mock agent which returns some response
    await enhanceBtn.click();

    // Wait for the textarea to change from the original value
    await expect(textarea).not.toHaveValue("fix the login bug", { timeout: 15_000 });

    // Textarea should have some non-empty content (the enhanced prompt)
    const enhanced = await textarea.inputValue();
    expect(enhanced.length).toBeGreaterThan(0);
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
