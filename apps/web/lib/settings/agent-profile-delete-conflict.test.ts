import { describe, expect, it } from "vitest";
import type { ActiveSessionInfo } from "@/lib/types/agent-profile-errors";
import { groupSessionsByWorkspace } from "./agent-profile-delete-conflict";

function session(
  task_id: string,
  task_title: string,
  workspace_id?: string,
  workspace_name?: string,
): ActiveSessionInfo {
  return { task_id, task_title, is_ephemeral: false, workspace_id, workspace_name };
}

describe("groupSessionsByWorkspace", () => {
  it("returns no groups for no sessions", () => {
    expect(groupSessionsByWorkspace([])).toEqual([]);
  });

  it("puts each session in exactly one group keyed by workspace id", () => {
    const groups = groupSessionsByWorkspace([
      session("t1", "Fix login", "ws-b", "Backend"),
      session("t2", "Billing", "ws-f", "Frontend"),
      session("t3", "Auth", "ws-b", "Backend"),
    ]);
    expect(groups.map((g) => [g.workspaceId, g.sessions.map((s) => s.task_id)])).toEqual([
      ["ws-b", ["t3", "t1"]],
      ["ws-f", ["t2"]],
    ]);
  });

  it("orders groups by name case-insensitively, then by workspace id", () => {
    const groups = groupSessionsByWorkspace([
      session("t1", "a", "ws-2", "beta"),
      session("t2", "a", "ws-3", "Alpha"),
      session("t3", "a", "ws-1", "Beta"),
    ]);
    expect(groups.map((g) => g.workspaceId)).toEqual(["ws-3", "ws-1", "ws-2"]);
  });

  it("orders items by title case-insensitively, then by task id", () => {
    const groups = groupSessionsByWorkspace([
      session("t3", "beta", "ws", "W"),
      session("t2", "Beta", "ws", "W"),
      session("t1", "alpha", "ws", "W"),
    ]);
    expect(groups[0].sessions.map((s) => s.task_id)).toEqual(["t1", "t2", "t3"]);
  });

  it("sorts groups without a workspace name after named groups", () => {
    const groups = groupSessionsByWorkspace([
      session("t1", "a", "ws-gone", ""),
      session("t2", "a"),
      session("t3", "a", "ws-z", "Zeta"),
    ]);
    expect(groups.map((g) => [g.workspaceId, g.workspaceName])).toEqual([
      ["ws-z", "Zeta"],
      ["", ""],
      ["ws-gone", ""],
    ]);
  });

  it("orders groups by the resolved store name when the response name is empty", () => {
    const groups = groupSessionsByWorkspace(
      [session("t1", "a", "ws-z", ""), session("t2", "a", "ws-a", "")],
      (id) => (id === "ws-z" ? "Zeta" : "Alpha"),
    );
    expect(groups.map((g) => [g.workspaceId, g.workspaceName])).toEqual([
      ["ws-a", "Alpha"],
      ["ws-z", "Zeta"],
    ]);
  });

  it("keeps the response name over a differing resolved name", () => {
    const groups = groupSessionsByWorkspace(
      [session("t1", "a", "ws-1", "Response name")],
      () => "Store name",
    );
    expect(groups.map((g) => g.workspaceName)).toEqual(["Response name"]);
  });
});
