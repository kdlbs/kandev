// Filename starts with "mobile-" so this runs in the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { seedAvailableCommands } from "../../helpers/session-store";
import { SessionPage } from "../../pages/session-page";

async function createPassthroughProfile(apiClient: ApiClient, name: string): Promise<string> {
  const { agents } = await apiClient.listAgents();
  if (agents.length === 0) throw new Error("no agents registered in this e2e profile");
  const profile = await apiClient.createAgentProfile(agents[0].id, name, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: true,
  });
  return profile.id;
}

async function openMobilePassthroughTask(
  testPage: import("@playwright/test").Page,
  apiClient: ApiClient,
  seedData: {
    workspaceId: string;
    workflowId: string;
    startStepId: string;
    repositoryId: string;
  },
  profileName: string,
  taskTitle: string,
) {
  const profileId = await createPassthroughProfile(apiClient, profileName);
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, taskTitle, profileId, {
    description: "initial prompt",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  if (!task.session_id) throw new Error("expected passthrough task to start a session");

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForPassthroughLoad(20_000);
  await session.waitForPassthroughLoaded(20_000);
  await session.expectPassthroughHasText("Processed:", 20_000);
  return { task, session };
}

test.describe("mobile CLI mode: passthrough composer", () => {
  test("slash remains literal and only passthrough composer controls are available", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const profileId = await createPassthroughProfile(apiClient, "Mobile CLI Commands");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile CLI Commands Task",
      profileId,
      {
        description: "initial prompt",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected passthrough task to start a session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForPassthroughLoad(20_000);
    await session.waitForPassthroughLoaded(20_000);
    await session.expectPassthroughHasText("Processed:", 20_000);
    await seedAvailableCommands(testPage, task.session_id, [
      { name: "slow", description: "Run slowly" },
      { name: "error", description: "Trigger an error" },
    ]);

    await testPage.getByTestId("passthrough-toggle-composer").tap();
    const composer = testPage.getByTestId("passthrough-composer");
    const editor = composer.locator(".tiptap.ProseMirror");
    await expect(editor).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByTestId("plan-mode-toggle-button")).toBeVisible();
    await expect(testPage.getByTestId("chat-context-button")).toBeVisible();
    await expect(testPage.getByTestId("chat-attachments-button")).toBeVisible();
    await expect(testPage.getByTestId("toolbar-item-mcp")).toHaveCount(0);
    await expect(testPage.getByTestId("toolbar-item-mode")).toHaveCount(0);
    await expect(testPage.getByTestId("toolbar-item-model")).toHaveCount(0);
    await expect(testPage.getByTestId("toolbar-item-reset-context")).toHaveCount(0);
    await expect(testPage.getByTestId("toolbar-item-enhance")).toHaveCount(0);

    await editor.fill("/s");

    await expect(testPage.getByRole("listbox", { name: "Command suggestions" })).toHaveCount(0);
    await expect(editor).toHaveText("/s");
  });

  test("context prompt selection creates a chip on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const promptName = `mobile-pt-prompt-${Date.now()}`;
    const promptContent = "MOBILE_PASSTHROUGH_PROMPT_MARKER";
    await apiClient.createPrompt(promptName, promptContent);
    const { session } = await openMobilePassthroughTask(
      testPage,
      apiClient,
      seedData,
      "Mobile CLI Context",
      "Mobile CLI Context Task",
    );

    await testPage.getByTestId("passthrough-toggle-composer").tap();
    const composer = testPage.getByTestId("passthrough-composer");
    const editor = composer.locator(".tiptap.ProseMirror");
    await expect(editor).toBeVisible({ timeout: 5_000 });

    await testPage.getByTestId("chat-context-button").tap();
    const searchInput = testPage.getByPlaceholder("Search files and prompts...");
    await expect(searchInput).toBeVisible({ timeout: 5_000 });
    await searchInput.fill(promptName);
    await testPage.getByText(promptName, { exact: true }).tap();

    await expect(composer.getByText(promptName, { exact: true })).toBeVisible({
      timeout: 5_000,
    });

    await editor.fill("mobile context e2e");
    await testPage.getByTestId("submit-message-button").tap();
    await expect(composer).toBeHidden({ timeout: 10_000 });

    await session.expectPassthroughHasText("mobile context e2e", 15_000);
    await session.expectPassthroughHasText("CONTEXT PROMPTS", 15_000);
    await session.expectPassthroughHasText(promptContent, 15_000);
  });
});
