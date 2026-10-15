import type { ReactNode } from "react";
import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { sessionId, taskId, type TaskSession } from "@/lib/types/http";
import { SessionRecoveryProvider, useSessionComposerRecovery } from "./session-recovery-context";

vi.mock("../task-launch-error-context", () => ({
  useTaskLaunchErrorContext: () => ({
    automaticRecovery: {
      requestIdentity: { taskId: "task", sessionId: "session", generation: 1, attemptId: 1 },
      resumptionState: "error",
      error: "Session recovery failed",
    },
  }),
}));

const session: TaskSession = {
  id: sessionId("session"),
  task_id: taskId("task"),
  state: "WAITING_FOR_INPUT",
  started_at: "2026-10-15T10:00:00Z",
  updated_at: "2026-10-15T10:00:00Z",
};

function recoveryFor(current: TaskSession) {
  return renderHook(() => useSessionComposerRecovery("session"), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <SessionRecoveryProvider session={current} messages={[]} taskId="task" enabled>
        {children}
      </SessionRecoveryProvider>
    ),
  });
}

describe("recovery model provenance", () => {
  it("identifies automatic request failure independently from a persisted session failure", () => {
    const { result } = recoveryFor(session);
    expect(result.current?.model).toMatchObject({
      kind: "generic",
      summary: "Session recovery failed",
      requestLocal: true,
    });
  });

  it("keeps the persisted crash card authoritative even when automatic resume also fails", () => {
    const { result } = recoveryFor({
      ...session,
      state: "FAILED",
      error_message: "Agent process exited",
    });
    expect(result.current?.model?.summary).toBe("Agent process exited");
    expect(result.current?.model?.requestLocal).not.toBe(true);
  });
});
