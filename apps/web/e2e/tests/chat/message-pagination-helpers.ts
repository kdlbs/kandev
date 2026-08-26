import type { Locator, Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export const TASK_DESCRIPTION_MARKER = "TASK-DESCRIPTION-FALLBACK-9R4M";
export const INITIAL_PROMPT_MARKER = "INITIAL-PROMPT-MARKER-7Q2X";
export const RECENT_AGENT_MARKER = "RECENT-AGENT-MARKER-4K8P";
export const PRE_PROMPT_MARKER = "HIDDEN-PRE-PROMPT-MARKER-6N3V";
export const EAGER_HISTORY_PROMPT_MARKER = "EAGER-HISTORY-PROMPT-MARKER-3J6W";
export const VISIBLE_PAGE_MARKER = "VISIBLE-PAGE-MARKER-8D5H";
export const SHORT_PAGE_BOUNDARY_MARKER = "SHORT-PAGE-BOUNDARY-MARKER-5T1C";

/** Seeds an older prompt followed by a tool-only newest window. */
export async function seedToolHeavyOpeningHistory(
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
    content: EAGER_HISTORY_PROMPT_MARKER,
    authorType: "user",
  });
  await apiClient.seedTaskSession(task.id, {
    sessionId,
    state: "IDLE",
    commandCount: 150,
  });
  return { taskId: task.id, sessionId };
}

/** Captures older-page requests made after this watcher is installed. */
export function watchOlderMessageRequests(page: Page, sessionId: string): string[] {
  const requests: string[] = [];
  page.on("request", (request) => {
    const url = request.url();
    if (url.includes(`/task-sessions/${sessionId}/messages?`) && url.includes("before=")) {
      requests.push(url);
    }
  });
  return requests;
}

/** Seeds collapsed history around the visible prompt boundary. */
export async function seedCollapsedMessageHistory(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  options?: { promptOutsideInitialWindow?: boolean },
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

  if (options?.promptOutsideInitialWindow) {
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
  } else {
    for (let i = 0; i < 20; i += 1) {
      await apiClient.seedSessionMessage(sessionId, {
        type: "tool_call",
        content: `${PRE_PROMPT_MARKER} ${i + 1}`,
      });
    }
    await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: INITIAL_PROMPT_MARKER,
      authorType: "user",
    });
    await apiClient.seedTaskSession(task.id, {
      sessionId,
      state: "IDLE",
      commandCount: 0,
    });
    // Keep the prompt at the oldest edge of the initial 100-message window:
    // 98 collapsed rows plus the recent agent row follow it, while the 20
    // pre-prompt rows remain on the next backend page.
    await apiClient.seedToolCallMessages(sessionId, 98);
  }
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: RECENT_AGENT_MARKER,
  });

  return { taskId: task.id, sessionId };
}

/** Seeds several backend pages of standalone messages behind the newest window. */
export async function seedVisibleMessageHistory(
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
  await apiClient.seedAgentMessages(sessionId, 140, VISIBLE_PAGE_MARKER);
  return { taskId: task.id, sessionId };
}

/** Seeds a boundary-changing older page whose rendered height stays inside
 * the sentinel's 200px preload margin: one short message plus one collapsed
 * group containing the other 19 backend rows. */
export async function seedShortBoundaryPageHistory(
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
  await apiClient.seedAgentMessages(sessionId, 20, "SHORT-PAGE-OLDER-FILLER");
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: SHORT_PAGE_BOUNDARY_MARKER,
  });
  for (let index = 0; index < 19; index += 1) {
    await apiClient.seedSessionMessage(sessionId, {
      type: "tool_call",
      content: `short-page completed tool ${index + 1}`,
      metadata: { status: "complete" },
    });
  }
  await apiClient.seedAgentMessages(sessionId, 100, VISIBLE_PAGE_MARKER);
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

/** Reads one already-rendered message row by its stable DOM id. */
export async function readMessageRowTopById(list: Locator, rowId: string): Promise<number> {
  return list.evaluate((element, id) => {
    const row = Array.from(element.querySelectorAll<HTMLElement>("[id^='msg-']")).find(
      (candidate) => candidate.id === id,
    );
    return row?.getBoundingClientRect().top ?? Number.NaN;
  }, rowId);
}

/** Scrolls the native transcript to the oldest loaded edge and captures the
 * anchored row's position before the next older-page request can prepend. */
export async function scrollToOldestLoadedEdge(
  list: Locator,
  marker: string,
): Promise<{ rowId: string | null; rowTop: number; scrollHeight: number }> {
  return list.evaluate((element, messageMarker) => {
    element.scrollTop = 0;
    element.dispatchEvent(new Event("scroll", { bubbles: true }));
    const row = Array.from(element.querySelectorAll<HTMLElement>("[id^='msg-']")).find(
      (candidate) => candidate.textContent?.includes(messageMarker),
    );
    return {
      rowId: row?.id ?? null,
      rowTop: row?.getBoundingClientRect().top ?? Number.NaN,
      scrollHeight: element.scrollHeight,
    };
  }, marker);
}

/** Applies a small upward movement after prepend restoration. */
export async function scrollUpSlightly(list: Locator): Promise<number> {
  return list.evaluate((element) => {
    const previous = element.scrollTop;
    element.scrollTop = Math.max(0, previous - 24);
    element.dispatchEvent(new Event("scroll", { bubbles: true }));
    return previous - element.scrollTop;
  });
}
