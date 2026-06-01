import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { qk } from "@/lib/query/keys";
import type { MessagesData } from "@/lib/query/query-options/session";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { useTaskPendingClarification } from "./use-task-pending-clarification";

function message(overrides: Partial<Message>): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "",
    type: "message",
    created_at: "2026-05-02T00:00:00Z",
    ...overrides,
  };
}

function seedMessages(client: QueryClient, sessionId: string, messages: Message[]) {
  client.setQueryData<MessagesData>(qk.session.messages(sessionId), {
    messages,
    hasMore: false,
    oldestCursor: null,
  });
}

function wrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("useTaskPendingClarification", () => {
  it("returns false when primarySessionId is null", () => {
    const client = createTestQueryClient();
    const { result } = renderHook(() => useTaskPendingClarification(null), {
      wrapper: wrapper(client),
    });

    expect(result.current).toBe(false);
  });

  it("returns false when the session has no messages in cache", () => {
    const client = createTestQueryClient();
    const { result } = renderHook(() => useTaskPendingClarification("session-1"), {
      wrapper: wrapper(client),
    });

    expect(result.current).toBe(false);
  });

  it("returns true when the session has a pending clarification in the query cache", () => {
    const client = createTestQueryClient();
    seedMessages(client, "session-1", [
      message({ type: "clarification_request", metadata: { status: "pending" } }),
    ]);
    const { result } = renderHook(() => useTaskPendingClarification("session-1"), {
      wrapper: wrapper(client),
    });

    expect(result.current).toBe(true);
  });
});
