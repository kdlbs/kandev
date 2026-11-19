import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { registerTaskSessionHandlers } from "@/lib/ws/handlers/agent-session";
import {
  flushFocusWork,
  membership,
  promote,
  response,
  withSiblings,
} from "@/lib/ws/handlers/task-primary-focus.test-helpers";
import { useSessionActions } from "./use-session-actions";

const harness = vi.hoisted(() => ({ store: null as StoreApi<AppState> | null, request: vi.fn() }));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => harness.store,
  useAppStore: (selector: (state: AppState) => unknown) => selector(harness.store!.getState()),
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn(), updateToast: vi.fn() }),
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: harness.request }),
}));
vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  fetchTaskSession: vi.fn(),
  listTaskSessions: vi.fn(),
}));
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
  registerTaskSessionHandlers(store)["session.removed"]!({
    id: "removed-a",
    type: "notification",
    action: "session.removed",
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
