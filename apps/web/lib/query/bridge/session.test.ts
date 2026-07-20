import { describe, expect, it } from "vitest";
import type { BackendMessageMap, BackendMessageType } from "@/lib/types/backend";
import type { BackendMessage } from "@/lib/types/backend-message";
import type { TaskPlan, TaskSession } from "@/lib/types/http";
import { agentProfileId } from "@/lib/types/ids";
import type { WebSocketClient } from "@/lib/ws/client";
import { makeQueryClient } from "../client";
import { qk } from "../keys";
import { registerSessionBridge } from "./session";

const TEST_SESSION_ID = "session-1";
const TEST_TASK_ID = "task-1";
const TEST_PROFILE_ID = agentProfileId("profile-1");
const TEST_STARTED_AT = "2026-06-24T00:00:00Z";
const TEST_UPDATED_AT = "2026-06-24T00:00:05Z";
const TEST_AGENT_NAME = "Codex";
const TEST_AGENT_ERROR = "peer disconnected before response";
const TEST_PLAN_ID = "plan-1";
const SESSION_STATE_CHANGED_ACTION = "session.state_changed";

type AnyBackendMessage = BackendMessage<string, Record<string, unknown>>;
type Handler = (message: AnyBackendMessage) => void;

class FakeWebSocketClient {
  private handlers = new Map<string, Set<Handler>>();

  on<T extends BackendMessageType>(type: T, handler: (message: BackendMessageMap[T]) => void) {
    const bucket = this.handlers.get(type) ?? new Set<Handler>();
    bucket.add(handler as Handler);
    this.handlers.set(type, bucket);
    return () => {
      bucket.delete(handler as Handler);
    };
  }

  emit(message: AnyBackendMessage) {
    this.handlers.get(message.action)?.forEach((handler) => handler(message));
  }
}

function setupBridge() {
  const ws = new FakeWebSocketClient();
  const queryClient = makeQueryClient();
  const registration = registerSessionBridge(ws as unknown as WebSocketClient, queryClient);
  return { ws, queryClient, cleanup: registration.cleanup };
}

function makeSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: TEST_SESSION_ID,
    task_id: TEST_TASK_ID,
    state: "STARTING",
    started_at: TEST_STARTED_AT,
    updated_at: TEST_STARTED_AT,
    ...overrides,
  } as TaskSession;
}

function makeTaskPlan(overrides: Partial<TaskPlan> = {}): TaskPlan {
  return {
    id: TEST_PLAN_ID,
    task_id: TEST_TASK_ID,
    title: "Plan",
    content: "# Plan",
    created_by: "agent",
    created_at: TEST_STARTED_AT,
    updated_at: TEST_UPDATED_AT,
    ...overrides,
  };
}

describe("session query bridge state events — identity", () => {
  it("preserves session identity fields when a partial state event patches the cache", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(
      qk.taskSession.byId(TEST_SESSION_ID),
      makeSession({
        agent_profile_id: TEST_PROFILE_ID,
        task_environment_id: "env-1",
        agent_profile_snapshot: { name: TEST_AGENT_NAME },
      }),
    );
    queryClient.setQueryData(qk.taskSession.byTask(TEST_TASK_ID), {
      sessions: [
        makeSession({
          agent_profile_id: TEST_PROFILE_ID,
          task_environment_id: "env-1",
          agent_profile_snapshot: { name: TEST_AGENT_NAME },
        }),
      ],
    });

    ws.emit({
      type: "notification",
      action: SESSION_STATE_CHANGED_ACTION,
      payload: {
        task_id: TEST_TASK_ID,
        session_id: TEST_SESSION_ID,
        new_state: "WAITING_FOR_INPUT",
        updated_at: TEST_UPDATED_AT,
      },
    });

    expect(queryClient.getQueryData(qk.taskSession.byId(TEST_SESSION_ID))).toMatchObject({
      state: "WAITING_FOR_INPUT",
      agent_profile_id: TEST_PROFILE_ID,
      task_environment_id: "env-1",
      agent_profile_snapshot: { name: TEST_AGENT_NAME },
    });
    expect(
      queryClient.getQueryData<{ sessions: TaskSession[] }>(qk.taskSession.byTask(TEST_TASK_ID)),
    ).toMatchObject({
      sessions: [
        {
          state: "WAITING_FOR_INPUT",
          agent_profile_id: TEST_PROFILE_ID,
          task_environment_id: "env-1",
          agent_profile_snapshot: { name: TEST_AGENT_NAME },
        },
      ],
    });

    cleanup();
  });

  it("upserts agent profile fields from state events before the HTTP session row loads", () => {
    const { ws, queryClient, cleanup } = setupBridge();

    ws.emit({
      type: "notification",
      action: SESSION_STATE_CHANGED_ACTION,
      payload: {
        task_id: TEST_TASK_ID,
        session_id: TEST_SESSION_ID,
        new_state: "RUNNING",
        agent_profile_id: TEST_PROFILE_ID,
        task_environment_id: "env-1",
        agent_profile_snapshot: { name: TEST_AGENT_NAME },
        updated_at: TEST_UPDATED_AT,
      },
    });

    expect(queryClient.getQueryData(qk.taskSession.byId(TEST_SESSION_ID))).toMatchObject({
      id: TEST_SESSION_ID,
      task_id: TEST_TASK_ID,
      state: "RUNNING",
      agent_profile_id: TEST_PROFILE_ID,
      task_environment_id: "env-1",
      agent_profile_snapshot: { name: TEST_AGENT_NAME },
    });
    expect(queryClient.getQueryState(qk.taskSession.byId(TEST_SESSION_ID))?.isInvalidated).toBe(
      true,
    );

    cleanup();
  });
});

