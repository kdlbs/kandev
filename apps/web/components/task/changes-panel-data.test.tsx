import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import {
  repositoryId,
  taskId,
  workflowId,
  workspaceId,
  type Repository,
  type Task,
} from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";

type MockState = {
  tasks: { activeTaskId: string | null };
  kanban: { tasks: KanbanState["tasks"] };
  taskSessions: { items: Record<string, { base_branch?: string }> };
  pendingPrUrlByTaskId: { byTaskId: Record<string, Record<string, string | undefined>> };
};

let mockState: MockState;
const TIMESTAMP = "2026-01-01T00:00:00Z";

const repositories: Repository[] = [
  {
    id: repositoryId("repo-1"),
    workspace_id: workspaceId("ws-1"),
    name: "repo-one",
    source_type: "local",
    local_path: "/repo-one",
    provider: "",
    provider_repo_id: "",
    provider_owner: "",
    provider_name: "",
    default_branch: "main",
    worktree_branch_prefix: "",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  },
];

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/hooks/domains/session/use-session-git", () => ({
  useSessionGit: () => ({
    hasAnything: false,
    hasUnstaged: false,
    hasStaged: false,
    hasCommits: false,
    canPush: false,
    canCreatePR: false,
    unstagedFiles: [],
    stagedFiles: [],
    allFiles: [],
    cumulativeDiff: null,
    commits: [],
    pendingStageFiles: new Set<string>(),
    ahead: 0,
    isLoading: false,
    loadingOperation: null,
    perRepoStatus: [],
    stage: vi.fn(),
    unstage: vi.fn(),
    stageAll: vi.fn(),
    unstageAll: vi.fn(),
    stageFile: vi.fn(() => Promise.resolve()),
    unstageFile: vi.fn(() => Promise.resolve()),
  }),
}));

vi.mock("@/hooks/use-session-file-reviews", () => ({
  useSessionFileReviews: () => ({ reviews: new Map() }),
}));

vi.mock("@/hooks/use-environment-session-id", () => ({
  useEnvironmentSessionId: () => "session-1",
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/components/vcs/vcs-dialogs", () => ({
  useVcsDialogs: () => ({
    openCommitDialog: vi.fn(),
    openPRDialog: vi.fn(),
  }),
}));

vi.mock("@/components/task/changes-panel-hooks", () => ({
  useChangesGitHandlers: () => ({
    handleGitOperation: vi.fn(),
    handlePush: vi.fn(),
    handleForcePush: vi.fn(),
    handleRevertCommit: vi.fn(),
  }),
  useChangesDialogHandlers: () => ({
    handleBulkDiscardClick: vi.fn(),
  }),
}));

vi.mock("@/hooks/domains/session/use-repo-display-name", () => ({
  useRepoDisplayName: () => undefined,
}));

vi.mock("@/hooks/domains/session/use-base-branch-by-repo", () => ({
  useBaseBranchByRepo: () => ({}),
}));

vi.mock("@/hooks/domains/workspace/use-repository-cache", () => ({
  useRepositoriesByWorkspace: () => ({ "ws-1": repositories }),
}));

vi.mock("@/hooks/domains/github/use-task-pr", () => ({
  useActiveTaskPR: () => ({
    owner: "owner",
    repo: "repo",
    pr_number: 1,
    last_synced_at: "sync",
  }),
}));

vi.mock("@/hooks/domains/github/use-active-task-pr-files", () => ({
  useActiveTaskPRsWithFiles: () => ({
    prs: [
      {
        owner: "owner",
        repo: "repo",
        pr_number: 1,
        repository_id: "repo-1",
        head_branch: "branch-a",
        last_synced_at: "sync",
        pr_url: "https://example.test/pr/1",
      },
      {
        owner: "owner",
        repo: "repo",
        pr_number: 2,
        repository_id: "repo-1",
        head_branch: "branch-b",
        last_synced_at: "sync",
        pr_url: "https://example.test/pr/2",
      },
    ],
    filesByPRKey: {
      "owner/repo/1/sync": [
        {
          filename: "a.ts",
          status: "modified",
          additions: 1,
          deletions: 0,
          patch: "@@",
        },
      ],
      "owner/repo/2/sync": [
        {
          filename: "b.ts",
          status: "modified",
          additions: 1,
          deletions: 0,
          patch: "@@",
        },
      ],
    },
  }),
}));

vi.mock("@/hooks/domains/github/use-pr-commits", () => ({
  usePRCommits: () => ({ commits: [] }),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn(() => new Promise(() => {})),
}));

import { useChangesPanelData } from "./changes-panel-data";

function makeTask(): Task {
  return {
    id: taskId("task-1"),
    workspace_id: workspaceId("ws-1"),
    workflow_id: workflowId("wf-1"),
    workflow_step_id: "step-1",
    position: 1,
    title: "Query task",
    description: "",
    state: "CREATED",
    priority: 0,
    repositories: [
      {
        id: "task-repo-1",
        task_id: taskId("task-1"),
        repository_id: repositoryId("repo-1"),
        base_branch: "main",
        position: 0,
        created_at: TIMESTAMP,
        updated_at: TIMESTAMP,
      },
      {
        id: "task-repo-2",
        task_id: taskId("task-1"),
        repository_id: repositoryId("repo-2"),
        base_branch: "main",
        position: 1,
        created_at: TIMESTAMP,
        updated_at: TIMESTAMP,
      },
    ],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

function wrapper(client: QueryClient) {
  return function TestWrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client }, children);
  };
}

describe("useChangesPanelData", () => {
  beforeEach(() => {
    mockState = {
      tasks: { activeTaskId: "task-1" },
      kanban: { tasks: [] },
      taskSessions: { items: { "session-1": { base_branch: "main" } } },
      pendingPrUrlByTaskId: { byTaskId: {} },
    };
  });

  it("uses task detail Query repositories to group multiple PRs on a multi-repo task", () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(qk.tasks.detail("task-1"), makeTask());

    const { result } = renderHook(() => useChangesPanelData(), { wrapper: wrapper(client) });

    expect(result.current.prFiles.map((file) => file.repository_name)).toEqual([
      "repo-one · branch-a",
      "repo-one · branch-b",
    ]);
  });
});
