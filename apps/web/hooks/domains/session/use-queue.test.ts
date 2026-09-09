import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { QueuedMessage, QueueOperationToken } from "@/lib/state/slices/session/types";
import type { EntityReference } from "@/lib/types/entity-reference";

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
    items: Record<
      string,
      {
        task_id?: string;
        queue_incarnation_id?: string;
        cancellation_pending?: boolean;
      }
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
}

beforeEach(() => {
  resetMockState();
  setDocumentVisibility("visible");
  queueApiMock.getQueueStatus.mockResolvedValue({ ...IDENTITY, entries: [], count: 0, max: 10 });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useQueue", () => {
  it("refetches the queue snapshot when the WebSocket reconnects", async () => {
    mockState.connection.status = "disconnected";
    const { rerender } = renderHook(() => useQueue(SESSION_ID));

    await act(async () => {});
    expect(queueApiMock.getQueueStatus).not.toHaveBeenCalled();

    mockState.connection.status = "connected";
    rerender();

    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY));
    expect(mockState.setQueueEntries).toHaveBeenCalledWith(
      SESSION_ID,
      [],
      {
        count: 0,
        max: 10,
        mergeEnabled: true,
        autoRun: true,
        taskId: TASK_ID,
        sessionIncarnationId: IDENTITY.session_incarnation_id,
      },
      { establishStatusEpoch: true },
    );
  });

  it("rejects an identity-less queue snapshot after session identity is known", async () => {
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    mockState.setQueueEntries.mockClear();
    queueApiMock.getQueueStatus.mockResolvedValueOnce({ entries: [entry()], count: 1, max: 10 });

    await act(async () => {
      await result.current.refetch();
    });

    expect(mockState.setQueueEntries).not.toHaveBeenCalled();
  });

  it("refetches a stale queue snapshot when a suspended tab becomes visible again", async () => {
    mockState.queue.bySessionId[SESSION_ID] = [entry()];
    mockState.queue.metaBySessionId[SESSION_ID] = { count: 1, max: 10 };
    queueApiMock.getQueueStatus.mockResolvedValueOnce({
      ...IDENTITY,
      entries: [entry()],
      count: 1,
      max: 10,
    });

    renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledTimes(1));

    queueApiMock.getQueueStatus.mockClear();
    mockState.setQueueEntries.mockClear();
    queueApiMock.getQueueStatus.mockResolvedValueOnce({
      ...IDENTITY,
      entries: [],
      count: 0,
      max: 10,
    });

    document.dispatchEvent(new Event("visibilitychange"));

    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY));
    expect(mockState.setQueueEntries).toHaveBeenCalledWith(
      SESSION_ID,
      [],
      {
        count: 0,
        max: 10,
        mergeEnabled: true,
        autoRun: true,
        taskId: TASK_ID,
        sessionIncarnationId: IDENTITY.session_incarnation_id,
      },
      { establishStatusEpoch: true },
    );
  });

  it("refetches a stale queue snapshot when the Kandev window regains focus", async () => {
    renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledTimes(1));
    queueApiMock.getQueueStatus.mockClear();

    window.dispatchEvent(new Event("focus"));

    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY));
  });

  it("does not refetch on foreground visibility while disconnected", async () => {
    mockState.connection.status = "disconnected";
    renderHook(() => useQueue(SESSION_ID));

    await act(async () => {});
    document.dispatchEvent(new Event("visibilitychange"));

    expect(queueApiMock.getQueueStatus).not.toHaveBeenCalled();
  });
});

describe("useQueue message metadata", () => {
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
      session_incarnation_id: IDENTITY.session_incarnation_id,
      task_id: TASK_ID,
      content: "queued reference",
      model: undefined,
      plan_mode: undefined,
      attachments: undefined,
      entity_references: [reference],
    });
  });

  it("refetches authoritative status after a queue admission error", async () => {
    const mutationError = new Error("queue failed");
    queueApiMock.queueMessage.mockRejectedValueOnce(mutationError);
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();

    await act(async () => {
      await expect(result.current.queue({ taskId: TASK_ID, content: "rejected" })).rejects.toBe(
        mutationError,
      );
    });

    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
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
      task_id: TASK_ID,
      session_incarnation_id: IDENTITY.session_incarnation_id,
      entry_id: "q-1",
      content: "reference removed",
      attachments: undefined,
      entity_references: [],
    });
  });
});

