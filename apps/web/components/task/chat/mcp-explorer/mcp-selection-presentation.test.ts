import { expect, it } from "vitest";
import type { MCPSessionApplyState } from "@/lib/types/http-mcp";
import {
  mcpSelectionInitialView,
  mcpSelectionMarker,
  mcpSelectionPresentation,
} from "./mcp-selection-presentation";

// @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.10
it.each<[MCPSessionApplyState, string | null]>([
  ["pending_idle", "pending"],
  ["deferred_restart", "deferred"],
  ["failed", "failed"],
  ["applied", null],
])("presents %s separately from server connection health", (apply_state, marker) => {
  const state = {
    apply_state,
    desired_revision: 2,
    applied_revision: apply_state === "applied" ? 2 : 1,
  };
  expect(mcpSelectionMarker(state)).toBe(marker);
  expect(mcpSelectionInitialView(state)).toBe(marker ? "customize" : "connections");
});
it("has no pending marker or customization default without saved state", () => {
  expect(mcpSelectionMarker(null)).toBeNull();
  expect(mcpSelectionInitialView(undefined)).toBe("connections");
});

// @covers AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.17
it.each(["applied", "pending_idle", "deferred_restart", "failed"] as const)(
  "provides a headline and consequence for %s",
  (apply_state) => {
    const presentation = mcpSelectionPresentation({
      apply_state,
      desired_revision: 3,
      applied_revision: 1,
    });
    expect(presentation.tone).toBe(apply_state);
    expect(presentation.descriptionKey).toBe(`${presentation.labelKey}Description`);
  },
);
