import { describe, expect, it } from "vitest";
import type { AppState } from "@/lib/state/store";
import { isOfficeWorkspace, selectActiveWorkspace, type WorkspaceItem } from "./selectors";

function workspace(id: string, officeWorkflowId?: string | null): WorkspaceItem {
  return {
    id,
    name: id,
    owner_id: "owner-1",
    office_workflow_id: officeWorkflowId ?? null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function stateWith(items: WorkspaceItem[], activeId: string | null): AppState {
  return { workspaces: { items, activeId } } as AppState;
}

describe("isOfficeWorkspace", () => {
  it("is true only when an office workflow is attached", () => {
    expect(isOfficeWorkspace(workspace("office-1", "wf-office"))).toBe(true);
    expect(isOfficeWorkspace(workspace("kanban-1"))).toBe(false);
  });

  it("treats an empty office workflow id as kanban", () => {
    // The backend serialises "no office workflow" as an empty string on some
    // paths and null on others; both mean the same thing here.
    expect(isOfficeWorkspace(workspace("kanban-1", ""))).toBe(false);
  });

  it("is false for a workspace that has not resolved", () => {
    expect(isOfficeWorkspace(undefined)).toBe(false);
    expect(isOfficeWorkspace(null)).toBe(false);
  });

  it("accepts any record carrying the two fields it reads", () => {
    // Server-side helpers and API responses pass workspace-shaped records that
    // are not store items; widening them just to ask this question is what the
    // structural parameter type avoids.
    expect(isOfficeWorkspace({ id: "office-1", office_workflow_id: "wf-office" })).toBe(true);
  });
});

describe("selectActiveWorkspace", () => {
  it("returns the record matching the active id", () => {
    const state = stateWith(
      [workspace("kanban-1"), workspace("office-1", "wf-office")],
      "office-1",
    );

    expect(selectActiveWorkspace(state)?.id).toBe("office-1");
  });

  it("returns undefined while the workspace list is unhydrated", () => {
    // Distinct from "the active workspace is a kanban workspace": callers that
    // derive mode have to hold rather than resolve to kanban here.
    expect(selectActiveWorkspace(stateWith([], "office-1"))).toBeUndefined();
  });

  it("returns undefined when no workspace is active", () => {
    expect(selectActiveWorkspace(stateWith([workspace("kanban-1")], null))).toBeUndefined();
  });
});
