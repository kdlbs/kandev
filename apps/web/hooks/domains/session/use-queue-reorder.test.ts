import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Mock } from "vitest";
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
    metaBySessionId: Record<
      string,
      { count: number; max: number; mergeEnabled?: boolean; autoRun?: boolean }
    >;
    activeOperationBySessionId: Record<string, QueueOperationToken>;
  };
  connection: { status: string };
  taskSessions: {
    items: Record<string, { task_id?: string; queue_incarnation_id?: string }>;
  };
  setQueueEntries: Mock;
  removeQueueEntry: Mock;
  beginQueueOperation: Mock;
  finishQueueOperation: Mock;
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
    queue: {
      bySessionId: {},
      metaBySessionId: {},
      activeOperationBySessionId: {},
    },
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
  queueApiMock.reorderQueuedEntries.mockReset();
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useQueue reorderEntries", () => {
  it("optimistically reorders the store and refetches the authoritative queue", async () => {
    const first = entry({ id: "q-1", content: "first" });
    const second = entry({ id: "q-2", content: "second" });
    mockState.queue.bySessionId[SESSION_ID] = [first, second];
    mockState.queue.metaBySessionId[SESSION_ID] = { count: 2, max: 10 };
    queueApiMock.getQueueStatus.mockResolvedValue({
      ...IDENTITY,
      entries: [first, second],
      count: 2,
      max: 10,
    });
    queueApiMock.reorderQueuedEntries.mockResolvedValue({
      session_id: SESSION_ID,
      reordered: 2,
    });

    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();

    await act(async () => {
      await result.current.reorderEntries([second.id, first.id]);
    });

    expect(queueApiMock.reorderQueuedEntries).toHaveBeenCalledWith({
      session_id: SESSION_ID,
      task_id: TASK_ID,
      session_incarnation_id: IDENTITY.session_incarnation_id,
      ordered_ids: [second.id, first.id],
    });
    expect(mockState.setQueueEntries).toHaveBeenCalledWith(SESSION_ID, [second, first], {
      count: 2,
      max: 10,
      mergeEnabled: true,
      autoRun: true,
    });
    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
  });

  it("refetches and rethrows when the reorder fails", async () => {
    const first = entry({ id: "q-1", content: "first" });
    const second = entry({ id: "q-2", content: "second" });
    queueApiMock.getQueueStatus.mockResolvedValue({
      ...IDENTITY,
      entries: [first, second],
      count: 2,
      max: 10,
    });
    queueApiMock.reorderQueuedEntries.mockRejectedValueOnce(new Error("reorder failed"));
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();
    queueApiMock.getQueueStatus.mockResolvedValueOnce({
      ...IDENTITY,
      entries: [first, second],
      count: 2,
      max: 10,
    });

    await act(async () => {
      await expect(result.current.reorderEntries([second.id, first.id])).rejects.toThrow(
        "reorder failed",
      );
    });

    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
  });

  it("refetches and rethrows a QueueReorderError so the panel can swallow it", async () => {
    const first = entry({ id: "q-1", content: "first" });
    queueApiMock.getQueueStatus.mockResolvedValue({
      ...IDENTITY,
      entries: [first],
      count: 1,
      max: 10,
    });
    queueApiMock.reorderQueuedEntries.mockRejectedValueOnce(new queueApiMock.QueueReorderError());

    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();

    await act(async () => {
      await expect(result.current.reorderEntries([first.id])).rejects.toThrow(
        queueApiMock.QueueReorderError,
      );
    });

    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
  });
});
