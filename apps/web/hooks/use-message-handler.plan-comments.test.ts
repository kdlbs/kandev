import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { WebSocketRequestError } from "@/lib/ws/request-error";
import { sendMessageRequest, useMessageHandler } from "./use-message-handler";

const getWebSocketClientMock = vi.hoisted(() => vi.fn());
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
  storeState.current.taskSessions.items = {};
  storeState.current.queue.metaBySessionId = {};
  getWebSocketClientMock.mockReturnValue({ request: vi.fn().mockResolvedValue(undefined) });
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

describe("useMessageHandler queued plan comments", () => {
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
});
