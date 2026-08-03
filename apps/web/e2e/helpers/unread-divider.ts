import { type Locator } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { multiMessageScript } from "./seed-session-messages";
import { waitForSessionDone } from "./session";

/** True only when element is actually scrolled into container's visible
 *  viewport. Unlike Playwright's toBeVisible(), which only checks CSS
 *  visibility, this also verifies the element is scrolled into the
 *  container's viewport, not merely present somewhere in its (taller)
 *  overflow area. Callers must root both locators at
 *  SessionPage.activeChat() — Dockview keeps background chat panels
 *  mounted, and an unscoped locator could otherwise resolve a stale,
 *  hidden panel's elements instead of the one the user is actually
 *  looking at. */
export async function isScrolledIntoView(container: Locator, element: Locator): Promise<boolean> {
  const [containerBox, elementBox] = await Promise.all([
    container.boundingBox(),
    element.boundingBox(),
  ]);
  if (!containerBox || !elementBox) return false;
  return (
    elementBox.y >= containerBox.y &&
    elementBox.y + elementBox.height <= containerBox.y + containerBox.height
  );
}

/**
 * Seeds a task whose transcript buries a read cursor under enough messages
 * both before and after it that, scrolled to the bottom (the naive,
 * unconditional behavior this feature replaces), the divider would land
 * well outside the visible viewport. Shared by the desktop and mobile
 * scroll-to-divider specs — same setup, different viewport/project.
 */
export async function seedScrollTestConversation(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{
  taskId: string;
  sessionId: string;
  cursorMessageId: string;
  newestMessageId: string;
}> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: multiMessageScript(["chat ready"], 5),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  const sessionId = task.session_id;

  await waitForSessionDone(apiClient, task.id, sessionId, "initial unread-divider conversation");

  await apiClient.seedAgentMessages(sessionId, 40, "before cursor");
  const beforeCursor = await apiClient.listSessionMessages(sessionId);
  const cursorMessageId = beforeCursor.messages[beforeCursor.messages.length - 1].id;
  await apiClient.seedAgentMessages(sessionId, 40, "after cursor");
  const fullTranscript = await apiClient.listSessionMessages(sessionId);
  const newestMessageId = fullTranscript.messages[fullTranscript.messages.length - 1].id;

  await apiClient.forceSetSessionReadCursor(sessionId, cursorMessageId);

  return { taskId: task.id, sessionId, cursorMessageId, newestMessageId };
}
