import { describe, expect, it } from "vitest";
import {
  isRepositoryDirty,
  type RepositoryWithScripts,
} from "@/app/settings/workspace/workspace-repositories-dirty";
import { repositoryId as toRepositoryId, workspaceId as toWorkspaceId } from "@/lib/types/http";
import { defaultWorktreeBranchTemplate } from "@/lib/worktree-branch-template";

function makeRepo(overrides: Partial<RepositoryWithScripts> = {}): RepositoryWithScripts {
  return {
    id: toRepositoryId("repo-1"),
    workspace_id: toWorkspaceId("ws-1"),
    name: "my-repo",
    source_type: "local",
    local_path: "/tmp/my-repo",
    provider: "",
    provider_repo_id: "",
    provider_owner: "",
    provider_name: "",
    default_branch: "main",
    worktree_branch_prefix: "feature/",
    worktree_branch_template: defaultWorktreeBranchTemplate,
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    startup_prompt: "",
    created_at: "",
    updated_at: "",
    scripts: [],
    ...overrides,
  };
}

describe("isRepositoryDirty", () => {
  it("returns false when copy_files matches", () => {
    const saved = makeRepo({ copy_files: ".env, .env.local" });
    const repo = makeRepo({ copy_files: ".env, .env.local" });
    expect(isRepositoryDirty(repo, saved)).toBe(false);
  });

  it("returns true when copy_files differs", () => {
    const saved = makeRepo({ copy_files: "" });
    const repo = makeRepo({ copy_files: ".env" });
    expect(isRepositoryDirty(repo, saved)).toBe(true);
  });

  it("returns true when worktree branch template differs", () => {
    const saved = makeRepo({ worktree_branch_template: defaultWorktreeBranchTemplate });
    const repo = makeRepo({ worktree_branch_template: "feature/{ticket}-{title}" });
    expect(isRepositoryDirty(repo, saved)).toBe(true);
  });

  it("treats an empty worktree branch template as the default", () => {
    const saved = makeRepo({ worktree_branch_template: defaultWorktreeBranchTemplate });
    const repo = makeRepo({ worktree_branch_template: "" });
    expect(isRepositoryDirty(repo, saved)).toBe(false);
  });

  it("returns true when there is no saved repository", () => {
    const repo = makeRepo();
    expect(isRepositoryDirty(repo, undefined)).toBe(true);
  });
});
