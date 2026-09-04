/* eslint-disable sonarjs/no-duplicate-string -- Session IDs are repeated to make primary-session transitions explicit. */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type {
  AgentMessageComment,
  DiffComment,
  PlanComment,
  WalkthroughComment,
} from "@/lib/state/slices/comments";
import { WebSocketRequestError } from "@/lib/ws/request-error";

// ---------------------------------------------------------------------------
// Mocks — declared before imports that use them
// ---------------------------------------------------------------------------

const mockRequest = vi.fn();
const mockAppendToQueue = vi.fn();
const mockQueueMessage = vi.fn();
const mockSendMessageRequest = vi.fn();
const mockMarkCommentsSent = vi.fn();
const mockAddMessage = vi.fn();
const mockSetTaskPlanComments = vi.fn();
const mockGetTaskPlanComments = vi.fn();
const mockSetTaskSession = vi.fn((next: Record<string, unknown>) => {
  const state = mockStoreState as ReturnType<typeof makeStoreState>;
  state.taskSessions.items[next.id as string] = next as never;
  const list = state.taskSessionsByTask.itemsByTaskId[next.task_id as string];
  const index = list.findIndex((candidate) => candidate.id === next.id);
  if (index >= 0) list[index] = next as never;
});
const mockSetState = vi.fn(
  (updater: (state: Record<string, unknown>) => Record<string, unknown>) => {
    mockStoreState = { ...mockStoreState, ...updater(mockStoreState) };
  },
);
const mockGetWebSocketClient = vi.fn(() => ({ request: mockRequest }));
let mockStoreState: Record<string, unknown> = {};

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mockGetWebSocketClient(),
}));

vi.mock("@/lib/api/domains/queue-api", () => ({
  appendToQueue: (...args: unknown[]) => mockAppendToQueue(...args),
  queueMessage: (...args: unknown[]) => mockQueueMessage(...args),
}));

vi.mock("@/lib/api/domains/plan-comment-api", () => ({
  getTaskPlanComments: (...args: unknown[]) => mockGetTaskPlanComments(...args),
}));

vi.mock("@/hooks/use-message-handler", () => ({
  sendMessageRequest: (...args: unknown[]) => mockSendMessageRequest(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => mockStoreState, setState: mockSetState }),
}));

vi.mock("@/lib/state/slices/comments", () => ({
  useCommentsStore: (selector: (s: Record<string, unknown>) => unknown) =>
    selector({ markCommentsSent: mockMarkCommentsSent }),
}));

vi.mock("@/lib/state/slices/comments/format", () => ({
  formatReviewCommentsAsMarkdown: (comments: DiffComment[]) => `[diff] ${comments[0]?.text ?? ""}`,
  formatPlanCommentsAsMarkdown: (comments: PlanComment[]) => `[plan] ${comments[0]?.text ?? ""}`,
  formatPRFeedbackAsMarkdown: () => "[pr-feedback]",
  formatWalkthroughCommentsAsMarkdown: (comments: WalkthroughComment[]) =>
    `[walkthrough] ${comments[0]?.text ?? ""}`,
  formatAgentMessageCommentsAsMarkdown: (comments: AgentMessageComment[]) =>
    `[agent-message] ${comments[0]?.text ?? ""}`,
}));

import { useRunComment } from "./use-run-comment";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type MockSession = {
  id?: string;
  task_id?: string;
  state: string;
  foreground_activity?: string;
  is_primary?: boolean;
};

function makeStoreState(sessionState: string, planMode = false, foregroundActivity?: string) {
  const selected = {
    id: "sess-1",
    task_id: "task-1",
    state: sessionState,
    foreground_activity: foregroundActivity,
    is_primary: false,
  };
  const primary = {
    id: "primary-session",
    task_id: "task-1",
    state: "WAITING_FOR_INPUT",
    is_primary: true,
  };
  const items: Record<string, MockSession> = {
    "sess-1": selected,
    "primary-session": primary,
    "other-session": { state: "RUNNING", foreground_activity: "generating" },
  };
  const itemsByTaskId: Record<string, MockSession[]> = { "task-1": [primary, selected] };
  return {
    kanban: {
      tasks: [{ id: "task-1", primarySessionId: "primary-session" as string | null }],
    },
    kanbanMulti: { snapshots: {} },
    taskSessions: {
      items,
    },
    taskSessionsByTask: { itemsByTaskId },
    chatInput: {
      planModeBySessionId: { "sess-1": planMode },
    },
    taskPlans: {
      commentsMigrationStatusByTaskId: { "task-1": "complete" },
    },
    queue: { metaBySessionId: {} as Record<string, { count: number }> },
    addMessage: mockAddMessage,
    setTaskPlanComments: mockSetTaskPlanComments,
    setTaskSession: mockSetTaskSession,
  };
}

