import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";

const mockHistory = vi.hoisted(() => {
  type Summary = TurnChangeSetSummary;
  type State = {
    taskSessions: { items: Record<string, object> };
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
