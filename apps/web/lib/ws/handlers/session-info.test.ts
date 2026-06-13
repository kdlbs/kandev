import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap, SessionInfoPayload } from "@/lib/types/backend";
import type { TaskSession } from "@/lib/types/http";
import { registerSessionInfoHandlers } from "./session-info";

function makeSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: "session-1",
    task_id: "task-1",
    state: "running",
    metadata: { existing: true },
    started_at: "2026-06-11T00:00:00.000Z",
    updated_at: "2026-06-11T00:00:00.000Z",
    ...overrides,
  } as TaskSession;
}

function makeStore(overrides: Partial<AppState> = {}) {
  const state = {
    taskSessions: {
      items: {
        "session-1": makeSession(),
      },
    },
    setTaskSession: vi.fn(),
    ...overrides,
  } as unknown as AppState;

  return {
    getState: () => state,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

function makePayload(overrides: Partial<SessionInfoPayload> = {}): SessionInfoPayload {
  return {
    task_id: "task-1",
    session_id: "session-1",
    agent_id: "agent-1",
    acp_session_id: "acp-session-1",
    session_title: "List files",
    session_updated_at: "2026-06-11T00:01:00.000Z",
    session_meta: { provider: "codex", nested: { value: true } },
    timestamp: "2026-06-11T00:02:00.000Z",
    ...overrides,
  };
}

function makeMessage(payload: SessionInfoPayload): BackendMessageMap["session.info_updated"] {
  return {
    id: "message-1",
    type: "notification",
    action: "session.info_updated",
    payload,
  };
}

describe("session.info_updated handler", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("merges ACP session info into existing session metadata", () => {
    const store = makeStore();
    const handler = registerSessionInfoHandlers(store)["session.info_updated"]!;

    handler(makeMessage(makePayload()));

    expect(store.getState().setTaskSession).toHaveBeenCalledWith({
      ...makeSession(),
      metadata: {
        existing: true,
        acp: {
          session_id: "acp-session-1",
          title: "List files",
          updated_at: "2026-06-11T00:01:00.000Z",
          meta: { provider: "codex", nested: { value: true } },
        },
      },
    });
  });

  it("does not overwrite the task session updated_at timestamp", () => {
    const store = makeStore();
    const handler = registerSessionInfoHandlers(store)["session.info_updated"]!;

    handler(makeMessage(makePayload()));

    expect(store.getState().setTaskSession).toHaveBeenCalledWith(
      expect.objectContaining({ updated_at: "2026-06-11T00:00:00.000Z" }),
    );
  });

  it("ignores updates for unknown sessions", () => {
    const store = makeStore({
      taskSessions: { items: {} },
    } as Partial<AppState>);
    const handler = registerSessionInfoHandlers(store)["session.info_updated"]!;

    handler(makeMessage(makePayload()));

    expect(store.getState().setTaskSession).not.toHaveBeenCalled();
  });

  it("ignores payloads without a session id", () => {
    const store = makeStore();
    const handler = registerSessionInfoHandlers(store)["session.info_updated"]!;

    handler(makeMessage(makePayload({ session_id: "" })));

    expect(store.getState().setTaskSession).not.toHaveBeenCalled();
  });
});
