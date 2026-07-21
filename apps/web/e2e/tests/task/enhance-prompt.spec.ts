import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";
import type { ExecutePromptRequest } from "@/lib/api/domains/utility-api";

// Exercises the regular task-create dialog (New Task in the sidebar); run with office off.
useRegularMode();

test.describe("Enhance prompt button in task creation", () => {
  test("enhance button is visible and enabled when utility agent is configured", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    let executeBody: ExecutePromptRequest | null = null;
    await testPage.route("**/api/v1/utility/execute", async (route) => {
      executeBody = route.request().postDataJSON() as ExecutePromptRequest;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          success: true,
          call_id: "call-stub",
          response: "Stubbed enhanced task description.",
        }),
      });
    });

    // Seed a default utility agent pointing at mock-agent (by name, not UUID).
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
    await textarea.fill("Draft task description.");

    // The enhance button should be visible and enabled
    const enhanceBtn = testPage.getByTestId("enhance-prompt-button");
    await expect(enhanceBtn).toBeVisible();
    await expect(enhanceBtn).toBeEnabled();

    await enhanceBtn.click();

    await expect(textarea).toHaveValue("Stubbed enhanced task description.");
    expect(executeBody).toMatchObject({
      utility_agent_id: "builtin-enhance-prompt",
      session_id: "",
    });
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
