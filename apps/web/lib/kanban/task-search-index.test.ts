import { describe, expect, it } from "vitest";
import {
  buildTaskVcsSearchIndex,
  getTaskPRsByTaskIdForCurrentWorkspace,
} from "@/lib/kanban/task-search-index";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";

function makePR(overrides: Partial<TaskPR> & { task_id: string; pr_number: number }): TaskPR {
  return {
    id: `pr-${overrides.pr_number}`,
    workspace_id: "ws-1",
    owner: "acme",
    repo: "widgets",
    pr_url: "",
    pr_title: "",
    head_branch: "",
    base_branch: "",
    ...overrides,
  } as TaskPR;
}

function makeMR(overrides: Partial<TaskMR> & { task_id: string; mr_iid: number }): TaskMR {
  return {
    id: `mr-${overrides.mr_iid}`,
    host: "gitlab.com",
    project_path: "acme/widgets",
    mr_url: "",
    mr_title: "",
    head_branch: "",
    base_branch: "",
    author_username: "",
    state: "open",
    approval_state: "",
    pipeline_state: "",
    merge_status: "",
    draft: false,
    ...overrides,
  } as TaskMR;
}

describe("buildTaskVcsSearchIndex", () => {
  it("indexes a GitHub-only task by its PR number", () => {
    const index = buildTaskVcsSearchIndex(
      { "task-1": [makePR({ task_id: "task-1", pr_number: 3315 })] },
      {},
    );

    expect(index["task-1"]).toContain("#3315");
  });

  it("indexes a GitLab-only task by its MR iid", () => {
    const index = buildTaskVcsSearchIndex(
      {},
      { "task-1": [makeMR({ task_id: "task-1", mr_iid: 42 })] },
    );

    expect(index["task-1"]).toContain("#42");
  });

  it("merges both PR and MR tokens for a task linked to both", () => {
    const index = buildTaskVcsSearchIndex(
      { "task-1": [makePR({ task_id: "task-1", pr_number: 3315 })] },
      { "task-1": [makeMR({ task_id: "task-1", mr_iid: 42 })] },
    );

    expect(index["task-1"]).toContain("#3315");
    expect(index["task-1"]).toContain("#42");
  });

  it("includes every linked PR for a multi-repo task", () => {
    const index = buildTaskVcsSearchIndex(
      {
        "task-1": [
          makePR({ task_id: "task-1", pr_number: 100, repository_id: "repo-a" }),
          makePR({ task_id: "task-1", pr_number: 200, repository_id: "repo-b" }),
        ],
      },
      {},
    );

    expect(index["task-1"]).toContain("#100");
    expect(index["task-1"]).toContain("#200");
  });

  it("omits an entry for a task with no linked PRs or MRs", () => {
    const index = buildTaskVcsSearchIndex({ "task-1": [] }, {});

    expect(index["task-1"]).toBeUndefined();
  });

  it("returns an empty index for missing/empty inputs", () => {
    expect(buildTaskVcsSearchIndex({}, {})).toEqual({});
  });

  it("includes the status-summary PR number when association data is unavailable", () => {
    const index = buildTaskVcsSearchIndex(
      {},
      {},
      { "task-1": { pull_request: { number: 3295 } } as never },
    );

    expect(index["task-1"]).toBe("#3295");
  });
});

describe("getTaskPRsByTaskIdForCurrentWorkspace", () => {
  const byTaskId = { "task-1": [makePR({ task_id: "task-1", pr_number: 3315 })] };

  it("returns the PR map when its scope matches the active workspace", () => {
    const taskPRs = {
      byTaskId,
      workspaceId: "ws-1",
      workspaceContextGeneration: 4,
    };

    expect(getTaskPRsByTaskIdForCurrentWorkspace(taskPRs, "ws-1", 4)).toBe(byTaskId);
  });

  it("returns an empty map when its workspace scope is stale", () => {
    const taskPRs = {
      byTaskId,
      workspaceId: "ws-1",
      workspaceContextGeneration: 4,
    };

    expect(getTaskPRsByTaskIdForCurrentWorkspace(taskPRs, "ws-2", 5)).toEqual({});
  });
});
