/* eslint-disable max-lines -- comprehensive session-state handler tests */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { registerTaskSessionHandlers } from "./agent-session";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSessionStateChangedPayload } from "@/lib/types/backend";

function makeStore(overrides: Record<string, unknown> = {}) {
  const state: Record<string, unknown> = {
    tasks: {
      activeTaskId: null,
      activeSessionId: null,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
    },
    taskSessions: { items: {} },
    taskSessionsByTask: { itemsByTaskId: {} },
    setTaskSession: vi.fn(),
    setTaskSessionsForTask: vi.fn(),
    upsertTaskSessionFromEvent: vi.fn(),
    setActiveSession: vi.fn(),
    setActiveSessionAuto: vi.fn(),
    setSessionAgentctlStatus: vi.fn(),
    setSessionFailureNotification: vi.fn(),
    setContextWindow: vi.fn(),
    ...overrides,
  };
  return {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

const STATE_CHANGED_EVENT = "session.state_changed";
const RECOVERABLE_ERROR_MESSAGE = "peer disconnected before response";
const RECOVERABLE_ERROR_AT = "2026-06-14T14:06:40Z";

function makeMessage(payload: TaskSessionStateChangedPayload) {
  return {
    id: "msg-1",
    type: "notification" as const,
    action: "session.state_changed" as const,
    payload,
  };
}

describe("session.state_changed handler", () => {
  let store: ReturnType<typeof makeStore>;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let handler: (msg: any) => void;

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("sets failure notification on first FAILED event", () => {
    store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "STARTING" } },
      },
    });
    handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-1",
        new_state: "FAILED",
        error_message: "container crashed",
      }),
    );

    expect(store.getState().setSessionFailureNotification).toHaveBeenCalledWith({
      sessionId: "s-1",
      taskId: "t-1",
      message: "container crashed",
    });
  });

  it("does not set failure notification when session is already FAILED", () => {
    store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "FAILED" } },
      },
    });
    handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-1",
        new_state: "FAILED",
        error_message: "container crashed",
      }),
    );

    expect(store.getState().setSessionFailureNotification).not.toHaveBeenCalled();
  });

  it("does not set failure notification for unknown session (snapshot replay)", () => {
    // When a session is replayed on reconnect/page-load, it lands in the FE
    // store for the first time already in FAILED state. This is not a real
    // transition we just observed, so no toast should fire.
    store = makeStore();
    handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-new",
        new_state: "FAILED",
        error_message: "timeout",
      }),
    );

    expect(store.getState().setSessionFailureNotification).not.toHaveBeenCalled();
  });

  it("respects suppress_toast flag", () => {
    store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "STARTING" } },
      },
    });
    handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-1",
        new_state: "FAILED",
        error_message: "missing branch",
        suppress_toast: true,
      }),
    );

    expect(store.getState().setSessionFailureNotification).not.toHaveBeenCalled();
  });
});

describe("session.state_changed name propagation", () => {
  let store: ReturnType<typeof makeStore>;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let handler: (msg: any) => void;

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("applies a session name from rename broadcasts, including clearing", () => {
    store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "RUNNING", started_at: "" } },
      },
    });
    handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({ task_id: "t-1", session_id: "s-1", new_state: "RUNNING", name: "reviewer" }),
    );
    expect(store.getState().upsertTaskSessionFromEvent).toHaveBeenCalledWith(
      "t-1",
      expect.objectContaining({ id: "s-1", name: "reviewer" }),
    );

    // Rename-to-clear carries name: "" and must still apply.
    handler(makeMessage({ task_id: "t-1", session_id: "s-1", new_state: "RUNNING", name: "" }));
    expect(store.getState().upsertTaskSessionFromEvent).toHaveBeenLastCalledWith(
      "t-1",
      expect.objectContaining({ id: "s-1", name: "" }),
    );
  });

  it("does not touch the name when the event omits it", () => {
    store = makeStore({
      taskSessions: {
        items: {
          "s-1": { id: "s-1", task_id: "t-1", state: "RUNNING", started_at: "", name: "reviewer" },
        },
      },
    });
    handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(makeMessage({ task_id: "t-1", session_id: "s-1", new_state: "COMPLETED" }));
    const call = vi.mocked(store.getState().upsertTaskSessionFromEvent).mock.calls.at(-1);
    expect(call?.[1]).not.toHaveProperty("name");
  });
});

