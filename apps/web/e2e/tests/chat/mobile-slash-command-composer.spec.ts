import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { seedAvailableCommands } from "../../helpers/session-store";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { CreateTaskResponse } from "../../../lib/types/http";

const SLOW_COMMAND = {
  name: "slow",
  description: "Run a slow response",
  input_hint: "duration",
};

async function createReadyTask(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<CreateTaskResponse> {
  return apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Slash Command Draft",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
}

async function openTaskChat(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

test.describe("Mobile slash command composer", () => {
  test("tapping a slash command keeps it as a draft", async ({ testPage, apiClient, seedData }) => {
    const task = await createReadyTask(apiClient, seedData);
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    const session = await openTaskChat(testPage, task.id);
    await seedAvailableCommands(testPage, task.session_id, [SLOW_COMMAND]);

    // Multiple TipTap instances can be mounted in mobile layouts; scope to the first visible one.
    const editor = testPage.locator(".tiptap.ProseMirror:visible").first();
    await editor.tap();
    await editor.fill("");
    await editor.pressSequentially("/s");

    const command = testPage.getByText("/slow", { exact: true });
    await expect(command).toBeVisible({ timeout: 5_000 });
    await command.tap();

    await expect(editor).toHaveText(/slow/, { timeout: 5_000 });
    await expect(editor.getByTestId("slash-command-chip")).toHaveText("slow", { timeout: 5_000 });
    const chatList = session.chat.locator(".chat-message-list:visible");
    await expect(chatList.getByText("/slow", { exact: false })).not.toBeVisible({ timeout: 1_000 });

    await editor.pressSequentially("1s");
    await expect(editor).toHaveText(/slow\s+1s/, { timeout: 5_000 });
    await testPage.getByTestId("submit-message-button").tap();
    await expect(chatList.getByText("/slow 1s", { exact: false })).toBeVisible({
      timeout: 10_000,
    });
  });
});
