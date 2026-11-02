import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";
import type { TurnRepositoryChange } from "@/lib/types/turn-changes";

export type TurnRepositoryAvailabilityProjection =
  | "pending"
  | "ready"
  | "unavailable"
  | "failed"
  | "expired"
  | "partial"
  | "content_unavailable";

export function projectTurnRepositoryAvailability(
  repository: TurnRepositoryChange,
): TurnRepositoryAvailabilityProjection {
  if (repository.availability === "unsupported") return "unavailable";
  if (repository.availability !== "ready") return repository.availability;
  if (!repository.comparison_complete) return "partial";
  if (!repository.content_complete && repository.preservation_pending) return "pending";
  if (repository.export_state === "unavailable") return "content_unavailable";
  if (repository.export_state && repository.content_complete !== true) return "ready";
  if (!repository.content_complete) return "content_unavailable";
  return "ready";
}

export type TurnChangeSummaryProjection =
  | { visible: false; state: "hidden" }
  | { visible: true; state: "preparing" | "unavailable" | "expired" | "ready" };

export function isTurnChangeCardVisible(summary: TurnChangeSetSummary): boolean {
  const projection = projectTurnChangeSummary(summary);
  if (!projection.visible) return false;
  if (isUnavailableHistoryComparison(summary)) return true;
  return summary.availability === "pending" || summary.file_count > 0;
}

// An unavailable history comparison is the only zero-entry exception: the
// existing unavailable status explains why no turn totals exist.
export function isUnavailableHistoryComparison(summary: TurnChangeSetSummary): boolean {
  if (summary.availability !== "unavailable" && summary.availability !== "failed") return false;
  if (summary.counts_available !== false) return false;
  if ((summary.reason ?? "").startsWith("history_")) return true;
  return (summary.repositories ?? []).some(
    (repository) => repository.comparison_basis === "history_unavailable",
  );
}

export function projectTurnChangeSummary(
  summary: TurnChangeSetSummary,
): TurnChangeSummaryProjection {
  if (!summary.terminal_at && !summary.terminal_outcome) return { visible: false, state: "hidden" };
  if (
    summary.reason === "capture_disabled" ||
    summary.reason === "policy_read_failed" ||
    summary.reason === "unsupported_executor" ||
    summary.reason === "no_git_repository" ||
    summary.availability === "unsupported"
  ) {
    return { visible: false, state: "hidden" };
  }
  if (summary.availability === "pending") return { visible: true, state: "preparing" };
  if (summary.availability === "expired") return { visible: true, state: "expired" };
  if (summary.availability === "unavailable" || summary.availability === "failed") {
    return { visible: true, state: "unavailable" };
  }
  if (summary.availability === "ready" && summary.file_count === 0) {
    return { visible: false, state: "hidden" };
  }
  return { visible: true, state: "ready" };
}

export function isTurnChangePartial(summary: TurnChangeSetSummary): boolean {
  if (summary.availability === "failed" || summary.summary_complete === false) return true;
  const repositories = summary.repositories ?? [];
  if (
    repositories.some(
      (repository) =>
        repository.availability === "ready" && repository.export_state === "unavailable",
    )
  ) {
    return true;
  }
  return (
    summary.content_complete === false &&
    repositories.every((repository) => !repository.export_state)
  );
}