describe("session.state_changed context window provenance", () => {
  it("retains the backend context-window source", () => {
    const setContextWindow = vi.fn();
    const store = makeStore({ setContextWindow });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-1",
        metadata: {
          context_window: {
            size: 258_400,
            used: 95_100,
            remaining: 163_300,
            efficiency: 36.8,
            source: "acp",
          },
        },
      }),
    );

    expect(setContextWindow).toHaveBeenCalledWith(
      "s-1",
      expect.objectContaining({ source: "acp" }),
    );
  });
});

describe("session.state_changed recoverable errors", () => {
  it("upserts recoverable error metadata for non-failed session states", () => {
    const upsertTaskSessionFromEvent = vi.fn();
    const store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "RUNNING" } },
      },
      upsertTaskSessionFromEvent,
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-1",
        new_state: "WAITING_FOR_INPUT",
        error_message: RECOVERABLE_ERROR_MESSAGE,
        session_metadata: {
          last_agent_error: {
            message: RECOVERABLE_ERROR_MESSAGE,
            occurred_at: RECOVERABLE_ERROR_AT,
          },
        },
      }),
    );

    expect(upsertTaskSessionFromEvent).toHaveBeenCalledWith(
      "t-1",
      expect.objectContaining({
        state: "WAITING_FOR_INPUT",
        error_message: RECOVERABLE_ERROR_MESSAGE,
        metadata: {
          last_agent_error: {
            message: RECOVERABLE_ERROR_MESSAGE,
            occurred_at: RECOVERABLE_ERROR_AT,
          },
        },
      }),
    );
  });
});

describe("session.state_changed stale guard", () => {
  let store: ReturnType<typeof makeStore>;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let handler: (msg: any) => void;

  it("ignores older state events before upserting the session", () => {
    const upsertTaskSessionFromEvent = vi.fn();
    store = makeStore({
      taskSessions: {
        items: {
          "s-1": {
            id: "s-1",
            task_id: "t-1",
            state: "WAITING_FOR_INPUT",
            updated_at: "2026-01-02T00:00:00.000Z",
          },
        },
      },
      upsertTaskSessionFromEvent,
    });
    handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler(
      makeMessage({
        task_id: "t-1",
        session_id: "s-1",
        new_state: "RUNNING",
        updated_at: "2026-01-01T00:00:00.000Z",
      }),
    );

    expect(upsertTaskSessionFromEvent).not.toHaveBeenCalled();
  });
});

describe("session.state_changed → active session switching", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("adopts a newly-created session for the active task", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: null,
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-new", new_state: "STARTING" },
    });

    expect(store.getState().setActiveSessionAuto).toHaveBeenCalledWith("t-1", "s-new");
    expect(store.getState().setActiveSession).not.toHaveBeenCalled();
  });

  it("does not adopt a new session for a task that is not active", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "other-task",
        activeSessionId: null,
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-new", new_state: "STARTING" },
    });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("does not adopt while the current active session is still running", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "RUNNING" } },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-new", new_state: "STARTING" },
    });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });
});

describe("session.state_changed → active session switching with pins", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("adopts the replacement when the pinned active session is already terminal", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: "s-old",
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "COMPLETED" } },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-new", new_state: "STARTING" },
    });

    expect(store.getState().setActiveSessionAuto).toHaveBeenCalledWith("t-1", "s-new");
  });

  it("does not adopt another session when a non-terminal pin was orphaned by active-session drift", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-drifted",
        pinnedSessionId: "s-pinned",
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: {
          "s-drifted": { id: "s-drifted", task_id: "t-1", state: "COMPLETED" },
          "s-pinned": { id: "s-pinned", task_id: "t-1", state: "RUNNING" },
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-background", new_state: "STARTING" },
    });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });
});

describe("session.state_changed → active session handoff on terminal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("hands off when the current active session transitions to terminal", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "RUNNING" } },
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-old", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
            { id: "s-new", task_id: "t-1", state: "STARTING", started_at: "", updated_at: "" },
          ],
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-old", new_state: "COMPLETED" },
    });

    expect(store.getState().setActiveSessionAuto).toHaveBeenCalledWith("t-1", "s-new");
    expect(store.getState().setActiveSession).not.toHaveBeenCalled();
  });

  // The per-task list here still shows s-old as RUNNING (pre-event state), so
  // pickReplacementSessionId returns s-old itself. This exercises the
  // `replacement !== sessionId` guard — without it, we'd set activeSessionId
  // to the same session that just became terminal.
  it("does not hand off when the only candidate is the terminating session itself", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "RUNNING" } },
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-old", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
          ],
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-old", new_state: "COMPLETED" },
    });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("does not hand off when all other sessions for the task are terminal", () => {
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "RUNNING" } },
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-done", task_id: "t-1", state: "COMPLETED", started_at: "", updated_at: "" },
            { id: "s-old", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
          ],
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-old", new_state: "COMPLETED" },
    });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });
});

