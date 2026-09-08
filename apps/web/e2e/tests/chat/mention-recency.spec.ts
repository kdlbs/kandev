import { type Locator, type Page } from "@playwright/test";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { CHAT_MENTION_RECENCY_STORAGE_KEY } from "../../../lib/chat-mention-recency";

const RECENT_TITLE = "Project-Mention-Recency-Target";
const STRONGER_TITLE = "Mention-Recency-Stronger-Match";

async function createReadyTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ id: string }> {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

async function openTaskChat(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

function visibleEditor(scope: Locator): Locator {
  return scope.locator(".tiptap.ProseMirror:visible").first();
}

async function typeMention(editor: Locator, query: string): Promise<Locator> {
  await editor.fill("");
  await editor.pressSequentially(`@${query}`);
  const menu = editor.page().getByRole("listbox", { name: /Mention tasks, files, prompts/i });
  await expect(menu).toBeVisible({ timeout: 10_000 });
  return menu;
}

// @covers AC-UI-COMPOSER-MENTION-RECENCY-001.1
// @covers AC-UI-COMPOSER-MENTION-RECENCY-001.2
// @covers AC-UI-COMPOSER-MENTION-RECENCY-001.4
// @covers AC-UI-COMPOSER-MENTION-RECENCY-001.6
test("ranks a selected task first and keeps the order after reload", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await apiClient.seedTask(seedData.workspaceId, RECENT_TITLE, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await apiClient.seedTask(seedData.workspaceId, STRONGER_TITLE, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const active = await createReadyTask(apiClient, seedData, "Mention Recency Active Task");

  await testPage.goto("/");
  await testPage.evaluate(
    (key) => window.localStorage.removeItem(key),
    CHAT_MENTION_RECENCY_STORAGE_KEY,
  );
  const session = await openTaskChat(testPage, active.id);
  const editor = visibleEditor(session.activeChat());

  const baselineMenu = await typeMention(editor, "mention");
  await expect(baselineMenu.getByRole("option").first()).toContainText(STRONGER_TITLE);
  await editor.press("Escape");

  const uniqueMenu = await typeMention(editor, "Project-Mention");
  await uniqueMenu.getByRole("option").filter({ hasText: RECENT_TITLE }).click();
  await expect(uniqueMenu).toHaveCount(0);

  const rankedMenu = await typeMention(editor, "mention");
  await expect(rankedMenu.getByRole("option").first()).toContainText(RECENT_TITLE);
  await prCapture.screenshot("desktop-mention-recency-menu", {
    caption: "A previously selected task leads the desktop mention menu",
  });

  await testPage.reload();
  const reloadedSession = new SessionPage(testPage);
  await reloadedSession.waitForLoad();
  const reloadedEditor = visibleEditor(reloadedSession.activeChat());
  await expect(reloadedEditor).toBeEditable({ timeout: 30_000 });
  const persistedMenu = await typeMention(reloadedEditor, "mention");
  await expect(persistedMenu.getByRole("option").first()).toContainText(RECENT_TITLE);
});
