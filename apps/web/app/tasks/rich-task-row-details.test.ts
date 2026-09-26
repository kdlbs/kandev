import { describe, expect, it } from "vitest";
import type { Repository, Task } from "@/lib/types/http";
import { resolveRichTaskRowDetails } from "./rich-task-row-details";

function task(overrides: Partial<Task>): Task {
  return {
    id: "task-1",
    workspace_id: "workspace-1",
    workflow_id: "workflow-1",
    workflow_step_id: "step-1",
    position: 0,
    title: "Child task",
    description: "",
    state: "TODO",
    priority: "medium",
    created_at: "2026-07-25T00:00:00Z",
    updated_at: "2026-07-25T00:00:00Z",
    ...overrides,
  } as Task;
}

function repository(overrides: Partial<Repository>): Repository {
  return {
    id: "repo-1",
    workspace_id: "workspace-1",
    name: "",
    source_type: "github",
    local_path: "/work/kandev",
    provider: "github",
    provider_repo_id: "1",
    provider_owner: "kdlbs",
    provider_name: "kandev",
    default_branch: "main",
    ...overrides,
  } as Repository;
}

describe("resolveRichTaskRowDetails", () => {
  it("derives safe row metadata without local paths", () => {
    const details = resolveRichTaskRowDetails({
      task: task({
        description: "Explain the work clearly.",
        session_count: 2,
        review_status: "changes_requested",
        parent_id: "parent-1" as Task["id"],
        repositories: [
          { repository_id: "repo-1" as Task["id"], position: 0 },
          { repository_id: "missing-repo" as Task["id"], position: 1 },
        ] as unknown as Task["repositories"],
      }),
      repositories: [repository({ local_path: "/private/project/kandev" })],
      parentTasks: [task({ id: "parent-1" as Task["id"], title: "Parent task" })],
    });

    expect(details).toEqual({
      repositoryLabels: ["kdlbs/kandev"],
      description: "Explain the work clearly.",
      sessionCount: 2,
      parentTitle: "Parent task",
      reviewAttention: "changes_requested",
    });
  });

  it("deduplicates repeated repository links", () => {
    const details = resolveRichTaskRowDetails({
      task: task({
        repositories: [
          { repository_id: "repo-1" as Task["id"], position: 0 },
          { repository_id: "repo-1" as Task["id"], position: 1 },
        ] as unknown as Task["repositories"],
      }),
      repositories: [repository({})],
      parentTasks: [],
    });

    expect(details.repositoryLabels).toEqual(["kdlbs/kandev"]);
  });

  it("omits a repository badge when only a local path is available", () => {
    const details = resolveRichTaskRowDetails({
      task: task({
        repositories: [
          { repository_id: "repo-1" as Task["id"], position: 0 },
        ] as unknown as Task["repositories"],
      }),
      repositories: [
        repository({
          name: "",
          provider_owner: undefined,
          provider_name: undefined,
          local_path: "/private/engineering/kandev",
        }),
      ],
      parentTasks: [],
    });

    expect(details.repositoryLabels).toEqual([]);
  });
});
