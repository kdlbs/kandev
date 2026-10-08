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
  if (!repository.content_complete) return "content_unavailable";
  return "ready";
}

export type TurnChangeSummaryProjection =
  | { visible: false; state: "hidden" }
  | { visible: true; state: "preparing" | "unavailable" | "expired" | "ready" };

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
