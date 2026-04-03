import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";

async function openTaskSession(page: Page, title: string): Promise<SessionPage> {
  const kanban = new KanbanPage(page);
  await kanban.goto();

  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 15_000 });
  await card.click();
  await expect(page).toHaveURL(/\/t\//, { timeout: 15_000 });

  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

/** Type "/" in the chat editor and assert the slash command menu appears with mock-agent commands. */
async function expectSlashCommandsVisible(page: Page) {
  const editor = page.locator(".tiptap.ProseMirror").first();
  await editor.click();
  await editor.pressSequentially("/");

  // The popup menu title is "Commands" (from slash-command-menu.tsx)
  const menu = page.getByText("Commands").first();
  await expect(menu).toBeVisible({ timeout: 10_000 });

  // Verify mock-agent commands are listed
  await expect(page.getByText("/slow")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText("/error")).toBeVisible({ timeout: 5_000 });

  // Clear the editor content to leave it in a clean state
  await editor.press("Escape");
  await editor.press("Backspace");
}

test.describe("Slash commands after session resume", () => {
  test.describe.configure({ retries: 1 });

  test("slash commands are available after backend restart and session resume", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // 1. Create task and start agent — the mock agent emits AvailableCommandsUpdate
    //    on the first prompt, registering slash commands like /slow, /error, etc.
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Commands Resume Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Navigate to session and wait for agent to finish first turn
    const session = await openTaskSession(testPage, "Commands Resume Task");
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });

    // 3. Verify slash commands work before restart
    await expectSlashCommandsVisible(testPage);

    // 4. Restart the backend
    await backend.restart();

    // 5. Reload page to reconnect
    await testPage.reload();
    await session.waitForLoad();

    // 6. Wait for auto-resume to complete
    await expect(session.idleInput()).toBeVisible({ timeout: 60_000 });
    await expect(session.chat.getByText("Resumed agent Mock", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 7. Send a follow-up message to trigger AvailableCommandsUpdate from the agent.
    //    The mock agent emits commands on the first Prompt after session load.
    await session.sendMessage("/e2e:simple-message");
    await expect(
      session.chat.getByText("simple mock response", { exact: false }).nth(1),
    ).toBeVisible({ timeout: 30_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });

    // 8. Verify slash commands work after resume
    await expectSlashCommandsVisible(testPage);
  });
});
