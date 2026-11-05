import type {
  MCPSetupChange,
  MCPSetupServer,
  MCPSessionSelectionState,
} from "@/lib/types/http-mcp";

export type MCPSelectionChanges =
  | { status: "hidden" | "unavailable"; changes: [] }
  | { status: "known"; changes: MCPSetupChange[] };

function validServer(server: MCPSetupServer | undefined, id: string): boolean {
  return Boolean(
    server &&
    server.definition_id === id &&
    Number.isSafeInteger(server.revision) &&
    server.revision > 0 &&
    typeof server.enabled === "boolean" &&
    typeof server.display_name === "string" &&
    server.display_name.trim() &&
    typeof server.runtime_name === "string" &&
    server.runtime_name.trim(),
  );
}

function validChange(change: MCPSetupChange): boolean {
  if (!change || typeof change.definition_id !== "string" || !change.definition_id) return false;
  switch (change.action) {
    case "add":
      return !change.before && validServer(change.after, change.definition_id);
    case "remove":
      return !change.after && validServer(change.before, change.definition_id);
    case "update":
      return (
        validServer(change.before, change.definition_id) &&
        validServer(change.after, change.definition_id)
      );
    default:
      return false;
  }
}

function sameSelections(comparisonIds: string[], selectedIds: string[]): boolean {
  if (!Array.isArray(comparisonIds) || comparisonIds.some((id) => typeof id !== "string"))
    return false;
  const comparison = new Set(comparisonIds);
  const selected = new Set(selectedIds);
  return comparison.size === selected.size && [...selected].every((id) => comparison.has(id));
}

export function mcpSelectionChanges(
  state: MCPSessionSelectionState | null | undefined,
  selectedIds: string[],
  excludedIds: string[] = [],
): MCPSelectionChanges {
  if (!state || state.apply_state === "applied") return { status: "hidden", changes: [] };
  const comparison = state.comparison;
  const changes = comparison?.changes ?? [];
  if (!matchingComparison(state, selectedIds, excludedIds) || !validChanges(changes))
    return { status: "unavailable", changes: [] };
  return { status: "known", changes };
}

function matchingComparison(
  state: MCPSessionSelectionState,
  selectedIds: string[],
  excludedIds: string[],
): boolean {
  const comparison = state.comparison;
  return Boolean(
    comparison?.version === 1 &&
    comparison.status === "known" &&
    comparison.desired_revision === state.desired_revision &&
    comparison.applied_revision === state.applied_revision &&
    (comparison.attachment_attempt_id ?? "") === (state.attachment_attempt_id ?? "") &&
    sameSelections(comparison.selection_ids ?? [], selectedIds) &&
    sameSelections(comparison.excluded_definition_ids ?? [], excludedIds),
  );
}

function validChanges(changes: MCPSetupChange[]): boolean {
  return (
    Array.isArray(changes) &&
    changes.length <= 1024 &&
    changes.every(validChange) &&
    new Set(changes.map((change) => change.definition_id)).size === changes.length
  );
}
