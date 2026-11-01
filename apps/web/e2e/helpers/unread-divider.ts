import { type Locator } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { multiMessageScript } from "./seed-session-messages";
import { waitForSessionDone } from "./session";
import type { WsFrame, WsWatcher } from "./causal-waits";

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

export const TASK_UNREAD_DOT_LABEL = "Task has unread messages";

/** Arms a wait for the task status summary that lists `taskId` as unread.
 *  Arm it before the action that makes the backend publish the summary. */
export function waitForTaskUnreadSummary(ws: WsWatcher, taskId: string): Promise<WsFrame> {
  return ws.waitForEvent("task.status_summary.updated", {
    where: (payload) => {
      const summary = payload.status_summary as { unread_session_ids?: string[] } | undefined;
      return payload.task_id === taskId && (summary?.unread_session_ids?.length ?? 0) > 0;
    },
  });
}

export type UnreadDotTasks = {
  navTitle: string;
  targetTitle: string;
  cursorlessTitle: string;
  navTaskId: string;
  navSessionId: string;
  targetTaskId: string;
  targetSessionId: string;
  cursorlessSessionId: string;
};

/**
 * Seeds three agent-less tasks for the task-row unread dot: a navigation task
 * the spec opens, a target whose session has a read cursor at its tail, and a
 * task whose session was never read (no cursor, so its agent output is
 * unread). The cursor is force-set through the e2e route, which also moves the
 * live task-status projection.
 */
export async function seedUnreadDotTasks(
  apiClient: ApiClient,
  seedData: SeedData,
  prefix: string,
): Promise<UnreadDotTasks> {
  const stepOptions = { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId };
  const titles = {
    navTitle: `${prefix} navigation`,
    targetTitle: `${prefix} target`,
    cursorlessTitle: `${prefix} never opened`,
  };
  const sessionFor = async (title: string) => {
    const task = await apiClient.seedTask(seedData.workspaceId, title, stepOptions);
    const session = await apiClient.seedTaskSession(task.task_id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    return { taskId: task.task_id, sessionId: session.session_id };
  };
  const nav = await sessionFor(titles.navTitle);
  const target = await sessionFor(titles.targetTitle);
  const cursorless = await sessionFor(titles.cursorlessTitle);

  const read = await apiClient.seedSessionMessage(target.sessionId, {
    type: "message",
    content: "already read",
  });
  await apiClient.forceSetSessionReadCursor(target.sessionId, read.messageId);
  await apiClient.seedSessionMessage(cursorless.sessionId, {
    type: "message",
    content: "never read",
  });

  return {
    ...titles,
    navTaskId: nav.taskId,
    navSessionId: nav.sessionId,
    targetTaskId: target.taskId,
    targetSessionId: target.sessionId,
    cursorlessSessionId: cursorless.sessionId,
  };
}

/** Row height in CSS pixels; the dot must never change it. */
export async function rowHeight(row: Locator): Promise<number> {
  const box = await row.boundingBox();
  if (!box) throw new Error("expected a measurable task row");
  return box.height;
}
