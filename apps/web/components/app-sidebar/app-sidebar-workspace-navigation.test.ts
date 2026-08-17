import { beforeEach, describe, expect, it } from "vitest";
import { LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE } from "@/lib/routing/route-bootstrap";
import {
  rememberWorkspaceSelection,
  rememberWorkspaceSelectionById,
  workspaceHomeHref,
} from "./app-sidebar-workspace-navigation";

const ACTIVE_WORKSPACE_COOKIE = "kandev-active-workspace";
const kanban = { id: "kanban-1", office_workflow_id: "" };
const office = { id: "office-1", office_workflow_id: "wf-office" };
const officeWithReservedChars = {
  id: "office/2;mode",
  office_workflow_id: "wf-office-reserved",
};

describe("app sidebar workspace navigation", () => {
  beforeEach(() => {
    document.cookie = "kandev-active-workspace=; path=/; max-age=0";
    document.cookie = "office-active-workspace=; path=/; max-age=0";
  });

  it("routes workspace home by active workspace type", () => {
    expect(workspaceHomeHref(kanban)).toBe("/?home=overview&workspaceId=kanban-1");
    expect(workspaceHomeHref(office)).toBe("/office?workspaceId=office-1");
    expect(workspaceHomeHref(undefined)).toBe("/?home=overview");
  });

  it("records the active workspace in one write", () => {
    rememberWorkspaceSelection(kanban);
    rememberWorkspaceSelection(office);

    expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}=office-1`);
    expect(document.cookie).toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}=office-1`);
  });

  it("does not write the legacy office cookie for a kanban selection", () => {
    rememberWorkspaceSelection(office);
    rememberWorkspaceSelection(kanban);

    // The office boot paths read the legacy cookie to pick an office workspace
    // when the unified cookie names a kanban board, so a kanban selection must
    // leave it pointing at the office workspace last used.
    expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}=kanban-1`);
    expect(document.cookie).toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}=office-1`);
  });

  it("writes active and legacy office workspace cookies with an encoded id", () => {
    rememberWorkspaceSelection(officeWithReservedChars);

    expect(document.cookie).toContain(
      `${ACTIVE_WORKSPACE_COOKIE}=${encodeURIComponent(officeWithReservedChars.id)}`,
    );
    expect(document.cookie).toContain(
      `${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}=${encodeURIComponent(officeWithReservedChars.id)}`,
    );
  });

  it("records a workspace known only by id and kind", () => {
    // The setup wizard path: the create response returns an id and nothing
    // else, so there is no record to pass.
    rememberWorkspaceSelectionById("office-new", "office");

    expect(document.cookie).toContain(`${ACTIVE_WORKSPACE_COOKIE}=office-new`);
    expect(document.cookie).toContain(`${LEGACY_OFFICE_ACTIVE_WORKSPACE_COOKIE}=office-new`);
  });
});
