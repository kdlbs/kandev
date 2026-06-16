import { beforeEach, describe, expect, it } from "vitest";

import { mapWorkspaceItem, readCookie } from "./route-bootstrap";
import type { ListWorkspacesResponse } from "@/lib/types/http";

type WorkspaceItem = ListWorkspacesResponse["workspaces"][number];

beforeEach(() => {
  document.cookie = "office-active-workspace=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/";
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
});
