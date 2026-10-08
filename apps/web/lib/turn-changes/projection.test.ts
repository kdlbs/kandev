import { describe, expect, it } from "vitest";
import type { TurnChangeSetSummary, TurnRepositoryChange } from "@/lib/types/turn-changes";
import { projectTurnChangeSummary, projectTurnRepositoryAvailability } from "./projection";

function summary(overrides: Partial<TurnChangeSetSummary> = {}): TurnChangeSetSummary {
  return {
    id: "set-1",
    task_id: "task-1",
    session_id: "session-1",
    turn_id: "turn-1",
    revision: 1,
    availability: "ready",
    complete: true,
    summary_complete: true,
    content_complete: true,
    turn_ordinal: 1,
    terminal_at: "2026-10-08T10:00:00Z",
    fallback_anchor: "turn-changes:turn-1",
    file_count: 1,
    binary_file_count: 0,
    unknown_count_file_count: 0,
    repository_count: 0,
    repositories: [],
    ...overrides,
  };
}

describe("projectTurnChangeSummary", () => {
  it.each([
    [{ reason: "capture_disabled" }, false],
    [{ reason: "unsupported_executor" }, false],
    [{ reason: "no_git_repository" }, false],
    [{ availability: "unsupported" }, false],
    [{ availability: "ready", file_count: 0 }, false],
    [{ availability: "failed" }, true],
    [{ availability: "unavailable" }, true],
    [{ availability: "pending" }, true],
    [{ availability: "expired" }, true],
    [{ terminal_at: undefined, terminal_outcome: undefined }, false],
  ] as const)("projects %j", (overrides, visible) => {
    expect(projectTurnChangeSummary(summary(overrides)).visible).toBe(visible);
  });
});

describe("projectTurnRepositoryAvailability", () => {
  it.each([
    [{ availability: "ready", comparison_complete: true, content_complete: true }, "ready"],
    [{ availability: "ready", comparison_complete: false, content_complete: false }, "partial"],
    [
      { availability: "ready", comparison_complete: true, content_complete: false },
      "content_unavailable",
    ],
    [
      { availability: "unavailable", comparison_complete: false, content_complete: false },
      "unavailable",
    ],
  ] as const)("projects %j", (overrides, expected) => {
    const repository = Object.assign(
      {
        id: "repo-1",
        checkout_id: "checkout-1",
        availability: "ready" as const,
        enumeration_complete: true,
        comparison_complete: true,
        content_complete: true,
      },
      overrides,
    ) as TurnRepositoryChange;
    expect(projectTurnRepositoryAvailability(repository)).toBe(expected);
  });
});
