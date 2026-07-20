import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { qk } from "@/lib/query/keys";
import type { Worktree } from "@/lib/state/slices/session/types";
import {
  repositoryId,
  sessionId as toSessionId,
  taskId as toTaskId,
  type TaskSession,
} from "@/lib/types/http";
import { useSessionWorktrees } from "./use-session-worktrees";

const SESSION_ID = toSessionId("session-1");
const TASK_ID = toTaskId("task-1");
const REPOSITORY_ID = repositoryId("repo-1");
const PRIMARY_WORKTREE_PATH = "/tmp/kandev/worktrees/primary-worktree";
const SIBLING_WORKTREE_PATH = "/tmp/kandev/worktrees/sibling-worktree";
const TIMESTAMP = "2026-06-24T00:00:00Z";

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
}

function wrapper(client: QueryClient) {
  return function TestWrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client }, children);
  };
}

function makeSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    state: "RUNNING",
    started_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

describe("useSessionWorktrees", () => {
  it("prepends the primary session worktree when the passive cache only has a sibling", () => {
    const queryClient = makeQueryClient();
    const primary: Worktree = {
      id: "primary-worktree",
      sessionId: SESSION_ID,
      repositoryId: REPOSITORY_ID,
      path: PRIMARY_WORKTREE_PATH,
      branch: "main",
    };
    const sibling: Worktree = {
      id: "sibling-worktree",
      sessionId: SESSION_ID,
      repositoryId: REPOSITORY_ID,
      path: SIBLING_WORKTREE_PATH,
      branch: "feature/sibling",
    };
    queryClient.setQueryData(
      qk.taskSession.byId(SESSION_ID),
      makeSession({
        repository_id: REPOSITORY_ID,
        worktree_id: primary.id,
        worktree_path: PRIMARY_WORKTREE_PATH,
        worktree_branch: primary.branch,
      }),
    );
    queryClient.setQueryData(qk.sessionRuntime.worktrees(SESSION_ID), [sibling]);

    const { result } = renderHook(() => useSessionWorktrees(SESSION_ID), {
      wrapper: wrapper(queryClient),
    });

    expect(result.current).toEqual([primary, sibling]);
  });
});
