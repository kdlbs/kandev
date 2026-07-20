import { describe, expect, it } from "vitest";
import { postDeleteWorkspaceHref, resolvePostDeleteWorkspace } from "./danger-zone-section";
import type { Workspace } from "@/lib/types/http";

function workspace(id: string, officeWorkflowId?: string | null): Workspace {
  return {
    id,
    name: id,
    description: "",
    owner_id: "user-1",
    office_workflow_id: officeWorkflowId,
    created_at: "2026-01-01T00:00:00.000Z",
    updated_at: "2026-01-01T00:00:00.000Z",
  } as Workspace;
}

describe("workspace delete navigation", () => {
  it("prefers another office workspace after deleting an office workspace", () => {
    const deleted = workspace("office-1", "flow-1");
    const kanban = workspace("kanban-1");
    const office = workspace("office-2", "flow-2");

    const next = resolvePostDeleteWorkspace(deleted.id, [deleted, kanban, office]);

    expect(next).toBe(office);
    expect(postDeleteWorkspaceHref(next)).toBe("/office?workspaceId=office-2");
  });

  it("falls back to a non-office workspace when no office workspaces remain", () => {
    const deleted = workspace("office-1", "flow-1");
    const kanban = workspace("kanban-1");

    const next = resolvePostDeleteWorkspace(deleted.id, [deleted, kanban]);

    expect(next).toBe(kanban);
    expect(postDeleteWorkspaceHref(next)).toBe("/?workspaceId=kanban-1");
  });

  it("routes to the new office workspace wizard when no workspaces remain", () => {
    const deleted = workspace("office-1", "flow-1");

    const next = resolvePostDeleteWorkspace(deleted.id, [deleted]);

    expect(next).toBeNull();
    expect(postDeleteWorkspaceHref(next)).toBe("/office/setup?mode=new");
  });
});
