import { beforeEach, describe, expect, it } from "vitest";

import {
  mapWorkspaceItem,
  readActiveWorkspaceCookie,
  readCookie,
  resolveSettingsActiveWorkspaceId,
} from "./route-bootstrap";
import type { ListWorkspacesResponse } from "@/lib/types/http";

type WorkspaceItem = ListWorkspacesResponse["workspaces"][number];

beforeEach(() => {
  document.cookie = "kandev-active-workspace=; path=/; max-age=0";
  document.cookie = "office-active-workspace=; path=/; max-age=0";
});

describe("mapWorkspaceItem", () => {
  it("normalizes optional workspace fields for store hydration", () => {
    expect(
      mapWorkspaceItem({
        id: "ws-1",
        name: "Workspace",
        owner_id: "owner-1",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-02T00:00:00Z",
      } as WorkspaceItem),
    ).toEqual({
      id: "ws-1",
      name: "Workspace",
      description: null,
      owner_id: "owner-1",
      default_executor_id: null,
      default_environment_id: null,
      default_agent_profile_id: null,
      default_config_agent_profile_id: null,
      office_workflow_id: null,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-02T00:00:00Z",
    });
  });
});

describe("readCookie", () => {
  it("reads encoded cookie values by encoded cookie name", () => {
    document.cookie = `${encodeURIComponent("office-active-workspace")}=${encodeURIComponent("ws 1/2")}`;

    expect(readCookie("office-active-workspace")).toBe("ws 1/2");
    expect(readCookie("missing")).toBeNull();
  });

  it("prefers the general active workspace cookie over the legacy office cookie", () => {
    document.cookie = "office-active-workspace=office-1; path=/";
    document.cookie = "kandev-active-workspace=kanban-1; path=/";

    expect(readActiveWorkspaceCookie()).toBe("kanban-1");
  });

  it("does not use the legacy office cookie as the generic active workspace", () => {
    document.cookie = "office-active-workspace=office-1; path=/";

    expect(readActiveWorkspaceCookie()).toBeNull();
  });
});

describe("resolveSettingsActiveWorkspaceId", () => {
  const OFFICE = { id: "office-1", office_workflow_id: "office-workflow" };
  const OFFICE_TWO = { id: "office-2", office_workflow_id: "office-workflow-2" };
  const KANBAN = { id: "kanban-1", office_workflow_id: null };
  const KANBAN_TWO = { id: "kanban-2", office_workflow_id: null };

  it("prefers the active workspace cookie whatever type it names", () => {
    // Previously this resolver filtered office workspaces out. That was
    // invisible while chrome came from the pathname; now that chrome follows
    // the active workspace, filtering here would switch an Office user's
    // workspace as a side effect of opening Settings.
    expect(resolveSettingsActiveWorkspaceId([KANBAN, OFFICE], OFFICE.id, null)).toBe(OFFICE.id);
    expect(resolveSettingsActiveWorkspaceId([OFFICE, KANBAN], KANBAN.id, null)).toBe(KANBAN.id);
  });

  it("falls back to the stored preference when no cookie matches", () => {
    expect(resolveSettingsActiveWorkspaceId([OFFICE, KANBAN], "ws-missing", OFFICE.id)).toBe(
      OFFICE.id,
    );
  });

  it("falls back to the first workspace when nothing is named", () => {
    expect(resolveSettingsActiveWorkspaceId([OFFICE, KANBAN_TWO], null, null)).toBe(OFFICE.id);
    expect(resolveSettingsActiveWorkspaceId([KANBAN, OFFICE_TWO], null, null)).toBe(KANBAN.id);
  });

  it("returns null when no workspaces exist", () => {
    expect(resolveSettingsActiveWorkspaceId([], "k-1", "k-2")).toBeNull();
  });
});
