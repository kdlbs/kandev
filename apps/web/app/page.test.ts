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

  it("honors explicit URL workspace IDs even for office workspaces", () => {
    const activeId = resolveActiveKanbanWorkspaceId(
      [
        { id: "ws-office", office_workflow_id: "office-flow" },
        { id: "ws-kanban", office_workflow_id: null },
      ],
      "ws-office",
      null,
      "ws-kanban",
    );

    expect(activeId).toBe("ws-office");
  });
});