describe("session.state_changed → respects user-pinned session", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("hands off when the pinned active session reaches a terminal state", () => {
    // Genuine RUNNING→COMPLETED transition: previousState is non-terminal,
    // so the workflow handoff should still fire even though the session
    // is pinned.
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: "s-old",
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "RUNNING" } },
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-old", task_id: "t-1", state: "RUNNING", started_at: "", updated_at: "" },
            { id: "s-new", task_id: "t-1", state: "STARTING", started_at: "", updated_at: "" },
          ],
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-old", new_state: "COMPLETED" },
    });

    expect(store.getState().setActiveSessionAuto).toHaveBeenCalledWith("t-1", "s-new");
    expect(store.getState().setActiveSession).not.toHaveBeenCalled();
  });

  it("hands off to the promoted primary when the new session is not yet in the per-task list", () => {
    // Race that produced the reported bug: a workflow step switch to a step
    // with a different agent profile promotes the new primary via task.updated
    // (reflected on the kanban task) BEFORE the old session's terminal
    // state_changed arrives, so s-new is not yet present in taskSessionsByTask.
    // pickReplacementSessionId finds no in-list successor, so focus must follow
    // the switch by falling back to the task's authoritative primary session.
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: "s-old",
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "RUNNING" } },
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-old", task_id: "t-1", state: "COMPLETED", started_at: "", updated_at: "" },
          ],
        },
      },
      kanban: { tasks: [{ id: "t-1", primarySessionId: "s-new" }] },
      kanbanMulti: { snapshots: {} },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-old", new_state: "COMPLETED" },
    });

    expect(store.getState().setActiveSessionAuto).toHaveBeenCalledWith("t-1", "s-new");
  });

  it("does not hand off when a pinned terminal session receives a replay state_changed", () => {
    // Replay: the session was already COMPLETED (previousState terminal) and
    // the backend re-emits the same terminal state. The user clicked this
    // session open to review it, so the pin must be honored — no handoff.
    const store = makeStore({
      tasks: {
        activeTaskId: "t-1",
        activeSessionId: "s-old",
        pinnedSessionId: "s-old",
        lastSessionByTaskId: {},
      },
      taskSessions: {
        items: { "s-old": { id: "s-old", task_id: "t-1", state: "COMPLETED" } },
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          "t-1": [
            { id: "s-old", task_id: "t-1", state: "COMPLETED", started_at: "", updated_at: "" },
            { id: "s-new", task_id: "t-1", state: "STARTING", started_at: "", updated_at: "" },
          ],
        },
      },
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      payload: { task_id: "t-1", session_id: "s-old", new_state: "COMPLETED" },
    });

    expect(store.getState().setActiveSessionAuto).not.toHaveBeenCalled();
  });
});

