import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { WebSocketRequestError } from "@/lib/ws/request-error";
import { sendMessageRequest, useMessageHandler } from "./use-message-handler";

const getWebSocketClientMock = vi.hoisted(() => vi.fn());
const listTaskSessionsMock = vi.hoisted(() => vi.fn());
const queueMock = vi.hoisted(() => vi.fn());
const addMessageMock = vi.hoisted(() => vi.fn());
const setTaskPlanCommentsMock = vi.hoisted(() => vi.fn());
const getTaskPlanCommentsMock = vi.hoisted(() => vi.fn());
const storeState = vi.hoisted(() => ({
  current: {
    taskSessions: { items: {} as Record<string, unknown> },
    queue: { metaBySessionId: {} as Record<string, { count: number }> },
    addMessage: addMessageMock,
    setTaskPlanComments: setTaskPlanCommentsMock,
  },
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: getWebSocketClientMock,
}));
vi.mock("@/lib/api/domains/session-api", () => ({
  listTaskSessions: listTaskSessionsMock,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => storeState.current }),
}));
vi.mock("@/lib/api/domains/plan-comment-api", () => ({
  getTaskPlanComments: getTaskPlanCommentsMock,
}));
vi.mock("./domains/session/use-queue", () => ({
  useQueue: () => ({ queue: queueMock }),
}));

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const COMMENT_ID = "comment-1";
const MESSAGE_ADD_ACTION = "message.add";

function selectedSession(state: string, foregroundActivity?: string) {
  storeState.current.taskSessions.items = {
    [SESSION_ID]: { state, foreground_activity: foregroundActivity },
  };
}

function renderMessageHandler() {
  return renderHook(() =>
    useMessageHandler({
      resolvedSessionId: SESSION_ID,
      taskId: TASK_ID,
      sessionModel: null,
      activeModel: null,
    }),
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  queueMock.mockResolvedValue(true);
  storeState.current.taskSessions.items = {};
  storeState.current.queue.metaBySessionId = {};
  getWebSocketClientMock.mockReturnValue({ request: vi.fn().mockResolvedValue(undefined) });
  listTaskSessionsMock.mockResolvedValue({ sessions: [{ id: SESSION_ID }], total: 1 });
  getTaskPlanCommentsMock.mockResolvedValue({
    task_id: TASK_ID,
    plan_id: "plan-1",
    revision: 3,
    comments: [],
  });
});

describe("sendMessageRequest task plan comments", () => {
  it("forwards task plan comment references and the primary guard", async () => {
    const request = vi.fn().mockResolvedValue(undefined);
    getWebSocketClientMock.mockReturnValue({ request });

    await sendMessageRequest({
      taskId: TASK_ID,
      resolvedSessionId: SESSION_ID,
      finalMessage: "",
      modelToSend: undefined,
      planMode: true,
      planCommentRefs: [{ id: COMMENT_ID, version: 4 }],
      requirePrimarySession: true,
    });

    expect(request).toHaveBeenCalledWith(
      MESSAGE_ADD_ACTION,
      expect.objectContaining({
        content: "",
        plan_mode: true,
        plan_comment_refs: [{ id: COMMENT_ID, version: 4 }],
        require_primary_session: true,
      }),
      10000,
    );
  });
});

describe("sendMessageRequest routed reconciliation", () => {
  it("finds a committed message after turn-start routing changes the session", async () => {
    const replacementSessionId = "session-routed";
    const committed = {
      id: "routed-message",
      session_id: replacementSessionId,
      task_id: TASK_ID,
      author_type: "user" as const,
      content: "retryable",
      type: "message" as const,
      created_at: "2026-08-01T18:00:00Z",
    };
    listTaskSessionsMock.mockResolvedValueOnce({
      sessions: [{ id: SESSION_ID }, { id: replacementSessionId }],
      total: 2,
    });
    const request = vi.fn(async (action: string, payload: { session_id?: string }) => {
      if (action === MESSAGE_ADD_ACTION) {
        throw new Error("WebSocket request timed out: message.add");
      }
      return payload.session_id === replacementSessionId
        ? { messages: [committed] }
        : { messages: [] };
    });
    getWebSocketClientMock.mockReturnValue({ request, getStatus: () => "connected" });

    await expect(
      sendMessageRequest({
        taskId: TASK_ID,
        resolvedSessionId: SESSION_ID,
        clientMessageId: committed.id,
        finalMessage: committed.content,
        modelToSend: undefined,
        planMode: false,
      }),
    ).resolves.toEqual(committed);
    expect(request).toHaveBeenCalledWith(
      "message.list",
      { session_id: replacementSessionId, limit: 100, sort: "desc" },
      5000,
    );
  });
});

