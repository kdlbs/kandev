import { describe, expect, it } from "vitest";
import {
  isRepositoryDirty,
  mergeSavedRepositoryDraft,
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

  it("treats a copy_files entry with a :symlink keyword as clean when unchanged", () => {
    const saved = makeRepo({ copy_files: ".env, .env.local:symlink" });
    const repo = makeRepo({ copy_files: ".env, .env.local:symlink" });
    expect(isRepositoryDirty(repo, saved)).toBe(false);
  });

  it("returns true when a copy_files entry's keyword changes", () => {
    const saved = makeRepo({ copy_files: ".env.local" });
    const repo = makeRepo({ copy_files: ".env.local:symlink" });
    expect(isRepositoryDirty(repo, saved)).toBe(true);
  });
});

describe("mergeSavedRepositoryDraft", () => {
  it("remaps created script IDs while preserving edits made during save", () => {
    const submittedScript = makeScript("temp-script-1", "submitted");
    const submitted = makeRepo({ scripts: [submittedScript] });
    const current = makeRepo({
      name: "newer repository name",
      scripts: [{ ...submittedScript, command: "newer command" }],
    });
    const saved = makeRepo({
      id: toRepositoryId("persisted-repo"),
      scripts: [makeScript("persisted-script", "submitted")],
    });

    const merged = mergeSavedRepositoryDraft(current, submitted, saved);

    expect(merged.id).toBe(toRepositoryId("persisted-repo"));
    expect(merged.name).toBe("newer repository name");
    expect(merged.scripts[0].id).toBe("persisted-script");
    expect(merged.scripts[0].command).toBe("newer command");
  });
});

function makeScript(id: string, command: string) {
  return {
    id,
    repository_id: toRepositoryId("repo-1"),
    name: "script",
    command,
    position: 0,
    created_at: "",
    updated_at: "",
  };
}
