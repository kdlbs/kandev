import { createElement, type ReactNode } from "react";
import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { repositoryId, sessionId, taskId, workspaceId, type Repository } from "@/lib/types/http";
import type { TaskMR } from "@/lib/types/gitlab";
import * as externalVcsFileURL from "@/lib/utils/external-vcs-file-url";

vi.mock("@/hooks/domains/github/use-task-pr", () => ({ useTaskPR: vi.fn() }));
vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({ useWorkspaceMRs: vi.fn() }));
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-task-pull-requests", () => ({
  useAzureDevOpsTaskPullRequests: vi.fn(),
}));

import { useExternalVcsFileLink } from "./use-external-vcs-file-link";

const WORKSPACE_ID = workspaceId("workspace-1");
const TASK_ID = taskId("task-1");
const SESSION_ID = sessionId("session-1");
const GITLAB_REPOSITORY_ID = "repo-gitlab";

function gitlabRepository(providerHost: string): Repository {
  return {
    id: repositoryId(GITLAB_REPOSITORY_ID),
    workspace_id: WORKSPACE_ID,
    name: "api",
    source_type: "remote",
    local_path: "",
    provider: "gitlab",
    provider_repo_id: "gitlab-api",
    provider_host: providerHost,
    provider_owner: "platform",
    provider_name: "api",
    remote_url: "https://gitlab.example.com/platform/api.git",
    default_branch: "trunk",
    worktree_branch_prefix: "kandev/",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: "",
    updated_at: "",
  };
}

function legacyMergeRequest(host: string): TaskMR {
  return {
    id: "mr-link-1",
    task_id: TASK_ID,
    repository_id: undefined,
    host,
    project_path: "platform/api",
    mr_iid: 7,
    mr_url: "https://gitlab.example.com/platform/api/-/merge_requests/7",
    mr_title: "Share links",
    head_branch: "feature/gitlab",
    base_branch: "trunk",
    author_username: "ada",
    state: "open",
    approval_state: "",
    pipeline_state: "",
    merge_status: "",
    draft: false,
    approval_count: 0,
    required_approvals: 0,
    pipeline_jobs_total: 0,
    pipeline_jobs_pass: 0,
    created_at: "",
    updated_at: "",
  };
}

function wrapper(repository: Repository, mr: TaskMR) {
  const initialState = {
    ...defaultState,
    tasks: { ...defaultState.tasks, activeTaskId: TASK_ID, activeSessionId: SESSION_ID },
    kanban: {
      ...defaultState.kanban,
      tasks: [
        {
          id: TASK_ID,
          workflowStepId: "step-1",
          title: "External links",
          position: 0,
          repositories: [
            {
              id: "task-repo-api",
              repository_id: GITLAB_REPOSITORY_ID,
              base_branch: "trunk",
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
    taskSessions: {
      items: {
        [SESSION_ID]: {
          id: SESSION_ID,
          task_id: TASK_ID,
          state: "RUNNING" as const,
          started_at: "",
          updated_at: "",
        },
      },
    },
    taskMRs: { ...defaultState.taskMRs, byWorkspaceId: { [WORKSPACE_ID]: { [TASK_ID]: [mr] } } },
  };
  return ({ children }: { children: ReactNode }) =>
    createElement(StateProvider, { initialState, children });
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("useExternalVcsFileLink legacy GitLab identity", () => {
  it.each([
    ["", ""],
    ["gitlab.example.com", "gitlab.example.com"],
    ["not a valid URL", "not a valid URL"],
  ])("rejects a legacy association with non-origin hosts %j and %j", (mrHost, repositoryHost) => {
    vi.spyOn(externalVcsFileURL, "resolveExternalVcsFileURL").mockImplementation((input) =>
      input.publishedBranch
        ? {
            provider: "gitlab",
            url: "https://example.com",
            path: input.path,
            revision: input.publishedBranch,
          }
        : null,
    );
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/legacy.ts",
          sessionId: SESSION_ID,
          repositoryId: GITLAB_REPOSITORY_ID,
        }),
      { wrapper: wrapper(gitlabRepository(repositoryHost), legacyMergeRequest(mrHost)) },
    );
    expect(result.current).toBeNull();
  });
});
