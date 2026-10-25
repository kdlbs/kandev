import { describe, expect, it } from "vitest";
import { historyPageCursor, selectTurnChangeSummaries } from "./use-turn-changes";
import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";

function summary(id: string): TurnChangeSetSummary {
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
    turn_ordinal: 1,
    fallback_anchor: `turn-changes:${id}`,
    file_count: 0,
    binary_file_count: 0,
    unknown_count_file_count: 0,
    repository_count: 0,
    repositories: [],
  };
}

describe("selectTurnChangeSummaries", () => {
  it("returns one stable empty value before session hydration and for unloaded sessions", () => {
    const state = { bySession: {} };
    const first = selectTurnChangeSummaries(state, null);

    expect(selectTurnChangeSummaries(state, null)).toBe(first);
    expect(selectTurnChangeSummaries(state, "session-1")).toBe(first);
  });

  it("returns the session's existing summary array", () => {
    const summaries = [summary("change-set-1")];

    expect(selectTurnChangeSummaries({ bySession: { "session-1": summaries } }, "session-1")).toBe(
      summaries,
    );
  });
});

describe("historyPageCursor", () => {
  it("maps the snake-case HTTP cursor into the session store shape", () => {
    expect(
      historyPageCursor({ change_sets: [], total: 90, offset: 50, limit: 50, next_offset: 100 }),
    ).toEqual({
      offset: 50,
      nextOffset: 100,
    });
  });
});
