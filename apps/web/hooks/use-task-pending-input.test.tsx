import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type TaskSession,
} from "@/lib/types/http";
import { useSessionPendingInput, useTaskPendingInput } from "./use-task-pending-input";

const PRIMARY_SESSION_ID = "session-primary";

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

function session(id: string, state: TaskSession["state"]): TaskSession {
  return {
    id: toSessionId(id),
    task_id: toTaskId("task-1"),
    state,
    started_at: "2026-05-02T00:00:00Z",
    updated_at: "2026-05-02T00:00:00Z",
  };
}

function wrapper(messagesBySession: Record<string, Message[]> = {}, sessions: TaskSession[] = []) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider
        initialState={{
          messages: { bySession: messagesBySession, metaBySession: {} },
          taskSessionsByTask: {
            itemsByTaskId: { "task-1": sessions },
            loadingByTaskId: {},
            loadedByTaskId: { "task-1": true },
          },
        }}
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

  it("uses the legacy primary-session snapshot while task session messages are unloaded", () => {
    const { result } = renderHook(
      () =>
        useTaskPendingInput(PRIMARY_SESSION_ID, {
          taskId: "task-1",
          primarySessionState: "WAITING_FOR_INPUT",
          primarySessionPendingAction: "permission",
        }),
      { wrapper: wrapper({}, [session(PRIMARY_SESSION_ID, "WAITING_FOR_INPUT")]) },
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

  it("finds pending input in a secondary input-capable session", () => {
    const { result } = renderHook(
      () =>
        useTaskPendingInput(PRIMARY_SESSION_ID, {
          taskId: "task-1",
          taskPendingAction: "clarification",
        }),
      {
        wrapper: wrapper(
          {
            [PRIMARY_SESSION_ID]: [],
            "session-secondary": [
              message({
                id: "secondary-permission",
                session_id: toSessionId("session-secondary"),
                type: "permission_request",
                metadata: { status: "pending" },
              }),
            ],
          },
          [
            session(PRIMARY_SESSION_ID, "RUNNING"),
            session("session-secondary", "WAITING_FOR_INPUT"),
          ],
        ),
      },
    );
    expect(result.current).toEqual({ clarification: false, permission: true });
  });

  it("excludes stale pending input from starting sessions", () => {
    const { result } = renderHook(
      () => useTaskPendingInput(PRIMARY_SESSION_ID, { taskId: "task-1" }),
      {
        wrapper: wrapper(
          {
            [PRIMARY_SESSION_ID]: [],
            "session-starting": [
              message({
                id: "stale-question",
                session_id: toSessionId("session-starting"),
                type: "clarification_request",
                metadata: { status: "pending" },
              }),
            ],
          },
          [session(PRIMARY_SESSION_ID, "RUNNING"), session("session-starting", "STARTING")],
        ),
      },
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
