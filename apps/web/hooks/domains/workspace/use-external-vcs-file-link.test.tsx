import { createElement, type ReactNode } from "react";
import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { repositoryId, sessionId, taskId, workspaceId, type Repository } from "@/lib/types/http";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";

const loaderMocks = vi.hoisted(() => ({
  github: vi.fn(),
  gitlab: vi.fn(),
  azure: vi.fn(),
}));

vi.mock("@/hooks/domains/github/use-task-pr", () => ({
  useTaskPR: (value: string | null) => loaderMocks.github(value),
}));
vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useWorkspaceMRs: (value: string | null) => loaderMocks.gitlab(value),
}));
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-task-pull-requests", () => ({
  useAzureDevOpsTaskPullRequests: (workspace: string | null, task: string | null) =>
    loaderMocks.azure(workspace, task),
}));

import {
  useExternalVcsFileLink,
  useExternalVcsFileLinkHydration,
} from "./use-external-vcs-file-link";

const WORKSPACE_ID = workspaceId("workspace-1");
const TASK_ID = taskId("task-1");
const SESSION_ID = sessionId("session-1");
const GITHUB_REPOSITORY_ID = "repo-github";
const GITLAB_REPOSITORY_ID = "repo-gitlab";
const FIRST_BRANCH = "feature/one";
const SECOND_BRANCH = "feature/two";

function repository(overrides: Partial<Repository> = {}): Repository {
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

function githubPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-link-1",
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

function gitlabMR(overrides: Partial<TaskMR> = {}): TaskMR {
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

function repeatedTaskRepositories() {
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

function repeatedSessionWorktrees(): NonNullable<InitialOptions["sessionWorktrees"]> {
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

function wrapper(options: InitialOptions = {}) {
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
          workflowStepId: "step-1",
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

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useExternalVcsFileLink repository and revision resolution", () => {
  it("uses an explicit repository id and its published review branch", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/app.ts",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
        }),
      { wrapper: wrapper({ prs: [githubPR()] }) },
    );

    expect(result.current).toMatchObject({
      provider: "github",
      revision: "feature/share",
      url: "https://github.com/acme/web/blob/feature%2Fshare/src/app.ts",
    });
  });

  it("resolves a multi-repository file by repository name without crossing providers", () => {
    const gitlab = repository({
      id: repositoryId(GITLAB_REPOSITORY_ID),
      name: "api",
      provider: "gitlab",
      provider_repo_id: "gitlab-api",
      provider_host: "https://gitlab.example.com",
      provider_owner: "platform",
      provider_name: "api",
      remote_url: "https://gitlab.example.com/platform/api.git",
      default_branch: "trunk",
    });
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "cmd/main.go",
          sessionId: SESSION_ID,
          repositoryName: "api",
        }),
      {
        wrapper: wrapper({
          repositories: [repository(), gitlab],
          taskRepositories: [
            {
              id: "task-repo-web",
              repository_id: GITHUB_REPOSITORY_ID,
              base_branch: "main",
              position: 0,
            },
            {
              id: "task-repo-api",
              repository_id: GITLAB_REPOSITORY_ID,
              base_branch: "trunk",
              position: 1,
            },
          ],
          prs: [githubPR()],
          mrs: [gitlabMR()],
        }),
      },
    );

    expect(result.current).toMatchObject({
      provider: "gitlab",
      revision: "feature/gitlab",
      url: "https://gitlab.example.com/platform/api/-/blob/feature%2Fgitlab/cmd/main.go",
    });
  });

  it("uses the sole legacy session repository and falls back to its task base branch", () => {
    const { result } = renderHook(
      () => useExternalVcsFileLink({ filePath: "README.md", sessionId: SESSION_ID }),
      { wrapper: wrapper({ sessionRepositoryId: GITHUB_REPOSITORY_ID }) },
    );

    expect(result.current).toMatchObject({ provider: "github", revision: "main" });
  });

  it("uses the sole linked task repository when commit detail has no session identity", () => {
    const { result } = renderHook(
      () => useExternalVcsFileLink({ filePath: "src/commit-detail.ts" }),
      { wrapper: wrapper() },
    );

    expect(result.current).toMatchObject({
      provider: "github",
      revision: "main",
      url: "https://github.com/acme/web/blob/main/src/commit-detail.ts",
    });
  });
});

