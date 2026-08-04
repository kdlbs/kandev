import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { QueuedMessage } from "@/lib/state/slices/session/types";
import type { EntityReference } from "@/lib/types/entity-reference";

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
    mergeQueuedEntry: vi.fn(),
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
const reference: EntityReference = {
  version: 1,
  ref: "mention:v1:github:issue:acme%2Frepo:42",
  provider: "github",
  kind: "issue",
  id: "42",
  key: "acme/repo#42",
  title: "Fix composer references",
  url: "https://github.com/acme/repo/issues/42",
  scope: "acme/repo",
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

  it("refetches a stale queue snapshot when the Kandev window regains focus", async () => {
    renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledTimes(1));
    queueApiMock.getQueueStatus.mockClear();

    window.dispatchEvent(new Event("focus"));

    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(SESSION_ID));
  });

  it("does not refetch on foreground visibility while disconnected", async () => {
    mockState.connection.status = "disconnected";
    renderHook(() => useQueue(SESSION_ID));

    await act(async () => {});
    document.dispatchEvent(new Event("visibilitychange"));

    expect(queueApiMock.getQueueStatus).not.toHaveBeenCalled();
  });

  it("queues structured references with busy-agent messages", async () => {
    queueApiMock.queueMessage.mockResolvedValue(entry());
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.queueMessage.mockClear();

    await act(async () => {
      await result.current.queue({
        taskId: TASK_ID,
        content: "queued reference",
        entityReferences: [reference],
      } as never);
    });

    expect(queueApiMock.queueMessage).toHaveBeenCalledWith({
      session_id: SESSION_ID,
      task_id: TASK_ID,
      content: "queued reference",
      model: undefined,
      plan_mode: undefined,
      attachments: undefined,
      entity_references: [reference],
    });
  });

  it("replaces queued reference metadata with an explicit empty array", async () => {
    queueApiMock.updateQueuedMessage.mockResolvedValue({ entry_id: "q-1" });
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());

    await act(async () => {
      await result.current.editEntry("q-1", "reference removed", undefined, [] as never);
    });

    expect(queueApiMock.updateQueuedMessage).toHaveBeenCalledWith({
      session_id: SESSION_ID,
      entry_id: "q-1",
      content: "reference removed",
      attachments: undefined,
      entity_references: [],
    });
  });
});

describe("useQueue mergeEntry", () => {
  beforeEach(() => {
    resetMockState();
    setDocumentVisibility("visible");
    queueApiMock.getQueueStatus.mockResolvedValue({ entries: [], count: 0, max: 10 });
  });

  it("merges an entry and refetches the queue", async () => {
    queueApiMock.mergeQueuedEntry.mockResolvedValue({ entry_id: "q-1" });
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();

    await act(async () => {
      await result.current.mergeEntry("q-2");
    });

    expect(queueApiMock.mergeQueuedEntry).toHaveBeenCalledWith({
      session_id: SESSION_ID,
      entry_id: "q-2",
    });
    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(SESSION_ID);
  });

  it("forwards an explicit user_id on merge", async () => {
    queueApiMock.mergeQueuedEntry.mockResolvedValue({ entry_id: "q-1" });
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());

    await act(async () => {
      await result.current.mergeEntry("q-2", "alice");
    });

    expect(queueApiMock.mergeQueuedEntry).toHaveBeenCalledWith({
      session_id: SESSION_ID,
      entry_id: "q-2",
      user_id: "alice",
    });
  });

  it("refetches the queue when the merge target was already drained", async () => {
    queueApiMock.mergeQueuedEntry.mockRejectedValue(new queueApiMock.QueueEntryNotFoundError());
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();

    await act(async () => {
      await expect(result.current.mergeEntry("q-2")).rejects.toThrow(
        queueApiMock.QueueEntryNotFoundError,
      );
    });

    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(SESSION_ID);
  });
});

