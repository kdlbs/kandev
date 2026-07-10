import { describe, expect, it } from "vitest";
import { resolveTaskRepositoryChips, type Task } from "./kanban-card";
import { repositoryId, workspaceId, type Repository } from "@/lib/types/http";

function repo(overrides: Partial<Repository>): Repository {
  return {
    id: repositoryId("repo-1"),
    workspace_id: workspaceId("workspace-1"),
    name: "",
    source_type: "github",
    local_path: "/home/carlos/.kandev/repos/NBCUDTC/olisipo-jenkins-job-dsl",
    provider: "github",
    provider_repo_id: "123",
    provider_owner: "NBCUDTC",
    provider_name: "olisipo-jenkins-job-dsl",
    default_branch: "main",
    scripts: [],
    worktree_branch_prefix: "",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    startup_prompt: "",
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function task(overrides: Partial<Task>): Task {
  return {
    id: "task-1",
    title: "Review Pull Request",
    workflowStepId: "step-1",
    repositoryId: "repo-1",
    repositories: [],
    ...overrides,
  };
}

describe("resolveTaskRepositoryChips", () => {
  it("uses the provider owner/name slug instead of the local clone path", () => {
    expect(resolveTaskRepositoryChips(task({}), [repo({})])).toEqual([
      {
        label: "NBCUDTC/olisipo-jenkins-job-dsl",
        path: "~/.kandev/repos/NBCUDTC/olisipo-jenkins-job-dsl",
      },
    ]);
  });

  it("falls back to the configured repository name for local repos", () => {
    expect(
      resolveTaskRepositoryChips(task({}), [
        repo({
          name: "olisipo-jenkins-job-dsl",
          source_type: "local",
          provider: "",
          provider_repo_id: "",
          provider_owner: "",
          provider_name: "",
        }),
      ]),
    ).toEqual([
      {
        label: "olisipo-jenkins-job-dsl",
        path: "~/.kandev/repos/NBCUDTC/olisipo-jenkins-job-dsl",
      },
    ]);
  });

  it("keeps multiple repositories in task order", () => {
    expect(
      resolveTaskRepositoryChips(
        task({
          repositoryId: "repo-1",
          repositories: [
            { id: "link-2", repository_id: "repo-2", position: 2 },
            { id: "link-1", repository_id: "repo-1", position: 1 },
          ],
        }),
        [
          repo({}),
          repo({
            id: repositoryId("repo-2"),
            local_path: "/home/carlos/src/api",
            provider_owner: "NBCUDTC",
            provider_name: "api",
          }),
        ],
      ),
    ).toEqual([
      {
        label: "NBCUDTC/olisipo-jenkins-job-dsl",
        path: "~/.kandev/repos/NBCUDTC/olisipo-jenkins-job-dsl",
      },
      { label: "NBCUDTC/api", path: "~/src/api" },
    ]);
  });

  it("renders no repo chips for repo-less local-folder tasks", () => {
    expect(
      resolveTaskRepositoryChips(
        task({
          repositoryId: undefined,
          repositories: [],
        }),
        [repo({})],
      ),
    ).toEqual([]);
  });

  it("renders no repo chips when the task has no repository information", () => {
    expect(resolveTaskRepositoryChips(task({ repositoryId: undefined }), [])).toEqual([]);
  });
});
