import { describe, expect, it } from "vitest";
import { resolveFileTreeEditorTarget } from "./file-tree-editor-target";

const PRIMARY = { id: "wt-1", path: "/tmp/task/kandev" };
const SIBLING = { id: "wt-2", path: "/tmp/task/kandev-feature-x" };
const README_PATH = "apps/web/README.md";
/** The tree is served from inside the primary worktree. */
const WORKTREE_ROOT = "kandev";
/** The tree is served from the task root that holds the worktrees. */
const TASK_ROOT = "task";

describe("resolveFileTreeEditorTarget", () => {
  it("passes the node path through for a session with no known worktree", () => {
    expect(resolveFileTreeEditorTarget(README_PATH, [], "e2e-repo")).toEqual({
      filePath: README_PATH,
    });
  });

  it("passes the node path through when the tree root is not known yet", () => {
    expect(resolveFileTreeEditorTarget(README_PATH, [PRIMARY], undefined)).toEqual({
      filePath: README_PATH,
    });
  });

  it("ignores worktrees with no recorded path", () => {
    expect(resolveFileTreeEditorTarget(README_PATH, [{ id: "wt-9" }], TASK_ROOT)).toEqual({
      filePath: README_PATH,
    });
  });

  it("targets the worktree the tree is rooted in", () => {
    expect(resolveFileTreeEditorTarget(README_PATH, [PRIMARY], WORKTREE_ROOT)).toEqual({
      filePath: README_PATH,
      worktreeId: "wt-1",
    });
  });

  it("strips the worktree directory segment when the tree is rooted at the task root", () => {
    expect(
      resolveFileTreeEditorTarget(`kandev-feature-x/${README_PATH}`, [PRIMARY, SIBLING], TASK_ROOT),
    ).toEqual({ filePath: README_PATH, worktreeId: "wt-2" });
  });

  it("resolves a worktree directory node to that worktree's root", () => {
    expect(resolveFileTreeEditorTarget("kandev", [PRIMARY, SIBLING], TASK_ROOT)).toEqual({
      filePath: "",
      worktreeId: "wt-1",
    });
  });

  it("matches worktree directories recorded with Windows separators", () => {
    const windowsSibling = { id: "wt-3", path: "C:\\tasks\\demo\\kandev-fix" };
    expect(
      resolveFileTreeEditorTarget(
        `kandev-fix/${README_PATH}`,
        [PRIMARY, windowsSibling],
        TASK_ROOT,
      ),
    ).toEqual({ filePath: README_PATH, worktreeId: "wt-3" });
  });

  // Attaching any source through "Add Repositories to workspace" rebinds the
  // session to the task root even when it adds no worktree, so a single-worktree
  // task can still serve a task-rooted tree.
  it("returns null for an attached folder that belongs to no worktree", () => {
    expect(resolveFileTreeEditorTarget("design-notes/spec.md", [PRIMARY], TASK_ROOT)).toBeNull();
  });

  it("returns null for a task-rooted entry that matches no worktree", () => {
    expect(resolveFileTreeEditorTarget("docs/README.md", [PRIMARY], TASK_ROOT)).toBeNull();
  });
});
