import { describe, expect, it } from "vitest";
import { isRepositoryDirty } from "@/app/settings/workspace/repository-dirty";
import {
  repositoryId as toRepositoryId,
  workspaceId as toWorkspaceId,
  type Repository,
  type RepositoryScript,
} from "@/lib/types/http";

type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

function baseRepo(overrides: Partial<Repository> = {}): RepositoryWithScripts {
  return {
    id: toRepositoryId("repo-1"),
    workspace_id: toWorkspaceId("ws-1"),
    name: "repo",
    source_type: "local",
    local_path: "/tmp/repo",
    provider: "",
    provider_repo_id: "",
    provider_owner: "",
    provider_name: "",
    default_branch: "main",
    worktree_branch_prefix: "feature/",
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    worktree_files: [],
    created_at: "",
    updated_at: "",
    scripts: [],
    ...overrides,
  };
}

describe("isRepositoryDirty (worktree file config)", () => {
  it("returns false when nothing changed", () => {
    const saved = baseRepo();
    expect(isRepositoryDirty(baseRepo(), saved)).toBe(false);
  });

  it("detects a per-file mode change", () => {
    const saved = baseRepo({ worktree_files: [{ path: ".env", mode: "copy" }] });
    const edited = baseRepo({ worktree_files: [{ path: ".env", mode: "symlink" }] });
    expect(isRepositoryDirty(edited, saved)).toBe(true);
  });

  it("detects a worktree file list change", () => {
    const saved = baseRepo({ worktree_files: [{ path: ".env", mode: "copy" }] });
    const edited = baseRepo({
      worktree_files: [
        { path: ".env", mode: "copy" },
        { path: ".env.local", mode: "symlink" },
      ],
    });
    expect(isRepositoryDirty(edited, saved)).toBe(true);
  });

  it("treats equal file lists as clean regardless of array identity", () => {
    const saved = baseRepo({ worktree_files: [{ path: ".env.local", mode: "symlink" }] });
    const edited = baseRepo({ worktree_files: [{ path: ".env.local", mode: "symlink" }] });
    expect(isRepositoryDirty(edited, saved)).toBe(false);
  });
});