describe("useMessageHandler queued plan comments", () => {
  it("returns false for a queue no-op so the composer keeps its draft", async () => {
    queueMock.mockResolvedValue(false);
    const request = vi.fn();
    getWebSocketClientMock.mockReturnValue({ request });
    selectedSession("WAITING_FOR_INPUT");
    const { result } = renderHook(() =>
      useMessageHandler({
        resolvedSessionId: SESSION_ID,
        taskId: TASK_ID,
        sessionModel: null,
        activeModel: null,
        hasPendingClarification: true,
      }),
    );

    await expect(result.current.handleSendMessage({ message: "Do not clear me" })).resolves.toBe(
      false,
    );
    expect(request).not.toHaveBeenCalled();
  });

  it("queues displayed task plan references as one idempotent admission", async () => {
    selectedSession("RUNNING", "generating");
    const { result } = renderMessageHandler();

    await act(async () => {
      await result.current.handleSendMessage({
        message: "Use this feedback",
        planCommentRefs: [{ id: COMMENT_ID, version: 2 }],
      } as never);
    });

    expect(queueMock).toHaveBeenCalledWith(
      expect.objectContaining({
        taskId: TASK_ID,
        content: "Use this feedback",
        clientQueueId: expect.any(String),
        planCommentRefs: [{ id: COMMENT_ID, version: 2 }],
      }),
    );
  });

  it("queues behind existing prompts for a steer-capable running session", async () => {
    selectedSession("RUNNING", "generating");
    Object.assign(storeState.current.taskSessions.items[SESSION_ID] as object, {
      supports_steering: true,
    });
    storeState.current.queue.metaBySessionId[SESSION_ID] = { count: 1 };
    const { result } = renderMessageHandler();

    await result.current.handleSendMessage({
      message: "Use this feedback",
      planCommentRefs: [{ id: COMMENT_ID, version: 2 }],
    } as never);

    expect(queueMock).toHaveBeenCalled();
    expect(getWebSocketClientMock().request).not.toHaveBeenCalled();
  });
});

describe("useMessageHandler plan comment retries", () => {
  it("reuses an unresolved queue admission ID on manual retry", async () => {
    selectedSession("RUNNING", "generating");
    queueMock
      .mockRejectedValueOnce(new Error("WebSocket request timed out: message.queue.add"))
      .mockResolvedValueOnce(true);
    const { result } = renderMessageHandler();
    const payload = {
      message: "Retry this feedback",
      planCommentRefs: [{ id: COMMENT_ID, version: 2 }],
    } as never;

    await expect(result.current.handleSendMessage(payload)).rejects.toThrow(
      "WebSocket request timed out",
    );
    await expect(result.current.handleSendMessage(payload)).resolves.toBeUndefined();

    const firstID = queueMock.mock.calls[0]?.[0]?.clientQueueId;
    const secondID = queueMock.mock.calls[1]?.[0]?.clientQueueId;
    expect(firstID).toEqual(expect.any(String));
    expect(secondID).toBe(firstID);
  });

  it("reconciles an accepted admission before retrying in a new input mode", async () => {
    selectedSession("RUNNING", "generating");
    queueMock.mockRejectedValueOnce(new Error("WebSocket request timed out: message.queue.add"));
    const { result } = renderMessageHandler();
    const payload = {
      message: "Retry after ready",
      planCommentRefs: [{ id: COMMENT_ID, version: 2 }],
    } as never;

    await expect(result.current.handleSendMessage(payload)).rejects.toThrow(
      "WebSocket request timed out",
    );
    const admissionID = queueMock.mock.calls[0]?.[0]?.clientQueueId;
    const committed = {
      id: admissionID,
      session_id: SESSION_ID,
      task_id: TASK_ID,
      author_type: "user" as const,
      content: "Retry after ready",
      type: "message" as const,
      created_at: "2026-08-01T18:00:00Z",
    };
    const request = vi.fn(async (action: string) =>
      action === "message.list" ? { messages: [committed] } : undefined,
    );
    getWebSocketClientMock.mockReturnValue({ request });
    selectedSession("WAITING_FOR_INPUT");

    await expect(result.current.handleSendMessage(payload)).resolves.toBeUndefined();

    expect(queueMock).toHaveBeenCalledTimes(1);
    expect(request).not.toHaveBeenCalledWith(
      MESSAGE_ADD_ACTION,
      expect.anything(),
      expect.anything(),
    );
    expect(addMessageMock).toHaveBeenCalledWith(committed);
  });
});

