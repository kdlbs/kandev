import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskSession } from "@/lib/types/http";

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const TIMESTAMP = "2026-06-24T00:00:00Z";
const mockListTaskSessions = vi.fn();

type MockState = {
  connection: { status: string };
  taskSessionsByTask: {
    itemsByTaskId: Record<string, TaskSession[]>;
    loadingByTaskId: Record<string, boolean>;
    loadedByTaskId: Record<string, boolean>;
  };
  setTaskSessionsForTask: ReturnType<typeof vi.fn>;
  setTaskSessionsLoading: ReturnType<typeof vi.fn>;
};

let mockState: MockState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  listTaskSessions: (...args: unknown[]) => mockListTaskSessions(...args),
}));

import { useTaskSessions } from "./use-task-sessions";

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
}

function wrapper(client = makeQueryClient()) {
  return function TestWrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client }, children);
  };
}

function makeSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    state: "WAITING_FOR_INPUT",
    started_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  } as TaskSession;
}

describe("useTaskSessions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockState = {
      connection: { status: "connected" },
      taskSessionsByTask: {
        itemsByTaskId: {},
        loadingByTaskId: {},
        loadedByTaskId: {},
      },
      setTaskSessionsForTask: vi.fn(),
      setTaskSessionsLoading: vi.fn(),
    };
  });

  it("mirrors query-owned task session metadata into the legacy session store", async () => {
    const session = makeSession({
      metadata: {
        last_agent_error: {
          message: "peer disconnected before response",
          occurred_at: TIMESTAMP,
        },
        plan_mode: true,
      },
    });
    mockListTaskSessions.mockResolvedValue({ sessions: [session] });

    const { result } = renderHook(() => useTaskSessions(TASK_ID), {
      wrapper: wrapper(),
    });

    await waitFor(() =>
      expect(mockState.setTaskSessionsForTask).toHaveBeenCalledWith(TASK_ID, [session]),
    );
    expect(result.current.sessions).toEqual([session]);
  });
});
