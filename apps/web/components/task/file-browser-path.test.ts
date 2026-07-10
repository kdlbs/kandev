import { describe, expect, it } from "vitest";
import { resolveFileBrowserPaths } from "./file-browser-path";

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
