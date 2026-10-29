import type { HistoricalTurnDiffTarget } from "@/lib/state/diff-target-types";
import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";

export function emptyHistoricalSummary(target: HistoricalTurnDiffTarget): TurnChangeSetSummary {
  return {
    id: target.changeSetId,
    task_id: "",
    session_id: target.sessionId,
    turn_id: "",
    revision: 0,
    availability: "pending",
    complete: false,
    summary_complete: false,
    content_complete: false,
    turn_ordinal: 0,
    fallback_anchor: "",
    file_count: 0,
    binary_file_count: 0,
    unknown_count_file_count: 0,
    repository_count: 0,
    repositories: [],
  };
}
