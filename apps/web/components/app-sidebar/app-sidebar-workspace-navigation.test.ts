import { beforeEach, describe, expect, it } from "vitest";
import {
  LAST_KANBAN_WORKSPACE_KEY,
  rememberLastKanbanWorkspace,
  resolveLastOfficeWorkspace,
  resolveLastKanbanWorkspace,
  workspaceHomeHref,
} from "./app-sidebar-workspace-navigation";

const kanban = { id: "kanban-1", office_workflow_id: "" };
const kanbanTwo = { id: "kanban-2", office_workflow_id: null };
const office = { id: "office-1", office_workflow_id: "wf-office" };
const officeTwo = { id: "office-2", office_workflow_id: "wf-office-2" };

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
    expect(resolveLastOfficeWorkspace([kanban, office, officeTwo])).toBe(office);
  });

  it("resolves the last office workspace from the office-active-workspace cookie", () => {
    document.cookie = "office-active-workspace=office-2; path=/";

    expect(resolveLastOfficeWorkspace([kanban, office, officeTwo])).toBe(officeTwo);
  });

  it("falls back to the first office workspace when the office cookie is stale", () => {
    document.cookie = "office-active-workspace=kanban-1; path=/";

    expect(resolveLastOfficeWorkspace([kanban, office, officeTwo])).toBe(office);
  });
});
