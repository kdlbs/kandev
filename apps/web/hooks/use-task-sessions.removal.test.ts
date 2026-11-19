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
