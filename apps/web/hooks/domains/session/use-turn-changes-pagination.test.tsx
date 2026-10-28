import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";

const mockHistory = vi.hoisted(() => {
  type Summary = TurnChangeSetSummary;
  type State = {
    taskSessions: { items: Record<string, object> };
    connection: { status: string };
    turnChanges: {
      bySession: Record<string, Summary[]>;
      loadedBySession: Record<string, boolean>;
      nextOffsetBySession: Record<string, number | null>;
    };
    mergeTurnChangePage: (
      sessionId: string,
      summaries: Summary[],
      cursor: { offset: number; nextOffset?: number },
    ) => void;
    mergeTurnChangeSummary: (sessionId: string, summary: Summary) => void;
  };
  const state: State = {
    taskSessions: { items: { "session-1": {} } },
    connection: { status: "disconnected" },
    turnChanges: {
      bySession: { "session-1": [] as Summary[] },
      loadedBySession: { "session-1": true },
      nextOffsetBySession: { "session-1": 50 },
    },
    mergeTurnChangePage: () => {},
    mergeTurnChangeSummary: () => {},
  };
  state.mergeTurnChangePage = (
    sessionId: string,
    summaries: Summary[],
    cursor: { offset: number; nextOffset?: number },
  ) => {
    const current = state.turnChanges.bySession[sessionId] ?? [];
    const merged = new Map<string, Summary>(current.map((item) => [item.id, item]));
    summaries.forEach((item) => merged.set(item.id, item));
    state.turnChanges.bySession[sessionId] = [...merged.values()].sort(
      (a, b) => a.turn_ordinal - b.turn_ordinal,
    );
    state.turnChanges.loadedBySession[sessionId] = true;
    state.turnChanges.nextOffsetBySession[sessionId] = cursor.nextOffset ?? null;
  };
  state.mergeTurnChangeSummary = (sessionId: string, summary: Summary) => {
    const current = state.turnChanges.bySession[sessionId] ?? [];
    state.turnChanges.bySession[sessionId] = [
      ...current.filter((item) => item.id !== summary.id),
      summary,
    ].sort((a, b) => a.turn_ordinal - b.turn_ordinal);
  };
  return {
    state,
    store: { getState: () => state },
    listPage: vi.fn(),
    getItem: vi.fn(),
    subscribe: vi.fn(),
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockHistory.state) => unknown) =>
    selector(mockHistory.state),
  useAppStoreApi: () => mockHistory.store,
}));

vi.mock("@/lib/api/domains/turn-changes-api", () => ({
  listTurnChangeHistory: (...args: unknown[]) => mockHistory.listPage(...args),
  getTurnChangeHistoryItem: (...args: unknown[]) => mockHistory.getItem(...args),
}));

vi.mock("@/lib/ws/connection", () => {
  const client = { subscribeSessionWithReady: mockHistory.subscribe };
  return { useWebSocketClient: () => client };
});

import { clearTurnChangeRequestsForTest, useSessionTurnChanges } from "./use-turn-changes";

function summary(id: string, turnOrdinal: number): TurnChangeSetSummary {
  return {
    id,
    task_id: "task-1",
    session_id: "session-1",
    turn_id: `turn-${id}`,
    revision: 1,
    availability: "ready",
    complete: true,
    summary_complete: true,
    content_complete: true,
    turn_ordinal: turnOrdinal,
    fallback_anchor: `turn-changes:${id}`,
    file_count: 1,
    binary_file_count: 0,
    unknown_count_file_count: 0,
    repository_count: 0,
    repositories: [],
  };
}

function page(offset: number, changeSets: TurnChangeSetSummary[], nextOffset?: number) {
  return { change_sets: changeSets, total: 100, offset, limit: 50, next_offset: nextOffset };
}

beforeEach(() => {
  clearTurnChangeRequestsForTest();
  mockHistory.state.taskSessions.items = { "session-1": {} };
  mockHistory.state.connection.status = "disconnected";
  mockHistory.subscribe.mockReset();
  mockHistory.state.turnChanges = {
    bySession: {
      "session-1": Array.from({ length: 50 }, (_, index) =>
        summary(`recent-${index}`, 100 - index),
      ),
    },
    loadedBySession: { "session-1": true },
    nextOffsetBySession: { "session-1": 50 },
  };
  mockHistory.listPage.mockReset();
  mockHistory.getItem.mockReset();
});