describe("useQueue context file metadata and Send Now", () => {
  it("forwards context file metadata with queued messages", async () => {
    queueApiMock.queueMessage.mockResolvedValue(entry());
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.queueMessage.mockClear();

    await act(async () => {
      await result.current.queue({
        taskId: TASK_ID,
        content: "queued context",
        contextFilesMeta: [{ path: "src/components", name: "components", is_directory: true }],
      } as never);
    });

    expect(queueApiMock.queueMessage).toHaveBeenCalledWith({
      session_id: SESSION_ID,
      session_incarnation_id: IDENTITY.session_incarnation_id,
      task_id: TASK_ID,
      content: "queued context",
      model: undefined,
      plan_mode: undefined,
      attachments: undefined,
      entity_references: undefined,
      context_files: [{ path: "src/components", name: "components", is_directory: true }],
    });
  });

  it("sends one exact entry now and refetches authoritative status", async () => {
    queueApiMock.sendQueuedNow.mockResolvedValue({
      session_id: SESSION_ID,
      dispatched: true,
      sent_count: 1,
    });
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();

    await act(async () => {
      await result.current.sendEntryNow("q-2");
    });

    expect(queueApiMock.sendQueuedNow).toHaveBeenCalledWith({
      session_id: SESSION_ID,
      task_id: TASK_ID,
      session_incarnation_id: IDENTITY.session_incarnation_id,
      scope: "entry",
      entry_id: "q-2",
    });
    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
  });
});

describe("useQueue automation controls", () => {
  it("refetches after an Auto-run mutation failure and preserves its error", async () => {
    const mutationError = new Error("policy update failed");
    queueApiMock.setQueueAutoRun.mockRejectedValueOnce(mutationError);
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();

    await act(async () => {
      await expect(result.current.setAutoRun(false)).rejects.toBe(mutationError);
    });

    expect(queueApiMock.setQueueAutoRun).toHaveBeenCalledWith(IDENTITY, false);
    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
  });

  it("exposes authoritative cancellation progress for disabling controls", async () => {
    mockState.taskSessions.items[SESSION_ID] = {
      task_id: TASK_ID,
      queue_incarnation_id: IDENTITY.session_incarnation_id,
      cancellation_pending: true,
    };
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());

    expect(result.current.cancellationPending).toBe(true);
  });

  it("sets Auto-run and refetches authoritative queue status", async () => {
    queueApiMock.setQueueAutoRun.mockResolvedValue({
      session_id: SESSION_ID,
      auto_run: false,
      dispatched: false,
    });
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();
    const setAutoRun = (
      result.current as typeof result.current & {
        setAutoRun?: (enabled: boolean) => Promise<void>;
      }
    ).setAutoRun;

    expect(setAutoRun).toBeTypeOf("function");
    await act(async () => {
      await setAutoRun!(false);
    });

    expect(queueApiMock.setQueueAutoRun).toHaveBeenCalledWith(IDENTITY, false);
    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
  });
  it("sets Auto-merge for the immutable session and refetches policy", async () => {
    queueApiMock.getQueueStatus.mockResolvedValue({
      ...IDENTITY,
      entries: [],
      count: 0,
      max: 10,
      ...IDENTITY,
      merge_enabled: true,
      auto_run: true,
      auto_merge_available: true,
      auto_merge_enabled: true,
      auto_merge_source: "global",
      auto_merge_revision: 4,
    });
    queueApiMock.setQueueAutoMerge.mockResolvedValue({
      ...IDENTITY,
      auto_merge_enabled: false,
      auto_merge_source: "session",
      auto_merge_revision: 1,
    });
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(mockState.setQueueEntries).toHaveBeenCalled());
    expect(mockState.setQueueEntries).toHaveBeenLastCalledWith(
      SESSION_ID,
      [],
      expect.objectContaining({
        autoMergeAvailable: true,
        autoMergeEnabled: true,
        autoMergeSource: "global",
        autoMergeRevision: 4,
      }),
      { establishStatusEpoch: true },
    );

    await act(async () => {
      await result.current.setAutoMerge(false);
    });

    expect(queueApiMock.setQueueAutoMerge).toHaveBeenCalledWith(IDENTITY, false);
    expect(queueApiMock.getQueueStatus).toHaveBeenLastCalledWith(IDENTITY);
  });
});

