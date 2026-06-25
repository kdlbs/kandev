import { describe, expect, it } from "vitest";
import type { BackendMessageMap, BackendMessageType } from "@/lib/types/backend";
import type { BackendMessage } from "@/lib/types/backend-message";
import type { TaskSession } from "@/lib/types/http";
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
      action: "session.state_changed",
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
      action: "session.state_changed",
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
      action: "session.state_changed",
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