describe("useQueue clearAll", () => {
  beforeEach(() => {
    resetMockState();
    setDocumentVisibility("visible");
    queueApiMock.clearQueue.mockReset();
    queueApiMock.getQueueStatus.mockResolvedValue({ entries: [], count: 0, max: 10 });
    queueApiMock.clearQueue.mockResolvedValue(undefined);
  });

  it("refetches authoritative status after a successful clear", async () => {
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();

    await act(async () => {
      await result.current.clearAll();
    });

    expect(queueApiMock.clearQueue).toHaveBeenCalledWith(SESSION_ID);
    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(SESSION_ID);
  });

  it("refetches authoritative status and rethrows when clear fails", async () => {
    queueApiMock.clearQueue.mockRejectedValueOnce(new Error("clear failed"));
    const authoritative = entry({ id: "still-queued" });
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();
    queueApiMock.getQueueStatus.mockResolvedValueOnce({
      entries: [authoritative],
      count: 1,
      max: 10,
    });

    await act(async () => {
      await expect(result.current.clearAll()).rejects.toThrow("clear failed");
    });

    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(SESSION_ID);
    expect(mockState.setQueueEntries).toHaveBeenCalledWith(SESSION_ID, [authoritative], {
      count: 1,
      max: 10,
    });
  });

  it("discards an in-flight refetch that resolves after the clear", async () => {
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());

    // A refetch starts before the clear and resolves afterwards with the
    // pre-clear entries.
    let resolveStale: (status: { entries: QueuedMessage[]; count: number; max: number }) => void;
    queueApiMock.getQueueStatus.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveStale = resolve;
        }),
    );
    let staleRefetch: Promise<void>;
    act(() => {
      staleRefetch = result.current.refetch();
    });
    expect(mockState.setQueueEntries).toHaveBeenCalledTimes(1);

    await act(async () => {
      await result.current.clearAll();
    });

    await act(async () => {
      resolveStale!({ entries: [entry({ id: "pre-clear" })], count: 1, max: 10 });
      await staleRefetch!;
    });

    // The stale pre-clear snapshot must never be applied; the empty snapshot
    // from clearAll stays the last one written.
    expect(mockState.setQueueEntries).not.toHaveBeenCalledWith(
      SESSION_ID,
      [entry({ id: "pre-clear" })],
      { count: 1, max: 10 },
    );
  });
});

describe("useQueue removeEntry", () => {
  beforeEach(() => {
    resetMockState();
    setDocumentVisibility("visible");
    queueApiMock.removeQueuedEntry.mockReset();
    queueApiMock.getQueueStatus.mockResolvedValue({ entries: [], count: 0, max: 10 });
  });

  it("optimistically removes then refetches authoritative status after success", async () => {
    queueApiMock.removeQueuedEntry.mockResolvedValueOnce({ entry_id: "q-1" });
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();

    await act(async () => {
      await result.current.removeEntry("q-1");
    });

    expect(mockState.removeQueueEntry).toHaveBeenCalledWith(SESSION_ID, "q-1");
    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(SESSION_ID);
  });

  it("refetches after a drain race without surfacing a benign error", async () => {
    queueApiMock.removeQueuedEntry.mockRejectedValueOnce(
      new queueApiMock.QueueEntryNotFoundError(),
    );
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();

    await act(async () => {
      await expect(result.current.removeEntry("q-1")).resolves.toBeUndefined();
    });

    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(SESSION_ID);
  });

  it("refetches and rethrows a failed removal", async () => {
    queueApiMock.removeQueuedEntry.mockRejectedValueOnce(new Error("remove failed"));
    const authoritative = entry({ id: "q-1" });
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();
    queueApiMock.getQueueStatus.mockResolvedValueOnce({
      entries: [authoritative],
      count: 1,
      max: 10,
    });

    await act(async () => {
      await expect(result.current.removeEntry("q-1")).rejects.toThrow("remove failed");
    });

    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(SESSION_ID);
    expect(mockState.setQueueEntries).toHaveBeenCalledWith(SESSION_ID, [authoritative], {
      count: 1,
      max: 10,
    });
  });
});
