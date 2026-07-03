import { describe, expect, it } from "vitest";

import { resolveActiveKanbanWorkspaceId } from "./page";

describe("resolveActiveKanbanWorkspaceId", () => {
  it("ignores office workspaces when resolving cookie and settings IDs", () => {
    const activeId = resolveActiveKanbanWorkspaceId(
      [
        { id: "ws-office", office_workflow_id: "office-flow" },
        { id: "ws-kanban", office_workflow_id: null },
      ],
      undefined,
      "ws-office",
      "ws-kanban",
    );

    expect(activeId).toBe("ws-kanban");
  });

  it("falls back when explicit URL workspace ID is an office workspace", () => {
    const activeId = resolveActiveKanbanWorkspaceId(
      [
        { id: "ws-office", office_workflow_id: "office-flow" },
        { id: "ws-kanban", office_workflow_id: null },
      ],
      "ws-office",
      null,
      "ws-kanban",
    );

    expect(activeId).toBe("ws-kanban");
  });
});