describe("session query bridge state events — task list upserts", () => {
  it("inserts missing sessions into an already-cached by-task list", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(qk.taskSession.byTask(TEST_TASK_ID), {
      sessions: [makeSession({ id: "session-existing" as TaskSession["id"] })],
    });

    ws.emit({
      type: "notification",
      action: SESSION_STATE_CHANGED_ACTION,
      payload: {
        task_id: TEST_TASK_ID,
        session_id: TEST_SESSION_ID,
        new_state: "RUNNING",
        agent_profile_id: TEST_PROFILE_ID,
        updated_at: TEST_UPDATED_AT,
      },
    });

    expect(
      queryClient.getQueryData<{ sessions: TaskSession[] }>(qk.taskSession.byTask(TEST_TASK_ID)),
    ).toMatchObject({
      sessions: [
        { id: "session-existing" },
        {
          id: TEST_SESSION_ID,
          task_id: TEST_TASK_ID,
          state: "RUNNING",
          agent_profile_id: TEST_PROFILE_ID,
          started_at: TEST_UPDATED_AT,
          updated_at: TEST_UPDATED_AT,
        },
      ],
    });
    expect(queryClient.getQueryState(qk.taskSession.byTask(TEST_TASK_ID))?.isInvalidated).toBe(
      true,
    );

    cleanup();
  });
});

describe("session query bridge state events — metadata", () => {
  it("merges partial metadata updates without dropping existing session metadata", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(
      qk.taskSession.byId(TEST_SESSION_ID),
      makeSession({
        metadata: {
          last_agent_error: {
            message: TEST_AGENT_ERROR,
            occurred_at: TEST_STARTED_AT,
          },
        },
      }),
    );
    queryClient.setQueryData(qk.taskSession.byTask(TEST_TASK_ID), {
      sessions: [
        makeSession({
          metadata: {
            last_agent_error: {
              message: TEST_AGENT_ERROR,
              occurred_at: TEST_STARTED_AT,
            },
          },
        }),
      ],
    });

    ws.emit({
      type: "notification",
      action: SESSION_STATE_CHANGED_ACTION,
      payload: {
        task_id: TEST_TASK_ID,
        session_id: TEST_SESSION_ID,
        metadata: {
          context_window: { size: 256000, used: 1024, remaining: 254976, efficiency: 0.004 },
        },
        updated_at: TEST_UPDATED_AT,
      },
    });

    expect(queryClient.getQueryData(qk.taskSession.byId(TEST_SESSION_ID))).toMatchObject({
      metadata: {
        last_agent_error: {
          message: TEST_AGENT_ERROR,
          occurred_at: TEST_STARTED_AT,
        },
        context_window: { size: 256000 },
      },
    });
    expect(
      queryClient.getQueryData<{ sessions: TaskSession[] }>(qk.taskSession.byTask(TEST_TASK_ID)),
    ).toMatchObject({
      sessions: [
        {
          metadata: {
            last_agent_error: {
              message: TEST_AGENT_ERROR,
              occurred_at: TEST_STARTED_AT,
            },
            context_window: { size: 256000 },
          },
        },
      ],
    });

    cleanup();
  });
});

describe("session query bridge task-plan events", () => {
  it("upserts task plans into the query cache", () => {
    const { ws, queryClient, cleanup } = setupBridge();

    ws.emit({
      type: "notification",
      action: "task.plan.created",
      payload: makeTaskPlan({ content: "# Created" }) as unknown as Record<string, unknown>,
    });
    ws.emit({
      type: "notification",
      action: "task.plan.updated",
      payload: makeTaskPlan({ content: "# Updated" }) as unknown as Record<string, unknown>,
    });

    expect(queryClient.getQueryData(qk.taskPlan.detail(TEST_TASK_ID))).toMatchObject({
      id: TEST_PLAN_ID,
      task_id: TEST_TASK_ID,
      content: "# Updated",
    });

    cleanup();
  });

  it("preserves implementation markers from task plan updates", () => {
    const { ws, queryClient, cleanup } = setupBridge();

    ws.emit({
      type: "notification",
      action: "task.plan.updated",
      payload: makeTaskPlan({
        implementation_started_at: TEST_UPDATED_AT,
        implementation_started_session_id: TEST_SESSION_ID,
        implementation_started_by: "user",
      }) as unknown as Record<string, unknown>,
    });

    expect(queryClient.getQueryData(qk.taskPlan.detail(TEST_TASK_ID))).toMatchObject({
      implementation_started_at: TEST_UPDATED_AT,
      implementation_started_session_id: TEST_SESSION_ID,
      implementation_started_by: "user",
    });

    cleanup();
  });

  it("stores deleted task plans as null", () => {
    const { ws, queryClient, cleanup } = setupBridge();
    queryClient.setQueryData(qk.taskPlan.detail(TEST_TASK_ID), makeTaskPlan());

    ws.emit({
      type: "notification",
      action: "task.plan.deleted",
      payload: { task_id: TEST_TASK_ID },
    });

    expect(queryClient.getQueryData(qk.taskPlan.detail(TEST_TASK_ID))).toBeNull();

    cleanup();
  });
});
