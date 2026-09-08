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
export const TEXT_BATCH_MARKER = "TEXT-BATCH-MARKER-1F9L";
export const TEXT_BATCH_ANCHOR_MARKER = "TEXT-BATCH-ANCHOR-MARKER-4C7N";
export const DEEP_PROMPT_MARKER = "DEEP-PROMPT-MARKER-2P7N";
export const LONG_HISTORY_TAIL_MARKER = "LONG-HISTORY-TAIL-MARKER-6V4R";
export const RESTORED_SESSION_OLDER_MARKER = "RESTORED-SESSION-OLDER-MARKER-1C9F";
export const RESTORED_SESSION_TAIL_MARKER = "RESTORED-SESSION-TAIL-MARKER-8B3K";

/** Simulates the browser missing a fresh transcript sentinel entry after a
 * hidden/restored geometry transition. Current-geometry recovery must remain
 * independently reachable from panel activation or hard-top input. */
export async function suppressChatPaginationIntersections(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const NativeIntersectionObserver = window.IntersectionObserver;
    window.IntersectionObserver = class extends NativeIntersectionObserver {
      constructor(callback: IntersectionObserverCallback, options?: IntersectionObserverInit) {
        super((entries, observer) => {
          callback(
            entries.filter((entry) => !entry.target.closest(".chat-message-list")),
            observer,
          );
        }, options);
      }
    };
  });
}

/** Seeds two sessions so the target transcript hydrates behind the active
 * primary Dockview tab with its oldest-page sentinel at hidden geometry. */
export async function seedRestoredInactiveSessionHistory(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ taskId: string; primarySessionId: string; targetSessionId: string }> {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: TASK_DESCRIPTION_MARKER,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const { session_id: primarySessionId } = await apiClient.seedTaskSession(task.id, {
    state: "IDLE",
    repositoryId: seedData.repositoryId,
  });
  await apiClient.seedSessionMessage(primarySessionId, {
    type: "message",
    content: "RESTORED-SESSION-PRIMARY-MARKER-4H2D",
  });
  const { session_id: targetSessionId } = await apiClient.seedTaskSession(task.id, {
    state: "IDLE",
    repositoryId: seedData.repositoryId,
  });
  await apiClient.seedSessionMessage(targetSessionId, {
    type: "message",
    content: INITIAL_PROMPT_MARKER,
    authorType: "user",
  });
  await apiClient.seedSessionMessage(targetSessionId, {
    type: "message",
    content: RESTORED_SESSION_OLDER_MARKER,
  });
  await apiClient.seedAgentMessages(targetSessionId, 110, RESTORED_SESSION_TAIL_MARKER);
  return { taskId: task.id, primarySessionId, targetSessionId };
}

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

/** Seeds a prompt more than twenty older pages behind a bounded tool-only
 * newest window. All newer activity shares one turn so collapsed rendering
 * keeps the sentinel in preload while the cursor pages are committed. */
export async function seedLongMessageHistory(
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
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: DEEP_PROMPT_MARKER,
    authorType: "user",
  });
  await apiClient.seedToolCallMessages(sessionId, 520, { status: "complete" });
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: LONG_HISTORY_TAIL_MARKER,
  });
  // Keep the initial bounded window taller than the sentinel preload margin
  // so opening the task does not start pagination before the test scrolls up.
  await apiClient.seedAgentMessages(sessionId, 40, "LONG-HISTORY-VISIBLE-TAIL");

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
    });
    await apiClient.seedToolCallMessages(sessionId, 140, { status: "complete" });
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
  if (options?.promptOutsideInitialWindow) {
    await apiClient.seedAgentMessages(sessionId, 20, "RECENT-VISIBLE-TAIL");
  }

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

/** Seeds a short standalone boundary page followed by a collapsed page. The
 * first older page remains inside the sentinel preload margin, so the native
 * transcript must continue once before the second page moves it out. */
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

/** Seeds one older page of standalone tool rows between the newest window and
 * twenty older text rows. One upward reach should cross the tool-only page and
 * expose the text batch without requiring another gesture. */
export async function seedTextSparseMessageHistory(
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
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: TEXT_BATCH_MARKER,
  });
  await apiClient.seedAgentMessages(sessionId, 19, "TEXT-BATCH-OLDER-FILLER");
  for (let index = 0; index < 20; index += 1) {
    await apiClient.seedSessionMessage(sessionId, {
      type: "tool_call",
      content: `standalone completed tool ${index + 1}`,
      metadata: { status: "complete" },
      newTurn: true,
    });
  }
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: TEXT_BATCH_ANCHOR_MARKER,
    newTurn: true,
  });
  await apiClient.seedAgentMessages(sessionId, 99, "TEXT-BATCH-VISIBLE-TAIL");

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