// @covers AC-TASKS-TURN-CHANGES-004.8
describe("turn change subscription handoff", () => {
  it("refreshes an initial pending capture after subscription acknowledgment", async () => {
    let acknowledge!: () => void;
    const ready = new Promise<void>((resolve) => {
      acknowledge = resolve;
    });
    mockHistory.subscribe.mockReturnValue({ ready, unsubscribe: vi.fn() });
    mockHistory.state.connection.status = "connected";
    mockHistory.state.turnChanges.bySession["session-1"] = [];
    mockHistory.state.turnChanges.loadedBySession["session-1"] = false;
    const pending = {
      ...summary("capture", 1),
      revision: 3,
      availability: "pending" as const,
      file_count: 0,
      complete: false,
      summary_complete: false,
      content_complete: false,
    };
    const terminal = { ...summary("capture", 1), revision: 9, terminal_outcome: "completed" };
    mockHistory.listPage
      .mockResolvedValueOnce(page(0, [pending]))
      .mockResolvedValueOnce(page(0, [terminal]));
    const { result, rerender } = renderHook(() => useSessionTurnChanges("session-1"));
    await waitFor(() => expect(result.current.summaries[0]?.revision).toBe(3));
    await act(async () => {
      acknowledge();
    });
    await waitFor(() =>
      expect(mockHistory.state.turnChanges.bySession["session-1"][0]).toMatchObject({
        revision: 9,
        terminal_outcome: "completed",
        file_count: 1,
      }),
    );
    rerender();
    expect(result.current.summaries[0]?.revision).toBe(9);
    expect(mockHistory.listPage).toHaveBeenCalledTimes(2);
  });

  it("finishes a pre-acknowledgment read before taking the fresh snapshot", async () => {
    let acknowledge!: () => void;
    const ready = new Promise<void>((resolve) => {
      acknowledge = resolve;
    });
    let finishInitial!: (value: ReturnType<typeof page>) => void;
    const initial = new Promise<ReturnType<typeof page>>((resolve) => {
      finishInitial = resolve;
    });
    mockHistory.subscribe.mockReturnValue({ ready, unsubscribe: vi.fn() });
    mockHistory.state.connection.status = "connected";
    mockHistory.state.turnChanges.bySession["session-1"] = [];
    mockHistory.state.turnChanges.loadedBySession["session-1"] = false;
    const terminal = { ...summary("capture", 1), revision: 9, terminal_outcome: "completed" };
    mockHistory.listPage.mockReturnValueOnce(initial).mockResolvedValueOnce(page(0, [terminal]));
    renderHook(() => useSessionTurnChanges("session-1"));
    await waitFor(() => expect(mockHistory.listPage).toHaveBeenCalledTimes(1));
    await act(async () => {
      acknowledge();
    });
    expect(mockHistory.listPage).toHaveBeenCalledTimes(1);
    await act(async () => {
      finishInitial(
        page(0, [
          {
            ...summary("capture", 1),
            revision: 3,
            availability: "pending",
            file_count: 0,
            complete: false,
          },
        ]),
      );
    });
    await waitFor(() =>
      expect(mockHistory.state.turnChanges.bySession["session-1"][0]?.revision).toBe(9),
    );
    expect(mockHistory.listPage).toHaveBeenCalledTimes(2);
  });
});

describe("turn change subscription ownership", () => {
  it("loads initial history without a rich lease when detail demand is inactive", async () => {
    mockHistory.state.connection.status = "connected";
    mockHistory.state.turnChanges.loadedBySession["session-1"] = false;
    mockHistory.listPage.mockResolvedValue(page(0, []));
    renderHook(() => useSessionTurnChanges("session-1", { detailActive: false }));
    await waitFor(() => expect(mockHistory.listPage).toHaveBeenCalledTimes(1));
    expect(mockHistory.subscribe).not.toHaveBeenCalled();
  });

  it("leases the rich stream only while detail demand is active", async () => {
    mockHistory.state.connection.status = "connected";
    const unsubscribe = vi.fn();
    mockHistory.subscribe.mockReturnValue({ ready: Promise.resolve(), unsubscribe });
    mockHistory.listPage.mockResolvedValue(page(0, []));
    const { rerender } = renderHook(
      ({ detailActive }) => useSessionTurnChanges("session-1", { detailActive }),
      {
        initialProps: { detailActive: false },
      },
    );
    expect(mockHistory.subscribe).not.toHaveBeenCalled();
    rerender({ detailActive: true });
    await waitFor(() => expect(mockHistory.listPage).toHaveBeenCalledTimes(1));
    expect(mockHistory.subscribe).toHaveBeenCalledTimes(1);
    rerender({ detailActive: false });
    expect(unsubscribe).toHaveBeenCalledTimes(1);
  });
});

