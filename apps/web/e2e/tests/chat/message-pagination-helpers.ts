import type { Locator } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export const TASK_DESCRIPTION_MARKER = "TASK-DESCRIPTION-FALLBACK-9R4M";
export const INITIAL_PROMPT_MARKER = "INITIAL-PROMPT-MARKER-7Q2X";
export const RECENT_AGENT_MARKER = "RECENT-AGENT-MARKER-4K8P";

/** Seeds a user prompt followed by one collapsed same-turn tool history. */
export async function seedCollapsedMessageHistory(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ taskId: string; sessionId: string }> {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: TASK_DESCRIPTION_MARKER,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "IDLE",
    repositoryId: seedData.repositoryId,
  });

  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: INITIAL_PROMPT_MARKER,
    authorType: "user",
  });
  await apiClient.seedTaskSession(task.id, {
    sessionId,
    state: "IDLE",
    commandCount: 80,
  });
  await apiClient.seedToolCallMessages(sessionId, 60);
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: RECENT_AGENT_MARKER,
  });

  return { taskId: task.id, sessionId };
}

/** Reads a rendered standalone message's viewport position from the list. */
export async function readStandaloneMessageTop(list: Locator, marker: string): Promise<number> {
  return list.evaluate((element, messageMarker) => {
    const row = Array.from(element.querySelectorAll<HTMLElement>("[id^='msg-']")).find(
      (candidate) => candidate.textContent?.includes(messageMarker),
    );
    return row?.getBoundingClientRect().top ?? Number.NaN;
  }, marker);
}

/** Scrolls the native transcript to the oldest loaded edge and captures the
 * anchored row's position before the next older-page request can prepend. */
export async function scrollToOldestLoadedEdge(
  list: Locator,
  marker: string,
): Promise<{ rowTop: number; scrollHeight: number }> {
  return list.evaluate((element, messageMarker) => {
    element.scrollTop = 0;
    element.dispatchEvent(new Event("scroll", { bubbles: true }));
    const row = Array.from(element.querySelectorAll<HTMLElement>("[id^='msg-']")).find(
      (candidate) => candidate.textContent?.includes(messageMarker),
    );
    return {
      rowTop: row?.getBoundingClientRect().top ?? Number.NaN,
      scrollHeight: element.scrollHeight,
    };
  }, marker);
}
