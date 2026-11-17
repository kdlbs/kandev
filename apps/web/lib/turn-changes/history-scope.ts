import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";
import { projectTurnChangeSummary } from "./projection";

export type TurnChangeScopeSelection =
  | { kind: "current" }
  | { kind: "historical"; changeSetId: string; scopeValue: string };

export function resolveTurnChangeScope(
  value: string,
  summaries: TurnChangeSetSummary[],
): TurnChangeScopeSelection | null {
  if (value === "current") return { kind: "current" };
  if (value === "latest") {
    const latest = summaries
      .filter((summary) => projectTurnChangeSummary(summary).visible)
      .sort((a, b) => b.turn_ordinal - a.turn_ordinal)[0];
    return latest ? { kind: "historical", changeSetId: latest.id, scopeValue: "latest" } : null;
  }
  const selected = summaries.find(
    (summary) => summary.id === value && projectTurnChangeSummary(summary).visible,
  );
  return selected
    ? { kind: "historical", changeSetId: selected.id, scopeValue: selected.id }
    : null;
}
