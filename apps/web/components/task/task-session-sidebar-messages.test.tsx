import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { taskId as toTaskId, type Message } from "@/lib/types/http";
import { useSidebarMessagesBySession } from "./task-session-sidebar-messages";

const mockListTaskSessionMessages = vi.fn();

vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: vi.fn(),
  getQueueStatus: vi.fn(),
  listSessionTurns: vi.fn(),
  listTaskSessionMessages: (...args: unknown[]) => mockListTaskSessionMessages(...args),
  listTaskSessions: vi.fn(),
  searchSessionMessages: vi.fn(),
}));

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  });
  return createElement(QueryClientProvider, { client }, children);
}

function makeMessage(sessionId: string): Message {
  return {
    id: `message-${sessionId}`,
    task_id: toTaskId("task-1"),
    session_id: sessionId,
    author_type: "agent",
    content: sessionId,
    type: "message",
    created_at: "2026-01-01T00:00:00Z",
  } as Message;
}

beforeEach(() => {
  mockListTaskSessionMessages.mockReset();
});

describe("useSidebarMessagesBySession", () => {
  it("does not pass Array.map indexes as message limits", async () => {
    mockListTaskSessionMessages.mockImplementation(async (sessionId: string) => ({
      messages: [makeMessage(sessionId)],
      has_more: false,
      cursor: null,
    }));

    renderHook(() => useSidebarMessagesBySession(["session-a", "session-b"]), { wrapper });

    await waitFor(() => expect(mockListTaskSessionMessages).toHaveBeenCalledTimes(2));
    expect(mockListTaskSessionMessages.mock.calls.map(([id, params]) => [id, params])).toEqual([
      ["session-a", { limit: 100, sort: "desc" }],
      ["session-b", { limit: 100, sort: "desc" }],
    ]);
  });
});
