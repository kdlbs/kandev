import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { QueueOperationToken } from "@/lib/state/slices/session/types";

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

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      queue: {
        bySessionId: {},
        metaBySessionId: {},
        activeOperationBySessionId: {} as Record<string, QueueOperationToken>,
      },
      connection: { status: "connected" },
      taskSessions: {
        items: {
          [SESSION_ID]: { task_id: "task-retry", queue_incarnation_id: "incarnation-retry" },
        },
      },
      setQueueEntries: vi.fn(),
      removeQueueEntry: vi.fn(),
      beginQueueOperation: vi.fn().mockReturnValue({
        sessionIncarnationId: "incarnation-retry",
        generation: 1,
      }),
      finishQueueOperation: vi.fn(),
    }),
}));
vi.mock("@/hooks/use-foreground-refresh", () => ({ useForegroundRefresh: vi.fn() }));
vi.mock("@/lib/api/domains/queue-api", () => queueApiMock);

import { useQueue } from "./use-queue";

const SESSION_ID = "retry-session";

beforeEach(() => {
  queueApiMock.getQueueStatus.mockResolvedValue({
    task_id: "task-retry",
    session_id: SESSION_ID,
    session_incarnation_id: "incarnation-retry",
    entries: [],
    count: 0,
    max: 10,
  });
  queueApiMock.updateQueuedMessage.mockResolvedValue({ entry_id: "q-1", target_revision: 2 });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useQueue edit retries", () => {
  it("reuses the operation ID for the same leased request", async () => {
    const { result } = renderHook(() => useQueue(SESSION_ID));
    await waitFor(() => expect(queueApiMock.getQueueStatus).toHaveBeenCalled());
    const lease = { lease_id: "lease-1", target_revision: 1 };

    await act(async () => {
      await Promise.all([
        result.current.editEntry("q-1", "retryable edit", undefined, [], lease as never),
        result.current.editEntry("q-1", "retryable edit", undefined, [], lease as never),
      ]);
    });

    const calls = queueApiMock.updateQueuedMessage.mock.calls;
    expect(calls[1][0].operation_id).toBe(calls[0][0].operation_id);
  });
});
