import { describe, expect, it } from "vitest";
import { getFileBrowserSessionWorkspacePath, resolveFileBrowserPaths } from "./file-browser-path";

describe("resolveFileBrowserPaths", () => {
  it("keeps the toolbar out of loading state when the loaded root path is empty", () => {
    expect(
      resolveFileBrowserPaths({
        sessionWorktreePath: "",
        repositoryLocalPath: "",
        treePath: "",
        treeLoaded: true,
      }),
    ).toEqual({
      fullPath: "",
      displayPath: "Workspace root",
    });
  });

  it("returns no display path before the tree is loaded when no absolute path is known", () => {
    expect(
      resolveFileBrowserPaths({
        sessionWorktreePath: "",
        repositoryLocalPath: "",
        treePath: "",
        treeLoaded: false,
      }),
    ).toEqual({
      fullPath: "",
      displayPath: "",
    });
  });

  it("prefers the session worktree path and shortens user home directories", () => {
    expect(
      resolveFileBrowserPaths({
        sessionWorktreePath: "/Users/cfl/Projects/kandev/.kandev/tasks/task-1",
        repositoryLocalPath: "/tmp/repo",
        treePath: "",
        treeLoaded: true,
      }),
    ).toEqual({
      fullPath: "/Users/cfl/Projects/kandev/.kandev/tasks/task-1",
      displayPath: "~/Projects/kandev/.kandev/tasks/task-1",
    });
  });
});

describe("getFileBrowserSessionWorkspacePath", () => {
  it("prefers the effective workspace path and falls back to the legacy worktree path", () => {
    expect(
      getFileBrowserSessionWorkspacePath({
        workspace_path: "/task-root",
        worktree_path: "/task-root/kandev",
      }),
    ).toBe("/task-root");
    expect(getFileBrowserSessionWorkspacePath({ worktree_path: "/legacy-worktree" })).toBe(
      "/legacy-worktree",
    );
  });
});
