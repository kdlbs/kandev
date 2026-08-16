import { describe, expect, it } from "vitest";

import { resolveKanbanRouteWorkspaceId } from "./kanban-route";
import type { WorkspaceState } from "@/lib/state/slices/workspace/types";

const TEST_TIMESTAMP = "2026-06-24T00:00:00Z";

function workspace(
  id: string,
  officeWorkflowId: string | null = null,
): WorkspaceState["items"][number] {
  return {
    id,
    name: id,
    description: null,
    owner_id: "owner-1",
    default_executor_id: null,
    default_environment_id: null,
    default_agent_profile_id: null,
    default_config_agent_profile_id: null,
    office_workflow_id: officeWorkflowId,
    created_at: TEST_TIMESTAMP,
    updated_at: TEST_TIMESTAMP,
  };
}

const KANBAN = workspace("ws-kanban");
const OFFICE = workspace("ws-office", "wf-office");

describe("resolveKanbanRouteWorkspaceId", () => {
  it("honours an explicitly named workspace of any type", () => {
    // An office id must resolve to the office workspace — resolving it against
    // kanban workspaces only is what used to reassign the active workspace when
    // going Home from Office.
    expect(resolveKanbanRouteWorkspaceId([KANBAN, OFFICE], OFFICE.id)).toBe(OFFICE.id);
    expect(resolveKanbanRouteWorkspaceId([KANBAN, OFFICE], KANBAN.id)).toBe(KANBAN.id);
  });

  it("takes the first preferred id that resolves, in order", () => {
    expect(resolveKanbanRouteWorkspaceId([KANBAN, OFFICE], null, OFFICE.id, KANBAN.id)).toBe(
      OFFICE.id,
    );
    expect(resolveKanbanRouteWorkspaceId([KANBAN, OFFICE], "ws-gone", KANBAN.id)).toBe(KANBAN.id);
  });

  it("prefers a kanban workspace only when nothing was named", () => {
    expect(resolveKanbanRouteWorkspaceId([OFFICE, KANBAN])).toBe(KANBAN.id);
  });

  it("falls back to the first workspace when only office workspaces exist", () => {
    const officeTwo = workspace("ws-office-2", "wf-office-2");
    expect(resolveKanbanRouteWorkspaceId([OFFICE, officeTwo])).toBe(OFFICE.id);
  });

  it("returns null when no workspaces exist", () => {
    expect(resolveKanbanRouteWorkspaceId([], "ws-anything")).toBeNull();
  });
});