describe("useQueue mergeEntry", () => {
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
      task_id: TASK_ID,
      session_incarnation_id: IDENTITY.session_incarnation_id,
      entry_id: "q-2",
      user_id: undefined,
    });
    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
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
      task_id: TASK_ID,
      session_incarnation_id: IDENTITY.session_incarnation_id,
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

    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
  });
});

describe("useQueue clearAll", () => {
  beforeEach(() => {
    queueApiMock.clearQueue.mockReset();
    queueApiMock.clearQueue.mockResolvedValue(undefined);
  });

  it("refetches authoritative status after a successful clear", async () => {
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();

    await act(async () => {
      await result.current.clearAll();
    });

    expect(queueApiMock.clearQueue).toHaveBeenCalledWith(IDENTITY);
    expect(mockState.setQueueEntries.mock.calls).toContainEqual([
      SESSION_ID,
      [],
      {
        count: 0,
        max: 0,
        mergeEnabled: true,
        autoRun: true,
        taskId: TASK_ID,
        sessionIncarnationId: IDENTITY.session_incarnation_id,
      },
    ]);
    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
  });

  it("refetches authoritative status and rethrows when clear fails", async () => {
    queueApiMock.clearQueue.mockRejectedValueOnce(new Error("clear failed"));
    const authoritative = entry({ id: "still-queued" });
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();
    queueApiMock.getQueueStatus.mockResolvedValueOnce({
      ...IDENTITY,
      entries: [authoritative],
      count: 1,
      max: 10,
    });

    await act(async () => {
      await expect(result.current.clearAll()).rejects.toThrow("clear failed");
    });

    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
    expect(mockState.setQueueEntries).toHaveBeenCalledWith(
      SESSION_ID,
      [authoritative],
      {
        count: 1,
        max: 10,
        mergeEnabled: true,
        autoRun: true,
        taskId: TASK_ID,
        sessionIncarnationId: IDENTITY.session_incarnation_id,
      },
      { establishStatusEpoch: true },
    );
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
      { count: 1, max: 10, mergeEnabled: true, autoRun: true },
    );
  });
});

describe("useQueue removeEntry", () => {
  beforeEach(() => {
    queueApiMock.removeQueuedEntry.mockReset();
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
    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
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

    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
  });

  it("refetches and rethrows a failed removal", async () => {
    queueApiMock.removeQueuedEntry.mockRejectedValueOnce(new Error("remove failed"));
    const authoritative = entry({ id: "q-1" });
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    queueApiMock.getQueueStatus.mockClear();
    queueApiMock.getQueueStatus.mockResolvedValueOnce({
      ...IDENTITY,
      entries: [authoritative],
      count: 1,
      max: 10,
    });

    await act(async () => {
      await expect(result.current.removeEntry("q-1")).rejects.toThrow("remove failed");
    });

    expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(IDENTITY);
    expect(mockState.setQueueEntries).toHaveBeenCalledWith(
      SESSION_ID,
      [authoritative],
      {
        count: 1,
        max: 10,
        mergeEnabled: true,
        autoRun: true,
        taskId: TASK_ID,
        sessionIncarnationId: IDENTITY.session_incarnation_id,
      },
      { establishStatusEpoch: true },
    );
  });
});