function makeDiffComment(text = "fix this"): DiffComment {
  return {
    id: "c-1",
    source: "diff",
    sessionId: "sess-1",
    filePath: "src/app.ts",
    startLine: 10,
    endLine: 12,
    side: "additions",
    codeContent: "const x = 1;",
    text,
    createdAt: new Date().toISOString(),
    status: "pending",
  };
}

function makePlanComment(text = "split step 2"): PlanComment {
  return {
    id: "c-2",
    source: "plan",
    sessionId: "",
    taskId: "task-1",
    planId: "plan-1",
    version: 2,
    text,
    selectedText: "step 2",
    createdAt: new Date().toISOString(),
    status: "pending",
  };
}

function makeWalkthroughComment(text = "explain this step"): WalkthroughComment {
  return {
    id: "c-3",
    source: "walkthrough",
    sessionId: "sess-1",
    taskId: "task-1",
    walkthroughId: "wt-1",
    walkthroughTitle: "Tour",
    stepIndex: 0,
    stepCount: 2,
    filePath: "src/app.ts",
    startLine: 10,
    endLine: 12,
    stepText: "Agent explanation",
    text,
    createdAt: new Date().toISOString(),
    status: "pending",
  };
}

function makeAgentMessageComment(text = "expand this answer"): AgentMessageComment {
  return {
    id: "c-4",
    source: "agent-message",
    sessionId: "sess-1",
    messageId: "reply-1",
    selectedText: "settled answer",
    anchor: {
      messageId: "reply-1",
      start: 0,
      end: 14,
      selectedText: "settled answer",
      prefix: "",
      suffix: "",
    },
    text,
    createdAt: new Date().toISOString(),
    status: "pending",
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

function renderCommentHook(sessionId: string | null = "sess-1") {
  return renderHook(() => useRunComment({ sessionId, taskId: "task-1" }));
}

function setup() {
  vi.clearAllMocks();
  mockStoreState = makeStoreState("WAITING_FOR_INPUT");
  mockGetTaskPlanComments.mockResolvedValue({
    task_id: "task-1",
    plan_id: "plan-1",
    revision: 3,
    comments: [],
  });
}

describe("useRunComment — idle agent sends directly", () => {
  beforeEach(setup);

  it("sends directly via message.add when agent is idle", async () => {
    mockStoreState = makeStoreState("WAITING_FOR_INPUT");
    const { result } = renderCommentHook();

    let res: { queued: boolean } | undefined;
    await act(async () => {
      res = await result.current.runComment(makeDiffComment());
    });

    expect(res).toEqual({ queued: false });
    expect(mockRequest).toHaveBeenCalledWith(
      "message.add",
      expect.objectContaining({
        task_id: "task-1",
        session_id: "sess-1",
        has_review_comments: true,
      }),
      10000,
    );
    expect(mockAppendToQueue).not.toHaveBeenCalled();
    expect(mockMarkCommentsSent).toHaveBeenCalledWith(["c-1"]);
  });

  it("sends directly for a CREATED session so its first prompt can start the agent", async () => {
    mockStoreState = makeStoreState("CREATED");
    const { result } = renderCommentHook();

    await expect(result.current.runComment(makeDiffComment())).resolves.toEqual({ queued: false });
    expect(mockRequest).toHaveBeenCalled();
    expect(mockAppendToQueue).not.toHaveBeenCalled();
  });

  it("reads fresh store state at call time, not from closure", async () => {
    mockStoreState = makeStoreState("RUNNING");
    const { result } = renderCommentHook();

    // Agent finishes — store changes but hook NOT re-rendered
    mockStoreState = makeStoreState("WAITING_FOR_INPUT");

    let res: { queued: boolean } | undefined;
    await act(async () => {
      res = await result.current.runComment(makeDiffComment());
    });

    expect(res).toEqual({ queued: false });
    expect(mockRequest).toHaveBeenCalled();
    expect(mockAppendToQueue).not.toHaveBeenCalled();
  });

  it("sends directly for RUNNING background work despite another generating session", async () => {
    mockStoreState = makeStoreState("RUNNING", false, "background");
    const { result } = renderCommentHook();

    let res: { queued: boolean } | undefined;
    await act(async () => {
      res = await result.current.runComment(makeDiffComment());
    });

    expect(res).toEqual({ queued: false });
    expect(mockRequest).toHaveBeenCalled();
    expect(mockAppendToQueue).not.toHaveBeenCalled();
  });

  it("re-throws when message.add fails", async () => {
    mockRequest.mockRejectedValueOnce(new Error("WS timeout"));
    const { result } = renderCommentHook();

    await expect(
      act(async () => {
        await result.current.runComment(makeDiffComment());
      }),
    ).rejects.toThrow("WS timeout");

    expect(mockMarkCommentsSent).not.toHaveBeenCalled();
  });

  // Regression: comments sent via "Run" sometimes did not appear in the chat
  // until a page refresh, because the hook depended entirely on the
  // session.message.added broadcast — which can be missed if the client's
  // session subscription is briefly absent or its send buffer drops.
  // The message returned in the message.add response must be added to the
  // store optimistically; addMessage is idempotent so a later broadcast for
  // the same id is a no-op.
  it("adds returned message to the store so chat updates without waiting for broadcast", async () => {
    const returnedMessage = {
      id: "msg-42",
      session_id: "sess-1",
      task_id: "task-1",
      author_type: "user",
      content: "[diff] fix this",
      type: "message",
      created_at: "2026-05-08T00:00:00Z",
    };
    mockRequest.mockResolvedValueOnce(returnedMessage);
    const { result } = renderCommentHook();

    await act(async () => {
      await result.current.runComment(makeDiffComment());
    });

    expect(mockAddMessage).toHaveBeenCalledWith(returnedMessage);
  });

  it("does not call addMessage when message.add returns no message", async () => {
    mockRequest.mockResolvedValueOnce(undefined);
    const { result } = renderCommentHook();

    await act(async () => {
      await result.current.runComment(makeDiffComment());
    });

    expect(mockAddMessage).not.toHaveBeenCalled();
  });
});

describe("useRunComment — busy agent queues", () => {
  beforeEach(setup);

  it("queues via appendToQueue when agent is RUNNING", async () => {
    mockStoreState = makeStoreState("RUNNING");
    const { result } = renderCommentHook();

    let res: { queued: boolean } | undefined;
    await act(async () => {
      res = await result.current.runComment(makeDiffComment());
    });

    expect(res).toEqual({ queued: true });
    expect(mockAppendToQueue).toHaveBeenCalledWith(
      expect.objectContaining({ session_id: "sess-1", task_id: "task-1" }),
    );
    expect(mockRequest).not.toHaveBeenCalled();
    expect(mockMarkCommentsSent).toHaveBeenCalledWith(["c-1"]);
  });

  it("queues via appendToQueue when the selected session is STARTING", async () => {
    mockStoreState = makeStoreState("STARTING");
    const { result } = renderCommentHook();

    await expect(result.current.runComment(makeDiffComment())).resolves.toEqual({ queued: true });
    expect(mockAppendToQueue).toHaveBeenCalled();
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it("formats walkthrough comments when queuing", async () => {
    mockStoreState = makeStoreState("RUNNING");
    const { result } = renderCommentHook();

    await act(async () => {
      await result.current.runComment(makeWalkthroughComment("please expand"));
    });

    expect(mockAppendToQueue).toHaveBeenCalledWith(
      expect.objectContaining({ content: "[walkthrough] please expand" }),
    );
    expect(mockMarkCommentsSent).toHaveBeenCalledWith(["c-3"]);
  });

  it("formats agent message comments when queuing", async () => {
    mockStoreState = makeStoreState("RUNNING");
    const { result } = renderCommentHook();

    await act(async () => {
      await result.current.runComment(makeAgentMessageComment("please expand"));
    });

    expect(mockAppendToQueue).toHaveBeenCalledWith(
      expect.objectContaining({ content: "[agent-message] please expand" }),
    );
    expect(mockMarkCommentsSent).toHaveBeenCalledWith(["c-4"]);
  });
});

describe("useRunComment — edge cases", () => {
  beforeEach(setup);

  it("returns { queued: false } when sessionId is null", async () => {
    const { result } = renderCommentHook(null);

    let res: { queued: boolean } | undefined;
    await act(async () => {
      res = await result.current.runComment(makeDiffComment());
    });

    expect(res).toEqual({ queued: false });
    expect(mockRequest).not.toHaveBeenCalled();
    expect(mockAppendToQueue).not.toHaveBeenCalled();
  });
});

// eslint-disable-next-line max-lines-per-function -- Primary routing and recovery cases share one store fixture.
describe("useRunComment — plan routing", () => {
  beforeEach(setup);

  it("routes a plan comment to the current primary with one guarded ref", async () => {
    mockStoreState = makeStoreState("WAITING_FOR_INPUT", false);
    const { result } = renderCommentHook();

    await act(async () => {
      await result.current.runComment(makePlanComment());
    });

    expect(mockSendMessageRequest).toHaveBeenCalledWith({
      taskId: "task-1",
      resolvedSessionId: "primary-session",
      clientMessageId: expect.any(String),
      finalMessage: "",
      modelToSend: undefined,
      planMode: true,
      planCommentRefs: [{ id: "c-2", version: 2 }],
      requirePrimarySession: true,
    });
    expect(mockRequest).not.toHaveBeenCalled();
    expect(mockMarkCommentsSent).not.toHaveBeenCalled();
  });

  it("uses the task primary while per-session flags lag behind", async () => {
    const state = makeStoreState("WAITING_FOR_INPUT");
    state.taskSessions.items["new-primary"] = {
      id: "new-primary",
      task_id: "task-1",
      state: "WAITING_FOR_INPUT",
      is_primary: false,
    };
    state.taskSessionsByTask.itemsByTaskId["task-1"].push(state.taskSessions.items["new-primary"]);
    state.kanban.tasks[0].primarySessionId = "new-primary";
    mockStoreState = state;
    const { result } = renderCommentHook();

    await result.current.runComment(makePlanComment());

    expect(mockSendMessageRequest).toHaveBeenCalledWith(
      expect.objectContaining({ resolvedSessionId: "new-primary" }),
    );
  });

  it("rejects Run until legacy plan comments finish migrating", async () => {
    const state = makeStoreState("WAITING_FOR_INPUT");
    state.taskPlans.commentsMigrationStatusByTaskId["task-1"] = "failed";
    mockStoreState = state;
    const { result } = renderCommentHook();

    await expect(result.current.runComment(makePlanComment())).rejects.toMatchObject({
      code: "plan-comment-migration-pending",
    });
    expect(mockSendMessageRequest).not.toHaveBeenCalled();
    expect(mockQueueMessage).not.toHaveBeenCalled();
  });

  it("queues a busy primary as a distinct idempotent entry instead of appending", async () => {
    const state = makeStoreState("WAITING_FOR_INPUT");
    (state.taskSessions.items["primary-session"] as { state: string }).state = "RUNNING";
    (state.taskSessionsByTask.itemsByTaskId["task-1"][0] as { state: string }).state = "RUNNING";
    mockStoreState = state;
    const { result } = renderCommentHook();

    await act(async () => {
      await result.current.runComment(makePlanComment());
    });

    expect(mockQueueMessage).toHaveBeenCalledWith({
      session_id: "primary-session",
      task_id: "task-1",
      client_queue_id: expect.any(String),
      content: "",
      plan_mode: true,
      plan_comment_refs: [{ id: "c-2", version: 2 }],
      require_primary_session: true,
    });
    expect(mockAppendToQueue).not.toHaveBeenCalled();
    expect(mockMarkCommentsSent).not.toHaveBeenCalled();
  });

  it("queues a steer-capable primary when earlier prompts are already queued", async () => {
    const state = makeStoreState("WAITING_FOR_INPUT");
    Object.assign(state.taskSessions.items["primary-session"], {
      state: "RUNNING",
      foreground_activity: "generating",
      supports_steering: true,
    });
    Object.assign(state.taskSessionsByTask.itemsByTaskId["task-1"][0], {
      state: "RUNNING",
      foreground_activity: "generating",
      supports_steering: true,
    });
    state.queue.metaBySessionId["primary-session"] = { count: 1 };
    mockStoreState = state;
    const { result } = renderCommentHook();

    await result.current.runComment(makePlanComment());

    expect(mockQueueMessage).toHaveBeenCalled();
    expect(mockSendMessageRequest).not.toHaveBeenCalled();
  });

  it("re-resolves the primary at action time", async () => {
    const { result } = renderCommentHook();
    const changed = makeStoreState("WAITING_FOR_INPUT");
    const replacement = {
      id: "new-primary",
      task_id: "task-1",
      state: "WAITING_FOR_INPUT",
      is_primary: true,
    };
    changed.taskSessions.items["primary-session"].is_primary = false;
    const sessions = changed.taskSessions.items as Record<
      string,
      { id?: string; task_id?: string; state: string; is_primary?: boolean }
    >;
    sessions["new-primary"] = replacement;
    changed.taskSessionsByTask.itemsByTaskId["task-1"] = [replacement];
    changed.kanban.tasks[0].primarySessionId = "new-primary";
    mockStoreState = changed;

    await act(async () => {
      await result.current.runComment(makePlanComment());
    });

    expect(mockSendMessageRequest).toHaveBeenCalledWith(
      expect.objectContaining({ resolvedSessionId: "new-primary" }),
    );
  });

  it("ignores a stale primary flag in the task list", async () => {
    const state = makeStoreState("WAITING_FOR_INPUT");
    state.taskSessions.items["primary-session"].is_primary = false;
    state.taskSessions.items["new-primary"] = {
      id: "new-primary",
      task_id: "task-1",
      state: "WAITING_FOR_INPUT",
      is_primary: true,
    } as never;
    state.kanban.tasks[0].primarySessionId = "new-primary";
    mockStoreState = state;
    const { result } = renderCommentHook();

    await result.current.runComment(makePlanComment());

    expect(mockSendMessageRequest).toHaveBeenCalledWith(
      expect.objectContaining({ resolvedSessionId: "new-primary" }),
    );
  });
});

describe("useRunComment — plan availability failures", () => {
  beforeEach(setup);

  it("rejects Run when the task has no primary", async () => {
    const state = makeStoreState("WAITING_FOR_INPUT");
    state.taskSessions.items["primary-session"].is_primary = false;
    state.taskSessionsByTask.itemsByTaskId["task-1"] = [];
    state.kanban.tasks[0].primarySessionId = null;
    mockStoreState = state;
    const { result } = renderCommentHook();

    await expect(result.current.runComment(makePlanComment())).rejects.toMatchObject({
      code: "no-primary-session",
    });
    expect(mockSendMessageRequest).not.toHaveBeenCalled();
    expect(mockQueueMessage).not.toHaveBeenCalled();
  });

  it("rejects Run when the primary is terminal", async () => {
    const state = makeStoreState("WAITING_FOR_INPUT");
    (state.taskSessions.items["primary-session"] as { state: string }).state = "COMPLETED";
    (state.taskSessionsByTask.itemsByTaskId["task-1"][0] as { state: string }).state = "COMPLETED";
    mockStoreState = state;
    const { result } = renderCommentHook();

    await expect(result.current.runComment(makePlanComment())).rejects.toMatchObject({
      code: "primary-session-unavailable",
    });
  });

  it("normalizes a stale-primary rejection for inline retry feedback", async () => {
    mockSendMessageRequest.mockRejectedValueOnce(
      new WebSocketRequestError("Primary session changed", "primary_session_changed"),
    );
    const { result } = renderCommentHook();

    await expect(result.current.runComment(makePlanComment())).rejects.toMatchObject({
      code: "primary-session-changed",
    });
    expect(mockMarkCommentsSent).not.toHaveBeenCalled();
  });

  it("applies stale-primary recovery details before retry", async () => {
    const state = makeStoreState("WAITING_FOR_INPUT");
    state.taskSessions.items["new-primary"] = {
      id: "new-primary",
      task_id: "task-1",
      state: "STARTING",
      is_primary: false,
    } as never;
    state.taskSessionsByTask.itemsByTaskId["task-1"].push(
      state.taskSessions.items["new-primary"] as never,
    );
    mockStoreState = state;
    mockSendMessageRequest.mockRejectedValueOnce(
      new WebSocketRequestError("Primary session changed", "primary_session_changed", {
        primary_session_id: "new-primary",
        primary_session_state: "WAITING_FOR_INPUT",
      }),
    );
    const { result } = renderCommentHook();

    await expect(result.current.runComment(makePlanComment())).rejects.toMatchObject({
      code: "primary-session-changed",
    });
    await result.current.runComment(makePlanComment());

    expect(mockSendMessageRequest).toHaveBeenLastCalledWith(
      expect.objectContaining({ resolvedSessionId: "new-primary" }),
    );
  });
});

describe("useRunComment — session delivery failures", () => {
  beforeEach(setup);

  it("throws when WS client is null and does not mark comment sent", async () => {
    mockGetWebSocketClient.mockReturnValueOnce(null as unknown as { request: typeof mockRequest });
    const { result } = renderCommentHook();

    await expect(
      act(async () => {
        await result.current.runComment(makeDiffComment());
      }),
    ).rejects.toThrow("WebSocket client unavailable");

    expect(mockRequest).not.toHaveBeenCalled();
    expect(mockMarkCommentsSent).not.toHaveBeenCalled();
  });
});
