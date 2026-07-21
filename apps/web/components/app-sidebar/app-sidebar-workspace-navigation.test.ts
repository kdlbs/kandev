import { beforeEach, describe, expect, it } from "vitest";
import {
  LAST_KANBAN_WORKSPACE_KEY,
  OFFICE_ACTIVE_WORKSPACE_COOKIE,
  rememberLastOfficeWorkspace,
  rememberLastKanbanWorkspace,
  resolveLastOfficeWorkspace,
  resolveLastKanbanWorkspace,
  workspaceHomeHref,
} from "./app-sidebar-workspace-navigation";

const ACTIVE_WORKSPACE_COOKIE = "kandev-active-workspace";
const kanban = { id: "kanban-1", office_workflow_id: "" };
const kanbanTwo = { id: "kanban-2", office_workflow_id: null };
const office = { id: "office-1", office_workflow_id: "wf-office" };
const officeTwo = { id: "office-2", office_workflow_id: "wf-office-2" };
const officeWithReservedChars = {
  id: "office/2;mode",
  office_workflow_id: "wf-office-reserved",
};

describe("app sidebar workspace navigation", () => {
  beforeEach(() => {
    window.localStorage.clear();
    document.cookie = "kandev-active-workspace=; path=/; max-age=0";
    document.cookie = "office-active-workspace=; path=/; max-age=0";
  });

  it("routes workspace home by active workspace type", () => {
    expect(workspaceHomeHref(kanban)).toBe("/?workspaceId=kanban-1");
    expect(workspaceHomeHref(office)).toBe("/office?workspaceId=office-1");
    expect(workspaceHomeHref(undefined)).toBe("/");
  });

  it("remembers and resolves the last kanban workspace", () => {
    rememberLastKanbanWorkspace(kanbanTwo);

    expect(window.localStorage.getItem(LAST_KANBAN_WORKSPACE_KEY)).toBe("kanban-2");
    expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}=kanban-2`);
    expect(resolveLastKanbanWorkspace([kanban, office, kanbanTwo])).toBe(kanbanTwo);
  });

  it("resolves the last kanban workspace from the active workspace cookie", () => {
    rememberLastKanbanWorkspace(kanban);
    document.cookie = `${ACTIVE_WORKSPACE_COOKIE}=kanban-2; path=/`;

    expect(resolveLastKanbanWorkspace([kanban, office, kanbanTwo])).toBe(kanbanTwo);
  });

  it("does not overwrite the last kanban workspace with office workspaces", () => {
    rememberLastKanbanWorkspace(kanban);
    rememberLastKanbanWorkspace(office);

    expect(resolveLastKanbanWorkspace([kanban, office])).toBe(kanban);
  });

  it("writes active and legacy office workspace cookies with an encoded id", () => {
    rememberLastOfficeWorkspace(officeWithReservedChars);

    expect(document.cookie).toContain(
      `${ACTIVE_WORKSPACE_COOKIE}=${encodeURIComponent(officeWithReservedChars.id)}`,
    );
    expect(document.cookie).toContain(
      `${OFFICE_ACTIVE_WORKSPACE_COOKIE}=${encodeURIComponent(officeWithReservedChars.id)}`,
    );
    expect(resolveLastOfficeWorkspace([office, officeWithReservedChars])).toBe(
      officeWithReservedChars,
    );
  });

  it("falls back to the first kanban workspace and first office workspace", () => {
    expect(resolveLastKanbanWorkspace([office, kanban, kanbanTwo])).toBe(kanban);
    expect(resolveLastOfficeWorkspace([kanban, office, officeTwo])).toBe(office);
  });

  it("resolves the last office workspace from the office-active-workspace cookie", () => {
    document.cookie = "office-active-workspace=office-2; path=/";

    expect(resolveLastOfficeWorkspace([kanban, office, officeTwo])).toBe(officeTwo);
  });

  it("resolves the last office workspace from the active workspace cookie first", () => {
    document.cookie = "office-active-workspace=office-1; path=/";
    document.cookie = `${ACTIVE_WORKSPACE_COOKIE}=office-2; path=/`;

    expect(resolveLastOfficeWorkspace([kanban, office, officeTwo])).toBe(officeTwo);
  });

  it("falls back to the office workspace cookie when the active cookie is kanban", () => {
    document.cookie = "office-active-workspace=office-2; path=/";
    document.cookie = `${ACTIVE_WORKSPACE_COOKIE}=kanban-1; path=/`;

    expect(resolveLastOfficeWorkspace([kanban, office, officeTwo])).toBe(officeTwo);
  });

  it("falls back to the first office workspace when the office cookie is stale", () => {
    document.cookie = "office-active-workspace=kanban-1; path=/";

    expect(resolveLastOfficeWorkspace([kanban, office, officeTwo])).toBe(office);
  });
});
