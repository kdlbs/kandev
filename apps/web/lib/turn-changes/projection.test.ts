import { describe, expect, it } from "vitest";
import type { TurnChangeSetSummary, TurnRepositoryChange } from "@/lib/types/turn-changes";
import {
  isTurnChangeCardVisible,
  isUnavailableHistoryComparison,
  projectTurnChangeSummary,
  projectTurnRepositoryAvailability,
} from "./projection";

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
      {
        availability: "ready",
        comparison_complete: true,
        content_complete: false,
        preservation_pending: true,
      },
      "pending",
    ],
    [
      {
        availability: "ready",
        comparison_complete: true,
        content_complete: false,
        export_state: "queued",
      },
      "ready",
    ],
    [
      {
        availability: "ready",
        comparison_complete: true,
        content_complete: false,
        export_state: "unavailable",
      },
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

describe("turn change transcript eligibility", () => {
  it.each(["ready", "expired"] as const)(
    "hides settled %s captures without recorded files",
    (availability) => {
      expect(isTurnChangeCardVisible(summary({ availability, file_count: 0 }))).toBe(false);
      expect(isTurnChangeCardVisible(summary({ availability, file_count: 1 }))).toBe(true);
    },
  );

  it("keeps preparation visible and respects active/disabled exclusions", () => {
    expect(isTurnChangeCardVisible(summary({ availability: "pending", file_count: 0 }))).toBe(true);
    expect(isTurnChangeCardVisible(summary({ reason: "capture_disabled" }))).toBe(false);
    expect(isTurnChangeCardVisible(summary({ terminal_at: undefined }))).toBe(false);
  });

  it("keeps usable partial summaries visible", () => {
    const partial = summary({ summary_complete: false, file_count: 2 });
    expect(isTurnChangeCardVisible(partial)).toBe(true);
  });

  it("keeps an unavailable empty turn in history while hiding its chat card", () => {
    const unavailable = summary({ availability: "unavailable", file_count: 0 });
    expect(projectTurnChangeSummary(unavailable).visible).toBe(true);
    expect(isTurnChangeCardVisible(unavailable)).toBe(false);
  });
});

describe("unavailable and failed transcript eligibility", () => {
  it.each(["unavailable", "failed"] as const)(
    "hides %s summaries with zero or retained files",
    (availability) => {
      expect(isTurnChangeCardVisible(summary({ availability, file_count: 0 }))).toBe(false);
      expect(isTurnChangeCardVisible(summary({ availability, file_count: 3 }))).toBe(false);
    },
  );

  function historyComparison(reason: string): TurnChangeSetSummary {
    return summary({
      availability: "unavailable",
      counts_available: false,
      file_count: 0,
      reason,
      repositories: [
        {
          id: "repo-1",
          checkout_id: "checkout-1",
          availability: "unavailable",
          comparison_basis: "history_unavailable",
          enumeration_complete: false,
          comparison_complete: false,
          content_complete: false,
        },
      ],
    });
  }

  it.each(["history_conflict", "history_context_changed"] as const)(
    "hides an unavailable history comparison with reason %s",
    (reason) => {
      const unavailable = historyComparison(reason);
      expect(projectTurnChangeSummary(unavailable).visible).toBe(true);
      expect(isTurnChangeCardVisible(unavailable)).toBe(false);
    },
  );

  it("retains the unavailable history projection for viewer diagnostics", () => {
    expect(isUnavailableHistoryComparison(historyComparison("history_conflict"))).toBe(true);
  });

  it("hides an ordinary unavailable capture that retains recorded files", () => {
    const captureFailure = summary({
      availability: "unavailable",
      counts_available: false,
      file_count: 2,
      reason: "checkout_unavailable",
      repositories: [],
    });
    expect(isTurnChangeCardVisible(captureFailure)).toBe(false);
  });
});
