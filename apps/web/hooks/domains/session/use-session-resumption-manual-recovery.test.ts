import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { sessionId, taskId } from "@/lib/types/http";
import { useSessionResumption } from "./use-session-resumption";

const mocks = vi.hoisted(() => ({ request: vi.fn() }));
let store = createAppStore();

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.request }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: ReturnType<typeof store.getState>) => unknown) =>
    selector(store.getState()),
  useAppStoreApi: () => store,
}));

afterEach(() => {
  cleanup();
  mocks.request.mockReset();
  store = createAppStore();
});

function seedStoppedSession() {
  store.getState().setTaskSession({
    id: sessionId("session-1"),
    task_id: taskId("task-1"),
    state: "FAILED",
    started_at: "2026-10-10T08:00:00Z",
    updated_at: "2026-10-10T08:00:00Z",
  });
}

describe("manual Resume recovery", () => {
  // @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8
  it("uses explicit recovery and hydrates the recovered workspace", async () => {
    seedStoppedSession();
    mocks.request.mockImplementation(async (action: string) => {
      if (action === "session.launch")
        throw new Error("session recovery required: unknown_prompt_outcome");
      return {
        success: true,
        task_id: "task-1",
        session_id: "session-1",
        state: "WAITING_FOR_INPUT",
        worktree_path: "/workspace",
        worktree_branch: "main",
      };
    });
    const { result } = renderHook(() =>
      useSessionResumption("task-1", "session-1", false, {
        skipAutomaticRecovery: true,
      }),
    );

    let resumed = false;
    await act(async () => {
      resumed = await result.current.resumeSession();
    });

    expect(resumed).toBe(true);
    expect(mocks.request).toHaveBeenCalledExactlyOnceWith(
      "session.recover",
      { task_id: "task-1", session_id: "session-1", action: "resume" },
      60_000,
    );
    expect(result.current.worktreePath).toBe("/workspace");
    expect(result.current.worktreeBranch).toBe("main");
    expect(store.getState().taskSessions.items["session-1"].state).toBe("WAITING_FOR_INPUT");
  });

  it("does not apply a completed recovery to a different active session", async () => {
    seedStoppedSession();
    let resolve: (value: unknown) => void = () => {};
    mocks.request.mockReturnValue(
      new Promise((done) => {
        resolve = done;
      }),
    );
    const { result, rerender } = renderHook(
      ({ selectedSession }) =>
        useSessionResumption("task-1", selectedSession, false, {
          skipAutomaticRecovery: true,
        }),
      { initialProps: { selectedSession: "session-1" } },
    );
    let pending: Promise<boolean>;
    act(() => {
      pending = result.current.resumeSession();
    });
    rerender({ selectedSession: "session-2" });
    await act(async () => {
      resolve({
        success: true,
        task_id: "task-1",
        session_id: "session-1",
        state: "WAITING_FOR_INPUT",
      });
      expect(await pending!).toBe(false);
    });
    expect(store.getState().taskSessions.items["session-2"]).toBeUndefined();
    expect(result.current.worktreePath).toBeNull();
  });
});
