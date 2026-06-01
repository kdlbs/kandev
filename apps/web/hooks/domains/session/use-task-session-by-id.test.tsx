import { describe, expect, it, vi, beforeEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { qk } from "@/lib/query/keys";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type TaskSession,
  type TaskSessionState,
  type TaskSessionsResponse,
} from "@/lib/types/http";
import { listTaskSessions } from "@/lib/api/domains/session-api";
import {
  useAllTaskSessionsByTaskFromCache,
  useTaskSessionById,
  useTaskSessionsByTask,
} from "./use-task-session-by-id";

vi.mock("@/lib/api/domains/session-api", () => ({
  listTaskSessions: vi.fn(),
}));

const mockListTaskSessions = vi.mocked(listTaskSessions);

function session(
  id: string,
  task: string,
  state: TaskSessionState,
  overrides: Partial<TaskSession> = {},
): TaskSession {
  return {
    id: toSessionId(id),
    task_id: toTaskId(task),
    state,
    started_at: "2026-05-31T00:00:00Z",
    updated_at: "2026-05-31T00:00:00Z",
    is_primary: true,
    ...overrides,
  };
}

function seedByTask(client: QueryClient, task: string, sessions: TaskSession[]) {
  client.setQueryData<TaskSessionsResponse>(qk.taskSession.byTask(task), {
    sessions,
    total: sessions.length,
  });
}

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

beforeEach(() => {
  mockListTaskSessions.mockReset();
});

describe("useTaskSessionById", () => {
  it("returns the cached by-id record without fetching (observe-only)", () => {
    const client = createTestQueryClient();
    const s = session("sess-a", "task-1", "RUNNING");
    client.setQueryData<TaskSession>(qk.taskSession.byId("sess-a"), s);

    const { result } = renderHook(() => useTaskSessionById("sess-a"), {
      wrapper: wrapper(client),
    });
    expect(result.current).toEqual(s);
  });

  it("returns null for a null/empty session id", () => {
    const client = createTestQueryClient();
    const { result } = renderHook(() => useTaskSessionById(null), {
      wrapper: wrapper(client),
    });
    expect(result.current).toBeNull();
  });
});

describe("useAllTaskSessionsByTaskFromCache", () => {
  it("builds a { taskId: sessions[] } map from the byTask cache", () => {
    const client = createTestQueryClient();
    seedByTask(client, "task-1", [session("s1", "task-1", "RUNNING")]);
    seedByTask(client, "task-2", [session("s2", "task-2", "COMPLETED")]);

    const { result } = renderHook(() => useAllTaskSessionsByTaskFromCache(), {
      wrapper: wrapper(client),
    });
    expect(result.current["task-1"]?.[0]?.state).toBe("RUNNING");
    expect(result.current["task-2"]?.[0]?.state).toBe("COMPLETED");
  });

  // Regression: the snapshot signature must encode each session's mutable
  // `state` (not just its id). A live session.state_changed event mutates
  // `state` in place WITHOUT adding/removing a session id — an ids-only
  // signature froze the memoized snapshot, so the sidebar status badge never
  // re-rendered (background tasks stuck on their initial state).
  it("recomputes the snapshot when a session's state changes in place", () => {
    const client = createTestQueryClient();
    seedByTask(client, "task-1", [session("s1", "task-1", "RUNNING")]);

    const { result } = renderHook(() => useAllTaskSessionsByTaskFromCache(), {
      wrapper: wrapper(client),
    });
    expect(result.current["task-1"]?.[0]?.state).toBe("RUNNING");

    act(() => {
      seedByTask(client, "task-1", [session("s1", "task-1", "COMPLETED")]);
    });

    expect(result.current["task-1"]?.[0]?.state).toBe("COMPLETED");
  });

  it("recomputes the snapshot when a session's review_status changes in place", () => {
    const client = createTestQueryClient();
    seedByTask(client, "task-1", [
      session("s1", "task-1", "WAITING_FOR_INPUT", { review_status: "pending" }),
    ]);

    const { result } = renderHook(() => useAllTaskSessionsByTaskFromCache(), {
      wrapper: wrapper(client),
    });
    expect(result.current["task-1"]?.[0]?.review_status).toBe("pending");

    act(() => {
      seedByTask(client, "task-1", [
        session("s1", "task-1", "WAITING_FOR_INPUT", { review_status: "approved" }),
      ]);
    });

    expect(result.current["task-1"]?.[0]?.review_status).toBe("approved");
  });

  it("keeps a referentially stable snapshot when nothing relevant changed", () => {
    const client = createTestQueryClient();
    seedByTask(client, "task-1", [session("s1", "task-1", "RUNNING")]);

    const { result, rerender } = renderHook(() => useAllTaskSessionsByTaskFromCache(), {
      wrapper: wrapper(client),
    });
    const first = result.current;
    rerender();
    expect(result.current).toBe(first);
  });
});

describe("useTaskSessionsByTask by-id seeding", () => {
  // Regression for the Zustand→TQ session migration: the validation gate in
  // TaskPageContent (`useTaskSessionById(activeSessionId)?.task_id === activeTaskId`)
  // and useSessionAgent's `pickCurrentSession` both read the observe-only by-id
  // cache. Selecting a task whose sessions were only ever learned via the
  // by-task list fetch (e.g. a freshly-prepared session for an
  // MCP-created sessionless subtask) must seed each session into its by-id slot
  // — otherwise the gate resolves to null and the layout falls back to a stale
  // session. This locks in the seed-from-list path the migration introduced.
  it("seeds each fetched by-task session into its by-id slot", async () => {
    const client = createTestQueryClient();
    const fetched = [
      session("sess-x", "task-7", "WAITING_FOR_INPUT"),
      session("sess-y", "task-7", "RUNNING", { is_primary: false }),
    ];
    mockListTaskSessions.mockResolvedValue({ sessions: fetched, total: fetched.length });

    // Mount the by-task fetch alongside by-id observers for both sessions, just
    // like TaskPageContent (useSessionAgent fetches the list; the validation
    // gate + chat title observe individual ids). The observers keep the seeded
    // by-id entries alive (the test client uses gcTime: 0).
    const { result } = renderHook(
      () => ({
        list: useTaskSessionsByTask("task-7"),
        x: useTaskSessionById("sess-x"),
        y: useTaskSessionById("sess-y"),
      }),
      { wrapper: wrapper(client) },
    );

    await waitFor(() => expect(result.current.list.isLoaded).toBe(true));

    // Each session is now resolvable through the observe-only by-id surface,
    // so the TaskPageContent validation gate can match task_id === activeTaskId.
    // The seed runs in a post-commit effect, so wait for it to land.
    await waitFor(() => expect(result.current.x?.task_id).toBe("task-7"));
    expect(result.current.y?.task_id).toBe("task-7");
  });

  it("does not fetch or seed for a sessionless task (null taskId)", async () => {
    const client = createTestQueryClient();

    const { result } = renderHook(() => useTaskSessionsByTask(null), {
      wrapper: wrapper(client),
    });

    expect(result.current.sessions).toEqual([]);
    expect(result.current.isLoaded).toBe(false);
    expect(mockListTaskSessions).not.toHaveBeenCalled();
  });
});
