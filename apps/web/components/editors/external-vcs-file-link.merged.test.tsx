import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { TaskPR } from "@/lib/types/github";
import { repositoryId, taskId, workspaceId, type Repository } from "@/lib/types/http";
import { ExternalVcsFileLink } from "./external-vcs-file-link";

const WORKSPACE_ID = workspaceId("merged-link-workspace");
const TASK_ID = taskId("merged-link-task");
const REPOSITORY_ID = repositoryId("merged-link-repository");
const HEAD_SHA = "6f0a35c67de244384e927fa6b0f1e8d3bcd5a012";
const SOURCE_BRANCH = "feature/removed-after-merge";
const FILE_PATH = "src/reviewed-file.ts";

const repository: Repository = {
  id: REPOSITORY_ID,
  workspace_id: WORKSPACE_ID,
  name: "application",
  source_type: "remote",
  local_path: "",
  provider: "github",
  provider_repo_id: "merged-link-provider-repository",
  provider_host: "https://github.com",
  provider_owner: "example",
  provider_name: "application",
  remote_url: "https://github.com/example/application.git",
  default_branch: "main",
  worktree_branch_prefix: "feature/",
  pull_before_worktree: false,
  setup_script: "",
  cleanup_script: "",
  dev_script: "",
  copy_files: "",
  created_at: "",
  updated_at: "",
};

function pullRequest(state: TaskPR["state"]): TaskPR {
  return {
    id: "merged-link-pr",
    workspace_id: WORKSPACE_ID,
    task_id: TASK_ID,
    repository_id: REPOSITORY_ID,
    owner: "example",
    repo: "application",
    pr_number: 21,
    pr_url: "https://github.com/example/application/pull/21",
    pr_title: "Reviewed change",
    head_branch: SOURCE_BRANCH,
    head_sha: HEAD_SHA,
    base_branch: "main",
    author_login: "developer",
    state,
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 1,
    deletions: 0,
    created_at: "",
    merged_at: state === "merged" ? "2026-10-26T12:00:00Z" : null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
  };
}

function renderTaskLink(size: "sm" | "touch", prs: TaskPR[]) {
  const initialState = {
    ...defaultState,
    tasks: { ...defaultState.tasks, activeTaskId: TASK_ID },
    kanban: {
      ...defaultState.kanban,
      tasks: [
        {
          id: TASK_ID,
          workflowId: "workflow",
          workflowStepId: "review",
          title: "Merged task",
          position: 0,
          repositories: [
            {
              id: "merged-link-task-repository",
              repository_id: REPOSITORY_ID,
              base_branch: "main",
              position: 0,
            },
          ],
        },
      ],
    },
    repositories: {
      ...defaultState.repositories,
      itemsByWorkspaceId: { [WORKSPACE_ID]: [repository] },
    },
    taskPRs: { ...defaultState.taskPRs, byTaskId: { [TASK_ID]: prs } },
  };

  render(
    <StateProvider initialState={initialState}>
      <TooltipProvider>
        <ExternalVcsFileLink filePath={FILE_PATH} repositoryId={REPOSITORY_ID} size={size} />
      </TooltipProvider>
    </StateProvider>,
  );
  return screen.getByRole("link", { name: "Open file in GitHub" });
}

afterEach(cleanup);

// @covers AC-UI-EXTERNAL-VCS-FILE-LINKS-001.2, AC-UI-EXTERNAL-VCS-FILE-LINKS-001.9
describe.each(["sm", "touch"] as const)("task file action (%s)", (size) => {
  it("opens the saved head snapshot after the merged source branch is deleted", () => {
    const link = renderTaskLink(size, [pullRequest("merged")]);

    expect(link.getAttribute("href")).toBe(
      `https://github.com/example/application/blob/${HEAD_SHA}/${FILE_PATH}`,
    );
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noopener noreferrer");
    expect(link.classList.contains(size === "touch" ? "size-11" : "size-6")).toBe(true);
  });

  it("keeps the published source branch while the pull request is open", () => {
    const link = renderTaskLink(size, [pullRequest("open")]);

    expect(link.getAttribute("href")).toBe(
      `https://github.com/example/application/blob/${encodeURIComponent(SOURCE_BRANCH)}/${FILE_PATH}`,
    );
  });

  it("uses the task base branch without a linked pull request", () => {
    const link = renderTaskLink(size, []);

    expect(link.getAttribute("href")).toBe(
      `https://github.com/example/application/blob/main/${FILE_PATH}`,
    );
  });
});
