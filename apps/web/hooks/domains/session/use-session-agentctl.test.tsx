import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { SessionAgentctlStatus } from "@/lib/state/slices/session/types";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";
import { useSessionAgentctl } from "./use-session-agentctl";

const SESSION_ID = toSessionId("session-1");
const TASK_ID = toTaskId("task-1");
const TIMESTAMP = "2026-06-24T00:00:00Z";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
}

function taskSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    state: "COMPLETED",
    started_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    task_environment_id: "env-1",
    worktree_path: "/tmp/repo",
    ...overrides,
  };
}

function wrapperFor(session: TaskSession, agentctlStatus?: SessionAgentctlStatus) {
  const queryClient = createQueryClient();
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <StateProvider
          initialState={{
            taskSessions: { items: { [SESSION_ID]: session } },
            sessionAgentctl: {
              itemsBySessionId: agentctlStatus ? { [SESSION_ID]: agentctlStatus } : {},
            },
            environmentIdBySessionId: { [SESSION_ID]: "env-1" },
          }}
        >
          {children}
        </StateProvider>
      </QueryClientProvider>
    );
  };
}

describe("useSessionAgentctl", () => {
  it("treats a prepared terminal session as ready when the transient ready event was missed", () => {
    const { result } = renderHook(() => useSessionAgentctl(SESSION_ID), {
      wrapper: wrapperFor(taskSession()),
    });

    expect(result.current).toMatchObject({
      status: "ready",
      isReady: true,
      isStarting: false,
      isError: false,
    });
  });

  it("promotes a stale starting status after the session has completed with a workspace", () => {
    const { result } = renderHook(() => useSessionAgentctl(SESSION_ID), {
      wrapper: wrapperFor(taskSession(), {
        status: "starting",
        updatedAt: TIMESTAMP,
      }),
    });

    expect(result.current).toMatchObject({
      status: "ready",
      isReady: true,
      isStarting: false,
      isError: false,
    });
  });
});
