import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap, SessionInfoPayload } from "@/lib/types/backend";
import type { TaskSession } from "@/lib/types/http";
import { registerSessionInfoHandlers } from "./session-info";

const SESSION_STARTED_AT = "2026-06-11T00:00:00.000Z";
const SESSION_INFO_UPDATED_AT = "2026-06-11T00:01:00.000Z";
const SPARSE_SESSION_INFO_UPDATED_AT = "2026-06-11T00:03:00.000Z";
const STALE_SESSION_INFO_UPDATED_AT = "2026-06-11T00:00:30.000Z";
const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const ACP_SESSION_ID = "acp-session-1";
const SESSION_TITLE = "List files";

function makeSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    state: "running",
    metadata: { existing: true },
    started_at: SESSION_STARTED_AT,
    updated_at: SESSION_STARTED_AT,
    ...overrides,
  } as TaskSession;
}

function makeStore(overrides: Partial<AppState> = {}) {
  const state = {
    taskSessions: {
      items: {
        [SESSION_ID]: makeSession(),
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
    task_id: TASK_ID,
    session_id: SESSION_ID,
    agent_id: "agent-1",
    acp_session_id: ACP_SESSION_ID,
    session_title: SESSION_TITLE,
    session_updated_at: SESSION_INFO_UPDATED_AT,
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
          session_id: ACP_SESSION_ID,
          title: SESSION_TITLE,
          updated_at: SESSION_INFO_UPDATED_AT,
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
      expect.objectContaining({ updated_at: SESSION_STARTED_AT }),
    );
  });
});

describe("session.info_updated partial updates", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("preserves existing ACP fields on sparse updates", () => {
    const store = makeStore({
      taskSessions: {
        items: {
          [SESSION_ID]: makeSession({
            metadata: {
              acp: {
                session_id: ACP_SESSION_ID,
                title: SESSION_TITLE,
                updated_at: SESSION_INFO_UPDATED_AT,
                meta: { provider: "codex" },
              },
            },
          }),
        },
      },
    } as Partial<AppState>);
    const handler = registerSessionInfoHandlers(store)["session.info_updated"]!;

    handler(
      makeMessage(
        makePayload({
          acp_session_id: undefined,
          session_title: undefined,
          session_updated_at: SPARSE_SESSION_INFO_UPDATED_AT,
          session_meta: undefined,
        }),
      ),
    );

    expect(store.getState().setTaskSession).toHaveBeenCalledWith(
      expect.objectContaining({
        metadata: {
          acp: {
            session_id: ACP_SESSION_ID,
            title: SESSION_TITLE,
            updated_at: SPARSE_SESSION_INFO_UPDATED_AT,
            meta: { provider: "codex" },
          },
        },
      }),
    );
  });

  it("ignores stale updates when existing ACP metadata is newer", () => {
    const store = makeStore({
      taskSessions: {
        items: {
          [SESSION_ID]: makeSession({
            metadata: {
              acp: {
                session_id: ACP_SESSION_ID,
                title: SESSION_TITLE,
                updated_at: SESSION_INFO_UPDATED_AT,
                meta: { provider: "codex" },
              },
            },
          }),
        },
      },
    } as Partial<AppState>);
    const handler = registerSessionInfoHandlers(store)["session.info_updated"]!;

    handler(makeMessage(makePayload({ session_updated_at: STALE_SESSION_INFO_UPDATED_AT })));

    expect(store.getState().setTaskSession).not.toHaveBeenCalled();
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
