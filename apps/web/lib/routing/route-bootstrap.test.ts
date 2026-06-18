import { beforeEach, describe, expect, it } from "vitest";

import { mapWorkspaceItem, readActiveWorkspaceCookie, readCookie } from "./route-bootstrap";
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

  it("falls back to the legacy office active workspace cookie", () => {
    document.cookie = "office-active-workspace=office-1; path=/";

    expect(readActiveWorkspaceCookie()).toBe("office-1");
  });
});
