import { describe, expect, it } from "vitest";

import { workspaceId, workflowId } from "@/lib/types/ids";
import type { ListWorkspacesResponse } from "@/lib/types/http";
import { resolveActiveKanbanWorkspaceId } from "./page";

type WorkspaceRow = ListWorkspacesResponse["workspaces"][number];

describe("resolveActiveKanbanWorkspaceId", () => {
  it("ignores office workspaces when resolving cookie and settings IDs", () => {
    const activeId = resolveActiveKanbanWorkspaceId(
      [workspaceRow("ws-office", "office-flow"), workspaceRow("ws-kanban", null)],
      undefined,
      "ws-office",
      "ws-kanban",
    );

    expect(activeId).toBe("ws-kanban");
  });

  it("falls back when explicit URL workspace ID is an office workspace", () => {
    const activeId = resolveActiveKanbanWorkspaceId(
      [workspaceRow("ws-office", "office-flow"), workspaceRow("ws-kanban", null)],
      "ws-office",
      null,
      "ws-kanban",
    );

    expect(activeId).toBe("ws-kanban");
  });
});

function workspaceRow(id: string, officeWorkflowId: string | null): WorkspaceRow {
  return {
    id: workspaceId(id),
    name: id,
    owner_id: "owner-1",
    office_workflow_id: officeWorkflowId ? workflowId(officeWorkflowId) : undefined,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  } as WorkspaceRow;
}
