import { describe, expect, it } from "vitest";
import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";
import { resolveTurnChangeScope } from "./history-scope";

function summary(
  id: string,
  turnOrdinal: number,
  overrides: Partial<TurnChangeSetSummary> = {},
): TurnChangeSetSummary {
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
    terminal_at: "2026-10-08T10:00:00Z",
    fallback_anchor: `turn-changes:${id}`,
    file_count: 1,
    binary_file_count: 0,
    unknown_count_file_count: 0,
    repository_count: 0,
    repositories: [],
    ...overrides,
  };
}

describe("resolveTurnChangeScope", () => {
  it("resolves latest once to the newest captured change-set ID", () => {
    const initial = [
      summary("turn-7", 7),
      summary("turn-8-disabled", 8, { reason: "capture_disabled" }),
      summary("turn-9-zero", 9, { file_count: 0 }),
    ];
    const selected = resolveTurnChangeScope("latest", initial);
    expect(selected).toEqual({ kind: "historical", changeSetId: "turn-7", scopeValue: "latest" });

    const laterHistory = [...initial, summary("turn-10", 10)];
    expect(selected?.kind === "historical" ? selected.changeSetId : null).toBe("turn-7");
    expect(resolveTurnChangeScope("latest", laterHistory)?.kind).toBe("historical");
    expect(resolveTurnChangeScope("turn-7", laterHistory)).toEqual({
      kind: "historical",
      changeSetId: "turn-7",
      scopeValue: "turn-7",
    });
  });

  it("returns to current changes and ignores uncaptured turns", () => {
    expect(resolveTurnChangeScope("current", [summary("turn-1", 1)])).toEqual({ kind: "current" });
    expect(
      resolveTurnChangeScope("turn-2", [summary("turn-2", 2, { reason: "capture_disabled" })]),
    ).toBeNull();
    expect(resolveTurnChangeScope("latest", [])).toBeNull();
  });
});
