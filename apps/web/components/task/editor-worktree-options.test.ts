import { describe, expect, it } from "vitest";
import { buildWorktreeOptions } from "./editor-worktree-options";
import type { Worktree } from "@/lib/state/slices/session/types";
import type { Repository } from "@/lib/types/http";

function repo(id: string, name: string): Repository {
  return { id, name } as Repository;
}

describe("buildWorktreeOptions", () => {
  it("labels worktrees with their repository name", () => {
    const worktrees: Worktree[] = [
      { id: "wt-1", sessionId: "s-1", repositoryId: "repo-a", path: "/tasks/x/a", branch: "kd/x" },
      { id: "wt-2", sessionId: "s-1", repositoryId: "repo-b", path: "/tasks/x/b", branch: "kd/y" },
    ];
    const options = buildWorktreeOptions(worktrees, [repo("repo-a", "api"), repo("repo-b", "web")]);
    expect(options).toEqual([
      { worktreeId: "wt-1", label: "api", branch: "kd/x" },
      { worktreeId: "wt-2", label: "web", branch: "kd/y" },
    ]);
  });

  it("falls back to the worktree folder name when the repository is unknown", () => {
    const worktrees: Worktree[] = [
      { id: "wt-1", sessionId: "s-1", repositoryId: "missing", path: "/tasks/x/kandev" },
    ];
    const options = buildWorktreeOptions(worktrees, []);
    expect(options).toEqual([{ worktreeId: "wt-1", label: "kandev", branch: undefined }]);
  });

  it("handles Windows-style paths", () => {
    const worktrees: Worktree[] = [{ id: "wt-1", sessionId: "s-1", path: "C:\\tasks\\x\\kandev" }];
    expect(buildWorktreeOptions(worktrees, [])[0].label).toBe("kandev");
  });

  it("falls back to a positional label without repository or path", () => {
    const worktrees: Worktree[] = [
      { id: "wt-1", sessionId: "s-1" },
      { id: "wt-2", sessionId: "s-1" },
    ];
    const options = buildWorktreeOptions(worktrees, []);
    expect(options.map((o) => o.label)).toEqual(["Worktree 1", "Worktree 2"]);
  });
});