describe("useExternalVcsFileLink GitHub published revisions", () => {
  it("keeps the published branch when fork provenance is unavailable", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/app.ts",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
        }),
      {
        wrapper: wrapper({
          prs: [githubPR({ head_branch: "contributor:feature/share", pr_number: 42 })],
        }),
      },
    );

    expect(result.current).toMatchObject({
      revision: "contributor:feature/share",
      url: "https://github.com/acme/web/blob/contributor%3Afeature%2Fshare/src/app.ts",
    });
  });
});

describe("useExternalVcsFileLink repeated repository matching", () => {
  it("matches a repeated repository through the named worktree's active branch", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/repeated.ts",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
          repositoryName: "web-feature-two",
        }),
      {
        wrapper: wrapper({
          taskRepositories: repeatedTaskRepositories(),
          prs: [
            githubPR({ id: "pr-one", head_branch: FIRST_BRANCH, base_branch: "main" }),
            githubPR({
              id: "pr-two",
              pr_number: 43,
              head_branch: SECOND_BRANCH,
              base_branch: "release",
            }),
          ],
          sessionWorktrees: repeatedSessionWorktrees(),
        }),
      },
    );

    expect(result.current).toMatchObject({ revision: SECOND_BRANCH });
  });

  it("does not reuse a sibling worktree's published branch", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/repeated.ts",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
          repositoryName: "web-feature-two",
        }),
      {
        wrapper: wrapper({
          taskRepositories: repeatedTaskRepositories(),
          prs: [githubPR({ head_branch: FIRST_BRANCH })],
          sessionWorktrees: repeatedSessionWorktrees(),
        }),
      },
    );

    expect(result.current?.revision).toBe("release");
  });

  it("resolves a production-shaped named worktree before repository metadata", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/editor.ts",
          sessionId: SESSION_ID,
          repositoryName: "web-feature-two",
        }),
      {
        wrapper: wrapper({
          taskRepositories: repeatedTaskRepositories(),
          prs: [
            githubPR({ id: "pr-one", head_branch: FIRST_BRANCH, base_branch: "main" }),
            githubPR({
              id: "pr-two",
              pr_number: 43,
              head_branch: SECOND_BRANCH,
              base_branch: "release",
            }),
          ],
          sessionWorktrees: repeatedSessionWorktrees(),
        }),
      },
    );

    expect(result.current).toMatchObject({
      provider: "github",
      revision: SECOND_BRANCH,
      url: "https://github.com/acme/web/blob/feature%2Ftwo/src/editor.ts",
    });
  });
});

describe("useExternalVcsFileLink ambiguity and legacy identity", () => {
  it("fails closed when repeated repository rows cannot be disambiguated", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/ambiguous.ts",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
        }),
      {
        wrapper: wrapper({
          taskRepositories: [
            { id: "one", repository_id: GITHUB_REPOSITORY_ID, base_branch: "main", position: 0 },
            {
              id: "two",
              repository_id: GITHUB_REPOSITORY_ID,
              base_branch: "release",
              position: 1,
            },
          ],
        }),
      },
    );

    expect(result.current).toBeNull();
  });

  it("fails closed when a repository name is ambiguous", () => {
    const duplicate = repository({
      id: repositoryId("repo-other"),
      provider_owner: "other",
      remote_url: "https://github.com/other/web.git",
    });
    const { result } = renderHook(
      () => useExternalVcsFileLink({ filePath: "src/app.ts", repositoryName: "web" }),
      {
        wrapper: wrapper({
          repositories: [repository(), duplicate],
          taskRepositories: [
            { id: "one", repository_id: GITHUB_REPOSITORY_ID, base_branch: "main", position: 0 },
            { id: "two", repository_id: "repo-other", base_branch: "main", position: 1 },
          ],
        }),
      },
    );

    expect(result.current).toBeNull();
  });
});

