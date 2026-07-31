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
  return session;
}

/**
 * Multiple TipTap instances can be mounted in dockview and mobile layouts;
 * scope to the first visible one.
 */
export function composerEditor(page: Page) {
  return page.locator(".tiptap.ProseMirror:visible").first();
}
