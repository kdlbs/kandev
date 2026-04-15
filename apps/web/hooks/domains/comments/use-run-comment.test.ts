import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { DiffComment, PlanComment } from "@/lib/state/slices/comments";

// ---------------------------------------------------------------------------
// Mocks — declared before imports that use them
// ---------------------------------------------------------------------------

const mockRequest = vi.fn();
const mockAppendToQueue = vi.fn();
const mockMarkCommentsSent = vi.fn();
const mockGetWebSocketClient = vi.fn(() => ({ request: mockRequest }));
let mockStoreState: Record<string, unknown> = {};

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mockGetWebSocketClient(),
}));

vi.mock("@/lib/api/domains/queue-api", () => ({
  appendToQueue: (...args: unknown[]) => mockAppendToQueue(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => mockStoreState }),
}));

vi.mock("@/lib/state/slices/comments", () => ({
  useCommentsStore: (selector: (s: Record<string, unknown>) => unknown) =>
    selector({ markCommentsSent: mockMarkCommentsSent }),
}));

vi.mock("@/lib/state/slices/comments/format", () => ({
  formatReviewCommentsAsMarkdown: (comments: DiffComment[]) => `[diff] ${comments[0]?.text ?? ""}`,
  formatPlanCommentsAsMarkdown: (comments: PlanComment[]) => `[plan] ${comments[0]?.text ?? ""}`,
  formatPRFeedbackAsMarkdown: () => "[pr-feedback]",
}));

import { useRunComment } from "./use-run-comment";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeStoreState(sessionState: string, planMode = false) {
  return {
    taskSessions: {
      items: { "sess-1": { state: sessionState } },
    },
    chatInput: {
      planModeBySessionId: { "sess-1": planMode },
    },
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
    sessionId: "sess-1",
    text,
    selectedText: "step 2",
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

  it("queues via appendToQueue when agent is STARTING", async () => {
    mockStoreState = makeStoreState("STARTING");
    const { result } = renderCommentHook();

    let res: { queued: boolean } | undefined;
    await act(async () => {
      res = await result.current.runComment(makeDiffComment());
    });

    expect(res).toEqual({ queued: true });
    expect(mockAppendToQueue).toHaveBeenCalled();
    expect(mockRequest).not.toHaveBeenCalled();
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

  it("includes plan_mode when plan mode is enabled", async () => {
    mockStoreState = makeStoreState("WAITING_FOR_INPUT", true);
    const { result } = renderCommentHook();

    await act(async () => {
      await result.current.runComment(makePlanComment());
    });

    expect(mockRequest).toHaveBeenCalledWith(
      "message.add",
      expect.objectContaining({ plan_mode: true }),
      10000,
    );
  });

  it("omits has_review_comments for plan comments", async () => {
    const { result } = renderCommentHook();

    await act(async () => {
      await result.current.runComment(makePlanComment());
    });

    const payload = mockRequest.mock.calls[0][1];
    expect(payload.has_review_comments).toBeUndefined();
  });

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