describe("turn change subscription lifetime", () => {
  it("refreshes a cached session after reconnecting", async () => {
    mockHistory.subscribe.mockReturnValue({ ready: Promise.resolve(), unsubscribe: vi.fn() });
    mockHistory.listPage.mockResolvedValue(page(0, [summary("reconnected", 1)]));
    const { rerender } = renderHook(() => useSessionTurnChanges("session-1"));
    expect(mockHistory.listPage).not.toHaveBeenCalled();
    mockHistory.state.connection.status = "connected";
    rerender();
    await waitFor(() => expect(mockHistory.listPage).toHaveBeenCalledTimes(1));
    mockHistory.state.connection.status = "disconnected";
    rerender();
    mockHistory.state.connection.status = "connected";
    rerender();
    await waitFor(() => expect(mockHistory.listPage).toHaveBeenCalledTimes(2));
    expect(mockHistory.subscribe).toHaveBeenCalledTimes(2);
  });

  it("ignores an old acknowledgment after changing sessions", async () => {
    let acknowledge!: () => void;
    const ready = new Promise<void>((resolve) => {
      acknowledge = resolve;
    });
    const unsubscribe = vi.fn();
    mockHistory.subscribe
      .mockReturnValueOnce({ ready, unsubscribe })
      .mockReturnValueOnce({ ready: Promise.resolve(), unsubscribe: vi.fn() });
    mockHistory.state.connection.status = "connected";
    mockHistory.state.taskSessions.items["session-2"] = {};
    mockHistory.state.turnChanges.loadedBySession["session-2"] = true;
    mockHistory.listPage.mockResolvedValue(page(0, []));
    const { rerender } = renderHook(({ sessionId }) => useSessionTurnChanges(sessionId), {
      initialProps: { sessionId: "session-1" },
    });
    rerender({ sessionId: "session-2" });
    await waitFor(() => expect(mockHistory.listPage).toHaveBeenCalledTimes(1));
    await act(async () => {
      acknowledge();
    });
    expect(unsubscribe).toHaveBeenCalledTimes(1);
    expect(mockHistory.listPage).toHaveBeenCalledTimes(1);
    expect(mockHistory.listPage).toHaveBeenCalledWith("session-2", 0, 50, { cache: "no-store" });
  });

  it("does not report an old refresh error after changing sessions", async () => {
    let rejectOld!: (error: Error) => void;
    const oldRefresh = new Promise<ReturnType<typeof page>>((_, reject) => {
      rejectOld = reject;
    });
    mockHistory.subscribe.mockReturnValue({ ready: Promise.resolve(), unsubscribe: vi.fn() });
    mockHistory.state.connection.status = "connected";
    mockHistory.state.taskSessions.items["session-2"] = {};
    mockHistory.state.turnChanges.loadedBySession["session-2"] = true;
    mockHistory.listPage.mockReturnValueOnce(oldRefresh).mockResolvedValueOnce(page(0, []));
    const { result, rerender } = renderHook(({ sessionId }) => useSessionTurnChanges(sessionId), {
      initialProps: { sessionId: "session-1" },
    });
    await waitFor(() => expect(mockHistory.listPage).toHaveBeenCalledTimes(1));
    rerender({ sessionId: "session-2" });
    await waitFor(() => expect(mockHistory.listPage).toHaveBeenCalledTimes(2));
    await act(async () => {
      rejectOld(new Error("old session failed"));
    });
    expect(result.current.error).toBeNull();
    expect(result.current.loading).toBe(false);
  });

  it("releases its subscription and skips refresh after unmount", async () => {
    let acknowledge!: () => void;
    const ready = new Promise<void>((resolve) => {
      acknowledge = resolve;
    });
    const unsubscribe = vi.fn();
    mockHistory.subscribe.mockReturnValue({ ready, unsubscribe });
    mockHistory.state.connection.status = "connected";
    const { unmount } = renderHook(() => useSessionTurnChanges("session-1"));
    unmount();
    await act(async () => {
      acknowledge();
    });
    expect(unsubscribe).toHaveBeenCalledTimes(1);
    expect(mockHistory.listPage).not.toHaveBeenCalled();
  });
});

describe("session turn change history hydration", () => {
  it("pages older summaries until visible transcript turns are hydrated", async () => {
    const older = summary("older", 1);
    mockHistory.listPage.mockResolvedValue(page(50, [older]));
    const { result } = renderHook(() => useSessionTurnChanges("session-1"));

    await act(async () => result.current.ensureTurnsLoaded([older.turn_id]));

    expect(mockHistory.listPage).toHaveBeenCalledWith("session-1", 50, 50, { cache: "no-store" });
    expect(
      mockHistory.state.turnChanges.bySession["session-1"].some(
        (item: TurnChangeSetSummary) => item.turn_id === older.turn_id,
      ),
    ).toBe(true);
    expect(mockHistory.state.turnChanges.nextOffsetBySession["session-1"]).toBeNull();
  });

  it("fetches an exact change-set target outside the first history page", async () => {
    const older = summary("older-exact", 1);
    mockHistory.getItem.mockResolvedValue(older);
    const { result } = renderHook(() => useSessionTurnChanges("session-1"));

    await act(async () => result.current.loadChangeSet(older.id));

    expect(mockHistory.getItem).toHaveBeenCalledWith("session-1", older.id, { cache: "no-store" });
    expect(
      mockHistory.state.turnChanges.bySession["session-1"].some(
        (item: TurnChangeSetSummary) => item.id === older.id,
      ),
    ).toBe(true);
  });
});
