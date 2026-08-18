import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type TaskSession,
  type Turn,
} from "@/lib/types/http";
import { useSessionPendingInput, useTaskPendingInput } from "./use-task-pending-input";

const PRIMARY_SESSION_ID = "session-primary";
const SECONDARY_SESSION_ID = "session-secondary";
const BASE_TIMESTAMP = "2026-05-02T00:00:00Z";

function message(overrides: Partial<Message>): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "",
    type: "message",
    created_at: BASE_TIMESTAMP,
    ...overrides,
  };
}

function session(id: string, state: TaskSession["state"]): TaskSession {
  return {
    id: toSessionId(id),
    task_id: toTaskId("task-1"),
    state,
    started_at: BASE_TIMESTAMP,
    updated_at: BASE_TIMESTAMP,
  };
}

function sessionWithPendingAction(id: string, action: "clarification" | "permission"): TaskSession {
  return Object.assign(session(id, "WAITING_FOR_INPUT"), { pending_action: action });
}

function turn(id: string, sessionId: string, startedAt: string): Turn {
  return {
    id,
    session_id: toSessionId(sessionId),
    task_id: toTaskId("task-1"),
    started_at: startedAt,
    created_at: startedAt,
    updated_at: startedAt,
  };
}

function wrapper(
  messagesBySession: Record<string, Message[]> = {},
  sessions: TaskSession[] = [],
  turnsBySession: Record<string, Turn[]> = {},
) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider
        initialState={{
          messages: { bySession: messagesBySession, metaBySession: {} },
          turns: {
            bySession: turnsBySession,
            activeBySession: {},
            loadedBySession: {},
            reconcileEpochBySession: {},
            settledBoundaryBySession: {},
          },
          taskSessions: {
            items: Object.fromEntries(sessions.map((item) => [item.id, item])),
          },
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
            [SECONDARY_SESSION_ID]: [
              message({
                id: "secondary-permission",
                session_id: toSessionId(SECONDARY_SESSION_ID),
                type: "permission_request",
                metadata: { status: "pending" },
              }),
            ],
          },
          [
            session(PRIMARY_SESSION_ID, "RUNNING"),
            session(SECONDARY_SESSION_ID, "WAITING_FOR_INPUT"),
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

describe("useTaskPendingInput pending clarification turn authority", () => {
  it("preserves pending clarification while session state authority is still loading", () => {
    const olderTurn = turn("turn-older", PRIMARY_SESSION_ID, BASE_TIMESTAMP);
    const newestTurn = turn("turn-newest", PRIMARY_SESSION_ID, "2026-05-02T00:01:00Z");
    const { result } = renderHook(
      () => useTaskPendingInput(PRIMARY_SESSION_ID, { taskId: "task-1" }),
      {
        wrapper: wrapper(
          {
            [PRIMARY_SESSION_ID]: [
              message({
                id: "question-before-session-hydration",
                session_id: toSessionId(PRIMARY_SESSION_ID),
                turn_id: olderTurn.id,
                type: "clarification_request",
                metadata: { status: "pending" },
              }),
            ],
          },
          [],
          { [PRIMARY_SESSION_ID]: [olderTurn, newestTurn] },
        ),
      },
    );

    expect(result.current).toEqual({ clarification: true, permission: false });
  });

  it("ignores a detached pending clarification from an older durable turn", () => {
    const oldTurn = turn("turn-old", PRIMARY_SESSION_ID, BASE_TIMESTAMP);
    const currentTurn = turn("turn-current", PRIMARY_SESSION_ID, "2026-05-02T00:01:00Z");
    const { result } = renderHook(
      () => useTaskPendingInput(PRIMARY_SESSION_ID, { taskId: "task-1" }),
      {
        wrapper: wrapper(
          {
            [PRIMARY_SESSION_ID]: [
              message({
                id: "detached-old-question",
                session_id: toSessionId(PRIMARY_SESSION_ID),
                turn_id: oldTurn.id,
                type: "clarification_request",
                metadata: { status: "pending", agent_disconnected: true },
              }),
              message({
                id: "newer-turn-message",
                session_id: toSessionId(PRIMARY_SESSION_ID),
                turn_id: currentTurn.id,
              }),
            ],
          },
          [session(PRIMARY_SESSION_ID, "RUNNING")],
          { [PRIMARY_SESSION_ID]: [oldTurn, currentTurn] },
        ),
      },
    );

    expect(result.current).toEqual({ clarification: false, permission: false });
  });
});

describe("useTaskPendingInput clean clarification turn authority", () => {
  it("suppresses a visible predecessor when the session projection is explicitly clean", () => {
    const visiblePredecessor = turn("turn-visible-predecessor", PRIMARY_SESSION_ID, BASE_TIMESTAMP);
    const cleanSession = Object.assign(session(PRIMARY_SESSION_ID, "RUNNING"), {
      pending_action: null,
    });
    const { result } = renderHook(
      () => useTaskPendingInput(PRIMARY_SESSION_ID, { taskId: "task-1" }),
      {
        wrapper: wrapper(
          {
            [PRIMARY_SESSION_ID]: [
              message({
                id: "stale-predecessor-question",
                session_id: toSessionId(PRIMARY_SESSION_ID),
                turn_id: visiblePredecessor.id,
                type: "clarification_request",
                metadata: { status: "pending" },
              }),
            ],
          },
          [cleanSession],
          { [PRIMARY_SESSION_ID]: [visiblePredecessor] },
        ),
      },
    );

    expect(result.current).toEqual({ clarification: false, permission: false });
  });

  it("quarantines pending clarification history for a terminal primary session", () => {
    const completedTurn = turn("turn-completed", PRIMARY_SESSION_ID, BASE_TIMESTAMP);
    const { result } = renderHook(
      () =>
        useTaskPendingInput(PRIMARY_SESSION_ID, {
          primarySessionState: "COMPLETED",
          primarySessionPendingAction: "clarification",
        }),
      {
        wrapper: wrapper(
          {
            [PRIMARY_SESSION_ID]: [
              message({
                id: "stale-completed-question",
                session_id: toSessionId(PRIMARY_SESSION_ID),
                turn_id: completedTurn.id,
                type: "clarification_request",
                metadata: { status: "pending" },
              }),
            ],
          },
          [],
          { [PRIMARY_SESSION_ID]: [completedTurn] },
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

  it("uses a per-session pending-action snapshot when messages are unloaded", () => {
    const { result } = renderHook(() => useSessionPendingInput(SECONDARY_SESSION_ID), {
      wrapper: wrapper({}, [sessionWithPendingAction(SECONDARY_SESSION_ID, "clarification")]),
    });
    expect(result.current).toEqual({ clarification: true, permission: false });
  });

  it("prefers loaded messages over a stale per-session pending-action snapshot", () => {
    const { result } = renderHook(() => useSessionPendingInput(SECONDARY_SESSION_ID), {
      wrapper: wrapper({ [SECONDARY_SESSION_ID]: [] }, [
        sessionWithPendingAction(SECONDARY_SESSION_ID, "permission"),
      ]),
    });
    expect(result.current).toEqual({ clarification: false, permission: false });
  });

  it("quarantines pending clarification history for a cancelled session", () => {
    const cancelledTurn = turn("turn-cancelled", SECONDARY_SESSION_ID, BASE_TIMESTAMP);
    const { result } = renderHook(() => useSessionPendingInput(SECONDARY_SESSION_ID), {
      wrapper: wrapper(
        {
          [SECONDARY_SESSION_ID]: [
            message({
              id: "stale-cancelled-question",
              session_id: toSessionId(SECONDARY_SESSION_ID),
              turn_id: cancelledTurn.id,
              type: "clarification_request",
              metadata: { status: "pending" },
            }),
          ],
        },
        [
          Object.assign(session(SECONDARY_SESSION_ID, "CANCELLED"), {
            pending_action: "clarification" as const,
          }),
        ],
        { [SECONDARY_SESSION_ID]: [cancelledTurn] },
      ),
    });

    expect(result.current).toEqual({ clarification: false, permission: false });
  });
});
