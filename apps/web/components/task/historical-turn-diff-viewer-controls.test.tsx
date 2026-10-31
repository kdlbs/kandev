import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";
import { HistoricalViewerHeader } from "./historical-turn-diff-viewer-controls";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: translate }),
}));

afterEach(cleanup);

const translations: Record<string, string> = {
  "task:turnChangesHistoricalTitle": "Turn",
  "task:turnChangesFileCount": "{{count}} changed files",
  "task:turnChangesRepositoryChangeCount": "{{count}} repository changes",
  "task:turnChangesAvailableChangeCount": "{{count}} available changes",
  "task:turnChangesPartial": "Partial",
  "task:turnChangesUnavailable": "This turn's change summary is unavailable.",
  "task:turnChangesSharedCheckout": "Shared checkout",
};

function translate(key: string, options?: Record<string, unknown>): string {
  const value = translations[key] ?? key;
  return value.replace("{{count}}", String(options?.count ?? ""));
}

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
    turn_ordinal: 2,
    fallback_anchor: "turn-changes:turn-1",
    file_count: 1,
    binary_file_count: 0,
    unknown_count_file_count: 0,
    repository_count: 1,
    repositories: [],
    ...overrides,
  };
}

it("uses repository wording for a legacy historical header", () => {
  render(<HistoricalViewerHeader summary={summary({ legacy_counts: true, file_count: 320 })} />);
  expect(screen.getByText(/^320 repository changes/)).toBeTruthy();
  expect(screen.queryByText(/^320 changed files/)).toBeNull();
});

it("hides numeric totals and Partial for an all-unavailable comparison", () => {
  render(
    <HistoricalViewerHeader
      summary={summary({
        availability: "unavailable",
        counts_available: false,
        summary_complete: false,
        file_count: 0,
        reason: "history_conflict",
      })}
    />,
  );
  expect(screen.queryByText(/changed files/)).toBeNull();
  expect(screen.queryByText("Partial")).toBeNull();
});

it("uses available-change wording for a mixed summary", () => {
  render(<HistoricalViewerHeader summary={summary({ summary_complete: false, file_count: 2 })} />);
  expect(screen.getByText(/^2 available changes/)).toBeTruthy();
  expect(screen.getByText("Partial")).toBeTruthy();
});

it("keeps pending preservation quiet", () => {
  render(
    <HistoricalViewerHeader
      summary={summary({
        complete: false,
        content_complete: false,
        repositories: [
          {
            id: "repo-1",
            checkout_id: "checkout-1",
            availability: "ready",
            enumeration_complete: true,
            comparison_complete: true,
            content_complete: false,
            export_state: "queued",
          },
        ],
      })}
    />,
  );
  expect(screen.queryByText("Partial")).toBeNull();
  expect(screen.getByText(/^1 changed files/)).toBeTruthy();
});
