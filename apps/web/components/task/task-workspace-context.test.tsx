import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import {
  TaskWorkspaceProvider,
  useTaskSessionId,
  useTaskWorkspaceId,
} from "./task-workspace-context";

afterEach(cleanup);

function Store({ children }: { children: ReactNode }) {
  return (
    <StateProvider
      initialState={{
        workspaces: { activeId: "alpha", items: [] },
        tasks: {
          activeTaskId: "ordinary",
          activeSessionId: "alpha-session",
          pinnedSessionId: null,
          lastSessionByTaskId: {},
          resumeSkippedSessionIds: {},
        },
      }}
    >
      {children}
    </StateProvider>
  );
}

function useScope(taskId: string | null) {
  return { workspace: useTaskWorkspaceId(taskId), session: useTaskSessionId(taskId ?? "") };
}

describe("original task VCS context (AC-UI-INTEGRATED-CONTROL-001.3)", () => {
  it("retains active workspace/session fallback for ordinary callers", () => {
    const { result } = renderHook(() => useScope("ordinary"), { wrapper: Store });
    expect(result.current).toEqual({ workspace: "alpha", session: "alpha-session" });
  });
  it.each(["embedded", "different", null])("isolates an embedded caller %s", (taskId) => {
    const { result } = renderHook(() => useScope(taskId), {
      wrapper: ({ children }) => (
        <Store>
          <TaskWorkspaceProvider taskId="embedded" workspaceId="beta" sessionId="beta-session">
            {children}
          </TaskWorkspaceProvider>
        </Store>
      ),
    });
    expect(result.current).toEqual(
      taskId === "embedded"
        ? { workspace: "beta", session: "beta-session" }
        : { workspace: null, session: null },
    );
  });
  it("never borrows the active session when the original task has no selected session", () => {
    const { result } = renderHook(() => useScope("embedded"), {
      wrapper: ({ children }) => (
        <Store>
          <TaskWorkspaceProvider taskId="embedded" workspaceId="beta">
            {children}
          </TaskWorkspaceProvider>
        </Store>
      ),
    });
    expect(result.current).toEqual({ workspace: "beta", session: null });
  });
});
