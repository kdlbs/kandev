import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { useAutomaticRecoveryChatOwner } from "./use-automatic-recovery-chat-owner";

afterEach(cleanup);
const recovery = {
  requestIdentity: { taskId: "task", sessionId: "session", generation: 1, attemptId: 1 },
  resumptionState: "error" as const,
  error: "Recovery failed",
  notice: null,
  recoveryFailure: {
    outcome: "recovery_failed" as const,
    resumeError: "Resume failed",
    restoreError: "Restore failed",
  },
  resumeSession: vi.fn(),
};
function wrapperFor(state = "FAILED", extra = {}) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider
        initialState={
          {
            taskSessions: {
              items: { session: { id: "session", task_id: "task", state, ...extra } },
            },
          } as unknown as Partial<AppState>
        }
      >
        {children}
      </StateProvider>
    );
  };
}
const params = { taskId: "task", sessionId: "session", recovery };
describe("automatic recovery composer ownership", () => {
  it("claims a failed legacy session without bootstrap metadata", () => {
    const { result } = renderHook(() => useAutomaticRecoveryChatOwner(params), {
      wrapper: wrapperFor(),
    });
    expect(result.current).toBe(true);
  });
  it("claims interrupted waiting but not a usable session", () => {
    const blocked = renderHook(() => useAutomaticRecoveryChatOwner(params), {
      wrapper: wrapperFor("WAITING_FOR_INPUT", { error_message: "Interrupted" }),
    });
    expect(blocked.result.current).toBe(true);
    const healthy = renderHook(() => useAutomaticRecoveryChatOwner(params), {
      wrapper: wrapperFor("WAITING_FOR_INPUT"),
    });
    expect(healthy.result.current).toBe(false);
  });
  it.each([
    { disabled: true },
    { sessionId: null },
    { taskId: "other" },
    {
      recovery: { ...recovery, requestIdentity: null },
    },
    {
      recovery: {
        ...recovery,
        recoveryFailure: {
          outcome: "status_unavailable" as const,
          kind: "request" as const,
          statusError: "Offline",
        },
      },
    },
    {
      summary: {
        revision: 1,
        updated_at: "",
        task_error: {
          scope: "task" as const,
          stamp: "task-error",
          preview: "Workspace failed",
          occurred_at: "",
        },
      },
    },
  ])("keeps feedback in its fallback when no eligible composer exists: %j", (override) => {
    const { result } = renderHook(() => useAutomaticRecoveryChatOwner({ ...params, ...override }), {
      wrapper: wrapperFor(),
    });
    expect(result.current).toBe(false);
  });
  it("keeps passthrough feedback outside Chat", () => {
    const { result } = renderHook(() => useAutomaticRecoveryChatOwner(params), {
      wrapper: wrapperFor("FAILED", { is_passthrough: true }),
    });
    expect(result.current).toBe(false);
  });
});
