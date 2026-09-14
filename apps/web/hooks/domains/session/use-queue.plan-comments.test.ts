import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { QueuedMessage, QueueOperationToken } from "@/lib/state/slices/session/types";

const queueApiMock = vi.hoisted(() => {
  class QueueEntryNotFoundError extends Error {}
  class QueueSendNowError extends Error {}
  class QueueReorderError extends Error {}
  return {
    QueueEntryNotFoundError,
    QueueSendNowError,
    QueueReorderError,
    queueMessage: vi.fn(),
    clearQueue: vi.fn(),
    getQueueStatus: vi.fn(),
    updateQueuedMessage: vi.fn(),
    removeQueuedEntry: vi.fn(),
    mergeQueuedEntry: vi.fn(),
    reorderQueuedEntries: vi.fn(),
    sendQueuedNow: vi.fn(),
    setQueueAutoRun: vi.fn(),
    setQueueAutoMerge: vi.fn(),
  };
});

type MockQueueState = {
  queue: {
    bySessionId: Record<string, QueuedMessage[]>;
    metaBySessionId: Record<string, { count: number; max: number }>;
    activeOperationBySessionId: Record<string, QueueOperationToken>;
  };
  connection: { status: string };
  taskSessions: {
    items: Record<
      string,
      { task_id?: string; queue_incarnation_id?: string; cancellation_pending?: boolean }
    >;
  };
  setQueueEntries: ReturnType<typeof vi.fn>;
  removeQueueEntry: ReturnType<typeof vi.fn>;
  beginQueueOperation: ReturnType<typeof vi.fn>;
  finishQueueOperation: ReturnType<typeof vi.fn>;
};

let mockState: MockQueueState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockQueueState) => unknown) => selector(mockState),
}));

vi.mock("@/lib/api/domains/queue-api", () => queueApiMock);

import { useQueue } from "./use-queue";

const SESSION_ID = "sess-1";
const TASK_ID = "task-1";
const IDENTITY = {
  task_id: TASK_ID,
  session_id: SESSION_ID,
  session_incarnation_id: "incarnation-1",
};

function entry(overrides: Partial<QueuedMessage> = {}): QueuedMessage {
  return {
    id: "q-1",
    session_id: SESSION_ID,
    task_id: TASK_ID,
    content: "queued prompt",
    plan_mode: false,
    queued_at: "2026-06-27T00:00:00Z",
    queued_by: "user",
    ...overrides,
  };
}

beforeEach(() => {
  mockState = {
    queue: { bySessionId: {}, metaBySessionId: {}, activeOperationBySessionId: {} },
    connection: { status: "connected" },
    taskSessions: {
      items: {
        [SESSION_ID]: {
          task_id: TASK_ID,
          queue_incarnation_id: IDENTITY.session_incarnation_id,
        },
      },
    },
    setQueueEntries: vi.fn(),
    removeQueueEntry: vi.fn(),
    beginQueueOperation: vi
      .fn()
      .mockReturnValue({ sessionIncarnationId: IDENTITY.session_incarnation_id, generation: 1 }),
    finishQueueOperation: vi.fn(),
  };
  queueApiMock.getQueueStatus.mockResolvedValue({ ...IDENTITY, entries: [], count: 0, max: 10 });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useQueue task plan comment admission", () => {
  it("forwards task plan comment admission fields", async () => {
    queueApiMock.queueMessage.mockResolvedValue(entry());
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.queueMessage.mockClear();

    await act(async () => {
      await result.current.queue({
        taskId: TASK_ID,
        content: "",
        clientQueueId: "client-queue-1",
        planMode: true,
        planCommentRefs: [{ id: "comment-1", version: 2 }],
        requirePrimarySession: true,
      });
    });

    expect(queueApiMock.queueMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        client_queue_id: "client-queue-1",
        plan_comment_refs: [{ id: "comment-1", version: 2 }],
        require_primary_session: true,
      }),
    );
  });

  it("keeps an accepted queue admission successful when reconciliation fails", async () => {
    queueApiMock.queueMessage.mockResolvedValue(entry({ id: "accepted" }));
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockRejectedValueOnce(new Error("snapshot unavailable"));

    await expect(
      act(async () => {
        await result.current.queue({
          taskId: TASK_ID,
          content: "",
          clientQueueId: "accepted",
          planCommentRefs: [{ id: "comment-1", version: 2 }],
        });
      }),
    ).resolves.toBeUndefined();
  });

  it("reports rejection when the session identity is unavailable", async () => {
    delete mockState.taskSessions.items[SESSION_ID];
    const { result } = renderHook(() => useQueue(SESSION_ID));

    await expect(
      result.current.queue({ taskId: TASK_ID, content: "keep this draft" }),
    ).resolves.toBe(false);
    expect(queueApiMock.queueMessage).not.toHaveBeenCalled();
  });

  it("reports rejection when another queue mutation owns admission", async () => {
    mockState.beginQueueOperation.mockReturnValue(null);
    const { result } = renderHook(() => useQueue(SESSION_ID));

    await expect(
      result.current.queue({ taskId: TASK_ID, content: "keep this draft" }),
    ).resolves.toBe(false);
    expect(queueApiMock.queueMessage).not.toHaveBeenCalled();
  });
});
