import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
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

describe("useTaskPendingClarification", () => {
  it("returns false when primarySessionId is null", () => {
    const { result } = renderHook(
      () => useTaskPendingClarification({ primarySessionId: null }),
      { wrapper: wrapper() },
    );

    expect(result.current).toBe(false);
  });

  it("returns false when the session has no messages in store and no snapshot flag", () => {
    const { result } = renderHook(
      () => useTaskPendingClarification({ primarySessionId: "session-1" }),
      { wrapper: wrapper() },
    );

    expect(result.current).toBe(false);
  });

  it("returns true when the session has a pending clarification", () => {
    const { result } = renderHook(
      () => useTaskPendingClarification({ primarySessionId: "session-1" }),
      {
        wrapper: wrapper({
          "session-1": [
            message({
              type: "clarification_request",
              metadata: { status: "pending" },
            }),
          ],
        }),
      },
    );

    expect(result.current).toBe(true);
  });

  it("falls back to the snapshot pending action when messages are not loaded", () => {
    const { result } = renderHook(
      () =>
        useTaskPendingClarification({
          primarySessionId: "session-1",
          primarySessionState: "WAITING_FOR_INPUT",
          primarySessionPendingAction: "clarification",
        }),
      { wrapper: wrapper() },
    );

    expect(result.current).toBe(true);
  });

  it("ignores the snapshot flag when the session is no longer WAITING_FOR_INPUT", () => {
    const { result } = renderHook(
      () =>
        useTaskPendingClarification({
          primarySessionId: "session-1",
          primarySessionState: "RUNNING",
          primarySessionPendingAction: "clarification",
        }),
      { wrapper: wrapper() },
    );

    expect(result.current).toBe(false);
  });

  it("prefers loaded messages over a stale snapshot flag", () => {
    const { result } = renderHook(
      () =>
        useTaskPendingClarification({
          primarySessionId: "session-1",
          primarySessionState: "WAITING_FOR_INPUT",
          primarySessionPendingAction: "clarification",
        }),
      {
        wrapper: wrapper({
          "session-1": [
            message({
              type: "clarification_request",
              metadata: { status: "answered" },
            }),
          ],
        }),
      },
    );

    expect(result.current).toBe(false);
  });
});
