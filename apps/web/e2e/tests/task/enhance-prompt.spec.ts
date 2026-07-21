import { test, expect } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";
import type { ExecutePromptRequest } from "@/lib/api/domains/utility-api";

// Exercises the regular task-create dialog (New Task in the sidebar); run with office off.
useRegularMode();

async function configureDefaultUtilityAgent(apiClient: {
  listAgents: () => Promise<{ agents: Array<{ name: string }> }>;
  saveUserSettings: (settings: { default_utility_agent_id: string }) => Promise<void>;
}) {
  const { agents } = await apiClient.listAgents();
  const mockAgent = agents.find((a) => a.name === "mock-agent");
  expect(mockAgent, "mock-agent must be registered").toBeTruthy();
  await apiClient.saveUserSettings({
    default_utility_agent_id: mockAgent!.name,
  });
}

async function openCreateTaskDialog(
  testPage: Parameters<typeof test>[0]["testPage"],
  workspaceId: string,
) {
  const kanban = new KanbanPage(testPage);
  await kanban.goto(workspaceId);
  await kanban.createTaskButton.first().click();

  const dialog = testPage.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  return dialog;
}

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

    await configureDefaultUtilityAgent(apiClient);
    await openCreateTaskDialog(testPage, seedData.workspaceId);

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

  test("keeps a mid-flight edit, offers recovery, and applies only when requested", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const initialPrompt = "Synthetic initial enhancement request.";
    const editedPrompt = "Synthetic mid-flight edited request.";
    const generatedPrompt = "Synthetic generated enhancement result.";
    const syntheticCallId = "synthetic-call-id";

    let executeBody: ExecutePromptRequest | null = null;
    let releaseResponse: (() => void) | null = null;
    const responseGate = new Promise<void>((resolve) => {
      releaseResponse = resolve;
    });

    await testPage.route("**/api/v1/utility/execute", async (route) => {
      executeBody = route.request().postDataJSON() as ExecutePromptRequest;
      await responseGate;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          success: true,
          call_id: syntheticCallId,
          response: generatedPrompt,
        }),
      });
    });

    await testPage.context().grantPermissions(["clipboard-read", "clipboard-write"]);
    await configureDefaultUtilityAgent(apiClient);
    const dialog = await openCreateTaskDialog(testPage, seedData.workspaceId);

    const textarea = testPage.getByTestId("task-description-input");
    const enhanceBtn = testPage.getByTestId("enhance-prompt-button");

    await textarea.fill(initialPrompt);
    await enhanceBtn.click();

    await expect
      .poll(() => executeBody?.user_prompt ?? null, { timeout: 5_000 })
      .toBe(initialPrompt);

    await textarea.fill(editedPrompt);
    releaseResponse?.();

    const recovery = testPage.getByTestId("prompt-result-recovery");
    await expect(recovery).toBeVisible();
    await expect(textarea).toHaveValue(editedPrompt);
    await expect(dialog).not.toContainText(generatedPrompt);
    await expect(dialog).not.toContainText(syntheticCallId);

    await recovery.getByRole("button", { name: "Copy" }).click();
    await expect
      .poll(() => testPage.evaluate(() => navigator.clipboard.readText()), { timeout: 5_000 })
      .toBe(generatedPrompt);
    await expect(textarea).toHaveValue(editedPrompt);

    await recovery.getByRole("button", { name: "Apply" }).click();
    await expect(textarea).toHaveValue(generatedPrompt);
    await expect(recovery).toHaveCount(0);
  });

  test("keeps the description unchanged and shows the existing failure toast when enhancement fails", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const initialPrompt = "Synthetic failure-path prompt.";
    const utilityError = "Synthetic utility failure.";

    await testPage.route("**/api/v1/utility/execute", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          success: false,
          error: utilityError,
        }),
      });
    });

    await configureDefaultUtilityAgent(apiClient);
    await openCreateTaskDialog(testPage, seedData.workspaceId);

    const textarea = testPage.getByTestId("task-description-input");
    const enhanceBtn = testPage.getByTestId("enhance-prompt-button");

    await textarea.fill(initialPrompt);
    await enhanceBtn.click();

    await expect(textarea).toHaveValue(initialPrompt);
    const toast = testPage.getByTestId("toast-message");
    await expect(toast).toBeVisible({ timeout: 5_000 });
    await expect(toast).toContainText("Generation failed");
    await expect(toast).toContainText(utilityError);
    await expect(testPage.getByTestId("prompt-result-recovery")).toHaveCount(0);
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
