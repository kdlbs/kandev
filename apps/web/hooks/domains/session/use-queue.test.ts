import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

const queueApiMock = vi.hoisted(() => {
  class QueueEntryNotFoundError extends Error {}
  return {
    QueueEntryNotFoundError,
    queueMessage: vi.fn(),
    clearQueue: vi.fn(),
    drainQueuedMessage: vi.fn(),
    getQueueStatus: vi.fn(),
    updateQueuedMessage: vi.fn(),
    removeQueuedEntry: vi.fn(),
  };
});

type MockQueueState = {
  queue: {
    bySessionId: Record<string, QueuedMessage[]>;
    metaBySessionId: Record<string, { count: number; max: number }>;
    isLoading: Record<string, boolean>;
  };
  connection: { status: string };
  setQueueEntries: ReturnType<typeof vi.fn>;
  removeQueueEntry: ReturnType<typeof vi.fn>;
  setQueueLoading: ReturnType<typeof vi.fn>;
};

let mockState: MockQueueState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockQueueState) => unknown) => selector(mockState),
}));

vi.mock("@/lib/api/domains/queue-api", () => queueApiMock);

import { useQueue } from "./use-queue";

const SESSION_ID = "sess-1";
const TASK_ID = "task-1";

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

function setDocumentVisibility(value: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    value,
  });
}

function resetMockState() {
  mockState = {
    queue: {
      bySessionId: {},
      metaBySessionId: {},
      isLoading: {},
    },
    connection: { status: "connected" },
    setQueueEntries: vi.fn(),
    removeQueueEntry: vi.fn(),
    setQueueLoading: vi.fn(),
  };
}

describe("useQueue", () => {
  beforeEach(() => {
    resetMockState();
    setDocumentVisibility("visible");
    queueApiMock.getQueueStatus.mockResolvedValue({ entries: [], count: 0, max: 10 });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("refetches the queue snapshot when the WebSocket reconnects", async () => {
    mockState.connection.status = "disconnected";
    const { rerender } = renderHook(() => useQueue(SESSION_ID));

    await act(async () => {});
    expect(queueApiMock.getQueueStatus).not.toHaveBeenCalled();

    mockState.connection.status = "connected";
    rerender();

    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(SESSION_ID));
    expect(mockState.setQueueEntries).toHaveBeenCalledWith(SESSION_ID, [], {
      count: 0,
      max: 10,
    });
  });

  it("refetches a stale queue snapshot when a suspended tab becomes visible again", async () => {
    mockState.queue.bySessionId[SESSION_ID] = [entry()];
    mockState.queue.metaBySessionId[SESSION_ID] = { count: 1, max: 10 };
    queueApiMock.getQueueStatus.mockResolvedValueOnce({
      entries: [entry()],
      count: 1,
      max: 10,
    });

    renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledTimes(1));

    queueApiMock.getQueueStatus.mockClear();
    mockState.setQueueEntries.mockClear();
    queueApiMock.getQueueStatus.mockResolvedValueOnce({ entries: [], count: 0, max: 10 });

    document.dispatchEvent(new Event("visibilitychange"));

    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(SESSION_ID));
    expect(mockState.setQueueEntries).toHaveBeenCalledWith(SESSION_ID, [], {
      count: 0,
      max: 10,
    });
  });

  it("does not refetch on foreground visibility while disconnected", async () => {
    mockState.connection.status = "disconnected";
    renderHook(() => useQueue(SESSION_ID));

    await act(async () => {});
    document.dispatchEvent(new Event("visibilitychange"));

    expect(queueApiMock.getQueueStatus).not.toHaveBeenCalled();
  });
});
