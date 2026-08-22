import { describe, expect, it } from "vitest";
import { mapWorkspaceItem } from "./route-bootstrap";
import type { Workspace } from "@/lib/types/http";

/**
 * Regression guard for a dropped hop.
 *
 * The API returned the caller's scopes correctly, but this mapper listed the
 * store item's fields by hand and silently omitted them, so the Team access
 * card rendered every control disabled for the workspace's own owner. Nothing
 * failed: no type error, no test, no console warning.
 */
function apiWorkspace(overrides: Partial<Workspace> = {}): Workspace {
  return {
    id: "ws-1",
    name: "Platform Team",
    owner_id: "user-ana",
    visibility: "org",
    viewer_role: "owner",
    scopes: ["workspace.read", "workspace.manage", "member.manage"],
    member_count: 3,
    created_at: "2026-08-22T10:00:00Z",
    updated_at: "2026-08-22T10:00:00Z",
    ...overrides,
  } as Workspace;
}

describe("mapWorkspaceItem", () => {
  it("carries visibility, viewer role, scopes and member count into the store", () => {
    const item = mapWorkspaceItem(apiWorkspace());
    expect(item.visibility).toBe("org");
    expect(item.viewer_role).toBe("owner");
    expect(item.scopes).toEqual(["workspace.read", "workspace.manage", "member.manage"]);
    expect(item.member_count).toBe(3);
  });

  it("leaves team-access fields undefined when the API omits them", () => {
    const item = mapWorkspaceItem(
      apiWorkspace({
        visibility: undefined,
        viewer_role: undefined,
        scopes: undefined,
        member_count: undefined,
      }),
    );
    expect(item.visibility).toBeUndefined();
    expect(item.scopes).toBeUndefined();
  });

  it("still maps the pre-existing workspace fields", () => {
    const item = mapWorkspaceItem(apiWorkspace({ name: "Ana - security spike" }));
    expect(item.id).toBe("ws-1");
    expect(item.name).toBe("Ana - security spike");
    expect(item.owner_id).toBe("user-ana");
  });
});
