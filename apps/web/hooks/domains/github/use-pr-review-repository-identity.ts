"use client";

import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  resolvePRReviewRepositoryIdentity,
  type ReviewWorktreeIdentity,
} from "@/components/review/review-repository-identity";
import { useTaskRepositories } from "@/hooks/domains/kanban/use-task-repositories";
import { useRepository } from "@/hooks/domains/workspace/use-repository";
import type { Worktree } from "@/lib/state/slices/session/types";
import type { TaskPR } from "@/lib/types/github";
import type { TaskSessionWorktree } from "@/lib/types/http";

const EMPTY_WORKTREE_IDS: string[] = [];

function normalizeReviewWorktrees(
  sessionWorktrees: TaskSessionWorktree[] | undefined,
  liveWorktreeIds: string[],
  liveWorktrees: Record<string, Worktree>,
): ReviewWorktreeIdentity[] {
  const normalized: ReviewWorktreeIdentity[] = (sessionWorktrees ?? []).map((worktree) => {
    const id = worktree.worktree_id || worktree.id;
    const live = liveWorktrees[id];
    return {
      id,
      repositoryId: worktree.repository_id || live?.repositoryId,
      branchSlug: worktree.branch_slug,
      branch: worktree.worktree_branch || live?.branch,
      path: worktree.worktree_path || live?.path,
      position: worktree.position,
    };
  });
  const seen = new Set(normalized.map((worktree) => worktree.id));
  for (const id of liveWorktreeIds) {
    const worktree = liveWorktrees[id];
    if (!worktree || seen.has(id)) continue;
    normalized.push({
      id: worktree.id,
      repositoryId: worktree.repositoryId,
      branch: worktree.branch,
      path: worktree.path,
    });
  }
  return normalized;
}

export function usePRReviewRepositoryIdentity(
  taskId: string | null,
  sessionId: string | null | undefined,
  pr: TaskPR | null,
): string | undefined {
  const taskRepositories = useTaskRepositories(taskId);
  const workspaceRepository = useRepository(pr?.repository_id ?? null);
  const sessionWorktrees = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId]?.worktrees : undefined,
  );
  const liveWorktreeIds = useAppStore((state) =>
    sessionId
      ? (state.sessionWorktreesBySessionId.itemsBySessionId[sessionId] ?? EMPTY_WORKTREE_IDS)
      : EMPTY_WORKTREE_IDS,
  );
  const liveWorktrees = useAppStore((state) => state.worktrees.items);

  const worktrees = useMemo(
    () => normalizeReviewWorktrees(sessionWorktrees, liveWorktreeIds, liveWorktrees),
    [sessionWorktrees, liveWorktreeIds, liveWorktrees],
  );

  return useMemo(
    () =>
      resolvePRReviewRepositoryIdentity({
        pr,
        workspaceRepositoryName: workspaceRepository?.name,
        taskRepositories,
        worktrees,
      }),
    [pr, workspaceRepository?.name, taskRepositories, worktrees],
  );
}