describe("useExternalVcsFileLink named worktree ambiguity", () => {
  it("fails closed when a named session worktree is ambiguous across repositories", () => {
    const otherRepositoryId = "repo-other";
    const duplicate = repository({
      id: repositoryId(otherRepositoryId),
      name: "api",
      provider_owner: "other",
      provider_name: "api",
      remote_url: "https://github.com/other/api.git",
    });
    const sharedPath = "/tmp/shared-feature";
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/ambiguous.ts",
          sessionId: SESSION_ID,
          repositoryName: "shared-feature",
        }),
      {
        wrapper: wrapper({
          repositories: [repository(), duplicate],
          taskRepositories: [
            { id: "one", repository_id: GITHUB_REPOSITORY_ID, base_branch: "main", position: 0 },
            { id: "two", repository_id: otherRepositoryId, base_branch: "main", position: 1 },
          ],
          sessionWorktrees: [
            {
              id: "worktree-one",
              worktree_id: "worktree-one",
              repository_id: repositoryId(GITHUB_REPOSITORY_ID),
              position: 0,
              worktree_path: sharedPath,
              worktree_branch: FIRST_BRANCH,
              session_id: SESSION_ID,
            },
            {
              id: "worktree-two",
              worktree_id: "worktree-two",
              repository_id: repositoryId(otherRepositoryId),
              position: 1,
              worktree_path: sharedPath,
              worktree_branch: SECOND_BRANCH,
              session_id: SESSION_ID,
            },
          ],
        }),
      },
    );

    expect(result.current).toBeNull();
  });
});

describe("useExternalVcsFileLink legacy provider identity", () => {
  it("accepts a legacy GitHub association only when provider identity matches", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/legacy.ts",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
        }),
      { wrapper: wrapper({ prs: [githubPR({ repository_id: undefined })] }) },
    );

    expect(result.current?.revision).toBe("feature/share");
  });
});

describe("useExternalVcsFileLinkHydration", () => {
  it("enables only providers attached to the task", () => {
    const gitlab = repository({
      id: repositoryId(GITLAB_REPOSITORY_ID),
      provider: "gitlab",
      provider_host: "https://gitlab.example.com",
      provider_owner: "platform",
      provider_name: "api",
      remote_url: "https://gitlab.example.com/platform/api.git",
    });
    const azure = repository({
      id: repositoryId("repo-azure"),
      provider: "azure_devops",
      provider_host: "",
      provider_owner: "Platform",
      provider_name: "api",
      remote_url: "https://dev.azure.com/acme/Platform/_git/api",
    });
    const task = {
      id: TASK_ID,
      workspace_id: WORKSPACE_ID,
      repositories: [
        {
          id: "one",
          task_id: TASK_ID,
          repository_id: repositoryId(GITHUB_REPOSITORY_ID),
          base_branch: "main",
          position: 0,
          created_at: "",
          updated_at: "",
        },
        {
          id: "two",
          task_id: TASK_ID,
          repository_id: repositoryId("repo-azure"),
          base_branch: "main",
          position: 1,
          created_at: "",
          updated_at: "",
        },
      ],
    };

    const { rerender } = renderHook(
      () => useExternalVcsFileLinkHydration(task, [repository(), gitlab, azure]),
      { wrapper: wrapper() },
    );
    rerender();

    expect(loaderMocks.github).toHaveBeenLastCalledWith(TASK_ID);
    expect(loaderMocks.gitlab).toHaveBeenLastCalledWith(null);
    expect(loaderMocks.azure).toHaveBeenLastCalledWith(WORKSPACE_ID, TASK_ID);
  });
});
