import { describe, expect, it } from "vitest";
import type { Repository } from "@/lib/types/http";
import { repositoryId, workspaceId } from "@/lib/types/ids";
import type { TaskRepositorySelection } from "./task-create-dialog-types";
import { applyRepositorySet } from "./task-create-dialog-repository-sets";
import { buildCreateTaskPayload, buildWorkspaceSourcesPayload } from "./task-create-dialog-helpers";
import { sourceMenuLabel } from "./task-create-dialog-workspace-source-menu";

const translate = (key: string, options?: { count: number }) =>
  options ? `${key}:${options.count}` : key;
const EXISTING_REPOSITORY_ID = "repo-existing";
const LOCAL_REPOSITORY_ID = "repo-local";

describe("workspace source serialization contract", () => {
  it("serializes mixed selections in their visible order", () => {
    const selections: TaskRepositorySelection[] = [
      { kind: "local", key: "repo", repositoryId: LOCAL_REPOSITORY_ID, branch: "feature/local" },
      { kind: "folder", key: "folder", localPath: "/work/assets", displayName: "assets" },
      {
        kind: "remote",
        key: "remote",
        url: "https://github.com/acme/api",
        branch: "main",
        source: "paste",
      },
    ];

    const payload = buildWorkspaceSourcesPayload({
      selections,
      useRemote: false,
      remoteRepos: [],
      repositories: [],
      discoveredRepositories: [],
      workspaceRepositories: [
        { id: LOCAL_REPOSITORY_ID, default_branch: "main" } as unknown as Repository,
      ],
      isLocalExecutor: true,
    });

    expect(payload).toEqual([
      {
        kind: "repository",
        repository_id: LOCAL_REPOSITORY_ID,
        base_branch: "main",
        checkout_branch: "feature/local",
      },
      { kind: "folder", local_path: "/work/assets", display_name: "assets" },
      { kind: "repository", base_branch: "main", github_url: "https://github.com/acme/api" },
    ]);
  });

  it("keeps explicit empty contents on the new field", () => {
    const payload = buildCreateTaskPayload({
      workspaceId: "workspace-1",
      effectiveWorkflowId: "workflow-1",
      trimmedTitle: "Scratch task",
      trimmedDescription: "Use an empty workspace",
      repositoriesPayload: [{ repository_id: "legacy", base_branch: "main" }],
      workspaceSourcesPayload: [],
      agentProfileId: "agent-1",
      executorId: "executor-1",
      executorProfileId: "profile-1",
      withAgent: false,
    });

    expect(payload.workspace_sources).toEqual([]);
    expect(payload).not.toHaveProperty("repositories");
    expect(payload).not.toHaveProperty("workspace_path");
  });

  it("carries fresh branch metadata through ordered repository sources", () => {
    const payload = buildWorkspaceSourcesPayload({
      selections: [
        { kind: "local", key: "repo", repositoryId: LOCAL_REPOSITORY_ID, branch: "develop" },
      ],
      useRemote: false,
      remoteRepos: [],
      repositories: [],
      discoveredRepositories: [],
      workspaceRepositories: [
        { id: LOCAL_REPOSITORY_ID, default_branch: "main" } as unknown as Repository,
      ],
      isLocalExecutor: true,
      freshBranch: {
        confirmDiscard: true,
        consentedDirtyFiles: ["src/app.ts"],
      },
    });

    expect(payload).toEqual([
      {
        kind: "repository",
        repository_id: LOCAL_REPOSITORY_ID,
        base_branch: "develop",
        fresh_branch: true,
        confirm_discard: true,
        consented_dirty_files: ["src/app.ts"],
      },
    ]);
  });
});

describe("workspace contents set actions", () => {
  it("uses the contextual Add labels and appends sets without losing folder rows", () => {
    expect(sourceMenuLabel(translate, 0)).toBe("task:addRepositoryFolder");
    expect(sourceMenuLabel(translate, 2)).toBe("task:add");

    const folder: TaskRepositorySelection = {
      kind: "folder",
      key: "folder",
      localPath: "/work/assets",
    };
    const existing = [{ key: "repo", repositoryId: EXISTING_REPOSITORY_ID, branch: "feature" }];
    const result = applyRepositorySet({
      rows: existing,
      set: {
        id: "set-1",
        workspace_id: workspaceId("workspace-1"),
        name: "Full stack",
        description: "",
        repositories: [
          { repository_id: repositoryId(EXISTING_REPOSITORY_ID), position: 0 },
          { repository_id: repositoryId("repo-added"), position: 1, base_branch: "develop" },
        ],
        created_at: "",
        updated_at: "",
      },
      repositories: [
        { id: EXISTING_REPOSITORY_ID } as unknown as Repository,
        { id: "repo-added" } as unknown as Repository,
      ],
    });

    const next = [folder, ...result.rows.map((row) => ({ kind: "local" as const, ...row }))];
    expect(next.map((selection) => selection.kind)).toEqual(["folder", "local", "local"]);
    expect(next[0]).toBe(folder);
    expect(result.rows[0]).toMatchObject({
      repositoryId: EXISTING_REPOSITORY_ID,
      branch: "feature",
    });
    expect(result.rows[1]).toMatchObject({ repositoryId: "repo-added", baseBranch: "develop" });
  });
});
