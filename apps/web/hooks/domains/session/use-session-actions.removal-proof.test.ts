import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { registerTaskSessionHandlers } from "@/lib/ws/handlers/agent-session";
import { registerTasksHandlers } from "@/lib/ws/handlers/tasks";
import { makeMessage, makeTask } from "@/lib/ws/handlers/tasks.test-helpers";
import { getTaskSessionReads } from "@/lib/state/task-session-reads";
import {
  flushFocusWork,
  membership,
  promote,
  response,
  withSiblings,
} from "@/lib/ws/handlers/task-primary-focus.test-helpers";
import { useSessionActions } from "./use-session-actions";

const REMOVED_ACTION = "session.removed";
const SURVIVING_PIN = "surviving-pin";

const harness = vi.hoisted(() => ({
  store: null as StoreApi<AppState> | null,
  request: vi.fn(),
  toast: vi.fn(),
  updateToast: vi.fn(),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => harness.store,
  useAppStore: (selector: (state: AppState) => unknown) => selector(harness.store!.getState()),
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: harness.toast, updateToast: harness.updateToast }),
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: harness.request }),
}));
vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  fetchTaskSession: vi.fn(),
  listTaskSessions: vi.fn(),
}));
beforeEach(() => {
  harness.request.mockReset();
  harness.toast.mockReset();
  harness.updateToast.mockReset();
  vi.mocked(fetchTaskSession).mockReset();
  vi.mocked(listTaskSessions).mockReset();
});
afterEach(() => {
  cleanup();
  harness.store = null;
  vi.clearAllMocks();
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
it("never selects a background session deleted while another removal proof is pending", async () => {
  const store = withSiblings("FAILED");
  harness.store = store;
  harness.request.mockResolvedValue(undefined);
  vi.mocked(fetchTaskSession).mockResolvedValue(response("FAILED"));
  promote(store);
  await flushFocusWork();
  const oldProof = Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
  const freshProof = Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
  vi.mocked(listTaskSessions)
    .mockReturnValueOnce(oldProof.promise)
    .mockReturnValue(freshProof.promise);
  registerTaskSessionHandlers(store)[REMOVED_ACTION]!({
    id: "removed-a",
    type: "notification",
    action: REMOVED_ACTION,
    payload: { session_id: "old", task_id: "t1" },
  });
  const selections: Array<string | null> = [];
  const release = store.subscribe((state) => selections.push(state.tasks.activeSessionId));
  const { result } = renderHook(() => useSessionActions({ taskId: "t1", sessionId: "next" }));
  try {
    await act(async () => {
      expect(await result.current.remove({ feedback: "inline" })).toBe(true);
    });
    expect(store.getState().tasks.activeSessionId).toBe("old");
    oldProof.resolve(membership("next", "latest"));
    await flushFocusWork();
    expect(selections).not.toContain("next");
    expect(store.getState().taskSessions.items.next).toBeUndefined();
    freshProof.resolve(membership("latest"));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("latest");
    expect(store.getState().taskSessionsByTask.itemsByTaskId.t1.map((row) => row.id)).toEqual([
      "latest",
    ]);
  } finally {
    release();
  }
});

function beginMove(store: ReturnType<typeof withSiblings>) {
  return store.getState().beginWorkflowSessionFocus({
    taskId: "t1",
    workflowId: "wf1",
    destinationStepId: "step2",
    presentationToken: 1,
    navigationRevision: store.getState().taskRemoval.navigationRevision,
  });
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
it.each(["continue", "newer-pin", "newer-workflow", "existing-workflow"] as const)(
  "owns local selected-delete acknowledgement with %s",
  async (outcome) => {
    const store = withSiblings("CANCELLED");
    harness.store = store;
    store.getState().setActiveTask("elsewhere");
    promote(store, "latest");
    store.getState().setTaskSessionsForTask(
      "t1",
      [
        response("CANCELLED").session,
        {
          ...response("WAITING_FOR_INPUT", "next").session,
          started_at: "2026-11-19T12:00:01Z",
          updated_at: "2026-11-19T12:00:01Z",
        },
        {
          ...response("WAITING_FOR_INPUT", "latest").session,
          last_read_message_id: "survivor-read",
        },
      ],
      {},
    );
    store.getState().setActiveSessionAuto("t1", "old");
    const acknowledgement = Promise.withResolvers<unknown>();
    harness.request.mockReturnValue(acknowledgement.promise);
    const background = Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
    const fresh = Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
    vi.mocked(listTaskSessions)
      .mockReturnValueOnce(background.promise)
      .mockReturnValue(fresh.promise);
    const selected: Array<string | null> = [];
    const release = store.subscribe((state) => selected.push(state.tasks.activeSessionId));
    const { result } = renderHook(() => useSessionActions({ taskId: "t1", sessionId: "old" }));
    let move: number | null = outcome === "existing-workflow" ? beginMove(store) : null;
    try {
      let deleting!: Promise<boolean>;
      await act(async () => {
        deleting = result.current.remove({ feedback: "inline" });
      });
      registerTaskSessionHandlers(store)[REMOVED_ACTION]!({
        id: "background-removed",
        type: "notification",
        action: REMOVED_ACTION,
        payload: { session_id: "next", task_id: "t1" },
      });
      if (outcome === "newer-pin") store.getState().setActiveSession("t1", "latest");
      if (outcome === "newer-workflow") {
        move = beginMove(store);
        expect(move).not.toBeNull();
      }
      await act(async () => {
        acknowledgement.resolve(undefined);
        expect(await deleting).toBe(true);
      });
      expect(selected).not.toContain("next");
      expect(harness.toast).not.toHaveBeenCalled();
      expect(harness.updateToast).not.toHaveBeenCalled();
      fresh.resolve(membership("latest"));
      background.resolve(membership("latest"));
      await flushFocusWork();
      expect(selected).not.toContain("next");
      expect(store.getState().taskSessions.items.old).toBeUndefined();
      expect(store.getState().taskSessions.items.next).toBeUndefined();
      expect(store.getState().taskSessions.items.latest.last_read_message_id).toBe("survivor-read");
      if (outcome === "continue") expect(store.getState().tasks.activeSessionId).toBe("latest");
      if (outcome === "newer-pin") expect(store.getState().tasks.pinnedSessionId).toBe("latest");
      if (outcome === "newer-workflow" || outcome === "existing-workflow") {
        expect(selected).not.toContain("latest");
        expect(store.getState().workflowSessionFocus.intent?.requestId).toBe(move);
        store.getState().bindWorkflowSessionFocus({
          requestId: move!,
          presentationToken: 1,
          entryIdentity: "entry:00000000000000000042",
        });
        registerTasksHandlers(store)["task.updated"]!(
          makeMessage({
            ...makeTask("t1", "latest"),
            workflow_step_id: "step2",
            metadata: {
              workflow_session_route: {
                operation_id: "move",
                destination_step_id: "step2",
                entry_identity: "entry:00000000000000000042",
                destination_session_id: "latest",
                phase: "committed",
              },
            },
          }),
        );
        expect(store.getState().tasks.activeSessionId).toBe("latest");
      }
    } finally {
      release();
    }
  },
);

it("fences pre-delete reads before notifying the successful-delete consumer", async () => {
  const store = withSiblings("CANCELLED");
  harness.store = store;
  harness.request.mockResolvedValue(undefined);
  vi.mocked(listTaskSessions).mockResolvedValue(membership("latest"));
  const reads = getTaskSessionReads(store);
  const release = reads.retain("t1");
  const oldRead = Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
  const retired = Promise.allSettled([reads.read("t1", () => oldRead.promise)]);
  let deletedSeen = false;
  const { result } = renderHook(() =>
    useSessionActions({
      taskId: "t1",
      sessionId: "old",
      onDeleted: () => {
        expect(store.getState().taskSessions.items.old).toBeUndefined();
        deletedSeen = true;
      },
    }),
  );
  try {
    await act(async () => {
      expect(await result.current.remove({ feedback: "inline" })).toBe(true);
    });
    oldRead.resolve(membership("old", "next", "latest"));
    expect((await retired)[0]).toMatchObject({
      status: "rejected",
      reason: { name: "AbortError" },
    });
    await flushFocusWork();
    expect(deletedSeen).toBe(true);
    expect(store.getState().tasks.activeSessionId).toBe("latest");
    expect(store.getState().taskSessions.items.old).toBeUndefined();
    expect(harness.toast).not.toHaveBeenCalled();
  } finally {
    release();
  }
});

it("clears a locally deleted last conversation without guessing a recipient", async () => {
  const store = withSiblings("CANCELLED");
  harness.store = store;
  store.getState().removeTaskSession("t1", "next");
  store.getState().removeTaskSession("t1", "latest");
  harness.request.mockResolvedValue(undefined);
  vi.mocked(listTaskSessions).mockResolvedValue({ sessions: [], total: 0 });
  const { result } = renderHook(() => useSessionActions({ taskId: "t1", sessionId: "old" }));
  await act(async () => {
    expect(await result.current.remove()).toBe(true);
  });
  await flushFocusWork();
  expect(store.getState().tasks.activeSessionId).toBeNull();
  expect(store.getState().taskSessionsByTask.itemsByTaskId.t1).toEqual([]);
  expect(store.getState().taskSessions.items.old).toBeUndefined();
});

it("preserves the conversation and reports one inline deletion failure", async () => {
  const store = withSiblings("CANCELLED");
  harness.store = store;
  harness.request.mockRejectedValue(new Error("connection lost"));
  let deletedSeen = false;
  const { result } = renderHook(() =>
    useSessionActions({
      taskId: "t1",
      sessionId: "old",
      onDeleted: () => {
        deletedSeen = true;
      },
    }),
  );
  await act(async () => {
    expect(await result.current.remove({ feedback: "inline" })).toBe(false);
  });
  expect(store.getState().tasks.activeSessionId).toBe("old");
  expect(store.getState().taskSessions.items.old.state).toBe("CANCELLED");
  expect(deletedSeen).toBe(false);
  expect(harness.toast).toHaveBeenCalledExactlyOnceWith(
    expect.objectContaining({
      variant: "error",
      description: "connection lost",
    }),
  );
  expect(harness.updateToast).not.toHaveBeenCalled();
});

it("retains an empty-slot continuation when the local ACK trails its removal event", async () => {
  const store = withSiblings("CANCELLED");
  harness.store = store;
  store.getState().removeTaskSession("t1", "next");
  const acknowledgement = Promise.withResolvers<unknown>();
  harness.request.mockReturnValue(acknowledgement.promise);
  vi.mocked(listTaskSessions)
    .mockResolvedValueOnce({
      sessions: [{ ...response("WAITING_FOR_INPUT", "latest").session, is_primary: false }],
      total: 1,
    })
    .mockResolvedValue(membership("latest"));
  const { result } = renderHook(() => useSessionActions({ taskId: "t1", sessionId: "old" }));
  let deleting!: Promise<boolean>;
  await act(async () => {
    deleting = result.current.remove({ feedback: "inline" });
  });
  registerTaskSessionHandlers(store)[REMOVED_ACTION]!({
    id: "local-removal-before-ack",
    type: "notification",
    action: REMOVED_ACTION,
    payload: { session_id: "old", task_id: "t1" },
  });
  await flushFocusWork();
  expect(store.getState().tasks.activeSessionId).toBeNull();
  expect(store.getState().taskSessions.items.old).toBeUndefined();
  await act(async () => {
    acknowledgement.resolve(undefined);
    expect(await deleting).toBe(true);
  });
  promote(store, "latest");
  await flushFocusWork();
  expect(store.getState().tasks.activeSessionId).toBe("latest");
  expect(store.getState().taskSessions.items.old).toBeUndefined();
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
it.each([
  ["event-first", "follow"],
  ["ack-first", "follow"],
  ["event-first", SURVIVING_PIN],
  ["ack-first", SURVIVING_PIN],
] as const)("retains a deleted tab selected after dispatch (%s, %s)", async (order, winner) => {
  const store = withSiblings("CANCELLED");
  harness.store = store;
  store.getState().setActiveTask("elsewhere");
  promote(store, "latest");
  store.getState().setActiveSessionAuto("t1", "next");
  const acknowledgement = Promise.withResolvers<unknown>();
  const proof = Promise.withResolvers<Awaited<ReturnType<typeof listTaskSessions>>>();
  harness.request.mockReturnValue(acknowledgement.promise);
  vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
  const { result } = renderHook(() => useSessionActions({ taskId: "t1", sessionId: "old" }));
  let deleting!: Promise<boolean>;
  await act(async () => {
    deleting = result.current.remove({ feedback: "inline" });
  });
  store.getState().setActiveSession("t1", "old");
  const removed = () =>
    registerTaskSessionHandlers(store)[REMOVED_ACTION]!({
      id: "selected-during-local-delete",
      type: "notification",
      action: REMOVED_ACTION,
      payload: { session_id: "old", task_id: "t1" },
    });
  if (order === "event-first") removed();
  if (winner === SURVIVING_PIN) store.getState().setActiveSession("t1", "next");
  await act(async () => {
    acknowledgement.resolve(undefined);
    expect(await deleting).toBe(true);
  });
  if (order === "ack-first") removed();
  proof.resolve(membership("latest", "next"));
  await flushFocusWork();
  expect(store.getState().taskSessions.items.old).toBeUndefined();
  expect(store.getState().tasks.activeSessionId).toBe(winner === "follow" ? "latest" : "next");
  if (winner === SURVIVING_PIN) expect(store.getState().tasks.pinnedSessionId).toBe("next");
});
