import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider, useAppStore } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";
import {
  repositoryId,
  sessionId,
  workspaceId,
  type Repository,
  type TaskSession,
} from "@/lib/types/http";
import { usePRReviewRepositoryIdentity } from "./use-pr-review-repository-identity";

afterEach(cleanup);

const TASK_ID = "task-1";
const SESSION_ID = sessionId("session-1");
const REPOSITORY_ID = repositoryId("repo-widgets");
const WORKSPACE_ID = workspaceId("workspace-1");
const REPOSITORY_NAME = "widgets";
const SIBLING_BRANCH = "feat/second";
const NOW = "2026-07-22T00:00:00Z";
const selectedPR: TaskPR = {
  id: "pr-2",
  task_id: TASK_ID,
  repository_id: REPOSITORY_ID,
  owner: "acme",
  repo: REPOSITORY_NAME,
  pr_number: 42,
  pr_url: "https://github.com/acme/widgets/pull/42",
  pr_title: "Sibling branch",
  head_branch: SIBLING_BRANCH,
  base_branch: "main",
  author_login: "reviewer",
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
  additions: 1,
  deletions: 0,
  created_at: NOW,
  merged_at: null,
  closed_at: null,
  last_synced_at: null,
  updated_at: NOW,
};

const repository: Repository = {
  id: REPOSITORY_ID,
  workspace_id: WORKSPACE_ID,
  name: REPOSITORY_NAME,
  source_type: "local",
  local_path: "/repos/widgets",
  provider: "github",
  provider_repo_id: REPOSITORY_NAME,
  provider_owner: "acme",
  provider_name: REPOSITORY_NAME,
  default_branch: "main",
  worktree_branch_prefix: "",
  pull_before_worktree: false,
  setup_script: "",
  cleanup_script: "",
  dev_script: "",
  copy_files: "",
  created_at: NOW,
  updated_at: NOW,
};

const initialState: Partial<AppState> = {
  kanban: {
    workflowId: null,
    steps: [],
    tasks: [
      {
        id: TASK_ID,
        workflowStepId: "step-1",
        title: "Multi-PR task",
        position: 0,
        repositories: [
          {
            id: "task-repo-1",
            repository_id: REPOSITORY_ID,
            base_branch: "main",
            checkout_branch: "feat/first",
            position: 0,
          },
          {
            id: "task-repo-2",
            repository_id: REPOSITORY_ID,
            base_branch: "main",
            checkout_branch: SIBLING_BRANCH,
            position: 1,
          },
        ],
      },
    ],
  },
  repositories: {
    itemsByWorkspaceId: { [WORKSPACE_ID]: [repository] },
    loadingByWorkspaceId: {},
    loadedByWorkspaceId: { [WORKSPACE_ID]: true },
  },
  taskSessions: {
    items: {
      [SESSION_ID]: {
        id: SESSION_ID,
        task_id: TASK_ID,
        state: "RUNNING",
        worktrees: [
          {
            id: "association-2",
            session_id: SESSION_ID,
            worktree_id: "worktree-2",
            repository_id: REPOSITORY_ID,
            worktree_branch: SIBLING_BRANCH,
            worktree_path: "/tasks/example/widgets-feat-second",
            position: 1,
          },
        ],
      } as TaskSession,
    },
  },
};

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, { initialState, children });
}

describe("usePRReviewRepositoryIdentity", () => {
  it("normalizes boot session worktrees to the selected branch directory", () => {
    const { result } = renderHook(
      () => usePRReviewRepositoryIdentity(TASK_ID, SESSION_ID, selectedPR),
      { wrapper },
    );

    expect(result.current).toBe("widgets-feat-second");
  });

  it("uses a live worktree when boot metadata has not arrived", () => {
    const { result } = renderHook(
      () => {
        const setWorktree = useAppStore((state) => state.setWorktree);
        const setSessionWorktrees = useAppStore((state) => state.setSessionWorktrees);
        return {
          identity: usePRReviewRepositoryIdentity(TASK_ID, SESSION_ID, selectedPR),
          setWorktree,
          setSessionWorktrees,
        };
      },
      {
        wrapper: ({ children }) =>
          createElement(StateProvider, {
            initialState: {
              ...initialState,
              taskSessions: { items: {} },
            },
            children,
          }),
      },
    );

    act(() => {
      result.current.setWorktree({
        id: "live-worktree-2",
        sessionId: SESSION_ID,
        repositoryId: REPOSITORY_ID,
        branch: SIBLING_BRANCH,
        path: "/tasks/example/widgets-live-second",
      });
      result.current.setSessionWorktrees(SESSION_ID, ["live-worktree-2"]);
    });

    expect(result.current.identity).toBe("widgets-live-second");
  });

  it("fills partial boot metadata from the same live worktree ID", () => {
    const partialSession = {
      ...(initialState.taskSessions?.items[SESSION_ID] as TaskSession),
      worktrees: [
        {
          id: "association-2",
          session_id: SESSION_ID,
          worktree_id: "worktree-2",
          worktree_branch: "",
          worktree_path: "",
          position: 1,
        },
      ],
    } as TaskSession;
    const { result } = renderHook(
      () => {
        const setWorktree = useAppStore((state) => state.setWorktree);
        return {
          identity: usePRReviewRepositoryIdentity(TASK_ID, SESSION_ID, selectedPR),
          setWorktree,
        };
      },
      {
        wrapper: ({ children }) =>
          createElement(StateProvider, {
            initialState: {
              ...initialState,
              taskSessions: { items: { [SESSION_ID]: partialSession } },
            },
            children,
          }),
      },
    );

    act(() => {
      result.current.setWorktree({
        id: "worktree-2",
        sessionId: SESSION_ID,
        repositoryId: REPOSITORY_ID,
        branch: SIBLING_BRANCH,
        path: "/tasks/example/widgets-live-hydrated",
      });
    });

    expect(result.current.identity).toBe("widgets-live-hydrated");
  });
});
