import { createElement, type ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { repositoryId, sessionId, taskId, workspaceId, type Repository } from "@/lib/types/http";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";

export const WORKSPACE_ID = workspaceId("workspace-1");
export const TASK_ID = taskId("task-1");
export const SESSION_ID = sessionId("session-1");
export const GITHUB_REPOSITORY_ID = "repo-github";
export const GITLAB_REPOSITORY_ID = "repo-gitlab";
export const FIRST_BRANCH = "feature/one";
export const SECOND_BRANCH = "feature/two";

export function repository(overrides: Partial<Repository> = {}): Repository {
  return {
    id: repositoryId(GITHUB_REPOSITORY_ID),
    workspace_id: WORKSPACE_ID,
    name: "web",
    source_type: "remote",
    local_path: "",
    provider: "github",
    provider_repo_id: "provider-repo-1",
    provider_host: "https://github.com",
    provider_owner: "acme",
    provider_name: "web",
    remote_url: "https://github.com/acme/web.git",
    default_branch: "main",
    worktree_branch_prefix: "kandev/",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

export function githubPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-link-1",
    workspace_id: WORKSPACE_ID,
    task_id: TASK_ID,
    repository_id: GITHUB_REPOSITORY_ID,
    owner: "acme",
    repo: "web",
    pr_number: 42,
    pr_url: "https://github.com/acme/web/pull/42",
    pr_title: "Share links",
    head_branch: "feature/share",
    base_branch: "main",
    author_login: "ada",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
    ...overrides,
  };
}

export function gitlabMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "mr-link-1",
    task_id: TASK_ID,
    repository_id: GITLAB_REPOSITORY_ID,
    host: "https://gitlab.example.com",
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
    reviewer_count: 0,
    unapproved_reviewers: 0,
    unresolved_discussions: 0,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

type InitialOptions = {
  repositories?: Repository[];
  taskRepositories?: Array<{
    id: string;
    repository_id: string;
    base_branch: string;
    checkout_branch?: string;
    position: number;
  }>;
  prs?: TaskPR[];
  mrs?: TaskMR[];
  sessionRepositoryId?: string;
  sessionWorktrees?: Array<{
    id: string;
    worktree_id: string;
    repository_id: ReturnType<typeof repositoryId>;
    position: number;
    worktree_path: string;
    worktree_branch: string;
    session_id: ReturnType<typeof sessionId>;
  }>;
};

export function repeatedTaskRepositories() {
  return [
    {
      id: "task-repo-one",
      repository_id: GITHUB_REPOSITORY_ID,
      base_branch: "main",
      checkout_branch: FIRST_BRANCH,
      position: 0,
    },
    {
      id: "task-repo-two",
      repository_id: GITHUB_REPOSITORY_ID,
      base_branch: "release",
      checkout_branch: SECOND_BRANCH,
      position: 1,
    },
  ];
}

export function repeatedSessionWorktrees(): NonNullable<InitialOptions["sessionWorktrees"]> {
  return [
    {
      id: "session-worktree-one",
      session_id: SESSION_ID,
      worktree_id: "worktree-one",
      repository_id: repositoryId(GITHUB_REPOSITORY_ID),
      position: 0,
      worktree_path: "/tmp/web-feature-one",
      worktree_branch: FIRST_BRANCH,
    },
    {
      id: "session-worktree-two",
      session_id: SESSION_ID,
      worktree_id: "worktree-two",
      repository_id: repositoryId(GITHUB_REPOSITORY_ID),
      position: 1,
      worktree_path: "/tmp/web-feature-two",
      worktree_branch: SECOND_BRANCH,
    },
  ];
}

export function wrapper(options: InitialOptions = {}) {
  const repositories = options.repositories ?? [repository()];
  const taskRepositories = options.taskRepositories ?? [
    {
      id: "task-repo-1",
      repository_id: GITHUB_REPOSITORY_ID,
      base_branch: "main",
      position: 0,
    },
  ];
  const initialState = {
    ...defaultState,
    tasks: { ...defaultState.tasks, activeTaskId: TASK_ID, activeSessionId: SESSION_ID },
    kanban: {
      ...defaultState.kanban,
      tasks: [
        {
          id: TASK_ID,
          workflowId: "wf-1",
          workflowStepId: "step-1",
          // i18n-exempt: Synthetic task title in a test-only state fixture.
          title: "External links",
          position: 0,
          repositories: taskRepositories,
        },
      ],
    },
    repositories: {
      ...defaultState.repositories,
      itemsByWorkspaceId: { [WORKSPACE_ID]: repositories },
    },
    taskSessions: {
      items: {
        [SESSION_ID]: {
          id: SESSION_ID,
          task_id: TASK_ID,
          repository_id: options.sessionRepositoryId
            ? repositoryId(options.sessionRepositoryId)
            : undefined,
          worktrees: options.sessionWorktrees,
          state: "RUNNING" as const,
          started_at: "",
          updated_at: "",
        },
      },
    },
    taskPRs: { ...defaultState.taskPRs, byTaskId: { [TASK_ID]: options.prs ?? [] } },
    taskMRs: {
      ...defaultState.taskMRs,
      byWorkspaceId: { [WORKSPACE_ID]: { [TASK_ID]: options.mrs ?? [] } },
    },
  };
  return ({ children }: { children: ReactNode }) =>
    createElement(StateProvider, { initialState, children });
}
