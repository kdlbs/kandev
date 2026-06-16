import { beforeEach, describe, expect, it } from "vitest";
import {
  LAST_KANBAN_WORKSPACE_KEY,
  rememberLastKanbanWorkspace,
  resolveFirstOfficeWorkspace,
  resolveLastKanbanWorkspace,
  workspaceHomeHref,
} from "./app-sidebar-workspace-navigation";

const kanban = { id: "kanban-1", office_workflow_id: "" };
const kanbanTwo = { id: "kanban-2", office_workflow_id: null };
const office = { id: "office-1", office_workflow_id: "wf-office" };

describe("app sidebar workspace navigation", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("routes workspace home by active workspace type", () => {
    expect(workspaceHomeHref(kanban)).toBe("/?workspaceId=kanban-1");
    expect(workspaceHomeHref(office)).toBe("/office?workspaceId=office-1");
    expect(workspaceHomeHref(undefined)).toBe("/");
  });

  it("remembers and resolves the last kanban workspace", () => {
    rememberLastKanbanWorkspace(kanbanTwo);

    expect(window.localStorage.getItem(LAST_KANBAN_WORKSPACE_KEY)).toBe("kanban-2");
    expect(resolveLastKanbanWorkspace([kanban, office, kanbanTwo])).toBe(kanbanTwo);
  });

  it("does not overwrite the last kanban workspace with office workspaces", () => {
    rememberLastKanbanWorkspace(kanban);
    rememberLastKanbanWorkspace(office);

    expect(resolveLastKanbanWorkspace([kanban, office])).toBe(kanban);
  });

  it("falls back to the first kanban workspace and first office workspace", () => {
    expect(resolveLastKanbanWorkspace([office, kanban, kanbanTwo])).toBe(kanban);
    expect(resolveFirstOfficeWorkspace([kanban, office, kanbanTwo])).toBe(office);
  });
});
