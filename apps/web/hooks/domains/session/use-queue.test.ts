import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { queueStatusQueryOptions } from "@/lib/query/query-options";
import type { QueueStatus, QueuedMessage } from "@/lib/state/slices/session/types";

const queueApiMock = vi.hoisted(() => {
  class QueueEntryNotFoundError extends Error {
    readonly code = "entry_not_found";

    constructor() {
      super("Queue entry was already drained or no longer exists.");
      this.name = "QueueEntryNotFoundError";
    }
  }

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

type MockState = {
  connection: { status: string };
};

let mockState: MockState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/lib/api/domains/queue-api", () => queueApiMock);

import { useQueue } from "./use-queue";

const SESSION_ID = "session-1";
const TASK_ID = "task-1";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
}

function createWrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client }, children);
  };
}

function entry(overrides: Partial<QueuedMessage> = {}): QueuedMessage {
  return {
    id: "entry-1",
    session_id: SESSION_ID,
    task_id: TASK_ID,
    content: "Queued prompt",
    plan_mode: false,
    queued_at: "2026-06-23T00:00:00Z",
    queued_by: "user",
    ...overrides,
  };
}

function seedQueue(client: QueryClient, status: QueueStatus) {
  client.setQueryData(queueStatusQueryOptions(SESSION_ID).queryKey, status);
}

function setDocumentVisibility(value: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    value,
  });
}

describe("useQueue", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockState = { connection: { status: "connected" } };
    setDocumentVisibility("visible");
    queueApiMock.getQueueStatus.mockResolvedValue({ entries: [], count: 0, max: 10 });
  });

  afterEach(() => {
    cleanup();
  });

  it("reads cached queue status from Query without queue Zustand state", () => {
    const client = createQueryClient();
    const queued = entry();
    seedQueue(client, { entries: [queued], count: 1, max: 3 });

    const { result } = renderHook(() => useQueue(SESSION_ID), {
      wrapper: createWrapper(client),
    });

    expect(result.current.entries).toEqual([queued]);
    expect(result.current.count).toBe(1);
    expect(result.current.max).toBe(3);
    expect(result.current.isFull).toBe(false);
  });

  it("optimistically removes an entry from the queue query cache", async () => {
    queueApiMock.removeQueuedEntry.mockResolvedValueOnce({ entry_id: "entry-1" });
    const client = createQueryClient();
    seedQueue(client, {
      entries: [entry(), entry({ id: "entry-2", content: "Second prompt" })],
      count: 2,
      max: 3,
    });
    const { result } = renderHook(() => useQueue(SESSION_ID), {
      wrapper: createWrapper(client),
    });

    await act(async () => {
      await result.current.removeEntry("entry-1");
    });

    expect(queueApiMock.removeQueuedEntry).toHaveBeenCalledWith({
      session_id: SESSION_ID,
      entry_id: "entry-1",
    });
    expect(client.getQueryData<QueueStatus>(queueStatusQueryOptions(SESSION_ID).queryKey)).toEqual({
      entries: [entry({ id: "entry-2", content: "Second prompt" })],
      count: 1,
      max: 3,
    });
  });

  it("refetches the queue snapshot when the WebSocket reconnects", async () => {
    mockState.connection.status = "disconnected";
    const { rerender } = renderHook(() => useQueue(SESSION_ID), {
      wrapper: createWrapper(createQueryClient()),
    });

    await act(async () => {});
    expect(queueApiMock.getQueueStatus).not.toHaveBeenCalled();

    mockState.connection.status = "connected";
    rerender();

    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(SESSION_ID));
  });

  it("refetches a stale queue snapshot when a suspended tab becomes visible again", async () => {
    renderHook(() => useQueue(SESSION_ID), {
      wrapper: createWrapper(createQueryClient()),
    });
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledTimes(1));

    queueApiMock.getQueueStatus.mockClear();
    queueApiMock.getQueueStatus.mockResolvedValueOnce({ entries: [], count: 0, max: 10 });

    document.dispatchEvent(new Event("visibilitychange"));

    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalledWith(SESSION_ID));
  });

  it("does not refetch on foreground visibility while disconnected", async () => {
    mockState.connection.status = "disconnected";
    renderHook(() => useQueue(SESSION_ID), {
      wrapper: createWrapper(createQueryClient()),
    });

    await act(async () => {});
    document.dispatchEvent(new Event("visibilitychange"));

    expect(queueApiMock.getQueueStatus).not.toHaveBeenCalled();
  });
});
