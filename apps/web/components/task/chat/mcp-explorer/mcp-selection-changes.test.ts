import { describe, expect, it } from "vitest";
import type { MCPSetupServer, MCPSessionSelectionState } from "@/lib/types/http-mcp";
import { mcpSelectionChanges } from "./mcp-selection-changes";

const server = (id: string, revision = 1): MCPSetupServer => ({
  definition_id: id,
  revision,
  enabled: true,
  display_name: `Name ${id}`,
  runtime_name: id,
});
function pending(): MCPSessionSelectionState {
  return {
    apply_state: "pending_idle",
    desired_revision: 4,
    applied_revision: 2,
    attachment_attempt_id: "attempt",
    comparison: {
      version: 1,
      status: "known",
      desired_revision: 4,
      applied_revision: 2,
      attachment_attempt_id: "attempt",
      selection_ids: ["added", "updated"],
      changes: [
        { definition_id: "added", action: "add", after: server("added") },
        { definition_id: "deleted", action: "remove", before: server("deleted") },
        {
          definition_id: "updated",
          action: "update",
          before: server("updated"),
          after: server("updated", 2),
        },
      ],
    },
  };
}
const ids = ["updated", "added"];
describe("saved MCP setup changes", () => {
  it("preserves named additions, deleted removals and updated definitions for each pending timing", () => {
    for (const apply_state of ["pending_idle", "deferred_restart", "failed"] as const) {
      const state = { ...pending(), apply_state };
      expect(mcpSelectionChanges(state, ids)).toEqual({
        status: "known",
        changes: state.comparison!.changes,
      });
    }
  });
  it("keeps a known empty difference distinct from unavailable and applied", () => {
    const state = pending();
    state.comparison!.changes = undefined;
    expect(mcpSelectionChanges(state, ids)).toEqual({ status: "known", changes: [] });
    state.comparison = undefined;
    expect(mcpSelectionChanges(state, ids).status).toBe("unavailable");
    expect(mcpSelectionChanges({ ...pending(), apply_state: "applied" }, ids).status).toBe(
      "hidden",
    );
    expect(mcpSelectionChanges(null, ids).status).toBe("hidden");
  });
  it("rejects a comparison for other selections, revisions or attachment attempts", () => {
    for (const patch of [
      { desired_revision: 3 },
      { applied_revision: 1 },
      { attachment_attempt_id: "old" },
      { selection_ids: ["sibling"] },
      { version: 2 },
      { status: "unavailable" },
    ]) {
      const state = pending();
      Object.assign(state.comparison!, patch);
      expect(mcpSelectionChanges(state, ids).status).toBe("unavailable");
    }
  });
  it("rejects malformed or duplicate descriptors instead of guessing actions", () => {
    for (const changes of [
      [{ definition_id: "added", action: "add" }],
      [{ definition_id: "added", action: "add", after: server("other") }],
      [{ definition_id: "added", action: "add", after: { ...server("added"), revision: 0 } }],
      [{ definition_id: "added", action: "wat", after: server("added") }],
      [pending().comparison!.changes![0], pending().comparison!.changes![0]],
    ]) {
      const state = pending();
      Object.assign(state.comparison!, { changes });
      expect(mcpSelectionChanges(state, ids).status).toBe("unavailable");
    }
  });
  it("matches empty and duplicated scope selections without multiplying changes", () => {
    const state = pending();
    expect(mcpSelectionChanges(state, [...ids, ...ids]).changes).toHaveLength(3);
    state.comparison!.selection_ids = undefined;
    expect(mcpSelectionChanges(state, []).status).toBe("known");
  });
});

it("rejects comparison from a different exclusion set", () => {
  const state = {
    desired_revision: 2,
    applied_revision: 1,
    apply_state: "pending_idle" as const,
    comparison: {
      version: 1 as const,
      status: "known" as const,
      desired_revision: 2,
      applied_revision: 1,
      selection_ids: [],
      excluded_definition_ids: [],
      changes: [],
    },
  };
  expect(mcpSelectionChanges(state, [], ["docs"])).toEqual({ status: "unavailable", changes: [] });
  expect(
    mcpSelectionChanges(
      { ...state, comparison: { ...state.comparison, excluded_definition_ids: ["docs"] } },
      [],
      ["docs"],
    ),
  ).toEqual({ status: "known", changes: [] });
});
