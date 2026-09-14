import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { QueueOperationToken, QueuedMessage } from "@/lib/state/slices/session/types";

const queueApiMock = vi.hoisted(() => ({
  QueueEntryNotFoundError: class QueueEntryNotFoundError extends Error {},
  QueueEditConflictError: class QueueEditConflictError extends Error {},
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
}));

type MockState = {
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

let mockState: MockState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));
vi.mock("@/hooks/use-foreground-refresh", () => ({ useForegroundRefresh: vi.fn() }));
vi.mock("@/lib/api/domains/queue-api", () => queueApiMock);

import { useQueue } from "./use-queue";

const SESSION_ID = "sess-refetch";
const TASK_ID = "task-refetch";
const IDENTITY = {
  task_id: TASK_ID,
  session_id: SESSION_ID,
  session_incarnation_id: "incarnation-refetch",
};

function entry(id: string, content = id): QueuedMessage {
  return {
    id,
    session_id: SESSION_ID,
    task_id: TASK_ID,
    content,
    plan_mode: false,
    queued_at: "2026-06-27T00:00:00Z",
    queued_by: "user",
  };
}

function resetMockState() {
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
    beginQueueOperation: vi.fn().mockReturnValue({
      sessionIncarnationId: IDENTITY.session_incarnation_id,
      generation: 1,
    }),
    finishQueueOperation: vi.fn(),
  };
}

describe("queue refetch lifecycle fencing", () => {
  beforeEach(() => {
    resetMockState();
    queueApiMock.getQueueStatus.mockResolvedValue({ ...IDENTITY, entries: [], count: 0, max: 10 });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("drops a delayed snapshot from an unmounted hook instance", async () => {
    const pendingSnapshot = Promise.withResolvers<{
      entries: QueuedMessage[];
      count: number;
      max: number;
    }>();
    queueApiMock.getQueueStatus.mockReturnValueOnce(pendingSnapshot.promise).mockResolvedValueOnce({
      ...IDENTITY,
      entries: [entry("new-entry", "new snapshot")],
      count: 1,
      max: 10,
    });

    const first = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledTimes(1));
    first.unmount();

    renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(mockState.setQueueEntries).toHaveBeenCalledWith(
        SESSION_ID,
        [expect.objectContaining({ id: "new-entry" })],
        expect.anything(),
        expect.anything(),
      ),
    );
    mockState.setQueueEntries.mockClear();

    await act(async () => {
      pendingSnapshot.resolve({ ...IDENTITY, entries: [entry("stale-entry")], count: 1, max: 10 });
      await Promise.resolve();
    });

    expect(mockState.setQueueEntries).not.toHaveBeenCalledWith(
      SESSION_ID,
      [expect.objectContaining({ id: "stale-entry" })],
      expect.anything(),
    );
  });
  it("keeps a surviving hook refetch alive when a sibling unmounts", async () => {
    const first = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledTimes(1));
    const second = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledTimes(2));
    queueApiMock.getQueueStatus.mockClear();

    const pendingSnapshot = Promise.withResolvers<{
      entries: QueuedMessage[];
      count: number;
      max: number;
    }>();
    queueApiMock.getQueueStatus.mockReturnValueOnce(pendingSnapshot.promise);
    let refetchPromise!: Promise<void>;
    act(() => {
      refetchPromise = first.result.current.refetch();
    });
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledTimes(1));
    second.unmount();

    await act(async () => {
      pendingSnapshot.resolve({
        ...IDENTITY,
        entries: [entry("surviving-entry")],
        count: 1,
        max: 10,
      });
      await refetchPromise;
    });

    expect(mockState.setQueueEntries).toHaveBeenCalledWith(
      SESSION_ID,
      [expect.objectContaining({ id: "surviving-entry" })],
      expect.anything(),
      expect.anything(),
    );
    expect(mockState.finishQueueOperation).toHaveBeenCalled();
    first.unmount();
  });

  it("does not start reconciliation after an action outlives its hook", async () => {
    const pendingQueue = Promise.withResolvers<QueuedMessage>();
    queueApiMock.queueMessage.mockReturnValueOnce(pendingQueue.promise);
    const { result, unmount } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledTimes(1));
    queueApiMock.getQueueStatus.mockClear();

    await act(async () => {
      const queuePromise = result.current.queue({
        taskId: TASK_ID,
        content: "queued after unmount",
      });
      unmount();
      pendingQueue.resolve(entry("queued-entry", "queued after unmount"));
      await queuePromise;
    });

    expect(queueApiMock.getQueueStatus).not.toHaveBeenCalled();
  });
});
