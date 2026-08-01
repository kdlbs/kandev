import { describe, expect, it } from "vitest";
import { getSessionWorkspacePath } from "./session-workspace-path";

describe("getSessionWorkspacePath", () => {
  it("prefers the effective task workspace root", () => {
    expect(
      getSessionWorkspacePath({
        workspace_path: "/task-root",
        worktree_path: "/task-root/kandev",
      }),
    ).toBe("/task-root");
  });

  it("falls back to the primary worktree for legacy sessions", () => {
    expect(getSessionWorkspacePath({ worktree_path: "/legacy/repository" })).toBe(
      "/legacy/repository",
    );
  });

  it("returns undefined when neither path is available", () => {
    expect(getSessionWorkspacePath(null)).toBeUndefined();
  });
});
