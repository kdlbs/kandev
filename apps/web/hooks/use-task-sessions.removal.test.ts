import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useStore } from "zustand";
import type { AppState } from "@/lib/state/store";
import { listTaskSessions as foregroundList } from "@/lib/api";
import { fetchTaskSession, listTaskSessions as proofList } from "@/lib/api/domains/session-api";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { ApiError } from "@/lib/api/client";
import type { TaskSessionsResponse } from "@/lib/types/http";
import { getTaskSessionReads } from "@/lib/state/task-session-reads";
import {
  flushFocusWork,
  membership,
  promote,
  withSiblings,
} from "@/lib/ws/handlers/task-primary-focus.test-helpers";
import { useTaskSessions } from "./use-task-sessions";
import { useTaskRemoval } from "./use-task-removal";

let store: ReturnType<typeof withSiblings>;
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
  useAppStore: (selector: (state: AppState) => unknown) => useStore(store, selector),
}));
vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  fetchTaskSession: vi.fn(),
  listTaskSessions: vi.fn(),
}));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  listTaskSessions: vi.fn(),
}));

beforeEach(() => {
  store = withSiblings("COMPLETED");
  vi.mocked(foregroundList).mockReset();
  vi.mocked(proofList).mockReset();
  vi.mocked(fetchTaskSession).mockReset();
  Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// Hold publication after the real shared read has accepted its transport result.
function pauseAdmittedRead() {
  const reads = getTaskSessionReads(store);
  const accepted = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  const read = reads.read.bind(reads);
  vi.spyOn(reads, "read").mockImplementation((...args) =>
    read(...args).then(async (value) => {
      accepted.resolve();
      await release.promise;
      return value;
    }),
  );
  return { accepted: accepted.promise, release: () => release.resolve() };
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
describe("task membership publication after removal proof", () => {
  it.each(["proof-first", "old-list-first", "accepted-first", "dialog-first"])(
    "never resurrects the removed row with %s",
    async (order) => {
      const admitted = order === "accepted-first" ? pauseAdmittedRead() : undefined;
      const dialog =
        order === "dialog-first" ? renderHook(() => useTaskRemoval({ store })) : undefined;
      let superseded = false;
      const old = Promise.withResolvers<TaskSessionsResponse>();
      const proof = Promise.withResolvers<TaskSessionsResponse>();
      vi.mocked(foregroundList)
        .mockReturnValueOnce(old.promise)
        .mockResolvedValue(membership("next", "latest", "newer"));
      vi.mocked(fetchTaskSession).mockRejectedValue(new ApiError("", 404, null));
      vi.mocked(proofList).mockReturnValue(proof.promise);
      const hook = renderHook(() => useTaskSessions("t1"));
      let refresh!: Promise<unknown>;
      act(() => {
        refresh = dialog
          ? dialog.result.current
              .loadTaskSessionsForTask("t1", { force: true })
              .catch((error: unknown) => {
                if (!(error instanceof Error) || error.name !== "AbortError") throw error;
                superseded = true;
              })
          : hook.result.current.loadSessions(true);
      });
      await waitFor(() => expect(hook.result.current.isLoading).toBe(true));
      let removed = false;
      let resurrected = false;
      const stop = store.subscribe((state) => {
        if (!state.taskSessions.items.old) removed = true;
        else if (removed) resurrected = true;
      });
      try {
        act(() => promote(store));
        if (order === "old-list-first" || admitted) {
          await act(async () => {
            old.resolve(membership("old", "next", "latest"));
            if (admitted) await admitted.accepted;
            else await refresh;
          });
        }
        await act(async () => {
          proof.resolve(membership("next", "latest"));
          await flushFocusWork();
        });
        expect(store.getState().tasks.activeSessionId).toBe("next");
        expect(store.getState().taskSessions.items.old).toBeUndefined();
        if (order !== "old-list-first") {
          await act(async () => {
            if (admitted) admitted.release();
            else old.resolve(membership("old", "next", "latest"));
            await refresh;
          });
        }
        await waitFor(() => expect(hook.result.current.isLoading).toBe(false));
        await act(async () => {
          await hook.result.current.loadSessions(true);
        });
        expect(resurrected).toBe(false);
        if (dialog) expect(superseded).toBe(true);
        expect(store.getState().taskSessions.items.old).toBeUndefined();
        expect(hook.result.current.sessions.map((row) => row.id)).toEqual([
          "next",
          "latest",
          "newer",
        ]);
        expect(store.getState().taskSessions.items.newer?.task_id).toBe("t1");
        expect(store.getState().tasks.activeSessionId).toBe("next");
      } finally {
        admitted?.release();
        stop();
        hook.unmount();
        dialog?.unmount();
      }
    },
  );
});

it.each([
  "ordinary",
  "incomplete-first",
  "stale-first",
  "newer-pin",
  "newer-workflow",
  "scope-change",
])("settles an uncertain 404 removal on a fresh ordinary membership: %s", async (order) => {
  vi.mocked(fetchTaskSession).mockRejectedValue(new ApiError("", 404, null));
  vi.mocked(proofList).mockRejectedValue(new ApiError("unavailable", 503, null));
  vi.mocked(foregroundList).mockResolvedValue(membership("next", "latest"));
  const stale = Promise.withResolvers<TaskSessionsResponse>();
  const staleFirst = order === "stale-first";
  if (staleFirst) vi.mocked(foregroundList).mockReturnValueOnce(stale.promise);
  const hook = renderHook(() => useTaskSessions("t1"));
  let oldRead: Promise<void> | undefined;
  if (staleFirst) {
    act(() => {
      oldRead = hook.result.current.loadSessions(true);
    });
    await waitFor(() => expect(foregroundList).toHaveBeenCalledTimes(1));
  }
  act(() => promote(store));
  await waitFor(() => expect(proofList).toHaveBeenCalledTimes(1));
  await act(async () => flushFocusWork());
  expect(store.getState().tasks.activeSessionId).toBe("old");
  expect(store.getState().taskSessions.items.old).toBeDefined();
  if (staleFirst) {
    await act(async () => {
      stale.resolve(membership("next", "latest"));
      await oldRead;
    });
    expect(store.getState().taskSessions.items.old).toBeDefined();
    expect(store.getState().tasks.activeSessionId).toBe("old");
  }
  if (order === "scope-change")
    act(() =>
      store.setState({
        workspaceContextGeneration: store.getState().workspaceContextGeneration + 1,
      }),
    );
  if (order === "newer-workflow")
    act(() => {
      const requestId = store.getState().beginWorkflowSessionFocus({
        taskId: "t1",
        workflowId: "wf1",
        destinationStepId: "review",
        presentationToken: 1,
        navigationRevision: store.getState().taskRemoval.navigationRevision,
      })!;
      store.getState().bindWorkflowSessionFocus({
        requestId,
        presentationToken: 1,
        entryIdentity: "entry:fresh",
      });
      store.getState().reconcileWorkflowSessionFocus("t1", {
        workflowStepId: "review",
        updatedAt: "2026-11-21T12:00:00Z",
        entryIdentity: "entry:fresh",
        metadata: {
          workflow_session_route: {
            operation_id: "fresh",
            destination_step_id: "review",
            entry_identity: "entry:fresh",
            destination_session_id: "latest",
            phase: "committed",
          },
        },
      });
      expect(store.getState().tasks.activeSessionId).toBe("latest");
    });
  if (order === "newer-pin") act(() => store.getState().setActiveSession("t1", "latest"));
  if (order === "incomplete-first") {
    vi.mocked(foregroundList).mockResolvedValueOnce({ ...membership("next"), total: 10 });
    await act(async () => hook.result.current.loadSessions(true));
    expect(store.getState().tasks.activeSessionId).toBe("old");
    expect(store.getState().taskSessions.items.old).toBeDefined();
  }
  await act(async () => hook.result.current.loadSessions(true));
  if (order === "scope-change") {
    expect(store.getState().taskSessions.items.old).toBeDefined();
    expect(store.getState().tasks.activeSessionId).toBe("old");
    hook.unmount();
    return;
  }
  await waitFor(() => expect(store.getState().taskSessions.items.old).toBeUndefined());
  expect(store.getState().tasks.activeSessionId).toBe(
    order === "newer-pin" || order === "newer-workflow" ? "latest" : "next",
  );
  expect(proofList).toHaveBeenCalledTimes(1);
  hook.unmount();
});
