import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import type * as SessionApi from "@/lib/api/domains/session-api";
import type { TaskSession } from "@/lib/types/http";
import * as runtimeConfig from "@/lib/config";
import { registerTasksHandlers } from "./tasks";
import { registerTaskSessionHandlers } from "./agent-session";
import { makeDeletedMessage } from "./tasks.test-helpers";
import {
  flushFocusWork,
  membership,
  promote,
  response,
  withSiblings,
} from "./task-primary-focus.test-helpers";

vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  fetchTaskSession: vi.fn(),
  listTaskSessions: vi.fn(),
}));

beforeEach(() => {
  vi.mocked(fetchTaskSession).mockReset();
  vi.mocked(listTaskSessions).mockReset();
});
afterEach(() => vi.restoreAllMocks());

async function removeViewedFailure() {
  const store = withSiblings("FAILED");
  const removed = store.getState().taskSessions.items.old;
  store.getState().updateSessionReadCursor("next", "next-read");
  store.getState().updateSessionReadCursor("latest", "latest-read");
  vi.mocked(fetchTaskSession).mockResolvedValue(response("FAILED"));
  vi.mocked(listTaskSessions).mockResolvedValue(membership("next", "latest"));
  promote(store);
  await flushFocusWork();
  registerTaskSessionHandlers(store)["session.removed"]!({
    id: "removed",
    type: "notification",
    action: "session.removed",
    payload: { session_id: "old", task_id: "t1" },
  });
  await flushFocusWork();
  expect(store.getState().tasks.activeSessionId).toBe("next");
  expect(store.getState().taskSessions.items.old).toBeUndefined();
  return { store, removed };
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
// @covers AC-TASKS-TASK-UNREAD-DOT-001.9
describe("confirmed removal admission", () => {
  it.each([
    "canonical-only",
    "canonical-and-membership",
    "selection",
    "list",
    "single",
    "lifecycle",
    "unproven-incarnation",
  ])("rejects a pre-removal %s publication without touching siblings", async (source) => {
    const { store, removed } = await removeViewedFailure();
    const state = store.getState();
    if (
      source === "canonical-only" ||
      source === "canonical-and-membership" ||
      source === "selection"
    ) {
      state.hydrate({
        taskSessions: { items: { old: removed } },
        ...(source === "selection"
          ? {
              tasks: { ...state.tasks, activeSessionId: "old", lastSessionByTaskId: { t1: "old" } },
            }
          : {}),
        ...(source === "canonical-and-membership"
          ? {
              taskSessionsByTask: {
                itemsByTaskId: { t1: [removed] },
                loadingByTaskId: {},
                loadedByTaskId: {},
              },
            }
          : {}),
      });
    } else if (source === "list") {
      state.setTaskSessionsForTask(
        "t1",
        [removed, state.taskSessions.items.next, state.taskSessions.items.latest],
        {},
      );
    } else if (source === "single") {
      state.setTaskSession(removed);
    } else {
      registerTaskSessionHandlers(store)["session.state_changed"]!({
        id: "late-state",
        type: "notification",
        action: "session.state_changed",
        payload: {
          task_id: "t1",
          session_id: "old",
          new_state: "FAILED",
          updated_at: removed.updated_at,
          ...(source === "unproven-incarnation"
            ? { queue_incarnation_id: "unobserved-incarnation" }
            : {}),
        },
      });
    }
    await flushFocusWork();
    const result = store.getState();
    expect(result.taskSessions.items.old).toBeUndefined();
    expect(result.taskSessionsByTask.itemsByTaskId.t1.some((row) => row.id === "old")).toBe(false);
    expect(result.taskSessionsByTask.itemsByTaskId.t1.map((row) => row.id).sort()).toEqual([
      "latest",
      "next",
    ]);
    expect(result.tasks.activeSessionId).toBe("next");
    expect(result.taskSessions.items.next.last_read_message_id).toBe("next-read");
    expect(result.taskSessions.items.latest.last_read_message_id).toBe("latest-read");
  });
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
describe("removal admission scope", () => {
  it("admits a reincarnated row after ordinary cache eviction", () => {
    const store = withSiblings("FAILED");
    const replacement: TaskSession = {
      ...store.getState().taskSessions.items.old,
      queue_incarnation_id: "replacement",
      state: "STARTING",
    };
    store.getState().removeTaskSession("t1", "old");
    store.getState().setTaskSession(replacement);
    expect(store.getState().taskSessions.items.old).toMatchObject(replacement);
  });

  it("keeps confirmed task deletion ahead of late session admission", () => {
    const store = withSiblings("FAILED");
    const removed = store.getState().taskSessions.items.old;
    registerTasksHandlers(store)["task.deleted"]!(
      makeDeletedMessage({ task_id: "t1", workflow_id: "wf1" }),
    );
    store.getState().upsertTaskSessionFromEvent("t1", removed);
    expect(store.getState().taskSessions.items.old).toBeUndefined();
    expect(store.getState().taskSessionsByTask.itemsByTaskId.t1 ?? []).toEqual([]);
  });

  it("does not carry deletion evidence into another backend scope", async () => {
    const { store, removed } = await removeViewedFailure();
    const config = runtimeConfig.getBackendConfig();
    vi.spyOn(runtimeConfig, "getBackendConfig").mockReturnValue({
      ...config,
      apiBaseUrl: `${config.apiBaseUrl}/other-backend`,
    });
    store.getState().upsertTaskSessionFromEvent("t1", removed);
    expect(store.getState().taskSessions.items.old?.id).toBe("old");
    expect(
      store.getState().taskSessionsByTask.itemsByTaskId.t1.some((row) => row.id === "old"),
    ).toBe(true);
  });
});
