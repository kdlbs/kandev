import { type Page } from "@playwright/test";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { CreateTaskResponse } from "../../../lib/types/http";

/** Shared setup for the desktop and mobile composer font-size specs. */
export async function createReadyTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<CreateTaskResponse> {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

export async function openTaskChat(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await session.composerReady();
  return session;
}

/**
 * The prompt composer of the active chat panel. Dockview and mobile layouts keep
 * background panels mounted, and a chat transcript can host read-only
 * `[contenteditable="false"]` ProseMirror decorations (mention chips, code-block
 * views), so a bare `.tiptap.ProseMirror` match could measure the wrong element.
 * Scope through `activeChat()` and require an editable host — the same locator
 * `SessionPage.sendMessage()` uses.
 */
export function composerEditor(session: SessionPage) {
  return session.activeChat().locator('.tiptap.ProseMirror[contenteditable="true"]').first();
}
