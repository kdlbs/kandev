import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { useSessionPendingInput, useTaskPendingInput } from "./use-task-pending-input";

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

function wrapper(messagesBySession: Record<string, Message[]> = {}) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider
        initialState={{ messages: { bySession: messagesBySession, metaBySession: {} } }}
      >
        {children}
      </StateProvider>
    );
  };
}

describe("useTaskPendingInput", () => {
  it("returns both flags false when primarySessionId is null", () => {
    const { result } = renderHook(() => useTaskPendingInput(null), { wrapper: wrapper() });
    expect(result.current).toEqual({ clarification: false, permission: false });
  });

  it("derives clarification and permission from loaded messages", () => {
    const { result } = renderHook(() => useTaskPendingInput("session-1"), {
      wrapper: wrapper({
        "session-1": [message({ type: "permission_request", metadata: { status: "pending" } })],
      }),
    });
    expect(result.current).toEqual({ clarification: false, permission: true });
  });

  it("falls back to the snapshot pending action when messages are not loaded", () => {
    const { result } = renderHook(
      () =>
        useTaskPendingInput("session-1", {
          primarySessionState: "WAITING_FOR_INPUT",
          primarySessionPendingAction: "permission",
        }),
      { wrapper: wrapper() },
    );
    expect(result.current).toEqual({ clarification: false, permission: true });
  });

  it("prefers loaded (empty) messages over a stale snapshot", () => {
    const { result } = renderHook(
      () =>
        useTaskPendingInput("session-1", {
          primarySessionState: "WAITING_FOR_INPUT",
          primarySessionPendingAction: "clarification",
        }),
      { wrapper: wrapper({ "session-1": [] }) },
    );
    expect(result.current).toEqual({ clarification: false, permission: false });
  });
});

describe("useSessionPendingInput", () => {
  it("returns both flags false when sessionId is null", () => {
    const { result } = renderHook(() => useSessionPendingInput(null), { wrapper: wrapper() });
    expect(result.current).toEqual({ clarification: false, permission: false });
  });

  it("derives per-session clarification from loaded messages", () => {
    const { result } = renderHook(() => useSessionPendingInput("session-1"), {
      wrapper: wrapper({
        "session-1": [message({ type: "clarification_request", metadata: { status: "pending" } })],
      }),
    });
    expect(result.current).toEqual({ clarification: true, permission: false });
  });
});
