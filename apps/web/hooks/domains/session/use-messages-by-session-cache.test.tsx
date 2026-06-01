import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { qk } from "@/lib/query/keys";
import type { MessagesData } from "@/lib/query/query-options/session";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import {
  useMessagesBySessionFromCache,
  useStablePrimarySessionIds,
} from "./use-messages-by-session-cache";

function message(id: string, sessionId: string, overrides: Partial<Message> = {}): Message {
  return {
    id,
    session_id: toSessionId(sessionId),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "",
    type: "message",
    created_at: "2026-05-02T00:00:00Z",
    ...overrides,
  };
}

function seed(client: QueryClient, sessionId: string, messages: Message[]) {
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

describe("useMessagesBySessionFromCache", () => {
  it("returns an empty map when given no session IDs", () => {
    const client = createTestQueryClient();
    const { result } = renderHook(() => useMessagesBySessionFromCache([]), {
      wrapper: wrapper(client),
    });
    expect(result.current).toEqual({});
  });

  it("maps each session ID to its cached messages (multi-session combine)", () => {
    const client = createTestQueryClient();
    const aMsgs = [message("a1", "sess-a"), message("a2", "sess-a")];
    const bMsgs = [message("b1", "sess-b")];
    seed(client, "sess-a", aMsgs);
    seed(client, "sess-b", bMsgs);

    const { result } = renderHook(() => useMessagesBySessionFromCache(["sess-a", "sess-b"]), {
      wrapper: wrapper(client),
    });

    // Verifies the sessionIds[index] → result mapping is correct (no off-by-one).
    expect(result.current["sess-a"]).toEqual(aMsgs);
    expect(result.current["sess-b"]).toEqual(bMsgs);
  });

  it("omits sessions that have no cache entry", () => {
    const client = createTestQueryClient();
    seed(client, "sess-a", [message("a1", "sess-a")]);

    const { result } = renderHook(() => useMessagesBySessionFromCache(["sess-a", "sess-missing"]), {
      wrapper: wrapper(client),
    });

    expect(result.current["sess-a"]).toHaveLength(1);
    expect(result.current["sess-missing"]).toBeUndefined();
    expect(Object.keys(result.current)).toEqual(["sess-a"]);
  });

  it("does not fetch — sessions with no seeded cache stay absent (observe-only)", () => {
    const client = createTestQueryClient();
    const { result } = renderHook(() => useMessagesBySessionFromCache(["sess-x"]), {
      wrapper: wrapper(client),
    });
    // enabled:false ⇒ no queryFn ran ⇒ no cached data appears for sess-x.
    expect(result.current).toEqual({});
    expect(client.getQueryData(qk.session.messages("sess-x"))).toBeUndefined();
  });
});

describe("useStablePrimarySessionIds", () => {
  it("filters out null/undefined primary session IDs", () => {
    const tasks = [
      { primarySessionId: "s1" },
      { primarySessionId: null },
      { primarySessionId: undefined },
      { primarySessionId: "s2" },
    ];
    const { result } = renderHook(() => useStablePrimarySessionIds(tasks));
    expect(result.current).toEqual(["s1", "s2"]);
  });

  it("keeps a stable array reference when the ID contents are unchanged", () => {
    const { result, rerender } = renderHook(({ tasks }) => useStablePrimarySessionIds(tasks), {
      initialProps: { tasks: [{ primarySessionId: "s1" }, { primarySessionId: "s2" }] },
    });
    const first = result.current;
    // New array, new task objects, identical IDs ⇒ reference must not change.
    rerender({ tasks: [{ primarySessionId: "s1" }, { primarySessionId: "s2" }] });
    expect(result.current).toBe(first);
  });

  it("returns a new array when the ID contents change", () => {
    const { result, rerender } = renderHook(({ tasks }) => useStablePrimarySessionIds(tasks), {
      initialProps: { tasks: [{ primarySessionId: "s1" }] },
    });
    const first = result.current;
    rerender({ tasks: [{ primarySessionId: "s1" }, { primarySessionId: "s2" }] });
    expect(result.current).not.toBe(first);
    expect(result.current).toEqual(["s1", "s2"]);
  });
});
