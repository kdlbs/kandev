import { describe, expect, it } from "vitest";

import { orderWorkspacesForDisplay } from "./workspace-display-order";

const WORKSPACES = [
  { id: "first", name: "First" },
  { id: "active", name: "Active" },
  { id: "last", name: "Last" },
];

describe("orderWorkspacesForDisplay", () => {
  it("moves the active workspace first and preserves the remaining order", () => {
    const ordered = orderWorkspacesForDisplay(WORKSPACES, "active");

    expect(ordered.map((workspace) => workspace.id)).toEqual(["active", "first", "last"]);
    expect(ordered).not.toBe(WORKSPACES);
    expect(WORKSPACES.map((workspace) => workspace.id)).toEqual(["first", "active", "last"]);
  });

  it.each([undefined, null, "missing"])(
    "preserves the input order when the active workspace is %s",
    (activeWorkspaceId) => {
      expect(orderWorkspacesForDisplay(WORKSPACES, activeWorkspaceId)).toEqual(WORKSPACES);
    },
  );

  it("preserves the input order when the active workspace is already first", () => {
    const workspaces = [WORKSPACES[1], WORKSPACES[0], WORKSPACES[2]];

    expect(orderWorkspacesForDisplay(workspaces, "active")).toEqual(workspaces);
  });
});
