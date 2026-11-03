import type { MCPSessionSelectionState } from "@/lib/types/http-mcp";

export type MCPSelectionMarker = "pending" | "deferred" | "failed";
export function mcpSelectionMarker(
  state?: MCPSessionSelectionState | null,
): MCPSelectionMarker | null {
  switch (state?.apply_state) {
    case "pending_idle":
      return "pending";
    case "deferred_restart":
      return "deferred";
    case "failed":
      return "failed";
    default:
      return null;
  }
}
export function mcpSelectionInitialView(
  state?: MCPSessionSelectionState | null,
): "connections" | "customize" {
  return mcpSelectionMarker(state) ? "customize" : "connections";
}
export function mcpSelectionStateLabelKey(state: MCPSessionSelectionState): string {
  switch (state.apply_state) {
    case "pending_idle":
      return "settings:mcpSessionStatePending";
    case "deferred_restart":
      return "settings:mcpSessionStateDeferred";
    case "failed":
      return "settings:mcpSessionStateFailed";
    case "applied":
      return "settings:mcpSessionStateApplied";
  }
}

export function mcpSelectionPresentation(state: MCPSessionSelectionState) {
  const descriptions = {
    applied: "settings:mcpSessionStateAppliedDescription",
    pending_idle: "settings:mcpSessionStatePendingDescription",
    deferred_restart: "settings:mcpSessionStateDeferredDescription",
    failed: "settings:mcpSessionStateFailedDescription",
  };
  return {
    labelKey: mcpSelectionStateLabelKey(state),
    descriptionKey: descriptions[state.apply_state],
    tone: state.apply_state,
  };
}
