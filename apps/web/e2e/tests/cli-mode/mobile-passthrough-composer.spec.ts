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

test.describe("mobile CLI mode: passthrough composer", () => {
  test("slash remains literal and shared composer controls are available", async ({
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

    await editor.fill("/s");

    await expect(testPage.getByRole("listbox", { name: "Command suggestions" })).toHaveCount(0);
    await expect(editor).toHaveText("/s");
  });
});