describe("useMessageHandler direct plan comments", () => {
  it("sends refs without duplicating their text in active-plan context", async () => {
    selectedSession("WAITING_FOR_INPUT");
    const request = vi.fn().mockResolvedValue(undefined);
    getWebSocketClientMock.mockReturnValue({ request });
    const { result } = renderHook(() =>
      useMessageHandler({
        resolvedSessionId: SESSION_ID,
        taskId: TASK_ID,
        sessionModel: null,
        activeModel: null,
        planModeEnabled: true,
        activeDocument: { type: "plan", taskId: TASK_ID } as never,
        planComments: [{ id: COMMENT_ID, version: 2, text: "Do not duplicate this text" } as never],
      }),
    );

    await act(async () => {
      await result.current.handleSendMessage({
        message: "Continue",
        planCommentRefs: [{ id: COMMENT_ID, version: 2 }],
      } as never);
    });

    const sent = request.mock.calls[0]?.[1] as Record<string, unknown>;
    expect(sent.content).not.toContain("Do not duplicate this text");
    expect(sent.plan_comment_refs).toEqual([{ id: COMMENT_ID, version: 2 }]);
  });

  it("reconciles a stale snapshot and returns a deterministic error", async () => {
    selectedSession("WAITING_FOR_INPUT");
    const snapshot = { task_id: TASK_ID, plan_id: "plan-1", revision: 7, comments: [] };
    const request = vi.fn().mockRejectedValue(
      new WebSocketRequestError("Task plan comments changed", "plan_comments_changed", {
        snapshot,
      }),
    );
    getWebSocketClientMock.mockReturnValue({ request });
    const { result } = renderMessageHandler();

    await expect(
      result.current.handleSendMessage({
        message: "Keep my draft",
        planCommentRefs: [{ id: COMMENT_ID, version: 1 }],
      } as never),
    ).rejects.toMatchObject({ code: "plan-comments-changed" });

    expect(setTaskPlanCommentsMock).toHaveBeenCalledWith(TASK_ID, snapshot);
  });

  it("keeps the admission ID when reconciliation reports a comment conflict", async () => {
    selectedSession("WAITING_FOR_INPUT");
    const request = vi
      .fn()
      .mockRejectedValueOnce(
        new WebSocketRequestError("Task plan comments changed", "plan_comments_changed", {
          snapshot: { task_id: TASK_ID, plan_id: "plan-1", revision: 7, comments: [] },
        }),
      )
      .mockResolvedValueOnce(undefined);
    getWebSocketClientMock.mockReturnValue({ request });
    const { result } = renderMessageHandler();
    const payload = {
      message: "Keep one admission",
      planCommentRefs: [{ id: COMMENT_ID, version: 2 }],
    } as never;

    await expect(result.current.handleSendMessage(payload)).rejects.toMatchObject({
      code: "plan-comments-changed",
    });
    await expect(result.current.handleSendMessage(payload)).resolves.toBeUndefined();

    const admissions = request.mock.calls.filter(([action]) => action === MESSAGE_ADD_ACTION);
    expect(admissions[1]?.[1]?.client_message_id).toBe(admissions[0]?.[1]?.client_message_id);
  });

  it("uses a new admission ID when review-comment presence changes", async () => {
    selectedSession("WAITING_FOR_INPUT");
    const request = vi
      .fn()
      .mockRejectedValueOnce(new Error("rejected"))
      .mockResolvedValueOnce(undefined);
    getWebSocketClientMock.mockReturnValue({ request });
    const { result } = renderMessageHandler();
    const original = {
      message: "Review this",
      planCommentRefs: [{ id: COMMENT_ID, version: 2 }],
    };

    await expect(result.current.handleSendMessage(original)).rejects.toThrow("rejected");
    await expect(
      result.current.handleSendMessage({
        ...original,
        reviewComments: [{}] as never,
      }),
    ).resolves.toBeUndefined();

    const admissions = request.mock.calls.filter(([action]) => action === MESSAGE_ADD_ACTION);
    expect(admissions[1]?.[1]?.client_message_id).not.toBe(admissions[0]?.[1]?.client_message_id);
  });
});