// eslint-disable-next-line max-lines-per-function -- test describe block, splitting hurts readability
describe("session.state_changed → agentctl ready fallback", () => {
  const TS = "2026-05-04T00:00:00Z";
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("promotes agentctl status to 'ready' when session enters RUNNING and ready event was missed", () => {
    const setSessionAgentctlStatus = vi.fn();
    const store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "STARTING" } },
      },
      sessionAgentctl: { itemsBySessionId: { "s-1": { status: "starting" } } },
      setSessionAgentctlStatus,
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      timestamp: TS,
      payload: { task_id: "t-1", session_id: "s-1", new_state: "RUNNING" },
    });

    expect(setSessionAgentctlStatus).toHaveBeenCalledWith(
      "s-1",
      expect.objectContaining({ status: "ready" }),
    );
  });

  it("promotes agentctl status to 'ready' on WAITING_FOR_INPUT even when no prior entry exists", () => {
    const setSessionAgentctlStatus = vi.fn();
    const store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "STARTING" } },
      },
      sessionAgentctl: { itemsBySessionId: {} },
      setSessionAgentctlStatus,
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      timestamp: TS,
      payload: { task_id: "t-1", session_id: "s-1", new_state: "WAITING_FOR_INPUT" },
    });

    expect(setSessionAgentctlStatus).toHaveBeenCalledWith(
      "s-1",
      expect.objectContaining({ status: "ready" }),
    );
  });

  it("does not re-set 'ready' when the session is already ready", () => {
    const setSessionAgentctlStatus = vi.fn();
    const store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "RUNNING" } },
      },
      sessionAgentctl: { itemsBySessionId: { "s-1": { status: "ready" } } },
      setSessionAgentctlStatus,
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    handler({
      id: "m",
      type: "notification",
      action: STATE_CHANGED_EVENT,
      timestamp: TS,
      payload: { task_id: "t-1", session_id: "s-1", new_state: "WAITING_FOR_INPUT" },
    });

    expect(setSessionAgentctlStatus).not.toHaveBeenCalled();
  });

  it("seeds env mapping and workspace path from agentctl_starting payload", () => {
    const upsertTaskSessionFromEvent = vi.fn();
    const store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "CREATED" } },
      },
      sessionAgentctl: { itemsBySessionId: {} },
      setSessionAgentctlStatus: vi.fn(),
      upsertTaskSessionFromEvent,
    });
    const handler = registerTaskSessionHandlers(store)["session.agentctl_starting"]!;

    handler({
      id: "m",
      type: "notification",
      action: "session.agentctl_starting",
      timestamp: TS,
      payload: {
        task_id: "t-1",
        session_id: "s-1",
        agent_execution_id: "ae-1",
        task_environment_id: "env-1",
        worktree_path: "/tmp/kandev/tasks/ws/task-1",
      },
    });

    expect(upsertTaskSessionFromEvent).toHaveBeenCalledWith(
      "t-1",
      expect.objectContaining({
        id: "s-1",
        task_environment_id: "env-1",
        worktree_path: "/tmp/kandev/tasks/ws/task-1",
      }),
    );
  });

  it("seeds env mapping from agentctl_ready payload", () => {
    const upsertTaskSessionFromEvent = vi.fn();
    const store = makeStore({
      taskSessions: { items: {} },
      sessionAgentctl: { itemsBySessionId: {} },
      setSessionAgentctlStatus: vi.fn(),
      upsertTaskSessionFromEvent,
      setWorktree: vi.fn(),
      sessionWorktreesBySessionId: { itemsBySessionId: {} },
      setSessionWorktrees: vi.fn(),
    });
    const handler = registerTaskSessionHandlers(store)["session.agentctl_ready"]!;

    handler({
      id: "m",
      type: "notification",
      action: "session.agentctl_ready",
      timestamp: TS,
      payload: {
        task_id: "t-1",
        session_id: "s-1",
        agent_execution_id: "ae-1",
        task_environment_id: "env-1",
      },
    });

    expect(upsertTaskSessionFromEvent).toHaveBeenCalledWith(
      "t-1",
      expect.objectContaining({ id: "s-1", task_environment_id: "env-1" }),
    );
  });

  it("does not call upsertTaskSessionFromEvent when agentctl payload omits task_environment_id", () => {
    const upsertTaskSessionFromEvent = vi.fn();
    const store = makeStore({
      taskSessions: { items: {} },
      sessionAgentctl: { itemsBySessionId: {} },
      setSessionAgentctlStatus: vi.fn(),
      upsertTaskSessionFromEvent,
    });
    const handler = registerTaskSessionHandlers(store)["session.agentctl_starting"]!;

    handler({
      id: "m",
      type: "notification",
      action: "session.agentctl_starting",
      timestamp: TS,
      payload: { task_id: "t-1", session_id: "s-1", agent_execution_id: "ae-1" },
    });

    expect(upsertTaskSessionFromEvent).not.toHaveBeenCalled();
  });

  it("does not promote on non-live states (STARTING, COMPLETED, FAILED)", () => {
    const setSessionAgentctlStatus = vi.fn();
    const store = makeStore({
      taskSessions: {
        items: { "s-1": { id: "s-1", task_id: "t-1", state: "CREATED" } },
      },
      sessionAgentctl: { itemsBySessionId: {} },
      setSessionAgentctlStatus,
    });
    const handler = registerTaskSessionHandlers(store)[STATE_CHANGED_EVENT]!;

    for (const newState of ["STARTING", "COMPLETED", "FAILED", "CANCELLED"]) {
      handler({
        id: "m",
        type: "notification",
        action: STATE_CHANGED_EVENT,
        timestamp: TS,
        payload: { task_id: "t-1", session_id: "s-1", new_state: newState },
      });
    }

    expect(setSessionAgentctlStatus).not.toHaveBeenCalled();
  });
});
