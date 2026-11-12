import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { AppState } from "@/lib/state/store";
import { sessionId, taskId } from "@/lib/types/http";
import { WebSocketRequestError } from "@/lib/ws/client";
import { useSessionRecoveryActions } from "./use-session-recovery-actions";
import { useSessionRestart } from "./use-session-restart";

const mockRequest = vi.fn();
const mockToast = vi.fn().mockReturnValue("restart-toast");
const mockUpdateToast = vi.fn();
let mockStore: ReturnType<typeof createAppStore>;

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => mockStore,
  useOptionalAppStoreApi: () => mockStore,
  useAppStore: (selector: (state: AppState) => unknown) => selector(mockStore.getState()),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast, updateToast: mockUpdateToast }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

// @covers AC-TASKS-SESSION-TAB-RESTART-001.2
describe("restart selection ownership", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockRequest.mockResolvedValue(undefined);
    mockStore = createAppStore();
    mockStore.getState().upsertTaskSessionFromEvent("t1", {
      id: sessionId("s1"),
      task_id: taskId("t1"),
      state: "RUNNING",
      started_at: "2026-10-20T00:00:00Z",
      updated_at: "2026-10-20T00:00:00Z",
    });
  });

  it("Restart publishes continuation guidance after its menu unmounts", async () => {
    const hint = new WebSocketRequestError("Saved history is available", "CONFLICT", {
      kind: "session_restore_required",
      recovery_action: "continue_from_history",
      reason: "unresolved_durable_work",
      session_id: "s1",
      generation: 2,
    });
    let rejectLaunch!: (error: Error) => void;
    mockRequest.mockImplementation((action: string) => {
      if (action === "session.stop") return Promise.resolve(undefined);
      return new Promise((_, reject) => {
        rejectLaunch = reject;
      });
    });
    const recovery = renderHook(() => useSessionRecoveryActions({ taskId: "t1", sessionId: "s1" }));
    const menu = renderHook(() => useSessionRestart("s1", "t1"));
    let restart!: Promise<boolean>;
    act(() => {
      restart = menu.result.current.restart();
    });
    await vi.waitFor(() => expect(rejectLaunch).toBeTypeOf("function"));

    menu.unmount();
    await act(async () => {
      rejectLaunch(hint);
      expect(await restart).toBe(false);
    });

    expect(recovery.result.current.continuationDetails).toMatchObject({
      kind: "session_restore_required",
      recovery_action: "continue_from_history",
      session_id: "s1",
      generation: 2,
    });
    expect(mockRequest).toHaveBeenCalledTimes(2);
    recovery.unmount();
  });

  it.each([
    { pinned: false, launchFails: false },
    { pinned: false, launchFails: true },
    { pinned: true, launchFails: false },
    { pinned: true, launchFails: true },
  ])(
    "restores selection ownership (pinned: $pinned, launch fails: $launchFails)",
    async ({ pinned, launchFails }) => {
      const initial = mockStore.getState();
      if (pinned) initial.setActiveSession("t1", "s1");
      else initial.setActiveSessionAuto("t1", "s1");
      const navigationRevision = mockStore.getState().taskRemoval.navigationRevision;
      let finishStop!: () => void;
      mockRequest.mockReturnValueOnce(
        new Promise<void>((resolve) => {
          finishStop = resolve;
        }),
      );
      if (launchFails) mockRequest.mockRejectedValueOnce(new Error("launch failed"));
      const { result } = renderHook(() => useSessionRestart("s1", "t1"));
      let pending!: Promise<boolean>;
      act(() => {
        pending = result.current.restart();
      });

      mockStore.setState((state) => ({ tasks: { ...state.tasks, pinnedSessionId: null } }));
      mockStore.getState().setActiveSessionAuto("t1", "s2");
      await act(async () => {
        finishStop();
        expect(await pending).toBe(!launchFails);
      });

      expect(mockStore.getState().tasks).toMatchObject({
        activeTaskId: "t1",
        activeSessionId: "s1",
        pinnedSessionId: pinned ? "s1" : null,
      });
      if (!pinned) {
        expect(mockStore.getState().taskRemoval.navigationRevision).toBe(navigationRevision);
      }
      expect(result.current.isRestarting).toBe(false);
    },
  );
});
